package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestRobotsTag(t *testing.T) {
	cases := map[string]string{
		"/":                          "",
		"/index.html":                "",
		"/assets/index-abc123.js":    "",
		"/og/og-image.jpg":           "",
		"/manifest.webmanifest":      "",
		"/login":                     robotsNoIndex,
		"/register":                  robotsNoIndex,
		"/restaurants":               robotsNoIndex,
		"/restaurants/abc123":        robotsNoIndex,
		"/page-inconnue":             robotsNoIndex,
		"/j/ABC123":                  robotsNoIndexNoFollow,
		"/e/ABCD1234":                robotsNoIndexNoFollow,
		"/party/abc":                 robotsNoIndexNoFollow,
		"/equipes/abc":               robotsNoIndexNoFollow,
		"/profile":                   robotsNoIndexNoFollow,
		"/admin":                     robotsNoIndexNoFollow,
		"/admin/restaurants":         robotsNoIndexNoFollow,
		"/api/occ/health":            robotsNoIndexNoFollow,
		"/_/":                        robotsNoIndexNoFollow,
		"/auth/verifier/eyJ.abc.def": robotsNoIndexNoFollow, // JWT with dots: not a static file
	}
	for p, want := range cases {
		if got := robotsTag(p); got != want {
			t.Errorf("robotsTag(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestRobotsTxtKeepsInvitesCrawlable(t *testing.T) {
	for _, d := range []string{"Disallow: /api/", "Disallow: /_/", "Disallow: /admin", "Disallow: /party/"} {
		if !strings.Contains(robotsTxt, d) {
			t.Errorf("robots.txt sans %q", d)
		}
	}
	for _, d := range []string{"Disallow: /j/", "Disallow: /e/", "Disallow: /\n"} {
		if strings.Contains(robotsTxt, d) {
			t.Errorf("robots.txt ne doit pas contenir %q (aperçus de liens)", d)
		}
	}
}

const testIndex = `<html><head><title>OCC Deliveries — accueil</title>
<meta name="description" content="Accueil" />
<link rel="canonical" href="https://eat.example/" />
<meta property="og:url" content="https://eat.example/" />
<meta property="og:title" content="Accueil" />
<meta property="og:description" content="Accueil" />
<meta name="twitter:title" content="Accueil" />
<meta name="twitter:description" content="Accueil" />
</head><body><div id="root"></div></body></html>`

func TestInviteHTML(t *testing.T) {
	out := string(inviteHTML([]byte(testIndex), inviteMeta{title: `A "B" $1`, description: "D & E"}, "https://eat.example/j/X$1"))
	for _, want := range []string{
		`<title>A &#34;B&#34; $1</title>`,
		`<meta property="og:title" content="A &#34;B&#34; $1" />`,
		`<meta name="twitter:title" content="A &#34;B&#34; $1" />`,
		`<meta name="description" content="D &amp; E" />`,
		`<meta property="og:description" content="D &amp; E" />`,
		`<meta name="twitter:description" content="D &amp; E" />`,
		`<meta property="og:url" content="https://eat.example/j/X$1" />`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("inviteHTML: %q absent de\n%s", want, out)
		}
	}
	if strings.Contains(out, "canonical") || strings.Contains(out, "Accueil") {
		t.Errorf("inviteHTML: canonical ou ancien texte restant\n%s", out)
	}
}

func TestSEORoutes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(testIndex), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCC_PUBLIC_DIR", dir)

	factory := func(t testing.TB) *tests.TestApp {
		app, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		app.Settings().Meta.AppURL = "https://eat.example"
		h := spaHandler()
		app.OnServe().Bind(&h)
		return app
	}
	header := func(name, want string) func(testing.TB, *tests.TestApp, *http.Response) {
		return func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			if got := res.Header.Get(name); got != want {
				t.Errorf("%s = %q, want %q", name, got, want)
			}
		}
	}

	scenarios := []tests.ApiScenario{
		{
			Name: "robots.txt réel", Method: http.MethodGet, URL: "/robots.txt", TestAppFactory: factory,
			ExpectedStatus: 200, ExpectedContent: []string{"User-agent: *", "Disallow: /api/"}, NotExpectedContent: []string{"<html"},
			AfterTestFunc: header("Content-Type", "text/plain; charset=utf-8"),
		},
		{
			Name: "sitemap absent = 404 (pas la SPA)", Method: http.MethodGet, URL: "/sitemap.xml", TestAppFactory: factory,
			ExpectedStatus: 404, ExpectedContent: []string{`"status":404`}, NotExpectedContent: []string{"<html"},
		},
		{
			Name: "accueil indexable", Method: http.MethodGet, URL: "/", TestAppFactory: factory,
			ExpectedStatus: 200, ExpectedContent: []string{`rel="canonical"`}, AfterTestFunc: header("X-Robots-Tag", ""),
		},
		{
			Name: "route SPA publique non indexée", Method: http.MethodGet, URL: "/login", TestAppFactory: factory,
			ExpectedStatus: 200, ExpectedContent: []string{`<div id="root">`}, AfterTestFunc: header("X-Robots-Tag", robotsNoIndex),
		},
		{
			Name: "invitation : carte dédiée et noindex", Method: http.MethodGet, URL: "/j/ABC123", TestAppFactory: factory,
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`<meta property="og:title" content="On commande ensemble ? — OCC Deliveries" />`,
				`<meta property="og:url" content="https://eat.example/j/ABC123" />`,
			},
			NotExpectedContent: []string{"canonical"},
			AfterTestFunc:      header("X-Robots-Tag", robotsNoIndexNoFollow),
		},
		{
			Name: "salon d'équipe : carte dédiée", Method: http.MethodGet, URL: "/e/ABCD1234", TestAppFactory: factory,
			ExpectedStatus: 200, ExpectedContent: []string{"Rejoins le salon de ton équipe"},
		},
		{
			Name: "API : noindex même en 404", Method: http.MethodGet, URL: "/api/inconnu", TestAppFactory: factory,
			ExpectedStatus: 404, ExpectedContent: []string{`"status":404`}, AfterTestFunc: header("X-Robots-Tag", robotsNoIndexNoFollow),
		},
		{
			Name: "API PocketBase : noindex", Method: http.MethodGet, URL: "/api/health", TestAppFactory: factory,
			ExpectedStatus: 200, ExpectedContent: []string{`"code":200`}, AfterTestFunc: header("X-Robots-Tag", robotsNoIndexNoFollow),
		},
	}
	for _, s := range scenarios {
		s.Test(t)
	}
}
