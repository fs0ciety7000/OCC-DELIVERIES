package menusync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHelpers(t *testing.T) {
	slugs := map[string]string{
		"Ô Sando":                "o-sando",
		"DONROLL’S":              "donrolls",
		"Smash & Shake":          "smash-et-shake",
		"Thaï Café Mons":         "thai-cafe-mons",
		"  Pizza   Milano (BX) ": "pizza-milano-bx",
	}
	for in, want := range slugs {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	prices := map[string]int{"€ 16,00": 1600, "4,95 €": 495, "13.5": 1350, "7": 700, "€ 1.234,50": 123450, "0,5": 50}
	for in, want := range prices {
		if got, ok := ParseEuroCents(in); !ok || got != want {
			t.Errorf("ParseEuroCents(%q) = %d %v, want %d", in, got, ok, want)
		}
	}
	if _, ok := ParseEuroCents("prix sur demande"); ok {
		t.Error("no number must not parse")
	}
	if EurosToCents(13.45) != 1345 || EurosToCents(1.1) != 110 {
		t.Error("EurosToCents rounding")
	}
}

const deliverooRobots = `User-Agent: Twitterbot
Disallow:

User-Agent: *
Noindex: */login
Disallow: /admin/
Disallow: /api/
Disallow: */legal$
Disallow: /login
Disallow: apply/confirm?
Disallow: *?rel
Disallow: *&sp_id=
Disallow: */graphql/
Disallow: */graphql

Sitemap: https://deliveroo.be/en/sitemap-index.xml
`

func TestRobots(t *testing.T) {
	r := ParseRobots([]byte(deliverooRobots), robotsToken)
	tests := map[string]bool{
		"/fr/restaurants/brussels/mons-center?fulfillment_method=DELIVERY&geohash=u0fz40u6z12k": true,
		"/fr/menu/Brussels/mons-center/baalbeck?geohash=u0fz40u6z1ex":                           true,
		"/api/restaurants":                     false,
		"/fr/api/x":                            true, // "/api/" is anchored at the root
		"/fr/graphql":                          false,
		"/consumer/graphql/":                   false,
		"/login":                               false,
		"/fr/legal":                            false,
		"/fr/legal/cookies":                    true, // "$" anchors the end
		"/fr/menu/x?rel=1":                     false,
		"/fr/menu/x?a=1&sp_id=2":               false,
		"/fr/apply/confirm?token=1":            false,
		"/admin/":                              false,
		"/fr/restaurants/brussels/mons-center": true,
	}
	for path, want := range tests {
		if got := r.Allowed(path); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", path, got, want)
		}
	}

	// a specific group for us beats "*"; Allow wins over a shorter Disallow
	own := ParseRobots([]byte("User-agent: *\nDisallow: /\n\nUser-agent: OCC-Deliveries-menusync\nDisallow: /private\nAllow: /private/menu\n"), robotsToken)
	if !own.Allowed("/menu") || own.Allowed("/private/x") || !own.Allowed("/private/menu/1") {
		t.Error("own group")
	}
	if ParseRobots([]byte("User-agent: *\nDisallow: /\n"), robotsToken).Allowed("/x") {
		t.Error("disallow all")
	}
	// an SPA answering robots.txt with its index.html: no rules
	if !ParseRobots([]byte("<!DOCTYPE html><html><head><title>x</title></head></html>"), robotsToken).Allowed("/api/x") {
		t.Error("html robots")
	}
}

func TestDeliverooListing(t *testing.T) {
	es, err := ParseDeliverooListing(fixture(t, "deliveroo_listing.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 3 { // the duplicated card is folded
		t.Fatalf("got %d entries: %+v", len(es), es)
	}
	SortEntriesByDistance(es)
	if es[2].Name != "Pizza Milano Binche" || es[2].DistanceKm != 15.2 {
		t.Fatalf("sort: %+v", es)
	}
	b := es[0]
	if b.Name != "Baalbeck" || b.DistanceKm != 0.6 || b.ID != "301405" ||
		b.PageURL() != "https://deliveroo.be/fr/menu/Brussels/mons-center/baalbeck" ||
		b.MenuURL() != "https://deliveroo.be/fr/menu/Brussels/mons-center/baalbeck?geohash=u0fz40u6z1ex" {
		t.Fatalf("entry: %+v %s", b, b.PageURL())
	}
	if _, err := ParseDeliverooListing([]byte("<html></html>")); err == nil {
		t.Fatal("missing __NEXT_DATA__ must fail")
	}
}

func TestDeliverooMenu(t *testing.T) {
	const page = "https://deliveroo.be/fr/menu/Brussels/mons-center/baalbeck"
	r, err := ParseDeliverooMenu(fixture(t, "deliveroo_menu.html"), page)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Baalbeck" || r.Slug != "baalbeck" || r.Address != "14 rue de la clef 7000" || r.Phone != "+32488014822" {
		t.Fatalf("meta: %+v", r.RestaurantImport)
	}
	if r.Lat != 50.4533983 || r.Lng != 3.9527341 {
		t.Fatalf("geo: %v %v", r.Lat, r.Lng)
	}
	if r.MinOrder != 500 || r.DeliveryFee != 495 || r.Rating != 5 || r.RatingCount != 1 || r.EtaMin != 0 || r.EtaMax != 0 {
		t.Fatalf("fees/rating/eta: min %d fee %d rating %v (%d) eta %d-%d", r.MinOrder, r.DeliveryFee, r.Rating, r.RatingCount, r.EtaMin, r.EtaMax)
	}
	if strings.Join(r.Cuisines, ",") != "libanais,mezzes,oriental" {
		t.Fatalf("cuisines %v", r.Cuisines)
	}
	if len(r.Providers) != 1 || r.Providers[0].ID != providers.Deliveroo || r.Providers[0].URL != page {
		t.Fatalf("providers %+v", r.Providers)
	}
	// 3 categories, 6 items: the orphan item (only a modifier option) is dropped
	if len(r.Categories) != 3 || r.ItemCount() != 6 || r.ItemsWithOptions() != 1 {
		t.Fatalf("menu: %d cats, %d items, %d with options", len(r.Categories), r.ItemCount(), r.ItemsWithOptions())
	}
	var withOpts catalog.ItemImport
	for _, c := range r.Categories {
		for _, it := range c.Items {
			if len(it.OptionGroups) > 0 {
				withOpts = it
			}
			if it.Available != nil {
				t.Fatalf("closed restaurant: availability must not be trusted (%s)", it.Name)
			}
		}
	}
	g := withOpts.OptionGroups[0]
	// Deliveroo asks for 5 of N; the trimmed fixture only keeps 3 choices
	if withOpts.Name != "Assiette decouverte" || withOpts.Price != 1550 || g.ID != "1273095071" || g.Min != 3 || g.Max != 0 || len(g.Choices) != 3 {
		t.Fatalf("options: %+v", withOpts)
	}
	if err := domain.ValidateOptionGroups(withOpts.OptionGroups); err != nil {
		t.Fatal(err)
	}
	if err := r.RestaurantImport.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDeliverooAddress(t *testing.T) {
	for in, want := range map[string]string{
		"14 rue de la clef 7000, Brussels": "14 rue de la clef 7000",
		"10 Rue de Nimy, Brussels, 7000":   "10 Rue de Nimy, 7000",
		"Rue Neuve 1, 1000 Brussels":       "Rue Neuve 1, 1000 Brussels",
		" Grand'Rue 110 ,  7000 Mons ":     "Grand'Rue 110 , 7000 Mons",
	} {
		if got := cleanDeliverooAddress(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestDeliverooHeaderWithoutCuisines(t *testing.T) {
	page := []byte(`<script id="__NEXT_DATA__" type="application/json">{"props":{"initialState":{"menuPage":{"menu":{
"metas":{"root":{"restaurant":{"id":"1","name":"Crousty Night"},"categories":[{"id":"c","name":"Menus"}],
 "items":[{"id":"i","categoryId":"c","name":"Menu 1","price":{"fractional":1299},"available":true}]}},
"header":{"headerTags":{"lines":[{"spans":[{"text":"À 1.22 km"},{"text":"·"},{"text":"Ouvre à 00:00"},{"text":"Montant min. de 16,99 €"},{"text":"2,99 € de livraison"}]}]}}}}}}}</script>`)
	r, err := ParseDeliverooMenu(page, "https://deliveroo.be/fr/menu/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cuisines) != 0 || r.MinOrder != 1699 || r.DeliveryFee != 299 || r.ItemCount() != 1 {
		t.Fatalf("cuisines %v min %d fee %d", r.Cuisines, r.MinOrder, r.DeliveryFee)
	}
}

func TestNormalizeGroup(t *testing.T) {
	ch := []domain.OptionChoice{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	tests := []struct{ min, max, wantMin, wantMax int }{
		{1, 1, 1, 1},
		{0, 2, 0, 2},
		{0, 3, 0, 0}, // max = all choices → unbounded
		{5, 5, 3, 0}, // more required than offered
		{2, 1, 2, 2}, // max < min
		{-1, -1, 0, 0},
	}
	for _, tt := range tests {
		g := normalizeGroup(domain.OptionGroup{ID: "g", Min: tt.min, Max: tt.max, Choices: ch})
		if g.Min != tt.wantMin || g.Max != tt.wantMax {
			t.Errorf("(%d,%d) → (%d,%d), want (%d,%d)", tt.min, tt.max, g.Min, g.Max, tt.wantMin, tt.wantMax)
		}
		if err := domain.ValidateOptionGroups([]domain.OptionGroup{g}); err != nil {
			t.Error(err)
		}
	}
}

func TestWeloveat(t *testing.T) {
	shops, err := ParseWeloveatSearch(fixture(t, "weloveat_search.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(shops) != 2 || shops[0].Slug != "tomo-ramen" || shops[0].Lat == 0 || shops[0].Address != "Rue d'Enghien 11, 7000 Mons" {
		t.Fatalf("search: %+v", shops)
	}

	raw := fixture(t, "weloveat_catalogue.json")
	clean, err := SanitizeWeloveat(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(clean), "email") || strings.Contains(string(clean), "fcm_token") {
		t.Fatal("personal data must be removed before caching")
	}
	r, refs, err := ParseWeloveatCatalogue(clean, WeloveatPageURL("tomo-ramen"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "TOMO RAMEN" || r.Slug != "tomo-ramen" || r.Phone != "065352964" || r.Address != "Rue d'Enghien 11, 7000 Mons" ||
		r.MinOrder != 1000 || r.EtaMin != 45 || r.Rating != 4.8 || r.RatingCount != 123 || r.DeliveryFee != 0 {
		t.Fatalf("meta: %+v", r.RestaurantImport)
	}
	if r.Providers[0].ID != providers.Weloveat || r.Providers[0].URL != "https://weloveat.be/tomo-ramen" {
		t.Fatalf("providers %+v", r.Providers)
	}
	// the empty "SUGGESTIONS" category is dropped
	if len(r.Categories) != 2 || r.Categories[0].Name != "PLATS PRINCIPAUX 🍜" || r.ItemCount() != 4 || len(refs) != 4 {
		t.Fatalf("menu: %+v refs %d", r.Categories, len(refs))
	}
	if it := r.Categories[0].Items[0]; it.Name != "Classic Ramen" || it.Price != 1300 {
		t.Fatalf("item %+v", it)
	}

	groups, err := ParseWeloveatProduct(fixture(t, "weloveat_product.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups %+v", groups)
	}
	sup, broth := groups[0], groups[1]
	if sup.Name != "Suppléments" || sup.Min != 0 || sup.Max != 0 || len(sup.Choices) != 3 || sup.Choices[0].Name != "Œuf mollet" || sup.Choices[0].Price != 200 {
		t.Fatalf("multi-select group %+v", sup)
	}
	if broth.Min != 1 || broth.Max != 1 || broth.Choices[1].Price != 50 {
		t.Fatalf("single-select group %+v", broth)
	}
}

func TestTakeawaySite(t *testing.T) {
	r, err := ParseTakeawaySite(fixture(t, "takeaway_site.html"), "https://www.tomomons.be/")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Tomo" || r.Slug != "tomo" || r.Address != "11 Rue d’Enghien, 7000 Mons" || r.Lat != 50.4543472 || r.Lng != 3.9512159 {
		t.Fatalf("meta: %+v", r.RestaurantImport)
	}
	if len(r.Providers) != 1 || r.Providers[0].ID != providers.Takeaway || r.Providers[0].URL != "https://www.takeaway.com/be/menu/tomo-mons" {
		t.Fatalf("providers %+v", r.Providers)
	}
	if !strings.HasPrefix(r.CoverURL, "https://static.takeaway.com/images/restaurants/") {
		t.Fatalf("cover %q", r.CoverURL)
	}
	if len(r.Categories) != 2 || r.Categories[0].Name != "Menus" || r.Categories[1].Name != "Plats principaux" {
		t.Fatalf("categories %+v", r.Categories)
	}
	first := r.Categories[0].Items[0]
	if first.Name != "Menu classique ramen" || first.Description != "1 ramen classique et 1 boisson." || first.Price != 1600 || len(first.OptionGroups) != 0 {
		t.Fatalf("item %+v", first)
	}
	if err := r.RestaurantImport.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseTakeawaySite([]byte("<html><title>x</title></html>"), "https://x.be/"); err == nil {
		t.Fatal("empty page must fail")
	}
}

const jsonLDPage = `<!doctype html><html><head><title>Chez Test</title>
<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@type":"WebSite","name":"x"},
{"@type":["Restaurant"],"name":"Chez Test","telephone":"+32 65 00 00 00","servesCuisine":["Italien","Pizza"],
 "address":{"@type":"PostalAddress","streetAddress":"Rue de Nimy 1","postalCode":"7000","addressLocality":"Mons"},
 "geo":{"@type":"GeoCoordinates","latitude":50.455,"longitude":3.953},
 "aggregateRating":{"@type":"AggregateRating","ratingValue":"9","bestRating":"10","ratingCount":"12"},
 "hasMenu":{"@type":"Menu","hasMenuSection":[
   {"@type":"MenuSection","name":"Pizzas","hasMenuItem":[
     {"@type":"MenuItem","name":"Margherita","description":"Tomate, mozzarella","offers":{"@type":"Offer","price":"11.50","priceCurrency":"EUR"}},
     {"@type":"MenuItem","name":"Sans prix"}]},
   {"@type":"MenuSection","name":"Desserts","hasMenuItem":{"@type":"MenuItem","name":"Tiramisu","offers":{"price":6}}}]}}]}</script>
</head><body></body></html>`

const microdataPage = `<html><body>
<div itemscope itemtype="https://schema.org/Restaurant"><h1 itemprop="name">Micro Resto</h1>
 <div itemprop="address" itemscope itemtype="https://schema.org/PostalAddress"><span itemprop="streetAddress">Grand-Place 2</span> <span itemprop="postalCode">7000</span> <span itemprop="addressLocality">Mons</span></div>
 <div itemprop="hasMenu" itemscope itemtype="https://schema.org/Menu">
  <section itemprop="hasMenuSection" itemscope itemtype="https://schema.org/MenuSection"><h2 itemprop="name">Plats</h2>
   <div itemprop="hasMenuItem" itemscope itemtype="https://schema.org/MenuItem"><span itemprop="name">Boulettes</span>
    <div itemprop="offers" itemscope itemtype="https://schema.org/Offer"><span itemprop="price">14,90 €</span></div></div>
  </section></div></div></body></html>`

func TestJSONLD(t *testing.T) {
	r, err := ParseJSONLD([]byte(jsonLDPage), "https://chez-test.be/")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Chez Test" || r.Address != "Rue de Nimy 1, 7000 Mons" || r.Phone != "+32 65 00 00 00" || r.Lat != 50.455 ||
		r.Rating != 4.5 || r.RatingCount != 12 || strings.Join(r.Cuisines, ",") != "italien,pizza" {
		t.Fatalf("meta: %+v", r.RestaurantImport)
	}
	// "Sans prix" is skipped: a price is never guessed
	if len(r.Categories) != 2 || r.ItemCount() != 2 || r.Categories[0].Items[0].Price != 1150 || r.Categories[1].Items[0].Price != 600 {
		t.Fatalf("menu: %+v", r.Categories)
	}

	m, err := ParseJSONLD([]byte(microdataPage), "https://micro.be/")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "Micro Resto" || m.Address != "Grand-Place 2, 7000 Mons" || m.ItemCount() != 1 || m.Categories[0].Name != "Plats" || m.Categories[0].Items[0].Price != 1490 {
		t.Fatalf("microdata: %+v", m)
	}
	if _, err := ParseJSONLD([]byte("<html></html>"), "https://x.be/"); err == nil {
		t.Fatal("no restaurant must fail")
	}
}

func rest(src, name, addr string, lat, lng float64, links ...providers.Link) Restaurant {
	r := Restaurant{Source: src}
	r.Name, r.Slug, r.Address, r.Lat, r.Lng, r.Providers = name, Slugify(name), addr, lat, lng, links
	return r
}

func withMenu(r Restaurant, items ...string) Restaurant {
	c := catalog.CategoryImport{Name: "Carte"}
	for _, n := range items {
		c.Items = append(c.Items, catalog.ItemImport{Name: n, Price: 100})
	}
	r.Categories = []catalog.CategoryImport{c}
	return r
}

func TestSameRestaurant(t *testing.T) {
	tomoTA := rest(SourceTakeawaySite, "Tomo", "11 Rue d’Enghien, 7000 Mons", 50.4543472, 3.9512159)
	tomoWL := rest(SourceWeloveat, "TOMO RAMEN", "Rue d'Enghien 11, 7000 Mons", 50.4543472, 3.9512159)
	tomoNoGeo := rest(SourceExisting, "Tomo", "Rue d'Enghien 11, 7000 Mons", 0, 0)
	baalDR := rest(SourceDeliveroo, "Baalbeck", "14 rue de la clef 7000", 50.4533983, 3.9527341)
	baalWL := rest(SourceWeloveat, "Baalbeck restaurant", "Rue de la Clef 16, 7000 Mons", 50.4533357, 3.9527035)
	twenty := rest(SourceWeloveat, "Twenty buns", "Rue de la Clef 18, 7000 Mons", 50.4533065, 3.9527065)
	donA := rest(SourceDeliveroo, "Donrolls", "", 50.4495182, 3.9492786)
	donB := rest(SourceWeloveat, "DONROLL’S", "Grand'Rue 110, 7000 Mons", 50.4495190, 3.9492790)
	farTomo := rest(SourceDeliveroo, "Tomo", "Rue X 1, 7000 Mons", 50.47, 3.99)
	linkA := rest(SourceExisting, "Cup Pasta", "", 0, 0, providers.Link{ID: "takeaway", URL: "https://www.takeaway.com/be/menu/cup-1"})
	linkB := rest(SourceTakeawaySite, "Seck Tebbiche", "", 0, 0, providers.Link{ID: "takeaway", URL: "https://www.takeaway.com/be-fr/menu/cup-1?x=1"})

	tests := []struct {
		name string
		a, b Restaurant
		want bool
	}{
		{"name subset + same geo", tomoTA, tomoWL, true},
		{"same street and number, no geo", tomoTA, tomoNoGeo, true},
		{"deliveroo/weloveat within 150 m", baalDR, baalWL, true},
		{"neighbours with other names", baalWL, twenty, false},
		{"spacing/apostrophe in name", donA, donB, true},
		{"same name far away", tomoTA, farTomo, false},
		{"same takeaway store link", linkA, linkB, true},
	}
	for _, tt := range tests {
		if got := SameRestaurant(tt.a, tt.b); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
		if got := SameRestaurant(tt.b, tt.a); got != tt.want {
			t.Errorf("%s (swapped): got %v", tt.name, got)
		}
	}
}

func TestMerge(t *testing.T) {
	yes := true
	existing := withMenu(rest("", "Tomo", "Rue d'Enghien 11, 7000 Mons", 50.45434, 3.95121,
		providers.Link{ID: "takeaway", URL: "https://www.takeaway.com/be/menu/tomo-mons"}), "Ancien")
	existing.Slug, existing.Emoji, existing.PriceLevel, existing.Active = "tomo-mons", "🍜", 2, &yes
	existing.SourceURLs = []string{"https://www.tomomons.be/"}
	existing.Cuisines = []string{"ramen"}

	site := withMenu(rest(SourceTakeawaySite, "Tomo", "11 Rue d’Enghien, 7000 Mons", 50.4543472, 3.9512159,
		providers.Link{ID: "takeaway", URL: "https://www.takeaway.com/be/menu/tomo-mons"}), "Shio ramen", "Miso ramen")
	site.SourceURLs = []string{"https://www.tomomons.be/"}
	site.MenuCheckedAt = "2026-10-09"

	wl := withMenu(rest(SourceWeloveat, "TOMO RAMEN", "Rue d'Enghien 11, 7000 Mons", 50.4543472, 3.9512159,
		providers.Link{ID: "weloveat", URL: "https://weloveat.be/tomo-ramen"}), "Classic Ramen")
	wl.Rating, wl.RatingCount, wl.MinOrder, wl.EtaMin, wl.EtaMax, wl.Phone = 4.8, 123, 1000, 45, 45, "065352964"
	wl.SourceURLs = []string{"https://weloveat.be/tomo-ramen"}
	wl.Cuisines = []string{"asiatique"}

	other := withMenu(rest(SourceDeliveroo, "Bagel City", "", 50.4532, 3.9517,
		providers.Link{ID: "deliveroo", URL: "https://deliveroo.be/fr/menu/Brussels/mons-center/bagel-city"}), "Bagel")

	out, stats := Merge([]Restaurant{wl, other, existing, site}, nil)
	if stats.In != 4 || stats.Out != 2 || stats.Duplicates() != 2 || len(stats.Merged) != 1 || len(stats.Merged[0].Sources) != 3 {
		t.Fatalf("stats %+v", stats)
	}
	var tomo Restaurant
	for _, r := range out {
		if strings.HasPrefix(r.Slug, "tomo") {
			tomo = r
		}
	}
	// curated fields from existing data, menu from the takeaway site (priority),
	// the rest from the first source that has it, all links kept
	if tomo.Slug != "tomo-mons" || tomo.Name != "Tomo" || tomo.Emoji != "🍜" || tomo.PriceLevel != 2 || tomo.Active == nil ||
		tomo.Address != "Rue d'Enghien 11, 7000 Mons" {
		t.Fatalf("curated: %+v", tomo.RestaurantImport)
	}
	if tomo.Source != SourceTakeawaySite || tomo.ItemCount() != 2 || tomo.MenuCheckedAt != "2026-10-09" {
		t.Fatalf("menu from %s, %d items", tomo.Source, tomo.ItemCount())
	}
	if tomo.Rating != 4.8 || tomo.RatingCount != 123 || tomo.MinOrder != 1000 || tomo.EtaMin != 45 || tomo.Phone != "065352964" || tomo.Lat != 50.4543472 {
		t.Fatalf("fields: %+v", tomo.RestaurantImport)
	}
	if len(tomo.Providers) != 2 || len(tomo.SourceURLs) != 2 || strings.Join(tomo.Cuisines, ",") != "ramen,asiatique" {
		t.Fatalf("unions: %+v %v %v", tomo.Providers, tomo.SourceURLs, tomo.Cuisines)
	}
	// nothing invented
	if tomo.DeliveryFee != 0 || tomo.Description != "" || tomo.CoverURL != "" {
		t.Fatalf("invented: %+v", tomo.RestaurantImport)
	}

	// another priority puts weloveat's menu first
	out, _ = Merge([]Restaurant{wl, existing, site}, []string{SourceWeloveat, SourceTakeawaySite, SourceExisting})
	if out[0].Source != SourceWeloveat || out[0].ItemCount() != 1 {
		t.Fatalf("priority: %s %d", out[0].Source, out[0].ItemCount())
	}

	// two distinct places with the same name keep unique slugs
	a := withMenu(rest(SourceDeliveroo, "Pizza Hut", "", 50.45, 3.95), "x")
	b := withMenu(rest(SourceDeliveroo, "Pizza Hut", "", 50.40, 3.90), "y")
	out, stats = Merge([]Restaurant{a, b}, nil)
	if stats.Out != 2 || out[0].Slug == out[1].Slug {
		t.Fatalf("slugs %s %s", out[0].Slug, out[1].Slug)
	}
}

// --- fetcher (local test server, no external network) -------------------

func testFetcher(t *testing.T) (*Fetcher, *[]time.Duration) {
	f := NewFetcher(t.TempDir())
	f.Jitter = 0
	var waits []time.Duration
	f.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	return f, &waits
}

func TestFetcher(t *testing.T) {
	hits := map[string]int{}
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		gotUA = r.Header.Get("User-Agent")
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /api/\n"))
		case "/ok":
			_, _ = w.Write([]byte("<html>menu</html>"))
		case "/flaky":
			if hits["/flaky"] == 1 {
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_, _ = w.Write([]byte("second time"))
		case "/down":
			w.WriteHeader(http.StatusBadGateway)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "/challenge":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("<html><head><title>Just a moment...</title></head><body><script src=\"/cdn-cgi/challenge-platform/x\"></script></body></html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	f, waits := testFetcher(t)
	b, err := f.Get(ctx, srv.URL+"/ok")
	if err != nil || string(b) != "<html>menu</html>" || gotUA != UserAgent {
		t.Fatalf("ok: %q %v ua=%q", b, err, gotUA)
	}
	// cache: no second request
	if _, err := f.Get(ctx, srv.URL+"/ok"); err != nil || hits["/ok"] != 1 || f.Cached != 1 {
		t.Fatalf("cache: hits %d cached %d", hits["/ok"], f.Cached)
	}
	// robots.txt enforced before any request
	if _, err := f.Get(ctx, srv.URL+"/api/menu"); err == nil || hits["/api/menu"] != 0 {
		t.Fatalf("robots: %v", err)
	} else if _, ok := err.(*ErrDisallowed); !ok {
		t.Fatalf("robots error type %T", err)
	}
	// 429 → one retry after Retry-After
	if b, err := f.Get(ctx, srv.URL+"/flaky"); err != nil || string(b) != "second time" || hits["/flaky"] != 2 {
		t.Fatalf("retry: %q %v %d", b, err, hits["/flaky"])
	}
	if !containsDur(*waits, 7*time.Second) {
		t.Fatalf("Retry-After not honoured: %v", *waits)
	}
	// 5xx twice → error after a single retry
	if _, err := f.Get(ctx, srv.URL+"/down"); err == nil || hits["/down"] != 2 {
		t.Fatalf("down: %v %d", err, hits["/down"])
	}
	// 403 and challenge pages stop immediately, without retry
	if _, err := f.Get(ctx, srv.URL+"/forbidden"); !IsBlocked(err) || hits["/forbidden"] != 1 {
		t.Fatalf("403: %v", err)
	}
	if _, err := f.Get(ctx, srv.URL+"/challenge"); !IsBlocked(err) || hits["/challenge"] != 1 {
		t.Fatalf("challenge: %v", err)
	}
	if hits["/robots.txt"] != 1 {
		t.Fatalf("robots fetched %d times", hits["/robots.txt"])
	}
	// the delay can't go under the minimum
	f.Delay = time.Millisecond
	f.last = time.Now()
	*waits = nil
	if _, err := f.Get(ctx, srv.URL+"/nope"); err == nil {
		t.Fatal("404 must fail")
	}
	if len(*waits) == 0 || (*waits)[0] < MinDelay-100*time.Millisecond {
		t.Fatalf("delay not clamped: %v", *waits)
	}
}

func containsDur(ds []time.Duration, d time.Duration) bool {
	for _, x := range ds {
		if x == d {
			return true
		}
	}
	return false
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	if retryAfter("30", now) != 30*time.Second || retryAfter("", now) != 0 || retryAfter("soon", now) != 0 {
		t.Error("seconds")
	}
	if d := retryAfter("Fri, 09 Oct 2026 12:01:00 GMT", now); d != time.Minute {
		t.Errorf("date: %v", d)
	}
}

func TestChallengeReason(t *testing.T) {
	h := http.Header{}
	if challengeReason(200, h, fixture(t, "deliveroo_menu.html")) != "" {
		t.Error("a normal page is not a challenge")
	}
	if challengeReason(200, h, []byte(`<div id="px-captcha"></div>`)) == "" {
		t.Error("PerimeterX captcha")
	}
	h.Set("Cf-Mitigated", "challenge")
	if challengeReason(200, h, nil) == "" {
		t.Error("cf-mitigated header")
	}
}
