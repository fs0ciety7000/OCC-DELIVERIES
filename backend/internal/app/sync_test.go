package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/feedsync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
	"github.com/fs0ciety7000/occ-deliveries/backend/migrations"
)

// feedServer serves the menusync fixtures like the real sites would: a
// Takeaway mini-site (Tomo), a Deliveroo listing + menu (Baalbeck), and a
// page that waits for release (to keep a run busy).
type feedServer struct {
	*httptest.Server
	release chan struct{}
	waiting atomic.Int32
}

func newFeedServer(t *testing.T) *feedServer {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "menusync", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	site, listing, menu := read("takeaway_site.html"), read("deliveroo_listing.html"), read("deliveroo_menu.html")
	fs := &feedServer{release: make(chan struct{})}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tomo/":
			_, _ = w.Write(site)
		case r.URL.Path == "/fr/restaurants/brussels/mons-center":
			_, _ = w.Write(listing)
		case strings.HasPrefix(r.URL.Path, "/fr/menu/"):
			_, _ = w.Write(menu)
		case r.URL.Path == "/slow/":
			fs.waiting.Add(1)
			<-fs.release
			_, _ = w.Write(site)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fs.Close)
	return fs
}

// syncEnv is an env whose synchronisation is enabled, reads only the local
// feed server, and never really pauses.
type syncEnv struct {
	*env
	h *handlers
}

func newSyncEnv(t *testing.T) *syncEnv {
	t.Helper()
	return newSyncEnvWith(t, nil)
}

// newSyncEnvWith is newSyncEnv with a hook on the sync configuration.
func newSyncEnvWith(t *testing.T, tweak func(*SyncConfig)) *syncEnv {
	t.Helper()
	cfg := testConfig
	cfg.Sync = SyncConfig{
		Enabled: true, Cron: "30 3 * * *", CacheDir: t.TempDir(),
		NewFetcher: func(dir string) *menusync.Fetcher {
			f := menusync.NewFetcher(dir)
			f.Sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() }
			return f
		},
	}
	if tweak != nil {
		tweak(&cfg.Sync)
	}
	ta := newTestAppNoRegister(t)
	h := register(ta, cfg)
	t.Cleanup(func() { h.sync.stop(); ta.Cleanup() })
	return &syncEnv{env: serveEnv(t, ta), h: h}
}

// setSources replaces the seeded (real) sources by local ones.
func (e *syncEnv) setSources(srcs ...map[string]any) {
	e.t.Helper()
	old, err := e.app.FindAllRecords(colSyncSources)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, r := range old {
		if err := e.app.Delete(r); err != nil {
			e.t.Fatal(err)
		}
	}
	col, _ := e.app.FindCollectionByNameOrId(colSyncSources)
	for _, s := range srcs {
		r := core.NewRecord(col)
		r.Load(map[string]any{"enabled": true, "city": "mons"})
		r.Load(s)
		if err := e.app.Save(r); err != nil {
			e.t.Fatal(err)
		}
	}
}

// runSync performs a run synchronously and returns the stored run.
func (e *syncEnv) runSync(trigger string) syncRun {
	e.t.Helper()
	rec, err := e.h.sync.begin(trigger)
	if err != nil {
		e.t.Fatal(err)
	}
	e.h.sync.execute(context.Background(), rec, "")
	rec, err = e.app.FindRecordById(colSyncRuns, rec.Id)
	if err != nil {
		e.t.Fatal(err)
	}
	return toSyncRun(rec, true)
}

func (e *syncEnv) waitIdle() {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for e.h.sync.running() != "" {
		if time.Now().After(deadline) {
			e.t.Fatal("run never finished")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (e *syncEnv) restaurantsNamed(name string) []*core.Record {
	e.t.Helper()
	recs, err := e.app.FindRecordsByFilter(colRestaurants, "name = {:n}", "", 0, 0, map[string]any{"n": name})
	if err != nil {
		e.t.Fatal(err)
	}
	return recs
}

func (e *syncEnv) item(restaurantID, name string) *core.Record {
	e.t.Helper()
	r, err := e.app.FindFirstRecordByFilter(colMenuItems, "restaurant = {:r} && name = {:n}", map[string]any{"r": restaurantID, "n": name})
	if err != nil {
		e.t.Fatalf("item %s: %v", name, err)
	}
	return r
}

func TestSyncReconcilesIntoDatabase(t *testing.T) {
	e := newSyncEnv(t)
	srv := newFeedServer(t)
	e.setSources(
		map[string]any{"provider": "takeaway-site", "label": "Tomo", "url": srv.URL + "/tomo/", "priority": 10},
		map[string]any{"provider": "deliveroo", "label": "Deliveroo", "url": srv.URL + "/fr/restaurants/brussels/mons-center?geohash=x", "priority": 50},
	)
	// the curated record already in production (hand-made extract of the menu)
	tomoID, _, err := catalog.Import(e.app, catalog.RestaurantImport{
		Slug: "tomo-mons", Name: "Tomo", Emoji: "🍜", Address: "Rue d'Enghien 11, 7000 Mons", Lat: 50.45436, Lng: 3.95122,
		Phone:     "065 35 29 64",
		Providers: []providers.Link{{ID: providers.Takeaway, URL: "https://www.takeaway.com/be/menu/tomo-mons"}},
		Categories: []catalog.CategoryImport{{Name: "Menus", Items: []catalog.ItemImport{
			{Name: "Menu classique ramen", Price: 1500},
			{Name: "Ancien plat", Price: 900},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := e.app.CountRecords(colRestaurants)

	run := e.runSync(triggerManual)
	if run.Status != runSuccess || run.Trigger != triggerManual || run.FinishedAt == "" {
		t.Fatalf("run %+v", run)
	}
	st := run.Stats
	if st.RestaurantsCreated != 1 || st.RestaurantsUpdated != 1 || st.ItemsPriceChanged != 1 || st.ItemsUnavailable != 1 || st.ItemsCreated == 0 {
		t.Fatalf("stats %+v\n%s", st, run.Log)
	}
	if !strings.Contains(strings.Join(run.Changes, "\n"), "Tomo — Menu classique ramen : 15,00 € → 16,00 €") {
		t.Fatalf("changes %v", run.Changes)
	}
	if after, _ := e.app.CountRecords(colRestaurants); after != before+1 {
		t.Fatalf("restaurants %d → %d (Tomo must be matched, Baalbeck created)", before, after)
	}
	if n := len(e.restaurantsNamed("Tomo")); n != 1 {
		t.Fatalf("%d Tomo", n)
	}
	tomo, _ := e.app.FindRecordById(colRestaurants, tomoID)
	if tomo.GetString("emoji") != "🍜" || tomo.GetString("phone") != "+3265352964" || tomo.GetFloat("lat") != 50.45436 ||
		!strings.HasPrefix(tomo.GetString("source_key"), "takeaway-site:") || tomo.GetString("sources") == "[]" {
		t.Fatalf("curated fields / provenance: %v", tomo.FieldsData())
	}
	if it := e.item(tomoID, "Ancien plat"); it.GetBool("available") {
		t.Fatal("an item missing from the feed must become unavailable")
	}
	baal := e.restaurantsNamed("Baalbeck")
	if len(baal) != 1 || !baal[0].GetBool("active") || baal[0].GetString("source_key") == "" {
		t.Fatalf("baalbeck %v", baal)
	}
	// items_count follows the reconciled menus (Tomo lost "Ancien plat")
	assertItemsCounts(t, e.app)
	if n, _ := catalog.CountAvailableItems(e.app, tomoID); n == 0 || tomo.GetInt("items_count") != n {
		t.Fatalf("tomo items_count %d, %d available", tomo.GetInt("items_count"), n)
	}
	for _, s := range run.Sources {
		if s.Status != feedsync.StatusOK {
			t.Fatalf("source %+v", s)
		}
	}
	src, _ := e.app.FindFirstRecordByData(colSyncSources, "label", "Tomo")
	if !strings.HasPrefix(src.GetString("last_status"), "ok:") || src.GetString("last_run_at") == "" {
		t.Fatalf("source status %q", src.GetString("last_status"))
	}

	// second run: answers come from the cache (a missing robots.txt is
	// re-checked), nothing changes
	run = e.runSync(triggerManual)
	if run.Stats != (feedsync.Stats{}) || len(run.Changes) != 0 || run.Sources[0].Network > 1 || run.Sources[0].Cached == 0 {
		t.Fatalf("second run must be a no-op from the cache: %+v %v %+v", run.Stats, run.Changes, run.Sources)
	}

	// an admin edit alone does not lock (the sync would restore the price);
	// locking is explicit: a locked item is left alone by the sync
	admin := e.admin("Root")
	menuID := e.item(tomoID, "Menu classique ramen").Id
	edited := e.expect(200, "PATCH", "/api/collections/menu_items/records/"+menuID, admin.token, map[string]any{"price": 1450}).m(t)
	if edited["locked"] != false {
		t.Fatalf("an edit must not lock by itself: %v", edited)
	}
	e.expect(200, "PATCH", "/api/collections/menu_items/records/"+menuID, admin.token, map[string]any{"price": 1450, "locked": true})
	e.runSync(triggerManual)
	if p := e.item(tomoID, "Menu classique ramen").GetInt("price"); p != 1450 {
		t.Fatalf("locked item modified: %d", p)
	}
	// unlocking (toggle) gives it back to the sync
	e.expect(200, "PATCH", "/api/collections/menu_items/records/"+menuID, admin.token, map[string]any{"locked": false})
	if run := e.runSync(triggerManual); run.Stats.ItemsPriceChanged != 1 {
		t.Fatalf("unlocked item: %+v", run.Stats)
	}
	if p := e.item(tomoID, "Menu classique ramen").GetInt("price"); p != 1600 {
		t.Fatalf("unlocked item price %d", p)
	}
	// restaurant edits never lock implicitly; the explicit toggle does
	e.expect(200, "PATCH", "/api/collections/restaurants/records/"+tomoID, admin.token, map[string]any{"phone": "065 00 00 00"})
	if r, _ := e.app.FindRecordById(colRestaurants, tomoID); r.GetBool("locked") {
		t.Fatal("restaurant edit must not lock by itself")
	}
	e.expect(200, "PATCH", "/api/collections/restaurants/records/"+tomoID, admin.token, map[string]any{"locked": true})
	if r, _ := e.app.FindRecordById(colRestaurants, tomoID); !r.GetBool("locked") {
		t.Fatal("explicit toggle must lock")
	}

	// a source disappears → its restaurants become stale (never deleted)
	e.setSources(map[string]any{"provider": "takeaway-site", "label": "Tomo", "url": srv.URL + "/tomo/", "priority": 10})
	run = e.runSync(triggerManual)
	if run.Stats.RestaurantsStale != 1 {
		t.Fatalf("stale %+v\n%v", run.Stats, run.Changes)
	}
	b, _ := e.app.FindRecordById(colRestaurants, baal[0].Id)
	if b.GetString("stale_since") == "" || !b.GetBool("active") {
		t.Fatalf("stale restaurant: %v", b.FieldsData())
	}
}

func TestSyncMutualExclusionAndAuthz(t *testing.T) {
	e := newSyncEnv(t)
	srv := newFeedServer(t)
	e.setSources(map[string]any{"provider": "takeaway-site", "label": "Lent", "url": srv.URL + "/slow/", "priority": 10})
	admin := e.admin("Root")
	bob := e.user("Bob")

	// authz: 401 without auth, 403 without the admin role
	for _, rt := range []struct{ method, url string }{
		{"GET", "/api/occ/admin/sync/status"}, {"GET", "/api/occ/admin/sync/runs"},
		{"GET", "/api/occ/admin/sync/runs/x"}, {"POST", "/api/occ/admin/sync/run"},
	} {
		e.expect(401, rt.method, rt.url, "", nil)
		e.expect(403, rt.method, rt.url, bob.token, nil)
	}
	if got := e.expect(200, "GET", "/api/collections/sync_sources/records", bob.token, nil).m(t); got["totalItems"].(float64) != 0 {
		t.Fatalf("a member must not see the sources: %v", got)
	}
	e.expect(400, "POST", "/api/collections/sync_sources/records", bob.token, map[string]any{"provider": "jsonld", "url": "https://x.be"})
	e.expect(400, "POST", "/api/collections/sync_sources/records", admin.token, map[string]any{"provider": "jsonld", "url": "ftp://x"})
	src := e.expect(200, "POST", "/api/collections/sync_sources/records", admin.token, map[string]any{
		"provider": "jsonld", "url": "https://resto.example/carte", "enabled": false, "last_status": "ok: forgé",
	}).m(t)
	if src["city"] != "mons" || src["last_status"] != "" || src["label"] != "jsonld" {
		t.Fatalf("source normalized: %v", src)
	}
	e.expect(403, "POST", "/api/collections/sync_runs/records", admin.token, map[string]any{"status": "success", "trigger": "manual"})

	status := e.expect(200, "GET", "/api/occ/admin/sync/status", admin.token, nil).m(t)
	if status["enabled"] != true || status["running"] != nil || status["nextRunAt"] == nil || status["timezone"] != "Europe/Brussels" {
		t.Fatalf("status %v", status)
	}

	// manual run in the background, blocked on the slow page
	started := e.expect(202, "POST", "/api/occ/admin/sync/run", admin.token, nil).m(t)
	runID := started["run"].(map[string]any)["id"].(string)
	deadline := time.Now().Add(5 * time.Second)
	for srv.waiting.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the run never reached the slow page")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// one run at a time: manual → 409, cron tick → skipped
	e.expect(409, "POST", "/api/occ/admin/sync/run", admin.token, nil)
	due := time.Date(2026, 10, 10, 3, 30, 0, 0, e.h.sync.loc)
	if e.h.sync.tick(due) {
		t.Fatal("a cron tick must not start a second run")
	}
	status = e.expect(200, "GET", "/api/occ/admin/sync/status", admin.token, nil).m(t)
	if status["running"].(map[string]any)["id"] != runID {
		t.Fatalf("status while running: %v", status)
	}
	close(srv.release)
	e.waitIdle()

	run := e.expect(200, "GET", "/api/occ/admin/sync/runs/"+runID, admin.token, nil).m(t)["run"].(map[string]any)
	if run["status"] != runSuccess || run["log"] == "" {
		t.Fatalf("run %v", run)
	}
	list := e.expect(200, "GET", "/api/occ/admin/sync/runs", admin.token, nil).m(t)
	if list["totalItems"].(float64) != 1 {
		t.Fatalf("runs %v", list)
	}
	if _, ok := list["items"].([]any)[0].(map[string]any)["log"]; ok {
		t.Fatal("the list must not carry the logs")
	}

	// the cron starts a run when due, and only then
	if e.h.sync.tick(due.Add(time.Minute)) {
		t.Fatal("not due")
	}
	if !e.h.sync.tick(due) {
		t.Fatal("due tick must start a run")
	}
	e.waitIdle()
	if n, _ := e.app.CountRecords(colSyncRuns); n != 2 {
		t.Fatalf("runs %d", n)
	}
}

func TestSyncDisabled(t *testing.T) {
	e := newEnv(t) // testConfig: synchronisation disabled
	admin := e.admin("Root")
	e.expect(400, "POST", "/api/occ/admin/sync/run", admin.token, nil)
	st := e.expect(200, "GET", "/api/occ/admin/sync/status", admin.token, nil).m(t)
	if st["enabled"] != false || st["nextRunAt"] != nil {
		t.Fatalf("status %v", st)
	}
}

func TestSyncConfigFromEnv(t *testing.T) {
	t.Setenv("OCC_SYNC_ENABLED", "false")
	t.Setenv("OCC_SYNC_CRON", "0 4 * * 1")
	t.Setenv("OCC_SYNC_ON_START", "non")
	t.Setenv("OCC_SYNC_START_DELAY", "5s")
	c := syncConfigFromEnv()
	if c.Enabled || c.Cron != "0 4 * * 1" || c.OnStart || c.StartDelay != 5*time.Second || c.CacheTTL != 20*time.Hour {
		t.Fatalf("%+v", c)
	}
	t.Setenv("OCC_SYNC_ENABLED", "")
	t.Setenv("OCC_SYNC_CRON", "")
	if c := syncConfigFromEnv(); !c.Enabled || c.Cron != DefaultSyncCron {
		t.Fatalf("defaults %+v", c)
	}
}

const appSnapshot = `[
 {"name":"Tomo (Mons)","url":"https://www.ubereats.com/be/store/tomo/aaa?ps=1","rating":4.9,"rating_count":12,"eta_min":25,
  "categories":["Japonais"],"promo":"","lat":0,"lng":0,"address":"","geo_approx":true,"items":[{"name":"Gyoza","price":650}],"checked_at":"2026-10-09"},
 {"name":"Le Nouveau Wok (Independant)","url":"https://www.ubereats.com/be/store/nouveau-wok/bbb","rating":4.2,"rating_count":30,"eta_min":30,
  "categories":["Chinois"],"promo":"Livraison offerte","lat":0,"lng":0,"address":"","geo_approx":true,
  "items":[{"name":"Nouilles sautées","price":1150},{"name":"Riz cantonais","price":990}],"checked_at":"2026-10-09"}
]`

func TestSyncUberEatsSnapshot(t *testing.T) {
	e := newSyncEnv(t)
	srv := newFeedServer(t)
	e.h.sync.cfg.Snapshot = func() ([]byte, error) { return []byte(appSnapshot), nil }
	e.setSources(
		map[string]any{"provider": "takeaway-site", "label": "Tomo", "url": srv.URL + "/tomo/", "priority": 10},
		map[string]any{"provider": "ubereats-snapshot", "label": "Uber Eats (instantané connecteur)", "priority": 70},
	)

	run := e.runSync(triggerManual)
	if run.Status != runSuccess || len(run.Sources) != 2 || run.Sources[1].Provider != "ubereats-snapshot" ||
		run.Sources[1].Status != feedsync.StatusOK || run.Sources[1].Network != 0 || run.Sources[1].Restaurants != 2 {
		t.Fatalf("run %+v\n%s", run.Sources, run.Log)
	}
	if run.Stats.RestaurantsCreated != 2 {
		t.Fatalf("Tomo (feed) + the wok (snapshot): %+v\n%s", run.Stats, run.Log)
	}

	// Tomo comes from its site: Uber Eats link added, full menu untouched
	tomo := e.restaurantsNamed("Tomo")
	if len(tomo) != 1 || tomo[0].GetBool("partial_menu") || !strings.Contains(tomo[0].GetString("providers"), "ubereats.com/be/store/tomo/aaa") ||
		strings.Contains(tomo[0].GetString("providers"), "?ps=") {
		t.Fatalf("tomo %v", tomo)
	}
	if n, _ := e.app.CountRecords(colMenuCategories, dbxName(tomo[0].Id, "Aperçu")); n != 0 {
		t.Fatal("the snapshot must not touch a full menu")
	}

	// the wok: active, partial menu, approximate position, preview category
	wok := e.restaurantsNamed("Le Nouveau Wok")
	if len(wok) != 1 {
		t.Fatalf("wok %v", wok)
	}
	w := wok[0]
	if !w.GetBool("active") || !w.GetBool("partial_menu") || !w.GetBool("geo_approx") || w.GetFloat("lat") != testConfig.DefaultLat ||
		w.GetFloat("lng") != testConfig.DefaultLng || w.GetString("emoji") != "🥡" || w.GetInt("eta_max") != 45 || w.GetFloat("rating") != 4.2 ||
		w.GetString("source_key") != "ubereats-snapshot:ubereats.com/be/store/nouveau-wok/bbb" {
		t.Fatalf("wok %v", w.FieldsData())
	}
	cat, err := e.app.FindFirstRecordByFilter(colMenuCategories, "restaurant = {:r} && name = 'Aperçu'", map[string]any{"r": w.Id})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := e.app.CountRecords(colMenuItems, dbxCat(cat.Id)); n != 2 {
		t.Fatalf("%d preview items", n)
	}
	// public: anyone sees the flags
	got := e.expect(200, "GET", "/api/collections/restaurants/records/"+w.Id, "", nil).m(t)
	if got["partial_menu"] != true || got["geo_approx"] != true {
		t.Fatalf("public record %v", got)
	}
	// nearby: approximate positions after the real ones
	items := e.expect(200, "GET", "/api/occ/restaurants/nearby?radiusKm=50", "", nil).m(t)["items"].([]any)
	last := items[len(items)-1].(map[string]any)
	if last["id"] != w.Id {
		t.Fatalf("approximate position must be listed last: %v", last["name"])
	}
	// members can order from the preview
	alice := e.user("Alice")
	party := e.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{"title": "Midi"}).m(t)
	e.expect(200, "POST", "/api/occ/parties/"+party["id"].(string)+"/transition", alice.token, map[string]any{"to": "ordering", "restaurant": w.Id})

	assertItemsCounts(t, e.app)
	if w2, _ := e.app.FindRecordById(colRestaurants, w.Id); w2.GetInt("items_count") != 2 {
		t.Fatalf("wok items_count %d", w2.GetInt("items_count"))
	}

	// second run: nothing changes; the wok is never stale (no feed lists it)
	run = e.runSync(triggerManual)
	if run.Stats != (feedsync.Stats{}) || len(run.Changes) != 0 {
		t.Fatalf("second run must be a no-op: %+v %v", run.Stats, run.Changes)
	}
	if r, _ := e.app.FindRecordById(colRestaurants, w.Id); r.GetString("stale_since") != "" {
		t.Fatal("snapshot restaurant marked stale")
	}

	// admin export / import carry the flags; a full import ends the preview
	ex, err := catalog.ExportOne(e.app, w.GetString("slug"))
	if err != nil || ex == nil || !ex.PartialMenu || !ex.GeoApprox {
		t.Fatalf("export %+v %v", ex, err)
	}
	ex.PartialMenu = false
	ex.Lat, ex.Lng = 0, 0 // coordinates (and their approximation) kept
	if _, _, err := catalog.Import(e.app, *ex); err != nil {
		t.Fatal(err)
	}
	if r, _ := e.app.FindRecordById(colRestaurants, w.Id); r.GetBool("partial_menu") || !r.GetBool("geo_approx") {
		t.Fatalf("after import %v", r.FieldsData())
	}

	// an invalid file: the source fails, the run is partial, nothing is stale
	e.h.sync.cfg.Snapshot = func() ([]byte, error) { return []byte("{oops"), nil }
	run = e.runSync(triggerManual)
	if run.Status != runPartial || run.Sources[1].Status != feedsync.StatusFailed || run.Stats.RestaurantsStale != 0 {
		t.Fatalf("invalid snapshot: %s %+v %+v", run.Status, run.Sources, run.Stats)
	}
}

func TestSyncSnapshotSourceHook(t *testing.T) {
	e := newSyncEnv(t)
	admin := e.admin("Root")
	src := e.expect(200, "POST", "/api/collections/sync_sources/records", admin.token, map[string]any{
		"provider": "ubereats-snapshot", "url": "https://ignored.example", "priority": 70, "enabled": true,
	}).m(t)
	if src["url"] != "" || src["label"] != "ubereats-snapshot" || src["city"] != "mons" {
		t.Fatalf("source %v", src)
	}
	e.expect(400, "POST", "/api/collections/sync_sources/records", admin.token, map[string]any{"provider": "ubereats-api"})
}

// The committed snapshot file must always be readable by the sync.
func TestEmbeddedUberEatsSnapshotIsValid(t *testing.T) {
	data, err := migrations.UberEatsSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	entries, problems, err := feedsync.ParseSnapshot(data, "mons")
	if err != nil || len(problems) > 0 {
		t.Fatalf("migrations/data/mons_ubereats.json: %v %v", err, problems)
	}
	t.Logf("%d restaurants Uber Eats dans l'instantané", len(entries))
}

func dbxName(restaurantID, name string) dbx.Expression {
	return dbx.HashExp{"restaurant": restaurantID, "name": name}
}

func dbxCat(categoryID string) dbx.Expression {
	return dbx.HashExp{"category": categoryID}
}
