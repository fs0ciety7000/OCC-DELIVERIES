package main

import (
	"html"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// robotsTxt is served by the binary itself: without it /robots.txt would fall
// back to the SPA index.html (200 text/html). Invitation links (/j/, /e/) are
// deliberately NOT disallowed: link-preview bots that honour robots.txt
// (Twitterbot, LinkedInBot, Slackbot…) would otherwise show no card. They are
// kept out of the index by the X-Robots-Tag header instead (robotsTag).
// No sitemap: the home page is the only indexable URL (see CLAUDE.md log).
const robotsTxt = `# OCC Deliveries — https://eat.fs0ciety.org
# Application privée : seule la page d'accueil est destinée aux moteurs de recherche.
# Les pages non listées ici (connexion, restaurants, invitations /j/ et /e/…) restent
# explorables pour les aperçus de liens mais portent l'en-tête « X-Robots-Tag: noindex ».
User-agent: *
Disallow: /api/
Disallow: /_/
Disallow: /admin
Disallow: /party/
Disallow: /equipes/
Disallow: /profile
Disallow: /auth/
`

// Robots directives sent as X-Robots-Tag.
const (
	robotsNoIndex         = "noindex"
	robotsNoIndexNoFollow = "noindex, nofollow"
)

// privatePrefixes are first path segments that must never be indexed nor
// followed: API, PocketBase admin, signed-in screens, e-mail tokens, invitations.
var privatePrefixes = map[string]bool{
	"api": true, "_": true, "admin": true, "party": true, "equipes": true,
	"profile": true, "auth": true, "j": true, "e": true,
}

// robotsTag returns the X-Robots-Tag value for a request path ("" = indexable).
// Default deny: only the home page and static files (with an extension) are
// left alone; every other SPA route (login, restaurants, unknown → soft 404)
// gets noindex.
func robotsTag(p string) string {
	p = strings.Trim(p, "/")
	if p == "" || p == "index.html" {
		return ""
	}
	first, _, _ := strings.Cut(p, "/")
	if privatePrefixes[first] {
		return robotsNoIndexNoFollow
	}
	if isStaticFile(p) {
		return ""
	}
	return robotsNoIndex
}

// isStaticFile reports whether the last path segment looks like a file name
// (short extension), e.g. assets/index-abc.js, og/og-image.jpg, sw.js.
func isStaticFile(p string) bool {
	ext := path.Ext(p)
	return len(ext) >= 2 && len(ext) <= 12 && !strings.Contains(ext, "/")
}

// missingFileNotFound lists well-known files we do not ship: answering them
// with the SPA index (200 text/html) would be a soft 404 for crawlers.
var missingFileNotFound = map[string]bool{
	"sitemap.xml": true, "sitemap_index.xml": true, "favicon.ico": true,
	"ads.txt": true, "app-ads.txt": true, "humans.txt": true, "security.txt": true,
}

// Invitation links are shared in messaging apps: generic, inviting card texts
// (never the party or team name, which would leak to anyone holding the link
// preview and to the preview services).
type inviteMeta struct{ title, description string }

var inviteMetas = map[string]inviteMeta{
	"j": {
		title:       "On commande ensemble ? — OCC Deliveries",
		description: "Un·e collègue t'invite à une commande groupée : vote pour le resto, compose ton panier, rembourse en un scan. Même sans compte, en invité·e.",
	},
	"e": {
		title:       "Rejoins le salon de ton équipe — OCC Deliveries",
		description: "Ton équipe commande à manger ensemble sur OCC Deliveries : rejoins le salon pour suivre la commande du jour en un geste.",
	},
}

var (
	reTitle     = regexp.MustCompile(`<title>[^<]*</title>`)
	reMetaTitle = regexp.MustCompile(`(<meta (?:property|name)="(?:og|twitter):title" content=")[^"]*(")`)
	reMetaDesc  = regexp.MustCompile(`(<meta (?:property="og:description"|name="twitter:description"|name="description") content=")[^"]*(")`)
	reMetaURL   = regexp.MustCompile(`(<meta property="og:url" content=")[^"]*(")`)
	reCanonical = regexp.MustCompile(`\s*<link rel="canonical"[^>]*>`)
)

// inviteHTML adapts index.html for an invitation link: card texts, og:url =
// the shared URL (Facebook re-scrapes og:url) and no canonical (noindex page).
func inviteHTML(index []byte, m inviteMeta, pageURL string) []byte {
	// escape for HTML attributes, then "$" for regexp expansion templates
	esc := func(s string) string { return strings.ReplaceAll(html.EscapeString(s), "$", "$$") }
	t, d, u := esc(m.title), esc(m.description), esc(pageURL)
	out := reTitle.ReplaceAll(index, []byte("<title>"+t+"</title>"))
	out = reMetaTitle.ReplaceAll(out, []byte("${1}"+t+"${2}"))
	out = reMetaDesc.ReplaceAll(out, []byte("${1}"+d+"${2}"))
	if pageURL != "" {
		out = reMetaURL.ReplaceAll(out, []byte("${1}"+u+"${2}"))
	}
	return reCanonical.ReplaceAll(out, nil)
}

// bindSEO registers /robots.txt and the global X-Robots-Tag middleware
// (it also covers /api/* and PocketBase's /_/).
func bindSEO(se *core.ServeEvent) {
	se.Router.BindFunc(func(e *core.RequestEvent) error {
		if tag := robotsTag(e.Request.URL.Path); tag != "" {
			e.Response.Header().Set("X-Robots-Tag", tag)
		}
		return e.Next()
	})
	if !se.Router.HasRoute(http.MethodGet, "/robots.txt") {
		se.Router.GET("/robots.txt", func(e *core.RequestEvent) error {
			e.Response.Header().Set("Cache-Control", "public, max-age=3600")
			return e.Blob(http.StatusOK, "text/plain; charset=utf-8", []byte(robotsTxt))
		})
	}
}
