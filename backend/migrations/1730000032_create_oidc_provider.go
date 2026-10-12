package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Storage for the OIDC provider (ADR-0010). Every API rule stays nil, so the
// generic record API serves these collections to superusers only. The panel
// reads and writes them through /api/panel/* and /api/oidc/*.
var oidcCollectionsChildFirst = []string{
	"oidc_session_auth_times", "oidc_signing_keys", "oidc_tokens",
	"oidc_codes", "oidc_auth_requests", "oidc_grants", "oidc_clients",
}

func init() {
	m.Register(createOIDCCollections, func(app core.App) error {
		for _, name := range oidcCollectionsChildFirst {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}

func createOIDCCollections(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	clients := oidcClientsCollection(users)
	if err := app.Save(clients); err != nil {
		return err
	}
	grants := oidcGrantsCollection(users, clients)
	if err := app.Save(grants); err != nil {
		return err
	}
	for _, c := range []*core.Collection{
		oidcAuthRequestsCollection(clients),
		oidcCodesCollection(grants),
		oidcTokensCollection(grants),
		oidcSigningKeysCollection(),
		oidcSessionAuthTimesCollection(users),
	} {
		if err := app.Save(c); err != nil {
			return err
		}
	}
	return nil
}

func addOIDCAutodates(c *core.Collection) {
	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
}

func oidcClientsCollection(users *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_clients")
	c.Fields.Add(&core.TextField{Name: "client_id", Required: true, Max: 64})
	c.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 128})
	c.Fields.Add(&core.TextField{Name: "secret_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.JSONField{Name: "redirect_uris", Required: true})
	c.Fields.Add(&core.JSONField{Name: "allowed_scopes", Required: true})
	c.Fields.Add(&core.BoolField{Name: "refresh_enabled"})
	c.Fields.Add(&core.BoolField{Name: "skip_consent"})
	c.Fields.Add(&core.BoolField{Name: "enabled"})
	c.Fields.Add(&core.RelationField{Name: "created_by", MaxSelect: 1, CollectionId: users.Id})
	c.Fields.Add(&core.DateField{Name: "last_used_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_clients_client_id", true, "client_id", "")
	return c
}

// A grant is one User's remembered consent for one Connected App. Only one
// unrevoked grant may exist per pair; scope growth updates it in place.
func oidcGrantsCollection(users, clients *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_grants")
	c.Fields.Add(&core.RelationField{Name: "user", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: users.Id})
	c.Fields.Add(&core.RelationField{Name: "client", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: clients.Id})
	c.Fields.Add(&core.JSONField{Name: "scopes", Required: true})
	c.Fields.Add(&core.DateField{Name: "revoked_at"})
	c.Fields.Add(&core.TextField{Name: "revoke_reason", Max: 64})
	c.Fields.Add(&core.DateField{Name: "last_used_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_grants_active_pair", true, "user, client", "revoked_at = ''")
	return c
}

// max_age 0 means absent: /authorize turns max_age=0 into prompt=login.
func oidcAuthRequestsCollection(clients *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_auth_requests")
	c.Fields.Add(&core.TextField{Name: "request_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.RelationField{Name: "client", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: clients.Id})
	c.Fields.Add(&core.TextField{Name: "redirect_uri", Required: true, Max: 2048})
	c.Fields.Add(&core.TextField{Name: "scope", Required: true, Max: 1024})
	c.Fields.Add(&core.TextField{Name: "state", Max: 1024})
	c.Fields.Add(&core.TextField{Name: "nonce", Max: 1024})
	c.Fields.Add(&core.TextField{Name: "code_challenge", Required: true, Max: 128})
	c.Fields.Add(&core.TextField{Name: "prompt", Max: 64})
	c.Fields.Add(&core.NumberField{Name: "max_age", OnlyInt: true, Min: types.Pointer(0.0)})
	c.Fields.Add(&core.DateField{Name: "expires_at", Required: true})
	c.Fields.Add(&core.DateField{Name: "consumed_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_auth_requests_hash", true, "request_hash", "")
	c.AddIndex("idx_oidc_auth_requests_expires_at", false, "expires_at", "")
	return c
}

// family links a code to every token issued from it, so a replayed code or
// refresh token revokes exactly that chain (RFC 6749 4.1.2, RFC 9700 4.14.2).
func oidcCodesCollection(grants *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_codes")
	c.Fields.Add(&core.TextField{Name: "code_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.RelationField{Name: "grant", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: grants.Id})
	c.Fields.Add(&core.TextField{Name: "family", Required: true, Max: 64})
	c.Fields.Add(&core.TextField{Name: "redirect_uri", Required: true, Max: 2048})
	c.Fields.Add(&core.TextField{Name: "scope", Required: true, Max: 1024})
	c.Fields.Add(&core.TextField{Name: "nonce", Max: 1024})
	c.Fields.Add(&core.TextField{Name: "code_challenge", Required: true, Max: 128})
	c.Fields.Add(&core.DateField{Name: "auth_time"})
	c.Fields.Add(&core.DateField{Name: "expires_at", Required: true})
	c.Fields.Add(&core.DateField{Name: "used_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_codes_hash", true, "code_hash", "")
	c.AddIndex("idx_oidc_codes_expires_at", false, "expires_at", "")
	return c
}

func oidcTokensCollection(grants *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_tokens")
	c.Fields.Add(&core.TextField{Name: "token_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.SelectField{Name: "kind", Required: true, MaxSelect: 1, Values: []string{"access", "refresh"}})
	c.Fields.Add(&core.RelationField{Name: "grant", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: grants.Id})
	c.Fields.Add(&core.TextField{Name: "family", Required: true, Max: 64})
	c.Fields.Add(&core.TextField{Name: "scope", Required: true, Max: 1024})
	c.Fields.Add(&core.DateField{Name: "auth_time"})
	c.Fields.Add(&core.DateField{Name: "expires_at", Required: true})
	c.Fields.Add(&core.DateField{Name: "absolute_expires_at"})
	c.Fields.Add(&core.DateField{Name: "rotated_at"})
	c.Fields.Add(&core.DateField{Name: "revoked_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_tokens_hash", true, "token_hash", "")
	c.AddIndex("idx_oidc_tokens_family", false, "family", "")
	c.AddIndex("idx_oidc_tokens_grant", false, "grant", "")
	c.AddIndex("idx_oidc_tokens_expires_at", false, "expires_at", "")
	return c
}

func oidcSigningKeysCollection() *core.Collection {
	c := core.NewBaseCollection("oidc_signing_keys")
	c.Fields.Add(&core.TextField{Name: "kid", Required: true, Max: 64})
	c.Fields.Add(&core.TextField{Name: "alg", Required: true, Max: 16})
	c.Fields.Add(&core.TextField{Name: "private_key", Required: true, Hidden: true})
	c.Fields.Add(&core.JSONField{Name: "public_jwk", Required: true})
	c.Fields.Add(&core.SelectField{Name: "state", Required: true, MaxSelect: 1, Values: []string{"active", "retired"}})
	c.Fields.Add(&core.DateField{Name: "retired_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_signing_keys_kid", true, "kid", "")
	c.AddIndex("idx_oidc_signing_keys_active", true, "state", "state = 'active'")
	return c
}

// PocketBase session tokens carry no auth time, so the panel records when
// each one came from a real sign-in. prompt=login and max_age read it.
func oidcSessionAuthTimesCollection(users *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_session_auth_times")
	c.Fields.Add(&core.TextField{Name: "token_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.RelationField{Name: "user", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: users.Id})
	// An empty auth_time marks a session of unknown age.
	c.Fields.Add(&core.DateField{Name: "auth_time"})
	c.Fields.Add(&core.DateField{Name: "expires_at", Required: true})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_session_auth_times_hash", true, "token_hash", "")
	c.AddIndex("idx_oidc_session_auth_times_expires_at", false, "expires_at", "")
	return c
}
