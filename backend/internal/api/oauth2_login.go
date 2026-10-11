package api

import (
	"net/http"
	"net/url"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// oauth2CallbackPath is the frontend route that completes the code exchange.
const oauth2CallbackPath = "/oauth-callback"

// bindOAuth2LoginOnly limits OAuth2 to existing users. New users go through
// the panel registration policy, which OAuth2 sign-up would bypass. PocketBase
// matches an existing user by a linked OAuth2 identity or by email.
func (h *Handlers) bindOAuth2LoginOnly() {
	h.app.OnRecordAuthWithOAuth2Request("users").BindFunc(func(e *core.RecordAuthWithOAuth2RequestEvent) error {
		if e.IsNewRecord || e.Record == nil {
			return apis.NewForbiddenError("no account matches this sign-in", nil)
		}
		return e.Next()
	})
}

// oauth2Redirect is the provider redirect_uri on the backend origin. It
// forwards the provider result to the frontend callback route. PocketBase's
// own /api/oauth2-redirect only serves the SDK realtime flow. POST handles
// providers that use response_mode=form_post, such as Apple.
func (h *Handlers) oauth2Redirect(e *core.RequestEvent) error {
	if err := e.Request.ParseForm(); err != nil {
		return apis.NewBadRequestError("invalid oauth2 redirect", err)
	}
	forwarded := url.Values{}
	for _, key := range []string{"code", "state", "error"} {
		if value := e.Request.Form.Get(key); value != "" {
			forwarded.Set(key, value)
		}
	}
	target := h.publicConfig.FrontendURL + oauth2CallbackPath
	if encoded := forwarded.Encode(); encoded != "" {
		target += "?" + encoded
	}
	return e.Redirect(http.StatusSeeOther, target)
}
