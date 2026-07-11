package migrations

import (
	"errors"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add the language selected by each Monitor and snapshot it onto Alerts so a
// firing/recovery pair cannot switch languages after the Monitor is edited.
func init() {
	m.Register(func(app core.App) error {
		for _, target := range []struct {
			collection string
			field      string
		}{
			{collection: "monitors", field: "notification_language"},
			{collection: "alerts", field: "notification_language_snapshot"},
		} {
			collection, err := app.FindCollectionByNameOrId(target.collection)
			if err != nil {
				return err
			}
			if collection.Fields.GetByName(target.field) == nil {
				collection.Fields.Add(&core.SelectField{
					Name:      target.field,
					Required:  false,
					MaxSelect: 1,
					Values:    []string{"en", "zh-cn"},
				})
				if err := app.Save(collection); err != nil {
					return err
				}
			}

			records, err := app.FindRecordsByFilter(target.collection, target.field+" = ''", "", 0, 0)
			if err != nil {
				return err
			}
			for _, record := range records {
				record.Set(target.field, "en")
				if err := app.Save(record); err != nil {
					return err
				}
			}

			field, ok := collection.Fields.GetByName(target.field).(*core.SelectField)
			if !ok {
				return errors.New(target.collection + "." + target.field + " field not found")
			}
			field.Required = true
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		for _, target := range []struct {
			collection string
			field      string
		}{
			{collection: "alerts", field: "notification_language_snapshot"},
			{collection: "monitors", field: "notification_language"},
		} {
			collection, err := app.FindCollectionByNameOrId(target.collection)
			if err != nil {
				return err
			}
			collection.Fields.RemoveByName(target.field)
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		return nil
	})
}
