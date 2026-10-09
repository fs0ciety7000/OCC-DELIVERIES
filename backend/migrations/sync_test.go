package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The sync migration applies on top of every earlier migration, with the
// real data of production (embedded file), and keeps that data untouched.
func TestSyncMigrationOnProductionHistory(t *testing.T) {
	list, _ := RealRestaurants()
	if len(list) == 0 {
		t.Skip("no embedded real data")
	}
	app := newApp(t) // runs 1760000000 … 1760000005 in order

	rest, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"source_key", "sources", "locked", "stale_since"} {
		if rest.Fields.GetByName(f) == nil {
			t.Errorf("restaurants.%s missing", f)
		}
	}
	items, _ := app.FindCollectionByNameOrId("menu_items")
	for _, f := range []string{"source_key", "sources", "locked"} {
		if items.Fields.GetByName(f) == nil {
			t.Errorf("menu_items.%s missing", f)
		}
	}
	if p, ok := items.Fields.GetByName("price").(*core.NumberField); !ok || p.Required || p.Min == nil || *p.Min != 0 {
		t.Error("menu_items.price: optional (0 € allowed), min 0")
	}

	// production data untouched: still the real restaurants, unlocked, not stale
	recs, err := app.FindAllRecords("restaurants")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != len(list) {
		t.Fatalf("%d restaurants, want %d", len(recs), len(list))
	}
	for _, r := range recs {
		if r.GetBool("locked") || r.GetString("stale_since") != "" || r.GetString("source_key") != "" {
			t.Fatalf("%s: %v", r.GetString("slug"), r.FieldsData())
		}
	}

	// rules: admin only, runs written by the server only
	src, err := app.FindCollectionByNameOrId("sync_sources")
	if err != nil {
		t.Fatal(err)
	}
	runs, err := app.FindCollectionByNameOrId("sync_runs")
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []*string{src.ListRule, src.ViewRule, src.CreateRule, src.UpdateRule, src.DeleteRule, runs.ListRule, runs.ViewRule} {
		if rule == nil || *rule != isAdminRule {
			t.Fatalf("rule %v", rule)
		}
	}
	if runs.CreateRule != nil || runs.UpdateRule != nil || runs.DeleteRule != nil {
		t.Fatal("sync_runs must be written by the server only")
	}

	// default sources: every Takeaway mini-site, Deliveroo Mons, weloveat Mons
	// (the Uber Eats snapshot row of 1760000006 is checked in ubereats_snapshot_test.go)
	seeded, err := app.FindRecordsByFilter("sync_sources", "enabled = true && provider != 'ubereats-snapshot'", "priority", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	sites := TakeawaySiteURLs()
	if len(sites) == 0 || len(seeded) != len(sites)+2 {
		t.Fatalf("%d sources for %d sites", len(seeded), len(sites))
	}
	byProvider := map[string]int{}
	for _, s := range seeded {
		byProvider[s.GetString("provider")]++
		if s.GetBool("options") || s.GetString("city") != "mons" {
			t.Fatalf("source %v", s.FieldsData())
		}
	}
	if byProvider["takeaway-site"] != len(sites) || byProvider["deliveroo"] != 1 || byProvider["weloveat"] != 1 {
		t.Fatalf("providers %v", byProvider)
	}
	dr, _ := app.FindFirstRecordByData("sync_sources", "provider", "deliveroo")
	if dr.GetString("url") != DefaultDeliverooListing {
		t.Fatalf("deliveroo url %q", dr.GetString("url"))
	}
	if seeded[0].GetString("provider") != "takeaway-site" {
		t.Fatal("the mini-sites (complete menus) come first")
	}

	// down then up again
	if err := downSync(app); err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindCollectionByNameOrId("sync_runs"); err == nil {
		t.Fatal("down must drop sync_runs")
	}
	if err := upSync(app); err != nil {
		t.Fatal(err)
	}
}
