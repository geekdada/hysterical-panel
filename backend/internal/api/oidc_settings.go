package api

import (
	"errors"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

const (
	oidcUnavailableBackendURLMissing  = "backend_url_missing"
	oidcUnavailableBackendURLInsecure = "backend_url_insecure"
	oidcUnavailableFrontendURLMissing = "frontend_url_missing"
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

// applyOIDCEnabled validates and stores the provider switch. Enabling creates
// the signing key at once so JWKS is never empty while the provider is on.
// Repeating true is a no-op, so a client that resends the whole form is not
// rejected after the environment changes.
func (h *Handlers) applyOIDCEnabled(rec *core.Record, enabled bool) error {
	if enabled && !rec.GetBool("oidc_enabled") {
		if _, reason := h.oidcAvailability(); reason != "" {
			return apis.NewBadRequestError("oidc provider requires an https PANEL_BACKEND_URL_BASE and PANEL_FRONTEND_URL_BASE", nil)
		}
		if _, err := oidc.ActiveSigningKey(h.app, h.box, time.Now()); err != nil {
			return apis.NewInternalServerError("failed to prepare the oidc signing key", err)
		}
	}
	rec.Set("oidc_enabled", enabled)
	return nil
}
