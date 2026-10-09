package migrations

import (
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// fixture: two real restaurants (unknown keys ignored) + one invalid entry.
const fixture = `[
  {"slug":"vrai-resto","name":"Vrai Resto","lat":50.45,"lng":3.95,"address":"Rue Vraie 1, 7000 Mons",
   "source_urls":["https://example.com"],"menu_checked_at":"2026-10-01",
   "categories":[{"name":"Plats","items":[{"name":"Boulets","price":1450,"extra":true}]}]},
  {"slug":"autre-resto","name":"Autre Resto","categories":[]},
  {"slug":"Bad Slug","name":""}
]`

func newApp(t *testing.T) *tests.TestApp {
	t.Helper()
	dir := t.TempDir()
	app, err := tests.NewTestApp(dir) // runs every migration on a fresh db
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	return app
}

func slugs(t *testing.T, app core.App) map[string]bool {
	t.Helper()
	recs, err := app.FindAllRecords("restaurants")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, r := range recs {
		out[r.GetString("slug")] = r.GetBool("active")
	}
	return out
}

func TestRealRestaurantsDecoding(t *testing.T) {
	defer SetRealDataForTesting([]byte(fixture))()
	list, problems := RealRestaurants()
	if len(list) != 2 || len(problems) != 1 || !strings.Contains(problems[0], "n° 3") {
		t.Fatalf("list=%d problems=%v", len(list), problems)
	}
	for _, data := range []string{"", "  ", "[]", "{\"restaurants\":[]}"} {
		restore := SetRealDataForTesting([]byte(data))
		if HasRealData() {
			t.Errorf("%q: no real data expected", data)
		}
		restore()
	}
	restore := SetRealDataForTesting(nil) // file missing
	if l, p := RealRestaurants(); len(l) != 0 || len(p) != 0 {
		t.Errorf("missing file: %v %v", l, p)
	}
	restore()
	restore = SetRealDataForTesting([]byte(`{"restaurants":[{"slug":"a","name":"A"}]}`))
	if !HasRealData() {
		t.Error("wrapped object not decoded")
	}
	restore()
	restore = SetRealDataForTesting([]byte(`not json`))
	if l, p := RealRestaurants(); len(l) != 0 || len(p) != 1 {
		t.Errorf("invalid json: %v %v", l, p)
	}
	restore()
}

func TestFreshInstallWithoutRealDataKeepsDemo(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)
	got := slugs(t, app)
	for _, d := range DemoRestaurants() {
		if !got[d.Slug] {
			t.Errorf("demo %s missing", d.Slug)
		}
	}
}

func TestFreshInstallWithRealDataSkipsDemo(t *testing.T) {
	defer SetRealDataForTesting([]byte(fixture))()
	app := newApp(t)
	got := slugs(t, app)
	if len(got) != 2 || !got["vrai-resto"] || !got["autre-resto"] {
		t.Fatalf("restaurants: %v", got)
	}
	r, _ := app.FindFirstRecordByData("restaurants", "slug", "vrai-resto")
	if n, _ := app.CountRecords("menu_items", dbx.HashExp{"restaurant": r.Id}); n != 1 {
		t.Fatalf("menu items: %d", n)
	}
}

func TestReplaceDemoOnExistingInstall(t *testing.T) {
	restore := SetRealDataForTesting([]byte("[]"))
	app := newApp(t) // demo seeded, replace was a no-op
	restore()

	// a party references La Bella Nonna (as restaurant) and Petit Bangkok (as candidate)
	bella, _ := app.FindFirstRecordByData("restaurants", "slug", "la-bella-nonna")
	bangkok, _ := app.FindFirstRecordByData("restaurants", "slug", "petit-bangkok")
	users, _ := app.FindCollectionByNameOrId("users")
	u := core.NewRecord(users)
	u.SetEmail("host@example.com")
	u.SetPassword("password123")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	parties, _ := app.FindCollectionByNameOrId("parties")
	p := core.NewRecord(parties)
	p.Load(map[string]any{"code": "ABCDEF", "host": u.Id, "members": []string{u.Id}, "status": "ordering",
		"restaurant": bella.Id, "candidates": []string{bella.Id, bangkok.Id}})
	if err := app.Save(p); err != nil {
		t.Fatal(err)
	}

	// nothing destructive without real data
	defer SetRealDataForTesting([]byte("[]"))()
	if res, err := ReplaceDemo(app); err != nil || res.Imported != 0 || res.Deleted != 0 {
		t.Fatalf("empty data: %+v %v", res, err)
	}
	if len(slugs(t, app)) != len(DemoRestaurants()) {
		t.Fatal("demo touched without real data")
	}

	SetRealDataForTesting([]byte(fixture))
	res, err := ReplaceDemo(app)
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 2 || res.Deactivated != 2 || res.Deleted != len(DemoRestaurants())-2 || len(res.Skipped) != 1 {
		t.Fatalf("result: %+v", res)
	}
	got := slugs(t, app)
	if active, ok := got["la-bella-nonna"]; !ok || active {
		t.Fatalf("bella should be kept inactive: %v", got)
	}
	if active, ok := got["petit-bangkok"]; !ok || active {
		t.Fatalf("bangkok should be kept inactive: %v", got)
	}
	if _, ok := got["maison-hanami"]; ok {
		t.Fatal("unused demo restaurant not deleted")
	}
	if !got["vrai-resto"] || !got["autre-resto"] {
		t.Fatalf("real data missing: %v", got)
	}
	// the party still points at its restaurant
	p, _ = app.FindRecordById("parties", p.Id)
	if p.GetString("restaurant") != bella.Id {
		t.Fatal("party lost its restaurant")
	}
	// idempotent
	if _, err := ReplaceDemo(app); err != nil {
		t.Fatal(err)
	}
}

// TestEmbeddedRealData checks that the shipped data file decodes cleanly.
func TestEmbeddedRealData(t *testing.T) {
	if _, err := os.Stat(RealDataFile); err != nil {
		t.Skip("no real data file")
	}
	list, problems := RealRestaurants()
	for _, p := range problems {
		t.Errorf("%s: %s", RealDataFile, p)
	}
	t.Logf("%d real restaurants", len(list))
}
