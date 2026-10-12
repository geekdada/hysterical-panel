package api

import (
	"errors"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

const (
	oidcUnavailableBackendURLMissing  = "backend_url_missing"
	oidcUnavailableBackendURLInsecure = "backend_url_insecure"
	oidcUnavailableFrontendURLMissing = "frontend_url_missing"
	oidcUnavailableIssuerChanged      = "issuer_changed"
)

// oidcAvailability reports the issuer and why the provider cannot be enabled.
// The consent page is served by the frontend, so it needs a frontend origin.
func (h *Handlers) oidcAvailability() (issuer, reason string) {
	issuer, err := oidc.Issuer(h.publicConfig.APIURL)
	switch {
	case errors.Is(err, oidc.ErrBackendURLMissing):
		return "", oidcUnavailableBackendURLMissing
	case err != nil:
		return "", oidcUnavailableBackendURLInsecure
	case h.publicConfig.FrontendURL == "":
		return issuer, oidcUnavailableFrontendURLMissing
	}
	return issuer, ""
}

// oidcStatus adds the published issuer to oidcAvailability. Relying parties
// pin the issuer, so a provider whose issuer moved must stop answering.
func (h *Handlers) oidcStatus(s settings) (issuer, reason string) {
	issuer, reason = h.oidcAvailability()
	if reason == "" && s.OIDCEnabled && s.OIDCIssuer != "" && s.OIDCIssuer != issuer {
		return issuer, oidcUnavailableIssuerChanged
	}
	return issuer, reason
}

// oidcServingIssuer reports whether the provider endpoints exist right now.
// A switch set outside the panel has no pinned issuer yet, so the first
// served request pins the current one.
func (h *Handlers) oidcServingIssuer() (string, bool) {
	rec, err := h.settingsRecord()
	if err != nil || !rec.GetBool("oidc_enabled") {
		return "", false
	}
	issuer, reason := h.oidcStatus(settingsFromRecord(rec))
	if reason != "" {
		return "", false
	}
	if rec.GetString("oidc_issuer") == "" {
		rec.Set("oidc_issuer", issuer)
		if err := h.app.Save(rec); err != nil {
			log.Printf("[oidc] pin issuer: %v", err)
		}
	}
	return issuer, true
}

// applyOIDCEnabled validates and stores the provider switch. Enabling creates
// the signing key at once so JWKS is never empty while the provider is on,
// and pins the issuer it publishes. Repeating true is a no-op, so a client
// that resends the whole form is not rejected after the environment changes.
func (h *Handlers) applyOIDCEnabled(rec *core.Record, enabled bool) error {
	if !enabled {
		rec.Set("oidc_enabled", false)
		rec.Set("oidc_issuer", "")
		return nil
	}
	if rec.GetBool("oidc_enabled") {
		return nil
	}
	issuer, reason := h.oidcAvailability()
	if reason != "" {
		return apis.NewBadRequestError("oidc provider requires an https PANEL_BACKEND_URL_BASE and PANEL_FRONTEND_URL_BASE", nil)
	}
	if _, err := oidc.ActiveSigningKey(h.app, h.box, time.Now()); err != nil {
		return apis.NewInternalServerError("failed to prepare the oidc signing key", err)
	}
	rec.Set("oidc_enabled", true)
	rec.Set("oidc_issuer", issuer)
	return nil
}
