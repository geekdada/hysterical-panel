package api

import (
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type userInput struct {
	Email      *string `json:"email"`
	Password   *string `json:"password"`
	AuthString *string `json:"auth_string"`
	Role       *string `json:"role"`
	QuotaBytes *int64  `json:"quota_bytes"`
	Status     *string `json:"status"`
	Enabled    *bool   `json:"enabled"`
}

func (h *Handlers) listUsers(e *core.RequestEvent) error {
	users, err := h.app.FindRecordsByFilter("users", "", "-created", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list users", err)
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, publicUser(u))
	}
	return ok(e, out)
}

func (h *Handlers) getUser(e *core.RequestEvent) error {
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	return ok(e, publicUser(u))
}

func (h *Handlers) createUser(e *core.RequestEvent) error {
	var in userInput
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if in.Email == nil || *in.Email == "" || in.Password == nil || *in.Password == "" ||
		in.AuthString == nil || *in.AuthString == "" {
		return apis.NewBadRequestError("email, password and auth_string are required", nil)
	}
	coll, err := h.app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	u := core.NewRecord(coll)
	u.SetEmail(*in.Email)
	u.SetPassword(*in.Password)
	u.SetVerified(true)
	u.Set("auth_string", *in.AuthString)
	u.Set("role", strOr(in.Role, "admin"))
	u.Set("status", strOr(in.Status, "active"))
	if in.QuotaBytes != nil {
		u.Set("quota_bytes", *in.QuotaBytes)
	}
	u.Set("enabled", boolOr(in.Enabled, true))
	u.Set("used_tx", 0)
	u.Set("used_rx", 0)
	if err := h.app.Save(u); err != nil {
		return apis.NewBadRequestError("failed to create user (email or auth_string may be taken)", err)
	}
	return ok(e, publicUser(u))
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
		u.Set("role", *in.Role)
	}
	if in.Status != nil {
		u.Set("status", *in.Status)
	}
	if in.QuotaBytes != nil {
		u.Set("quota_bytes", *in.QuotaBytes)
	}
	if in.Enabled != nil {
		u.Set("enabled", *in.Enabled)
	}
	if err := h.app.Save(u); err != nil {
		return apis.NewBadRequestError("failed to update user", err)
	}
	return ok(e, publicUser(u))
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

func strOr(p *string, def string) string {
	if p != nil && *p != "" {
		return *p
	}
	return def
}

func boolOr(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}
