package catalog

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// ItemsCountField is restaurants.items_count (migration 1760000010): the
// number of AVAILABLE menu items, maintained by the server.
const ItemsCountField = "items_count"

// CountAvailableItems returns the number of available menu items of a restaurant.
func CountAvailableItems(app core.App, restaurantID string) (int, error) {
	n, err := app.CountRecords(MenuItems, dbx.HashExp{"restaurant": restaurantID, "available": true})
	return int(n), err
}

// RefreshItemsCount recomputes restaurants.items_count for the given
// restaurants with a plain UPDATE (no hook, no `updated` change). It is a
// no-op while the field does not exist yet (older migrations importing).
func RefreshItemsCount(app core.App, restaurantIDs ...string) error {
	col, err := app.FindCollectionByNameOrId(Restaurants)
	if err != nil {
		return err
	}
	if col.Fields.GetByName(ItemsCountField) == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, id := range restaurantIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		_, err := app.DB().NewQuery(
			"UPDATE {{restaurants}} SET [[items_count]] = (SELECT COUNT(*) FROM {{menu_items}} WHERE [[restaurant]] = {:id} AND [[available]] = TRUE) WHERE [[id]] = {:id}",
		).Bind(dbx.Params{"id": id}).Execute()
		if err != nil {
			return err
		}
	}
	return nil
}
