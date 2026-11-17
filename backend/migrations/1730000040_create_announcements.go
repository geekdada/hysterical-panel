package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// uploadMaxBytes caps one stored file. PocketBase treats 0 as its 5 MB default.
const uploadMaxBytes = 1 << 30

// Announcements and the generic file store behind their editor (ADR-0015).
// Both keep nil API rules, so only the custom /api/panel routes can write them.
// Files are not protected, so /api/files serves them without a token.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if err := createFiles(app, users.Id); err != nil {
			return err
		}
		return createAnnouncements(app, users.Id)
	}, func(app core.App) error {
		for _, name := range []string{"announcements", "files"} {
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

func createFiles(app core.App, usersID string) error {
	files := core.NewBaseCollection("files")
	files.Fields.Add(&core.FileField{Name: "file", Required: true, MaxSelect: 1, MaxSize: uploadMaxBytes})
	files.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 255})
	files.Fields.Add(&core.TextField{Name: "mime", Max: 255})
	files.Fields.Add(&core.NumberField{Name: "size", OnlyInt: true})
	files.Fields.Add(&core.RelationField{Name: "uploaded_by", CollectionId: usersID, MaxSelect: 1})
	files.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	return app.Save(files)
}

func createAnnouncements(app core.App, usersID string) error {
	announcements := core.NewBaseCollection("announcements")
	announcements.Fields.Add(&core.TextField{Name: "title", Required: true, Max: 200})
	announcements.Fields.Add(&core.TextField{Name: "html", Required: true, Max: 1_000_000})
	announcements.Fields.Add(&core.JSONField{Name: "content", MaxSize: 1 << 20})
	announcements.Fields.Add(&core.BoolField{Name: "pinned"})
	announcements.Fields.Add(&core.BoolField{Name: "hidden"})
	announcements.Fields.Add(&core.RelationField{Name: "created_by", CollectionId: usersID, MaxSelect: 1})
	announcements.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	announcements.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	announcements.AddIndex("idx_announcements_created", false, "created", "")
	return app.Save(announcements)
}
