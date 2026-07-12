package api

import (
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/authstrings"
)

// Hysteria 2 HTTP auth callback (POST /api/hysteria/auth). Clients send the raw
// auth_string; see node_client_auth.go for the shared request/response contract.

func (h *Handlers) hysteriaAuth(e *core.RequestEvent) error {
	return h.handleNodeClientAuth(e, "hysteria-auth", h.lookupUserByAuthString)
}

// lookupUserByAuthString matches Hysteria clients, which send auth_string as-is.
func (h *Handlers) lookupUserByAuthString(auth string) (*core.Record, error) {
	return authstrings.FindCurrentUserByAuthString(h.app, auth)
}
