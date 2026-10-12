package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/token"
)

// SigningAlgorithm is the only ID token algorithm. OIDC Core 15.1 makes
// RS256 mandatory for every OP.
const SigningAlgorithm = "RS256"

const (
	signingKeyBits  = 2048
	keyStateActive  = "active"
	keyStateRetired = "retired"
)

// SigningKey is the active ID token signing key.
type SigningKey struct {
	ID      string
	Private *rsa.PrivateKey
}

// JWK is the public half of a signing key as published in JWKS (RFC 7517).
type JWK struct {
	KeyType   string `json:"kty"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

// JWKS is the document served at jwks_uri.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// ActiveSigningKey returns the active key and creates one on first use. A key
// that no longer decrypts, because PANEL_MASTER_KEY changed, can never sign
// again, so it is retired and replaced.
func ActiveSigningKey(app core.App, box *cryptobox.Box, now time.Time) (*SigningKey, error) {
	rec, err := app.FindFirstRecordByFilter(SigningKeysCollection, "state = {:state}", dbx.Params{"state": keyStateActive})
	if errors.Is(err, sql.ErrNoRows) {
		return createFirstKey(app, box)
	}
	if err != nil {
		return nil, err
	}
	key, err := decodeSigningKey(rec, box)
	if err == nil {
		return key, nil
	}
	log.Printf("[oidc] signing key %s no longer decrypts, rotating: %v", rec.GetString("kid"), err)
	return replaceUndecryptableKey(app, box, rec.GetString("kid"), now)
}

// RotateSigningKey makes a new key active and retires the current one. With
// dropPrevious every older key leaves JWKS at once, which also invalidates the
// ID tokens they signed. Use that after a key compromise.
func RotateSigningKey(app core.App, box *cryptobox.Box, dropPrevious bool, now time.Time) (string, error) {
	key, _, err := rotateSigningKey(app, box, dropPrevious, "", now)
	if err != nil {
		return "", err
	}
	return key.ID, nil
}

// replaceUndecryptableKey rotates staleKID out unless a concurrent request
// already did, then returns the active key once instead of retrying.
func replaceUndecryptableKey(app core.App, box *cryptobox.Box, staleKID string, now time.Time) (*SigningKey, error) {
	key, rotated, err := rotateSigningKey(app, box, false, staleKID, now)
	if err != nil || rotated {
		return key, err
	}
	active, err := app.FindFirstRecordByFilter(SigningKeysCollection, "state = 'active'")
	if err != nil {
		return nil, err
	}
	return decodeSigningKey(active, box)
}

// rotateSigningKey skips the rotation when onlyIfActiveKID is set and is no
// longer the active key.
func rotateSigningKey(app core.App, box *cryptobox.Box, dropPrevious bool, onlyIfActiveKID string, now time.Time) (*SigningKey, bool, error) {
	key, rec, err := newSigningKeyRecord(app, box)
	if err != nil {
		return nil, false, err
	}
	rotated := false
	err = app.RunInTransaction(func(tx core.App) error {
		if onlyIfActiveKID != "" && !isActiveKey(tx, onlyIfActiveKID) {
			return nil
		}
		rotated = true
		existing, err := tx.FindAllRecords(SigningKeysCollection)
		if err != nil {
			return err
		}
		for _, old := range existing {
			if err := retireOrDrop(tx, old, dropPrevious, now); err != nil {
				return err
			}
		}
		return tx.Save(rec)
	})
	if err != nil || !rotated {
		return nil, false, err
	}
	return key, true, nil
}

func isActiveKey(app core.App, kid string) bool {
	_, err := app.FindFirstRecordByFilter(SigningKeysCollection, "kid = {:kid} && state = 'active'", dbx.Params{"kid": kid})
	return err == nil
}

// ActiveKeyDecrypts reports whether box can decrypt the active key. It is
// true when no key exists yet.
func ActiveKeyDecrypts(app core.App, box *cryptobox.Box) (bool, error) {
	rec, err := app.FindFirstRecordByFilter(SigningKeysCollection, "state = 'active'")
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	_, err = decodeSigningKey(rec, box)
	return err == nil, nil
}

// PublicJWKS lists the active key first, then keys retired within
// RetiredKeyRetention.
func PublicJWKS(app core.App, now time.Time) (JWKS, error) {
	cutoff := now.UTC().Add(-RetiredKeyRetention).Format(types.DefaultDateLayout)
	recs, err := app.FindRecordsByFilter(SigningKeysCollection,
		"state = 'active' || retired_at > {:cutoff}", "state,-created", 0, 0, dbx.Params{"cutoff": cutoff})
	if err != nil {
		return JWKS{}, err
	}
	jwks := JWKS{Keys: make([]JWK, 0, len(recs))}
	for _, rec := range recs {
		var jwk JWK
		if err := rec.UnmarshalJSONField("public_jwk", &jwk); err != nil {
			return JWKS{}, fmt.Errorf("decode jwk %s: %w", rec.GetString("kid"), err)
		}
		jwks.Keys = append(jwks.Keys, jwk)
	}
	return jwks, nil
}

func createFirstKey(app core.App, box *cryptobox.Box) (*SigningKey, error) {
	key, rec, err := newSigningKeyRecord(app, box)
	if err != nil {
		return nil, err
	}
	if err := app.Save(rec); err != nil {
		// A concurrent first request may have won the active-key unique index.
		active, findErr := app.FindFirstRecordByFilter(SigningKeysCollection, "state = 'active'")
		if findErr != nil {
			return nil, err
		}
		return decodeSigningKey(active, box)
	}
	return key, nil
}

func retireOrDrop(tx core.App, old *core.Record, dropPrevious bool, now time.Time) error {
	if dropPrevious {
		return tx.Delete(old)
	}
	if old.GetString("state") != keyStateActive {
		return nil
	}
	old.Set("state", keyStateRetired)
	old.Set("retired_at", now.UTC())
	return tx.Save(old)
}

func newSigningKeyRecord(app core.App, box *cryptobox.Box) (*SigningKey, *core.Record, error) {
	col, err := app.FindCollectionByNameOrId(SigningKeysCollection)
	if err != nil {
		return nil, nil, fmt.Errorf("%s is missing; apply migrations first by starting the server once or running `migrate up`: %w", SigningKeysCollection, err)
	}
	private, err := rsa.GenerateKey(rand.Reader, signingKeyBits)
	if err != nil {
		return nil, nil, err
	}
	kid, err := token.New(16)
	if err != nil {
		return nil, nil, err
	}
	encrypted, err := encryptPrivateKey(private, box)
	if err != nil {
		return nil, nil, err
	}
	rec := core.NewRecord(col)
	rec.Set("kid", kid)
	rec.Set("alg", SigningAlgorithm)
	rec.Set("private_key", encrypted)
	rec.Set("public_jwk", publicJWK(kid, &private.PublicKey))
	rec.Set("state", keyStateActive)
	return &SigningKey{ID: kid, Private: private}, rec, nil
}

func encryptPrivateKey(private *rsa.PrivateKey, box *cryptobox.Box) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return "", err
	}
	return box.Encrypt(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
}

func decodeSigningKey(rec *core.Record, box *cryptobox.Box) (*SigningKey, error) {
	plain, err := box.Decrypt(rec.GetString("private_key"))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(plain))
	if block == nil {
		return nil, errors.New("private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	private, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return &SigningKey{ID: rec.GetString("kid"), Private: private}, nil
}

func publicJWK(kid string, public *rsa.PublicKey) JWK {
	return JWK{
		KeyType:   "RSA",
		Use:       "sig",
		Algorithm: SigningAlgorithm,
		KeyID:     kid,
		Modulus:   base64.RawURLEncoding.EncodeToString(public.N.Bytes()),
		Exponent:  base64.RawURLEncoding.EncodeToString(big.NewInt(int64(public.E)).Bytes()),
	}
}
