package api

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/authstrings"
)

// anytls HTTP auth callback (POST /api/anytls/auth). Clients send
// hex(sha256(password)) where password is auth_string; see node_client_auth.go
// for the shared request/response contract.

func (h *Handlers) anytlsAuth(e *core.RequestEvent) error {
	return h.handleNodeClientAuth(e, "anytls-auth", h.lookupUserByAuthStringAnytlsHash)
}

// lookupUserByAuthStringAnytlsHash matches anytls clients, which send
// hex(sha256(password)) where password is auth_string. Hex casing is normalized
// before lookup.
func (h *Handlers) lookupUserByAuthStringAnytlsHash(hash string) (*core.Record, error) {
	return authstrings.FindCurrentUserByAnytlsHash(h.app, strings.ToLower(hash))
}
