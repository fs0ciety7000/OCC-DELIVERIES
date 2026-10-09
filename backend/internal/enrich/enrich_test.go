package enrich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var mons = Area{Lat: 50.4542, Lng: 3.9567, City: "mons", Label: "Mons"}

func place(name, cat, typ string, lat, lng float64) Place {
	return Place{OSMType: "node", OSMID: 42, Category: cat, Type: typ, Name: name, Names: []string{name}, Lat: lat, Lng: lng,
		Phone: "+32 65 84 11 22", Road: "Rue de la Chaussée", HouseNumber: "12", Postcode: "7000", Locality: "Mons"}
}

func TestPick(t *testing.T) {
	near := place("Pizza Hut", "amenity", "restaurant", 50.4545, 3.9520)
	cases := []struct {
		name   string
		target Target
		places []Place
		want   bool
		why    string
	}{
		{"exact name, no geo", Target{Name: "Pizza Hut"}, []Place{near}, true, ""},
		{"store suffix ignored", Target{Name: "Pizza Hut Mons (Independant)"}, []Place{near}, true, ""},
		{"alt name (brand)", Target{Name: "Domino's"}, []Place{{OSMType: "way", OSMID: 7, Category: "amenity", Type: "fast_food", Name: "Domino's Pizza Mons", Names: []string{"Domino's Pizza Mons", "Domino's"}, Lat: 50.45, Lng: 3.95}}, true, ""},
		{"not food", Target{Name: "Pizza Hut"}, []Place{place("Pizza Hut", "office", "company", 50.4545, 3.9520)}, false, "aucun établissement"},
		{"other name", Target{Name: "Adriatica"}, []Place{near}, false, "aucun établissement"},
		{"too far from Mons", Target{Name: "Pizza Hut"}, []Place{place("Pizza Hut", "amenity", "restaurant", 50.85, 4.35)}, false, "trop loin"},
		{"too far from known coords", Target{Name: "Pizza Hut", Lat: 50.40, Lng: 3.90}, []Place{near}, false, "position connue"},
		{"approx coords ignored", Target{Name: "Pizza Hut", Lat: 50.40, Lng: 3.90, GeoApprox: true}, []Place{near}, true, ""},
		{"bar needs strong name", Target{Name: "Snack Pitta Grill Akropolis"}, []Place{place("Snack Pitta Grec Akropolis", "amenity", "bar", 50.4545, 3.9520)}, false, "aucun établissement"},
		{"restaurant with similar name", Target{Name: "Snack Pitta Grill Akropolis"}, []Place{place("Snack Pitta Grec Akropolis", "amenity", "fast_food", 50.4545, 3.9520)}, true, ""},
		{"bar same name", Target{Name: "Le Central"}, []Place{place("Le Central", "amenity", "bar", 50.4545, 3.9520)}, true, ""},
		{"bakery shop", Target{Name: "Boulangerie Dupont"}, []Place{place("Boulangerie Dupont", "shop", "bakery", 50.4545, 3.9520)}, true, ""},
		{"other street", Target{Name: "Pizza Hut", Address: "Rue de Nimy 3, 7000 Mons"}, []Place{near}, false, "autre rue"},
		{"same street", Target{Name: "Pizza Hut", Address: "Rue de la Chaussée 12, 7000 Mons"}, []Place{near}, true, ""},
		{"two branches far apart, no geo", Target{Name: "Pizza Hut"}, []Place{near, place("Pizza Hut", "amenity", "restaurant", 50.43, 3.85)}, false, "ambigu"},
		{"two branches, real coords pick nearest", Target{Name: "Pizza Hut", Lat: 50.4300, Lng: 3.8510}, []Place{near, place("Pizza Hut", "amenity", "restaurant", 50.43, 3.85)}, true, ""},
		{"node + building at the same spot", Target{Name: "Pizza Hut"}, []Place{near, place("Pizza Hut", "amenity", "restaurant", 50.4546, 3.9521)}, true, ""},
		{"no result", Target{Name: "X"}, nil, false, "aucun résultat"},
	}
	for _, c := range cases {
		p, why := Pick(c.target, c.places, mons)
		if (why == "") != c.want || (c.why != "" && !strings.Contains(why, c.why)) {
			t.Errorf("%s: got %+v / %q", c.name, p.Name, why)
		}
	}
}

func TestFillOnlyEmpty(t *testing.T) {
	p := place("Pizza Hut", "amenity", "restaurant", 50.4545, 3.9520)
	cases := []struct {
		name   string
		target Target
		fields string
	}{
		{"everything missing", Target{Name: "Pizza Hut"}, "phone,address,geo"},
		{"approx position", Target{Name: "Pizza Hut", Phone: "+3265000001", Address: "Rue X 1, 7000 Mons", Lat: 50.4542, Lng: 3.9567, GeoApprox: true}, "geo"},
		{"complete", Target{Name: "Pizza Hut", Phone: "+3265000001", Address: "Rue X 1, 7000 Mons", Lat: 50.45, Lng: 3.95}, ""},
		{"phone only", Target{Name: "Pizza Hut", Address: "Rue X 1, 7000 Mons", Lat: 50.45, Lng: 3.95}, "phone"},
	}
	for _, c := range cases {
		patch := Fill(c.target, p)
		if got := strings.Join(patch.Fields, ","); got != c.fields {
			t.Errorf("%s: fields %q, want %q", c.name, got, c.fields)
		}
	}
	all := Fill(Target{ID: "r1", Name: "Pizza Hut"}, p)
	if all.Phone != "+3265841122" || all.Address != "Rue de la Chaussée 12, 7000 Mons" || !all.SetGeo || all.Lat != 50.4545 || all.URL != "https://www.openstreetmap.org/node/42" {
		t.Fatalf("patch %+v", all)
	}
	if got := all.Change(); got != "Pizza Hut — téléphone ajouté, adresse ajoutée, position précisée (OpenStreetMap)" {
		t.Fatalf("change %q", got)
	}
	junk := p
	junk.Phone = "n/a"
	junk.Road = ""
	if f := Fill(Target{Name: "Pizza Hut", Lat: 50.45, Lng: 3.95}, junk); !f.Empty() {
		t.Fatalf("junk filled: %+v", f)
	}
}

func TestQuery(t *testing.T) {
	cases := map[string]Target{
		"Pizza Hut, Mons":         {Name: "Pizza Hut Mons"},
		"CTR Chicken, Mons":       {Name: "CTR Chicken Mons (Independant)"},
		"Snack Atlas, Jemappes":   {Name: "Snack Atlas", Address: "Rue de Mons 3, 7012"},
		"Altaj, Quaregnon":        {Name: "Altaj", Address: "Rue X 1, 7390 Quaregnon"},
		"Chez Paul, Mons":         {Name: "Chez Paul", Address: "Rue sans code postal 4"},
		"Le Coin, Saint-Ghislain": {Name: "Le Coin", Address: "Av. de l'Enseignement 1, 7330 Saint-Ghislain"},
	}
	for want, tg := range cases {
		if got := Query(tg, mons); got != want {
			t.Errorf("Query(%q) = %q, want %q", tg.Name, got, want)
		}
	}
}

const nominatimHit = `[{"place_id":1,"osm_type":"node","osm_id":4242,"lat":"50.4545","lon":"3.9520","category":"amenity","type":"fast_food",
"name":"Pizza Hut","display_name":"Pizza Hut, 12, Rue de la Chaussée, Mons","address":{"house_number":"12","road":"Rue de la Chaussée","city":"Mons","postcode":"7000","country_code":"be"},
"extratags":{"contact:phone":"+32 65 84 11 22","opening_hours":"Mo-Su 11:00-22:00"},"namedetails":{"name":"Pizza Hut","brand":"Pizza Hut"}}]`

// fakeNominatim answers like Nominatim and records the requests.
type fakeNominatim struct {
	*httptest.Server
	mu       sync.Mutex
	queries  []string
	agents   []string
	status   int
	answers  map[string]string // query → body (default "[]")
	requests int
}

func newFake(t *testing.T) *fakeNominatim {
	f := &fakeNominatim{answers: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		q := r.URL.Query()
		f.queries = append(f.queries, q.Get("q"))
		f.agents = append(f.agents, r.UserAgent())
		if q.Get("format") != "jsonv2" || q.Get("countrycodes") != "be" || q.Get("extratags") != "1" || q.Get("addressdetails") != "1" || q.Get("namedetails") != "1" {
			http.Error(w, "bad params", http.StatusBadRequest)
			return
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		body, ok := f.answers[q.Get("q")]
		if !ok {
			body = "[]"
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

// fakeClock is a manual clock: Sleep advances it and records the pauses.
type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *fakeClock) client(endpoint, dir string) *Client {
	cl := NewClient(dir)
	cl.Endpoint = endpoint
	cl.Now = func() time.Time { return c.now }
	cl.Sleep = func(ctx context.Context, d time.Duration) error {
		c.sleeps = append(c.sleeps, d)
		c.now = c.now.Add(d)
		return ctx.Err()
	}
	return cl
}

func TestEnrichPoliteAndCached(t *testing.T) {
	fk := newFake(t)
	fk.answers["Pizza Hut, Mons"] = nominatimHit
	dir := t.TempDir()
	clock := &fakeClock{now: time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)}
	c := clock.client(fk.URL, dir)
	c.Interval = 10 * time.Millisecond // clamped to MinInterval

	targets := []Target{
		{ID: "a", Name: "Pizza Hut", Lat: 50.4542, Lng: 3.9567, GeoApprox: true},
		{ID: "b", Name: "Inconnu du Bataillon"},
		{ID: "c", Name: "Complet", Phone: "+3265000001", Address: "Rue X 1, 7000 Mons", Lat: 50.45, Lng: 3.95},
	}
	res := c.Enrich(context.Background(), targets, Options{Area: mons})
	if fk.requests != 2 || res.Candidates != 2 || res.Looked != 2 || len(res.Patches) != 1 {
		t.Fatalf("requests %d, result %+v", fk.requests, res)
	}
	for _, ua := range fk.agents {
		if ua != UserAgent || !strings.Contains(ua, "https://eat.fs0ciety.org") {
			t.Fatalf("user agent %q", ua)
		}
	}
	// one pause between the two requests, at least MinInterval
	if len(clock.sleeps) != 1 || clock.sleeps[0] < MinInterval {
		t.Fatalf("pauses %v", clock.sleeps)
	}
	p := res.Patches[0]
	if p.ID != "a" || p.Phone != "+3265841122" || p.Address != "Rue de la Chaussée 12, 7000 Mons" || !p.SetGeo || p.URL != "https://www.openstreetmap.org/node/4242" {
		t.Fatalf("patch %+v", p)
	}

	// second run: hits and misses come from the cache, no request at all
	c2 := clock.client(fk.URL, dir)
	res2 := c2.Enrich(context.Background(), targets, Options{Area: mons})
	if fk.requests != 2 || c2.Network != 0 || c2.Cached != 2 || len(res2.Patches) != 1 {
		t.Fatalf("cached run: requests %d, network %d, cached %d, %+v", fk.requests, c2.Network, c2.Cached, res2)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "nominatim")); len(entries) != 2 {
		t.Fatalf("cache files: %d", len(entries))
	}

	// after the TTL, the answers are fetched again
	clock.now = clock.now.Add(DefaultCacheTTL + time.Hour)
	c3 := clock.client(fk.URL, dir)
	c3.Enrich(context.Background(), targets[:1], Options{Area: mons})
	if c3.Network != 1 {
		t.Fatalf("expired cache: network %d", c3.Network)
	}
}

func TestEnrichCapAndStop(t *testing.T) {
	fk := newFake(t)
	clock := &fakeClock{now: time.Now()}
	var targets []Target
	for _, n := range []string{"Alpha Grill", "Beta Wok", "Gamma Kebab", "Delta Tacos"} {
		targets = append(targets, Target{ID: n, Name: n})
	}
	c := clock.client(fk.URL, t.TempDir())
	res := c.Enrich(context.Background(), targets, Options{Area: mons, MaxLookups: 2})
	if fk.requests != 2 || res.Deferred != 2 || res.Looked != 2 {
		t.Fatalf("cap: requests %d, %+v", fk.requests, res)
	}

	fk2 := newFake(t)
	fk2.status = http.StatusTooManyRequests
	c2 := clock.client(fk2.URL, t.TempDir())
	res2 := c2.Enrich(context.Background(), targets, Options{Area: mons})
	if fk2.requests != 1 || res2.Stopped == "" || res2.Deferred != 4 || !strings.Contains(res2.Stopped, "429") {
		t.Fatalf("stop on 429: requests %d, %+v", fk2.requests, res2)
	}
	// a refusal is never cached
	fk2.status = 0
	c3 := clock.client(fk2.URL, c2.CacheDir)
	c3.Enrich(context.Background(), targets[:1], Options{Area: mons})
	if c3.Network != 1 {
		t.Fatalf("refusal cached: network %d", c3.Network)
	}
}

func TestParsePlaces(t *testing.T) {
	ps, err := ParsePlaces([]byte(nominatimHit))
	if err != nil || len(ps) != 1 {
		t.Fatalf("%v %v", ps, err)
	}
	p := ps[0]
	if p.OSMType != "node" || p.OSMID != 4242 || p.Category != "amenity" || p.Type != "fast_food" || p.Phone != "+32 65 84 11 22" ||
		p.Road != "Rue de la Chaussée" || p.HouseNumber != "12" || p.Postcode != "7000" || p.Locality != "Mons" || p.Lat != 50.4545 {
		t.Fatalf("place %+v", p)
	}
	if strings.Join(p.Names, "|") != "Pizza Hut" {
		t.Fatalf("names %v", p.Names)
	}
	if _, err := ParsePlaces([]byte("<html>")); err == nil {
		t.Fatal("invalid answer accepted")
	}
}
