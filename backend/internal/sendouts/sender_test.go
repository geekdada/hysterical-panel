package sendouts

import "testing"

func TestSenderUsesApplicationName(t *testing.T) {
	app := newTestApp(t)
	meta := &app.Settings().Meta
	meta.AppName = "TEEHEE Network"
	meta.SenderName = "Hysterical Panel"
	meta.SenderAddress = "panel@example.com"

	got := Sender(app)
	if got.Name != "TEEHEE Network" || got.Address != "panel@example.com" {
		t.Fatalf("Sender() = %+v, want application name with the configured address", got)
	}
}

func TestSenderFallsBackToSenderName(t *testing.T) {
	app := newTestApp(t)
	meta := &app.Settings().Meta
	meta.AppName = ""
	meta.SenderName = "Support"

	if got := Sender(app).Name; got != "Support" {
		t.Fatalf("Sender().Name = %q, want the PocketBase sender name", got)
	}
}
