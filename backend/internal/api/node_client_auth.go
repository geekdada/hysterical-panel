package api

import (
	"log"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"hysterical-panel/internal/subscriptions"
)

// HTTP auth callbacks for node clients. Hysteria 2 and anytls share the same
// request/response contract (see nodeClientAuthRequest and handleNodeClientAuth);
// they differ only in how the client sends credentials and how we look up users.
//
//   - Hysteria: clients send the raw Auth String; we match the Current credential.
//   - AnyTLS: clients send hex(sha256(password)); we match the Current credential's
//     derived hash (password is the same Auth String).
//
// Both endpoints return the matched User ID as the stable Node Client ID. The
// credential is therefore never retained by the Node as its accounting key.

// nodeClientAuthRequest matches Hysteria 2's HTTP auth payload:
// https://v2.hysteria.network/docs/advanced/Auth/#http
// anytls sends the same shape plus an extra "variant" field, which BindBody ignores.
type nodeClientAuthRequest struct {
	Addr string `json:"addr"`
	Auth string `json:"auth"`
	Tx   int64  `json:"tx"`
}

// handleNodeClientAuth implements the shared HTTP-auth contract. lookup resolves
// the user from the request's auth value; the response always uses user.id as
// the stable identifier consumed by the Node's stats and kick APIs.
//
// Every rejection answers {"ok":false} as the Hysteria contract requires, while
// keeping a descriptive status code for logs. We never log the auth value itself
// (it's a credential), only addr and the rejection reason.
func (h *Handlers) handleNodeClientAuth(
	e *core.RequestEvent,
	logPrefix string,
	lookup func(string) (*core.Record, error),
) error {
	var in nodeClientAuthRequest
	if err := e.BindBody(&in); err != nil {
		return rejectNodeClient(e, http.StatusBadRequest, logPrefix, in.Addr, "invalid body")
	}
	if in.Auth == "" {
		return rejectNodeClient(e, http.StatusBadRequest, logPrefix, in.Addr, "auth required")
	}

	user, err := lookup(in.Auth)
	if err != nil || user == nil {
		return rejectNodeClient(e, http.StatusUnauthorized, logPrefix, in.Addr, "unknown auth")
	}
	if user.GetString("status") != "active" {
		return rejectNodeClient(e, http.StatusForbidden, logPrefix, in.Addr, "account disabled")
	}
	if !user.GetBool("verified") {
		return rejectNodeClient(e, http.StatusForbidden, logPrefix, in.Addr, "email not verified")
	}
	allowed, err := subscriptions.Allowed(h.app, user, time.Now().UTC())
	if err != nil {
		log.Printf("[%s] subscription check failed for user %s: %v", logPrefix, user.Id, err)
		return rejectNodeClient(e, http.StatusInternalServerError, logPrefix, in.Addr, "subscription check failed")
	}
	if !allowed {
		return rejectNodeClient(e, http.StatusForbidden, logPrefix, in.Addr, "no available subscription")
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
		"id": user.Id,
	})
}

// rejectNodeClient logs the reason and answers with the {"ok":false} body that
// Hysteria and anytls expect for a refused client.
func rejectNodeClient(e *core.RequestEvent, status int, logPrefix, addr, reason string) error {
	log.Printf("[%s] reject addr=%s: %s", logPrefix, addr, reason)
	return e.JSON(status, map[string]any{"ok": false})
}
