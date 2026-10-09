package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
)

// discoverWeb fakes the candidate hosts of a discovery (the first path
// segment is the host): no real DNS, no real network.
type discoverWeb struct {
	*httptest.Server
	hits    atomic.Int32
	release chan struct{}
}

var discoverResolving = []string{"www.tomo.be", "www.tomomons.be", "tomomons.be", "www.slow.be"}

func newDiscoverWeb(t *testing.T) *discoverWeb {
	t.Helper()
	site, err := os.ReadFile(filepath.Join("..", "menusync", "testdata", "takeaway_site.html"))
	if err != nil {
		t.Fatal(err)
	}
	dw := &discoverWeb{release: make(chan struct{})}
	dw.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dw.hits.Add(1)
		if r.Header.Get("User-Agent") != menusync.UserAgent {
			t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
		}
		host, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		switch {
		case rest == "robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case host == "www.tomomons.be" || host == "tomomons.be":
			_, _ = w.Write(site)
		case host == "www.slow.be":
			<-dw.release
			_, _ = w.Write(site)
		case host == "www.tomo.be":
			_, _ = w.Write([]byte("<html><title>Autre Tomo</title><body>Bienvenue</body></html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(dw.Close)
	t.Cleanup(func() {
		select {
		case <-dw.release:
		default:
			close(dw.release)
		}
	})
	return dw
}

func newDiscoverEnv(t *testing.T, dw *discoverWeb) *syncEnv {
	t.Helper()
	return newSyncEnvWith(t, func(c *SyncConfig) {
		c.NewProber = func() *menusync.Prober {
			p := menusync.NewProber()
			p.Lookup = func(_ context.Context, host string) error {
				if slices.Contains(discoverResolving, host) {
					return nil
				}
				return errors.New("no such host")
			}
			p.URLFor = func(host string) string { return dw.URL + "/" + host }
			p.Sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() }
			return p
		}
	})
}

func TestSyncDiscoverAuthz(t *testing.T) {
	dw := newDiscoverWeb(t)
	e := newDiscoverEnv(t, dw)
	bob := e.user("Bob")
	for _, rt := range []struct {
		url  string
		body any
	}{
		{"/api/occ/admin/sync/discover", map[string]any{"query": "Tomo"}},
		{"/api/occ/admin/sync/sources", map[string]any{"url": "https://www.tomomons.be/"}},
		{"/api/occ/admin/sync/run", map[string]any{"sourceId": "x"}},
	} {
		e.expect(401, "POST", rt.url, "", rt.body)
		e.expect(403, "POST", rt.url, bob.token, rt.body)
	}
	if dw.hits.Load() != 0 {
		t.Fatal("aucune requête sortante sans droits")
	}

	// disabled synchronisation: no outgoing request at all
	off := newEnv(t)
	admin := off.admin("Root")
	off.expect(400, "POST", "/api/occ/admin/sync/discover", admin.token, map[string]any{"query": "Tomo"})
}

func TestSyncDiscoverAddAndRunOneSource(t *testing.T) {
	dw := newDiscoverWeb(t)
	e := newDiscoverEnv(t, dw)
	srv := newFeedServer(t)
	e.setSources(
		map[string]any{"provider": "takeaway-site", "label": "Lent", "url": srv.URL + "/slow/", "priority": 10, "enabled": false},
		map[string]any{"provider": "deliveroo", "label": "Deliveroo", "url": srv.URL + "/fr/restaurants/brussels/mons-center?geohash=x", "priority": 50},
	)
	admin := e.admin("Root")

	// validation
	for _, q := range []string{"", "   ", "https://www.takeaway.com/be-fr", "ftp://x.be"} {
		e.expect(400, "POST", "/api/occ/admin/sync/discover", admin.token, map[string]any{"query": q})
	}
	e.expect(400, "POST", "/api/occ/admin/sync/discover", admin.token, map[string]any{"query": strings.Repeat("a", 301)})

	// a full run first: Baalbeck comes from Deliveroo
	if run := e.runSync(triggerManual); run.Status != runSuccess || run.Stats.RestaurantsCreated != 1 {
		t.Fatalf("première exécution %+v\n%s", run, run.Log)
	}

	// discovery from the takeaway.com link: takeaway.com is never requested
	var res struct {
		Kind  string `json:"kind"`
		Slug  string `json:"slug"`
		Tried []menusync.DiscoverTry
		Found []discoverFound
	}
	e.expect(200, "POST", "/api/occ/admin/sync/discover", admin.token,
		map[string]any{"query": "https://www.takeaway.com/be-fr/menu/tomo-mons#pre-order"}).json(t, &res)
	if res.Kind != menusync.DiscoverTakeaway || res.Slug != "tomo-mons" || len(res.Found) != 1 {
		t.Fatalf("découverte %+v", res)
	}
	f := res.Found[0]
	if f.Name != "Tomo" || f.Items != 3 || f.Categories != 2 || f.AlreadySource || f.DistanceKm == nil || f.Host != "www.tomomons.be" {
		t.Fatalf("site trouvé %+v", f)
	}
	status := map[string]string{}
	for _, tr := range res.Tried {
		status[tr.Host] = tr.Status
	}
	if status["www.tomo-mons.be"] != menusync.ProbeAbsent || status["www.tomomons.be"] != menusync.ProbeFound ||
		status["tomomons.be"] != menusync.ProbeSkipped || status["www.tomo.be"] != menusync.ProbeOther {
		t.Fatalf("essais %v", res.Tried)
	}

	// add it: first free priority in 10–39, label = parsed name; idempotent
	e.expect(400, "POST", "/api/occ/admin/sync/sources", admin.token, map[string]any{"url": "https://www.takeaway.com/be/menu/tomo-mons"})
	e.expect(400, "POST", "/api/occ/admin/sync/sources", admin.token, map[string]any{"url": "javascript:alert(1)"})
	var added struct {
		Source  map[string]any `json:"source"`
		Created bool           `json:"created"`
	}
	e.expect(201, "POST", "/api/occ/admin/sync/sources", admin.token, map[string]any{"url": f.URL, "label": f.Name}).json(t, &added)
	src := added.Source
	if !added.Created || src["provider"] != "takeaway-site" || src["label"] != "Tomo" || src["priority"].(float64) != 11 ||
		src["enabled"] != true || src["city"] != "mons" || src["url"] != f.URL {
		t.Fatalf("source ajoutée %v", added)
	}
	count, _ := e.app.CountRecords(colSyncSources)
	again := e.expect(200, "POST", "/api/occ/admin/sync/sources", admin.token, map[string]any{"url": strings.ToUpper(f.URL[:4]) + f.URL[4:] + "?x=1#y"}).m(t)
	if again["created"] != false || again["source"].(map[string]any)["id"] != src["id"] {
		t.Fatalf("ajout idempotent %v", again)
	}
	if n, _ := e.app.CountRecords(colSyncSources); n != count {
		t.Fatalf("doublon de source : %d → %d", count, n)
	}
	e.expect(200, "POST", "/api/occ/admin/sync/discover", admin.token, map[string]any{"query": "https://www.tomomons.be/"}).json(t, &res)
	if len(res.Found) != 1 || !res.Found[0].AlreadySource || res.Found[0].SourceID != src["id"] || len(res.Tried) != 1 {
		t.Fatalf("site déjà suivi %+v", res)
	}

	// run only that source
	e.expect(404, "POST", "/api/occ/admin/sync/run", admin.token, map[string]any{"sourceId": "inconnu"})
	started := e.expect(202, "POST", "/api/occ/admin/sync/run", admin.token, map[string]any{"sourceId": src["id"]}).m(t)
	e.waitIdle()
	runID := started["run"].(map[string]any)["id"].(string)
	run := e.expect(200, "GET", "/api/occ/admin/sync/runs/"+runID, admin.token, nil).m(t)["run"].(map[string]any)
	srcs := run["sources"].([]any)
	if run["status"] != runSuccess || len(srcs) != 1 || srcs[0].(map[string]any)["id"] != src["id"] {
		t.Fatalf("exécution ciblée %v", run)
	}
	stats := run["stats"].(map[string]any)
	if stats["restaurants_created"].(float64) != 1 || stats["restaurants_stale"].(float64) != 0 || !strings.Contains(run["log"].(string), "ciblée") {
		t.Fatalf("stats %v\n%s", stats, run["log"])
	}
	if len(e.restaurantsNamed("Tomo")) != 1 {
		t.Fatal("Tomo doit être créé")
	}
	baalbeck, err := e.app.FindFirstRecordByFilter(colRestaurants, "sources ~ 'deliveroo'")
	if err != nil || baalbeck.GetString("stale_since") != "" {
		t.Fatalf("une exécution ciblée ne rend rien obsolète : %v", err)
	}

	// single-run lock: a scoped run holds it too
	slow := e.expect(201, "POST", "/api/occ/admin/sync/sources", admin.token, map[string]any{"url": dw.URL + "/www.slow.be/"}).m(t)
	slowID := slow["source"].(map[string]any)["id"]
	if p := slow["source"].(map[string]any)["priority"].(float64); p != 12 {
		t.Fatalf("priorité suivante %v", p)
	}
	e.expect(202, "POST", "/api/occ/admin/sync/run", admin.token, map[string]any{"sourceId": slowID})
	e.expect(409, "POST", "/api/occ/admin/sync/run", admin.token, map[string]any{"sourceId": src["id"]})
	e.expect(409, "POST", "/api/occ/admin/sync/run", admin.token, nil)
	close(dw.release)
	e.waitIdle()
}

func TestSyncDiscoverOneAtATime(t *testing.T) {
	dw := newDiscoverWeb(t)
	e := newDiscoverEnv(t, dw)
	admin := e.admin("Root")
	done := make(chan resp)
	go func() {
		done <- e.do("POST", "/api/occ/admin/sync/discover", admin.token, map[string]any{"query": "https://www.slow.be/"})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for dw.hits.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("la découverte n'a pas atteint la page lente")
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.expect(409, "POST", "/api/occ/admin/sync/discover", admin.token, map[string]any{"query": "Tomo"})
	close(dw.release)
	if r := <-done; r.status != 200 {
		t.Fatalf("découverte %d %s", r.status, r.body)
	}
}
