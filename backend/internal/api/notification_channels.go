package api

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/notifications"
)

const notificationChannelTestMessagePrefix = "Hysterical Panel test notification at "

// notificationDelivery is deliberately small so handler tests can verify
// persisted test outcomes without contacting external notification services.
type notificationDelivery interface {
	Validate(rawURL string) (notifications.Service, error)
	Send(rawURL, message string) notifications.Result
}

func publicNotificationChannel(rec *core.Record) NotificationChannel {
	lastTestedAt := rec.GetString("last_tested_at")
	var lastTestedAtValue *string
	if lastTestedAt != "" {
		lastTestedAtValue = &lastTestedAt
	}
	return NotificationChannel{
		ID:             rec.Id,
		Name:           rec.GetString("name"),
		Service:        rec.GetString("service"),
		Enabled:        rec.GetBool("enabled"),
		LastTestStatus: rec.GetString("last_test_status"),
		LastTestedAt:   lastTestedAtValue,
		LastTestError:  rec.GetString("last_test_error"),
		Created:        rec.GetString("created"),
		Updated:        rec.GetString("updated"),
	}
}

func (h *Handlers) findNotificationChannel(id string) (*core.Record, error) {
	rec, err := h.app.FindRecordById("notification_channels", id)
	if err != nil {
		return nil, apis.NewNotFoundError("notification channel not found", err)
	}
	return rec, nil
}

func (h *Handlers) listNotificationChannels(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("notification_channels", "", "name_key", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list notification channels", err)
	}
	out := make([]NotificationChannel, 0, len(records))
	for _, record := range records {
		out = append(out, publicNotificationChannel(record))
	}
	return ok(e, out)
}

func (h *Handlers) createNotificationChannel(e *core.RequestEvent) error {
	var in NotificationChannelCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	name, nameKey, err := normalizeNotificationChannelName(in.Name)
	if err != nil {
		return err
	}
	rawURL := strings.TrimSpace(in.URL)
	service, err := h.notifications.Validate(rawURL)
	if err != nil {
		return apis.NewBadRequestError("invalid notification channel URL", nil)
	}
	if err := h.ensureNotificationChannelNameAvailable(nameKey, ""); err != nil {
		return err
	}
	ciphertext, err := h.box.Encrypt(rawURL)
	if err != nil {
		return apis.NewBadRequestError("failed to encrypt notification channel URL", err)
	}
	coll, err := h.app.FindCollectionByNameOrId("notification_channels")
	if err != nil {
		return apis.NewBadRequestError("failed to load notification channels", err)
	}
	rec := core.NewRecord(coll)
	rec.Set("name", name)
	rec.Set("name_key", nameKey)
	rec.Set("service", string(service))
	rec.Set("url_encrypted", ciphertext)
	rec.Set("enabled", in.Enabled != nil && *in.Enabled)
	rec.Set("last_test_status", "never")
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save notification channel", err)
	}
	return ok(e, publicNotificationChannel(rec))
}

func (h *Handlers) updateNotificationChannel(e *core.RequestEvent) error {
	rec, err := h.findNotificationChannel(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	var in NotificationChannelUpdateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if in.Name != nil {
		name, nameKey, err := normalizeNotificationChannelName(*in.Name)
		if err != nil {
			return err
		}
		if err := h.ensureNotificationChannelNameAvailable(nameKey, rec.Id); err != nil {
			return err
		}
		rec.Set("name", name)
		rec.Set("name_key", nameKey)
	}
	if in.URL != nil {
		rawURL := strings.TrimSpace(*in.URL)
		if rawURL == "" {
			return apis.NewBadRequestError("notification channel URL cannot be empty", nil)
		}
		service, err := h.notifications.Validate(rawURL)
		if err != nil {
			return apis.NewBadRequestError("invalid notification channel URL", nil)
		}
		ciphertext, err := h.box.Encrypt(rawURL)
		if err != nil {
			return apis.NewBadRequestError("failed to encrypt notification channel URL", err)
		}
		rec.Set("url_encrypted", ciphertext)
		rec.Set("service", string(service))
		rec.Set("last_test_status", "never")
		rec.Set("last_tested_at", nil)
		rec.Set("last_test_error", "")
	}
	if in.Enabled != nil {
		rec.Set("enabled", *in.Enabled)
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save notification channel", err)
	}
	return ok(e, publicNotificationChannel(rec))
}

func (h *Handlers) deleteNotificationChannel(e *core.RequestEvent) error {
	rec, err := h.findNotificationChannel(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	if err := h.app.Delete(rec); err != nil {
		return apis.NewBadRequestError("failed to delete notification channel", err)
	}
	return ok(e, map[string]any{"deleted": true})
}

func (h *Handlers) testNotificationChannel(e *core.RequestEvent) error {
	rec, err := h.findNotificationChannel(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	rawURL, err := h.box.Decrypt(rec.GetString("url_encrypted"))
	if err != nil {
		return apis.NewBadRequestError("failed to decrypt notification channel URL", err)
	}
	now := time.Now().UTC()
	result := h.notifications.Send(rawURL, notificationChannelTestMessagePrefix+now.Format(time.RFC3339))
	rec.Set("last_tested_at", now)
	if result.Succeeded {
		rec.Set("last_test_status", "succeeded")
		rec.Set("last_test_error", "")
	} else {
		rec.Set("last_test_status", "failed")
		rec.Set("last_test_error", string(result.ErrorCode))
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save notification channel test result", err)
	}
	return ok(e, NotificationChannelTestResponse{
		Status:   rec.GetString("last_test_status"),
		TestedAt: rec.GetString("last_tested_at"),
		Error:    rec.GetString("last_test_error"),
	})
}

func normalizeNotificationChannelName(input string) (name, key string, apiErr error) {
	name = strings.TrimSpace(input)
	if name == "" || utf8.RuneCountInString(name) > 128 {
		return "", "", apis.NewBadRequestError("notification channel name must be between 1 and 128 characters", nil)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", "", apis.NewBadRequestError("notification channel name cannot contain control characters", nil)
		}
	}
	return name, strings.ToLower(name), nil
}

func (h *Handlers) ensureNotificationChannelNameAvailable(nameKey, exceptID string) error {
	rec, err := h.app.FindFirstRecordByFilter(
		"notification_channels",
		"name_key = {:name_key}",
		map[string]any{"name_key": nameKey},
	)
	if errors.Is(err, sql.ErrNoRows) || rec == nil || rec.Id == exceptID {
		return nil
	}
	if err != nil {
		return apis.NewBadRequestError("failed to check notification channel name", err)
	}
	return apis.NewBadRequestError("notification channel name is already in use", nil)
}
