package menusync

import (
	"strings"
	"testing"
)

func cleaner(t *testing.T) func(Restaurant, error) Restaurant {
	return func(r Restaurant, err error) Restaurant {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		r.Clean("mons")
		return r
	}
}

func TestTakeawaySiteContact(t *testing.T) {
	cl := cleaner(t)
	page := string(fixture(t, "takeaway_site.html"))
	r := cl(ParseTakeawaySite([]byte(page), "https://www.tomomons.be/"))
	if r.Address != "Rue d’Enghien 11, 7000 Mons" || r.Lat != 50.4543472 || r.Lng != 3.9512159 || r.Phone != "" {
		t.Fatalf("fixture contact: %q %q %v %v", r.Address, r.Phone, r.Lat, r.Lng)
	}
	footer := `<div class="widget" id="address">`
	if !strings.Contains(page, footer) {
		t.Fatal("fixture changed: footer widget not found")
	}
	cases := map[string]string{
		"tel: link":          strings.Replace(page, footer, footer+`<a href="tel:+3265352964">Appeler</a>`, 1),
		"contact block text": strings.Replace(page, footer, footer+`<p>Tél. : 065/35.29.64</p>`, 1),
		"unlabelled number":  strings.Replace(page, footer, footer+`<p>065 35 29 64</p>`, 1),
		"microdata":          strings.Replace(page, `<h2 itemprop="name" content="Tomo Mons">Tomo</h2>`, `<h2 itemprop="name" content="Tomo Mons">Tomo</h2><span itemprop="telephone">+32 (0)65 35 29 64</span>`, 1),
		"json in script":     strings.Replace(page, "</body>", `<script>window.__R = {"name":"Tomo","phone":"065 35 29 64"}</script></body>`, 1),
		"json-ld":            strings.Replace(page, "</body>", `<script type="application/ld+json">{"@type":"LocalBusiness","name":"Tomo","telephone":"065352964"}</script></body>`, 1),
	}
	for name, p := range cases {
		r := cl(ParseTakeawaySite([]byte(p), "https://www.tomomons.be/"))
		if r.Phone != "+3265352964" {
			t.Errorf("%s: phone %q", name, r.Phone)
		}
		if r.Address != "Rue d’Enghien 11, 7000 Mons" {
			t.Errorf("%s: address %q", name, r.Address)
		}
	}
}

func TestJSONLDContact(t *testing.T) {
	cl := cleaner(t)
	r := cl(ParseJSONLD([]byte(jsonLDPage), "https://chez-test.be/"))
	if r.Phone != "+3265000000" || r.Address != "Rue de Nimy 1, 7000 Mons" || r.Lat != 50.455 || r.Lng != 3.953 {
		t.Fatalf("json-ld contact: %q %q %v %v", r.Phone, r.Address, r.Lat, r.Lng)
	}
	// microdata menu, contact only in a separate JSON-LD LocalBusiness and a string address
	page := strings.Replace(microdataPage, "<html><body>", `<html><head><script type="application/ld+json">
{"@type":"LocalBusiness","name":"Micro Resto","telephone":["0475 12 34 56"],"geo":{"latitude":"50.45","longitude":"3.95"}}</script></head><body>`, 1)
	m := cl(ParseJSONLD([]byte(page), "https://micro.be/"))
	if m.Phone != "+32475123456" || m.Address != "Grand-Place 2, 7000 Mons" || m.Lat != 50.45 || m.Lng != 3.95 {
		t.Fatalf("fallback contact: %q %q %v %v", m.Phone, m.Address, m.Lat, m.Lng)
	}
	// junk phones are dropped, never stored
	junk := strings.Replace(jsonLDPage, `"+32 65 00 00 00"`, `"n/a"`, 1)
	j := cl(ParseJSONLD([]byte(junk), "https://chez-test.be/"))
	if j.Phone != "" {
		t.Fatalf("junk phone kept: %q", j.Phone)
	}
}

func TestDeliverooContact(t *testing.T) {
	cl := cleaner(t)
	r := cl(ParseDeliverooMenu(fixture(t, "deliveroo_menu.html"), "https://deliveroo.be/fr/menu/brussels/mons-center/baalbeck"))
	if r.Address != "Rue de la Clef 14, 7000 Mons" || r.Phone != "+32488014822" || r.Lat != 50.4533983 || r.Lng != 3.9527341 {
		t.Fatalf("deliveroo contact: %q %q %v %v", r.Address, r.Phone, r.Lat, r.Lng)
	}
	// phone only in the restaurant object, postcode in its own field
	page := `<script id="__NEXT_DATA__" type="application/json">{"props":{"initialState":{"menuPage":{"menu":{"metas":{"root":{
"restaurant":{"id":"1","name":"Snack Test","location":{"address":{"address1":"5 rue de nimy","postCode":"7000"}},"phoneNumbers":{"primary":"x"},"phone_number":"065 31 31 31"},
"categories":[{"id":"c","name":"Plats"}],"items":[{"id":"i","categoryId":"c","name":"Frites","price":{"fractional":350},"available":true}]}}}}}}}</script>`
	s := cl(ParseDeliverooMenu([]byte(page), "https://deliveroo.be/fr/menu/x/y/snack-test"))
	if s.Phone != "+3265313131" || s.Address != "Rue de Nimy 5, 7000 Mons" {
		t.Fatalf("deliveroo restaurant object: %q %q", s.Phone, s.Address)
	}
}

func TestWeloveatContact(t *testing.T) {
	cl := cleaner(t)
	r, _, err := ParseWeloveatCatalogue(fixture(t, "weloveat_catalogue.json"), "https://weloveat.be/tomo-ramen")
	r = cl(r, err)
	if r.Phone != "+3265352964" || r.Address != "Rue d'Enghien 11, 7000 Mons" {
		t.Fatalf("weloveat contact: %q %q", r.Phone, r.Address)
	}
	raw := []byte(`{"establishment":{"name":"Chez W","phone":"0475/12.34.56","street":"rue grande","street_number":"3","postal_code":"7390","city":"QUAREGNON",
"owner":{"first_name":"Jean","phone":"0499 99 88 77"},"user":{"email":"x@example.invalid"}},
"results":[{"category":{"name":"Plats"},"products":[{"name":"Frites","price":3}]}]}`)
	clean, err := SanitizeWeloveat(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"Jean", "0499", "example.invalid"} {
		if strings.Contains(string(clean), leak) {
			t.Fatalf("personal data kept (%s): %s", leak, clean)
		}
	}
	w, _, err := ParseWeloveatCatalogue(clean, "https://weloveat.be/chez-w")
	w = cl(w, err)
	if w.Phone != "+32475123456" || w.Address != "Rue Grande 3, 7390 Quaregnon" {
		t.Fatalf("weloveat parts: %q %q", w.Phone, w.Address)
	}
	shops, err := ParseWeloveatSearch([]byte(`{"establishments":{"data":[{"name":"A","slug":"a","phone_number":"065 11 22 33","address":"Rue A 1, 7000 Mons, Belgique"}]}}`))
	if err != nil || len(shops) != 1 || shops[0].Phone != "065 11 22 33" || shops[0].Address != "Rue A 1, 7000 Mons" {
		t.Fatalf("search contact: %+v %v", shops, err)
	}
}
