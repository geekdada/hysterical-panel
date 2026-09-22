// Package sendouts stores Email Sendouts and delivers their Sendout Recipients
// one at a time through PocketBase's SMTP settings (ADR 0008).
package sendouts

import (
	"log"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	SendoutsCollection   = "email_sendouts"
	RecipientsCollection = "email_sendout_recipients"

	AudienceSingle = "single"
	AudienceAll    = "all"

	StatusPending   = "pending"
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusFailed    = "failed"
	StatusSkipped   = "skipped"
	StatusCancelled = "cancelled"

	ReasonDeliveryFailed = "delivery_failed"
	ReasonSMTPDisabled   = "smtp_disabled"
	ReasonInterrupted    = "interrupted"
	ReasonUserIneligible = "user_ineligible"
	ReasonUserDeleted    = "user_deleted"

	SendoutStatusSending   = "sending"
	SendoutStatusCompleted = "completed"
	SendoutStatusCancelled = "cancelled"

	DefaultRatePerMinute = 30
	MinRatePerMinute     = 1
	MaxRatePerMinute     = 600

	dbDateLayout   = "2006-01-02 15:04:05.000Z"
	eligibleFilter = "status = 'active' && verified = true"
)

var (
	Languages         = []string{"en", "zh-cn"}
	RecipientStatuses = []string{StatusPending, StatusSending, StatusSent, StatusFailed, StatusSkipped, StatusCancelled}
	Reasons           = []string{ReasonDeliveryFailed, ReasonSMTPDisabled, ReasonInterrupted, ReasonUserIneligible, ReasonUserDeleted}
)

// Eligible applies the same active + Verified rule that gates panel login and
// Node Client Auth.
func Eligible(user *core.Record) bool {
	return user.GetString("status") == "active" && user.GetBool("verified")
}

func CountEligible(app core.App) (int64, error) {
	return app.CountRecords("users", dbx.NewExp("status = 'active' AND verified = TRUE"))
}

// FindEligible returns eligible Users ordered by email. An empty search matches
// every eligible User; limit 0 means no limit.
func FindEligible(app core.App, search string, limit int) ([]*core.Record, error) {
	if search == "" {
		return app.FindRecordsByFilter("users", eligibleFilter, "email", limit, 0)
	}
	return app.FindRecordsByFilter("users", eligibleFilter+" && email ~ {:search}", "email", limit, 0, dbx.Params{"search": search})
}

// NormalizeRate maps an unset (zero) rate to the default and caps the rest.
func NormalizeRate(rate int) int {
	if rate < MinRatePerMinute {
		return DefaultRatePerMinute
	}
	if rate > MaxRatePerMinute {
		return MaxRatePerMinute
	}
	return rate
}

func RatePerMinute(app core.App) int {
	records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0)
	if err != nil {
		log.Printf("[sendouts] load rate setting: %v", err)
		return DefaultRatePerMinute
	}
	if len(records) == 0 {
		return DefaultRatePerMinute
	}
	return NormalizeRate(records[0].GetInt("email_sendout_rate_per_minute"))
}

// Interval is the gap the worker leaves between two deliveries.
func Interval(rate int) time.Duration {
	return time.Minute / time.Duration(NormalizeRate(rate))
}
