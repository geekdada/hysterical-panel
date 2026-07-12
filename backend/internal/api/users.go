package api

import (
	"fmt"
	"math"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

type userInput struct {
	Email      *string `json:"email"`
	Password   *string `json:"password"`
	AuthString *string `json:"auth_string"`
	Role       *string `json:"role"`
	QuotaBytes *int64  `json:"quota_bytes"`
	Status     *string `json:"status"`
}

func (h *Handlers) getUser(e *core.RequestEvent) error {
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	ignored := h.loadIgnoredConnectionIPSet()
	public := publicUser(u, h.ipLookup, ignored)
	onlineDevices, err := h.onlineDevicesForUser(u.Id)
	if err != nil {
		return apis.NewBadRequestError("failed to load online devices", err)
	}
	public["online_devices"] = onlineDevices
	return ok(e, public)
}

func (h *Handlers) onlineDevicesForUser(userID string) (*int64, error) {
	nodes, err := h.nodesForUser(userID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		zero := int64(0)
		return &zero, nil
	}

	visibleNodeIDs := make(map[string]struct{}, len(nodes))
	hasObservation := false
	for _, node := range nodes {
		visibleNodeIDs[node.Id] = struct{}{}
		if !node.GetDateTime("online_devices_observed_at").IsZero() {
			hasObservation = true
		}
	}
	if !hasObservation {
		return nil, nil
	}

	counts, err := h.app.FindRecordsByFilter(
		"online_device_counts",
		"user = {:user}",
		"",
		0,
		0,
		map[string]any{"user": userID},
	)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, count := range counts {
		if _, visible := visibleNodeIDs[count.GetString("node")]; !visible {
			continue
		}
		value := int64(count.GetInt("count"))
		if value < 0 || value > math.MaxInt64-total {
			return nil, fmt.Errorf("invalid stored online device count")
		}
		total += value
	}
	return &total, nil
}

// newUserParams carries resolved field values for a new users record. Callers
// resolve their own inputs (client-supplied vs system-generated) and any
// role/status validation first; this helper only builds the record with the
// shared defaults (trimmed email, zeroed usage counters). The record is
// returned unsaved so callers keep their own Save error message and side effects.
type newUserParams struct {
	Email      string
	Password   string
	AuthString string
	Role       string
	Status     string
	Verified   bool
	QuotaBytes *int64 // nil leaves the field unset
}

func (h *Handlers) newUserRecord(p newUserParams) (*core.Record, error) {
	coll, err := h.app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}
	u := core.NewRecord(coll)
	u.SetEmail(strings.TrimSpace(p.Email))
	u.SetPassword(p.Password)
	u.SetVerified(p.Verified)
	u.Set("auth_string", p.AuthString)
	u.Set("role", p.Role)
	u.Set("status", p.Status)
	if p.QuotaBytes != nil {
		u.Set("quota_bytes", *p.QuotaBytes)
	}
	u.Set("used_tx", 0)
	u.Set("used_rx", 0)
	return u, nil
}

// createUser provisions a user. Only email is required: an admin can quick-create
// an account by email alone, in which case the password and auth_string are
// system-generated (the same scheme as self-registration and the management API).
// Callers may still supply an explicit password and/or auth_string. Accounts are
// always created verified, defaulting to role=user / status=active.
func (h *Handlers) createUser(e *core.RequestEvent) error {
	var in userInput
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	email := ""
	if in.Email != nil {
		email = strings.TrimSpace(*in.Email)
	}
	if email == "" {
		return apis.NewBadRequestError("email is required", nil)
	}
	role := strOr(in.Role, "user")
	if !validUserRole(role) {
		return apis.NewBadRequestError("role must be admin or user", nil)
	}
	status := strOr(in.Status, "active")
	if !validUserStatus(status) {
		return apis.NewBadRequestError("status must be active or disabled", nil)
	}

	password := ""
	if in.Password != nil {
		password = *in.Password
	}
	if password == "" {
		generated, err := token.Alphanumeric(24)
		if err != nil {
			return apis.NewBadRequestError("failed to provision account", err)
		}
		password = generated
	}

	authString := ""
	if in.AuthString != nil {
		authString = strings.TrimSpace(*in.AuthString)
	}
	if authString == "" {
		generated, err := h.generateUniqueAuthString()
		if err != nil {
			return apis.NewBadRequestError("failed to provision account", err)
		}
		authString = generated
	}

	u, err := h.newUserRecord(newUserParams{
		Email:      email,
		Password:   password,
		AuthString: authString,
		Role:       role,
		Status:     status,
		Verified:   true,
		QuotaBytes: in.QuotaBytes,
	})
	if err != nil {
		return err
	}
	if err := h.app.Save(u); err != nil {
		return apis.NewBadRequestError("failed to create user (email or auth_string may be taken)", err)
	}
	ignored := h.loadIgnoredConnectionIPSet()
	return ok(e, publicUser(u, h.ipLookup, ignored))
}

func (h *Handlers) updateUser(e *core.RequestEvent) error {
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	var in userInput
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	// Capture the pre-mutation status so we can detect an active -> disabled
	// transition after Save. That transition triggers a best-effort /kick
	// fan-out to drop currently-established Hysteria sessions; reconnects are
	// already denied by hysteriaAuth's 403 for disabled users.
	wasActive := u.GetString("status") == "active"
	if in.Email != nil {
		u.SetEmail(*in.Email)
	}
	if in.Password != nil && *in.Password != "" {
		u.SetPassword(*in.Password)
	}
	if in.AuthString != nil {
		if *in.AuthString == "" {
			return apis.NewBadRequestError("auth_string cannot be empty", nil)
		}
		u.Set("auth_string", *in.AuthString)
	}
	if in.Role != nil {
		if !validUserRole(*in.Role) {
			return apis.NewBadRequestError("role must be admin or user", nil)
		}
		u.Set("role", *in.Role)
	}
	if in.Status != nil {
		if !validUserStatus(*in.Status) {
			return apis.NewBadRequestError("status must be active or disabled", nil)
		}
		u.Set("status", *in.Status)
	}
	if in.QuotaBytes != nil {
		u.Set("quota_bytes", *in.QuotaBytes)
	}
	if err := h.app.Save(u); err != nil {
		return apis.NewBadRequestError("failed to update user", err)
	}
	if wasActive && u.GetString("status") == "disabled" {
		go h.kickUser(u.Id, u.GetString("auth_string"))
	}
	ignored := h.loadIgnoredConnectionIPSet()
	return ok(e, publicUser(u, h.ipLookup, ignored))
}

func (h *Handlers) deleteUser(e *core.RequestEvent) error {
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	if err := h.app.Delete(u); err != nil {
		return apis.NewBadRequestError("failed to delete user", err)
	}
	return ok(e, map[string]any{"deleted": true})
}

// resetUserAuthString rotates a user's Hysteria auth key. The new value is
// server-generated (the client never supplies one) and the existing usage
// counters / traffic history are preserved; the old key simply stops matching
// on the next Hysteria connect callback, and the collector resumes tracking
// the user under the new key on its next poll.
func (h *Handlers) resetUserAuthString(e *core.RequestEvent) error {
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	authString, err := h.generateUniqueAuthString()
	if err != nil {
		return apis.NewBadRequestError("failed to reset auth key", err)
	}
	u.Set("auth_string", authString)
	if err := h.app.Save(u); err != nil {
		return apis.NewBadRequestError("failed to reset auth key", err)
	}
	ignored := h.loadIgnoredConnectionIPSet()
	return ok(e, publicUser(u, h.ipLookup, ignored))
}

func strOr(p *string, def string) string {
	if p != nil && *p != "" {
		return *p
	}
	return def
}

// validUserRole mirrors the users.role select values. Keep in sync with the
// migration that defines the field.
func validUserRole(s string) bool {
	return s == "admin" || s == "user"
}

// validUserStatus mirrors the users.status select values. Keep in sync with the
// migration that defines the field.
func validUserStatus(s string) bool {
	return s == "active" || s == "disabled"
}
