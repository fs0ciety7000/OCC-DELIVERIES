package migrations

import (
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// 1760000006 applies on the full history (production data embedded), adds
// the two flags and the snapshot source, and can be rolled back.
func TestUberEatsSnapshotMigration(t *testing.T) {
	app := newApp(t) // runs every migration in order

	rest, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"partial_menu", "geo_approx"} {
		if _, ok := rest.Fields.GetByName(f).(*core.BoolField); !ok {
			t.Errorf("restaurants.%s missing", f)
		}
	}
	recs, err := app.FindAllRecords("restaurants")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.GetBool("partial_menu") || r.GetBool("geo_approx") {
			t.Fatalf("%s: existing restaurants keep a full menu / real position", r.GetString("slug"))
		}
	}

	src, _ := app.FindCollectionByNameOrId("sync_sources")
	sel, ok := src.Fields.GetByName("provider").(*core.SelectField)
	if !ok || !slices.Contains(sel.Values, SourceUberEatsSnapshot) || !slices.Contains(sel.Values, "deliveroo") {
		t.Fatalf("provider values %v", sel)
	}
	rows, err := app.FindAllRecords("sync_sources")
	if err != nil {
		t.Fatal(err)
	}
	var snap []*core.Record
	for _, r := range rows {
		if r.GetString("provider") == SourceUberEatsSnapshot {
			snap = append(snap, r)
		}
	}
	if len(snap) != 1 {
		t.Fatalf("%d snapshot sources", len(snap))
	}
	s := snap[0]
	if !s.GetBool("enabled") || s.GetInt("priority") != 70 || s.GetString("label") != "Uber Eats (instantané connecteur)" ||
		s.GetString("city") != "mons" || s.GetString("url") != "" {
		t.Fatalf("seeded row %v", s.FieldsData())
	}
	for _, r := range rows {
		if r.GetString("provider") != SourceUberEatsSnapshot && r.GetInt("priority") >= 70 {
			t.Fatalf("feeds must come before the snapshot: %v", r.FieldsData())
		}
	}

	// idempotent, then down / up
	if err := upUberEatsSnapshot(app); err != nil {
		t.Fatal(err)
	}
	if n, _ := app.CountRecords("sync_sources"); int(n) != len(rows) {
		t.Fatalf("up twice: %d rows, want %d", n, len(rows))
	}
	if err := downUberEatsSnapshot(app); err != nil {
		t.Fatal(err)
	}
	rest, _ = app.FindCollectionByNameOrId("restaurants")
	if rest.Fields.GetByName("partial_menu") != nil {
		t.Fatal("down must drop partial_menu")
	}
	if n, _ := app.CountRecords("sync_sources"); int(n) != len(rows)-1 {
		t.Fatalf("down: %d rows", n)
	}
	if err := upUberEatsSnapshot(app); err != nil {
		t.Fatal(err)
	}
}

func TestUberEatsSnapshotData(t *testing.T) {
	b, err := UberEatsSnapshot()
	if err != nil || b == nil {
		t.Fatalf("embedded file: %v", err)
	}
	restore := SetUberEatsSnapshotForTesting([]byte(`[{"name":"X"}]`))
	if b, _ := UberEatsSnapshot(); string(b) != `[{"name":"X"}]` {
		t.Fatalf("override %q", b)
	}
	restore()
}
