package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Notification Channels hold one encrypted Shoutrrr service URL per outbound
// destination. The URL is only ever returned through the custom panel API after
// a scoped WebAuthn assertion; it must never be exposed by PocketBase's generic
// collection API.
func init() {
	m.Register(func(app core.App) error {
		channels := core.NewBaseCollection("notification_channels")
		channels.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 128})
		channels.Fields.Add(&core.TextField{Name: "name_key", Required: true, Max: 128, Hidden: true})
		channels.Fields.Add(&core.SelectField{
			Name:      "service",
			Required:  true,
			MaxSelect: 1,
			Values: []string{
				"generic", "bark", "discord", "gotify", "googlechat", "ifttt", "join", "lark",
				"mattermost", "matrix", "mqtt", "ntfy", "opsgenie", "pushbullet", "pushover",
				"rocketchat", "signal", "slack", "teams", "telegram", "twilio", "wecom", "zulip",
			},
		})
		channels.Fields.Add(&core.TextField{Name: "url_encrypted", Required: true, Max: 8192, Hidden: true})
		channels.Fields.Add(&core.BoolField{Name: "enabled"})
		channels.Fields.Add(&core.SelectField{
			Name:      "last_test_status",
			Required:  true,
			MaxSelect: 1,
			Values:    []string{"never", "succeeded", "failed"},
		})
		channels.Fields.Add(&core.DateField{Name: "last_tested_at"})
		channels.Fields.Add(&core.SelectField{
			Name:      "last_test_error",
			MaxSelect: 1,
			Values:    []string{"timed_out", "delivery_failed"},
		})
		channels.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		channels.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		channels.AddIndex("idx_notification_channels_name_key", true, "name_key", "")
		channels.AddIndex("idx_notification_channels_service", false, "service", "")
		if err := app.Save(channels); err != nil {
			return err
		}

		sessions, err := app.FindCollectionByNameOrId("passkey_sessions")
		if err != nil {
			return err
		}
		kind, ok := sessions.Fields.GetByName("kind").(*core.SelectField)
		if !ok {
			return fmt.Errorf("passkey_sessions.kind is not a select field")
		}
		kind.Values = []string{"login", "registration", "sensitive_field_reveal"}
		if sessions.Fields.GetByName("scope") == nil {
			sessions.Fields.Add(&core.TextField{Name: "scope", Max: 128, Hidden: true})
		}
		return app.Save(sessions)
	}, func(app core.App) error {
		channels, err := app.FindCollectionByNameOrId("notification_channels")
		if err != nil {
			return err
		}
		if err := app.Delete(channels); err != nil {
			return err
		}

		sessions, err := app.FindCollectionByNameOrId("passkey_sessions")
		if err != nil {
			return err
		}
		kind, ok := sessions.Fields.GetByName("kind").(*core.SelectField)
		if !ok {
			return fmt.Errorf("passkey_sessions.kind is not a select field")
		}
		kind.Values = []string{"login", "registration"}
		sessions.Fields.RemoveByName("scope")
		return app.Save(sessions)
	})
}
