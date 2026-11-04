package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

var (
	automaticEmailKinds    = []string{"low_allowance", "allowance_exhausted", "allowance_restored"}
	manualSendoutLanguages = []string{"en", "zh-cn"}
	allSendoutLanguages    = []string{"en", "zh-cn", "bilingual"}
)

const defaultLowAllowanceThresholdPercent = 10

// Automatic Emails (ADR-0014): one editable template per Kind, one state row
// per User, and a source marker on Email Sendouts so the history can tell
// panel-sent mail from administrator-sent mail. All new collections keep nil
// API rules, so only the custom /api/panel routes can reach them.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if err := createAutomaticTemplates(app); err != nil {
			return err
		}
		if err := createAutomaticStates(app, users.Id); err != nil {
			return err
		}
		return addSendoutSource(app)
	}, func(app core.App) error {
		if err := removeSendoutSource(app); err != nil {
			return err
		}
		for _, name := range []string{"automatic_email_states", "automatic_email_templates"} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		return nil
	})
}

func createAutomaticTemplates(app core.App) error {
	templates := core.NewBaseCollection("automatic_email_templates")
	templates.Fields.Add(&core.SelectField{Name: "kind", Required: true, MaxSelect: 1, Values: automaticEmailKinds})
	templates.Fields.Add(&core.BoolField{Name: "enabled"})
	templates.Fields.Add(&core.NumberField{Name: "threshold_percent", OnlyInt: true})
	templates.Fields.Add(&core.TextField{Name: "subject", Max: 200})
	templates.Fields.Add(&core.TextField{Name: "html", Max: 1_000_000})
	templates.Fields.Add(&core.TextField{Name: "text", Max: 256_000})
	templates.Fields.Add(&core.JSONField{Name: "content", MaxSize: 1 << 20})
	templates.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	templates.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	templates.AddIndex("idx_automatic_email_templates_kind", true, "kind", "")
	if err := app.Save(templates); err != nil {
		return err
	}
	for _, kind := range automaticEmailKinds {
		rec := core.NewRecord(templates)
		rec.Set("kind", kind)
		if kind == "low_allowance" {
			rec.Set("threshold_percent", defaultLowAllowanceThresholdPercent)
		}
		if err := app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}

func createAutomaticStates(app core.App, usersID string) error {
	states := core.NewBaseCollection("automatic_email_states")
	states.Fields.Add(&core.RelationField{Name: "user", Required: true, MaxSelect: 1, CollectionId: usersID, CascadeDelete: true})
	states.Fields.Add(&core.BoolField{Name: "low"})
	states.Fields.Add(&core.BoolField{Name: "exhausted"})
	states.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	states.AddIndex("idx_automatic_email_states_user", true, "user", "")
	return app.Save(states)
}

// addSendoutSource adds the columns first, backfills, and only then marks
// source required, so rows created before this migration stay valid.
func addSendoutSource(app core.App) error {
	sendouts, err := app.FindCollectionByNameOrId("email_sendouts")
	if err != nil {
		return err
	}
	if err := setSelectValues(sendouts, "language", allSendoutLanguages); err != nil {
		return err
	}
	sendouts.Fields.Add(&core.SelectField{Name: "source", MaxSelect: 1, Values: []string{"manual", "automatic"}})
	sendouts.Fields.Add(&core.SelectField{Name: "automatic_kind", MaxSelect: 1, Values: automaticEmailKinds})
	if err := app.Save(sendouts); err != nil {
		return err
	}
	if _, err := app.DB().NewQuery("UPDATE email_sendouts SET source = 'manual'").Execute(); err != nil {
		return err
	}
	if err := setRequired(sendouts, []string{"source"}, true); err != nil {
		return err
	}
	return app.Save(sendouts)
}

func removeSendoutSource(app core.App) error {
	// Automatic Sendouts cannot exist without the fields and value being removed.
	if _, err := app.DB().NewQuery("DELETE FROM email_sendouts WHERE source = 'automatic' OR language = 'bilingual'").Execute(); err != nil {
		return err
	}
	sendouts, err := app.FindCollectionByNameOrId("email_sendouts")
	if err != nil {
		return err
	}
	sendouts.Fields.RemoveByName("source")
	sendouts.Fields.RemoveByName("automatic_kind")
	if err := setSelectValues(sendouts, "language", manualSendoutLanguages); err != nil {
		return err
	}
	return app.Save(sendouts)
}
