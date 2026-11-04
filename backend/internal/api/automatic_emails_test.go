package api

import (
	"errors"
	"strings"

	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/automaticemails"
	"net/http/httptest"

	"encoding/json"
	"github.com/pocketbase/pocketbase/core"
	"net/http"
	"testing"
	"time"

	"hysterical-panel/internal/sendouts"
)

func validAutomaticTemplate() AutomaticEmailTemplateRequest {
	return AutomaticEmailTemplateRequest{
		Subject: "Low allowance for {{email}}",
		HTML:    "<p>{{remaining}} left</p>",
		Text:    "{{remaining}} left",
		Content: map[string]any{"type": "doc"},
	}
}

func TestAutomaticEmailsList(t *testing.T) {
	h, _, _ := newSendoutTestHandlers(t)
	e, response := sendoutEvent(t, h.app, http.MethodGet, "/api/panel/automatic-emails", "", nil, nil)
	if err := h.listAutomaticEmails(e); err != nil {
		t.Fatalf("listAutomaticEmails() error = %v", err)
	}
	var out AutomaticEmailsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{"low_allowance", "allowance_exhausted", "allowance_restored"}
	if len(out.Kinds) != len(want) {
		t.Fatalf("kinds = %+v, want %v", out.Kinds, want)
	}
	for i, kind := range out.Kinds {
		if kind.Kind != want[i] {
			t.Fatalf("kind[%d] = %s, want %s", i, kind.Kind, want[i])
		}
		if kind.Enabled || kind.HasTemplate && kind.Subject == "" {
			t.Fatalf("kind %+v should be disabled", kind)
		}
		if (i == 0) != (kind.ThresholdPercent != nil) {
			t.Fatalf("kind %s threshold = %v", kind.Kind, kind.ThresholdPercent)
		}
	}
	if *out.Kinds[0].ThresholdPercent != 10 {
		t.Fatalf("low threshold = %d, want 10", *out.Kinds[0].ThresholdPercent)
	}
}

func automaticCall(t *testing.T, h *Handlers, method, kind string, body any, handler func(*core.RequestEvent) error) (int, *httptest.ResponseRecorder) {
	t.Helper()
	e, response := sendoutEvent(t, h.app, method, "/api/panel/automatic-emails/"+kind, "", body, nil)
	e.Request.SetPathValue("kind", kind)
	err := handler(e)
	if err != nil {
		return sendoutAPIStatus(t, err), response
	}
	return http.StatusOK, response
}

func TestSaveAutomaticEmailTemplateValidates(t *testing.T) {
	h, _, _ := newSendoutTestHandlers(t)
	cases := map[string]func(*AutomaticEmailTemplateRequest){
		"unknown placeholder": func(r *AutomaticEmailTemplateRequest) { r.Text = "{{nope}}"; r.HTML = "<p>{{nope}}</p>" },
		"broken placeholder":  func(r *AutomaticEmailTemplateRequest) { r.HTML = "<p>{{<b>remaining</b>}}</p>" },
		"encoded placeholder": func(r *AutomaticEmailTemplateRequest) {
			r.HTML = `<a href="https://x/%7B%7Bemail%7D%7D">x</a><p>{{remaining}}</p>`
		},
		"empty subject": func(r *AutomaticEmailTemplateRequest) { r.Subject = " " },
		"nil content":   func(r *AutomaticEmailTemplateRequest) { r.Content = nil },
	}
	for name, mutate := range cases {
		in := validAutomaticTemplate()
		mutate(&in)
		status, _ := automaticCall(t, h, http.MethodPut, "low_allowance", in, h.saveAutomaticEmailTemplate)
		if status != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, status)
		}
	}
	status, response := automaticCall(t, h, http.MethodPut, "low_allowance", validAutomaticTemplate(), h.saveAutomaticEmailTemplate)
	var detail AutomaticEmailKindDetail
	if err := json.Unmarshal(response.Body.Bytes(), &detail); status != http.StatusOK || err != nil || !detail.HasTemplate || detail.Content["type"] != "doc" {
		t.Fatalf("valid save = %d %+v (%v)", status, detail, err)
	}
}

func TestAutomaticTemplateValidationMessages(t *testing.T) {
	cases := map[error]string{
		automaticemails.ErrUnknownPlaceholder: "unknown placeholder",
		automaticemails.ErrBrokenPlaceholder:  "a placeholder was split by formatting; retype it as plain text",
		automaticemails.ErrEncodedPlaceholder: "placeholders cannot be used inside links",
	}
	for cause, message := range cases {
		var apiErr *router.ApiError
		if !errors.As(automaticTemplateValidationError(cause), &apiErr) || apiErr.Status != http.StatusBadRequest || !strings.EqualFold(strings.TrimSuffix(apiErr.Message, "."), message) {
			t.Errorf("%v maps to %+v, want 400 %q", cause, apiErr, message)
		}
	}
}

func TestEnableRequiresTemplate(t *testing.T) {
	h, _, _ := newSendoutTestHandlers(t)
	enable := AutomaticEmailKindUpdateRequest{Enabled: ptrTo(true)}
	if status, _ := automaticCall(t, h, http.MethodPatch, "allowance_exhausted", enable, h.updateAutomaticEmail); status != http.StatusBadRequest {
		t.Fatalf("enable without template status = %d, want 400", status)
	}
	automaticCall(t, h, http.MethodPut, "allowance_exhausted", validAutomaticTemplate(), h.saveAutomaticEmailTemplate)
	status, response := automaticCall(t, h, http.MethodPatch, "allowance_exhausted", enable, h.updateAutomaticEmail)
	var detail AutomaticEmailKindDetail
	if err := json.Unmarshal(response.Body.Bytes(), &detail); status != http.StatusOK || err != nil || !detail.Enabled {
		t.Fatalf("enable with template = %d %+v (%v)", status, detail, err)
	}
}

func TestThresholdValidation(t *testing.T) {
	h, _, _ := newSendoutTestHandlers(t)
	for _, percent := range []int{0, 100} {
		in := AutomaticEmailKindUpdateRequest{ThresholdPercent: &percent}
		if status, _ := automaticCall(t, h, http.MethodPatch, "low_allowance", in, h.updateAutomaticEmail); status != http.StatusBadRequest {
			t.Errorf("threshold %d status = %d, want 400", percent, status)
		}
	}
	ten := 10
	in := AutomaticEmailKindUpdateRequest{ThresholdPercent: &ten}
	if status, _ := automaticCall(t, h, http.MethodPatch, "allowance_exhausted", in, h.updateAutomaticEmail); status != http.StatusBadRequest {
		t.Errorf("threshold on exhausted status = %d, want 400", status)
	}
	in = AutomaticEmailKindUpdateRequest{ThresholdPercent: ptrTo(25)}
	status, response := automaticCall(t, h, http.MethodPatch, "low_allowance", in, h.updateAutomaticEmail)
	var detail AutomaticEmailKindDetail
	if err := json.Unmarshal(response.Body.Bytes(), &detail); status != http.StatusOK || err != nil || *detail.ThresholdPercent != 25 {
		t.Fatalf("threshold 25 = %d %+v (%v)", status, detail, err)
	}
}

func TestUnknownKind404(t *testing.T) {
	h, _, _ := newSendoutTestHandlers(t)
	handlers := map[string]func(*core.RequestEvent) error{
		http.MethodGet: h.getAutomaticEmail, http.MethodPut: h.saveAutomaticEmailTemplate, http.MethodPatch: h.updateAutomaticEmail,
	}
	for method, handler := range handlers {
		if status, _ := automaticCall(t, h, method, "nope", validAutomaticTemplate(), handler); status != http.StatusNotFound {
			t.Errorf("%s unknown kind status = %d, want 404", method, status)
		}
	}
}

func TestListEmailSendoutsFiltersBySource(t *testing.T) {
	h, _, admin := newSendoutTestHandlers(t)
	draft, err := validateEmailSendout(validSendoutRequest())
	if err != nil {
		t.Fatalf("validateEmailSendout() error = %v", err)
	}
	now := time.Now().UTC()
	if _, err := sendouts.Create(h.app, draft, nil, admin, now); err != nil {
		t.Fatalf("create manual: %v", err)
	}
	draft.Source, draft.AutomaticKind, draft.Language = sendouts.SourceAutomatic, "low_allowance", sendouts.LanguageBilingual
	if _, err := sendouts.Create(h.app, draft, nil, nil, now); err != nil {
		t.Fatalf("create automatic: %v", err)
	}
	list := func(query string) (int, []EmailSendout) {
		e, response := sendoutEvent(t, h.app, http.MethodGet, "/api/panel/email-sendouts"+query, "", nil, nil)
		if err := h.listEmailSendouts(e); err != nil {
			return sendoutAPIStatus(t, err), nil
		}
		var out []EmailSendout
		if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return http.StatusOK, out
	}
	if _, all := list(""); len(all) != 2 {
		t.Fatalf("unfiltered = %d, want 2", len(all))
	}
	_, manual := list("?source=manual")
	_, automatic := list("?source=automatic")
	if len(manual) != 1 || manual[0].Source != "manual" || manual[0].AutomaticKind != nil {
		t.Fatalf("manual = %+v", manual)
	}
	if len(automatic) != 1 || automatic[0].Source != "automatic" || automatic[0].AutomaticKind == nil || *automatic[0].AutomaticKind != "low_allowance" {
		t.Fatalf("automatic = %+v", automatic)
	}
	if status, _ := list("?source=other"); status != http.StatusBadRequest {
		t.Fatalf("invalid source status = %d, want 400", status)
	}
}

func TestOpenAPIDescribesAutomaticEmails(t *testing.T) {
	spec, err := BuildOpenAPISpec()
	if err != nil {
		t.Fatalf("BuildOpenAPISpec: %v", err)
	}
	for _, path := range []string{"/api/panel/automatic-emails", "/api/panel/automatic-emails/{kind}", "/api/panel/automatic-emails/{kind}/template"} {
		if spec.Paths.Find(path) == nil {
			t.Errorf("OpenAPI is missing %s", path)
		}
	}
}

func ptrTo[T any](v T) *T { return &v }
