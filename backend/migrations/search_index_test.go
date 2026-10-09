package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/search"
)

func searchCounts(t *testing.T, app core.App) (int, int) {
	t.Helper()
	r, i, err := search.Counts(app)
	if err != nil {
		t.Fatal(err)
	}
	return r, i
}

func TestSearchIndexMigration(t *testing.T) {
	for name, data := range map[string]string{"real data": fixture, "demo": "[]"} {
		t.Run(name, func(t *testing.T) {
			defer SetRealDataForTesting([]byte(data))()
			app := newApp(t) // full migration history on a fresh db
			if !search.Ready(app) {
				t.Fatal("search tables missing")
			}
			rc, _ := app.CountRecords("restaurants")
			ic, _ := app.CountRecords("menu_items")
			if ic == 0 {
				t.Fatal("empty catalogue")
			}
			ri, ii := searchCounts(t, app)
			if ri != int(rc) || ii != int(ic) {
				t.Fatalf("indexed %d/%d, catalogue %d/%d", ri, ii, rc, ic)
			}
			// a dish of the catalogue is found, accents folded
			term := "boulets"
			if name == "demo" {
				term = "margherita"
			}
			res, err := search.Search(app, search.Params{Query: term})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Dishes) == 0 {
				t.Fatalf("%q not found after migration", term)
			}

			// down then up again (idempotent)
			if err := downSearchIndex(app); err != nil {
				t.Fatal(err)
			}
			if search.Ready(app) {
				t.Fatal("tables not dropped")
			}
			if res, err := search.Search(app, search.Params{Query: term}); err != nil || len(res.Dishes) != 0 {
				t.Fatalf("search without tables: %v %v", res, err)
			}
			if err := upSearchIndex(app); err != nil {
				t.Fatal(err)
			}
			if err := upSearchIndex(app); err != nil { // re-run: rebuilt, no duplicate
				t.Fatal(err)
			}
			if ri, ii := searchCounts(t, app); ri != int(rc) || ii != int(ic) {
				t.Fatalf("after re-run %d/%d", ri, ii)
			}
		})
	}
}
