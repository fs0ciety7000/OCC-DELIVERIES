package menusync

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
)

func TestTakeawaySlug(t *testing.T) {
	cases := []struct {
		in   string
		slug string
		ok   bool
	}{
		{"https://www.takeaway.com/be-fr/menu/snack-a-la-gare#pre-order", "snack-a-la-gare", true},
		{"https://www.takeaway.com/be/menu/tomo-mons", "tomo-mons", true},
		{"https://www.takeaway.com/be-nl/menu/cup-pasta-mons?utm=x", "cup-pasta-mons", true},
		{"https://takeaway.com/BE-FR/MENU/La-Frite-Mayo-Mons", "la-frite-mayo-mons", true},
		{"https://www.just-eat.be/fr/menu/pizza-roma-mons", "pizza-roma-mons", true},
		{"https://www.thuisbezorgd.nl/menu/sushi-x", "sushi-x", true},
		{"https://www.takeaway.com/be-fr/eten-bestellen-mons-7000", "", false},
		{"https://www.tomomons.be/menu/foo", "", false},
		{"pas une url", "", false},
	}
	for _, c := range cases {
		slug, ok := TakeawaySlug(c.in)
		if slug != c.slug || ok != c.ok {
			t.Errorf("TakeawaySlug(%q) = %q, %v ; attendu %q, %v", c.in, slug, ok, c.slug, c.ok)
		}
	}
}

func TestDiscoverHosts(t *testing.T) {
	cases := []struct {
		name, slug, city string
		first            []string // expected prefix
		contains         []string
		absent           []string
	}{
		{
			name: "slug simple", slug: "snack-a-la-gare", city: "mons",
			first: []string{"www.snack-a-la-gare.be", "snack-a-la-gare.be", "www.snackalagare.be", "snackalagare.be",
				"www.snack-a-la-gare-mons.be", "snack-a-la-gare-mons.be", "www.snackalagaremons.be"},
			contains: []string{"www.snackalagare-mons.be", "www.snack-a-la-gare.com", "www.snackalagare.com", "www.gare.be", "www.garemons.be"},
		},
		{
			name: "slug avec la ville", slug: "tomo-mons", city: "mons",
			first:    []string{"www.tomo-mons.be", "tomo-mons.be", "www.tomomons.be", "tomomons.be", "www.tomo.be", "tomo.be"},
			contains: []string{"www.tomomons-mons.be", "www.tomo-mons.com", "www.tomomons.com"},
		},
		{
			name: "nom libre court", slug: "tomo", city: "mons",
			first:    []string{"www.tomo.be", "tomo.be", "www.tomo-mons.be", "tomo-mons.be", "www.tomomons.be", "tomomons.be"},
			contains: []string{"www.tomo.com"},
			absent:   []string{"www.tomomons-mons.be"},
		},
		{
			name: "double ville", slug: "la-frite-mayo-mons", city: "mons",
			contains: []string{"www.la-frite-mayo-mons.be", "www.lafritemayomons.be", "www.lafritemayomons-mons.be", "www.lafritemayo.be", "www.frite-mayo.be", "www.fritemayomons.be"},
		},
		{
			name: "autre ville", slug: "cup-pasta", city: "namur",
			first: []string{"www.cup-pasta.be", "cup-pasta.be", "www.cuppasta.be", "cuppasta.be", "www.cup-pasta-namur.be"},
		},
		{
			name: "ville vide = mons", slug: "cup-pasta", city: "",
			contains: []string{"www.cup-pasta-mons.be", "www.cuppastamons.be"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DiscoverHosts(c.slug, c.city)
			if len(got) > 2*MaxDiscoverDomains {
				t.Fatalf("%d hôtes > plafond", len(got))
			}
			seen := map[string]bool{}
			for i, h := range got {
				if seen[h] {
					t.Fatalf("doublon %q dans %v", h, got)
				}
				seen[h] = true
				if i%2 == 0 && !strings.HasPrefix(h, "www.") || i%2 == 1 && got[i-1] != "www."+h {
					t.Fatalf("ordre www./nu cassé à %d : %v", i, got)
				}
			}
			if len(got) < len(c.first) || !slices.Equal(got[:len(c.first)], c.first) {
				t.Fatalf("début %v, attendu %v", got, c.first)
			}
			for _, h := range c.contains {
				if !seen[h] {
					t.Errorf("%q absent de %v", h, got)
				}
			}
			for _, h := range c.absent {
				if seen[h] {
					t.Errorf("%q inattendu dans %v", h, got)
				}
			}
		})
	}
	// the stop-word forms always come after the full ones
	got := DiscoverHosts("pizzeria-da-mario", "mons")
	if i, j := slices.Index(got, "www.pizzeriadamario.com"), slices.Index(got, "www.da-mario.be"); i < 0 || j < 0 || j < i {
		t.Fatalf("variantes sans mots génériques en dernier : %v", got)
	}
	long := DiscoverHosts("le-petit-restaurant-de-la-grand-place-et-du-beffroi", "mons")
	if len(long) == 0 || len(long) > 2*MaxDiscoverDomains {
		t.Fatalf("plafond non appliqué : %d", len(long))
	}
}

func TestPlanDiscovery(t *testing.T) {
	cases := []struct {
		query, kind, slug, site, firstHost, err string
	}{
		{query: "https://www.takeaway.com/be-fr/menu/snack-a-la-gare#pre-order", kind: DiscoverTakeaway, slug: "snack-a-la-gare", firstHost: "www.snack-a-la-gare.be"},
		{query: "takeaway.com/be/menu/tomo-mons", kind: DiscoverTakeaway, slug: "tomo-mons", firstHost: "www.tomo-mons.be"},
		{query: "  Snack à la Gare ", kind: DiscoverName, slug: "snack-a-la-gare", firstHost: "www.snack-a-la-gare.be"},
		{query: "L'Atelier", kind: DiscoverName, slug: "l-atelier", firstHost: "www.l-atelier.be"},
		{query: "Tomo", kind: DiscoverName, slug: "tomo", firstHost: "www.tomo.be"},
		{query: "https://www.tomomons.be/", kind: DiscoverSite, site: "https://www.tomomons.be/", firstHost: "www.tomomons.be"},
		{query: "www.cup-pasta-mons.be", kind: DiscoverSite, site: "https://www.cup-pasta-mons.be/", firstHost: "www.cup-pasta-mons.be"},
		{query: "https://www.lafritemayomons-mons.be/menu#top", kind: DiscoverSite, site: "https://www.lafritemayomons-mons.be/menu", firstHost: "www.lafritemayomons-mons.be"},
		{query: "", err: "indique"},
		{query: "   ", err: "indique"},
		{query: "https://www.takeaway.com/be-fr", err: "sans nom de restaurant"},
		{query: "ftp://x.be", err: "https"},
		{query: "!!!", err: "illisible"},
	}
	for _, c := range cases {
		p, err := PlanDiscovery(c.query, "mons")
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("PlanDiscovery(%q) : erreur %v, attendu %q", c.query, err, c.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("PlanDiscovery(%q) : %v", c.query, err)
			continue
		}
		if p.Kind != c.kind || p.Slug != c.slug || p.SiteURL != c.site || len(p.Hosts) == 0 || p.Hosts[0] != c.firstHost {
			t.Errorf("PlanDiscovery(%q) = %+v", c.query, p)
		}
		if c.kind == DiscoverSite && len(p.Hosts) != 1 {
			t.Errorf("site direct : un seul hôte attendu, %v", p.Hosts)
		}
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLooksLikeTakeawaySite(t *testing.T) {
	site := readFixture(t, "takeaway_site.html")
	cases := []struct {
		name string
		page []byte
		want bool
	}{
		{"satellite", site, true},
		{"deliveroo", readFixture(t, "deliveroo_menu.html"), false},
		{"page vide", []byte("<html><body>Bienvenue</body></html>"), false},
		{"sans référence takeaway", []byte(strings.NewReplacer("takeaway.com", "example.com", "takeaway.css", "x.css").Replace(string(site))), false},
		{"sans microdonnées", []byte(strings.ReplaceAll(string(site), "itemprop", "data-x")), false},
	}
	for _, c := range cases {
		if got := LooksLikeTakeawaySite(c.page); got != c.want {
			t.Errorf("%s : %v, attendu %v", c.name, got, c.want)
		}
	}
}

// discoverServer serves fake candidate hosts through one httptest server:
// the host is the first path segment (URLFor maps host → server/host).
type discoverServer struct {
	*httptest.Server
	hits     atomic.Int32
	requests []string
	uas      []string
}

func newDiscoverServer(t *testing.T, sites map[string]http.HandlerFunc) *discoverServer {
	t.Helper()
	ds := &discoverServer{}
	ds.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ds.hits.Add(1)
		ds.requests = append(ds.requests, r.URL.Path)
		ds.uas = append(ds.uas, r.Header.Get("User-Agent"))
		host, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		h, ok := sites[host]
		if !ok {
			http.NotFound(w, r)
			return
		}
		r.URL.Path = "/" + rest
		h(w, r)
	}))
	t.Cleanup(ds.Close)
	return ds
}

func (ds *discoverServer) prober(resolving ...string) (*Prober, *[]time.Duration) {
	var waits []time.Duration
	p := NewProber()
	p.Lookup = func(_ context.Context, host string) error {
		if slices.Contains(resolving, host) {
			return nil
		}
		return errors.New("no such host")
	}
	p.URLFor = func(host string) string { return ds.URL + "/" + host }
	p.Sleep = func(ctx context.Context, d time.Duration) error { waits = append(waits, d); return ctx.Err() }
	p.Center = Cities["mons"]
	return p, &waits
}

func TestProberDiscover(t *testing.T) {
	site := readFixture(t, "takeaway_site.html")
	page := func(body []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
				return
			}
			_, _ = w.Write(body)
		}
	}
	ds := newDiscoverServer(t, map[string]http.HandlerFunc{
		"www.tomo.be":      page([]byte("<html><title>Tomo, autre chose</title></html>")),
		"www.tomomons.be":  page(site),
		"tomomons.be":      page(site),
		"www.tomo-mons.be": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) },
		"www.tomo.com": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
				return
			}
			t.Error("une page interdite par robots.txt a été demandée")
		},
	})
	p, waits := ds.prober("www.tomo.be", "www.tomo-mons.be", "www.tomomons.be", "tomomons.be", "www.tomo.com")
	plan, err := PlanDiscovery("Tomo", "mons")
	if err != nil {
		t.Fatal(err)
	}
	res := p.Discover(context.Background(), plan)

	status := map[string]string{}
	for _, tr := range res.Tried {
		status[tr.Host] = tr.Status
	}
	want := map[string]string{
		"www.tomo.be": ProbeOther, "tomo.be": ProbeSkipped, "www.tomo-mons.be": ProbeBlocked, "tomo-mons.be": ProbeSkipped,
		"www.tomomons.be": ProbeFound, "tomomons.be": ProbeSkipped, "www.tomo.com": ProbeRobots, "tomo.com": ProbeSkipped,
	}
	for h, s := range want {
		if status[h] != s {
			t.Errorf("%s : %q, attendu %q (%v)", h, status[h], s, res.Tried)
		}
	}
	if len(res.Tried) != len(plan.Hosts) {
		t.Fatalf("%d essais pour %d hôtes", len(res.Tried), len(plan.Hosts))
	}
	if len(res.Found) != 1 {
		t.Fatalf("trouvés : %+v", res.Found)
	}
	f := res.Found[0]
	if f.Name != "Tomo" || f.Host != "www.tomomons.be" || f.Items != 3 || f.Categories != 2 || f.Address == "" || f.DistanceKm == nil || *f.DistanceKm > 3 ||
		f.TakeawayURL != "https://www.takeaway.com/be/menu/tomo-mons" || !strings.HasSuffix(f.URL, "/www.tomomons.be/") {
		t.Fatalf("site trouvé : %+v", f)
	}
	// politeness: robots.txt before every page, identified UA, a pause
	// before every request but the first, nothing sent to absent hosts
	if int(ds.hits.Load()) != p.Network || p.Network != 6 {
		t.Fatalf("requêtes : %d (serveur %d) : %v", p.Network, ds.hits.Load(), ds.requests)
	}
	for i, r := range ds.requests {
		if !strings.HasSuffix(r, "/robots.txt") {
			host, _, _ := strings.Cut(strings.TrimPrefix(r, "/"), "/")
			if i == 0 || ds.requests[i-1] != "/"+host+"/robots.txt" {
				t.Fatalf("robots.txt attendu avant %s : %v", r, ds.requests)
			}
		}
	}
	for _, ua := range ds.uas {
		if ua != UserAgent {
			t.Fatalf("User-Agent %q", ua)
		}
	}
	if len(*waits) != p.Network-1 {
		t.Fatalf("pauses : %d pour %d requêtes", len(*waits), p.Network)
	}
}

func TestProberDirectSiteAndChallenge(t *testing.T) {
	site := readFixture(t, "takeaway_site.html")
	ds := newDiscoverServer(t, map[string]http.HandlerFunc{
		"www.tomomons.be": func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/robots.txt":
				http.NotFound(w, r) // no robots.txt: allowed
			case "/carte":
				_, _ = w.Write(site)
			default:
				http.NotFound(w, r)
			}
		},
		"www.cf.be": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("<html><title>Just a moment...</title></html>"))
		},
	})
	p, _ := ds.prober("www.tomomons.be", "www.cf.be")
	plan, err := PlanDiscovery("https://www.tomomons.be/carte", "mons")
	if err != nil {
		t.Fatal(err)
	}
	res := p.Discover(context.Background(), plan)
	if len(res.Found) != 1 || res.Found[0].Items != 3 || len(res.Tried) != 1 {
		t.Fatalf("site direct : %+v", res)
	}

	n := p.Network
	plan, _ = PlanDiscovery("https://www.cf.be/", "mons")
	res = p.Discover(context.Background(), plan)
	if len(res.Found) != 0 || res.Tried[0].Status != ProbeBlocked || p.Network != n+1 {
		t.Fatalf("défi anti-robot : %+v (%d requêtes)", res, p.Network-n)
	}
}

func TestProberNeverFollowsPlatformRedirects(t *testing.T) {
	ds := newDiscoverServer(t, map[string]http.HandlerFunc{
		"www.redir.be": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				return
			}
			http.Redirect(w, r, "https://www.takeaway.com/be-fr/menu/redir", http.StatusFound)
		},
	})
	p, _ := ds.prober("www.redir.be")
	res := p.Discover(context.Background(), DiscoverPlan{Kind: DiscoverName, Hosts: []string{"www.redir.be"}})
	if res.Tried[0].Status != ProbeUnreachable || !strings.Contains(res.Tried[0].Message, "jamais suivie") {
		t.Fatalf("redirection : %+v", res.Tried)
	}
}
