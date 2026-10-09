package migrations

import (
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func takeawaySiteRows(t *testing.T, app core.App) map[string][]*core.Record {
	t.Helper()
	recs, err := app.FindRecordsByFilter("sync_sources", "provider = 'takeaway-site'", "priority", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]*core.Record{}
	for _, r := range recs {
		out[r.GetString("url")] = append(out[r.GetString("url")], r)
	}
	return out
}

// 1760000009 seeds one enabled row per mini-site of the embedded list, never
// a duplicate, whether the sync collection was seeded with the full list
// (fresh database) or with the first ten sites only (production).
func TestMoreTakeawaySitesMigration(t *testing.T) {
	app := newApp(t) // runs every migration in order

	sites := TakeawaySiteURLs()
	added := NewTakeawaySiteURLs()
	if len(added) == 0 || len(sites) != len(originalTakeawaySites)+len(added) {
		t.Fatalf("%d sites, %d new: the ten original sites must stay in the file", len(sites), len(added))
	}
	if len(slices.Compact(slices.Sorted(slices.Values(sites)))) != len(sites) {
		t.Fatal("duplicate URL in data/mons_takeaway_sites.txt")
	}

	check := func(step string) {
		t.Helper()
		rows := takeawaySiteRows(t, app)
		if len(rows) != len(sites) {
			t.Fatalf("%s: %d mini-site rows, want %d", step, len(rows), len(sites))
		}
		for _, u := range sites {
			rs := rows[u]
			if len(rs) != 1 {
				t.Fatalf("%s: %d rows for %s", step, len(rs), u)
			}
			r := rs[0]
			p := r.GetInt("priority")
			if !r.GetBool("enabled") || r.GetString("city") != "mons" || r.GetString("label") != takeawaySiteLabel(u) ||
				p < takeawaySiteFirstPriority || p > takeawaySiteLastPriority {
				t.Fatalf("%s: row %v", step, r.FieldsData())
			}
		}
	}
	check("fresh database")

	// production history: only the ten original sites before 1760000009
	if err := downMoreTakeawaySites(app); err != nil {
		t.Fatal(err)
	}
	rows := takeawaySiteRows(t, app)
	if len(rows) != len(originalTakeawaySites) {
		t.Fatalf("after down: %d rows, want %d", len(rows), len(originalTakeawaySites))
	}
	for _, u := range originalTakeawaySites {
		if len(rows[u]) != 1 {
			t.Fatalf("after down: original site %s missing", u)
		}
	}
	if err := upMoreTakeawaySites(app); err != nil {
		t.Fatal(err)
	}
	check("production history")

	// idempotent
	if err := upMoreTakeawaySites(app); err != nil {
		t.Fatal(err)
	}
	check("second run")
}
