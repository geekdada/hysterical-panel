package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Email Sendouts: one row per administrator-initiated transactional email and
// one row per addressed User. The browser composes html/text; the backend
// stores and sends them unchanged (ADR 0008). Both collections keep nil API
// rules, so only the custom /api/panel routes can reach them.
func init() {
	m.Register(func(app core.App) error {
		if err := addSendoutRateSetting(app); err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		sendouts := core.NewBaseCollection("email_sendouts")
		sendouts.Fields.Add(&core.TextField{Name: "subject", Required: true, Max: 200})
		sendouts.Fields.Add(&core.SelectField{Name: "language", Required: true, MaxSelect: 1, Values: []string{"en", "zh-cn"}})
		sendouts.Fields.Add(&core.SelectField{Name: "audience", Required: true, MaxSelect: 1, Values: []string{"single", "all"}})
		sendouts.Fields.Add(&core.TextField{Name: "html", Required: true, Max: 1_000_000})
		sendouts.Fields.Add(&core.TextField{Name: "text", Required: true, Max: 256_000})
		sendouts.Fields.Add(&core.JSONField{Name: "content", MaxSize: 1 << 20})
		sendouts.Fields.Add(&core.RelationField{Name: "created_by", MaxSelect: 1, CollectionId: users.Id})
		sendouts.Fields.Add(&core.TextField{Name: "created_by_email", Max: 255})
		sendouts.Fields.Add(&core.DateField{Name: "cancelled_at"})
		sendouts.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		sendouts.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		sendouts.AddIndex("idx_email_sendouts_created", false, "created", "")
		if err := app.Save(sendouts); err != nil {
			return err
		}

		recipients := core.NewBaseCollection("email_sendout_recipients")
		recipients.Fields.Add(&core.RelationField{Name: "sendout", Required: true, MaxSelect: 1, CollectionId: sendouts.Id, CascadeDelete: true})
		recipients.Fields.Add(&core.RelationField{Name: "user", MaxSelect: 1, CollectionId: users.Id})
		recipients.Fields.Add(&core.TextField{Name: "email", Required: true, Max: 255})
		recipients.Fields.Add(&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"pending", "sending", "sent", "failed", "skipped", "cancelled"}})
		recipients.Fields.Add(&core.SelectField{Name: "reason", MaxSelect: 1, Values: []string{"delivery_failed", "smtp_disabled", "interrupted", "user_ineligible", "user_deleted"}})
		recipients.Fields.Add(&core.NumberField{Name: "attempts", OnlyInt: true})
		recipients.Fields.Add(&core.DateField{Name: "queued_at", Required: true})
		recipients.Fields.Add(&core.DateField{Name: "last_attempt_at"})
		recipients.Fields.Add(&core.DateField{Name: "sent_at"})
		recipients.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		recipients.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		recipients.AddIndex("idx_email_sendout_recipients_queue", false, "status, queued_at", "")
		recipients.AddIndex("idx_email_sendout_recipients_sendout", false, "sendout, status", "")
		return app.Save(recipients)
	}, func(app core.App) error {
		for _, name := range []string{"email_sendout_recipients", "email_sendouts"} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.RemoveByName("email_sendout_rate_per_minute")
		return app.Save(settings)
	})
}

// addSendoutRateSetting adds the per-minute delivery rate and seeds the
// existing singleton with the default of 30.
func addSendoutRateSetting(app core.App) error {
	settings, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return err
	}
	settings.Fields.Add(&core.NumberField{Name: "email_sendout_rate_per_minute", OnlyInt: true})
	if err := app.Save(settings); err != nil {
		return err
	}
	records, err := app.FindRecordsByFilter("app_settings", "", "", 0, 0)
	if err != nil {
		return err
	}
	for _, rec := range records {
		rec.Set("email_sendout_rate_per_minute", 30)
		if err := app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}
