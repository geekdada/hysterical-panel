package api

import (
	"github.com/pocketbase/pocketbase/core"
)

func (h *Handlers) handlePanelConfig(e *core.RequestEvent) error {
	// Static fields (URLs, version, passkeys) come from startup config; the
	// registration flags are runtime-mutable so they're read live each request.
	cfg := h.publicConfig
	s := h.loadSettings()
	cfg.RegistrationOpen = s.OpenRegistration
	cfg.RegistrationRequireInvite = s.RequireInviteForOpen
	cfg.InvitationsEnabled = s.InvitationsEnabled
	return ok(e, cfg)
}
