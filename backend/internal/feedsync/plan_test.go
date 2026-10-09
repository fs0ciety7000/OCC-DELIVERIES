package feedsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

var now = time.Date(2026, 10, 9, 3, 30, 0, 0, time.UTC)

func opts() Options { return Options{Now: now, City: "mons", Incomplete: map[string]bool{}} }

func boolp(b bool) *bool { return &b }

// feedTomo is the Tomo mini-site as read by menusync.
func feedTomo(items ...catalog.ItemImport) menusync.Restaurant {
	r := menusync.Restaurant{Source: menusync.SourceTakeawaySite, SourceURLs: []string{"https://www.tomomons.be/"}}
	r.Name, r.Address, r.Lat, r.Lng = "Tomo", "11 Rue d’Enghien, 7000 Mons", 50.4543472, 3.9512159
	r.Phone, r.Rating, r.RatingCount = "", 4.7, 210
	r.Cuisines = []string{"Japonais", "ramen"}
	r.Providers = []providers.Link{{ID: providers.Takeaway, URL: "https://www.takeaway.com/be/menu/tomo-mons"}}
	r.Origins = []menusync.Origin{{Source: menusync.SourceTakeawaySite, URL: "https://www.tomomons.be/", CheckedAt: "2026-10-09"}}
	r.Categories = []catalog.CategoryImport{{Name: "Ramen", Items: items}}
	return r
}

func item(name string, price int) catalog.ItemImport {
	return catalog.ItemImport{Name: name, Price: price}
}

// curatedTomo is the hand-made record already in production.
func curatedTomo() *Restaurant {
	return &Restaurant{
		ID: "r_tomo", Slug: "tomo-mons", Name: "Tomo", Emoji: "🍜", Address: "Rue d'Enghien 11, 7000 Mons",
		Lat: 50.45436, Lng: 3.95122, Phone: "065 35 29 64", Rating: 4.5, RatingCount: 100, PriceLevel: 2,
		Cuisines:   []string{"japonais", "ramen"},
		Providers:  []providers.Link{{ID: providers.Takeaway, URL: "https://www.takeaway.com/be/menu/tomo-mons"}},
		Active:     true,
		Categories: []*Category{{ID: "c_ramen", Name: "Ramen", Position: 0}},
		Items: []*Item{
			{ID: "i_miso", CategoryID: "c_ramen", Name: "Miso ramen", Price: 1450, Available: true, OptionGroups: []domain.OptionGroup{}},
			{ID: "i_shio", CategoryID: "c_ramen", Name: "Shio ramen", Price: 1400, Available: true},
		},
	}
}

// applySeq numbers the invented ids (unique across apply calls).
var applySeq int

// apply simulates the database writes of a plan (ids are invented).
func apply(t *testing.T, db []*Restaurant, p Plan) []*Restaurant {
	t.Helper()
	byID := map[string]*Restaurant{}
	for _, r := range db {
		byID[r.ID] = r
	}
	n := &applySeq
	for _, rp := range p.Restaurants {
		r := rp.Restaurant
		cur := byID[r.ID]
		if r.ID == "" {
			*n++
			r.ID = fmt.Sprintf("r_new%d", *n)
			cur = &Restaurant{}
			db = append(db, cur)
			byID[r.ID] = cur
		}
		cats, items := cur.Categories, cur.Items
		*cur = *r
		cur.Categories, cur.Items = cats, items
		ids := map[string]string{}
		for _, c := range rp.NewCategories {
			*n++
			cc := *c
			cc.ID = fmt.Sprintf("c_new%d", *n)
			ids[c.Name] = cc.ID
			cur.Categories = append(cur.Categories, &cc)
		}
		for _, it := range rp.Items {
			cp := *it
			if cp.CategoryID == "" {
				cp.CategoryID = ids[cp.CategoryName]
				if cp.CategoryID == "" {
					t.Fatalf("item %s: unknown category %q", cp.Name, cp.CategoryName)
				}
			}
			if cp.ID == "" {
				*n++
				cp.ID = fmt.Sprintf("i_new%d", *n)
				cur.Items = append(cur.Items, &cp)
				continue
			}
			i := slices.IndexFunc(cur.Items, func(x *Item) bool { return x.ID == cp.ID })
			if i < 0 {
				t.Fatalf("update of unknown item %s", cp.ID)
			}
			cur.Items[i] = &cp
		}
	}
	return db
}

func total(p Plan) Stats {
	var s Stats
	for _, rp := range p.Restaurants {
		s.Add(rp.Stats)
	}
	return s
}

func changes(p Plan) string {
	var out []string
	for _, rp := range p.Restaurants {
		out = append(out, rp.Changes...)
	}
	return strings.Join(out, "\n")
}

func findItem(r *Restaurant, name string) *Item {
	for _, it := range r.Items {
		if it.Name == name {
			return it
		}
	}
	return nil
}

func TestReconcileCuratedRestaurant(t *testing.T) {
	db := []*Restaurant{curatedTomo()}
	feed := []menusync.Restaurant{feedTomo(item("Miso ramen", 1500), item("Shio ramen", 1400), item("Tantan ramen", 1550))}

	p := Reconcile(db, feed, opts())
	st := total(p)
	want := Stats{RestaurantsUpdated: 1, ItemsCreated: 1, ItemsUpdated: 1, ItemsPriceChanged: 1}
	if st != want {
		t.Fatalf("stats %+v, want %+v\n%s", st, want, changes(p))
	}
	if !strings.Contains(changes(p), "Tomo — Miso ramen : 14,50 € → 15,00 €") || !strings.Contains(changes(p), "Tomo — Tantan ramen : nouveau plat (15,50 €)") {
		t.Fatalf("changes:\n%s", changes(p))
	}
	db = apply(t, db, p)
	if len(db) != 1 {
		t.Fatalf("the curated restaurant must be matched, not duplicated: %d restaurants", len(db))
	}
	r := db[0]
	// curated fields preserved; live fields follow the feed
	if r.Slug != "tomo-mons" || r.Emoji != "🍜" || r.Address != "Rue d'Enghien 11, 7000 Mons" || r.Lat != 50.45436 ||
		r.Phone != "065 35 29 64" || r.Rating != 4.7 || r.RatingCount != 210 || strings.Join(r.Cuisines, ",") != "japonais,ramen" {
		t.Fatalf("restaurant %+v", r)
	}
	if r.SourceKey != "takeaway-site:tomomons.be" || len(r.Sources) != 1 || r.Sources[0].CheckedAt != "2026-10-09" {
		t.Fatalf("provenance %q %+v", r.SourceKey, r.Sources)
	}
	if it := findItem(r, "Miso ramen"); it.Price != 1500 || it.SourceKey != "miso-ramen" {
		t.Fatalf("miso %+v", it)
	}

	// same feed again: nothing to write
	if p := Reconcile(db, feed, opts()); len(p.Restaurants) != 0 {
		t.Fatalf("second run must be a no-op: %+v\n%s", total(p), changes(p))
	}

	// an item disappears → unavailable; it comes back → available again
	short := []menusync.Restaurant{feedTomo(item("Miso ramen", 1500), item("Tantan ramen", 1550))}
	p = Reconcile(db, short, opts())
	if st := total(p); st.ItemsUnavailable != 1 || st.ItemsUpdated != 1 || !strings.Contains(changes(p), "Shio ramen : retiré de la carte") {
		t.Fatalf("missing item: %+v\n%s", st, changes(p))
	}
	db = apply(t, db, p)
	if findItem(db[0], "Shio ramen").Available {
		t.Fatal("missing item must become unavailable")
	}
	p = Reconcile(db, feed, opts())
	if st := total(p); st.ItemsUpdated != 1 || st.ItemsCreated != 0 || !strings.Contains(changes(p), "Shio ramen : de nouveau disponible") {
		t.Fatalf("item back: %+v\n%s", st, changes(p))
	}
	db = apply(t, db, p)
	if !findItem(db[0], "Shio ramen").Available || len(db[0].Items) != 3 {
		t.Fatalf("item back: %+v", db[0].Items)
	}
}

func TestReconcileLocked(t *testing.T) {
	// locked item: never touched, not even marked unavailable
	db := []*Restaurant{curatedTomo()}
	db[0].Items[0].Locked = true // Miso ramen, 14,50 €
	db[0].Items[1].Locked = true // Shio ramen, absent from the feed
	feed := []menusync.Restaurant{feedTomo(item("Miso ramen", 1700))}
	p := Reconcile(db, feed, opts())
	for _, rp := range p.Restaurants {
		for _, it := range rp.Items {
			if it.ID == "i_miso" || it.ID == "i_shio" {
				t.Fatalf("locked item written: %+v", it)
			}
		}
	}
	if st := total(p); st.ItemsPriceChanged != 0 || st.ItemsUnavailable != 0 {
		t.Fatalf("locked stats %+v", st)
	}

	// locked restaurant: nothing at all, and not duplicated
	db = []*Restaurant{curatedTomo()}
	db[0].Locked = true
	p = Reconcile(db, feed, opts())
	if len(p.Restaurants) != 0 || p.Locked != 1 || p.Matched != 1 {
		t.Fatalf("locked restaurant: %d plans, locked %d", len(p.Restaurants), p.Locked)
	}
}

func TestReconcileNewAndDedupe(t *testing.T) {
	// "BAGEL CITY" on weloveat (other address, 550 m away) is the curated "Bagel City"
	bagel := &Restaurant{ID: "r_bagel", Slug: "bagel-city", Name: "Bagel City", Lat: 50.4579174, Lng: 3.9552754, Active: true}
	wl := menusync.Restaurant{Source: menusync.SourceWeloveat, SourceURLs: []string{"https://weloveat.be/bagel-city"}}
	wl.Name, wl.Lat, wl.Lng, wl.Address = "BAGEL CITY", 50.4532346, 3.9517091, "Rue de la Chaussée 19, 7000 Mons"
	wl.Origins = []menusync.Origin{{Source: menusync.SourceWeloveat, URL: "https://weloveat.be/bagel-city"}}
	wl.Providers = []providers.Link{{ID: providers.Weloveat, URL: "https://weloveat.be/bagel-city"}}
	wl.Categories = []catalog.CategoryImport{{Name: "Bagels", Items: []catalog.ItemImport{item("Bagel saumon", 950), item("Sauce", 0)}}}

	// a brand new restaurant whose slug is taken
	nw := menusync.Restaurant{Source: menusync.SourceDeliveroo, SourceURLs: []string{"https://deliveroo.be/fr/menu/Brussels/mons-center/tomo"}}
	nw.Name, nw.Lat, nw.Lng = "TOMO - Mons Center", 50.47, 3.99 // 3 km away: another place
	nw.Cuisines = []string{"Burgers", "fast food", "boissons"}
	nw.Origins = []menusync.Origin{{Source: menusync.SourceDeliveroo, URL: nw.SourceURLs[0]}}
	nw.Categories = []catalog.CategoryImport{{Name: "Menus", Items: []catalog.ItemImport{item("Menu", 1200), item("Menu", 1200)}},
		{Name: "Boissons", Items: []catalog.ItemImport{item("Coca", 250), item("Menu", 1400)}}}

	db := []*Restaurant{bagel, {ID: "r_t", Slug: "tomo", Name: "Tomo", Lat: 50.4543, Lng: 3.9512, Active: true}}
	p := Reconcile(db, []menusync.Restaurant{wl, nw}, opts())
	if st := total(p); st.RestaurantsCreated != 1 || st.RestaurantsUpdated != 1 || st.ItemsCreated != 4 {
		t.Fatalf("stats %+v\n%s", st, changes(p))
	}
	db = apply(t, db, p)
	if len(db) != 3 {
		t.Fatalf("%d restaurants", len(db))
	}
	b := db[0]
	if b.Name != "Bagel City" || b.Lat != 50.4579174 || b.Address != "Rue de la Chaussée 19, 7000 Mons" || len(b.Items) != 1 || len(b.Providers) != 1 {
		t.Fatalf("bagel %+v", b)
	}
	n := db[2]
	if n.Slug != "tomo-2" || n.Name != "Tomo" || n.SourceKey != "deliveroo:deliveroo.be/fr/menu/brussels/mons-center/tomo" ||
		strings.Join(n.Cuisines, ",") != "burger,fast-food" || len(n.Categories) != 2 || n.PriceLevel != 2 {
		t.Fatalf("new %+v", n)
	}
	var keys []string
	for _, it := range n.Items {
		keys = append(keys, it.SourceKey)
	}
	if strings.Join(keys, ",") != "menu--menus,coca,menu--boissons" {
		t.Fatalf("item keys %v", keys)
	}
	if p := Reconcile(db, []menusync.Restaurant{wl, nw}, opts()); len(p.Restaurants) != 0 {
		t.Fatalf("second run: %s", changes(p))
	}
}

func TestReconcileStale(t *testing.T) {
	db := []*Restaurant{curatedTomo()}
	feed := []menusync.Restaurant{feedTomo(item("Miso ramen", 1450), item("Shio ramen", 1400))}
	db = apply(t, db, Reconcile(db, feed, opts()))
	other := &Restaurant{ID: "r_manual", Slug: "manual", Name: "Fait main", Active: true} // never synced
	db = append(db, other)

	// the source failed this run: not stale
	o := opts()
	o.Incomplete = map[string]bool{menusync.SourceTakeawaySite: true}
	if p := Reconcile(db, nil, o); len(p.Restaurants) != 0 {
		t.Fatalf("incomplete provider must not mark stale: %s", changes(p))
	}
	// the source succeeded without it: stale (once), the manual one is ignored
	p := Reconcile(db, nil, opts())
	if st := total(p); st.RestaurantsStale != 1 || len(p.Restaurants) != 1 || p.Restaurants[0].Restaurant.ID != "r_tomo" {
		t.Fatalf("stale %+v", st)
	}
	db = apply(t, db, p)
	if db[0].StaleSince == "" || !db[0].Active || len(db[0].Items) != 2 {
		t.Fatalf("stale restaurant must stay active with its menu: %+v", db[0])
	}
	if p := Reconcile(db, nil, opts()); len(p.Restaurants) != 0 {
		t.Fatal("already stale: nothing to do")
	}
	// it comes back
	p = Reconcile(db, feed, opts())
	db = apply(t, db, p)
	if db[0].StaleSince != "" || !strings.Contains(changes(p), "de nouveau proposé") {
		t.Fatalf("back: %q %s", db[0].StaleSince, changes(p))
	}
}

func TestReconcileNeverEmpties(t *testing.T) {
	db := []*Restaurant{curatedTomo()}
	feed := feedTomo()
	feed.Phone, feed.Address, feed.Lat, feed.Lng = "", "", 0, 0
	p := Reconcile(db, []menusync.Restaurant{feed}, opts())
	db = apply(t, db, p)
	r := db[0]
	if r.Phone == "" || r.Address == "" || r.Lat == 0 || len(r.Items) != 2 || !r.Items[0].Available {
		t.Fatalf("an empty feed must not erase anything: %+v", r)
	}
}

func TestFormatEuros(t *testing.T) {
	for in, want := range map[int]string{1450: "14,50 €", 5: "0,05 €", 0: "0,00 €", -250: "-2,50 €"} {
		if got := FormatEuros(in); got != want {
			t.Errorf("%d: %q", in, got)
		}
	}
}

// --- FetchAll on a local server (no external network) --------------------

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "menusync", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFetchAll(t *testing.T) {
	site, listing, menu := testdata(t, "takeaway_site.html"), testdata(t, "deliveroo_listing.html"), testdata(t, "deliveroo_menu.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/robots.txt":
			http.NotFound(w, r)
		case r.URL.Path == "/tomo/":
			_, _ = w.Write(site)
		case r.URL.Path == "/fr/restaurants/brussels/mons-center":
			_, _ = w.Write(listing)
		case strings.HasPrefix(r.URL.Path, "/fr/menu/"):
			_, _ = w.Write(menu)
		case r.URL.Path == "/blocked/":
			w.WriteHeader(http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	f := menusync.NewFetcher(t.TempDir())
	var waits []time.Duration
	f.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	srcs := []Source{
		{ID: "b", Provider: menusync.SourceTakeawaySite, Label: "Bloqué", URL: srv.URL + "/blocked/", Priority: 5},
		{ID: "d", Provider: menusync.SourceDeliveroo, Label: "Deliveroo", URL: srv.URL + "/fr/restaurants/brussels/mons-center?geohash=x", City: "mons", Priority: 20},
		{ID: "t", Provider: menusync.SourceTakeawaySite, Label: "Tomo", URL: srv.URL + "/tomo/", Priority: 10},
		{ID: "x", Provider: menusync.SourceTakeawaySite, Label: "Absent", URL: srv.URL + "/nope/", Priority: 30},
	}
	list, res := FetchAll(context.Background(), f, srcs, FetchOptions{RadiusKm: 8, Today: "2026-10-09"})
	var got []string
	for _, r := range res {
		got = append(got, r.ID+"="+r.Status)
	}
	if strings.Join(got, ",") != "b=blocked,t=ok,d=ok,x=failed" {
		t.Fatalf("results %v", res)
	}
	if len(list) != 2 || list[0].Name != "Tomo" || list[1].Name != "Baalbeck" || list[1].Providers[0].URL != srv.URL+"/fr/menu/Brussels/mons-center/baalbeck" {
		t.Fatalf("restaurants %d %+v", len(list), list)
	}
	if list[0].SourceKey() == "" || list[0].Origins[0].CheckedAt != "2026-10-09" {
		t.Fatalf("origins %+v", list[0].Origins)
	}
	for _, w := range waits {
		if w > 0 && w < menusync.MinDelay-100*time.Millisecond {
			t.Fatalf("politeness delay not respected: %v", waits)
		}
	}
	if pr := Priority(srcs); strings.Join(pr[:2], ",") != "takeaway-site,deliveroo" {
		t.Fatalf("priority %v", pr)
	}
}

func TestReconcileSummarizesBulkChanges(t *testing.T) {
	items := []catalog.ItemImport{item("Miso ramen", 1600)}
	for i := range 12 {
		items = append(items, item(fmt.Sprintf("Gyoza %d", i), 500))
	}
	p := Reconcile([]*Restaurant{curatedTomo()}, []menusync.Restaurant{feedTomo(items...)}, opts())
	got := changes(p)
	if !strings.Contains(got, "Tomo : 12 nouveaux plats") || strings.Contains(got, "Gyoza 3") ||
		!strings.Contains(got, "Tomo — Miso ramen : 14,50 € → 16,00 €") || !strings.Contains(got, "Tomo : infos mises à jour (note)") {
		t.Fatalf("changes:\n%s", got)
	}
}

func TestReconcileScoped(t *testing.T) {
	// Tomo is known from Deliveroo (preferred here) and its site; another
	// restaurant only from Deliveroo.
	cur := curatedTomo()
	cur.Sources = []SourceRef{
		{Provider: menusync.SourceDeliveroo, URL: "https://deliveroo.be/fr/menu/mons/tomo", CheckedAt: "2026-10-08"},
		{Provider: menusync.SourceTakeawaySite, URL: "https://www.tomomons.be/", CheckedAt: "2026-10-08"},
	}
	other := &Restaurant{ID: "r_other", Slug: "autre", Name: "Autre", Active: true, Lat: 50.46, Lng: 3.95,
		Sources: []SourceRef{{Provider: menusync.SourceDeliveroo, URL: "https://deliveroo.be/fr/menu/mons/autre"}}}
	feed := []menusync.Restaurant{feedTomo(item("Miso ramen", 1600), item("Tonkotsu", 1500))}

	o := opts()
	o.Scoped = true
	o.Priority = []string{menusync.SourceDeliveroo, menusync.SourceTakeawaySite}
	p := Reconcile([]*Restaurant{cur, other}, feed, o)
	if st := total(p); st.RestaurantsStale != 0 || st.ItemsCreated != 0 || st.ItemsPriceChanged != 0 {
		t.Fatalf("source moins prioritaire : menu intact, rien d'obsolète : %+v\n%s", st, changes(p))
	}
	for _, rp := range p.Restaurants {
		if rp.Restaurant.ID == "r_tomo" && len(rp.Restaurant.Sources) != 2 {
			t.Fatalf("provenance étendue, jamais remplacée : %+v", rp.Restaurant.Sources)
		}
	}

	// the site is now preferred: its menu applies, provenance kept
	o.Priority = []string{menusync.SourceTakeawaySite, menusync.SourceDeliveroo}
	p = Reconcile([]*Restaurant{cur, other}, feed, o)
	st := total(p)
	if st.RestaurantsStale != 0 || st.ItemsCreated != 1 || st.ItemsPriceChanged != 1 {
		t.Fatalf("source préférée : %+v\n%s", st, changes(p))
	}
	for _, rp := range p.Restaurants {
		if rp.Restaurant.ID == "r_other" {
			t.Fatal("une exécution ciblée ne touche pas aux autres restaurants")
		}
		if rp.Restaurant.ID == "r_tomo" && len(rp.Restaurant.Sources) != 2 {
			t.Fatalf("provenance : %+v", rp.Restaurant.Sources)
		}
	}

	// the same feed in a full run marks the other one stale
	if st := total(Reconcile([]*Restaurant{cur, other}, feed, opts())); st.RestaurantsStale != 1 {
		t.Fatalf("exécution complète : %+v", st)
	}
}

// A name truncated by the former cleaning rule ("Pitta" for the source's
// "Pitta Mons") is repaired from the source; a real name is left alone.
func TestReconcileRepairsTruncatedName(t *testing.T) {
	cur := curatedTomo()
	cur.Name = "Tomo Ramen de"
	feed := feedTomo(item("Miso ramen", 1450))
	feed.Name = "Tomo Ramen de Mons"
	db := apply(t, []*Restaurant{cur}, Reconcile([]*Restaurant{cur}, []menusync.Restaurant{feed}, opts()))
	if db[0].Name != "Tomo Ramen de Mons" {
		t.Fatalf("name not repaired: %q", db[0].Name)
	}

	cur = curatedTomo() // "Tomo": a real name, the feed's longer name does not replace it
	feed = feedTomo(item("Miso ramen", 1450))
	feed.Name = "Tomo Mons Ramen"
	db = apply(t, []*Restaurant{cur}, Reconcile([]*Restaurant{cur}, []menusync.Restaurant{feed}, opts()))
	if db[0].Name != cur.Name {
		t.Fatalf("real name changed: %q", db[0].Name)
	}
}
