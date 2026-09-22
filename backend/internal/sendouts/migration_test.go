package sendouts

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestEmailSendoutMigrationSchema(t *testing.T) {
	app := newTestApp(t)

	sendouts, err := app.FindCollectionByNameOrId(SendoutsCollection)
	if err != nil {
		t.Fatalf("find %s: %v", SendoutsCollection, err)
	}
	for _, name := range []string{"subject", "language", "audience", "html", "text", "content", "created_by", "created_by_email", "cancelled_at", "created"} {
		if sendouts.Fields.GetByName(name) == nil {
			t.Errorf("%s missing %q", SendoutsCollection, name)
		}
	}

	recipients, err := app.FindCollectionByNameOrId(RecipientsCollection)
	if err != nil {
		t.Fatalf("find %s: %v", RecipientsCollection, err)
	}
	status, ok := recipients.Fields.GetByName("status").(*core.SelectField)
	if !ok || strings.Join(status.Values, ",") != strings.Join(RecipientStatuses, ",") {
		t.Fatalf("recipient status values must equal %v", RecipientStatuses)
	}
	reason, ok := recipients.Fields.GetByName("reason").(*core.SelectField)
	if !ok || strings.Join(reason.Values, ",") != strings.Join(Reasons, ",") {
		t.Fatalf("recipient reason values must equal %v", Reasons)
	}
	if recipients.GetIndex("idx_email_sendout_recipients_queue") == "" {
		t.Fatal("recipients must index the queue order")
	}
	if sendouts.ListRule != nil || recipients.ListRule != nil || sendouts.ViewRule != nil || recipients.ViewRule != nil {
		t.Fatal("sendout collections must stay superuser-only")
	}
	if got := RatePerMinute(app); got != DefaultRatePerMinute {
		t.Fatalf("seeded rate = %d, want %d", got, DefaultRatePerMinute)
	}
}

func TestNormalizeRateAndInterval(t *testing.T) {
	for rate, want := range map[int]int{0: 30, -5: 30, 1: 1, 45: 45, 600: 600, 601: 600} {
		if got := NormalizeRate(rate); got != want {
			t.Errorf("NormalizeRate(%d) = %d, want %d", rate, got, want)
		}
	}
	if got := Interval(30); got.Seconds() != 2 {
		t.Errorf("Interval(30) = %v, want 2s", got)
	}
	if got := Interval(600); got.Milliseconds() != 100 {
		t.Errorf("Interval(600) = %v, want 100ms", got)
	}
}

func TestEligibleUsers(t *testing.T) {
	app := newTestApp(t)
	newTestUser(t, app, "ops@example.com", "active", true)
	newTestUser(t, app, "dev@example.com", "active", true)
	newTestUser(t, app, "off@example.com", "disabled", true)
	newTestUser(t, app, "new@example.com", "active", false)

	count, err := CountEligible(app)
	if err != nil || count != 2 {
		t.Fatalf("CountEligible() = %d, %v; want 2", count, err)
	}
	found, err := FindEligible(app, "ops", 20)
	if err != nil || len(found) != 1 || found[0].Email() != "ops@example.com" {
		t.Fatalf("FindEligible(ops) = %v, %v; want only ops@example.com", found, err)
	}
	all, err := FindEligible(app, "", 0)
	if err != nil || len(all) != 2 || all[0].Email() != "dev@example.com" {
		t.Fatalf("FindEligible(\"\") = %v, %v; want dev, ops in email order", all, err)
	}
}
