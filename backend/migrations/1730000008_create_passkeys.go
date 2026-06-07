package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		credentials := core.NewBaseCollection("passkey_credentials")
		credentials.Fields.Add(&core.RelationField{
			Name:          "user",
			Required:      true,
			MaxSelect:     1,
			CascadeDelete: true,
			CollectionId:  users.Id,
		})
		credentials.Fields.Add(&core.TextField{Name: "credential_id", Required: true, Max: 1024})
		credentials.Fields.Add(&core.TextField{Name: "user_handle", Required: true, Max: 128})
		credentials.Fields.Add(&core.TextField{Name: "rp_id", Required: true, Max: 255})
		credentials.Fields.Add(&core.TextField{Name: "name", Max: 128})
		credentials.Fields.Add(&core.JSONField{Name: "credential", Required: true, Hidden: true})
		credentials.Fields.Add(&core.JSONField{Name: "transports"})
		credentials.Fields.Add(&core.NumberField{Name: "sign_count", OnlyInt: true})
		credentials.Fields.Add(&core.BoolField{Name: "backup_eligible"})
		credentials.Fields.Add(&core.BoolField{Name: "backup_state"})
		credentials.Fields.Add(&core.BoolField{Name: "clone_warning"})
		credentials.Fields.Add(&core.DateField{Name: "last_used_at"})
		credentials.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		credentials.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		credentials.AddIndex("idx_passkey_credentials_credential_id", true, "credential_id", "")
		credentials.AddIndex("idx_passkey_credentials_user", false, "user", "")
		if err := app.Save(credentials); err != nil {
			return err
		}

		sessions := core.NewBaseCollection("passkey_sessions")
		sessions.Fields.Add(&core.TextField{Name: "challenge_id", Required: true, Max: 128})
		sessions.Fields.Add(&core.SelectField{
			Name:      "kind",
			Required:  true,
			MaxSelect: 1,
			Values:    []string{"login", "registration"},
		})
		sessions.Fields.Add(&core.RelationField{
			Name:          "user",
			MaxSelect:     1,
			CascadeDelete: true,
			CollectionId:  users.Id,
		})
		sessions.Fields.Add(&core.JSONField{Name: "session_data", Required: true, Hidden: true})
		sessions.Fields.Add(&core.DateField{Name: "expires_at", Required: true})
		sessions.Fields.Add(&core.DateField{Name: "consumed_at"})
		sessions.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		sessions.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		sessions.AddIndex("idx_passkey_sessions_challenge_id", true, "challenge_id", "")
		sessions.AddIndex("idx_passkey_sessions_expires_at", false, "expires_at", "")
		return app.Save(sessions)
	}, func(app core.App) error {
		for _, name := range []string{"passkey_sessions", "passkey_credentials"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}
