package automaticemails

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/subscriptions"
)

func TestValidateTemplateRejectsUnknownPlaceholder(t *testing.T) {
	err := ValidateTemplate(Template{Subject: "Hi {{name}}", HTML: "<p>x</p>", Text: "x"})
	if !errors.Is(err, ErrUnknownPlaceholder) {
		t.Fatalf("err = %v, want ErrUnknownPlaceholder", err)
	}
}

func TestValidateTemplateRejectsBrokenHTMLPlaceholder(t *testing.T) {
	err := ValidateTemplate(Template{Subject: "s", HTML: "<p>{{<strong>remaining</strong>}}</p>", Text: "{{remaining}}"})
	if !errors.Is(err, ErrBrokenPlaceholder) {
		t.Fatalf("err = %v, want ErrBrokenPlaceholder", err)
	}
}

func TestValidateTemplateRejectsEncodedPlaceholder(t *testing.T) {
	err := ValidateTemplate(Template{Subject: "s", HTML: `<a href="%7B%7Bpanel_url%7D%7D">x</a>`, Text: "x"})
	if !errors.Is(err, ErrEncodedPlaceholder) {
		t.Fatalf("err = %v, want ErrEncodedPlaceholder", err)
	}
}

func TestValidateTemplateAcceptsMatchingPlaceholders(t *testing.T) {
	err := ValidateTemplate(Template{Subject: "{{email}}", HTML: "<p>{{remaining}}</p>", Text: "{{remaining}}"})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestRenderEscapesHTMLValues(t *testing.T) {
	tmpl := Template{Subject: "{{email}}", HTML: "<p>{{email}}</p>", Text: "{{email}}"}
	got := Render(tmpl, map[string]string{"email": "a<b>&c"})
	for _, want := range []string{"&lt;", "&gt;", "&amp;"} {
		if !strings.Contains(got.HTML, want) {
			t.Fatalf("html %q missing %q", got.HTML, want)
		}
	}
	if got.Text != "a<b>&c" || got.Subject != "a<b>&c" {
		t.Fatalf("text %q subject %q should be raw", got.Text, got.Subject)
	}
}

func TestVariables(t *testing.T) {
	subscriptionType := core.NewRecord(core.NewBaseCollection("subscription_types"))
	subscriptionType.Set("reset_days", 30)
	grant := core.NewRecord(core.NewBaseCollection("user_subscriptions"))
	grant.Set("starts_at", time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC))
	grant.Set("ends_at", time.Date(2027, 10, 1, 8, 0, 0, 0, time.UTC))
	state := &subscriptions.State{
		Grant: grant, Type: subscriptionType, Window: 0,
		Used: subscriptions.Usage{Tx: 1, Rx: 2}, Allowance: 100 << 30, Remaining: -1,
	}

	vars := Variables("a@example.com", state, "https://panel.example")

	if vars["remaining"] != "0 B" || vars["remaining_percent"] != "0%" {
		t.Fatalf("remaining %q percent %q", vars["remaining"], vars["remaining_percent"])
	}
	if vars["window_ends_at"] != "2026-11-04 08:00 UTC" {
		t.Fatalf("window_ends_at = %q", vars["window_ends_at"])
	}
	if vars["window_starts_at"] != "2026-10-05 08:00 UTC" || vars["panel_url"] != "https://panel.example" || len(vars) != 9 {
		t.Fatalf("vars = %v", vars)
	}
}
