package sendouts

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/mail"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

const idlePoll = 5 * time.Second

// Service delivers Sendout Recipients one at a time in queue order.
type Service struct {
	app  core.App
	send func(*mailer.Message) error
	now  func() time.Time
	wake chan struct{}
}

func New(app core.App) *Service {
	return &Service{
		app:  app,
		send: func(msg *mailer.Message) error { return app.NewMailClient().Send(msg) },
		now:  func() time.Time { return time.Now().UTC() },
		wake: make(chan struct{}, 1),
	}
}

// Notify wakes an idle worker after rows are queued, so a new Sendout starts
// without waiting for the next idle poll.
func (s *Service) Notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) Start(ctx context.Context) {
	go s.run(ctx)
}

func (s *Service) run(ctx context.Context) {
	if err := s.RecoverInterrupted(); err != nil {
		log.Printf("[sendouts] recover interrupted recipients: %v", err)
	}
	for {
		processed, err := s.ProcessNext()
		if err != nil {
			log.Printf("[sendouts] process recipient: %v", err)
		}
		if !s.wait(ctx, processed) {
			return
		}
	}
}

// wait paces deliveries at the configured rate. Only an idle worker listens
// to Notify, so a wake-up cannot shorten the gap between two emails.
func (s *Service) wait(ctx context.Context, processed bool) bool {
	delay, wake := idlePoll, s.wake
	if processed {
		delay, wake = Interval(RatePerMinute(s.app)), nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-wake:
		return true
	case <-timer.C:
		return true
	}
}

// RecoverInterrupted fails rows a previous process claimed but never
// finished. The email may already have gone out, so they are not resent
// automatically.
func (s *Service) RecoverInterrupted() error {
	_, err := s.app.DB().Update(RecipientsCollection,
		dbx.Params{"status": StatusFailed, "reason": ReasonInterrupted, "updated": s.now().Format(dbDateLayout)},
		dbx.HashExp{"status": StatusSending},
	).Execute()
	return err
}

// ProcessNext delivers the oldest pending row. It reports false when the
// queue is empty or the row was cancelled before the worker claimed it.
func (s *Service) ProcessNext() (bool, error) {
	row, err := s.claimNext()
	if err != nil || row == nil {
		return false, err
	}
	status, reason := s.deliver(row)
	return true, s.finish(row, status, reason)
}

// claimNext moves the oldest pending row to `sending` with a guarded UPDATE,
// so a concurrent Cancel either wins before the claim or leaves the row alone.
func (s *Service) claimNext() (*core.Record, error) {
	rows, err := s.app.FindRecordsByFilter(RecipientsCollection, "status = 'pending'", "queued_at,id", 1, 0)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	stamp := s.now().Format(dbDateLayout)
	res, err := s.app.DB().NewQuery(
		"UPDATE " + RecipientsCollection + " SET status = 'sending', attempts = attempts + 1, last_attempt_at = {:now}, updated = {:now} WHERE id = {:id} AND status = 'pending'",
	).Bind(dbx.Params{"now": stamp, "id": rows[0].Id}).Execute()
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return nil, err
	}
	return s.app.FindRecordById(RecipientsCollection, rows[0].Id)
}

// deliver returns the row's final status and reason.
func (s *Service) deliver(row *core.Record) (string, string) {
	user, status, reason := s.recipientUser(row)
	if user == nil {
		return status, reason
	}
	if !s.app.Settings().SMTP.Enabled {
		return StatusFailed, ReasonSMTPDisabled
	}
	sendout, err := s.app.FindRecordById(SendoutsCollection, row.GetString("sendout"))
	if err != nil {
		log.Printf("[sendouts] load sendout for recipient %s: %v", row.Id, err)
		return StatusFailed, ReasonDeliveryFailed
	}
	row.Set("email", user.Email())
	if err := s.send(s.message(sendout, user.Email())); err != nil {
		log.Printf("[sendouts] deliver recipient %s of sendout %s: %v", row.Id, sendout.Id, err)
		return StatusFailed, ReasonDeliveryFailed
	}
	return StatusSent, ""
}

// recipientUser rechecks eligibility at send time: a User disabled, unverified
// or deleted since the Sendout was created is skipped rather than emailed.
func (s *Service) recipientUser(row *core.Record) (*core.Record, string, string) {
	userID := row.GetString("user")
	if userID == "" {
		return nil, StatusSkipped, ReasonUserDeleted
	}
	user, err := s.app.FindRecordById("users", userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, StatusSkipped, ReasonUserDeleted
	}
	if err != nil {
		log.Printf("[sendouts] load user for recipient %s: %v", row.Id, err)
		return nil, StatusFailed, ReasonDeliveryFailed
	}
	if !Eligible(user) {
		return nil, StatusSkipped, ReasonUserIneligible
	}
	return user, "", ""
}

func (s *Service) message(sendout *core.Record, to string) *mailer.Message {
	meta := s.app.Settings().Meta
	return &mailer.Message{
		From:    mail.Address{Name: meta.SenderName, Address: meta.SenderAddress},
		To:      []mail.Address{{Address: to}},
		Subject: sendout.GetString("subject"),
		HTML:    sendout.GetString("html"),
		Text:    sendout.GetString("text"),
	}
}

func (s *Service) finish(row *core.Record, status, reason string) error {
	row.Set("status", status)
	row.Set("reason", reason)
	if status == StatusSent {
		row.Set("sent_at", s.now())
	}
	return s.app.Save(row)
}
