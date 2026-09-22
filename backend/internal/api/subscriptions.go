package api

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"hysterical-panel/internal/subscriptions"
)

func publicSubscriptionType(record *core.Record) SubscriptionType {
	return SubscriptionType{
		ID: record.Id, Name: record.GetString("name"),
		AllowanceBytes: int64(record.GetInt("allowance_bytes")),
		ResetDays:      record.GetInt("reset_days"), Hidden: record.GetBool("hidden"),
		Assigned: record.GetBool("ever_assigned"),
	}
}

func (h *Handlers) listSubscriptionTypes(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("subscription_types", "", "name", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list subscription types", err)
	}
	out := make([]SubscriptionType, 0, len(records))
	for _, record := range records {
		out = append(out, publicSubscriptionType(record))
	}
	return ok(e, out)
}

func (h *Handlers) createSubscriptionType(e *core.RequestEvent) error {
	var in SubscriptionTypeCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 128 || in.AllowanceBytes <= 0 || (in.ResetDays != 30 && in.ResetDays != 360) {
		return apis.NewBadRequestError("invalid subscription type", nil)
	}
	collection, err := h.app.FindCollectionByNameOrId("subscription_types")
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	record.Set("name", name)
	record.Set("allowance_bytes", in.AllowanceBytes)
	record.Set("reset_days", in.ResetDays)
	if err := h.app.Save(record); err != nil {
		return apis.NewBadRequestError("failed to create subscription type", err)
	}
	return ok(e, publicSubscriptionType(record))
}

func (h *Handlers) updateSubscriptionType(e *core.RequestEvent) error {
	var in SubscriptionTypeUpdateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	var updated *core.Record
	var kicks []string
	err := h.app.RunInTransaction(func(app core.App) error {
		record, err := app.FindRecordById("subscription_types", e.Request.PathValue("id"))
		if err != nil {
			return apis.NewNotFoundError("subscription type not found", err)
		}
		if in.Name != nil {
			name := strings.TrimSpace(*in.Name)
			if name == "" || len(name) > 128 {
				return apis.NewBadRequestError("invalid subscription type name", nil)
			}
			record.Set("name", name)
		}
		if in.ResetDays != nil {
			if *in.ResetDays != 30 && *in.ResetDays != 360 {
				return apis.NewBadRequestError("reset_days must be 30 or 360", nil)
			}
			if record.GetBool("ever_assigned") && record.GetInt("reset_days") != *in.ResetDays {
				return apis.NewBadRequestError("cannot change assigned type reset interval", nil)
			}
			record.Set("reset_days", *in.ResetDays)
		}
		if in.Hidden != nil {
			record.Set("hidden", *in.Hidden)
		}
		if in.AllowanceBytes != nil && *in.AllowanceBytes != int64(record.GetInt("allowance_bytes")) {
			if *in.AllowanceBytes <= 0 {
				return apis.NewBadRequestError("allowance_bytes must be positive", nil)
			}
			record.Set("allowance_bytes", *in.AllowanceBytes)
			grants, err := app.FindRecordsByFilter("user_subscriptions", "subscription_type = {:id} && terminated_at = ''", "", 0, 0, map[string]any{"id": record.Id})
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			for _, grant := range grants {
				if now.Before(grant.GetDateTime("starts_at").Time()) || !now.Before(grant.GetDateTime("ends_at").Time()) {
					continue
				}
				index, _ := subscriptions.WindowAt(grant.GetDateTime("starts_at").Time(), now, record.GetInt("reset_days"))
				used, _ := subscriptions.WindowUsage(grant, index)
				subscriptions.SetWindow(grant, index, used, 0)
				if err := app.Save(grant); err != nil {
					return err
				}
				if used.Total() >= *in.AllowanceBytes {
					kicks = append(kicks, grant.GetString("user"))
				}
			}
		}
		if err := app.Save(record); err != nil {
			return err
		}
		updated = record
		return nil
	})
	if err != nil {
		return err
	}
	for _, id := range kicks {
		go h.kickUser(id)
	}
	return ok(e, publicSubscriptionType(updated))
}

func (h *Handlers) deleteSubscriptionType(e *core.RequestEvent) error {
	err := h.app.RunInTransaction(func(app core.App) error {
		record, err := app.FindRecordById("subscription_types", e.Request.PathValue("id"))
		if err != nil {
			return apis.NewNotFoundError("subscription type not found", err)
		}
		if record.GetBool("ever_assigned") {
			return apis.NewBadRequestError("assigned subscription types cannot be deleted", nil)
		}
		return app.Delete(record)
	})
	if err != nil {
		return err
	}
	return ok(e, DeleteResponse{Deleted: true})
}

func grantView(app core.App, grant *core.Record, now time.Time) (UserSubscription, error) {
	typ, err := app.FindRecordById("subscription_types", grant.GetString("subscription_type"))
	if err != nil {
		return UserSubscription{}, err
	}
	start := grant.GetDateTime("starts_at").Time()
	end := grant.GetDateTime("ends_at").Time()
	grantUsage := subscriptions.GrantUsage(grant)
	out := UserSubscription{
		ID: grant.Id, SubscriptionType: typ.Id, TypeName: typ.GetString("name"),
		StartsAt: grant.GetString("starts_at"), EndsAt: grant.GetString("ends_at"), TerminatedAt: grant.GetString("terminated_at"),
		AllowanceBytes: int64(typ.GetInt("allowance_bytes")),
		GrantTxBytes:   grantUsage.Tx, GrantRxBytes: grantUsage.Rx,
	}
	switch {
	case !grant.GetDateTime("terminated_at").IsZero():
		out.Status = "terminated"
	case now.Before(start):
		out.Status = "queued"
	case !now.Before(end):
		out.Status = "expired"
	default:
		out.Status = "current"
		index, _ := subscriptions.WindowAt(start, now, typ.GetInt("reset_days"))
		used, extra := subscriptions.WindowUsage(grant, index)
		out.UsedTxBytes, out.UsedRxBytes, out.UsedBytes = used.Tx, used.Rx, used.Total()
		out.AllowanceBytes += extra
		windowEnd := start.Add(time.Duration(index+1) * time.Duration(typ.GetInt("reset_days")) * 24 * time.Hour)
		if windowEnd.After(end) {
			windowEnd = end
		}
		out.WindowEndsAt = windowEnd.UTC().Format(time.RFC3339)
	}
	out.RemainingBytes = out.AllowanceBytes - out.UsedBytes
	out.OverAllowance = out.Status == "current" && out.RemainingBytes <= 0
	return out, nil
}

func (h *Handlers) listUserSubscriptions(e *core.RequestEvent) error {
	userID := e.Request.PathValue("id")
	if _, err := h.app.FindRecordById("users", userID); err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	out, err := userSubscriptionViews(h.app, userID, time.Now().UTC())
	if err != nil {
		return err
	}
	return ok(e, out)
}

func userSubscriptionViews(app core.App, userID string, now time.Time) ([]UserSubscription, error) {
	records, err := app.FindRecordsByFilter("user_subscriptions", "user = {:u}", "-starts_at", 0, 0, map[string]any{"u": userID})
	if err != nil {
		return nil, err
	}
	out := make([]UserSubscription, 0, len(records))
	for _, record := range records {
		item, err := grantView(app, record, now)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (h *Handlers) rescheduleSubscription(e *core.RequestEvent) error {
	var in SubscriptionRescheduleRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	start, err := types.ParseDateTime(in.StartsAt)
	if err != nil || start.IsZero() {
		return apis.NewBadRequestError("starts_at must be a datetime", err)
	}
	userID := e.Request.PathValue("id")
	now := time.Now().UTC()
	err = h.app.RunInTransaction(func(app core.App) error {
		grant, err := app.FindRecordById("user_subscriptions", e.Request.PathValue("subscriptionId"))
		if err != nil || grant.GetString("user") != userID {
			return apis.NewNotFoundError("subscription not found", err)
		}
		return subscriptions.Reschedule(app, userID, grant.Id, start.Time().UTC().Truncate(time.Millisecond), now)
	})
	for _, rule := range []error{subscriptions.ErrRescheduleNotCurrent, subscriptions.ErrRescheduleFuture, subscriptions.ErrRescheduleEnded, subscriptions.ErrRescheduleOverlap} {
		if errors.Is(err, rule) {
			return apis.NewBadRequestError(rule.Error(), nil)
		}
	}
	if err != nil {
		return err
	}
	out, err := userSubscriptionViews(h.app, userID, now)
	if err != nil {
		return err
	}
	return ok(e, out)
}

func (h *Handlers) grantSubscription(e *core.RequestEvent) error {
	var in SubscriptionGrantRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	var created *core.Record
	err := h.app.RunInTransaction(func(app core.App) error {
		user, err := app.FindRecordById("users", e.Request.PathValue("id"))
		if err != nil {
			return apis.NewNotFoundError("user not found", err)
		}
		typ, err := app.FindRecordById("subscription_types", in.SubscriptionType)
		if err != nil || typ.GetBool("hidden") {
			return apis.NewBadRequestError("subscription type unavailable", err)
		}
		now := time.Now().UTC().Truncate(time.Millisecond)
		grants, err := app.FindRecordsByFilter("user_subscriptions", "user = {:u} && terminated_at = ''", "", 0, 0, map[string]any{"u": user.Id})
		if err != nil {
			return err
		}
		start := now
		future := false
		for _, grant := range grants {
			from, to := grant.GetDateTime("starts_at").Time(), grant.GetDateTime("ends_at").Time()
			if !now.Before(to) {
				continue
			}
			if now.Before(from) {
				future = true
				continue
			}
			start = to
		}
		if future {
			return apis.NewBadRequestError("a future subscription is already queued", nil)
		}
		coll, err := app.FindCollectionByNameOrId("user_subscriptions")
		if err != nil {
			return err
		}
		created = core.NewRecord(coll)
		created.Set("user", user.Id)
		created.Set("subscription_type", typ.Id)
		created.Set("starts_at", start)
		created.Set("ends_at", start.Add(360*24*time.Hour))
		if err := app.Save(created); err != nil {
			return err
		}
		typ.Set("ever_assigned", true)
		if err := app.Save(typ); err != nil {
			return err
		}
		if !user.GetBool("subscription_required") {
			user.Set("subscription_required", true)
			if err := app.Save(user); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	view, err := grantView(h.app, created, time.Now().UTC())
	if err != nil {
		return err
	}
	return ok(e, view)
}

func (h *Handlers) topUpSubscription(e *core.RequestEvent) error {
	var in SubscriptionTopUpRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if in.AllowanceBytes <= 0 {
		return apis.NewBadRequestError("allowance_bytes must be positive", nil)
	}
	var changed *core.Record
	err := h.app.RunInTransaction(func(app core.App) error {
		grant, err := app.FindRecordById("user_subscriptions", e.Request.PathValue("subscriptionId"))
		if err != nil || grant.GetString("user") != e.Request.PathValue("id") {
			return apis.NewNotFoundError("subscription not found", err)
		}
		state, err := subscriptions.Current(app, grant.GetString("user"), time.Now().UTC())
		if err != nil {
			return err
		}
		if state == nil || state.Grant.Id != grant.Id {
			return apis.NewBadRequestError("only a current subscription can be topped up", nil)
		}
		// Bound the effective allowance, not only the top-up total, so Current can still read the grant.
		if in.AllowanceBytes > math.MaxInt64-state.Allowance {
			return apis.NewBadRequestError("allowance overflow", nil)
		}
		subscriptions.SetWindow(grant, state.Window, state.Used, state.Extra+in.AllowanceBytes)
		if err := app.Save(grant); err != nil {
			return err
		}
		changed = grant
		return nil
	})
	if err != nil {
		return err
	}
	view, err := grantView(h.app, changed, time.Now().UTC())
	if err != nil {
		return err
	}
	return ok(e, view)
}

func (h *Handlers) terminateSubscription(e *core.RequestEvent) error {
	var kick bool
	err := h.app.RunInTransaction(func(app core.App) error {
		grant, err := app.FindRecordById("user_subscriptions", e.Request.PathValue("subscriptionId"))
		if err != nil || grant.GetString("user") != e.Request.PathValue("id") {
			return apis.NewNotFoundError("subscription not found", err)
		}
		now := time.Now().UTC()
		if !grant.GetDateTime("terminated_at").IsZero() || !now.Before(grant.GetDateTime("ends_at").Time()) {
			return apis.NewBadRequestError("subscription has already ended", nil)
		}
		kick = !now.Before(grant.GetDateTime("starts_at").Time())
		grant.Set("terminated_at", now)
		grant.Set("expiry_processed", true)
		return app.Save(grant)
	})
	if err != nil {
		return err
	}
	if kick {
		go h.kickUser(e.Request.PathValue("id"))
	}
	return ok(e, DeleteResponse{Deleted: true})
}
