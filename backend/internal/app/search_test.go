package app

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/search"
)

type searchResp struct {
	Query       string   `json:"query"`
	Terms       []string `json:"terms"`
	Fuzzy       bool     `json:"fuzzy"`
	Restaurants []struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		Emoji      string   `json:"emoji"`
		Cuisines   []string `json:"cuisines"`
		ItemsCount int      `json:"itemsCount"`
		DistanceKm *float64 `json:"distanceKm"`
	} `json:"restaurants"`
	Dishes []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Price      int    `json:"price"`
		Snippet    string `json:"snippet"`
		Restaurant struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"restaurant"`
	} `json:"dishes"`
	People []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		SharedParties int    `json:"sharedParties"`
		RecentParties []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"recentParties"`
	} `json:"people"`
	Actions []struct {
		ID   string `json:"id"`
		Href string `json:"href"`
	} `json:"actions"`
}

func (e *env) search(q, token string, extra ...string) searchResp {
	e.t.Helper()
	u := "/api/occ/search?q=" + url.QueryEscape(q)
	for _, x := range extra {
		u += "&" + x
	}
	var out searchResp
	e.expect(200, "GET", u, token, nil).json(e.t, &out)
	return out
}

func (s searchResp) restaurantNames() []string {
	out := []string{}
	for _, r := range s.Restaurants {
		out = append(out, r.Name)
	}
	return out
}

func (s searchResp) dishNames() []string {
	out := []string{}
	for _, d := range s.Dishes {
		out = append(out, d.Name)
	}
	return out
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// importRamen adds a deterministic ramen restaurant (absent from the demo seed).
func (e *env) importRamen() string {
	e.t.Helper()
	id, _, err := catalog.Import(e.app, catalog.RestaurantImport{
		Slug: "tomo-test", Name: "Tomo Râmen", Emoji: "🍜", Cuisines: []string{"ramen", "japonais"},
		Address: "Rue de Nimy 12, 7000 Mons", Lat: 50.4560, Lng: 3.9530,
		Categories: []catalog.CategoryImport{
			{Name: "Bols", Items: []catalog.ItemImport{
				{Name: "Râmen miso", Price: 1450, Description: "Bouillon de porc mijoté douze heures, nouilles fraîches, œuf mariné et oignons nouveaux."},
				{Name: "Shoyu", Price: 1350, Description: "Le classique au soja."},
				{Name: "Karaage", Price: 650, Available: boolPtr(false)},
			}},
			{Name: "Desserts", Items: []catalog.ItemImport{{Name: "Mochi", Price: 450}}},
		},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestSearchAccentsPrefixAndTypos(t *testing.T) {
	e := newEnv(t)
	tomo := e.importRamen()

	for _, q := range []string{"ramen", "râmen", "RAM", "Ramen", "rAmÊn"} {
		res := e.search(q, "")
		if !has(res.restaurantNames(), "Tomo Râmen") {
			t.Errorf("%q: restaurant not found: %v", q, res.restaurantNames())
		}
		if !has(res.dishNames(), "Râmen miso") {
			t.Errorf("%q: dish not found: %v", q, res.dishNames())
		}
		if res.Fuzzy {
			t.Errorf("%q: exact query marked fuzzy", q)
		}
	}

	// exact restaurant name ranks first, dish carries its restaurant
	res := e.search("tomo ramen", "")
	if len(res.Restaurants) == 0 || res.Restaurants[0].ID != tomo || res.Restaurants[0].Emoji != "🍜" {
		t.Fatalf("exact name not first: %+v", res.Restaurants)
	}
	if res.Restaurants[0].ItemsCount != 3 {
		t.Fatalf("itemsCount %d (unavailable counted?)", res.Restaurants[0].ItemsCount)
	}

	// typo tolerance
	res = e.search("ramne", "")
	if !res.Fuzzy || !has(res.dishNames(), "Râmen miso") || !has(res.Terms, "ramen") {
		t.Fatalf("typo not corrected: %+v", res)
	}

	// the best correction ranks first: « piza » → « pizza » (frequent) before « pita » (rarer, higher bm25)
	if _, _, err := catalog.Import(e.app, catalog.RestaurantImport{
		Slug: "snack-test", Name: "Snack Test",
		Categories: []catalog.CategoryImport{{Name: "Snacks", Items: []catalog.ItemImport{
			{Name: "Pita poulet", Price: 800}, {Name: "Pizza truffe", Price: 1600},
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	res = e.search("piza", "")
	if !res.Fuzzy || len(res.Dishes) == 0 || res.Dishes[0].Name != "Pizza truffe" || res.Terms[0] != "pizza" {
		t.Fatalf("best correction not first: %v %v", res.Terms, res.dishNames())
	}

	// description match: snippet keeps the accents around the match
	res = e.search("oeuf marine", "")
	if len(res.Dishes) != 1 || res.Dishes[0].Name != "Râmen miso" || !strings.Contains(res.Dishes[0].Snippet, "œuf mariné") {
		t.Fatalf("description search: %+v", res.Dishes)
	}
	if res.Dishes[0].Restaurant.ID != tomo || res.Dishes[0].Price != 1450 {
		t.Fatalf("dish payload: %+v", res.Dishes[0])
	}

	// category match
	if res := e.search("dessert", ""); !has(res.dishNames(), "Mochi") {
		t.Fatalf("category search: %v", res.dishNames())
	}

	// the restaurant name alone does not list its whole menu
	if res := e.search("tomo", ""); len(res.Dishes) != 0 {
		t.Fatalf("restaurant name flooded the dishes: %v", res.dishNames())
	}
	// but narrows a dish search
	if res := e.search("tomo shoyu", ""); len(res.Dishes) != 1 || res.Dishes[0].Name != "Shoyu" {
		t.Fatalf("restaurant + dish: %v", res.dishNames())
	}

	// cuisine and address
	if res := e.search("japonais", ""); !has(res.restaurantNames(), "Tomo Râmen") {
		t.Fatalf("cuisine search: %v", res.restaurantNames())
	}
	if res := e.search("nimy", ""); !has(res.restaurantNames(), "Tomo Râmen") {
		t.Fatalf("address search: %v", res.restaurantNames())
	}

	// distance only when a position is given
	if res := e.search("tomo", ""); res.Restaurants[0].DistanceKm != nil {
		t.Fatal("distance without position")
	}
	if res := e.search("tomo", "", "lat=50.4542", "lng=3.9567"); res.Restaurants[0].DistanceKm == nil || *res.Restaurants[0].DistanceKm <= 0 {
		t.Fatalf("distance missing: %+v", res.Restaurants[0])
	}

	// FTS syntax in the query is neutralised
	for _, q := range []string{`"ramen`, `ramen*`, `NEAR(ramen`, `{name}: ramen`, `ramen OR`, `-ramen`, `'`, `^`} {
		e.search(q, "")
	}
	// empty / one letter: only the actions
	if res := e.search("", ""); len(res.Restaurants)+len(res.Dishes) != 0 || len(res.Actions) == 0 {
		t.Fatalf("empty query: %+v", res)
	}
	if res := e.search("r", ""); len(res.Restaurants)+len(res.Dishes) != 0 {
		t.Fatalf("one letter: %+v", res)
	}
	e.expect(400, "GET", "/api/occ/search?q="+strings.Repeat("a", 201), "", nil)

	// limit
	if res := e.search("pizza", "", "limit=1"); len(res.Dishes) > 1 || len(res.Restaurants) > 1 {
		t.Fatalf("limit ignored: %d dishes", len(res.Dishes))
	}
}

func TestSearchVisibility(t *testing.T) {
	e := newEnv(t)
	tomo := e.importRamen()

	// unavailable dish never listed
	if res := e.search("karaage", ""); len(res.Dishes) != 0 {
		t.Fatalf("unavailable dish listed: %v", res.dishNames())
	}

	// incomplete menus hidden (same rule as /nearby), dishes included
	settings, err := settingsRecord(e.app)
	if err != nil || settings == nil {
		t.Fatal("settings", err)
	}
	settings.Set("min_menu_items", 4)
	if err := e.app.Save(settings); err != nil {
		t.Fatal(err)
	}
	res := e.search("ramen", "")
	if has(res.restaurantNames(), "Tomo Râmen") || len(res.Dishes) != 0 {
		t.Fatalf("incomplete menu listed: %v %v", res.restaurantNames(), res.dishNames())
	}
	settings.Set("min_menu_items", 3)
	if err := e.app.Save(settings); err != nil {
		t.Fatal(err)
	}
	if res := e.search("ramen", ""); !has(res.restaurantNames(), "Tomo Râmen") {
		t.Fatalf("restaurant not back with a lower threshold: %v", res.restaurantNames())
	}

	// inactive restaurant hidden, with its dishes
	rec, _ := e.app.FindRecordById(colRestaurants, tomo)
	rec.Set("active", false)
	if err := e.app.Save(rec); err != nil {
		t.Fatal(err)
	}
	if res := e.search("ramen", ""); len(res.Restaurants) != 0 || len(res.Dishes) != 0 {
		t.Fatalf("inactive restaurant listed: %+v", res)
	}
}

func TestSearchReindex(t *testing.T) {
	e := newEnv(t)
	tomo := e.importRamen()
	admin := e.admin("Ada")

	// re-import replaces the menu: the index follows (catalog.Import → search.Reindex)
	if _, _, err := catalog.Import(e.app, catalog.RestaurantImport{
		Slug: "tomo-test", Name: "Tomo Râmen",
		Categories: []catalog.CategoryImport{{Name: "Bols", Items: []catalog.ItemImport{
			{Name: "Tantanmen épicé", Price: 1550}, {Name: "Râmen miso", Price: 1450}, {Name: "Shoyu", Price: 1350}, {Name: "Mochi", Price: 450},
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	if res := e.search("tantan", ""); !has(res.dishNames(), "Tantanmen épicé") {
		t.Fatalf("imported dish not indexed: %v", res.dishNames())
	}
	if res := e.search("karaage", ""); len(res.Dishes) != 0 {
		t.Fatalf("removed dish still indexed: %v", res.dishNames())
	}

	// admin edits through the collections API (record hooks)
	item := e.menuItem(tomo, "Shoyu")
	e.expect(200, "PATCH", "/api/collections/menu_items/records/"+item, admin.token, map[string]any{"name": "Shio paitan"})
	if res := e.search("paitan", ""); !has(res.dishNames(), "Shio paitan") {
		t.Fatalf("renamed dish not indexed: %v", res.dishNames())
	}
	if res := e.search("shoyu", ""); has(res.dishNames(), "Shoyu") {
		t.Fatal("old name still indexed")
	}
	e.expect(204, "DELETE", "/api/collections/menu_items/records/"+item, admin.token, nil)
	if res := e.search("paitan", ""); len(res.Dishes) != 0 {
		t.Fatalf("deleted dish still indexed: %v", res.dishNames())
	}

	// renaming the restaurant re-indexes its row and its dishes
	e.expect(200, "PATCH", "/api/collections/restaurants/records/"+tomo, admin.token, map[string]any{"name": "Kazoku Ramen"})
	if res := e.search("kazoku", ""); !has(res.restaurantNames(), "Kazoku Ramen") {
		t.Fatalf("renamed restaurant: %v", res.restaurantNames())
	}
	if res := e.search("kazoku mochi", ""); !has(res.dishNames(), "Mochi") {
		t.Fatalf("dish not re-indexed with the restaurant name: %v", res.dishNames())
	}

	// renaming a category re-indexes its dishes
	cat, err := e.app.FindFirstRecordByFilter(colMenuCategories, "restaurant = {:r}", map[string]any{"r": tomo})
	if err != nil {
		t.Fatal(err)
	}
	e.expect(200, "PATCH", "/api/collections/menu_categories/records/"+cat.Id, admin.token, map[string]any{"name": "Soupes de nouilles"})
	if res := e.search("nouilles", ""); len(res.Dishes) == 0 {
		t.Fatal("category rename not indexed")
	}

	// new restaurant created by an admin
	e.expect(200, "POST", "/api/collections/restaurants/records", admin.token, map[string]any{
		"name": "Zébulon Grill", "slug": "zebulon", "active": true, "cuisines": []string{"grill"},
	})
	if res := e.search("zebulon", ""); !has(res.restaurantNames(), "Zébulon Grill") {
		t.Fatalf("created restaurant: %v", res.restaurantNames())
	}

	// the counts match the catalogue; a drifted index is rebuilt
	ri, ii, err := search.Counts(e.app)
	if err != nil {
		t.Fatal(err)
	}
	rc, _ := e.app.CountRecords(colRestaurants)
	ic, _ := e.app.CountRecords(colMenuItems)
	if ri != int(rc) || ii != int(ic) {
		t.Fatalf("index %d/%d, catalogue %d/%d", ri, ii, rc, ic)
	}
	if _, err := e.app.DB().NewQuery("DELETE FROM " + search.TableItemDocs).Execute(); err != nil {
		t.Fatal(err)
	}
	if err := search.RebuildIfDrifted(e.app); err != nil {
		t.Fatal(err)
	}
	if _, ii, _ := search.Counts(e.app); ii != int(ic) {
		t.Fatalf("not rebuilt: %d", ii)
	}
	if res := e.search("mochi", ""); len(res.Dishes) == 0 {
		t.Fatal("rebuilt index empty")
	}
}

func TestSearchPeoplePrivacy(t *testing.T) {
	e := newEnv(t)
	_, _, _, _, alice, bob, carol := setupParty(t, e)
	dave := e.user("Dave")
	dave.rec.Set("name", "Dave Dupont")
	if err := e.app.Save(dave.rec); err != nil {
		t.Fatal(err)
	}
	// carol and dave are in another party together, without alice
	p := e.expect(200, "POST", "/api/collections/parties/records", carol.token, map[string]any{"title": "Autre"}).m(t)
	e.expect(200, "POST", "/api/occ/parties/join", dave.token, map[string]any{"code": p["code"]})

	// anonymous: never any person
	if res := e.search("bob", ""); len(res.People) != 0 {
		t.Fatalf("people leaked to anonymous: %+v", res.People)
	}
	// alice finds bob (shared party) with the shared party
	res := e.search("bo", alice.token)
	if len(res.People) != 1 || res.People[0].ID != bob.id() || res.People[0].SharedParties != 1 ||
		len(res.People[0].RecentParties) != 1 || res.People[0].RecentParties[0].Title != "Test" {
		t.Fatalf("colleague: %+v", res.People)
	}
	// but neither carol nor dave (no shared party), nor herself
	for _, q := range []string{"carol", "dave", "dupont", "alice"} {
		if res := e.search(q, alice.token); len(res.People) != 0 {
			t.Fatalf("%q leaked: %+v", q, res.People)
		}
	}
	// carol finds dave, accent / case-insensitive, by any word of the name
	if res := e.search("DUPÔNT", carol.token); len(res.People) != 1 || res.People[0].ID != dave.id() {
		t.Fatalf("carol → dave: %+v", res.People)
	}
	// e-mail is never searched
	if res := e.search("example", alice.token); len(res.People) != 0 {
		t.Fatal("e-mail matched")
	}
	// suspended / deleted colleagues are not suggested
	bob.rec.Set("deleted_at", time.Now().UTC())
	if err := e.app.Save(bob.rec); err != nil {
		t.Fatal(err)
	}
	if res := e.search("bob", alice.token); len(res.People) != 0 {
		t.Fatalf("deleted account suggested: %+v", res.People)
	}

	// actions: « Mes commandes » for users, « Admin » for admins only
	ids := func(r searchResp) []string {
		out := []string{}
		for _, a := range r.Actions {
			out = append(out, a.ID)
		}
		return out
	}
	if got := ids(e.search("", "")); has(got, "my-orders") || has(got, "admin") {
		t.Fatalf("anonymous actions %v", got)
	}
	if got := ids(e.search("", alice.token)); !has(got, "my-orders") || has(got, "admin") {
		t.Fatalf("user actions %v", got)
	}
	if got := ids(e.search("", e.admin("Ada").token)); !has(got, "admin") {
		t.Fatalf("admin actions %v", got)
	}
}

// TestSearchTeamColleagues: members of a common team are colleagues even
// without a shared party (skipped while the teams collection is absent).
func TestSearchTeamColleagues(t *testing.T) {
	e := newEnv(t)
	teams, err := e.app.FindCollectionByNameOrId("teams")
	if err != nil {
		t.Skip("teams collection absent")
	}
	alice, erin, zoe := e.user("Alice"), e.user("Erin"), e.user("Zoé")
	team := core.NewRecord(teams)
	team.Load(map[string]any{"name": "Compta", "code": "ABCDEFGH", "owner": alice.id(), "members": []string{alice.id(), erin.id()}})
	if err := e.app.Save(team); err != nil {
		t.Skipf("team schema changed: %v", err)
	}
	if res := e.search("erin", alice.token); len(res.People) != 1 || res.People[0].SharedParties != 0 {
		t.Fatalf("team colleague: %+v", res.People)
	}
	if res := e.search("alice", erin.token); len(res.People) != 1 {
		t.Fatalf("team owner: %+v", res.People)
	}
	if res := e.search("zoe", alice.token); len(res.People) != 0 {
		t.Fatalf("outsider leaked: %+v", res.People)
	}
	_ = zoe
}
