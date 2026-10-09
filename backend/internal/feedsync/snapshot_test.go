package feedsync

import (
	"slices"
	"strings"
	"testing"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

const snapshotFixture = `[
 {"name":"CTR Chicken Mons (Independant)","url":"https://www.ubereats.com/be/store/ctr-chicken-mons/abc?diningMode=DELIVERY",
  "rating":4.6,"rating_count":320,"eta_min":25,"categories":["Poulet","Burgers"],"promo":"-20 %","lat":50.4551,"lng":3.9512,
  "address":"Rue de la Chaussée 12, 7000 Mons","geo_approx":false,"items":[{"name":"Tenders x6","price":890}],"checked_at":"2026-10-09"},
 {"name":"Snack Pitta Grill Akropolis (Mons)","url":"https://www.ubereats.com/be/store/akropolis/def","rating":4.2,"rating_count":85,
  "eta_min":30,"categories":["Grec"],"lat":0,"lng":0,"address":"","geo_approx":true,"items":[],"checked_at":"2026-10-09"},
 {"name":"Donroll’s (Mons)","url":"https://www.ubereats.com/be/store/donrolls/ghi","rating":4.8,"rating_count":40,"eta_min":20,
  "categories":["Sushi"],"lat":50.4549,"lng":3.9530,"address":"","geo_approx":false,"items":[],"checked_at":"2026-10-09"},
 {"name":"Wok Express Nouveau","url":"https://www.ubereats.com/be/store/wok-nouveau/jkl","rating":4.1,"rating_count":12,"eta_min":35,
  "categories":["Chinois","Asiatique"],"lat":0,"lng":0,"address":"","geo_approx":true,
  "items":[{"name":"Nouilles sautées","price":1150},{"name":"Riz cantonais","price":990},{"name":"Gratuit","price":0},{"name":"Nouilles  sautées","price":1150}],
  "checked_at":"2026-10-09"},
 {"name":"","url":"https://www.ubereats.com/be/store/x/1"},
 {"name":"Pas Uber","url":"https://www.example.com/store"},
 {"name":"Doublon CTR","url":"https://www.ubereats.com/be/store/ctr-chicken-mons/abc"}
]`

func snapshotEntries(t *testing.T) []SnapshotEntry {
	t.Helper()
	entries, problems, err := ParseSnapshot([]byte(snapshotFixture), "mons")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 || len(problems) != 3 {
		t.Fatalf("entries %d, problems %v", len(entries), problems)
	}
	return entries
}

func TestParseSnapshot(t *testing.T) {
	entries := snapshotEntries(t)
	ctr := entries[0]
	if ctr.Name != "CTR Chicken" || ctr.URL != "https://www.ubereats.com/be/store/ctr-chicken-mons/abc" || !ctr.HasGeo() {
		t.Fatalf("ctr %+v", ctr)
	}
	if entries[1].Name != "Snack Pitta Grill Akropolis" || entries[1].HasGeo() {
		t.Fatalf("akropolis %+v", entries[1])
	}
	wok := entries[3]
	if len(wok.Items) != 2 || wok.Items[0].Name != "Nouilles sautées" {
		t.Fatalf("items: 0 € and duplicate dropped: %+v", wok.Items)
	}
	for _, data := range []string{"", "  ", "[]"} {
		if e, p, err := ParseSnapshot([]byte(data), "mons"); len(e) != 0 || len(p) != 0 || err != nil {
			t.Errorf("%q: %v %v %v", data, e, p, err)
		}
	}
	if _, _, err := ParseSnapshot([]byte("{nope"), "mons"); err == nil {
		t.Error("invalid JSON must fail")
	}
	// ReadSnapshot: an empty file is a normal (ok) state; bad JSON fails
	if _, res := ReadSnapshot(Source{ID: "s", Provider: ProviderUberEatsSnapshot}, []byte("[]"), nil); res.Status != StatusOK {
		t.Errorf("empty file: %+v", res)
	}
	if _, res := ReadSnapshot(Source{ID: "s", Provider: ProviderUberEatsSnapshot}, []byte("{"), nil); res.Status != StatusFailed {
		t.Errorf("bad file: %+v", res)
	}
	if e, res := ReadSnapshot(Source{ID: "s", Provider: ProviderUberEatsSnapshot}, []byte(snapshotFixture), nil); res.Status != StatusOK || res.Restaurants != 4 || len(e) != 4 {
		t.Errorf("fixture: %+v", res)
	}
}

func TestGuessEmoji(t *testing.T) {
	cases := []struct {
		cuisines []string
		name     string
		want     string
	}{
		{[]string{"pizza"}, "Roma", "🍕"},
		{[]string{"grec"}, "Akropolis", "🥙"},
		{nil, "Sushi Bar", "🍣"},
		{[]string{"poulet", "burger"}, "", "🍗"},
		{nil, "The Kitchen", "🍽️"},
		{[]string{"poké"}, "", "🥗"},
	}
	for _, c := range cases {
		if got := GuessEmoji(c.cuisines, c.name); got != c.want {
			t.Errorf("GuessEmoji(%v, %q) = %q, want %q", c.cuisines, c.name, got, c.want)
		}
	}
}

// storedCatalogue is the curated production data the snapshot meets.
func storedCatalogue() []*Restaurant {
	menu := func(id string) ([]*Category, []*Item) {
		return []*Category{{ID: "c_" + id, Name: "Plats"}},
			[]*Item{{ID: "i_" + id, CategoryID: "c_" + id, Name: "Plat maison", Price: 1000, Available: true}}
	}
	ctr := &Restaurant{ID: "r_ctr", Slug: "ctr-chicken", Name: "CTR Chicken", Lat: 50.4552, Lng: 3.9515, Active: true,
		Rating: 0, RatingCount: 0, EtaMin: 0, Providers: []providers.Link{{ID: providers.Takeaway, URL: "https://www.takeaway.com/be/menu/ctr"}}}
	ctr.Categories, ctr.Items = menu("ctr")
	ak := &Restaurant{ID: "r_ak", Slug: "akropolis", Name: "Snack Pitta Grec Akropolis", Lat: 50.4530, Lng: 3.9560, Active: true,
		Rating: 4.0, RatingCount: 10, EtaMin: 20, EtaMax: 30, Sources: []SourceRef{{Provider: menusync.SourceDeliveroo, URL: "https://deliveroo.be/fr/menu/mons/akropolis"}}}
	ak.Categories, ak.Items = menu("ak")
	don := &Restaurant{ID: "r_don", Slug: "donrolls", Name: "Donroll's", Lat: 50.4548, Lng: 3.9531, Active: true, Rating: 4.5}
	don.Categories, don.Items = menu("don")
	return []*Restaurant{ctr, ak, don}
}

func snapOpts(entries []SnapshotEntry) Options {
	o := opts()
	o.Snapshot = entries
	o.DefaultLat, o.DefaultLng = 50.46, 3.95
	return o
}

func byID(db []*Restaurant, id string) *Restaurant {
	for _, r := range db {
		if r.ID == id {
			return r
		}
	}
	return nil
}

func TestSnapshotMatchesAndFillsWithoutTouchingMenus(t *testing.T) {
	entries := snapshotEntries(t)
	db := storedCatalogue()
	p := Reconcile(db, nil, snapOpts(entries))
	if p.SnapshotMatched != 3 {
		t.Fatalf("matched %d\n%s", p.SnapshotMatched, strings.Join(p.Log, "\n"))
	}
	for _, rp := range p.Restaurants {
		if rp.Restaurant.ID != "" && (len(rp.Items) > 0 || len(rp.NewCategories) > 0) {
			t.Fatalf("%s: the menu of a matched restaurant must never change", rp.Restaurant.Name)
		}
	}
	db = apply(t, db, p)
	if len(db) != 4 {
		t.Fatalf("%d restaurants: only the unknown wok is created", len(db))
	}

	ctr := byID(db, "r_ctr")
	if ubereatsURL(ctr) != "https://www.ubereats.com/be/store/ctr-chicken-mons/abc" || ctr.Rating != 4.6 || ctr.RatingCount != 320 ||
		ctr.EtaMin != 25 || ctr.EtaMax != 40 || ctr.PartialMenu || len(ctr.Items) != 1 {
		t.Fatalf("ctr %+v", ctr)
	}
	ak := byID(db, "r_ak")
	if ubereatsURL(ak) == "" || ak.Rating != 4.0 || ak.RatingCount != 10 || ak.EtaMin != 20 || ak.EtaMax != 30 {
		t.Fatalf("akropolis: link only, existing note / delay kept: %+v", ak)
	}
	don := byID(db, "r_don")
	if ubereatsURL(don) == "" || don.Rating != 4.5 || don.RatingCount != 40 || don.EtaMin != 20 {
		t.Fatalf("donroll's: %+v", don)
	}
	if !strings.Contains(changes(p), "CTR Chicken : complété par Uber Eats (lien Uber Eats, note, délai)") {
		t.Fatalf("changes:\n%s", changes(p))
	}

	// second run with the same snapshot: nothing to write
	p2 := Reconcile(db, nil, snapOpts(entries))
	if len(p2.Restaurants) != 0 {
		t.Fatalf("second run must be a no-op:\n%s", changes(p2))
	}
}

func TestSnapshotCreatesPartialRestaurant(t *testing.T) {
	entries := snapshotEntries(t)
	p := Reconcile(nil, nil, snapOpts(entries))
	st := total(p)
	if st.RestaurantsCreated != 4 || st.ItemsCreated != 3 {
		t.Fatalf("stats %+v", st)
	}
	db := apply(t, nil, p)
	var wok, ak, ctr *Restaurant
	for _, r := range db {
		switch r.Name {
		case "Wok Express Nouveau":
			wok = r
		case "Snack Pitta Grill Akropolis":
			ak = r
		case "CTR Chicken":
			ctr = r
		}
	}
	if wok == nil || ak == nil || ctr == nil {
		t.Fatalf("created: %v", db)
	}
	if !wok.PartialMenu || !wok.Active || !wok.GeoApprox || wok.Lat != 50.46 || wok.Lng != 3.95 || wok.Emoji != "🥡" ||
		wok.Slug != "wok-express-nouveau" || wok.EtaMin != 35 || wok.EtaMax != 50 || wok.Rating != 4.1 || wok.RatingCount != 12 ||
		!slices.Equal(wok.Cuisines, []string{"chinois", "asiatique"}) || ubereatsURL(wok) == "" ||
		wok.SourceKey != "ubereats-snapshot:ubereats.com/be/store/wok-nouveau/jkl" ||
		len(wok.Sources) != 1 || wok.Sources[0].Provider != ProviderUberEatsSnapshot || wok.Sources[0].CheckedAt != "2026-10-09" {
		t.Fatalf("wok %+v", wok)
	}
	if len(wok.Categories) != 1 || wok.Categories[0].Name != PreviewCategory || len(wok.Items) != 2 || !wok.Items[0].Available {
		t.Fatalf("wok menu %+v %+v", wok.Categories, wok.Items)
	}
	if len(ak.Categories) != 0 || len(ak.Items) != 0 || !ak.PartialMenu {
		t.Fatalf("no item → no category: %+v", ak)
	}
	if ctr.GeoApprox || ctr.Lat != 50.4551 || ctr.Address != "Rue de la Chaussée 12, 7000 Mons" || ctr.Emoji != "🍗" {
		t.Fatalf("ctr with real coordinates %+v", ctr)
	}
	if !strings.Contains(changes(p), "Wok Express Nouveau : nouveau restaurant Uber Eats, carte partielle (2 plats en aperçu)") {
		t.Fatalf("changes:\n%s", changes(p))
	}

	// next run, same file: no write; updated file: preview refreshed
	if p := Reconcile(db, nil, snapOpts(entries)); len(p.Restaurants) != 0 {
		t.Fatalf("no-op expected:\n%s", changes(p))
	}
	upd := slices.Clone(entries)
	upd[3].Rating = 4.3
	upd[3].Items = []SnapshotItem{{Name: "Nouilles sautées", Price: 1200}, {Name: "Canard laqué", Price: 1690}}
	p = Reconcile(db, nil, snapOpts(upd))
	db = apply(t, db, p)
	if wok.Rating != 4.3 || findItem(wok, "Nouilles sautées").Price != 1200 || findItem(wok, "Canard laqué") == nil ||
		findItem(wok, "Riz cantonais").Available || !wok.PartialMenu {
		t.Fatalf("preview refresh: %+v\n%s", wok, changes(p))
	}
}

func fullWokFeed(n int) menusync.Restaurant {
	r := menusync.Restaurant{Source: menusync.SourceDeliveroo, SourceURLs: []string{"https://deliveroo.be/fr/menu/mons/wok-express"}}
	r.Name, r.Address, r.Lat, r.Lng = "WOK EXPRESS NOUVEAU - Mons", "Rue de Nimy 5, 7000 Mons", 50.4560, 3.9540
	r.Rating, r.RatingCount = 4.4, 200
	r.Providers = []providers.Link{{ID: providers.Deliveroo, URL: "https://deliveroo.be/fr/menu/mons/wok-express"}}
	r.Origins = []menusync.Origin{{Source: menusync.SourceDeliveroo, URL: "https://deliveroo.be/fr/menu/mons/wok-express", CheckedAt: "2026-10-10"}}
	names := []string{"Nouilles sautées", "Riz cantonais", "Poulet aigre-doux", "Bœuf aux oignons", "Rouleaux de printemps", "Soupe miso"}
	var items []catalog.ItemImport
	for i := range n {
		items = append(items, item(names[i], 1000+i*100))
	}
	r.Categories = []catalog.CategoryImport{{Name: "Plats", Items: items}}
	return r
}

func TestFullMenuReplacesPartialMenu(t *testing.T) {
	entries := snapshotEntries(t)[3:] // the wok only
	db := apply(t, nil, Reconcile(nil, nil, snapOpts(entries)))
	wok := db[0]

	// a feed with a small menu: matched, menu kept
	p := Reconcile(db, []menusync.Restaurant{fullWokFeed(3)}, snapOpts(entries))
	db = apply(t, db, p)
	if len(db) != 1 || !wok.PartialMenu || len(wok.Items) != 2 || findItem(wok, "Poulet aigre-doux") != nil {
		t.Fatalf("small feed menu must not replace the preview: %+v\n%s", wok, changes(p))
	}
	if wok.SourceKey != entries[0].Key() || len(wok.Sources) != 2 || wok.GeoApprox || wok.Lat != 50.4560 || wok.Address != "Rue de Nimy 5, 7000 Mons" {
		t.Fatalf("provenance / real position: %+v", wok)
	}
	// stable: same inputs, nothing to write
	if p := Reconcile(db, []menusync.Restaurant{fullWokFeed(3)}, snapOpts(entries)); len(p.Restaurants) != 0 {
		t.Fatalf("no-op expected:\n%s", changes(p))
	}

	// a full menu: takes over, partial_menu false
	p = Reconcile(db, []menusync.Restaurant{fullWokFeed(6)}, snapOpts(entries))
	db = apply(t, db, p)
	if len(db) != 1 || wok.PartialMenu || !strings.HasPrefix(wok.SourceKey, "deliveroo:") {
		t.Fatalf("takeover: %+v\n%s", wok, changes(p))
	}
	if it := findItem(wok, "Nouilles sautées"); it == nil || it.Price != 1000 || !it.Available {
		t.Fatalf("preview item matched by name and updated: %+v", it)
	}
	avail := 0
	for _, it := range wok.Items {
		if it.Available {
			avail++
		}
	}
	if avail != 6 || len(wok.Items) != 6 {
		t.Fatalf("items %d available / %d", avail, len(wok.Items))
	}
	if !strings.Contains(changes(p), "carte complète reçue (6 plats)") {
		t.Fatalf("changes:\n%s", changes(p))
	}

	// later runs: the snapshot only fills, never touches the full menu
	p = Reconcile(db, []menusync.Restaurant{fullWokFeed(6)}, snapOpts(entries))
	for _, rp := range p.Restaurants {
		if len(rp.Items) > 0 || len(rp.NewCategories) > 0 {
			t.Fatalf("menu touched:\n%s", changes(p))
		}
	}
	if len(db) != 1 {
		t.Fatal("no duplicate")
	}
}

func TestSnapshotLockedAndGeo(t *testing.T) {
	entries := snapshotEntries(t)
	db := storedCatalogue()
	db[0].Locked = true                // CTR
	db[2].Lat, db[2].Lng = 50.50, 3.95 // Donroll's ~5 km away from the Uber Eats store: another place
	p := Reconcile(db, nil, snapOpts(entries))
	for _, rp := range p.Restaurants {
		if rp.Restaurant.ID == "r_ctr" {
			t.Fatal("a locked restaurant is never touched")
		}
	}
	if p.Locked != 1 {
		t.Fatalf("locked %d", p.Locked)
	}
	db = apply(t, db, p)
	n := 0
	for _, r := range db {
		if strings.HasPrefix(r.Name, "Donroll") {
			n++
		}
	}
	if n != 2 || ubereatsURL(byID(db, "r_don")) != "" {
		t.Fatalf("too far: a new restaurant is created (%d)", n)
	}
	// an approximate position never blocks a name match (Akropolis has none)
	if ubereatsURL(byID(db, "r_ak")) == "" {
		t.Fatal("akropolis must match")
	}
}

func TestSnapshotRestaurantsAreNotStale(t *testing.T) {
	entries := snapshotEntries(t)
	db := storedCatalogue()
	// Akropolis is known from Deliveroo, which no longer lists it
	p := Reconcile(db, nil, snapOpts(entries))
	db = apply(t, db, p)
	for _, r := range db {
		if r.StaleSince != "" {
			t.Fatalf("%s marked stale although the snapshot lists it", r.Name)
		}
	}
	// a snapshot-only restaurant: feeds never list it, still not stale
	p = Reconcile(db, []menusync.Restaurant{feedTomo(item("Miso ramen", 1500))}, snapOpts(entries))
	db = apply(t, db, p)
	for _, r := range db {
		if r.StaleSince != "" {
			t.Fatalf("%s marked stale", r.Name)
		}
	}
	// snapshot failed this run: its restaurants are not marked stale either
	o := snapOpts(nil)
	o.Incomplete = map[string]bool{ProviderUberEatsSnapshot: true}
	p = Reconcile(db, nil, o)
	for _, rp := range p.Restaurants {
		if rp.Restaurant.PartialMenu && rp.Restaurant.StaleSince != "" {
			t.Fatalf("%s stale while the snapshot failed", rp.Restaurant.Name)
		}
	}
	// removed from the file (snapshot read fine): stale like any source
	p = Reconcile(db, nil, snapOpts(entries[:3]))
	if !strings.Contains(changes(p), "Wok Express Nouveau : plus proposé par aucune source (obsolète)") {
		t.Fatalf("changes:\n%s", changes(p))
	}
}
