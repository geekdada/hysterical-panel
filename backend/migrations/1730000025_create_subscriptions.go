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
		users.Fields.RemoveByName("quota_bytes")
		users.Fields.Add(&core.BoolField{Name: "subscription_required"})
		if err := app.Save(users); err != nil {
			return err
		}

		types := core.NewBaseCollection("subscription_types")
		types.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 128})
		// PocketBase NumberField converts through float64 and cannot preserve int64 bytes.
		types.Fields.Add(&core.TextField{Name: "allowance_bytes", Required: true})
		types.Fields.Add(&core.NumberField{Name: "reset_days", Required: true, OnlyInt: true})
		types.Fields.Add(&core.BoolField{Name: "hidden"})
		types.Fields.Add(&core.BoolField{Name: "ever_assigned", Hidden: true})
		types.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		types.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		if err := app.Save(types); err != nil {
			return err
		}

		grants := core.NewBaseCollection("user_subscriptions")
		grants.Fields.Add(&core.RelationField{Name: "user", Required: true, MaxSelect: 1, CollectionId: users.Id, CascadeDelete: true})
		grants.Fields.Add(&core.RelationField{Name: "subscription_type", Required: true, MaxSelect: 1, CollectionId: types.Id})
		grants.Fields.Add(&core.DateField{Name: "starts_at", Required: true})
		grants.Fields.Add(&core.DateField{Name: "ends_at", Required: true})
		grants.Fields.Add(&core.DateField{Name: "terminated_at"})
		grants.Fields.Add(&core.NumberField{Name: "window_index", OnlyInt: true})
		grants.Fields.Add(&core.TextField{Name: "used_bytes"})
		grants.Fields.Add(&core.TextField{Name: "extra_bytes"})
		grants.Fields.Add(&core.BoolField{Name: "expiry_processed"})
		grants.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		grants.AddIndex("idx_user_subscriptions_user_start", false, "user, starts_at", "")
		grants.AddIndex("idx_user_subscriptions_type", false, "subscription_type", "")
		grants.AddIndex("idx_user_subscriptions_expiry", false, "expiry_processed, terminated_at, ends_at", "")
		return app.Save(grants)
	}, func(app core.App) error {
		for _, name := range []string{"user_subscriptions", "subscription_types"} {
			coll, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err := app.Delete(coll); err != nil {
				return err
			}
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.Fields.RemoveByName("subscription_required")
		users.Fields.Add(&core.NumberField{Name: "quota_bytes", OnlyInt: true})
		return app.Save(users)
	})
}
