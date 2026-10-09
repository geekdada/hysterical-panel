package monitoring

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/subscriptions"
)

// usableMeteredUsersFilter selects the Users a User Monitor covers. Legacy
// Unmetered Users have no Allowance Window, so they are left out.
const usableMeteredUsersFilter = "status = 'active' && verified = true && subscription_required = true"

// BindHooks cancels a User's firing Alerts before the User is deleted.
// PocketBase then clears the Alert's user reference, and the email snapshot
// keeps the history readable.
func (s *Service) BindHooks() {
	s.app.OnRecordDelete("users").BindFunc(func(e *core.RecordEvent) error {
		alerts, err := e.App.FindRecordsByFilter("alerts", "user = {:u} && status = 'firing'", "", 0, 0, map[string]any{"u": e.Record.Id})
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, alert := range alerts {
			if err := cancelAlertIn(e.App, alert, "user_deleted", now); err != nil {
				return err
			}
		}
		return e.Next()
	})
}

// evaluateUserMonitor evaluates every usable metered User and then cancels
// Alerts whose User left that scope.
func (s *Service) evaluateUserMonitor(monitor *core.Record, now time.Time) error {
	users, err := s.app.FindRecordsByFilter("users", usableMeteredUsersFilter, "", 0, 0)
	if err != nil {
		return err
	}
	subscribed := make(map[string]bool, len(users))
	for _, user := range users {
		hasSubscription, err := s.evaluateUser(monitor, user, now)
		if err != nil {
			log.Printf("[monitoring] monitor=%s user=%s: %v", monitor.Id, user.Id, err)
		}
		subscribed[user.Id] = hasSubscription
	}
	return s.cancelUserAlertsOutOfScope(monitor.Id, subscribed, now)
}

// evaluateUser reports whether the User still has a current User
// Subscription. An unknown state counts as subscribed so an error never
// cancels an Alert.
func (s *Service) evaluateUser(monitor, user *core.Record, now time.Time) (bool, error) {
	state, err := subscriptions.Current(s.app, user.Id, now)
	if err != nil {
		return true, err
	}
	if state == nil {
		return false, nil
	}
	met, value, err := s.userCondition(monitor, user.Id, state, now)
	if err != nil {
		return true, err
	}
	subject := alertSubject{user: user}
	firing, err := s.findFiring(monitor.Id, subject)
	if err != nil {
		return true, err
	}
	return true, s.applyDecision(DecideCondition(firing != nil, met), monitor, subject, firing, value, now)
}

// cancelUserAlertsOutOfScope ends Alerts without a Notification when their
// User became unusable or lost the current User Subscription.
func (s *Service) cancelUserAlertsOutOfScope(monitorID string, subscribed map[string]bool, now time.Time) error {
	firing, err := s.app.FindRecordsByFilter("alerts", "monitor = {:m} && user != '' && status = 'firing'", "", 0, 0, map[string]any{"m": monitorID})
	if err != nil {
		return err
	}
	for _, alert := range firing {
		hasSubscription, usable := subscribed[alert.GetString("user")]
		reason := ""
		switch {
		case !usable:
			reason = "user_unavailable"
		case !hasSubscription:
			reason = "subscription_ended"
		default:
			continue
		}
		if err := s.cancelAlert(alert, reason, now); err != nil {
			return err
		}
	}
	return nil
}

// userCondition returns whether the Monitor's condition holds and the figures
// both admin Notifications and a later User email need.
func (s *Service) userCondition(monitor *core.Record, userID string, state *subscriptions.State, now time.Time) (bool, map[string]any, error) {
	switch monitor.GetString("kind") {
	case KindLowAllowance:
		return lowAllowanceCondition(monitor, state)
	case KindExpiringSubscription:
		return s.expiringCondition(monitor, userID, state, now)
	default:
		return false, nil, fmt.Errorf("unsupported user monitor kind %q", monitor.GetString("kind"))
	}
}

func lowAllowanceCondition(monitor *core.Record, state *subscriptions.State) (bool, map[string]any, error) {
	percent, err := configInt(monitor, "threshold_percent")
	if err != nil {
		return false, nil, err
	}
	value := map[string]any{
		"used_tx_bytes":        state.Used.Tx,
		"used_rx_bytes":        state.Used.Rx,
		"allowance_bytes":      state.Allowance,
		"remaining_bytes":      state.Remaining,
		"threshold_percent":    percent,
		"window_ends_at":       dateString(state.WindowEndsAt()),
		"subscription_ends_at": dateString(state.Grant.GetDateTime("ends_at").Time()),
	}
	return IsLowAllowance(state.Remaining, state.Allowance, percent), value, nil
}

func (s *Service) expiringCondition(monitor *core.Record, userID string, state *subscriptions.State, now time.Time) (bool, map[string]any, error) {
	days, err := configInt(monitor, "threshold_days")
	if err != nil {
		return false, nil, err
	}
	queued, err := subscriptions.Queued(s.app, userID, now)
	if err != nil {
		return false, nil, err
	}
	end := state.Grant.GetDateTime("ends_at").Time()
	value := map[string]any{
		"subscription_ends_at": dateString(end),
		"remaining_seconds":    int64(end.Sub(now).Seconds()),
		"threshold_days":       days,
	}
	if queued != nil {
		value["next_subscription_starts_at"] = dateString(queued.GetDateTime("starts_at").Time())
	}
	return IsExpiring(end.Sub(now), days, queued != nil), value, nil
}
