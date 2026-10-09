package search

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Tables (migration 1760000017). The FTS5 tables hold FOLDED text only; the
// *_docs tables map their integer rowid to the PocketBase record ids (an
// UNINDEXED id column would be scanned on every delete). Visibility
// (active restaurant, available item, incomplete menus) is never stored:
// it is joined at query time, so it follows the catalogue without reindexing.
const (
	TableRestaurants     = "occ_search_restaurants"
	TableRestaurantDocs  = "occ_search_restaurant_docs"
	TableItems           = "occ_search_items"
	TableItemDocs        = "occ_search_item_docs"
	TableVocabRestaurant = "occ_search_vocab_restaurants"
	TableVocabItems      = "occ_search_vocab_items"
)

// ftsOptions: unicode61 also removes diacritics (defence in depth: the text
// is already folded in Go); prefix indexes speed up « ram* ».
const ftsOptions = "tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3'"

// SchemaSQL creates the search tables (idempotent).
var SchemaSQL = []string{
	"CREATE TABLE IF NOT EXISTS " + TableRestaurantDocs + " (docid INTEGER PRIMARY KEY AUTOINCREMENT, restaurant TEXT NOT NULL UNIQUE)",
	"CREATE TABLE IF NOT EXISTS " + TableItemDocs + " (docid INTEGER PRIMARY KEY AUTOINCREMENT, item TEXT NOT NULL UNIQUE, restaurant TEXT NOT NULL)",
	"CREATE INDEX IF NOT EXISTS idx_" + TableItemDocs + "_restaurant ON " + TableItemDocs + " (restaurant)",
	"CREATE VIRTUAL TABLE IF NOT EXISTS " + TableRestaurants + " USING fts5(name, cuisines, address, " + ftsOptions + ")",
	"CREATE VIRTUAL TABLE IF NOT EXISTS " + TableItems + " USING fts5(name, description, category, restaurant, " + ftsOptions + ")",
	"CREATE VIRTUAL TABLE IF NOT EXISTS " + TableVocabRestaurant + " USING fts5vocab(" + TableRestaurants + ", 'row')",
	"CREATE VIRTUAL TABLE IF NOT EXISTS " + TableVocabItems + " USING fts5vocab(" + TableItems + ", 'row')",
}

// DropSQL removes the search tables.
var DropSQL = []string{
	"DROP TABLE IF EXISTS " + TableVocabItems,
	"DROP TABLE IF EXISTS " + TableVocabRestaurant,
	"DROP TABLE IF EXISTS " + TableItems,
	"DROP TABLE IF EXISTS " + TableRestaurants,
	"DROP TABLE IF EXISTS " + TableItemDocs,
	"DROP TABLE IF EXISTS " + TableRestaurantDocs,
}

// CreateSchema creates the tables then indexes the whole catalogue.
func CreateSchema(app core.App) error {
	return app.RunInTransaction(func(tx core.App) error {
		for _, q := range SchemaSQL {
			if _, err := tx.DB().NewQuery(q).Execute(); err != nil {
				return err
			}
		}
		return rebuild(tx)
	})
}

// DropSchema removes the search tables.
func DropSchema(app core.App) error {
	for _, q := range DropSQL {
		if _, err := app.DB().NewQuery(q).Execute(); err != nil {
			return err
		}
	}
	return nil
}

// generation changes on every index write (vocabulary cache invalidation).
var generation atomic.Int64

// Ready reports whether the search tables exist (false before migration
// 1760000017: every index function is then a no-op).
func Ready(app core.App) bool {
	var n int
	err := app.DB().NewQuery("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = {:t}").
		Bind(dbx.Params{"t": TableItems}).Row(&n)
	return err == nil && n == 1
}

// Rebuild re-indexes the whole catalogue (in a transaction).
func Rebuild(app core.App) error {
	if !Ready(app) {
		return nil
	}
	return app.RunInTransaction(rebuild)
}

func rebuild(tx core.App) error {
	for _, t := range []string{TableItems, TableItemDocs, TableRestaurants, TableRestaurantDocs} {
		if _, err := tx.DB().NewQuery("DELETE FROM " + t).Execute(); err != nil {
			return err
		}
	}
	if err := indexRestaurants(tx, ""); err != nil {
		return err
	}
	if err := indexItems(tx, "", ""); err != nil {
		return err
	}
	generation.Add(1)
	return nil
}

// Reindex re-indexes restaurants (their row and every menu item). Called by
// catalog.Import and the synchronisation after writing a restaurant (inside
// their transaction); unknown ids are removed from the index.
func Reindex(app core.App, restaurantIDs ...string) error {
	if len(restaurantIDs) == 0 || !Ready(app) {
		return nil
	}
	return app.RunInTransaction(func(tx core.App) error {
		seen := map[string]bool{}
		for _, id := range restaurantIDs {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			if err := removeRestaurant(tx, id); err != nil {
				return err
			}
			if err := indexRestaurants(tx, id); err != nil {
				return err
			}
			if err := indexItems(tx, id, ""); err != nil {
				return err
			}
		}
		generation.Add(1)
		return nil
	})
}

// ReindexItem re-indexes one menu item (removed when it no longer exists).
func ReindexItem(app core.App, itemID string) error {
	if itemID == "" || !Ready(app) {
		return nil
	}
	return app.RunInTransaction(func(tx core.App) error {
		if err := removeItem(tx, itemID); err != nil {
			return err
		}
		if err := indexItems(tx, "", itemID); err != nil {
			return err
		}
		generation.Add(1)
		return nil
	})
}

// RemoveItem drops one menu item from the index.
func RemoveItem(app core.App, itemID string) error {
	if itemID == "" || !Ready(app) {
		return nil
	}
	generation.Add(1)
	return removeItem(app, itemID)
}

// RemoveRestaurant drops a restaurant and its items from the index.
func RemoveRestaurant(app core.App, restaurantID string) error {
	if restaurantID == "" || !Ready(app) {
		return nil
	}
	generation.Add(1)
	return app.RunInTransaction(func(tx core.App) error { return removeRestaurant(tx, restaurantID) })
}

// ReindexRestaurantRow re-indexes the restaurant row only (name unchanged).
func ReindexRestaurantRow(app core.App, restaurantID string) error {
	if restaurantID == "" || !Ready(app) {
		return nil
	}
	return app.RunInTransaction(func(tx core.App) error {
		if err := removeRestaurantRow(tx, restaurantID); err != nil {
			return err
		}
		generation.Add(1)
		return indexRestaurants(tx, restaurantID)
	})
}

func removeRestaurantRow(tx core.App, id string) error {
	if _, err := tx.DB().NewQuery("DELETE FROM " + TableRestaurants + " WHERE rowid IN (SELECT docid FROM " + TableRestaurantDocs + " WHERE restaurant = {:id})").
		Bind(dbx.Params{"id": id}).Execute(); err != nil {
		return err
	}
	_, err := tx.DB().NewQuery("DELETE FROM " + TableRestaurantDocs + " WHERE restaurant = {:id}").Bind(dbx.Params{"id": id}).Execute()
	return err
}

func removeRestaurant(tx core.App, id string) error {
	if err := removeRestaurantRow(tx, id); err != nil {
		return err
	}
	if _, err := tx.DB().NewQuery("DELETE FROM " + TableItems + " WHERE rowid IN (SELECT docid FROM " + TableItemDocs + " WHERE restaurant = {:id})").
		Bind(dbx.Params{"id": id}).Execute(); err != nil {
		return err
	}
	_, err := tx.DB().NewQuery("DELETE FROM " + TableItemDocs + " WHERE restaurant = {:id}").Bind(dbx.Params{"id": id}).Execute()
	return err
}

func removeItem(tx core.App, id string) error {
	if _, err := tx.DB().NewQuery("DELETE FROM " + TableItems + " WHERE rowid IN (SELECT docid FROM " + TableItemDocs + " WHERE item = {:id})").
		Bind(dbx.Params{"id": id}).Execute(); err != nil {
		return err
	}
	_, err := tx.DB().NewQuery("DELETE FROM " + TableItemDocs + " WHERE item = {:id}").Bind(dbx.Params{"id": id}).Execute()
	return err
}

type restaurantRow struct {
	ID       string `db:"id"`
	Name     string `db:"name"`
	Cuisines string `db:"cuisines"`
	Address  string `db:"address"`
}

// indexRestaurants indexes one restaurant (id) or all of them ("").
func indexRestaurants(tx core.App, id string) error {
	q := tx.DB().Select("id", "name", "cuisines", "address").From("restaurants")
	if id != "" {
		q.Where(dbx.HashExp{"id": id})
	}
	var rows []restaurantRow
	if err := q.All(&rows); err != nil {
		return err
	}
	for _, r := range rows {
		res, err := tx.DB().NewQuery("INSERT INTO " + TableRestaurantDocs + " (restaurant) VALUES ({:id})").Bind(dbx.Params{"id": r.ID}).Execute()
		if err != nil {
			return err
		}
		docid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.DB().NewQuery("INSERT INTO " + TableRestaurants + " (rowid, name, cuisines, address) VALUES ({:doc}, {:name}, {:cuisines}, {:address})").
			Bind(dbx.Params{"doc": docid, "name": Fold(r.Name), "cuisines": Fold(strings.Join(jsonList(r.Cuisines), " ")), "address": Fold(r.Address)}).
			Execute(); err != nil {
			return err
		}
	}
	return nil
}

type itemRow struct {
	ID          string `db:"id"`
	Restaurant  string `db:"restaurant"`
	Name        string `db:"name"`
	Description string `db:"description"`
	Category    string `db:"category"`
	RestName    string `db:"rname"`
}

// indexItems indexes the items of one restaurant, one item, or every item.
func indexItems(tx core.App, restaurantID, itemID string) error {
	q := tx.DB().NewQuery(`SELECT m.id AS id, m.restaurant AS restaurant, m.name AS name, m.description AS description,
		COALESCE(c.name, '') AS category, COALESCE(r.name, '') AS rname
		FROM menu_items m
		LEFT JOIN menu_categories c ON c.id = m.category
		LEFT JOIN restaurants r ON r.id = m.restaurant
		WHERE ({:rid} = '' OR m.restaurant = {:rid}) AND ({:iid} = '' OR m.id = {:iid})`).
		Bind(dbx.Params{"rid": restaurantID, "iid": itemID})
	var rows []itemRow
	if err := q.All(&rows); err != nil {
		return err
	}
	for _, r := range rows {
		res, err := tx.DB().NewQuery("INSERT INTO " + TableItemDocs + " (item, restaurant) VALUES ({:id}, {:rid})").
			Bind(dbx.Params{"id": r.ID, "rid": r.Restaurant}).Execute()
		if err != nil {
			return err
		}
		docid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.DB().NewQuery("INSERT INTO " + TableItems + " (rowid, name, description, category, restaurant) VALUES ({:doc}, {:name}, {:desc}, {:cat}, {:rname})").
			Bind(dbx.Params{"doc": docid, "name": Fold(r.Name), "desc": Fold(r.Description), "cat": Fold(r.Category), "rname": Fold(r.RestName)}).
			Execute(); err != nil {
			return err
		}
	}
	return nil
}

func jsonList(raw string) []string {
	var out []string
	if raw = strings.TrimSpace(raw); raw == "" || raw == "null" {
		return nil
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// Counts returns the number of indexed restaurants and items.
func Counts(app core.App) (restaurants, items int, err error) {
	if err = app.DB().NewQuery("SELECT COUNT(*) FROM " + TableRestaurantDocs).Row(&restaurants); err != nil {
		return
	}
	err = app.DB().NewQuery("SELECT COUNT(*) FROM " + TableItemDocs).Row(&items)
	return
}

// ---------------------------------------------------------------- hooks

// Bind keeps the index in sync with every record write (collections API,
// import, synchronisation, admin /_/). The hooks run around the DB write,
// inside its transaction when there is one; an index failure is logged and
// never blocks the write (the index is derived data, rebuilt at startup if
// the counts drift).
func Bind(app core.App) {
	app.OnRecordCreate("restaurants").BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		logErr(e.App, ReindexRestaurantRow(e.App, e.Record.Id))
		return nil
	})
	app.OnRecordUpdate("restaurants").BindFunc(func(e *core.RecordEvent) error {
		o := e.Record.Original()
		nameChanged := o.GetString("name") != e.Record.GetString("name")
		rowChanged := nameChanged || o.GetString("address") != e.Record.GetString("address") ||
			strings.Join(o.GetStringSlice("cuisines"), "|") != strings.Join(e.Record.GetStringSlice("cuisines"), "|")
		if err := e.Next(); err != nil {
			return err
		}
		switch {
		case nameChanged: // the restaurant name is indexed with every item
			logErr(e.App, Reindex(e.App, e.Record.Id))
		case rowChanged:
			logErr(e.App, ReindexRestaurantRow(e.App, e.Record.Id))
		}
		return nil
	})
	app.OnRecordDelete("restaurants").BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		logErr(e.App, RemoveRestaurant(e.App, e.Record.Id))
		return nil
	})

	app.OnRecordCreate("menu_items").BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		logErr(e.App, ReindexItem(e.App, e.Record.Id))
		return nil
	})
	app.OnRecordUpdate("menu_items").BindFunc(func(e *core.RecordEvent) error {
		o := e.Record.Original()
		changed := false
		for _, f := range []string{"name", "description", "category", "restaurant"} {
			if o.GetString(f) != e.Record.GetString(f) {
				changed = true
				break
			}
		}
		if err := e.Next(); err != nil {
			return err
		}
		if changed { // price / availability are not indexed (joined at query time)
			logErr(e.App, ReindexItem(e.App, e.Record.Id))
		}
		return nil
	})
	app.OnRecordDelete("menu_items").BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		logErr(e.App, RemoveItem(e.App, e.Record.Id))
		return nil
	})

	// a renamed / deleted category changes the indexed text of its items
	app.OnRecordUpdate("menu_categories").BindFunc(func(e *core.RecordEvent) error {
		changed := e.Record.Original().GetString("name") != e.Record.GetString("name")
		if err := e.Next(); err != nil {
			return err
		}
		if changed {
			logErr(e.App, Reindex(e.App, e.Record.GetString("restaurant")))
		}
		return nil
	})
	app.OnRecordAfterDeleteSuccess("menu_categories").BindFunc(func(e *core.RecordEvent) error {
		logErr(e.App, Reindex(e.App, e.Record.GetString("restaurant")))
		return e.Next()
	})

	// startup safety net: rebuild when the index drifted from the catalogue
	// (records written with raw SQL by a migration, restored backup…)
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		logErr(se.App, RebuildIfDrifted(se.App))
		return se.Next()
	})
}

// RebuildIfDrifted rebuilds the index when its counts differ from the catalogue.
func RebuildIfDrifted(app core.App) error {
	if !Ready(app) {
		return nil
	}
	ri, ii, err := Counts(app)
	if err != nil {
		return err
	}
	var rc, ic int
	if err := app.DB().NewQuery("SELECT COUNT(*) FROM restaurants").Row(&rc); err != nil {
		return err
	}
	if err := app.DB().NewQuery("SELECT COUNT(*) FROM menu_items").Row(&ic); err != nil {
		return err
	}
	if ri == rc && ii == ic {
		return nil
	}
	app.Logger().Info("search: index rebuilt", "restaurants", rc, "items", ic, "indexedRestaurants", ri, "indexedItems", ii)
	return Rebuild(app)
}

var logOnce sync.Map

func logErr(app core.App, err error) {
	if err == nil {
		return
	}
	// one line per distinct error (a broken index would otherwise flood the logs)
	if _, dup := logOnce.LoadOrStore(err.Error(), true); dup {
		return
	}
	app.Logger().Warn("search: index update failed", "error", err.Error())
}
