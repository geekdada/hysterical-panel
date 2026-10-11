package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func TestOAuth2LoginRejectsNewUsers(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	h.bindOAuth2LoginOnly()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}

	submitted := false
	newEvent := &core.RecordAuthWithOAuth2RequestEvent{
		RequestEvent: &core.RequestEvent{App: app},
		IsNewRecord:  true,
	}
	newEvent.Collection = users
	err = app.OnRecordAuthWithOAuth2Request().Trigger(newEvent, func(*core.RecordAuthWithOAuth2RequestEvent) error {
		submitted = true
		return nil
	})
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden {
		t.Fatalf("new OAuth2 user err = %v, want 403", err)
	}
	if submitted {
		t.Fatal("OAuth2 sign-up reached the record submit step")
	}

	existing := newUsersTestRecord(t, app, "oauth@example.com", "OAuthSecret1234")
	existingEvent := &core.RecordAuthWithOAuth2RequestEvent{
		RequestEvent: &core.RequestEvent{App: app},
		Record:       existing,
	}
	existingEvent.Collection = users
	err = app.OnRecordAuthWithOAuth2Request().Trigger(existingEvent, func(*core.RecordAuthWithOAuth2RequestEvent) error {
		submitted = true
		return nil
	})
	if err != nil || !submitted {
		t.Fatalf("existing OAuth2 user err = %v submitted = %v", err, submitted)
	}
}

func TestOAuth2LinkRejectsIdentityOwnedByAnotherUser(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	h.bindOAuth2LoginOnly()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	signedIn := newUsersTestRecord(t, app, "linker@example.com", "LinkerSecret1234")
	owner := newUsersTestRecord(t, app, "owner@example.com", "OwnerSecret1234")

	trigger := func(record *core.Record) (bool, error) {
		submitted := false
		event := &core.RecordAuthWithOAuth2RequestEvent{
			RequestEvent: &core.RequestEvent{App: app, Auth: signedIn},
			Record:       record,
		}
		event.Collection = users
		err := app.OnRecordAuthWithOAuth2Request().Trigger(event, func(*core.RecordAuthWithOAuth2RequestEvent) error {
			submitted = true
			return nil
		})
		return submitted, err
	}

	submitted, err := trigger(owner)
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
		t.Fatalf("link to another user's identity err = %v, want 409", err)
	}
	if submitted {
		t.Fatal("link to another user's identity reached the record submit step")
	}

	submitted, err = trigger(signedIn)
	if err != nil || !submitted {
		t.Fatalf("link to signed-in user err = %v submitted = %v", err, submitted)
	}
}

func TestOAuth2RedirectForwardsProviderResultToFrontend(t *testing.T) {
	cases := []struct {
		name        string
		frontendURL string
		method      string
		target      string
		form        url.Values
		want        string
	}{
		{
			name:        "query redirect",
			frontendURL: "https://panel.example.com",
			method:      http.MethodGet,
			target:      "/api/panel/oauth2-redirect?code=abc&state=xyz&scope=email",
			want:        "https://panel.example.com/oauth-callback?code=abc&state=xyz",
		},
		{
			name:        "form_post redirect",
			frontendURL: "https://panel.example.com",
			method:      http.MethodPost,
			target:      "/api/panel/oauth2-redirect",
			form:        url.Values{"code": {"abc"}, "state": {"xyz"}, "user": {`{"name":{}}`}},
			want:        "https://panel.example.com/oauth-callback?code=abc&state=xyz",
		},
		{
			name:   "provider error without frontend origin",
			method: http.MethodGet,
			target: "/api/panel/oauth2-redirect?error=access_denied&state=xyz",
			want:   "/oauth-callback?error=access_denied&state=xyz",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handlers{publicConfig: PanelConfigResponse{FrontendURL: tc.frontendURL}}
			request := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.form.Encode()))
			if tc.form != nil {
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			response := httptest.NewRecorder()
			event := &core.RequestEvent{Event: router.Event{Request: request, Response: response}}
			if err := h.oauth2Redirect(event); err != nil {
				t.Fatalf("oauth2Redirect: %v", err)
			}
			if response.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", response.Code)
			}
			if got := response.Header().Get("Location"); got != tc.want {
				t.Fatalf("Location = %q, want %q", got, tc.want)
			}
		})
	}
}
