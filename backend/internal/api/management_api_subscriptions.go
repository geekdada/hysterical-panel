package api

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func (h *Handlers) mgmtListSubscriptionTypes(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("subscription_types", "hidden = false", "name", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list subscription types", err)
	}
	out := make([]ManagementSubscriptionType, 0, len(records))
	for _, record := range records {
		out = append(out, ManagementSubscriptionType{
			ID:             record.Id,
			Name:           record.GetString("name"),
			AllowanceBytes: int64(record.GetInt("allowance_bytes")),
			ResetDays:      record.GetInt("reset_days"),
		})
	}
	return ok(e, out)
}

func (h *Handlers) mgmtGrantSubscription(e *core.RequestEvent) error {
	return mgmtSubscriptionStateConflict(e, h.grantSubscription)
}

func (h *Handlers) mgmtTopUpSubscription(e *core.RequestEvent) error {
	return mgmtSubscriptionStateConflict(e, h.topUpSubscription)
}

func (h *Handlers) mgmtTerminateSubscription(e *core.RequestEvent) error {
	return mgmtSubscriptionStateConflict(e, h.terminateSubscription)
}

func mgmtSubscriptionStateConflict(e *core.RequestEvent, next func(*core.RequestEvent) error) error {
	err := next(e)
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
		return err
	}
	switch apiErr.Message {
	case subscriptionQueuedConflictMessage, subscriptionNotCurrentConflictMessage, subscriptionEndedConflictMessage:
		return apis.NewApiError(http.StatusConflict, apiErr.Message, nil)
	default:
		return err
	}
}
