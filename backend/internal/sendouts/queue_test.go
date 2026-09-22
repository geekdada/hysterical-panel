package sendouts

import (
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

var testNow = time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)

func TestCreateQueuesOnePendingRecipientPerUser(t *testing.T) {
	app := newTestApp(t)
	admin := newTestUser(t, app, "admin@example.com", "active", true)
	user := newTestUser(t, app, "user@example.com", "active", true)

	sendout, err := Create(app, testDraft(), []*core.Record{admin, user}, admin, testNow)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sendout.GetString("created_by_email") != "admin@example.com" || sendout.GetString("subject") != testDraft().Subject {
		t.Fatalf("sendout = %v, want creator snapshot and draft subject", sendout)
	}
	rows := recipientsOf(t, app, sendout.Id)
	if len(rows) != 2 {
		t.Fatalf("queued %d recipients, want 2", len(rows))
	}
	for _, row := range rows {
		if row.GetString("status") != StatusPending || row.GetInt("attempts") != 0 || !row.GetDateTime("queued_at").Time().Equal(testNow) {
			t.Fatalf("recipient %s = status %q attempts %d queued_at %v", row.Id, row.GetString("status"), row.GetInt("attempts"), row.GetDateTime("queued_at"))
		}
	}
}

func TestCountRecipientsAndStatus(t *testing.T) {
	app := newTestApp(t)
	users := []*core.Record{
		newTestUser(t, app, "a@example.com", "active", true),
		newTestUser(t, app, "b@example.com", "active", true),
		newTestUser(t, app, "c@example.com", "active", true),
		newTestUser(t, app, "d@example.com", "active", true),
	}
	sendout := newTestSendout(t, app, testNow, users...)
	rows := recipientsOf(t, app, sendout.Id)
	setRecipientStatus(t, app, rows[0], StatusSending, "")
	setRecipientStatus(t, app, rows[1], StatusSent, "")
	setRecipientStatus(t, app, rows[2], StatusFailed, ReasonDeliveryFailed)

	counts, err := CountRecipients(app, sendout.Id)
	if err != nil {
		t.Fatalf("CountRecipients() error = %v", err)
	}
	got := counts[sendout.Id]
	if want := (Counts{Total: 4, Pending: 2, Sent: 1, Failed: 1}); got != want {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
	if Status(sendout, got) != SendoutStatusSending {
		t.Fatalf("status with pending rows = %q, want sending", Status(sendout, got))
	}
	if Status(sendout, Counts{Total: 4, Sent: 4}) != SendoutStatusCompleted {
		t.Fatal("status without pending rows must be completed")
	}
	sendout.Set("cancelled_at", testNow)
	if Status(sendout, got) != SendoutStatusCancelled {
		t.Fatal("status with cancelled_at must be cancelled")
	}
}

func TestCancelOnlyTouchesPendingRecipients(t *testing.T) {
	app := newTestApp(t)
	sendout := newTestSendout(t, app, testNow,
		newTestUser(t, app, "a@example.com", "active", true),
		newTestUser(t, app, "b@example.com", "active", true),
		newTestUser(t, app, "c@example.com", "active", true),
	)
	rows := recipientsOf(t, app, sendout.Id)
	setRecipientStatus(t, app, rows[1], StatusSending, "")
	setRecipientStatus(t, app, rows[2], StatusSent, "")

	if err := Cancel(app, sendout, testNow); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	want := []string{StatusCancelled, StatusSending, StatusSent}
	for i, row := range rows {
		if got := reload(t, app, RecipientsCollection, row.Id).GetString("status"); got != want[i] {
			t.Errorf("row %d status = %q, want %q", i, got, want[i])
		}
	}
	if reload(t, app, SendoutsCollection, sendout.Id).GetDateTime("cancelled_at").IsZero() {
		t.Fatal("Cancel() did not set cancelled_at")
	}
	if err := Cancel(app, sendout, testNow); !errors.Is(err, ErrCancelled) {
		t.Fatalf("second Cancel() error = %v, want ErrCancelled", err)
	}
}

func TestCancelRejectsFinishedSendout(t *testing.T) {
	app := newTestApp(t)
	sendout := newTestSendout(t, app, testNow, newTestUser(t, app, "a@example.com", "active", true))
	setRecipientStatus(t, app, recipientsOf(t, app, sendout.Id)[0], StatusSent, "")

	if err := Cancel(app, sendout, testNow); !errors.Is(err, ErrNothingToCancel) {
		t.Fatalf("Cancel() error = %v, want ErrNothingToCancel", err)
	}
}

func TestRequeueMovesFailedRecipientsToQueueTail(t *testing.T) {
	app := newTestApp(t)
	sendout := newTestSendout(t, app, testNow,
		newTestUser(t, app, "a@example.com", "active", true),
		newTestUser(t, app, "b@example.com", "active", true),
		newTestUser(t, app, "c@example.com", "active", true),
	)
	rows := recipientsOf(t, app, sendout.Id)
	setRecipientStatus(t, app, rows[0], StatusFailed, ReasonDeliveryFailed)
	setRecipientStatus(t, app, rows[1], StatusFailed, ReasonSMTPDisabled)
	setRecipientStatus(t, app, rows[2], StatusSkipped, ReasonUserIneligible)
	later := testNow.Add(time.Hour)

	n, err := Requeue(app, sendout, []string{rows[0].Id}, later)
	if err != nil || n != 1 {
		t.Fatalf("Requeue(one) = %d, %v; want 1", n, err)
	}
	first := reload(t, app, RecipientsCollection, rows[0].Id)
	if first.GetString("status") != StatusPending || first.GetString("reason") != "" || !first.GetDateTime("queued_at").Time().Equal(later) {
		t.Fatalf("requeued row = status %q reason %q queued_at %v", first.GetString("status"), first.GetString("reason"), first.GetDateTime("queued_at"))
	}
	n, err = Requeue(app, sendout, nil, later)
	if err != nil || n != 1 {
		t.Fatalf("Requeue(all failed) = %d, %v; want 1", n, err)
	}
	if got := reload(t, app, RecipientsCollection, rows[2].Id).GetString("status"); got != StatusSkipped {
		t.Fatalf("skipped row status = %q, want skipped", got)
	}
}

func TestRequeueRejectsCancelledSendout(t *testing.T) {
	app := newTestApp(t)
	sendout := newTestSendout(t, app, testNow, newTestUser(t, app, "a@example.com", "active", true))
	if err := Cancel(app, sendout, testNow); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if _, err := Requeue(app, sendout, nil, testNow); !errors.Is(err, ErrCancelled) {
		t.Fatalf("Requeue() error = %v, want ErrCancelled", err)
	}
}

func TestDeletingUsersKeepsSendoutHistory(t *testing.T) {
	app := newTestApp(t)
	admin := newTestUser(t, app, "admin@example.com", "active", true)
	user := newTestUser(t, app, "user@example.com", "active", true)
	sendout, err := Create(app, testDraft(), []*core.Record{user}, admin, testNow)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := app.Delete(user); err != nil {
		t.Fatalf("delete recipient user: %v", err)
	}
	if err := app.Delete(admin); err != nil {
		t.Fatalf("delete creator: %v", err)
	}

	rows := recipientsOf(t, app, sendout.Id)
	if len(rows) != 1 || rows[0].GetString("user") != "" || rows[0].GetString("email") != "user@example.com" {
		t.Fatalf("recipient after user delete = %v, want kept row with cleared user", rows)
	}
	kept := reload(t, app, SendoutsCollection, sendout.Id)
	if kept.GetString("created_by") != "" || kept.GetString("created_by_email") != "admin@example.com" {
		t.Fatalf("sendout after creator delete = %v, want cleared relation and kept email", kept)
	}
}
