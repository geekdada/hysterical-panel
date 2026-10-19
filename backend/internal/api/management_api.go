package api

import (
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/authstrings"
	"hysterical-panel/internal/token"
)

// managementRoute is one /api/mgmt route. The same table registers the
// routes and lets tests check that openapi_management.go documents each one.
type managementRoute struct {
	method  string
	path    string
	handler func(*core.RequestEvent) error
}

func (h *Handlers) managementRoutes() []managementRoute {
	return []managementRoute{
		{http.MethodGet, "/users", h.mgmtGetUser},
		{http.MethodPost, "/users", h.mgmtCreateUser},
		{http.MethodGet, "/nodes", h.listNodes},
		{http.MethodPost, "/nodes", h.createNode},
		{http.MethodGet, "/nodes/{id}", h.getNode},
		{http.MethodPatch, "/nodes/{id}", h.mgmtUpdateNode},
		{http.MethodDelete, "/nodes/{id}", h.deleteNode},
		{http.MethodPost, "/nodes/{id}/test", h.testNode},
		{http.MethodGet, "/openapi.json", h.managementOpenAPISpec},
	}
}

// bindManagementRoutes registers every management route behind the token gate.
func (h *Handlers) bindManagementRoutes(group *router.RouterGroup[*core.RequestEvent]) {
	group.Bind(h.requireMgmtToken())
	for _, r := range h.managementRoutes() {
		group.Route(r.method, r.path, r.handler)
	}
}

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

// mgmtGetUser looks up a single user by User ID, exact email or Current Auth
// String. Exactly one of them must be provided. Connected Apps receive the
// User ID as sub, so apps signing Users in through OIDC look them up by id.
// It returns the panel's PanelUser shape (including only the Current Auth
// String).
func (h *Handlers) mgmtGetUser(e *core.RequestEvent) error {
	query := e.Request.URL.Query()
	id := strings.TrimSpace(query.Get("id"))
	email := strings.TrimSpace(query.Get("email"))
	authString := strings.TrimSpace(query.Get("auth_string"))
	if countNonEmpty(id, email, authString) > 1 {
		return apis.NewBadRequestError("provide only one of id, email or auth_string", nil)
	}

	var user *core.Record
	var err error
	switch {
	case id != "":
		user, err = h.app.FindRecordById("users", id)
	case email != "":
		user, err = h.app.FindFirstRecordByFilter("users", "email = {:v}", map[string]any{"v": email})
	case authString != "":
		user, err = authstrings.FindCurrentUserByAuthString(h.app, authString)
	default:
		return apis.NewBadRequestError("id, email or auth_string is required", nil)
	}
	if err != nil || user == nil {
		return apis.NewNotFoundError("user not found", nil)
	}
	currentAuthString, err := authstrings.CurrentValue(h.app, user.Id)
	if err != nil {
		return apis.NewNotFoundError("user not found", nil)
	}
	return e.JSON(http.StatusOK, panelUser(user, currentAuthString, h.ipLookup, h.loadIgnoredConnectionIPSet()))
}

func countNonEmpty(values ...string) int {
	count := 0
	for _, value := range values {
		if value != "" {
			count++
		}
	}
	return count
}

// mgmtCreateUser provisions a user from an email only. Password and
// auth_string are system-generated and never returned.
func (h *Handlers) mgmtCreateUser(e *core.RequestEvent) error {
	var in ManagementUserCreateRequest
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
		Email:    email,
		Password: password,
		Role:     "user",
		Status:   "active",
		Verified: true,
	})
	if err != nil {
		return err
	}
	if err := h.saveNewUserWithAuthString(u, authString); err != nil {
		return apis.NewBadRequestError("failed to create user (email may be taken)", err)
	}
	return e.JSON(http.StatusCreated, ManagementUserCreateResponse{
		ID:     u.Id,
		Email:  u.GetString("email"),
		Status: u.GetString("status"),
	})
}
