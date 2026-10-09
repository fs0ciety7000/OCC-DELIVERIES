package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/enrich"
)

const osmPizzaHut = `[{"osm_type":"node","osm_id":4242,"lat":"50.4545","lon":"3.9520","category":"amenity","type":"fast_food",
"name":"Pizza Hut","address":{"house_number":"12","road":"Rue de la Chaussée","city":"Mons","postcode":"7000"},
"extratags":{"contact:phone":"+32 65 84 11 22"},"namedetails":{"name":"Pizza Hut"}}]`

const osmTomo = `[{"osm_type":"way","osm_id":99,"lat":"50.45437","lon":"3.95121","category":"amenity","type":"restaurant",
"name":"Tomo","address":{"house_number":"11","road":"Rue d'Enghien","city":"Mons","postcode":"7000"},
"extratags":{"phone":"065 35 29 64"},"namedetails":{"name":"Tomo"}}]`

// osmServer is a fake Nominatim recording the queries.
type osmServer struct {
	*httptest.Server
	mu      sync.Mutex
	queries []string
}

func newOSMServer(t *testing.T, answers map[string]string) *osmServer {
	t.Helper()
	s := &osmServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.queries = append(s.queries, r.URL.Query().Get("q"))
		s.mu.Unlock()
		if r.UserAgent() != enrich.UserAgent {
			http.Error(w, "unidentified", http.StatusForbidden)
			return
		}
		body, ok := answers[r.URL.Query().Get("q")]
		if !ok {
			body = "[]"
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *osmServer) asked(q string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.queries {
		if x == q {
			return true
		}
	}
	return false
}

func newRestaurant(t *testing.T, app core.App, fields map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(colRestaurants)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Load(map[string]any{"price_level": 2, "active": true})
	r.Load(fields)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSyncEnrichesFromOpenStreetMap(t *testing.T) {
	osm := newOSMServer(t, map[string]string{"Pizza Hut, Mons": osmPizzaHut, "Tomo, Mons": osmTomo, "Verrouillé Pizza, Mons": osmPizzaHut})
	var sleeps []time.Duration
	e := newSyncEnvWith(t, func(c *SyncConfig) {
		c.Enrich = EnrichConfig{Enabled: true, NewClient: func(dir string) *enrich.Client {
			cl := enrich.NewClient(dir)
			cl.Endpoint = osm.URL + "/search"
			cl.Sleep = func(ctx context.Context, d time.Duration) error { sleeps = append(sleeps, d); return ctx.Err() }
			return cl
		}}
	})
	srv := newFeedServer(t)
	e.setSources(map[string]any{"provider": "takeaway-site", "label": "Tomo", "url": srv.URL + "/tomo/", "priority": 10})
	// an Uber Eats preview: no phone, no address, approximate position
	ph := newRestaurant(t, e.app, map[string]any{"name": "Pizza Hut", "slug": "pizza-hut", "partial_menu": true, "geo_approx": true, "lat": 50.4542, "lng": 3.9567})
	locked := newRestaurant(t, e.app, map[string]any{"name": "Verrouillé Pizza", "slug": "verrouille", "locked": true})
	inactive := newRestaurant(t, e.app, map[string]any{"name": "Fermé", "slug": "ferme", "active": false})

	run := e.runSync(triggerManual)
	if run.Status != runSuccess || run.Stats.RestaurantsEnriched != 2 {
		t.Fatalf("run %s, stats %+v\n%s", run.Status, run.Stats, run.Log)
	}
	changes := strings.Join(run.Changes, "\n")
	if !strings.Contains(changes, "Pizza Hut — téléphone ajouté, adresse ajoutée, position précisée (OpenStreetMap)") ||
		!strings.Contains(changes, "Tomo — téléphone ajouté (OpenStreetMap)") {
		t.Fatalf("changes:\n%s", changes)
	}
	r, _ := e.app.FindRecordById(colRestaurants, ph.Id)
	if r.GetString("phone") != "+3265841122" || r.GetString("address") != "Rue de la Chaussée 12, 7000 Mons" || r.GetBool("geo_approx") ||
		r.GetFloat("lat") != 50.4545 || r.GetFloat("lng") != 3.952 {
		t.Fatalf("pizza hut %v", r.FieldsData())
	}
	var a enrich.Attribution
	if err := r.UnmarshalJSONField("enriched_from", &a); err != nil || a.Provider != "osm" || a.URL != "https://www.openstreetmap.org/node/4242" ||
		strings.Join(a.Fields, ",") != "phone,address,geo" || a.CheckedAt == "" {
		t.Fatalf("attribution %+v %v", a, err)
	}
	// Tomo (from its site): address and coordinates kept, only the phone added
	tomo := e.restaurantsNamed("Tomo")[0]
	if tomo.GetString("phone") != "+3265352964" || tomo.GetString("address") != "Rue d’Enghien 11, 7000 Mons" || tomo.GetFloat("lat") != 50.4543472 {
		t.Fatalf("tomo %v", tomo.FieldsData())
	}
	if !strings.Contains(tomo.GetString("enriched_from"), `"fields":["phone"]`) {
		t.Fatalf("tomo attribution %s", tomo.GetString("enriched_from"))
	}
	// locked and inactive restaurants: never looked up, never touched
	if osm.asked("Verrouillé Pizza, Mons") || osm.asked("Fermé, Mons") {
		t.Fatalf("queries %v", osm.queries)
	}
	for _, id := range []string{locked.Id, inactive.Id} {
		if x, _ := e.app.FindRecordById(colRestaurants, id); x.GetString("phone") != "" || x.GetString("enriched_from") != "null" && x.GetString("enriched_from") != "" {
			t.Fatalf("touched %v", x.FieldsData())
		}
	}
	// polite: one pause (≥ 1 s) between the two requests
	if len(sleeps) != 1 || sleeps[0] < time.Second { // MinInterval minus the time already elapsed
		t.Fatalf("pauses %v", sleeps)
	}

	// second run: nothing left to do, answers from the 30-day cache
	n := len(osm.queries)
	run = e.runSync(triggerManual)
	if run.Stats.RestaurantsEnriched != 0 || len(osm.queries) != n {
		t.Fatalf("second run: %+v, %d → %d queries\n%s", run.Stats, n, len(osm.queries), run.Log)
	}

	// an admin edit of an OSM field drops its attribution; contact normalized
	admin := e.admin("Root")
	got := e.expect(200, "PATCH", "/api/collections/restaurants/records/"+ph.Id, admin.token, map[string]any{
		"phone": "0475/12.34.56", "address": "3 rue de nimy, 7000", "enriched_from": map[string]any{"provider": "x", "url": "https://evil.example", "fields": []string{"phone"}},
	}).m(t)
	if got["phone"] != "+32475123456" || got["address"] != "Rue de Nimy 3, 7000 Mons" {
		t.Fatalf("normalized %v", got)
	}
	r, _ = e.app.FindRecordById(colRestaurants, ph.Id)
	if err := r.UnmarshalJSONField("enriched_from", &a); err != nil || a.URL != "https://www.openstreetmap.org/node/4242" || strings.Join(a.Fields, ",") != "geo" {
		t.Fatalf("attribution after edit %+v", a)
	}
	e.expect(400, "PATCH", "/api/collections/restaurants/records/"+ph.Id, admin.token, map[string]any{"phone": "pas de numéro"})
}

func TestSyncEnrichDisabled(t *testing.T) {
	osm := newOSMServer(t, map[string]string{"Pizza Hut, Mons": osmPizzaHut})
	e := newSyncEnvWith(t, func(c *SyncConfig) {
		c.Enrich = EnrichConfig{Enabled: false, NewClient: func(dir string) *enrich.Client {
			cl := enrich.NewClient(dir)
			cl.Endpoint = osm.URL + "/search"
			return cl
		}}
	})
	srv := newFeedServer(t)
	e.setSources(map[string]any{"provider": "takeaway-site", "label": "Tomo", "url": srv.URL + "/tomo/", "priority": 10})
	newRestaurant(t, e.app, map[string]any{"name": "Pizza Hut", "slug": "pizza-hut"})
	if run := e.runSync(triggerManual); run.Stats.RestaurantsEnriched != 0 || len(osm.queries) != 0 {
		t.Fatalf("disabled enrichment ran: %+v %v", run.Stats, osm.queries)
	}
	t.Setenv("OCC_ENRICH_ENABLED", "")
	if !syncConfigFromEnv().Enrich.Enabled {
		t.Fatal("OCC_ENRICH_ENABLED must default to true")
	}
	t.Setenv("OCC_ENRICH_ENABLED", "false")
	if syncConfigFromEnv().Enrich.Enabled {
		t.Fatal("OCC_ENRICH_ENABLED=false must disable")
	}
}
