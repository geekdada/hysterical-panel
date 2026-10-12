package api

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

// bindOIDCSessionAuthTimes records when each panel session token came from a
// real sign-in, so the OIDC provider can honour prompt=login and max_age. A
// failed write only means the user re-authenticates later, so sign-in proceeds.
func (h *Handlers) bindOIDCSessionAuthTimes() {
	h.app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		previous := bearerToken(e.Request)
		if err := oidc.RecordSessionAuth(e.App, e.Record, e.Token, e.AuthMethod, previous, time.Now()); err != nil {
			log.Printf("[oidc] record session auth time for user %s: %v", e.Record.Id, err)
		}
		return nil
	})
}

// issueSessionToken mints a panel session token outside PocketBase's auth
// routes, where the auth hook never runs, and records it as a fresh sign-in.
func issueSessionToken(app core.App, user *core.Record) (string, error) {
	sessionToken, err := user.NewAuthToken()
	if err != nil {
		return "", err
	}
	if err := oidc.RecordSessionAuth(app, user, sessionToken, "register", "", time.Now()); err != nil {
		log.Printf("[oidc] record session auth time for user %s: %v", user.Id, err)
	}
	return sessionToken, nil
}

// bearerToken reads the Authorization header the way PocketBase does: the
// Bearer prefix is optional and case-insensitive.
func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if len(value) > 7 && strings.EqualFold(value[:7], "Bearer ") {
		return value[7:]
	}
	return value
}
