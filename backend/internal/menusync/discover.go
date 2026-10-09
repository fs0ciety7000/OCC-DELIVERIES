package menusync

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Discovery finds the official Takeaway.com satellite site of a restaurant
// (www.<slug>.be, shared template) from a Takeaway / Just Eat link, a free
// restaurant name or a direct site URL. Takeaway.com itself is never
// requested (Cloudflare challenge): only the candidate hosts are.

// Discovery query kinds.
const (
	DiscoverTakeaway = "takeaway" // a takeaway.com / just-eat link: slug read from /menu/<slug>
	DiscoverName     = "name"     // a free restaurant name
	DiscoverSite     = "site"     // a direct site URL: only that page is checked
)

// MaxDiscoverDomains caps the candidate domains of a query (each domain is
// tried as www.<domain> then <domain>).
const MaxDiscoverDomains = 16

// DiscoverPlan is what a discovery will check, in order.
type DiscoverPlan struct {
	Kind string
	// Slug is the takeaway slug or the slugified name ("" for a site).
	Slug string
	// SiteURL is the page to check for a direct site URL.
	SiteURL string
	// Hosts are the candidate hosts, in order: www.<domain> then <domain>
	// for each candidate domain (a single host for a direct site URL).
	Hosts []string
}

// platformHostRe matches the delivery platforms whose store pages carry the
// Takeaway slug (never requested).
var platformHostRe = regexp.MustCompile(`(^|\.)(takeaway\.com|just-eat\.[a-z.]+|justeat\.[a-z.]+|thuisbezorgd\.nl|lieferando\.[a-z]+|pyszne\.pl)$`)

var menuSlugRe = regexp.MustCompile(`/menu/([a-z0-9][a-z0-9-]*)`)

// IsTakeawayPlatformHost reports whether host is takeaway.com or a sister
// platform (never requested by the discovery).
func IsTakeawayPlatformHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	return platformHostRe.MatchString(host)
}

// TakeawaySlug extracts the store slug of a takeaway.com / just-eat link
// (https://www.takeaway.com/be-fr/menu/snack-a-la-gare#pre-order →
// "snack-a-la-gare"). ok is false for any other URL.
func TakeawaySlug(raw string) (slug string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || !IsTakeawayPlatformHost(u.Host) {
		return "", false
	}
	m := menuSlugRe.FindStringSubmatch(strings.ToLower(u.Path))
	if m == nil {
		return "", false
	}
	return strings.Trim(m[1], "-"), m[1] != ""
}

// PlanDiscovery reads a query and lists what to check. city (e.g. "mons")
// adds the "<slug>-mons" style forms.
func PlanDiscovery(query, city string) (DiscoverPlan, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return DiscoverPlan{}, errors.New("indique un lien takeaway.com, un nom de restaurant ou l'adresse du site")
	}
	if looksLikeURL(q) {
		raw := q
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return DiscoverPlan{}, errors.New("adresse invalide : un lien en https:// est attendu")
		}
		if IsTakeawayPlatformHost(u.Host) {
			slug, ok := TakeawaySlug(raw)
			if !ok {
				return DiscoverPlan{}, errors.New("lien Takeaway sans nom de restaurant : colle le lien de la page du restaurant (…/menu/nom-du-resto)")
			}
			return DiscoverPlan{Kind: DiscoverTakeaway, Slug: slug, Hosts: DiscoverHosts(slug, city)}, nil
		}
		u.Fragment = ""
		if u.Path == "" {
			u.Path = "/"
		}
		host := strings.ToLower(u.Hostname())
		if !validHostRe.MatchString(host) {
			return DiscoverPlan{}, errors.New("adresse invalide : nom de domaine attendu")
		}
		return DiscoverPlan{Kind: DiscoverSite, SiteURL: u.String(), Hosts: []string{strings.ToLower(u.Host)}}, nil
	}
	slug := discoverSlug(q)
	if slug == "" {
		return DiscoverPlan{}, errors.New("nom de restaurant illisible")
	}
	return DiscoverPlan{Kind: DiscoverName, Slug: slug, Hosts: DiscoverHosts(slug, city)}, nil
}

var validHostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$|^127\.0\.0\.1$|^localhost$`)

func looksLikeURL(q string) bool {
	if strings.Contains(q, "://") {
		return true
	}
	if strings.ContainsAny(q, " \t") {
		return false
	}
	// "www.tomomons.be", "tomomons.be/", "takeaway.com/be/menu/x"
	host, _, _ := strings.Cut(q, "/")
	return strings.Contains(host, ".") && validHostRe.MatchString(strings.ToLower(host))
}

// discoverSlug slugifies a free name; apostrophes separate words
// ("L'Atelier" → "l-atelier").
func discoverSlug(name string) string {
	return Slugify(strings.NewReplacer("’", " ", "'", " ", "`", " ").Replace(name))
}

// discoverStop are the words dropped by the last-resort candidates.
var discoverStop = map[string]bool{
	"le": true, "la": true, "les": true, "l": true, "de": true, "du": true, "des": true, "d": true,
	"a": true, "au": true, "aux": true, "et": true, "chez": true, "the": true,
	"snack": true, "restaurant": true, "resto": true, "pizzeria": true, "friterie": true, "brasserie": true,
}

// DiscoverHosts lists the candidate hosts of a slug, most likely first:
// the slug as is, without hyphens, then with the city suffix ("-mons",
// "mons"), in .be then .com, then the same forms with the articles and
// generic words ("snack", "restaurant", "pizzeria"…) removed. Each domain
// appears as www.<domain> then <domain>; at most MaxDiscoverDomains domains.
func DiscoverHosts(slug, city string) []string {
	slug = Slugify(slug)
	city = Slugify(city)
	if city == "" {
		city = DefaultCity
	}
	var domains []string
	add := func(label, tld string) {
		label = strings.Trim(label, "-")
		if len(label) < 2 || len(label) > 63 {
			return
		}
		if d := label + "." + tld; !slices.Contains(domains, d) {
			domains = append(domains, d)
		}
	}
	forms := func(s string) []string {
		if s == "" {
			return nil
		}
		compact := strings.ReplaceAll(s, "-", "")
		out := []string{s, compact}
		base := strings.TrimSuffix(s, "-"+city)
		if base != s && base != "" {
			// the takeaway slug already carries the city ("tomo-mons"):
			// also try without it, then the doubled form some sites use
			// ("lafritemayomons-mons")
			out = append(out, base, strings.ReplaceAll(base, "-", ""), compact+"-"+city)
		} else {
			out = append(out, s+"-"+city, compact+city, compact+"-"+city)
		}
		return out
	}
	primary := forms(slug)
	for _, f := range primary {
		add(f, "be")
	}
	for _, f := range primary[:min(2, len(primary))] {
		add(f, "com")
	}
	var kept []string
	for _, w := range strings.Split(slug, "-") {
		if !discoverStop[w] && w != city {
			kept = append(kept, w)
		}
	}
	if stripped := strings.Join(kept, "-"); stripped != "" && stripped != slug && stripped != strings.TrimSuffix(slug, "-"+city) {
		for _, f := range forms(stripped)[:4] {
			add(f, "be")
		}
	}
	if len(domains) > MaxDiscoverDomains {
		domains = domains[:MaxDiscoverDomains]
	}
	hosts := make([]string, 0, 2*len(domains))
	for _, d := range domains {
		hosts = append(hosts, "www."+d, d)
	}
	return hosts
}

// BareHost strips "www." and the port: the site identity of a host.
func BareHost(host string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	return strings.TrimPrefix(host, "www.")
}

// takeawayTemplateMarkers are the class names of the shared satellite
// template (menu categories and meal blocks).
var takeawayTemplateMarkers = [][]byte{
	[]byte("menucard__meals-group"),
	[]byte("meal-wrapper"),
	[]byte("menucard-listing"),
}

var (
	itempropNameRe  = regexp.MustCompile(`itemprop\s*=\s*["']?name\b`)
	itempropPriceRe = regexp.MustCompile(`itemprop\s*=\s*["']?price\b`)
	menucatRe       = regexp.MustCompile(`class\s*=\s*["']?[^"'>]*\bmenucat\b`)
)

// LooksLikeTakeawaySite reports whether a page is built on the Takeaway.com
// satellite template: its menu classes, schema.org Product microdata and a
// Takeaway reference (footer link, orderUrl meta or stylesheet).
func LooksLikeTakeawaySite(page []byte) bool {
	lower := bytes.ToLower(page)
	markers := 0
	for _, m := range takeawayTemplateMarkers {
		if bytes.Contains(lower, m) {
			markers++
		}
	}
	if !menucatRe.Match(lower) {
		return false
	}
	ref := bytes.Contains(lower, []byte("takeaway.com")) || bytes.Contains(lower, []byte("just-eat")) || bytes.Contains(lower, []byte("takeaway.css"))
	micro := itempropNameRe.Match(lower) && itempropPriceRe.Match(lower)
	return markers >= 1 && ref && micro
}
