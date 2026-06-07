package api

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	passkeySessionKindLogin        = "login"
	passkeySessionKindRegistration = "registration"
	passkeyChallengeTTL            = 5 * time.Minute
)

// NewWebAuthn initializes the panel WebAuthn relying party.
func NewWebAuthn(rpID string, origins []string) (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{
		RPID:                  rpID,
		RPDisplayName:         "Hysterical Panel",
		RPOrigins:             origins,
		AttestationPreference: protocol.PreferNoAttestation,
		Timeouts: webauthn.TimeoutsConfig{
			Login: webauthn.TimeoutConfig{
				Enforce: true,
				Timeout: passkeyChallengeTTL,
			},
			Registration: webauthn.TimeoutConfig{
				Enforce: true,
				Timeout: passkeyChallengeTTL,
			},
		},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyRequired(),
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		},
	})
}

type passkeyFinishInput struct {
	ChallengeID string          `json:"challenge_id"`
	Credential  json.RawMessage `json:"credential"`
	Name        *string         `json:"name"`
}

type passkeyRateLimiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	clients map[string]passkeyRateClient
}

type passkeyRateClient struct {
	count int
	reset time.Time
}

func newPasskeyRateLimiter(max int, window time.Duration) *passkeyRateLimiter {
	return &passkeyRateLimiter{
		max:     max,
		window:  window,
		clients: map[string]passkeyRateClient{},
	}
}

func (l *passkeyRateLimiter) allow(key string, now time.Time) bool {
	if l == nil {
		return true
	}
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	client := l.clients[key]
	if client.reset.IsZero() || now.After(client.reset) {
		l.clients[key] = passkeyRateClient{count: 1, reset: now.Add(l.window)}
		l.cleanup(now)
		return true
	}
	if client.count >= l.max {
		return false
	}
	client.count++
	l.clients[key] = client
	return true
}

func (l *passkeyRateLimiter) cleanup(now time.Time) {
	for key, client := range l.clients {
		if now.After(client.reset) {
			delete(l.clients, key)
		}
	}
}

type webAuthnUser struct {
	record      *core.Record
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte {
	return []byte(u.record.Id)
}

func (u *webAuthnUser) WebAuthnName() string {
	return u.record.Email()
}

func (u *webAuthnUser) WebAuthnDisplayName() string {
	return u.record.Email()
}

func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func (h *Handlers) passkeysEnabled() bool {
	return h.passkeys != nil
}

func (h *Handlers) requirePasskeys() error {
	if !h.passkeysEnabled() {
		return apis.NewBadRequestError("passkeys are not configured", nil)
	}
	return nil
}

func (h *Handlers) passkeyLoginOptions(e *core.RequestEvent) error {
	if err := h.requirePasskeys(); err != nil {
		return err
	}
	if !h.passkeyLimit.allow(e.RealIP(), time.Now().UTC()) {
		return apis.NewTooManyRequestsError("too many passkey login attempts", nil)
	}
	h.deleteExpiredPasskeySessions()

	assertion, session, err := h.passkeys.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationRequired),
	)
	if err != nil {
		return apis.NewBadRequestError("failed to start passkey login", err)
	}
	challengeID, err := h.createPasskeySession(passkeySessionKindLogin, "", session)
	if err != nil {
		return apis.NewBadRequestError("failed to store passkey challenge", err)
	}
	return ok(e, map[string]any{
		"challenge_id": challengeID,
		"options":      assertion.Response,
	})
}

func (h *Handlers) passkeyLoginFinish(e *core.RequestEvent) error {
	if err := h.requirePasskeys(); err != nil {
		return err
	}
	in, err := bindPasskeyFinish(e)
	if err != nil {
		return err
	}
	session, err := h.consumePasskeySession(in.ChallengeID, passkeySessionKindLogin)
	if err != nil {
		return err
	}

	req := passkeyCredentialRequest(e.Request, in.Credential)
	user, credential, err := h.passkeys.FinishPasskeyLogin(h.discoverablePasskeyUser, *session, req)
	if err != nil {
		return apis.NewBadRequestError("passkey authentication failed", err)
	}
	panelUser, ok := user.(*webAuthnUser)
	if !ok || panelUser.record == nil {
		return apis.NewBadRequestError("passkey authentication failed", nil)
	}
	if panelUser.record.GetString("status") != "active" {
		return apis.NewForbiddenError("account is disabled", nil)
	}
	if err := h.updatePasskeyCredential(panelUser.record.Id, credential); err != nil {
		return apis.NewBadRequestError("failed to update passkey", err)
	}
	return apis.RecordAuthResponse(e, panelUser.record, "passkey", nil)
}

func (h *Handlers) rejectPasswordForPasskeyUser(e *core.RecordAuthRequestEvent) error {
	if !h.passkeysEnabled() || e.AuthMethod != core.MFAMethodPassword {
		return nil
	}
	records, err := h.passkeyCredentialRecordsForUser(e.Record.Id)
	if err != nil {
		return apis.NewBadRequestError("failed to read passkeys", err)
	}
	if len(records) == 0 {
		return nil
	}
	return apis.NewForbiddenError("password login is disabled for accounts with passkeys", nil)
}

func (h *Handlers) passkeyRegistrationOptions(e *core.RequestEvent) error {
	if err := h.requirePasskeys(); err != nil {
		return err
	}
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	if u.GetString("status") != "active" {
		return apis.NewForbiddenError("active account required", nil)
	}
	h.deleteExpiredPasskeySessions()

	waUser, err := h.webAuthnUserForRecord(u)
	if err != nil {
		return apis.NewBadRequestError("failed to read passkeys", err)
	}
	creation, session, err := h.passkeys.BeginRegistration(
		waUser,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyRequired(),
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(webauthn.Credentials(waUser.credentials).CredentialDescriptors()),
		webauthn.WithExtensions(map[string]any{"credProps": true}),
	)
	if err != nil {
		return apis.NewBadRequestError("failed to start passkey enrollment", err)
	}
	challengeID, err := h.createPasskeySession(passkeySessionKindRegistration, u.Id, session)
	if err != nil {
		return apis.NewBadRequestError("failed to store passkey challenge", err)
	}
	return ok(e, map[string]any{
		"challenge_id": challengeID,
		"options":      creation.Response,
	})
}

func (h *Handlers) passkeyRegistrationFinish(e *core.RequestEvent) error {
	if err := h.requirePasskeys(); err != nil {
		return err
	}
	in, err := bindPasskeyFinish(e)
	if err != nil {
		return err
	}
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	if u.GetString("status") != "active" {
		return apis.NewForbiddenError("active account required", nil)
	}
	session, err := h.consumePasskeySession(in.ChallengeID, passkeySessionKindRegistration)
	if err != nil {
		return err
	}
	if string(session.UserID) != u.Id {
		return apis.NewBadRequestError("passkey challenge does not match user", nil)
	}

	waUser, err := h.webAuthnUserForRecord(u)
	if err != nil {
		return apis.NewBadRequestError("failed to read passkeys", err)
	}
	req := passkeyCredentialRequest(e.Request, in.Credential)
	credential, err := h.passkeys.FinishRegistration(waUser, *session, req)
	if err != nil {
		return apis.NewBadRequestError("passkey enrollment failed", err)
	}
	created, err := h.storePasskeyCredential(u, credential, strOr(in.Name, "Passkey"))
	if err != nil {
		return apis.NewBadRequestError("failed to store passkey", err)
	}
	return ok(e, publicPasskey(created))
}

func (h *Handlers) listPasskeys(e *core.RequestEvent) error {
	if err := h.requirePasskeys(); err != nil {
		return err
	}
	userID := e.Request.PathValue("id")
	if _, err := h.app.FindRecordById("users", userID); err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	records, err := h.passkeyCredentialRecordsForUser(userID)
	if err != nil {
		return apis.NewBadRequestError("failed to list passkeys", err)
	}
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, publicPasskey(record))
	}
	return ok(e, out)
}

func (h *Handlers) deletePasskey(e *core.RequestEvent) error {
	if err := h.requirePasskeys(); err != nil {
		return err
	}
	userID := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("passkey_credentials", e.Request.PathValue("passkeyId"))
	if err != nil {
		return apis.NewNotFoundError("passkey not found", err)
	}
	if record.GetString("user") != userID {
		return apis.NewNotFoundError("passkey not found", nil)
	}
	if err := h.app.Delete(record); err != nil {
		return apis.NewBadRequestError("failed to delete passkey", err)
	}
	return ok(e, map[string]any{"deleted": true})
}

func bindPasskeyFinish(e *core.RequestEvent) (passkeyFinishInput, error) {
	var in passkeyFinishInput
	if err := e.BindBody(&in); err != nil {
		return in, apis.NewBadRequestError("invalid body", err)
	}
	if strings.TrimSpace(in.ChallengeID) == "" || len(in.Credential) == 0 || bytes.Equal(in.Credential, []byte("null")) {
		return in, apis.NewBadRequestError("challenge_id and credential are required", nil)
	}
	return in, nil
}

func passkeyCredentialRequest(base *http.Request, raw json.RawMessage) *http.Request {
	req := base.Clone(base.Context())
	req.Body = io.NopCloser(bytes.NewReader(raw))
	req.ContentLength = int64(len(raw))
	req.Header = base.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	return req
}

func (h *Handlers) createPasskeySession(kind, userID string, session *webauthn.SessionData) (string, error) {
	coll, err := h.app.FindCollectionByNameOrId("passkey_sessions")
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	if session.Expires.IsZero() {
		session.Expires = time.Now().UTC().Add(passkeyChallengeTTL)
	}
	challengeID := security.RandomString(32)
	record := core.NewRecord(coll)
	record.Set("challenge_id", challengeID)
	record.Set("kind", kind)
	if userID != "" {
		record.Set("user", userID)
	}
	record.Set("session_data", string(raw))
	record.Set("expires_at", session.Expires.UTC())
	if err := h.app.Save(record); err != nil {
		return "", err
	}
	return challengeID, nil
}

func (h *Handlers) consumePasskeySession(challengeID, kind string) (*webauthn.SessionData, error) {
	challengeID = strings.TrimSpace(challengeID)
	record, err := h.app.FindFirstRecordByFilter(
		"passkey_sessions",
		"challenge_id = {:challenge} && kind = {:kind}",
		dbx.Params{"challenge": challengeID, "kind": kind},
	)
	if err != nil {
		return nil, apis.NewBadRequestError("invalid or expired passkey challenge", err)
	}
	defer func() {
		_ = h.app.Delete(record)
	}()
	if !record.GetDateTime("consumed_at").IsZero() {
		return nil, apis.NewBadRequestError("invalid or expired passkey challenge", nil)
	}
	if expiresAt := record.GetDateTime("expires_at").Time(); !expiresAt.IsZero() && time.Now().UTC().After(expiresAt) {
		return nil, apis.NewBadRequestError("invalid or expired passkey challenge", nil)
	}
	var session webauthn.SessionData
	if err := record.UnmarshalJSONField("session_data", &session); err != nil {
		return nil, apis.NewBadRequestError("invalid passkey challenge", err)
	}
	return &session, nil
}

func (h *Handlers) deleteExpiredPasskeySessions() {
	rows, err := h.app.FindRecordsByFilter(
		"passkey_sessions",
		"expires_at <= {:now} || consumed_at != ''",
		"",
		50,
		0,
		dbx.Params{"now": time.Now().UTC().Format("2006-01-02 15:04:05.000Z")},
	)
	if err != nil {
		return
	}
	for _, row := range rows {
		_ = h.app.Delete(row)
	}
}

func (h *Handlers) discoverablePasskeyUser(rawID, userHandle []byte) (webauthn.User, error) {
	credentialID := encodePasskeyBytes(rawID)
	record, err := h.app.FindFirstRecordByFilter(
		"passkey_credentials",
		"credential_id = {:id}",
		dbx.Params{"id": credentialID},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("credential not found")
		}
		return nil, err
	}
	userID := record.GetString("user")
	if len(userHandle) > 0 && string(userHandle) != userID {
		return nil, fmt.Errorf("credential user mismatch")
	}
	u, err := h.app.FindRecordById("users", userID)
	if err != nil {
		return nil, err
	}
	return h.webAuthnUserForRecord(u)
}

func (h *Handlers) webAuthnUserForRecord(u *core.Record) (*webAuthnUser, error) {
	records, err := h.passkeyCredentialRecordsForUser(u.Id)
	if err != nil {
		return nil, err
	}
	credentials := make([]webauthn.Credential, 0, len(records))
	for _, record := range records {
		credential, err := credentialFromRecord(record)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, *credential)
	}
	return &webAuthnUser{record: u, credentials: credentials}, nil
}

func (h *Handlers) passkeyCredentialRecordsForUser(userID string) ([]*core.Record, error) {
	return h.app.FindRecordsByFilter(
		"passkey_credentials",
		"user = {:user}",
		"-created",
		0,
		0,
		dbx.Params{"user": userID},
	)
}

func credentialFromRecord(record *core.Record) (*webauthn.Credential, error) {
	var credential webauthn.Credential
	if err := record.UnmarshalJSONField("credential", &credential); err != nil {
		return nil, err
	}
	return &credential, nil
}

func (h *Handlers) storePasskeyCredential(u *core.Record, credential *webauthn.Credential, name string) (*core.Record, error) {
	coll, err := h.app.FindCollectionByNameOrId("passkey_credentials")
	if err != nil {
		return nil, err
	}
	record := core.NewRecord(coll)
	record.Set("user", u.Id)
	record.Set("credential_id", encodePasskeyBytes(credential.ID))
	record.Set("user_handle", encodePasskeyBytes([]byte(u.Id)))
	record.Set("rp_id", h.passkeys.Config.RPID)
	applyCredentialToRecord(record, credential)
	record.Set("name", cleanPasskeyName(name))
	if err := h.app.Save(record); err != nil {
		return nil, err
	}
	return record, nil
}

func (h *Handlers) updatePasskeyCredential(userID string, credential *webauthn.Credential) error {
	record, err := h.app.FindFirstRecordByFilter(
		"passkey_credentials",
		"credential_id = {:id}",
		dbx.Params{"id": encodePasskeyBytes(credential.ID)},
	)
	if err != nil {
		return err
	}
	if record.GetString("user") != userID {
		return fmt.Errorf("credential user mismatch")
	}
	applyCredentialToRecord(record, credential)
	record.Set("last_used_at", time.Now().UTC())
	return h.app.Save(record)
}

func applyCredentialToRecord(record *core.Record, credential *webauthn.Credential) {
	raw, _ := json.Marshal(credential)
	transports := passkeyTransports(credential)
	transportsRaw, _ := json.Marshal(transports)
	record.Set("credential", string(raw))
	record.Set("transports", string(transportsRaw))
	record.Set("sign_count", int64(credential.Authenticator.SignCount))
	record.Set("backup_eligible", credential.Flags.BackupEligible)
	record.Set("backup_state", credential.Flags.BackupState)
	record.Set("clone_warning", credential.Authenticator.CloneWarning)
}

func publicPasskey(record *core.Record) map[string]any {
	var transports []string
	_ = record.UnmarshalJSONField("transports", &transports)
	return map[string]any{
		"id":              record.Id,
		"name":            record.GetString("name"),
		"transports":      transports,
		"sign_count":      record.GetInt("sign_count"),
		"backup_eligible": record.GetBool("backup_eligible"),
		"backup_state":    record.GetBool("backup_state"),
		"clone_warning":   record.GetBool("clone_warning"),
		"created":         record.GetString("created"),
		"updated":         record.GetString("updated"),
		"last_used_at":    record.GetString("last_used_at"),
	}
}

func passkeyTransports(credential *webauthn.Credential) []string {
	out := make([]string, 0, len(credential.Transport))
	for _, transport := range credential.Transport {
		out = append(out, string(transport))
	}
	return out
}

func encodePasskeyBytes(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func cleanPasskeyName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Passkey"
	}
	if len(name) > 128 {
		return name[:128]
	}
	return name
}
