package menusync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

func TestNormalizeName(t *testing.T) {
	tests := map[string]string{
		"Chamas Tacos - Mons - Mons Center": "Chamas Tacos",
		"GRILL PITA - Mons Center":          "Grill Pita",
		"Hashtag Bakeries (MON)":            "Hashtag Bakeries",
		"New Delhi (MONS)":                  "New Delhi",
		"Pili Pili Mons":                    "Pili Pili",
		"Pitta Mons":                        "Pitta Mons",
		"Pizza - Mons":                      "Pizza - Mons",
		"O'Tacos Centre ville de Mons":      "O'Tacos Centre ville de Mons",
		"Melanza Mons":                      "Melanza",
		"BUGA RAMEN MONS":                   "Buga Ramen",
		"BBQ Home - Mons":                   "BBQ Home",
		"BAGEL CITY":                        "Bagel City",
		"L’ARÈNE SANDWICHERIE":              "L’Arène Sandwicherie",
		"SEASON’S":                          "Season’s",
		"MODERNA - LE COMPTOIR À PIZZA":     "Moderna - Le Comptoir à Pizza",
		"CTR CHICKEN MONS":                  "CTR Chicken",
		"OB POKÉ BOWL MONS":                 "OB Poké Bowl",
		"SNACK PITTA-GREC":                  "Snack Pitta-Grec",
		"PIZZA DE LA GARE":                  "Pizza de la Gare",
		"Ô Sando":                           "Ô Sando",
		"China express Nimy":                "China express Nimy",
		"Cumin & Cannelle * VEGETARIEN":     "Cumin & Cannelle * VEGETARIEN",
		"  Le   Mons  ":                     "Le Mons",
		"Mons":                              "Mons",
		"Khéops & Bryan’s Burger House":     "Khéops & Bryan’s Burger House",
	}
	for in, want := range tests {
		if got := NormalizeName(in, "mons"); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := NormalizeName("Pili Pili Mons", ""); got != "Pili Pili Mons" {
		t.Errorf("no city: %q", got)
	}
	if NameKey("BAGEL CITY", "mons") != NameKey("Bagel City", "mons") || NameKey("Pili Pili Mons", "mons") != NameKey("PILI PILI MONS", "mons") {
		t.Error("NameKey must ignore case, accents and city suffix")
	}
}

func TestNormalizeCuisines(t *testing.T) {
	tests := []struct {
		in, want []string
	}{
		{[]string{"Burgers", "burger", "Pizzas"}, []string{"burger", "pizza"}},
		{[]string{"sandwichs", "Sandwich", "fast food"}, []string{"sandwich", "fast-food"}},
		{[]string{"thailandais", "Thaï", "poke"}, []string{"thaï", "poké"}},
		{[]string{"boissons", "indien", "desserts", "poulet"}, []string{"indien", "poulet"}},
		{[]string{"desserts", "boissons"}, []string{"desserts", "boissons"}},
		{[]string{"végétarien", "Pâtes", "pates", "italien", "salades", "japonais"}, []string{"végétarien", "pâtes", "italien", "salade"}},
		{nil, []string{}},
	}
	for _, tt := range tests {
		if got := NormalizeCuisines(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("NormalizeCuisines(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestCleanMenu(t *testing.T) {
	opts := []domain.OptionGroup{{ID: "s", Name: "Taille", Min: 1, Max: 1, Choices: []domain.OptionChoice{{ID: "m", Name: "M", Price: 900}}}}
	in := []catalog.CategoryImport{
		{Name: "  Pizzas ", Items: []catalog.ItemImport{
			{Name: " Margherita  napoletana ", Description: "tomate,   mozza", Price: 1000},
			{Name: "Margherita napoletana", Description: "tomate, mozza", Price: 1000}, // duplicate
			{Name: "Margherita napoletana", Description: "tomate, mozza", Price: 1200}, // other price: kept
			{Name: "Sauce offerte", Price: 0},                                          // dropped
			{Name: "Pizza à composer", Price: 0, OptionGroups: opts},                   // kept: price in options
			{Name: "  ", Price: 500},
		}},
		{Name: "PIZZAS", Items: []catalog.ItemImport{{Name: "Calzone", Description: "Calzone", Price: 1200}}},
		{Name: "Vide", Items: []catalog.ItemImport{{Name: "Rien", Price: 0}}},
	}
	out := CleanMenu(in)
	if len(out) != 1 || out[0].Name != "Pizzas" {
		t.Fatalf("categories: %+v", out)
	}
	var names []string
	for _, it := range out[0].Items {
		names = append(names, it.Name)
	}
	if strings.Join(names, "|") != "Margherita napoletana|Margherita napoletana|Pizza à composer|Calzone" {
		t.Fatalf("items: %v", names)
	}
	if out[0].Items[0].Description != "tomate, mozza" || out[0].Items[3].Description != "" {
		t.Fatalf("descriptions: %+v", out[0].Items)
	}
}

func TestSameRestaurantRelaxed(t *testing.T) {
	bagelDR := rest(SourceDeliveroo, "Bagel City", "Rue de nimy 87, 7000", 50.4579174, 3.9552754)
	bagelWL := rest(SourceWeloveat, "BAGEL CITY", "Rue de la Chaussée 19, 7000 Mons", 50.4532346, 3.9517091)
	pizza := rest(SourceWeloveat, "La Pizza", "", 50.4538, 3.9538)
	moderna := rest(SourceWeloveat, "MODERNA - LE COMPTOIR À PIZZA", "", 50.4538234, 3.9538883)
	cuminA := rest(SourceDeliveroo, "Cumin et cannelle Jurbise", "Route d ath 49, 7050", 50.4816258, 3.9483143)
	cuminB := rest(SourceDeliveroo, "Cumin & Cannelle * VEGETARIEN", "Route d ath 49, 7050", 50.4816258, 3.9483143)
	siteA := rest(SourceTakeawaySite, "Tomo", "", 0, 0)
	siteA.SourceURLs = []string{"https://www.tomomons.be/"}
	siteB := rest(SourceExisting, "Tomo Ramen Bar", "", 0, 0)
	siteB.SourceURLs = []string{"http://tomomons.be"}
	tests := []struct {
		name string
		a, b Restaurant
		want bool
	}{
		{"same name, 550 m apart", bagelDR, bagelWL, true},
		{"generic word only", pizza, moderna, false},
		{"two brands of one kitchen", cuminA, cuminB, false},
		{"same source page", siteA, siteB, true},
	}
	for _, tt := range tests {
		if got := SameRestaurant(tt.a, tt.b); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
	bagelDR.SourceURLs = []string{"https://deliveroo.be/fr/menu/Brussels/mons-center/bagel-city"}
	bagelWL.SourceURLs = []string{"https://weloveat.be/bagel-city"}
	out, st := Merge([]Restaurant{withMenu(bagelDR, "Bagel"), withMenu(bagelWL, "Bagel saumon")}, nil)
	if st.Out != 1 || out[0].Name != "Bagel City" || out[0].Slug != "bagel-city" || len(out[0].Origins) != 2 {
		t.Fatalf("bagel merge: %+v %+v", st, out)
	}
	if k := out[0].SourceKey(); !strings.HasPrefix(k, SourceDeliveroo+":") {
		t.Fatalf("source key %q", k)
	}
}

func TestMergeNormalizes(t *testing.T) {
	r := withMenu(rest(SourceWeloveat, "PILI PILI MONS", "", 50.45, 3.95,
		providers.Link{ID: "weloveat", URL: "https://weloveat.be/pili-pili-mons"}), "Poulet")
	r.Cuisines = []string{"fast food", "Burgers", "burger", "boissons", "poulet", "africain"}
	r.CoverURL = "?width=1200&height=630&fit=crop" // placeholder without host
	out, _ := Merge([]Restaurant{r}, nil)
	if out[0].CoverURL != "" {
		t.Fatalf("relative cover kept: %q", out[0].CoverURL)
	}
	if out[0].Name != "Pili Pili" || out[0].Slug != "pili-pili" || strings.Join(out[0].Cuisines, ",") != "fast-food,burger,poulet,africain" {
		t.Fatalf("normalized: %s %s %v", out[0].Name, out[0].Slug, out[0].Cuisines)
	}
}

func TestFetcherCacheTTL(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			hits++
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	f, _ := testFetcher(t)
	f.CacheTTL = time.Hour
	ctx := context.Background()
	for range 2 {
		if _, err := f.Get(ctx, srv.URL+"/page"); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 1 {
		t.Fatalf("fresh cache must be used: %d hits", hits)
	}
	// age every cached file beyond the TTL
	old := time.Now().Add(-2 * time.Hour)
	_ = filepath.Walk(f.CacheDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			_ = os.Chtimes(p, old, old)
		}
		return nil
	})
	if _, err := f.Get(ctx, srv.URL+"/page"); err != nil || hits != 2 {
		t.Fatalf("expired cache must be refetched: %v %d", err, hits)
	}
}

func TestTruncatedName(t *testing.T) {
	cases := []struct {
		stored, published string
		want              bool
	}{
		{"Pitta", "Pitta Mons", true},
		{"O'Tacos Centre ville de", "O'Tacos Centre ville de Mons", true},
		{"Melanza", "Melanza Mons", false},  // a real name, not truncated
		{"Pitta Mons", "Pitta Mons", false}, // unchanged
		{"Pizza", "Pizza Hut", true},        // generic word alone, longer name published
		{"Tomo", "Sushi Tomo", false},       // not a prefix
	}
	for _, c := range cases {
		if got := TruncatedName(c.stored, c.published); got != c.want {
			t.Errorf("TruncatedName(%q, %q) = %v", c.stored, c.published, got)
		}
	}
}
