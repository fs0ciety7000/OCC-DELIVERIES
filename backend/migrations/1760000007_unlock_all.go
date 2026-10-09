package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Locking is now explicit only (an admin edit no longer locks a record):
// clear the locks that the former automatic locking may have set so that the
// synchronisation follows every restaurant and menu item again.

func init() {
	m.Register(func(app core.App) error {
		for _, table := range []string{"restaurants", "menu_items"} {
			if _, err := app.DB().Update(table, dbx.Params{"locked": false}, dbx.HashExp{"locked": true}).Execute(); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error { return nil })
}
