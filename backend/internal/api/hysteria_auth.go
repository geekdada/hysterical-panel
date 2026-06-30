package api

import (
	"log"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HTTP auth callbacks for node clients. Hysteria 2 and anytls share the same
// request/response contract (see nodeClientAuthRequest and handleNodeClientAuth);
// they differ only in how the client sends credentials and how we look up users.
//
//   - Hysteria: clients send the raw auth_string; we match users.auth_string.
//   - anytls: clients send hex(sha256(password)); we match users.auth_string_hash
//     (password is the same auth_string; the hash is synced on every user save).
//
// Both endpoints return the matched user's auth_string as "id" so /traffic keys
// stay consistent with the collector regardless of protocol.

// nodeClientAuthRequest matches Hysteria 2's HTTP auth payload:
// https://v2.hysteria.network/docs/advanced/Auth/#http
// anytls sends the same shape plus an extra "variant" field, which BindBody ignores.
type nodeClientAuthRequest struct {
	Addr string `json:"addr"`
	Auth string `json:"auth"`
	Tx   int64  `json:"tx"`
}

func (h *Handlers) hysteriaAuth(e *core.RequestEvent) error {
	return h.handleNodeClientAuth(e, "hysteria-auth", h.lookupUserByAuthString)
}

func (h *Handlers) anytlsAuth(e *core.RequestEvent) error {
	return h.handleNodeClientAuth(e, "anytls-auth", h.lookupUserByAuthStringHash)
}

// lookupUserByAuthString matches Hysteria clients, which send auth_string as-is.
func (h *Handlers) lookupUserByAuthString(auth string) (*core.Record, error) {
	return h.app.FindFirstRecordByFilter(
		"users",
		"auth_string = {:a}",
		map[string]any{"a": auth},
	)
}

// lookupUserByAuthStringHash matches anytls clients, which send hex(sha256(password))
// where password is auth_string. Hex casing is normalized before lookup.
func (h *Handlers) lookupUserByAuthStringHash(hash string) (*core.Record, error) {
	return h.app.FindFirstRecordByFilter(
		"users",
		"auth_string_hash = {:a}",
		map[string]any{"a": strings.ToLower(hash)},
	)
}

// handleNodeClientAuth implements the shared HTTP-auth contract. lookup resolves
// the user from the request's auth value; the response always uses auth_string as
// id so node /traffic and the collector stay aligned across protocols.
//
// Failure semantics follow the node contract: any non-200 status rejects the
// client. We never log the auth value itself (it's a credential), only addr
// and the rejection reason.
func (h *Handlers) handleNodeClientAuth(
	e *core.RequestEvent,
	logPrefix string,
	lookup func(string) (*core.Record, error),
) error {
	var in nodeClientAuthRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if in.Auth == "" {
		return apis.NewBadRequestError("auth required", nil)
	}

	user, err := lookup(in.Auth)
	if err != nil || user == nil {
		log.Printf("[%s] reject addr=%s: unknown auth", logPrefix, in.Addr)
		return apis.NewUnauthorizedError("invalid auth", nil)
	}
	if user.GetString("status") != "active" {
		log.Printf("[%s] reject addr=%s: account disabled", logPrefix, in.Addr)
		return apis.NewForbiddenError("account is disabled", nil)
	}
	if !user.GetBool("verified") {
		log.Printf("[%s] reject addr=%s: email not verified", logPrefix, in.Addr)
		return apis.NewForbiddenError("email not verified", nil)
	}

	userID := user.Id
	clientIP, hasClientIP := clientIPFromHysteriaAddr(in.Addr)
	go func() {
		rec, err := h.app.FindRecordById("users", userID)
		if err != nil {
			log.Printf("[%s] connection metadata update: user %s not found: %v", logPrefix, userID, err)
			return
		}
		now := time.Now().UTC()
		rec.Set("last_connected_at", now)
		if hasClientIP && !h.isConnectionIPIgnored(clientIP) {
			if err := updateRecentConnections(rec, clientIP, now); err != nil {
				log.Printf("[%s] recent connections update failed for user %s: %v", logPrefix, userID, err)
				return
			}
		}
		if err := h.app.Save(rec); err != nil {
			log.Printf("[%s] connection metadata update failed for user %s: %v", logPrefix, userID, err)
		}
	}()

	return ok(e, map[string]any{
		"ok": true,
		"id": user.GetString("auth_string"),
	})
}