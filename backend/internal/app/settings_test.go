package app

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
)

// assertItemsCounts checks restaurants.items_count against the menus.
func assertItemsCounts(t *testing.T, app core.App) {
	t.Helper()
	recs, err := app.FindAllRecords(colRestaurants)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		n, err := catalog.CountAvailableItems(app, r.Id)
		if err != nil {
			t.Fatal(err)
		}
		if got := r.GetInt(catalog.ItemsCountField); got != n {
			t.Fatalf("%s: items_count %d, %d available items", r.GetString("name"), got, n)
		}
	}
}

func (e *env) itemsCount(id string) int {
	e.t.Helper()
	r, err := e.app.FindRecordById(colRestaurants, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return r.GetInt(catalog.ItemsCountField)
}

func (e *env) nearbyIDs() []string {
	e.t.Helper()
	var out struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	e.expect(200, "GET", "/api/occ/restaurants/nearby?radiusKm=50", "", nil).json(e.t, &out)
	ids := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		ids = append(ids, it.ID)
	}
	return ids
}

func (e *env) settings(token string) adminSettingsResponse {
	e.t.Helper()
	var out adminSettingsResponse
	e.expect(200, "GET", "/api/occ/admin/settings", token, nil).json(e.t, &out)
	return out
}

func TestItemsCountBackfilledByMigration(t *testing.T) {
	// the template is migrated without the app hooks: only the migration
	// (and catalog.Import) can have filled the counts of the demo seed
	ta := newTestAppNoRegister(t)
	defer ta.Cleanup()
	assertItemsCounts(t, ta)
	r, err := ta.FindFirstRecordByData(colRestaurants, "slug", "la-bella-nonna")
	if err != nil {
		t.Fatal(err)
	}
	if r.GetInt(catalog.ItemsCountField) == 0 {
		t.Fatal("demo restaurant without items_count")
	}
	// demo install (no real data): filter off
	if got := minMenuItems(ta); got != 0 {
		t.Fatalf("demo threshold %d", got)
	}
}

func TestItemsCountFollowsMenuWrites(t *testing.T) {
	e := newEnv(t)
	pizza, burger := e.testRestaurants()
	// catalog.Import: unavailable items are not counted
	if e.itemsCount(pizza) != 2 || e.itemsCount(burger) != 1 {
		t.Fatalf("after import: pizza %d, burger %d", e.itemsCount(pizza), e.itemsCount(burger))
	}
	admin := e.admin("Root")

	// create / update (availability) / delete through the collections API
	it := e.expect(200, "POST", "/api/collections/menu_items/records", admin.token, map[string]any{
		"restaurant": burger, "name": "Double burger", "price": 1200, "available": true,
	}).m(t)
	if e.itemsCount(burger) != 2 {
		t.Fatalf("after create: %d", e.itemsCount(burger))
	}
	e.expect(200, "PATCH", "/api/collections/menu_items/records/"+it["id"].(string), admin.token, map[string]any{"available": false})
	if e.itemsCount(burger) != 1 {
		t.Fatalf("after unavailable: %d", e.itemsCount(burger))
	}
	e.expect(200, "PATCH", "/api/collections/menu_items/records/"+it["id"].(string), admin.token, map[string]any{"available": true})
	if e.itemsCount(burger) != 2 {
		t.Fatalf("after available: %d", e.itemsCount(burger))
	}
	e.expect(204, "DELETE", "/api/collections/menu_items/records/"+it["id"].(string), admin.token, nil)
	if e.itemsCount(burger) != 1 {
		t.Fatalf("after delete: %d", e.itemsCount(burger))
	}

	// the count is computed by the server, never taken from the client
	got := e.expect(200, "PATCH", "/api/collections/restaurants/records/"+burger, admin.token, map[string]any{
		"phone": "065 11 22 33", "items_count": 99,
	}).m(t)
	if got["items_count"] != float64(1) || e.itemsCount(burger) != 1 {
		t.Fatalf("items_count forged: %v", got["items_count"])
	}
	created := e.expect(200, "POST", "/api/collections/restaurants/records", admin.token, map[string]any{
		"name": "Nouveau", "slug": "nouveau", "active": true, "items_count": 42,
	}).m(t)
	if created["items_count"] != float64(0) {
		t.Fatalf("new restaurant items_count %v", created["items_count"])
	}

	// a re-import replaces the menu and recounts
	if _, _, err := catalog.Import(e.app, catalog.RestaurantImport{Slug: "test-burger", Name: "Test Burger",
		Categories: []catalog.CategoryImport{{Name: "Burgers", Items: []catalog.ItemImport{
			{Name: "A", Price: 100}, {Name: "B", Price: 100}, {Name: "C", Price: 100},
		}}}}); err != nil {
		t.Fatal(err)
	}
	if e.itemsCount(burger) != 3 {
		t.Fatalf("after re-import: %d", e.itemsCount(burger))
	}
	assertItemsCounts(t, e.app)
}

func TestSettingsAuthz(t *testing.T) {
	factory := func(t testing.TB) *tests.TestApp { return newTestApp(t) }
	scenarios := []tests.ApiScenario{
		{
			Name: "settings: anonymous", Method: http.MethodGet, URL: "/api/occ/admin/settings",
			ExpectedStatus: 401, ExpectedContent: []string{`"status":401`}, TestAppFactory: factory,
		},
		{
			Name: "settings: patch anonymous", Method: http.MethodPatch, URL: "/api/occ/admin/settings",
			Body: strings.NewReader(`{"minMenuItems":3}`), ExpectedStatus: 401, ExpectedContent: []string{`"status":401`},
			TestAppFactory: factory,
		},
	}
	for _, s := range scenarios {
		s.Test(t)
	}

	e := newEnv(t)
	admin, bob := e.admin("Root"), e.user("Bob")
	e.expect(403, "GET", "/api/occ/admin/settings", bob.token, nil)
	e.expect(403, "PATCH", "/api/occ/admin/settings", bob.token, map[string]any{"minMenuItems": 3})

	// superusers too
	suCol, err := e.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	su := core.NewRecord(suCol)
	su.SetEmail("su@example.com")
	su.SetPassword("password123456")
	if err := e.app.Save(su); err != nil {
		t.Fatal(err)
	}
	suTok, err := su.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	if got := e.settings(suTok); got.MinMenuItems != 0 || got.HiddenRestaurants != 0 || got.ActiveRestaurants == 0 {
		t.Fatalf("superuser settings %+v", got)
	}

	// validation (French messages)
	for _, body := range []map[string]any{{"minMenuItems": 101}, {"minMenuItems": -1}, {"minMenuItems": 2.5}, {}} {
		r := e.expect(400, "PATCH", "/api/occ/admin/settings", admin.token, body)
		if msg, _ := r.m(t)["message"].(string); !strings.Contains(msg, "plats") {
			t.Fatalf("%v: message %q", body, msg)
		}
	}

	// the collection: admins only, validated by the hook; no create / delete
	list := e.expect(200, "GET", "/api/collections/app_settings/records", bob.token, nil).m(t)
	if list["totalItems"] != float64(0) {
		t.Fatalf("non-admin must not read the settings: %v", list)
	}
	rows := e.expect(200, "GET", "/api/collections/app_settings/records", admin.token, nil).m(t)["items"].([]any)
	if len(rows) != 1 {
		t.Fatalf("settings rows %v", rows)
	}
	id := rows[0].(map[string]any)["id"].(string)
	e.expect(404, "PATCH", "/api/collections/app_settings/records/"+id, bob.token, map[string]any{"min_menu_items": 5})
	e.expect(400, "PATCH", "/api/collections/app_settings/records/"+id, admin.token, map[string]any{"min_menu_items": 500})
	e.expect(200, "PATCH", "/api/collections/app_settings/records/"+id, admin.token, map[string]any{"min_menu_items": 5})
	if minMenuItems(e.app) != 5 {
		t.Fatal("collection update ignored")
	}
	e.expect(403, "POST", "/api/collections/app_settings/records", admin.token, map[string]any{"min_menu_items": 1})
	e.expect(403, "DELETE", "/api/collections/app_settings/records/"+id, admin.token, nil)
}

func TestIncompleteMenusHiddenFromListings(t *testing.T) {
	e := newEnv(t)
	pizza, burger := e.testRestaurants() // 2 and 1 available items
	admin := e.admin("Root")
	alice, bob := e.user("Alice"), e.user("Bob")

	// filter off: everything listed; config says 0
	if ids := e.nearbyIDs(); !slices.Contains(ids, pizza) || !slices.Contains(ids, burger) {
		t.Fatal("filter off: both restaurants expected")
	}
	if cfg := e.expect(200, "GET", "/api/occ/config", "", nil).m(t); cfg["minMenuItems"] != float64(0) {
		t.Fatalf("config %v", cfg)
	}

	// a party picks the burger before the threshold changes
	party := e.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{
		"title": "Midi", "candidates": []string{pizza, burger},
	}).m(t)
	pid := party["id"].(string)
	code := party["code"].(string)
	e.expect(200, "POST", "/api/occ/parties/join", bob.token, map[string]any{"code": code})

	// threshold 2: the burger (1 item) disappears from the listings
	var got adminSettingsResponse
	e.expect(200, "PATCH", "/api/occ/admin/settings", admin.token, map[string]any{"minMenuItems": 2}).json(t, &got)
	if got.MinMenuItems != 2 || got.HiddenRestaurants != 1 {
		t.Fatalf("settings %+v", got)
	}
	ids := e.nearbyIDs()
	if !slices.Contains(ids, pizza) || slices.Contains(ids, burger) {
		t.Fatalf("threshold 2: %v", ids)
	}
	if cfg := e.expect(200, "GET", "/api/occ/config", "", nil).m(t); cfg["minMenuItems"] != float64(2) {
		t.Fatalf("config %v", cfg)
	}
	// still reachable by direct link, and by admins in the collection
	e.expect(200, "GET", "/api/collections/restaurants/records/"+burger, "", nil)
	adminList := e.expect(200, "GET", "/api/collections/restaurants/records?perPage=500", admin.token, nil)
	if !strings.Contains(string(adminList.body), burger) {
		t.Fatal("admins must see hidden restaurants")
	}

	// the party chosen before keeps working: vote then impose the burger
	e.expect(200, "POST", "/api/occ/parties/"+pid+"/transition", alice.token, map[string]any{"to": "voting"})
	e.expect(200, "POST", "/api/collections/votes/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "restaurant": burger})
	p := e.expect(200, "POST", "/api/occ/parties/"+pid+"/transition", alice.token, map[string]any{"to": "ordering", "restaurant": burger}).m(t)
	if p["party"].(map[string]any)["restaurant"] != burger {
		t.Fatalf("party %v", p)
	}
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": pid, "user": bob.id(), "menu_item": e.menuItem(burger, "Cheeseburger"), "quantity": 1,
	})

	// the menu grows → listed again automatically
	e.expect(200, "POST", "/api/collections/menu_items/records", admin.token, map[string]any{
		"restaurant": burger, "name": "Frites", "price": 350, "available": true,
	})
	if ids := e.nearbyIDs(); !slices.Contains(ids, burger) {
		t.Fatal("a completed menu must be listed again")
	}
	if got := e.settings(admin.token); got.HiddenRestaurants != 0 {
		t.Fatalf("hidden %+v", got)
	}

	// threshold 3 hides both; back to 0 shows everything (reversible)
	e.expect(200, "PATCH", "/api/occ/admin/settings", admin.token, map[string]any{"minMenuItems": 3})
	if ids := e.nearbyIDs(); slices.Contains(ids, pizza) || slices.Contains(ids, burger) {
		t.Fatalf("threshold 3: %v", ids)
	}
	e.expect(200, "PATCH", "/api/occ/admin/settings", admin.token, map[string]any{"minMenuItems": 0})
	if ids := e.nearbyIDs(); !slices.Contains(ids, pizza) || !slices.Contains(ids, burger) {
		t.Fatal("threshold 0 must show everything")
	}
}
