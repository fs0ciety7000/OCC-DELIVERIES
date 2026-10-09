package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// newPartyProviders are the delivery platforms added after the initial schema.
var newPartyProviders = []string{"deliveroo", "weloveat"}

// Adds Deliveroo and weloveat to parties.provider (docs/ARCHITECTURE.md §3).
// Existing values are kept as they are.
func init() {
	m.Register(func(app core.App) error {
		return editPartyProviders(app, func(values []string) []string {
			for _, v := range newPartyProviders {
				if !slices.Contains(values, v) {
					values = append(values, v)
				}
			}
			return values
		})
	}, func(app core.App) error {
		return editPartyProviders(app, func(values []string) []string {
			return slices.DeleteFunc(values, func(v string) bool { return slices.Contains(newPartyProviders, v) })
		})
	})
}

func editPartyProviders(app core.App, edit func([]string) []string) error {
	col, err := app.FindCollectionByNameOrId("parties")
	if err != nil {
		return err
	}
	f, ok := col.Fields.GetByName("provider").(*core.SelectField)
	if !ok {
		return nil
	}
	f.Values = edit(slices.Clone(f.Values))
	return app.Save(col)
}
