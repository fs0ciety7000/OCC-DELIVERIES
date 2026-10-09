package menusync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Probe statuses (DiscoverTry.Status).
const (
	ProbeFound       = "found"       // Takeaway satellite site, menu read
	ProbeAbsent      = "absent"      // the name does not resolve (DNS)
	ProbeUnreachable = "unreachable" // resolves, but no usable answer (timeout, TLS, 5xx…)
	ProbeHTTP        = "http"        // other non-2xx answer (404…)
	ProbeRobots      = "robots"      // robots.txt forbids it: not requested
	ProbeBlocked     = "blocked"     // 403 / anti-bot page: host abandoned, never retried
	ProbeOther       = "other"       // a site, but not the Takeaway template
	ProbeInvalid     = "invalid"     // Takeaway template, but no readable menu
	ProbeSkipped     = "skipped"     // www./bare variant of a site already found
)

// MinProbeGap is the minimum pause between two requests of a discovery.
const MinProbeGap = time.Second

// ProbeTimeout is the per-request timeout of a discovery.
const ProbeTimeout = 6 * time.Second

const maxProbeBody = 8 << 20

// Prober checks discovery candidates politely: identified User-Agent,
// robots.txt first, one request at a time with ≥ MinProbeGap between two,
// short timeout, no retry, a 403 / anti-bot page abandons the host. Hosts
// that do not resolve cost no request at all.
type Prober struct {
	Client *http.Client
	// Lookup resolves a host; an error means "absent" (default: system resolver).
	Lookup func(ctx context.Context, host string) error
	// URLFor maps a candidate host to its base URL (default "https://<host>";
	// tests point it at an httptest server).
	URLFor func(host string) string
	Gap    time.Duration // clamped to MinProbeGap
	// Sleep replaces the real pauses (tests only).
	Sleep func(context.Context, time.Duration) error
	Logf  func(format string, args ...any)
	// Center is the reference point of DiscoverFound.DistanceKm (0,0 = none).
	Center City

	// Network counts the requests actually sent.
	Network int
	last    time.Time
}

// NewProber returns a Prober with the production defaults.
func NewProber() *Prober {
	return &Prober{
		Client: &http.Client{Timeout: ProbeTimeout},
		Gap:    MinProbeGap,
	}
}

// DiscoverTry is one checked host.
type DiscoverTry struct {
	Host    string `json:"host"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// DiscoverFound is a Takeaway satellite site found by a discovery.
type DiscoverFound struct {
	URL         string   `json:"url"`
	Host        string   `json:"host"`
	Name        string   `json:"name"`
	Address     string   `json:"address"`
	Items       int      `json:"items"`
	Categories  int      `json:"categories"`
	TakeawayURL string   `json:"takeawayUrl,omitempty"`
	Lat         float64  `json:"lat,omitempty"`
	Lng         float64  `json:"lng,omitempty"`
	DistanceKm  *float64 `json:"distanceKm,omitempty"`
}

// DiscoverResult is the outcome of a discovery.
type DiscoverResult struct {
	Tried []DiscoverTry
	Found []DiscoverFound
}

func (p *Prober) logf(format string, args ...any) {
	if p.Logf != nil {
		p.Logf(format, args...)
	}
}

func (p *Prober) baseURL(host string) string {
	if p.URLFor != nil {
		return strings.TrimSuffix(p.URLFor(host), "/")
	}
	return "https://" + host
}

func (p *Prober) lookup(ctx context.Context, host string) error {
	if p.Lookup != nil {
		return p.Lookup(ctx, host)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := net.DefaultResolver.LookupHost(ctx, host)
	return err
}

func (p *Prober) wait(ctx context.Context) error {
	if p.last.IsZero() {
		return nil
	}
	d := time.Until(p.last.Add(max(p.Gap, MinProbeGap)))
	if p.Sleep != nil {
		return p.Sleep(ctx, d)
	}
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (p *Prober) client() *http.Client {
	c := p.Client
	if c == nil {
		c = &http.Client{Timeout: ProbeTimeout}
	}
	cp := *c
	if cp.Timeout <= 0 || cp.Timeout > ProbeTimeout {
		cp.Timeout = ProbeTimeout
	}
	// never follow a redirect to takeaway.com (Cloudflare challenge) or its
	// sister platforms: the satellite site is what we are after
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("trop de redirections")
		}
		if IsTakeawayPlatformHost(req.URL.Host) {
			return errPlatformRedirect
		}
		return nil
	}
	return &cp
}

var errPlatformRedirect = errors.New("redirige vers la plateforme Takeaway (jamais suivie)")

type probeAnswer struct {
	status int
	header http.Header
	body   []byte
	final  *url.URL
}

// get performs one polite GET (no retry).
func (p *Prober) get(ctx context.Context, c *http.Client, u string) (probeAnswer, error) {
	if err := p.wait(ctx); err != nil {
		return probeAnswer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return probeAnswer{}, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,text/plain;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "fr-BE,fr;q=0.9")
	p.Network++
	p.logf("  GET %s", u)
	res, err := c.Do(req)
	p.last = time.Now()
	if err != nil {
		return probeAnswer{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxProbeBody))
	if err != nil {
		return probeAnswer{}, err
	}
	return probeAnswer{status: res.StatusCode, header: res.Header, body: body, final: res.Request.URL}, nil
}

// Discover checks the hosts of a plan one by one and returns what it found.
// It stops early only when ctx ends.
func (p *Prober) Discover(ctx context.Context, plan DiscoverPlan) DiscoverResult {
	var out DiscoverResult
	found := map[string]bool{}    // bare host → found
	answered := map[string]bool{} // bare host → a server answered
	c := p.client()
	for _, host := range plan.Hosts {
		if ctx.Err() != nil {
			break
		}
		if found[BareHost(host)] {
			out.Tried = append(out.Tried, DiscoverTry{Host: host, Status: ProbeSkipped, Message: "même site que la variante déjà trouvée"})
			continue
		}
		if answered[BareHost(host)] {
			out.Tried = append(out.Tried, DiscoverTry{Host: host, Status: ProbeSkipped, Message: "même domaine que la variante déjà vérifiée"})
			continue
		}
		page := "/"
		if plan.Kind == DiscoverSite {
			if u, err := url.Parse(plan.SiteURL); err == nil {
				page = u.RequestURI()
			}
		}
		try, f := p.probe(ctx, c, host, page)
		out.Tried = append(out.Tried, try)
		switch try.Status {
		case ProbeOther, ProbeHTTP, ProbeRobots, ProbeBlocked, ProbeInvalid:
			answered[BareHost(host)] = true
		}
		if f != nil {
			found[BareHost(host)] = true
			found[BareHost(f.Host)] = true
			out.Found = append(out.Found, *f)
		}
	}
	return out
}

func (p *Prober) probe(ctx context.Context, c *http.Client, host, page string) (DiscoverTry, *DiscoverFound) {
	try := DiscoverTry{Host: host}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	if err := p.lookup(ctx, hostname); err != nil {
		try.Status, try.Message = ProbeAbsent, "nom de domaine inexistant"
		return try, nil
	}
	base := p.baseURL(host)

	// robots.txt first (RFC 9309: 4xx = everything allowed)
	ans, err := p.get(ctx, c, base+"/robots.txt")
	var rb *Robots
	switch {
	case err != nil:
		try.Status, try.Message = ProbeUnreachable, shortNetErr(err)
		return try, nil
	case challengeReason(ans.status, ans.header, ans.body) != "":
		try.Status, try.Message = ProbeBlocked, "accès refusé ("+challengeReason(ans.status, ans.header, ans.body)+") : site abandonné, aucun contournement"
		return try, nil
	case ans.status >= 200 && ans.status < 300:
		rb = ParseRobots(ans.body, robotsToken)
	case ans.status >= 400 && ans.status < 500:
		rb = &Robots{}
	default:
		try.Status, try.Message = ProbeUnreachable, fmt.Sprintf("robots.txt illisible (HTTP %d)", ans.status)
		return try, nil
	}
	if !rb.Allowed(page) {
		try.Status, try.Message = ProbeRobots, "interdit par robots.txt : page non demandée"
		return try, nil
	}

	ans, err = p.get(ctx, c, base+page)
	switch {
	case err != nil:
		try.Status, try.Message = ProbeUnreachable, shortNetErr(err)
		return try, nil
	case challengeReason(ans.status, ans.header, ans.body) != "":
		try.Status, try.Message = ProbeBlocked, "accès refusé ("+challengeReason(ans.status, ans.header, ans.body)+") : site abandonné, aucun contournement"
		return try, nil
	case ans.status < 200 || ans.status >= 300:
		try.Status, try.Message = ProbeHTTP, fmt.Sprintf("HTTP %d", ans.status)
		return try, nil
	}
	if !LooksLikeTakeawaySite(ans.body) {
		try.Status, try.Message = ProbeOther, "site sans le modèle Takeaway"
		return try, nil
	}
	pageURL := base + page
	if p.URLFor == nil && ans.final != nil {
		pageURL = ans.final.String() // after redirects (bare → www, http → https)
	}
	r, err := ParseTakeawaySite(ans.body, pageURL)
	if err != nil {
		try.Status, try.Message = ProbeInvalid, err.Error()
		return try, nil
	}
	f := &DiscoverFound{
		URL: pageURL, Host: host, Name: r.Name, Address: domain.NormalizeAddress(r.Address),
		Items: r.ItemCount(), Categories: len(r.Categories), Lat: r.Lat, Lng: r.Lng,
	}
	if pu, err := url.Parse(pageURL); err == nil && p.URLFor == nil {
		f.Host = strings.ToLower(pu.Host)
	}
	for _, l := range r.Providers {
		f.TakeawayURL = l.URL
	}
	if (r.Lat != 0 || r.Lng != 0) && (p.Center.Lat != 0 || p.Center.Lng != 0) {
		d := float64(int(domain.HaversineKm(p.Center.Lat, p.Center.Lng, r.Lat, r.Lng)*10+0.5)) / 10
		f.DistanceKm = &d
	}
	try.Status, try.Message = ProbeFound, fmt.Sprintf("%s : %d plats", r.Name, f.Items)
	return try, f
}

// shortNetErr turns a client error into a short French message.
func shortNetErr(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, errPlatformRedirect):
		return errPlatformRedirect.Error()
	case errors.As(err, &ne) && ne.Timeout():
		return "délai dépassé"
	case errors.Is(err, context.Canceled):
		return "interrompu"
	}
	msg := err.Error()
	if len(msg) > 160 {
		msg = msg[:160] + "…"
	}
	return "injoignable (" + msg + ")"
}
