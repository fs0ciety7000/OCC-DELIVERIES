package migrations

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func settingsThreshold(t *testing.T, app core.App) int {
	t.Helper()
	rows, err := app.FindAllRecords("app_settings")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d settings rows", len(rows))
	}
	return rows[0].GetInt("min_menu_items")
}

func TestIncompleteMenusMigration(t *testing.T) {
	t.Run("real data: threshold 10, counts backfilled", func(t *testing.T) {
		defer SetRealDataForTesting([]byte(fixture))()
		app := newApp(t)
		if got := settingsThreshold(t, app); got != MinMenuItemsDefault {
			t.Fatalf("threshold %d", got)
		}
		for slug, want := range map[string]int{"vrai-resto": 1, "autre-resto": 0} {
			r, err := app.FindFirstRecordByData("restaurants", "slug", slug)
			if err != nil {
				t.Fatal(err)
			}
			if got := r.GetInt("items_count"); got != want {
				t.Fatalf("%s items_count %d, want %d", slug, got, want)
			}
		}
	})
	t.Run("demo: filter off, unavailable items not counted", func(t *testing.T) {
		defer SetRealDataForTesting([]byte("[]"))()
		app := newApp(t)
		if got := settingsThreshold(t, app); got != 0 {
			t.Fatalf("threshold %d", got)
		}
		bella, _ := app.FindFirstRecordByData("restaurants", "slug", "la-bella-nonna")
		n, _ := app.CountRecords("menu_items", dbx.HashExp{"restaurant": bella.Id})
		if n == 0 || bella.GetInt("items_count") != int(n) {
			t.Fatalf("bella items_count %d, %d items", bella.GetInt("items_count"), n)
		}
		// re-running the backfill counts only available items
		if _, err := app.DB().Update("menu_items", dbx.Params{"available": false}, dbx.HashExp{"restaurant": bella.Id}).Execute(); err != nil {
			t.Fatal(err)
		}
		if err := upIncompleteMenus(app); err != nil {
			t.Fatal(err)
		}
		bella, _ = app.FindRecordById("restaurants", bella.Id)
		if bella.GetInt("items_count") != 0 {
			t.Fatalf("unavailable items counted: %d", bella.GetInt("items_count"))
		}
		if got := settingsThreshold(t, app); got != 0 {
			t.Fatal("idempotent: the settings row must not be duplicated")
		}
	})
}
