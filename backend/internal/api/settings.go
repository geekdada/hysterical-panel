package api

import (
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// settings holds the runtime-mutable feature flags stored as a singleton record
// in the app_settings collection. See registrationDecision for how the three
// flags combine to govern self-service registration.
type settings struct {
	InvitationsEnabled   bool
	OpenRegistration     bool
	RequireInviteForOpen bool
}

// loadSettings reads the singleton app_settings record. A missing record yields
// the zero value (everything off), keeping registration closed by default.
func (h *Handlers) loadSettings() settings {
	recs, err := h.app.FindRecordsByFilter("app_settings", "", "", 1, 0)
	if err != nil || len(recs) == 0 {
		return settings{}
	}
	rec := recs[0]
	return settings{
		InvitationsEnabled:   rec.GetBool("invitations_enabled"),
		OpenRegistration:     rec.GetBool("open_registration"),
		RequireInviteForOpen: rec.GetBool("require_invite_for_open"),
	}
}

// settingsRecord returns the singleton app_settings record, lazily creating it
// if it is somehow missing (the migration seeds one on install).
func (h *Handlers) settingsRecord() (*core.Record, error) {
	recs, err := h.app.FindRecordsByFilter("app_settings", "", "", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(recs) > 0 {
		return recs[0], nil
	}
	coll, err := h.app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(coll)
	rec.Set("invitations_enabled", false)
	rec.Set("open_registration", false)
	rec.Set("require_invite_for_open", false)
	if err := h.app.Save(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func settingsResponse(s settings) SettingsResponse {
	return SettingsResponse{
		InvitationsEnabled:   s.InvitationsEnabled,
		OpenRegistration:     s.OpenRegistration,
		RequireInviteForOpen: s.RequireInviteForOpen,
	}
}

func (h *Handlers) getSettings(e *core.RequestEvent) error {
	return ok(e, settingsResponse(h.loadSettings()))
}

func (h *Handlers) updateSettings(e *core.RequestEvent) error {
	var in SettingsUpdateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	rec, err := h.settingsRecord()
	if err != nil {
		return apis.NewBadRequestError("failed to load settings", err)
	}
	if in.InvitationsEnabled != nil {
		rec.Set("invitations_enabled", *in.InvitationsEnabled)
	}
	if in.OpenRegistration != nil {
		rec.Set("open_registration", *in.OpenRegistration)
	}
	if in.RequireInviteForOpen != nil {
		rec.Set("require_invite_for_open", *in.RequireInviteForOpen)
	}
	// Requiring an invite code on open registration only makes sense when the
	// invitation system is enabled — otherwise no codes are valid and the open
	// path becomes an inescapable dead end.
	if rec.GetBool("require_invite_for_open") && !rec.GetBool("invitations_enabled") {
		return apis.NewBadRequestError("require_invite_for_open requires invitations_enabled", nil)
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save settings", err)
	}
	return ok(e, settingsResponse(h.loadSettings()))
}
