package api

import (
	"encoding/json"
	"errors"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/automaticemails"
)

const (
	automaticTemplatesCollection = "automatic_email_templates"
	minLowThresholdPercent       = 1
	maxLowThresholdPercent       = 99
)

func (h *Handlers) listAutomaticEmails(e *core.RequestEvent) error {
	kinds := make([]AutomaticEmailKindSummary, 0, len(automaticemails.Kinds))
	for _, kind := range automaticemails.Kinds {
		rec, err := h.findAutomaticTemplate(kind)
		if err != nil {
			return err
		}
		kinds = append(kinds, automaticKindSummary(rec))
	}
	return ok(e, AutomaticEmailsResponse{
		SMTPEnabled: h.smtpEnabled(),
		AppName:     h.app.Settings().Meta.AppName,
		FrontendURL: h.publicConfig.FrontendURL,
		Kinds:       kinds,
	})
}

func (h *Handlers) getAutomaticEmail(e *core.RequestEvent) error {
	rec, err := h.automaticTemplateFromPath(e)
	if err != nil {
		return err
	}
	return ok(e, automaticKindDetail(rec))
}

func (h *Handlers) saveAutomaticEmailTemplate(e *core.RequestEvent) error {
	rec, err := h.automaticTemplateFromPath(e)
	if err != nil {
		return err
	}
	var in AutomaticEmailTemplateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	subject, err := validateAutomaticTemplate(in)
	if err != nil {
		return err
	}
	rec.Set("subject", subject)
	rec.Set("html", in.HTML)
	rec.Set("text", in.Text)
	rec.Set("content", in.Content)
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save template", err)
	}
	return ok(e, automaticKindDetail(rec))
}

func (h *Handlers) updateAutomaticEmail(e *core.RequestEvent) error {
	rec, err := h.automaticTemplateFromPath(e)
	if err != nil {
		return err
	}
	var in AutomaticEmailKindUpdateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if err := applyAutomaticKindUpdate(rec, in); err != nil {
		return err
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to update automatic email", err)
	}
	return ok(e, automaticKindDetail(rec))
}

func applyAutomaticKindUpdate(rec *core.Record, in AutomaticEmailKindUpdateRequest) error {
	if in.ThresholdPercent != nil {
		if automaticemails.Kind(rec.GetString("kind")) != automaticemails.KindLowAllowance {
			return apis.NewBadRequestError("threshold_percent is only valid for low_allowance", nil)
		}
		if *in.ThresholdPercent < minLowThresholdPercent || *in.ThresholdPercent > maxLowThresholdPercent {
			return apis.NewBadRequestError("threshold_percent must be between 1 and 99", nil)
		}
		rec.Set("threshold_percent", *in.ThresholdPercent)
	}
	if in.Enabled != nil {
		if *in.Enabled && !hasAutomaticTemplate(rec) {
			return apis.NewBadRequestError("save a template before enabling", nil)
		}
		rec.Set("enabled", *in.Enabled)
	}
	return nil
}

func validateAutomaticTemplate(in AutomaticEmailTemplateRequest) (string, error) {
	subject, err := normalizeSendoutSubject(in.Subject)
	if err != nil {
		return "", err
	}
	body := EmailSendoutCreateRequest{HTML: in.HTML, Text: in.Text, Content: in.Content}
	if err := validateSendoutBody(body); err != nil {
		return "", err
	}
	err = automaticemails.ValidateTemplate(automaticemails.Template{Subject: subject, HTML: in.HTML, Text: in.Text})
	return subject, automaticTemplateValidationError(err)
}

func automaticTemplateValidationError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, automaticemails.ErrUnknownPlaceholder):
		return apis.NewBadRequestError("unknown placeholder", nil)
	case errors.Is(err, automaticemails.ErrBrokenPlaceholder):
		return apis.NewBadRequestError("a placeholder was split by formatting; retype it as plain text", nil)
	case errors.Is(err, automaticemails.ErrEncodedPlaceholder):
		return apis.NewBadRequestError("placeholders cannot be used inside links", nil)
	}
	return apis.NewBadRequestError("invalid template", err)
}

func (h *Handlers) automaticTemplateFromPath(e *core.RequestEvent) (*core.Record, error) {
	kind, found := automaticemails.ParseKind(e.Request.PathValue("kind"))
	if !found {
		return nil, apis.NewNotFoundError("automatic email not found", nil)
	}
	return h.findAutomaticTemplate(kind)
}

func (h *Handlers) findAutomaticTemplate(kind automaticemails.Kind) (*core.Record, error) {
	rec, err := h.app.FindFirstRecordByData(automaticTemplatesCollection, "kind", string(kind))
	if err != nil {
		return nil, apis.NewNotFoundError("automatic email not found", err)
	}
	return rec, nil
}

func hasAutomaticTemplate(rec *core.Record) bool {
	return rec.GetString("subject") != "" && rec.GetString("html") != "" && rec.GetString("text") != ""
}

func automaticKindSummary(rec *core.Record) AutomaticEmailKindSummary {
	kind := rec.GetString("kind")
	summary := AutomaticEmailKindSummary{
		Kind:        kind,
		Enabled:     rec.GetBool("enabled"),
		Subject:     rec.GetString("subject"),
		HasTemplate: hasAutomaticTemplate(rec),
		Updated:     rec.GetString("updated"),
	}
	if automaticemails.Kind(kind) == automaticemails.KindLowAllowance {
		percent := rec.GetInt("threshold_percent")
		summary.ThresholdPercent = &percent
	}
	return summary
}

func automaticKindDetail(rec *core.Record) AutomaticEmailKindDetail {
	detail := AutomaticEmailKindDetail{
		AutomaticEmailKindSummary: automaticKindSummary(rec),
		HTML:                      rec.GetString("html"),
		Text:                      rec.GetString("text"),
	}
	if raw := rec.GetString("content"); raw != "" && raw != "null" {
		_ = json.Unmarshal([]byte(raw), &detail.Content)
	}
	return detail
}
