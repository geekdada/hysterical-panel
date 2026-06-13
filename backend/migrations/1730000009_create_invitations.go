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

		invitations := core.NewBaseCollection("invitations")
		// Generic, reusable invite code. The code is shareable to any email;
		// `email` is only recorded for sending/auditing and does not bind the code.
		invitations.Fields.Add(&core.TextField{Name: "code", Required: true, Max: 128})
		invitations.Fields.Add(&core.TextField{Name: "email", Max: 255})
		// max_uses: 0 (or empty) means unlimited.
		invitations.Fields.Add(&core.NumberField{Name: "max_uses", OnlyInt: true})
		invitations.Fields.Add(&core.NumberField{Name: "used_count", OnlyInt: true})
		// expires_at: empty means never expires (stored UTC).
		invitations.Fields.Add(&core.DateField{Name: "expires_at"})
		invitations.Fields.Add(&core.BoolField{Name: "revoked"})
		invitations.Fields.Add(&core.TextField{Name: "note", Max: 255})
		invitations.Fields.Add(&core.RelationField{
			Name:         "created_by",
			MaxSelect:    1,
			CollectionId: users.Id,
		})
		invitations.Fields.Add(&core.DateField{Name: "last_used_at"})
		invitations.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		invitations.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		invitations.AddIndex("idx_invitations_code", true, "code", "")
		return app.Save(invitations)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("invitations")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
