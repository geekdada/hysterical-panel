package sendouts

import (
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tools/mailer"
)

func newTestService(t *testing.T, sendErr error) (*Service, *[]*mailer.Message) {
	t.Helper()
	app := newTestApp(t)
	app.Settings().SMTP.Enabled = true
	sent := []*mailer.Message{}
	s := New(app)
	s.send = func(msg *mailer.Message) error {
		sent = append(sent, msg)
		return sendErr
	}
	return s, &sent
}

func TestProcessNextSendsStoredContentToCurrentEmail(t *testing.T) {
	s, sent := newTestService(t, nil)
	user := newTestUser(t, s.app, "old@example.com", "active", true)
	sendout := newTestSendout(t, s.app, testNow, user)
	user.Set("email", "new@example.com")
	if err := s.app.Save(user); err != nil {
		t.Fatalf("change email: %v", err)
	}

	processed, err := s.ProcessNext()
	if err != nil || !processed {
		t.Fatalf("ProcessNext() = %v, %v; want true, nil", processed, err)
	}
	if len(*sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(*sent))
	}
	msg := (*sent)[0]
	if msg.To[0].Address != "new@example.com" || msg.Subject != sendout.GetString("subject") ||
		msg.HTML != sendout.GetString("html") || msg.Text != sendout.GetString("text") {
		t.Fatalf("message = %+v, want stored content sent to the current email", msg)
	}
	row := recipientsOf(t, s.app, sendout.Id)[0]
	if row.GetString("status") != StatusSent || row.GetInt("attempts") != 1 ||
		row.GetDateTime("sent_at").IsZero() || row.GetString("email") != "new@example.com" {
		t.Fatalf("recipient = status %q attempts %d sent_at %v email %q",
			row.GetString("status"), row.GetInt("attempts"), row.GetDateTime("sent_at"), row.GetString("email"))
	}
}

func TestProcessNextSkipsUsersWhoLostEligibility(t *testing.T) {
	s, sent := newTestService(t, nil)
	disabled := newTestUser(t, s.app, "disabled@example.com", "active", true)
	deleted := newTestUser(t, s.app, "deleted@example.com", "active", true)
	sendout := newTestSendout(t, s.app, testNow, disabled, deleted)
	disabled.Set("status", "disabled")
	if err := s.app.Save(disabled); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if err := s.app.Delete(deleted); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := s.ProcessNext(); err != nil {
			t.Fatalf("ProcessNext() error = %v", err)
		}
	}
	if len(*sent) != 0 {
		t.Fatalf("sent %d messages, want 0", len(*sent))
	}
	got := map[string]string{}
	for _, row := range recipientsOf(t, s.app, sendout.Id) {
		got[row.GetString("email")] = row.GetString("status") + "/" + row.GetString("reason")
	}
	if got["disabled@example.com"] != "skipped/user_ineligible" || got["deleted@example.com"] != "skipped/user_deleted" {
		t.Fatalf("outcomes = %v", got)
	}
}

func TestProcessNextFailsWhileSMTPIsDisabled(t *testing.T) {
	s, sent := newTestService(t, nil)
	s.app.Settings().SMTP.Enabled = false
	sendout := newTestSendout(t, s.app, testNow, newTestUser(t, s.app, "a@example.com", "active", true))

	if _, err := s.ProcessNext(); err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	row := recipientsOf(t, s.app, sendout.Id)[0]
	if len(*sent) != 0 || row.GetString("status") != StatusFailed || row.GetString("reason") != ReasonSMTPDisabled {
		t.Fatalf("sent %d, recipient %s/%s; want failed/smtp_disabled", len(*sent), row.GetString("status"), row.GetString("reason"))
	}
}

func TestProcessNextRecordsDeliveryFailure(t *testing.T) {
	s, _ := newTestService(t, errors.New("421 service not available"))
	sendout := newTestSendout(t, s.app, testNow, newTestUser(t, s.app, "a@example.com", "active", true))

	if _, err := s.ProcessNext(); err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	row := recipientsOf(t, s.app, sendout.Id)[0]
	if row.GetString("status") != StatusFailed || row.GetString("reason") != ReasonDeliveryFailed || row.GetInt("attempts") != 1 {
		t.Fatalf("recipient = %s/%s attempts %d; want failed/delivery_failed attempts 1",
			row.GetString("status"), row.GetString("reason"), row.GetInt("attempts"))
	}
}

func TestProcessNextFollowsQueueOrder(t *testing.T) {
	s, sent := newTestService(t, nil)
	newTestSendout(t, s.app, testNow, newTestUser(t, s.app, "late@example.com", "active", true))
	newTestSendout(t, s.app, testNow.Add(-time.Minute), newTestUser(t, s.app, "early@example.com", "active", true))

	for i := 0; i < 2; i++ {
		if _, err := s.ProcessNext(); err != nil {
			t.Fatalf("ProcessNext() error = %v", err)
		}
	}
	if len(*sent) != 2 || (*sent)[0].To[0].Address != "early@example.com" {
		t.Fatalf("first delivery went to %v, want early@example.com", (*sent)[0].To)
	}
}

func TestProcessNextReportsEmptyQueue(t *testing.T) {
	s, _ := newTestService(t, nil)
	processed, err := s.ProcessNext()
	if err != nil || processed {
		t.Fatalf("ProcessNext() on empty queue = %v, %v; want false, nil", processed, err)
	}
}

func TestRecoverInterruptedFailsSendingRows(t *testing.T) {
	s, _ := newTestService(t, nil)
	sendout := newTestSendout(t, s.app, testNow, newTestUser(t, s.app, "a@example.com", "active", true))
	row := recipientsOf(t, s.app, sendout.Id)[0]
	setRecipientStatus(t, s.app, row, StatusSending, "")

	if err := s.RecoverInterrupted(); err != nil {
		t.Fatalf("RecoverInterrupted() error = %v", err)
	}
	row = reload(t, s.app, RecipientsCollection, row.Id)
	if row.GetString("status") != StatusFailed || row.GetString("reason") != ReasonInterrupted {
		t.Fatalf("recovered row = %s/%s; want failed/interrupted", row.GetString("status"), row.GetString("reason"))
	}
}

func TestNotifyDoesNotBlock(t *testing.T) {
	s, _ := newTestService(t, nil)
	s.Notify()
	s.Notify()
}
