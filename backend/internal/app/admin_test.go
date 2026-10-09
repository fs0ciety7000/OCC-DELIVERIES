package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
)

// admin creates a user with the admin role.
func (e *env) admin(name string) user {
	e.t.Helper()
	u := e.user(name)
	u.rec.Set("role", roleAdmin)
	if err := e.app.Save(u.rec); err != nil {
		e.t.Fatal(err)
	}
	tok, err := u.rec.NewAuthToken()
	if err != nil {
		e.t.Fatal(err)
	}
	u.token = tok
	return u
}

func (e *env) role(id string) string {
	e.t.Helper()
	r, err := e.app.FindRecordById(colUsers, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return r.GetString("role")
}

func TestRoleBootstrap(t *testing.T) {
	cfg := testConfig
	cfg.AdminEmails = parseEmails(" Boss@Example.com ; chef@example.com,,nope ")
	if !reflect.DeepEqual(cfg.AdminEmails, []string{"boss@example.com", "chef@example.com"}) {
		t.Fatalf("parseEmails: %v", cfg.AdminEmails)
	}
	var bossID string
	e := newEnvWith(t, cfg, func(app *tests.TestApp) {
		col, _ := app.FindCollectionByNameOrId(colUsers)
		r := core.NewRecord(col)
		r.SetEmail("boss@example.com")
		r.SetPassword("password123")
		r.Set("name", "Boss")
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
		// simulate a user registered before OCC_ADMINS was set
		if _, err := app.DB().Update(colUsers, dbx.Params{"role": roleUser}, dbx.HashExp{"id": r.Id}).Execute(); err != nil {
			t.Fatal(err)
		}
		bossID = r.Id
	})
	if got := e.role(bossID); got != roleAdmin {
		t.Fatalf("existing bootstrap user promoted on serve: role=%q", got)
	}

	// registering with a bootstrap e-mail grants the role, others get "user"
	chef := e.expect(200, "POST", "/api/collections/users/records", "", map[string]any{
		"email": "Chef@example.com", "password": "password123", "passwordConfirm": "password123", "name": "Chef",
	}).m(t)
	if got := e.role(chef["id"].(string)); got != roleAdmin {
		t.Fatalf("chef role %q", got)
	}
	bob := e.expect(200, "POST", "/api/collections/users/records", "", map[string]any{
		"email": "bob@example.com", "password": "password123", "passwordConfirm": "password123", "name": "Bob",
	}).m(t)
	if got := e.role(bob["id"].(string)); got != roleUser {
		t.Fatalf("bob role %q", got)
	}
}

func TestRoleEscalationBlocked(t *testing.T) {
	e := newEnv(t)
	alice := e.user("Alice")
	if e.role(alice.id()) != roleUser {
		t.Fatal("default role must be user")
	}

	// self promotion through the collections API
	e.expect(403, "PATCH", "/api/collections/users/records/"+alice.id(), alice.token, map[string]any{"role": "admin"})
	e.expect(200, "PATCH", "/api/collections/users/records/"+alice.id(), alice.token, map[string]any{"name": "Alice B."})
	e.expect(403, "POST", "/api/collections/users/records", "", map[string]any{
		"email": "eve@example.com", "password": "password123", "passwordConfirm": "password123", "role": "admin",
	})
	if e.role(alice.id()) != roleUser {
		t.Fatal("alice escalated")
	}

	// admin endpoints
	e.expect(401, "GET", "/api/occ/admin/stats", "", nil)
	e.expect(403, "GET", "/api/occ/admin/stats", alice.token, nil)
	e.expect(403, "PATCH", "/api/occ/admin/users/"+alice.id()+"/role", alice.token, map[string]any{"role": "admin"})
	e.expect(403, "GET", "/api/occ/admin/export", alice.token, nil)
	e.expect(403, "POST", "/api/occ/admin/import?dryRun=1", alice.token, map[string]any{"slug": "x", "name": "X"})

	boss := e.admin("Boss")
	r := e.expect(200, "PATCH", "/api/occ/admin/users/"+alice.id()+"/role", boss.token, map[string]any{"role": "admin"}).m(t)
	if r["user"].(map[string]any)["role"] != "admin" || e.role(alice.id()) != roleAdmin {
		t.Fatalf("promote: %v", r)
	}
	e.expect(400, "PATCH", "/api/occ/admin/users/"+boss.id()+"/role", boss.token, map[string]any{"role": "user"})
	e.expect(400, "PATCH", "/api/occ/admin/users/"+alice.id()+"/role", boss.token, map[string]any{"role": "root"})
	e.expect(404, "PATCH", "/api/occ/admin/users/nope/role", boss.token, map[string]any{"role": "user"})
	e.expect(200, "PATCH", "/api/occ/admin/users/"+alice.id()+"/role", boss.token, map[string]any{"role": "user"})
	// other users' records stay owner-only in the collections API (roles go through the endpoint)
	e.expect(404, "PATCH", "/api/collections/users/records/"+alice.id(), boss.token, map[string]any{"role": "admin"})

	list := e.expect(200, "GET", "/api/occ/admin/users?q=alice", boss.token, nil).m(t)
	items := list["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["email"] != "alice@example.com" {
		t.Fatalf("users search: %v", list)
	}
	admins := e.expect(200, "GET", "/api/occ/admin/users?role=admin", boss.token, nil).m(t)
	if admins["totalItems"] != float64(1) {
		t.Fatalf("admins: %v", admins)
	}
}

func TestAdminRules(t *testing.T) {
	e := newEnv(t)
	pid, _, pizza, burger, _, _, carol := setupParty(t, e)
	boss := e.admin("Boss")

	// inactive restaurants: visible to admins only
	rest, _ := e.app.FindRecordById(colRestaurants, burger)
	rest.Set("active", false)
	if err := e.app.Save(rest); err != nil {
		t.Fatal(err)
	}
	e.expect(404, "GET", "/api/collections/restaurants/records/"+burger, carol.token, nil)
	e.expect(200, "GET", "/api/collections/restaurants/records/"+burger, boss.token, nil)

	// catalogue writes: admins only
	e.expect(400, "POST", "/api/collections/restaurants/records", carol.token, map[string]any{"name": "Pirate", "slug": "pirate"})
	e.expect(404, "PATCH", "/api/collections/restaurants/records/"+pizza, carol.token, map[string]any{"name": "Pirate"})
	e.expect(404, "DELETE", "/api/collections/restaurants/records/"+burger, carol.token, nil)
	e.expect(400, "POST", "/api/collections/menu_items/records", carol.token, map[string]any{"restaurant": pizza, "name": "x", "price": 100})

	nr := e.expect(200, "POST", "/api/collections/restaurants/records", boss.token, map[string]any{
		"name": " Nouveau ", "slug": "Nouveau-Resto", "active": true, "cuisines": []string{"Pizza", "pizza", " Italien "},
		"providers": []map[string]any{{"id": "ubereats", "url": "https://www.ubereats.com/be/store/x"}, {"id": "takeaway", "url": ""}},
	}).m(t)
	if nr["slug"] != "nouveau-resto" || nr["name"] != "Nouveau" || !reflect.DeepEqual(nr["cuisines"], []any{"pizza", "italien"}) || len(nr["providers"].([]any)) != 1 {
		t.Fatalf("restaurant normalized: %v", nr)
	}
	e.expect(400, "POST", "/api/collections/restaurants/records", boss.token, map[string]any{"name": "Doublon", "slug": "nouveau-resto"})
	e.expect(400, "POST", "/api/collections/restaurants/records", boss.token, map[string]any{"name": "X", "slug": "x-x", "providers": []map[string]any{{"id": "glovo", "url": "https://g"}}})
	// every delivery platform of providers.Platforms is accepted (Deliveroo, weloveat…)
	e.expect(200, "POST", "/api/collections/restaurants/records", boss.token, map[string]any{"name": "Y", "slug": "y-y", "providers": []map[string]any{{"id": "deliveroo", "url": "https://deliveroo.be/fr/menu/x"}}})

	cat := e.expect(200, "POST", "/api/collections/menu_categories/records", boss.token, map[string]any{"restaurant": nr["id"], "name": "Plats", "position": 0}).m(t)
	e.expect(200, "POST", "/api/collections/menu_items/records", boss.token, map[string]any{
		"restaurant": nr["id"], "category": cat["id"], "name": "Plat", "price": 1250, "available": true, "tags": []string{"Veggie"},
		"option_groups": []map[string]any{{"id": "s", "name": "Sauce", "min": 0, "max": 1, "choices": []map[string]any{{"id": "a", "name": "A", "price": 50}}}},
	})
	// invalid option groups / foreign category
	e.expect(400, "POST", "/api/collections/menu_items/records", boss.token, map[string]any{
		"restaurant": nr["id"], "name": "Bad", "price": 100,
		"option_groups": []map[string]any{{"id": "s", "name": "S", "min": 2, "max": 1, "choices": []map[string]any{{"id": "a", "name": "A"}}}},
	})
	pizzaCat, _ := e.app.FindFirstRecordByData(colMenuCategories, "restaurant", pizza)
	e.expect(400, "POST", "/api/collections/menu_items/records", boss.token, map[string]any{"restaurant": nr["id"], "category": pizzaCat.Id, "name": "X", "price": 100})

	// a restaurant used by a party cannot be deleted, an unused one can
	e.expect(400, "DELETE", "/api/collections/restaurants/records/"+pizza, boss.token, nil)
	e.expect(204, "DELETE", "/api/collections/restaurants/records/"+nr["id"].(string), boss.token, nil)

	// admins read every party and its records, but cannot write them
	e.expect(200, "GET", "/api/collections/parties/records/"+pid, boss.token, nil)
	for _, col := range []string{"party_members", "votes", "order_items", "payments"} {
		r := e.expect(200, "GET", "/api/collections/"+col+"/records?filter=party%3D%27"+pid+"%27", boss.token, nil)
		if col == "party_members" && !strings.Contains(string(r.body), `"totalItems":2`) {
			t.Fatalf("admin lists members: %s", r.body)
		}
		if r := e.expect(200, "GET", "/api/collections/"+col+"/records", carol.token, nil); !strings.Contains(string(r.body), `"totalItems":0`) {
			t.Fatalf("carol lists %s: %s", col, r.body)
		}
	}
	e.expect(404, "PATCH", "/api/collections/parties/records/"+pid, boss.token, map[string]any{"title": "hack"})
	e.expect(403, "GET", path("/api/occ/parties/%s/summary", pid), boss.token, nil)
}

func TestAdminStatsAndCancel(t *testing.T) {
	e := newEnv(t)
	pid, _, pizza, _, alice, bob, _ := setupParty(t, e)
	boss := e.admin("Boss")
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	tiramisu := e.menuItem(pizza, "Tiramisu")
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "menu_item": tiramisu, "quantity": 2})

	var stats struct {
		Users       int `json:"users"`
		Admins      int `json:"admins"`
		Restaurants struct{ Total, Active int }
		Parties     struct {
			Total    int            `json:"total"`
			ByStatus map[string]int `json:"byStatus"`
		} `json:"parties"`
		PartiesPerDay []dayCount      `json:"partiesPerDay"`
		OrderedTotal  int             `json:"orderedTotal"`
		Top           []topRestaurant `json:"topRestaurants"`
	}
	e.expect(200, "GET", "/api/occ/admin/stats", boss.token, nil).json(t, &stats)
	if stats.Users != 4 || stats.Admins != 1 || stats.Parties.Total != 1 || stats.Parties.ByStatus["ordering"] != 1 || stats.Parties.ByStatus["lobby"] != 0 {
		t.Fatalf("stats: %+v", stats)
	}
	if stats.Restaurants.Total < 2 || stats.Restaurants.Active > stats.Restaurants.Total {
		t.Fatalf("restaurants: %+v", stats.Restaurants)
	}
	if len(stats.PartiesPerDay) != 30 || stats.PartiesPerDay[29].Count != 1 {
		t.Fatalf("per day: %+v", stats.PartiesPerDay)
	}
	if stats.OrderedTotal != 1200 || len(stats.Top) != 1 || stats.Top[0].Slug != "test-pizza" || stats.Top[0].Parties != 1 || stats.Top[0].Amount != 1200 {
		t.Fatalf("totals: %d %+v", stats.OrderedTotal, stats.Top)
	}

	// force cancel
	e.expect(403, "POST", path("/api/occ/admin/parties/%s/cancel", pid), alice.token, nil)
	r := e.expect(200, "POST", path("/api/occ/admin/parties/%s/cancel", pid), boss.token, nil).m(t)
	if r["party"].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("cancel: %v", r)
	}
	e.expect(400, "POST", path("/api/occ/admin/parties/%s/cancel", pid), boss.token, nil)
	e.expect(404, "POST", "/api/occ/admin/parties/nope/cancel", boss.token, nil)
	e.expect(200, "GET", "/api/occ/admin/stats", boss.token, nil).json(t, &stats)
	if stats.OrderedTotal != 0 || stats.Parties.ByStatus["cancelled"] != 1 || len(stats.Top) != 0 {
		t.Fatalf("after cancel: %+v", stats)
	}
}

func csvUpload(t *testing.T, content string) (string, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "menu.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(content))
	_ = w.Close()
	return w.FormDataContentType(), &buf
}

func TestAdminImportCSV(t *testing.T) {
	e := newEnv(t)
	boss := e.admin("Boss")
	pizza, _ := e.testRestaurants()

	src := "restaurant_slug;restaurant_name;category;item_name;description;price_eur;tags;popular;address;lat;lng;cuisines;phone;ubereats_url;takeaway_url\n" +
		"chez-csv;Chez CSV;Plats;Boulets liégeois;Sauce lapin;12,50;;oui;Rue de la Clef 3, 7000 Mons;50,4531;3,9512;belge|brasserie;;;https://www.takeaway.com/be-fr/chez-csv\n" +
		"chez-csv;;Plats;Stoemp saucisse;;\"9,9\";veggie;;;;;;;;\n" +
		"chez-csv;;Desserts;Gaufre;;4 €;;x;;;;;;;\n" +
		// existing restaurant: only the menu changes, options of Margherita are kept
		"test-pizza;;Pizzas;Margherita;;11,00;;;;;;;;;\n"

	// dry run: report, nothing written
	ct, body := csvUpload(t, src)
	var dry struct {
		Report catalog.Report `json:"report"`
	}
	e.doRaw("POST", "/api/occ/admin/import/csv?dryRun=1", boss.token, ct, body).json(t, &dry)
	if !dry.Report.DryRun || !dry.Report.Valid || dry.Report.Items != 4 || len(dry.Report.Restaurants) != 2 {
		t.Fatalf("dry run: %+v", dry.Report)
	}
	if dry.Report.Restaurants[0].Exists || !dry.Report.Restaurants[1].Exists || dry.Report.Restaurants[0].Menu[1].Price != 990 {
		t.Fatalf("dry run details: %+v", dry.Report.Restaurants)
	}
	if _, err := e.app.FindFirstRecordByData(colRestaurants, "slug", "chez-csv"); err == nil {
		t.Fatal("dry run wrote data")
	}

	// real import
	ct, body = csvUpload(t, src)
	r := e.doRaw("POST", "/api/occ/admin/import/csv", boss.token, ct, body)
	if r.status != 200 {
		t.Fatalf("import: %d %s", r.status, r.body)
	}
	rest, err := e.app.FindFirstRecordByData(colRestaurants, "slug", "chez-csv")
	if err != nil {
		t.Fatal(err)
	}
	if rest.GetFloat("lat") != 50.4531 || rest.GetString("name") != "Chez CSV" || !rest.GetBool("active") {
		t.Fatalf("restaurant: %v", rest.PublicExport())
	}
	boulets, err := e.app.FindFirstRecordByFilter(colMenuItems, "restaurant = {:r} && name = 'Boulets liégeois'", dbx.Params{"r": rest.Id})
	if err != nil || boulets.GetInt("price") != 1250 || !boulets.GetBool("popular") || !boulets.GetBool("available") {
		t.Fatalf("boulets: %v %v", err, boulets)
	}
	marg, err := e.app.FindFirstRecordByFilter(colMenuItems, "restaurant = {:r} && name = 'Margherita'", dbx.Params{"r": pizza})
	if err != nil || marg.GetInt("price") != 1100 || !strings.Contains(marg.GetString("option_groups"), `"size"`) {
		t.Fatalf("margherita kept options: %v %v", err, marg)
	}
	if n, _ := e.app.CountRecords(colMenuItems, dbx.HashExp{"restaurant": pizza}); n != 1 {
		t.Fatalf("pizza menu replaced: %d items", n)
	}
	pr, _ := e.app.FindRecordById(colRestaurants, pizza)
	if pr.GetString("phone") != "065 00 00 00" || pr.GetFloat("lat") != 50.4542 {
		t.Fatalf("existing metadata kept: %v", pr.PublicExport())
	}

	// errors: 400 with report, nothing written; dry run answers 200 valid=false
	bad := "restaurant_slug,restaurant_name,category,item_name,description,price_eur,tags,popular\n" +
		"nouveau,Nouveau,Plats,Plat,,\"12,505\",,\n" +
		"sans-nom,,Plats,Plat,,3,,\n"
	ct, body = csvUpload(t, bad)
	r = e.doRaw("POST", "/api/occ/admin/import/csv", boss.token, ct, body)
	var badResp struct {
		Message string         `json:"message"`
		Report  catalog.Report `json:"report"`
	}
	r.json(t, &badResp)
	if r.status != 400 || badResp.Report.Valid || badResp.Report.ErrorCount() != 2 || !strings.Contains(badResp.Message, "Rien n'a été modifié") {
		t.Fatalf("bad import: %d %s", r.status, r.body)
	}
	if _, err := e.app.FindFirstRecordByData(colRestaurants, "slug", "sans-nom"); err == nil {
		t.Fatal("invalid batch partially written")
	}
	ct, body = csvUpload(t, bad)
	r = e.doRaw("POST", "/api/occ/admin/import/csv?dryRun=1", boss.token, ct, body)
	if r.status != 200 || !strings.Contains(string(r.body), `"valid":false`) {
		t.Fatalf("bad dry run: %d %s", r.status, r.body)
	}
	e.expect(400, "POST", "/api/occ/admin/import/csv", boss.token, nil)
}

func TestAdminExportImportRoundtrip(t *testing.T) {
	e := newEnv(t)
	boss := e.admin("Boss")
	e.testRestaurants()

	first := e.expect(200, "GET", "/api/occ/admin/export", boss.token, nil)
	if !strings.Contains(first.header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("disposition: %v", first.header)
	}
	var exported []catalog.RestaurantImport
	first.json(t, &exported)
	if len(exported) < 2 {
		t.Fatalf("export: %d restaurants", len(exported))
	}

	// dry run of the whole catalogue, then import it back (array body)
	var dry struct {
		Report catalog.Report `json:"report"`
	}
	e.expect(200, "POST", "/api/occ/admin/import?dryRun=1", boss.token, exported).json(t, &dry)
	if !dry.Report.Valid || len(dry.Report.Restaurants) != len(exported) {
		t.Fatalf("dry run: %+v", dry.Report)
	}
	for _, rr := range dry.Report.Restaurants {
		if !rr.Exists {
			t.Fatalf("%s should exist", rr.Slug)
		}
	}
	e.expect(200, "POST", "/api/occ/admin/import", boss.token, exported)

	var again []catalog.RestaurantImport
	e.expect(200, "GET", "/api/occ/admin/export", boss.token, nil).json(t, &again)
	a, _ := json.Marshal(exported)
	b, _ := json.Marshal(again)
	if !bytes.Equal(a, b) {
		t.Fatalf("roundtrip differs:\n%s\n%s", a, b)
	}

	// an inactive restaurant stays inactive through the roundtrip
	no := false
	exported[0].Active = &no
	e.expect(200, "POST", "/api/occ/admin/import", boss.token, exported[0])
	r, _ := e.app.FindFirstRecordByData(colRestaurants, "slug", exported[0].Slug)
	if r.GetBool("active") {
		t.Fatal("active flag not imported")
	}

	// invalid entry in a batch: nothing written
	batch := []map[string]any{{"slug": "ok-resto", "name": "OK"}, {"slug": "Bad Slug", "name": ""}}
	r2 := e.expect(400, "POST", "/api/occ/admin/import", boss.token, batch)
	if !strings.Contains(string(r2.body), `"report"`) {
		t.Fatalf("no report: %s", r2.body)
	}
	if _, err := e.app.FindFirstRecordByData(colRestaurants, "slug", "ok-resto"); err == nil {
		t.Fatal("partial import")
	}
	e.expect(400, "POST", "/api/occ/admin/import", boss.token, []map[string]any{{"slug": "dup", "name": "A"}, {"slug": "dup", "name": "B"}})
	e.expect(400, "POST", "/api/occ/admin/import", boss.token, nil)
}

// TestAdminImportBookmarkletPayload covers the /outils/export-menu.html output:
// one object, extra keys, no coordinates / address, zero fees.
func TestAdminImportBookmarkletPayload(t *testing.T) {
	e := newEnv(t)
	boss := e.admin("Boss")
	payload := `{"slug":"chez-uber","name":"Chez Uber","description":"","emoji":"🍔","cover_url":"","cuisines":["burger"],
		"address":"","lat":0,"lng":0,"phone":"","rating":0,"rating_count":0,"price_level":2,"eta_min":25,"eta_max":45,
		"delivery_fee":0,"min_order":0,"providers":[{"id":"ubereats","url":"https://www.ubereats.com/be/store/chez-uber/abc"}],
		"active":true,"categories":[{"name":"Burgers","items":[{"name":"Cheese","description":"","price":1190,"tags":[],"option_groups":[],"popular":false}]}],
		"source_urls":["https://www.ubereats.com/be/store/chez-uber/abc"],"menu_checked_at":"2026-10-09"}`

	r := e.doRaw("POST", "/api/occ/admin/import?dryRun=1", boss.token, "application/json", strings.NewReader(payload))
	var dry struct {
		Report catalog.Report `json:"report"`
	}
	r.json(t, &dry)
	if r.status != 200 || !dry.Report.Valid || len(dry.Report.Restaurants) != 1 {
		t.Fatalf("dry run: %d %s", r.status, r.body)
	}
	if w := strings.Join(dry.Report.Restaurants[0].Warnings, " | "); !strings.Contains(w, "Coordonnées manquantes") {
		t.Fatalf("warnings: %q", w)
	}
	if r := e.doRaw("POST", "/api/occ/admin/import", boss.token, "application/json", strings.NewReader(payload)); r.status != 200 {
		t.Fatalf("import: %d %s", r.status, r.body)
	}

	// once geocoded in /admin, a fresh export from the tool keeps the coordinates
	rest, _ := e.app.FindFirstRecordByData(colRestaurants, "slug", "chez-uber")
	rest.Set("lat", 50.45)
	rest.Set("lng", 3.95)
	rest.Set("address", "Grand-Place 1, 7000 Mons")
	if err := e.app.Save(rest); err != nil {
		t.Fatal(err)
	}
	e.doRaw("POST", "/api/occ/admin/import?dryRun=1", boss.token, "application/json", strings.NewReader(payload)).json(t, &dry)
	if w := strings.Join(dry.Report.Restaurants[0].Warnings, " | "); !strings.Contains(w, "conservées") {
		t.Fatalf("warnings (existing): %q", w)
	}
	if r := e.doRaw("POST", "/api/occ/admin/import", boss.token, "application/json", strings.NewReader(payload)); r.status != 200 {
		t.Fatalf("re-import: %d %s", r.status, r.body)
	}
	rest, _ = e.app.FindFirstRecordByData(colRestaurants, "slug", "chez-uber")
	if rest.GetFloat("lat") != 50.45 || rest.GetString("address") != "Grand-Place 1, 7000 Mons" {
		t.Fatalf("coordinates lost: %v", rest.PublicExport())
	}
}

// The export tool names restaurants after the platform page ("Pizza Hut - Mons"
// → slug "pizza-hut-mons"): importing it must complete the catalogue entry
// (matched by platform link or name), not create a duplicate.
func TestAdminImportMatchesExistingByLinkOrName(t *testing.T) {
	e := newEnv(t)
	boss := e.admin("Boss")
	base := `{"slug":"pizza-hut","name":"Pizza Hut","emoji":"🍕","cuisines":["pizza"],"lat":50.4535,"lng":3.9438,
		"address":"Place Léopold 7, 7000 Mons","rating":4.5,"rating_count":1000,"eta_min":12,"eta_max":27,"partial_menu":true,
		"providers":[{"id":"ubereats","url":"https://www.ubereats.com/store/pizza-hut-mons/ISVg"}],
		"categories":[{"name":"Aperçu","items":[{"name":"The BOX","price":2800}]}]}`
	if r := e.doRaw("POST", "/api/occ/admin/import", boss.token, "application/json", strings.NewReader(base)); r.status != 200 {
		t.Fatalf("seed: %d %s", r.status, r.body)
	}
	items := `[` + strings.TrimSuffix(strings.Repeat(`{"name":"P%d","price":1200},`, 6), ",") + `]`
	for i := 1; i <= 6; i++ {
		items = strings.Replace(items, "%d", fmt.Sprint(i), 1)
	}
	// matched by link (other slug, query string, "be/" path variant ignored only by host+path)
	full := `{"slug":"pizza-hut-mons","name":"Pizza Hut - Mons","lat":0,"lng":0,"address":"",
		"providers":[{"id":"ubereats","url":"https://www.ubereats.com/store/pizza-hut-mons/ISVg?diningMode=DELIVERY"}],
		"categories":[{"name":"Pizzas","items":` + items + `}]}`
	if r := e.doRaw("POST", "/api/occ/admin/import", boss.token, "application/json", strings.NewReader(full)); r.status != 200 {
		t.Fatalf("import: %d %s", r.status, r.body)
	}
	all, _ := e.app.FindAllRecords(colRestaurants)
	n := 0
	for _, r := range all {
		if strings.HasPrefix(r.GetString("slug"), "pizza-hut") {
			n++
		}
	}
	rest, err := e.app.FindFirstRecordByData(colRestaurants, "slug", "pizza-hut")
	if n != 1 || err != nil {
		t.Fatalf("expected one Pizza Hut, got %d (%v)", n, err)
	}
	if rest.GetBool("partial_menu") || rest.GetFloat("rating") != 4.5 || rest.GetFloat("lat") != 50.4535 {
		t.Fatalf("existing fields lost or preview kept: %v", rest.PublicExport())
	}
	// matched by name when the payload has no link
	byName := `{"slug":"pizzahut","name":"PIZZA HUT (Mons)","categories":[{"name":"Pizzas","items":[{"name":"P1","price":1200}]}]}`
	if r := e.doRaw("POST", "/api/occ/admin/import", boss.token, "application/json", strings.NewReader(byName)); r.status != 200 {
		t.Fatalf("by name: %d %s", r.status, r.body)
	}
	if _, err := e.app.FindFirstRecordByData(colRestaurants, "slug", "pizzahut"); err == nil {
		t.Fatal("name match must not create a duplicate")
	}
}
