package api

import (
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

// bindOIDCGrantRevocation revokes a User's Connected App grants when a save
// makes the User unusable. It hooks the record save rather than the panel
// handler, so edits made in /_/ count too (ADR-0010). A failure is logged
// and never blocks the save, because every token lookup checks the User
// again anyway.
func (h *Handlers) bindOIDCGrantRevocation() {
	h.app.OnRecordUpdate("users").BindFunc(func(e *core.RecordEvent) error {
		reason := oidcRevokeReason(e.Record.Original(), e.Record)
		if err := e.Next(); err != nil || reason == "" {
			return err
		}
		if err := oidc.RevokeUserGrants(e.App, e.Record.Id, reason, time.Now()); err != nil {
			log.Printf("[oidc] revoke grants of user %s: %v", e.Record.Id, err)
		}
		return nil
	})
}

// oidcRevokeReason names why a save ends a User's grants, or returns "" when
// the User was already unusable or still is usable.
func oidcRevokeReason(before, after *core.Record) string {
	if before.GetString("status") != "active" || !before.GetBool("verified") {
		return ""
	}
	switch {
	case after.GetString("status") != "active":
		return oidc.RevokeReasonUserDisabled
	case !after.GetBool("verified"):
		return oidc.RevokeReasonUserUnverified
	}
	return ""
}
