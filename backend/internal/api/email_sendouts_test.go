package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func sendoutEvent(
	t *testing.T,
	app core.App,
	method, target, id string,
	body any,
	auth *core.Record,
) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	raw := []byte("{}")
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	e := &core.RequestEvent{App: app, Event: router.Event{Request: req, Response: response}}
	e.Auth = auth
	return e, response
}

func sendoutAPIStatus(t *testing.T, err error) int {
	t.Helper()
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an ApiError", err)
	}
	return apiErr.Status
}

func TestSettingsEmailSendoutRate(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}

	e, response := sendoutEvent(t, app, http.MethodGet, "/api/panel/settings", "", nil, nil)
	if err := h.getSettings(e); err != nil {
		t.Fatalf("getSettings() error = %v", err)
	}
	var current SettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil || current.EmailSendoutRatePerMinute != 30 {
		t.Fatalf("default rate = %d (%v), want 30", current.EmailSendoutRatePerMinute, err)
	}

	for _, rate := range []int{0, 601} {
		e, _ := sendoutEvent(t, app, http.MethodPatch, "/api/panel/settings", "", SettingsUpdateRequest{EmailSendoutRatePerMinute: &rate}, nil)
		if err := h.updateSettings(e); err == nil || sendoutAPIStatus(t, err) != http.StatusBadRequest {
			t.Fatalf("updateSettings(rate=%d) error = %v, want 400", rate, err)
		}
	}

	rate := 45
	e, response = sendoutEvent(t, app, http.MethodPatch, "/api/panel/settings", "", SettingsUpdateRequest{EmailSendoutRatePerMinute: &rate}, nil)
	if err := h.updateSettings(e); err != nil {
		t.Fatalf("updateSettings(rate=45) error = %v", err)
	}
	var updated SettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil || updated.EmailSendoutRatePerMinute != 45 {
		t.Fatalf("updated rate = %d (%v), want 45", updated.EmailSendoutRatePerMinute, err)
	}
}
