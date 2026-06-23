package api

import (
	"log"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// hysteriaAuthRequest matches Hysteria 2's HTTP auth payload:
// https://v2.hysteria.network/docs/advanced/Auth/#http
type hysteriaAuthRequest struct {
	Addr string `json:"addr"`
	Auth string `json:"auth"`
	Tx   int64  `json:"tx"`
}

// hysteriaAuth is called by Hysteria 2 servers on every client connect. We
// look the supplied auth string up against users.auth_string and return the
// match's auth_string back as the id — that's the key Hysteria will report in
// /traffic, and the collector keys off auth_string too (collector.go:106), so
// keeping them identical means zero collector changes.
//
// Failure semantics follow Hysteria's contract: any non-200 status rejects the
// client. We never log the auth value itself (it's a credential), only addr
// and the rejection reason.
func (h *Handlers) hysteriaAuth(e *core.RequestEvent) error {
	var in hysteriaAuthRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if in.Auth == "" {
		return apis.NewBadRequestError("auth required", nil)
	}

	user, err := h.app.FindFirstRecordByFilter(
		"users",
		"auth_string = {:a}",
		map[string]any{"a": in.Auth},
	)
	if err != nil || user == nil {
		log.Printf("[hysteria-auth] reject addr=%s: unknown auth", in.Addr)
		return apis.NewUnauthorizedError("invalid auth", nil)
	}
	if user.GetString("status") != "active" {
		log.Printf("[hysteria-auth] reject addr=%s: account disabled", in.Addr)
		return apis.NewForbiddenError("account is disabled", nil)
	}
	if !user.GetBool("verified") {
		log.Printf("[hysteria-auth] reject addr=%s: email not verified", in.Addr)
		return apis.NewForbiddenError("email not verified", nil)
	}

	userID := user.Id
	clientIP, hasClientIP := clientIPFromHysteriaAddr(in.Addr)
	go func() {
		rec, err := h.app.FindRecordById("users", userID)
		if err != nil {
			log.Printf("[hysteria-auth] connection metadata update: user %s not found: %v", userID, err)
			return
		}
		now := time.Now().UTC()
		rec.Set("last_connected_at", now)
		if hasClientIP && !h.isConnectionIPIgnored(clientIP) {
			if err := updateRecentConnections(rec, clientIP, now); err != nil {
				log.Printf("[hysteria-auth] recent connections update failed for user %s: %v", userID, err)
				return
			}
		}
		if err := h.app.Save(rec); err != nil {
			log.Printf("[hysteria-auth] connection metadata update failed for user %s: %v", userID, err)
		}
	}()

	return ok(e, map[string]any{
		"ok": true,
		"id": user.GetString("auth_string"),
	})
}
