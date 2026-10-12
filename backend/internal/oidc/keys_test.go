package oidc

import (
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

var keyTime = time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)

func TestActiveSigningKeyIsCreatedOnceAndReused(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, "master-a")
	first, err := ActiveSigningKey(app, box, keyTime)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := ActiveSigningKey(app, box, keyTime)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID != second.ID || first.Private.N.Cmp(second.Private.N) != 0 {
		t.Fatal("second call did not return the stored key")
	}
	if bits := first.Private.N.BitLen(); bits != 2048 {
		t.Fatalf("key size = %d bits, want 2048", bits)
	}
}

func TestRotateSigningKeyKeepsRetiredKeyInJWKSForADay(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, "master-a")
	old, _ := ActiveSigningKey(app, box, keyTime)
	newID, err := RotateSigningKey(app, box, false, keyTime)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	active, _ := ActiveSigningKey(app, box, keyTime)
	if active.ID != newID || newID == old.ID {
		t.Fatalf("active = %s, rotated = %s, old = %s", active.ID, newID, old.ID)
	}
	jwks, _ := PublicJWKS(app, keyTime.Add(time.Hour))
	if ids := jwkIDs(jwks); ids != newID+","+old.ID {
		t.Fatalf("jwks kids = %s, want active first then retired", ids)
	}
	jwks, _ = PublicJWKS(app, keyTime.Add(RetiredKeyRetention+time.Minute))
	if ids := jwkIDs(jwks); ids != newID {
		t.Fatalf("jwks kids after retention = %s, want %s", ids, newID)
	}
}

func TestRotateSigningKeyDropPreviousRemovesOlderKeys(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, "master-a")
	_, _ = ActiveSigningKey(app, box, keyTime)
	newID, err := RotateSigningKey(app, box, true, keyTime)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	jwks, _ := PublicJWKS(app, keyTime)
	if ids := jwkIDs(jwks); ids != newID {
		t.Fatalf("jwks kids = %s, want only %s", ids, newID)
	}
	if n := countRecords(t, app, SigningKeysCollection); n != 1 {
		t.Fatalf("stored keys = %d, want 1", n)
	}
}

func TestActiveSigningKeyReplacesKeyThatNoLongerDecrypts(t *testing.T) {
	app := newTestApp(t)
	old, err := ActiveSigningKey(app, newTestBox(t, "master-a"), keyTime)
	if err != nil {
		t.Fatalf("create with old master key: %v", err)
	}
	replaced, err := ActiveSigningKey(app, newTestBox(t, "master-b"), keyTime)
	if err != nil {
		t.Fatalf("load with new master key: %v", err)
	}
	if replaced.ID == old.ID {
		t.Fatal("undecryptable key was reused")
	}
	jwks, _ := PublicJWKS(app, keyTime)
	if ids := jwkIDs(jwks); ids != replaced.ID+","+old.ID {
		t.Fatalf("jwks kids = %s, want replacement then retired key", ids)
	}
}

func TestPublicJWKSExposesOnlyPublicParts(t *testing.T) {
	app := newTestApp(t)
	key, _ := ActiveSigningKey(app, newTestBox(t, "master-a"), keyTime)
	jwks, _ := PublicJWKS(app, keyTime)
	raw, _ := json.Marshal(jwks)
	for _, private := range []string{`"d"`, `"p"`, `"q"`, `"dp"`, `"dq"`, `"qi"`} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("jwks contains private member %s: %s", private, raw)
		}
	}
	jwk := jwks.Keys[0]
	if jwk.KeyType != "RSA" || jwk.Use != "sig" || jwk.Algorithm != SigningAlgorithm || jwk.KeyID != key.ID {
		t.Fatalf("unexpected jwk header: %+v", jwk)
	}
	n, err := base64.RawURLEncoding.DecodeString(jwk.Modulus)
	if err != nil || new(big.Int).SetBytes(n).Cmp(key.Private.N) != 0 {
		t.Fatalf("modulus does not match the signing key: %v", err)
	}
	if jwk.Exponent != "AQAB" {
		t.Fatalf("exponent = %s, want AQAB", jwk.Exponent)
	}
}

func jwkIDs(jwks JWKS) string {
	ids := make([]string, 0, len(jwks.Keys))
	for _, k := range jwks.Keys {
		ids = append(ids, k.KeyID)
	}
	return strings.Join(ids, ",")
}

func TestCreateFirstKeyReturnsKeyCreatedConcurrently(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, "master-a")
	winner, _ := ActiveSigningKey(app, box, keyTime)
	got, err := createFirstKey(app, box)
	if err != nil || got.ID != winner.ID {
		t.Fatalf("createFirstKey = %v err=%v, want the existing key %s", got, err, winner.ID)
	}
	if n := countRecords(t, app, SigningKeysCollection); n != 1 {
		t.Fatalf("stored keys = %d, want 1", n)
	}
}

// Requests that all saw the same undecryptable key must rotate it only once.
func TestReplaceUndecryptableKeySkipsWhenAnotherRequestReplacedItFirst(t *testing.T) {
	app := newTestApp(t)
	stale, _ := ActiveSigningKey(app, newTestBox(t, "master-a"), keyTime)
	box := newTestBox(t, "master-b")
	if _, err := replaceUndecryptableKey(app, box, stale.ID, keyTime); err != nil {
		t.Fatalf("first replacement: %v", err)
	}
	replaced, err := replaceUndecryptableKey(app, box, stale.ID, keyTime)
	if err != nil {
		t.Fatalf("second replacement: %v", err)
	}
	if n := countRecords(t, app, SigningKeysCollection); n != 2 {
		t.Fatalf("stored keys = %d, want the stale key and one replacement", n)
	}
	active, _ := ActiveSigningKey(app, box, keyTime)
	if replaced.ID != active.ID {
		t.Fatalf("second caller got %s, want the active key %s", replaced.ID, active.ID)
	}
}
