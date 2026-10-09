package catalog

import (
	"slices"
	"strings"
	"testing"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

func TestParseCSVSemicolonFrenchDecimals(t *testing.T) {
	src := "\xef\xbb\xbfrestaurant_slug;restaurant_name;category;item_name;description;price_eur;tags;popular;address;lat;lng;cuisines;phone;ubereats_url;takeaway_url\n" +
		"chez-mario;Chez Mario;Pizzas;Margherita;\"Tomate; mozzarella\";12,50;veggie|new;oui;Rue de Nimy 1, 7000 Mons;50,4542;3,9567;pizza, italien;065 00 00 00;https://www.ubereats.com/be/store/chez-mario;\n" +
		"chez-mario;;Pizzas;Regina;;13.9;;;;;;;;;\n" +
		"chez-mario;;Desserts;Tiramisu;;6 €;;x;;;;;;;\n" +
		"\n" +
		"wok-a-mons;Wok à Mons;Wok;Pad thaï;;1 012,5;spicy;non;;;;;;;https://www.takeaway.com/be-fr/wok\n"
	got, errs := ParseCSV(strings.NewReader(src))
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(got) != 2 {
		t.Fatalf("restaurants: %d", len(got))
	}
	m := got[0].Import
	if m.Slug != "chez-mario" || m.Name != "Chez Mario" || m.Lat != 50.4542 || m.Lng != 3.9567 || m.Address != "Rue de Nimy 1, 7000 Mons" {
		t.Fatalf("metadata: %+v", m)
	}
	if !slices.Equal(m.Cuisines, []string{"pizza", "italien"}) {
		t.Fatalf("cuisines: %v", m.Cuisines)
	}
	if len(m.Providers) != 1 || m.Providers[0].ID != providers.UberEats {
		t.Fatalf("providers: %v", m.Providers)
	}
	if len(m.Categories) != 2 || m.Categories[0].Name != "Pizzas" || len(m.Categories[0].Items) != 2 {
		t.Fatalf("categories: %+v", m.Categories)
	}
	marg := m.Categories[0].Items[0]
	if marg.Price != 1250 || !marg.Popular || marg.Description != "Tomate; mozzarella" || !slices.Equal(marg.Tags, []string{"veggie", "new"}) {
		t.Fatalf("margherita: %+v", marg)
	}
	if p := m.Categories[0].Items[1].Price; p != 1390 {
		t.Fatalf("regina price %d", p)
	}
	if it := m.Categories[1].Items[0]; it.Price != 600 || !it.Popular {
		t.Fatalf("tiramisu %+v", it)
	}
	w := got[1].Import
	if w.Categories[0].Items[0].Price != 101250 || w.Providers[0].ID != providers.Takeaway {
		t.Fatalf("wok: %+v", w)
	}
	if probs := m.Problems(); len(probs) > 0 {
		t.Fatalf("problems: %v", probs)
	}
}

func TestParseCSVErrors(t *testing.T) {
	if _, errs := ParseCSV(strings.NewReader("")); len(errs) == 0 {
		t.Fatal("empty file must fail")
	}
	if _, errs := ParseCSV(strings.NewReader("restaurant_slug,item_name\nx,y\n")); len(errs) != 2 {
		t.Fatalf("missing columns: %v", errs)
	}
	src := "restaurant_slug,restaurant_name,category,item_name,description,price_eur,tags,popular\n" +
		"a,A,Plats,Bon,,\"12,50\",,\n" +
		"a,,Plats,Sans prix,,,,\n" +
		"a,,Plats,Prix faux,,douze,,\n" +
		"a,,,Sans catégorie,,3,,\n" +
		"a,,Plats,Pop,,3,,peut-être\n" +
		",,Plats,Orphelin,,3,,\n"
	got, errs := ParseCSV(strings.NewReader(src))
	if len(got) != 1 || len(got[0].Import.Categories[0].Items) != 1 {
		t.Fatalf("parsed: %+v", got)
	}
	if len(errs) != 5 {
		t.Fatalf("errors: %v", errs)
	}
	for i, want := range []string{"Ligne 3", "Ligne 4", "Ligne 5", "Ligne 6", "Ligne 7"} {
		if !strings.HasPrefix(errs[i], want) {
			t.Errorf("error %d = %q, want prefix %q", i, errs[i], want)
		}
	}
}

func TestMergeKeepsOptionsAndMetadata(t *testing.T) {
	yes := true
	existing := &RestaurantImport{
		Slug: "chez-mario", Name: "Chez Mario", Address: "Ancienne adresse", Lat: 50.1, Lng: 3.1, Phone: "065",
		Cuisines:  []string{"pizza"},
		Providers: []providers.Link{{ID: providers.UberEats, URL: "https://u"}, {ID: providers.Takeaway, URL: "https://t"}},
		Active:    &yes,
		Categories: []CategoryImport{{Name: "Pizzas", Items: []ItemImport{{
			Name: "Margherita", Price: 1000, Emoji: "🍕", Description: "Ancienne",
			OptionGroups: []domain.OptionGroup{{ID: "size", Name: "Taille", Min: 1, Max: 1, Choices: []domain.OptionChoice{{ID: "m", Name: "M"}}}},
		}}}},
	}
	csvSrc := "restaurant_slug,category,item_name,price_eur,address,takeaway_url\n" +
		"chez-mario,Pizzas,margherita,\"12,50\",Nouvelle adresse,https://t2\n" +
		"chez-mario,Pizzas,Calzone,13,,\n"
	parsed, errs := ParseCSV(strings.NewReader(csvSrc))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	out := Merge(existing, parsed[0])
	if out.Name != "Chez Mario" || out.Address != "Nouvelle adresse" || out.Lat != 50.1 || out.Phone != "065" || !slices.Equal(out.Cuisines, []string{"pizza"}) {
		t.Fatalf("metadata: %+v", out)
	}
	if len(out.Providers) != 2 || out.Providers[0].URL != "https://u" || out.Providers[1].URL != "https://t2" {
		t.Fatalf("providers: %+v", out.Providers)
	}
	items := out.Categories[0].Items
	if len(items) != 2 || items[0].Price != 1250 || len(items[0].OptionGroups) != 1 || items[0].Emoji != "🍕" || items[0].Description != "Ancienne" {
		t.Fatalf("items: %+v", items)
	}
	if len(items[1].OptionGroups) != 0 {
		t.Fatalf("new item got options: %+v", items[1])
	}
	if Merge(nil, parsed[0]).Name != "" {
		t.Fatal("new restaurant without name must stay empty (validation reports it)")
	}
}
