package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/notifications"
	"hysterical-panel/internal/token"
)

func TestNotificationChannelMigrationSchema(t *testing.T) {
	app := newMigratedTestApp(t)

	channels, err := app.FindCollectionByNameOrId("notification_channels")
	if err != nil {
		t.Fatalf("find notification_channels: %v", err)
	}
	for _, name := range []string{
		"name", "name_key", "service", "url_encrypted", "enabled",
		"last_test_status", "last_tested_at", "last_test_error",
	} {
		if channels.Fields.GetByName(name) == nil {
			t.Errorf("notification_channels missing %q", name)
		}
	}
	if field, ok := channels.Fields.GetByName("url_encrypted").(*core.TextField); !ok || !field.Hidden {
		t.Fatal("notification_channels.url_encrypted must be a hidden text field")
	}
	lastTestError, ok := channels.Fields.GetByName("last_test_error").(*core.SelectField)
	if !ok || strings.Join(lastTestError.Values, ",") != "timed_out,delivery_failed" {
		t.Fatal("notification_channels.last_test_error must only allow safe error codes")
	}
	if channels.GetIndex("idx_notification_channels_name_key") == "" {
		t.Fatal("notification_channels must enforce unique name_key")
	}

	sessions, err := app.FindCollectionByNameOrId("passkey_sessions")
	if err != nil {
		t.Fatalf("find passkey_sessions: %v", err)
	}
	kind, ok := sessions.Fields.GetByName("kind").(*core.SelectField)
	if !ok {
		t.Fatal("passkey_sessions.kind is not a select field")
	}
	if !strings.Contains(strings.Join(kind.Values, ","), "sensitive_field_reveal") {
		t.Fatal("passkey_sessions.kind must allow sensitive_field_reveal")
	}
	if field, ok := sessions.Fields.GetByName("scope").(*core.TextField); !ok || !field.Hidden {
		t.Fatal("passkey_sessions.scope must be a hidden text field")
	}
}

func TestNormalizeNotificationChannelName(t *testing.T) {
	name, key, err := normalizeNotificationChannelName("  Ops Alerts  ")
	if err != nil {
		t.Fatalf("normalizeNotificationChannelName() error = %v", err)
	}
	if name != "Ops Alerts" || key != "ops alerts" {
		t.Fatalf("normalizeNotificationChannelName() = (%q, %q), want (%q, %q)", name, key, "Ops Alerts", "ops alerts")
	}
	if _, _, err := normalizeNotificationChannelName("Ops\nAlerts"); err == nil {
		t.Fatal("normalizeNotificationChannelName() accepted a control character")
	}
}

func TestPublicNotificationChannelNeverExposesEncryptedURL(t *testing.T) {
	collection := core.NewBaseCollection("notification_channels")
	collection.Fields.Add(&core.TextField{Name: "name"})
	collection.Fields.Add(&core.TextField{Name: "service"})
	collection.Fields.Add(&core.BoolField{Name: "enabled"})
	collection.Fields.Add(&core.TextField{Name: "last_test_status"})
	collection.Fields.Add(&core.TextField{Name: "last_test_error"})
	collection.Fields.Add(&core.TextField{Name: "url_encrypted"})
	record := core.NewRecord(collection)
	record.Set("name", "Ops Alerts")
	record.Set("service", "slack")
	record.Set("enabled", true)
	record.Set("last_test_status", "failed")
	record.Set("last_test_error", "delivery_failed")
	record.Set("url_encrypted", "not-a-URL-or-a-secret")

	public := publicNotificationChannel(record)
	if strings.Contains(public.Name+public.Service+public.LastTestError, "not-a-URL-or-a-secret") {
		t.Fatalf("public notification channel leaked encrypted URL: %#v", public)
	}
	if public.LastTestedAt != nil {
		t.Fatalf("empty optional last_tested_at = %q, want nil", *public.LastTestedAt)
	}
}

func TestNotificationChannelCRUDKeepsURLSecretAndResetsOnlyOnReplacement(t *testing.T) {
	app := newMigratedTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("cryptobox.New: %v", err)
	}
	h := &Handlers{app: app, box: box, notifications: notifications.New()}
	const firstURL = "generic://notification-target.example.test?template=json&token=first-secret"

	e, response := notificationChannelEvent(t, app, http.MethodPost, "", NotificationChannelCreateRequest{
		Name: "  Ops Alerts  ",
		URL:  firstURL,
	})
	if err := h.createNotificationChannel(e); err != nil {
		t.Fatalf("createNotificationChannel() error = %v", err)
	}
	assertNotificationChannelResponseDoesNotContain(t, response.Body.String(), firstURL)
	var created NotificationChannel
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Enabled || created.LastTestedAt != nil || created.LastTestStatus != "never" {
		t.Fatalf("create defaults = %#v, want disabled and untested", created)
	}
	rec, err := app.FindRecordById("notification_channels", created.ID)
	if err != nil {
		t.Fatalf("reload created channel: %v", err)
	}
	if rec.GetString("name") != "Ops Alerts" || rec.GetString("name_key") != "ops alerts" {
		t.Fatalf("persisted normalized name = %q/%q", rec.GetString("name"), rec.GetString("name_key"))
	}
	if rec.GetString("url_encrypted") == firstURL {
		t.Fatal("created channel persisted a raw URL")
	}

	e, _ = notificationChannelEvent(t, app, http.MethodPost, "", NotificationChannelCreateRequest{
		Name: "ops alerts",
		URL:  "generic://another-target.example.test?template=json",
	})
	if err := h.createNotificationChannel(e); err == nil {
		t.Fatal("createNotificationChannel accepted a case-insensitive duplicate name")
	}

	rec.Set("last_test_status", "succeeded")
	rec.Set("last_tested_at", time.Now().UTC())
	if err := app.Save(rec); err != nil {
		t.Fatalf("seed last test state: %v", err)
	}
	enabled := true
	e, response = notificationChannelEvent(t, app, http.MethodPatch, created.ID, NotificationChannelUpdateRequest{
		Name:    ptr(" Renamed alerts "),
		Enabled: &enabled,
	})
	if err := h.updateNotificationChannel(e); err != nil {
		t.Fatalf("name-only update error = %v", err)
	}
	assertNotificationChannelResponseDoesNotContain(t, response.Body.String(), firstURL)
	rec, err = app.FindRecordById("notification_channels", created.ID)
	if err != nil {
		t.Fatalf("reload after name-only update: %v", err)
	}
	if rec.GetString("last_test_status") != "succeeded" || rec.GetString("last_tested_at") == "" {
		t.Fatal("name-only update reset test metadata")
	}
	oldCiphertext := rec.GetString("url_encrypted")
	const replacementURL = "generic+https://replacement-target.example.test/notify?token=replacement-secret"
	e, response = notificationChannelEvent(t, app, http.MethodPatch, created.ID, NotificationChannelUpdateRequest{
		URL: ptr(replacementURL),
	})
	if err := h.updateNotificationChannel(e); err != nil {
		t.Fatalf("URL replacement error = %v", err)
	}
	assertNotificationChannelResponseDoesNotContain(t, response.Body.String(), replacementURL)
	rec, err = app.FindRecordById("notification_channels", created.ID)
	if err != nil {
		t.Fatalf("reload after URL replacement: %v", err)
	}
	if rec.GetString("url_encrypted") == oldCiphertext || rec.GetString("url_encrypted") == replacementURL {
		t.Fatal("URL replacement did not persist a new ciphertext")
	}
	if rec.GetString("service") != "generic" || rec.GetString("last_test_status") != "never" ||
		rec.GetString("last_tested_at") != "" || rec.GetString("last_test_error") != "" {
		t.Fatalf("URL replacement state = %#v, want normalized service and cleared test metadata", publicNotificationChannel(rec))
	}

	e, response = notificationChannelEvent(t, app, http.MethodGet, "", nil)
	if err := h.listNotificationChannels(e); err != nil {
		t.Fatalf("listNotificationChannels() error = %v", err)
	}
	assertNotificationChannelResponseDoesNotContain(t, response.Body.String(), replacementURL, rec.GetString("url_encrypted"))

	e, response = notificationChannelEvent(t, app, http.MethodDelete, created.ID, nil)
	if err := h.deleteNotificationChannel(e); err != nil {
		t.Fatalf("deleteNotificationChannel() error = %v", err)
	}
	if !strings.Contains(response.Body.String(), `"deleted":true`) {
		t.Fatalf("delete response = %s, want deleted true", response.Body.String())
	}
	if _, err := app.FindRecordById("notification_channels", created.ID); err == nil {
		t.Fatal("deleteNotificationChannel did not permanently remove the record")
	}
}

func TestCreateNotificationChannelRejectsInvalidURLWithoutPersistingCiphertext(t *testing.T) {
	app := newMigratedTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("cryptobox.New: %v", err)
	}
	h := &Handlers{app: app, box: box, notifications: notifications.New()}
	const rawURL = "smtp://user:provider-secret@example.test:587/?toaddresses=recipient@example.test"
	e, _ := notificationChannelEvent(t, app, http.MethodPost, "", NotificationChannelCreateRequest{Name: "Mail", URL: rawURL})
	if err := h.createNotificationChannel(e); err == nil {
		t.Fatal("createNotificationChannel accepted an unsupported Shoutrrr service")
	}
	records, err := app.FindRecordsByFilter("notification_channels", "", "", 0, 0)
	if err != nil {
		t.Fatalf("list persisted records: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("invalid URL persisted %d notification channels", len(records))
	}
}

func TestTestNotificationChannelPersistsSafeResultForDisabledChannel(t *testing.T) {
	app := newMigratedTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("cryptobox.New: %v", err)
	}
	const rawURL = "generic://notification-target.example.test?template=json&token=secret"
	ciphertext, err := box.Encrypt(rawURL)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	channel := newNotificationChannelRecord(t, app, ciphertext, false)

	fake := &fakeNotificationDelivery{result: notifications.Result{Succeeded: true}}
	h := &Handlers{app: app, box: box, notifications: fake}
	response := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/panel/notification-channels/"+channel.Id+"/test", nil)
	req.SetPathValue("id", channel.Id)
	e := &core.RequestEvent{
		App: app,
		Event: router.Event{
			Request:  req,
			Response: response,
		},
	}
	if err := h.testNotificationChannel(e); err != nil {
		t.Fatalf("testNotificationChannel() error = %v", err)
	}
	if got := fake.url; got != rawURL {
		t.Fatalf("delivery URL = %q, want decrypted URL", got)
	}
	if !strings.HasPrefix(fake.message, notificationChannelTestMessagePrefix) {
		t.Fatalf("test message = %q, want fixed verification prefix", fake.message)
	}
	if strings.Contains(response.Body.String(), rawURL) || strings.Contains(response.Body.String(), ciphertext) {
		t.Fatalf("test response leaked URL material: %s", response.Body.String())
	}
	var body NotificationChannelTestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "succeeded" || body.Error != "" {
		t.Fatalf("test response = %#v, want successful safe result", body)
	}
	reloaded, err := app.FindRecordById("notification_channels", channel.Id)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if reloaded.GetString("last_test_status") != "succeeded" || reloaded.GetString("last_test_error") != "" {
		t.Fatalf("persisted status = %q/%q, want succeeded/empty", reloaded.GetString("last_test_status"), reloaded.GetString("last_test_error"))
	}
}

func TestTestNotificationChannelPersistsSafeFailureCode(t *testing.T) {
	app := newMigratedTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("cryptobox.New: %v", err)
	}
	ciphertext, err := box.Encrypt("generic://notification-target.example.test?template=json")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	channel := newNotificationChannelRecord(t, app, ciphertext, true)
	h := &Handlers{
		app:           app,
		box:           box,
		notifications: &fakeNotificationDelivery{result: notifications.Result{ErrorCode: notifications.ErrorTimedOut}},
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/panel/notification-channels/"+channel.Id+"/test", nil)
	req.SetPathValue("id", channel.Id)
	e := &core.RequestEvent{App: app, Event: router.Event{Request: req, Response: response}}
	if err := h.testNotificationChannel(e); err != nil {
		t.Fatalf("testNotificationChannel() error = %v", err)
	}
	var body NotificationChannelTestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "failed" || body.Error != "timed_out" {
		t.Fatalf("test response = %#v, want failed/timed_out", body)
	}
}

func TestNotificationRevealPasskeySessionIsBoundToChannelScope(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	session := &webauthn.SessionData{Expires: time.Now().UTC().Add(time.Minute)}
	challengeID, err := h.createPasskeySession(
		passkeySessionKindSensitiveFieldReveal,
		"",
		"channel-a",
		session,
	)
	if err != nil {
		t.Fatalf("createPasskeySession: %v", err)
	}
	if _, err := h.consumePasskeySession(
		challengeID,
		passkeySessionKindSensitiveFieldReveal,
		"channel-b",
	); err == nil {
		t.Fatal("consumePasskeySession accepted a challenge for another channel")
	}
	if _, err := h.consumePasskeySession(
		challengeID,
		passkeySessionKindSensitiveFieldReveal,
		"channel-a",
	); err == nil {
		t.Fatal("scope-mismatched challenge was not consumed")
	}
}

func TestNotificationRevealPasskeySessionExpiresAndCannotReplay(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	expiredID, err := h.createPasskeySession(
		passkeySessionKindSensitiveFieldReveal,
		"",
		"channel-a",
		&webauthn.SessionData{Expires: time.Now().UTC().Add(-time.Minute)},
	)
	if err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	if _, err := h.consumePasskeySession(expiredID, passkeySessionKindSensitiveFieldReveal, "channel-a"); err == nil {
		t.Fatal("expired reveal challenge was accepted")
	}
	if _, err := app.FindFirstRecordByFilter("passkey_sessions", "challenge_id = {:id}", map[string]any{"id": expiredID}); err == nil {
		t.Fatal("expired reveal challenge was not consumed")
	}

	challengeID, err := h.createPasskeySession(
		passkeySessionKindSensitiveFieldReveal,
		"",
		"channel-a",
		&webauthn.SessionData{Expires: time.Now().UTC().Add(time.Minute)},
	)
	if err != nil {
		t.Fatalf("create valid session: %v", err)
	}
	if _, err := h.consumePasskeySession(challengeID, passkeySessionKindSensitiveFieldReveal, "channel-a"); err != nil {
		t.Fatalf("consume valid reveal challenge: %v", err)
	}
	if _, err := h.consumePasskeySession(challengeID, passkeySessionKindSensitiveFieldReveal, "channel-a"); err == nil {
		t.Fatal("replayed reveal challenge was accepted")
	}
}

func TestNotificationRevealOptionsRequiresPasskeysAndEnrollment(t *testing.T) {
	app := newMigratedTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("cryptobox.New: %v", err)
	}
	ciphertext, err := box.Encrypt("generic://notification-target.example.test?template=json")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	channel := newNotificationChannelRecord(t, app, ciphertext, false)
	e, _ := notificationChannelEvent(t, app, http.MethodPost, channel.Id, nil)
	if err := (&Handlers{app: app, box: box}).notificationChannelRevealOptions(e); err == nil {
		t.Fatal("reveal options accepted while passkeys are disabled")
	}

	admin := newUsersTestRecord(t, app, "admin@example.test", "admin-auth-string")
	admin.Set("role", "admin")
	admin.Set("auth_string_anytls_hash", token.Sha256Hex("admin-auth-string"))
	if err := app.Save(admin); err != nil {
		t.Fatalf("save admin: %v", err)
	}
	passkeys, err := NewWebAuthn("localhost", []string{"http://localhost"})
	if err != nil {
		t.Fatalf("NewWebAuthn: %v", err)
	}
	e.Auth = admin
	if err := (&Handlers{app: app, box: box, passkeys: passkeys}).notificationChannelRevealOptions(e); err == nil {
		t.Fatal("reveal options accepted for an admin with no registered passkey")
	}
}

type fakeNotificationDelivery struct {
	result  notifications.Result
	url     string
	message string
}

func (f *fakeNotificationDelivery) Validate(rawURL string) (notifications.Service, error) {
	return notifications.ServiceGeneric, nil
}

func (f *fakeNotificationDelivery) Send(rawURL, message string) notifications.Result {
	f.url = rawURL
	f.message = message
	return f.result
}

func newNotificationChannelRecord(
	t *testing.T,
	app core.App,
	ciphertext string,
	enabled bool,
) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("notification_channels")
	if err != nil {
		t.Fatalf("find notification_channels: %v", err)
	}
	record := core.NewRecord(collection)
	record.Set("name", "Ops Alerts")
	record.Set("name_key", "ops alerts")
	record.Set("service", "generic")
	record.Set("url_encrypted", ciphertext)
	record.Set("enabled", enabled)
	record.Set("last_test_status", "never")
	if err := app.Save(record); err != nil {
		t.Fatalf("save notification channel: %v", err)
	}
	return record
}

func notificationChannelEvent(
	t *testing.T,
	app core.App,
	method, id string,
	body any,
) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	var requestBody *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		requestBody = bytes.NewReader(raw)
	} else {
		requestBody = bytes.NewReader(nil)
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/api/panel/notification-channels/"+id, requestBody)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	return &core.RequestEvent{App: app, Event: router.Event{Request: req, Response: response}}, response
}

func assertNotificationChannelResponseDoesNotContain(t *testing.T, body string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(body, secret) {
			t.Fatalf("notification channel response leaked secret %q: %s", secret, body)
		}
	}
}
