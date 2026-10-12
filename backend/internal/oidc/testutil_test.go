package oidc

import (
	"maps"
	"net/url"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	_ "hysterical-panel/migrations"
)

func newTestApp(t *testing.T) core.App {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return app
}

func newTestUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.SetPassword("password12345")
	rec.Set("role", "user")
	rec.Set("status", "active")
	rec.Set("verified", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return rec
}

func newTestBox(t *testing.T, masterKey string) *cryptobox.Box {
	t.Helper()
	box, err := cryptobox.New(masterKey)
	if err != nil {
		t.Fatalf("cryptobox: %v", err)
	}
	return box
}

func saveTestRecord(t *testing.T, app core.App, collection string, fields map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatalf("find %s: %v", collection, err)
	}
	rec := core.NewRecord(col)
	for key, value := range fields {
		rec.Set(key, value)
	}
	if err := app.Save(rec); err != nil {
		t.Fatalf("save %s: %v", collection, err)
	}
	return rec
}

func countRecords(t *testing.T, app core.App, collection string) int {
	t.Helper()
	recs, err := app.FindAllRecords(collection)
	if err != nil {
		t.Fatalf("list %s: %v", collection, err)
	}
	return len(recs)
}

const (
	testIssuer      = "https://api.example.com/api/oidc"
	testRedirectURI = "https://app.example.com/callback"
)

// newTestClient saves an enabled Connected App and returns it with its
// plaintext secret. fields override the defaults.
func newTestClient(t *testing.T, app core.App, fields map[string]any) (*core.Record, string) {
	t.Helper()
	clientID, err := NewClientID()
	if err != nil {
		t.Fatalf("client id: %v", err)
	}
	secret, secretHash, err := NewClientSecret()
	if err != nil {
		t.Fatalf("client secret: %v", err)
	}
	values := map[string]any{
		"client_id":      clientID,
		"name":           "Test app",
		"secret_hash":    secretHash,
		"redirect_uris":  []string{testRedirectURI},
		"allowed_scopes": []string{ScopeOpenID, ScopeEmail},
		"enabled":        true,
	}
	maps.Copy(values, fields)
	return saveTestRecord(t, app, ClientsCollection, values), secret
}

// testVerifier and testChallenge are the PKCE example of RFC 7636 appendix B.
const (
	testVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	testChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

// authorizeForm is a valid authorization request for client. An override
// replaces a value, and an empty override removes the parameter.
func authorizeForm(client *core.Record, overrides map[string]string) url.Values {
	form := url.Values{
		"client_id":             {client.GetString("client_id")},
		"redirect_uri":          {testRedirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid email"},
		"state":                 {"state-1"},
		"nonce":                 {"nonce-1"},
		"code_challenge":        {testChallenge},
		"code_challenge_method": {"S256"},
	}
	for key, value := range overrides {
		if value == "" {
			form.Del(key)
		} else {
			form.Set(key, value)
		}
	}
	return form
}
