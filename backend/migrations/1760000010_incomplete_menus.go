package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Incomplete menus (« cartes incomplètes ») : restaurants.items_count (number
// of available menu items, maintained by the server) and the app_settings
// singleton holding min_menu_items, the threshold under which the public
// listings hide a restaurant (0 = disabled).

// MinMenuItemsDefault is the threshold seeded on installs with real data
// (the request was « masquer les restos avec moins de 10 plats »). Demo
// installs (fictional restaurants of ~9 items) start with the filter off.
const MinMenuItemsDefault = 10

func init() {
	m.Register(upIncompleteMenus, downIncompleteMenus)
}

func upIncompleteMenus(app core.App) error {
	rest, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		return err
	}
	if rest.Fields.GetByName("items_count") == nil {
		rest.Fields.Add(intField("items_count", fptr(0), false))
		if err := app.Save(rest); err != nil {
			return err
		}
	}
	if _, err := app.DB().NewQuery(
		"UPDATE {{restaurants}} SET [[items_count]] = (SELECT COUNT(*) FROM {{menu_items}} m WHERE m.[[restaurant]] = {{restaurants}}.[[id]] AND m.[[available]] = TRUE)",
	).Execute(); err != nil {
		return err
	}

	if _, err := app.FindCollectionByNameOrId("app_settings"); err == nil {
		return nil
	}
	col := core.NewBaseCollection("app_settings")
	// a single row read / written by admins; created by this migration only
	col.ListRule = ptr(isAdminRule)
	col.ViewRule = ptr(isAdminRule)
	col.UpdateRule = ptr(isAdminRule)
	col.Fields.Add(&core.NumberField{Name: "min_menu_items", OnlyInt: true, Min: fptr(0), Max: fptr(100)})
	autodates(col)
	if err := app.Save(col); err != nil {
		return err
	}
	row := core.NewRecord(col)
	threshold := 0
	if HasRealData() {
		threshold = MinMenuItemsDefault
	}
	row.Set("min_menu_items", threshold)
	return app.Save(row)
}

func downIncompleteMenus(app core.App) error {
	if col, err := app.FindCollectionByNameOrId("app_settings"); err == nil {
		if err := app.Delete(col); err != nil {
			return err
		}
	}
	rest, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		return err
	}
	if f := rest.Fields.GetByName("items_count"); f != nil {
		rest.Fields.RemoveById(f.GetId())
		return app.Save(rest)
	}
	return nil
}
