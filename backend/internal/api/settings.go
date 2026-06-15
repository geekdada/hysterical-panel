package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

// settings holds the runtime-mutable feature flags stored as a singleton record
// in the app_settings collection. See registrationDecision for how the three
// flags combine to govern self-service registration.
type settings struct {
	InvitationsEnabled     bool
	OpenRegistration       bool
	RequireInviteForOpen   bool
	ManagementAPIEnabled   bool
	ManagementAPITokenHash string
}

// settingsFromRecord reads the feature flags off an app_settings record.
func settingsFromRecord(rec *core.Record) settings {
	return settings{
		InvitationsEnabled:     rec.GetBool("invitations_enabled"),
		OpenRegistration:       rec.GetBool("open_registration"),
		RequireInviteForOpen:   rec.GetBool("require_invite_for_open"),
		ManagementAPIEnabled:   rec.GetBool("management_api_enabled"),
		ManagementAPITokenHash: rec.GetString("management_api_token_hash"),
	}
}

// loadSettings reads the singleton app_settings record. A missing record yields
// the zero value (everything off), keeping registration closed by default.
func (h *Handlers) loadSettings() settings {
	recs, err := h.app.FindRecordsByFilter("app_settings", "", "", 1, 0)
	if err != nil || len(recs) == 0 {
		return settings{}
	}
	return settingsFromRecord(recs[0])
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
		InvitationsEnabled:    s.InvitationsEnabled,
		OpenRegistration:      s.OpenRegistration,
		RequireInviteForOpen:  s.RequireInviteForOpen,
		ManagementAPIEnabled:  s.ManagementAPIEnabled,
		ManagementAPITokenSet: s.ManagementAPITokenHash != "",
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
	// Management API: enabling auto-generates a token if none exists yet. The
	// plaintext token is returned exactly once in the response.
	var plaintextToken string
	if in.ManagementAPIEnabled != nil {
		if *in.ManagementAPIEnabled && rec.GetString("management_api_token_hash") == "" {
			t, err := generateManagementAPIToken()
			if err != nil {
				return apis.NewBadRequestError("failed to generate management API token", err)
			}
			plaintextToken = t
			rec.Set("management_api_token_hash", hashManagementAPIToken(t))
		}
		rec.Set("management_api_enabled", *in.ManagementAPIEnabled)
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save settings", err)
	}
	resp := settingsResponse(settingsFromRecord(rec))
	resp.ManagementAPIToken = plaintextToken // shown once, only when just generated
	return ok(e, resp)
}

// rotateManagementAPIToken generates a new management API token, replaces the
// stored hash, and returns the plaintext exactly once.
func (h *Handlers) rotateManagementAPIToken(e *core.RequestEvent) error {
	rec, err := h.settingsRecord()
	if err != nil {
		return apis.NewBadRequestError("failed to load settings", err)
	}
	t, err := generateManagementAPIToken()
	if err != nil {
		return apis.NewBadRequestError("failed to generate management API token", err)
	}
	rec.Set("management_api_token_hash", hashManagementAPIToken(t))
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save settings", err)
	}
	return ok(e, ManagementAPITokenResponse{ManagementAPIToken: t})
}

// generateManagementAPIToken produces a cryptographically random,
// URL-safe token for the Management API.
func generateManagementAPIToken() (string, error) {
	return token.New(32) // 256 bits, ~43 chars base64
}

// hashManagementAPIToken returns the lower-case hex SHA-256 digest of a token.
func hashManagementAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// verifyManagementAPIToken does a constant-time comparison between a plaintext
// token and the stored SHA-256 hash. Returns false if the hash is empty.
func verifyManagementAPIToken(token, storedHash string) bool {
	if storedHash == "" {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(storedHash)) == 1
}
