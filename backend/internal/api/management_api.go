package api

import (
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"hysterical-panel/internal/token"
)

// requireMgmtToken gates the /api/mgmt/* surface. The endpoints are public
// (no panel login), so a bearer token is the sole gatekeeper. When the feature
// is disabled the surface responds 404 to avoid leaking its existence.
func (h *Handlers) requireMgmtToken() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Func: func(e *core.RequestEvent) error {
			s := h.loadSettings()
			if !s.ManagementAPIEnabled {
				return apis.NewNotFoundError("", nil)
			}
			authHeader := e.Request.Header.Get("Authorization")
			const prefix = "Bearer "
			if len(authHeader) <= len(prefix) || !strings.EqualFold(authHeader[:len(prefix)], prefix) {
				return apis.NewUnauthorizedError("missing or invalid Authorization header", nil)
			}
			provided := strings.TrimSpace(authHeader[len(prefix):])
			if provided == "" || !verifyManagementAPIToken(provided, s.ManagementAPITokenHash) {
				return apis.NewUnauthorizedError("invalid token", nil)
			}
			return e.Next()
		},
	}
}

// mgmtCreateUserRequest is the body for POST /api/mgmt/users.
type mgmtCreateUserRequest struct {
	Email string `json:"email"`
}

// mgmtCreateUserResponse confirms a user was created without echoing any
// credential. The integrator relies on the reset-password flow for access.
type mgmtCreateUserResponse struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Status string `json:"status"`
}

// mgmtGetUser looks up a single user by exact email or auth_string match,
// exactly one of which must be provided. It returns the panel's PanelUser
// shape (including auth_string, the upstream proxy credential key).
func (h *Handlers) mgmtGetUser(e *core.RequestEvent) error {
	email := strings.TrimSpace(e.Request.URL.Query().Get("email"))
	authString := strings.TrimSpace(e.Request.URL.Query().Get("auth_string"))

	var field, value string
	switch {
	case email != "" && authString != "":
		return apis.NewBadRequestError("provide either email or auth_string, not both", nil)
	case email != "":
		field, value = "email", email
	case authString != "":
		field, value = "auth_string", authString
	default:
		return apis.NewBadRequestError("email or auth_string is required", nil)
	}

	user, err := h.app.FindFirstRecordByFilter(
		"users",
		field+" = {:v}",
		map[string]any{"v": value},
	)
	if err != nil || user == nil {
		return apis.NewNotFoundError("user not found", nil)
	}
	return e.JSON(http.StatusOK, panelUser(user, h.ipLookup))
}

// mgmtCreateUser provisions a user from an email only. Password and
// auth_string are system-generated and never returned.
func (h *Handlers) mgmtCreateUser(e *core.RequestEvent) error {
	var in mgmtCreateUserRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	email := strings.TrimSpace(in.Email)
	if email == "" {
		return apis.NewBadRequestError("email is required", nil)
	}

	password, err := token.Alphanumeric(24)
	if err != nil {
		return apis.NewBadRequestError("failed to provision account", err)
	}
	authString, err := h.generateUniqueAuthString()
	if err != nil {
		return apis.NewBadRequestError("failed to provision account", err)
	}

	u, err := h.newUserRecord(newUserParams{
		Email:      email,
		Password:   password,
		AuthString: authString,
		Role:       "user",
		Status:     "active",
		Verified:   true,
	})
	if err != nil {
		return err
	}
	if err := h.app.Save(u); err != nil {
		return apis.NewBadRequestError("failed to create user (email may be taken)", err)
	}
	return e.JSON(http.StatusCreated, mgmtCreateUserResponse{
		ID:     u.Id,
		Email:  u.GetString("email"),
		Status: u.GetString("status"),
	})
}
