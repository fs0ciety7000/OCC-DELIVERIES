// Package enrich completes the contact details of restaurants (phone,
// postal address, real coordinates) from OpenStreetMap through the public
// Nominatim search API, at the end of each synchronisation run.
//
// Nominatim usage policy (https://operations.osmfoundation.org/policies/nominatim/)
// is strictly applied and cannot be relaxed by configuration:
//   - identified User-Agent with a contact URL;
//   - at most one request per second (MinInterval), one at a time;
//   - every answer cached on disk (hits and misses), 30 days by default;
//   - a small number of lookups per run (DefaultMaxLookups);
//   - no retry: 429 / 403 / 5xx stop the enrichment for the run.
//
// OSM data is © OpenStreetMap contributors, ODbL: every filled restaurant
// records where the data comes from (Attribution) and the UI credits it.
//
// Matching (Pick) and filling (Fill) are pure and table-tested.
package enrich

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Defaults.
const (
	DefaultEndpoint = "https://nominatim.openstreetmap.org/search"
	// UserAgent identifies the application and gives a contact URL (policy).
	UserAgent = "OCC-Deliveries-enrich/1.0 (+https://eat.fs0ciety.org)"
	// MinInterval is the minimal pause between two requests (policy: ≤ 1/s).
	MinInterval     = 1100 * time.Millisecond
	DefaultCacheTTL = 30 * 24 * time.Hour
	// DefaultMaxLookups caps the network lookups of one run (cache hits are free).
	DefaultMaxLookups = 60
	maxBody           = 2 << 20
)

// ErrStopped means Nominatim refused or throttled us: no further request in
// this run (never retried, never worked around).
type ErrStopped struct{ Reason string }

func (e *ErrStopped) Error() string {
	return "OpenStreetMap (Nominatim) : " + e.Reason + " — arrêt de l'enrichissement pour cette exécution"
}

// Client queries Nominatim politely.
type Client struct {
	Endpoint string
	HTTP     *http.Client
	CacheDir string // "" disables the cache
	CacheTTL time.Duration
	// Interval between two requests, clamped to MinInterval.
	Interval time.Duration
	// Now and Sleep replace the clock in tests (the pauses are still
	// computed, never skipped in production).
	Now   func() time.Time
	Sleep func(context.Context, time.Duration) error
	Logf  func(format string, args ...any)

	// Network counts the requests sent, Cached the cache hits.
	Network, Cached int

	last time.Time
}

// NewClient returns a client with the production defaults.
func NewClient(cacheDir string) *Client {
	return &Client{
		Endpoint: DefaultEndpoint,
		HTTP:     &http.Client{Timeout: 20 * time.Second},
		CacheDir: cacheDir,
		CacheTTL: DefaultCacheTTL,
		Interval: MinInterval,
	}
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
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

func (c *Client) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

// SearchURL is the request sent for a free-form query (Belgium only, French
// names, address details, extra tags such as phone, alternative names).
func (c *Client) SearchURL(q string) string {
	v := url.Values{}
	v.Set("format", "jsonv2")
	v.Set("addressdetails", "1")
	v.Set("extratags", "1")
	v.Set("namedetails", "1")
	v.Set("countrycodes", "be")
	v.Set("accept-language", "fr")
	v.Set("limit", "5")
	v.Set("q", q)
	ep := c.Endpoint
	if ep == "" {
		ep = DefaultEndpoint
	}
	return ep + "?" + v.Encode()
}

func (c *Client) cachePath(u string) string {
	if c.CacheDir == "" {
		return ""
	}
	h := sha256.Sum256([]byte(u))
	return filepath.Join(c.CacheDir, "nominatim", hex.EncodeToString(h[:])[:40]+".json")
}

// IsCached reports whether the answer to q is in the (fresh) cache.
func (c *Client) IsCached(q string) bool {
	_, ok := c.fromCache(c.SearchURL(q))
	return ok
}

func (c *Client) fromCache(u string) ([]byte, bool) {
	p := c.cachePath(u)
	if p == "" {
		return nil, false
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, false
	}
	ttl := c.CacheTTL
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	if c.now().Sub(st.ModTime()) >= ttl {
		return nil, false
	}
	b, err := os.ReadFile(p)
	return b, err == nil
}

// Search returns the places Nominatim finds for q (cached answers first).
func (c *Client) Search(ctx context.Context, q string) ([]Place, error) {
	u := c.SearchURL(q)
	if b, ok := c.fromCache(u); ok {
		c.Cached++
		return ParsePlaces(b)
	}
	b, err := c.fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	places, err := ParsePlaces(b)
	if err != nil {
		return nil, err
	}
	if p := c.cachePath(u); p != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			// misses ("[]") are cached too; dated with our clock (TTL)
			if os.WriteFile(p, b, 0o644) == nil {
				_ = os.Chtimes(p, c.now(), c.now())
			}
		}
	}
	return places, nil
}

func (c *Client) fetch(ctx context.Context, u string) ([]byte, error) {
	interval := max(c.Interval, MinInterval)
	if !c.last.IsZero() {
		if err := c.sleep(ctx, c.last.Add(interval).Sub(c.now())); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", "https://eat.fs0ciety.org/")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "fr-BE,fr;q=0.9")
	c.Network++
	c.logf("  GET %s", u)
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	c.last = c.now()
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return nil, &ErrStopped{Reason: "trop de requêtes (HTTP 429)"}
	case res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized:
		return nil, &ErrStopped{Reason: fmt.Sprintf("accès refusé (HTTP %d)", res.StatusCode)}
	case res.StatusCode >= 500:
		return nil, &ErrStopped{Reason: fmt.Sprintf("service indisponible (HTTP %d)", res.StatusCode)}
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return nil, fmt.Errorf("HTTP %d pour %s", res.StatusCode, u)
	}
	return body, nil
}

// IsStopped reports whether err stops the enrichment of the run.
func IsStopped(err error) bool {
	var s *ErrStopped
	return errors.As(err, &s)
}

// ---------------------------------------------------------------- places

// Place is a Nominatim search result.
type Place struct {
	OSMType  string // node, way, relation
	OSMID    int64
	Category string // amenity, shop…
	Type     string // restaurant, fast_food…
	Name     string
	// Names holds every name of the place (name, brand, official_name, alt_name…).
	Names       []string
	Lat, Lng    float64
	Phone       string // raw phone / contact:phone tag
	HouseNumber string
	Road        string
	Postcode    string
	Locality    string
	DisplayName string
}

// URL is the public OpenStreetMap page of the place (attribution link).
func (p Place) URL() string {
	if p.OSMType == "" || p.OSMID == 0 {
		return "https://www.openstreetmap.org/"
	}
	return fmt.Sprintf("https://www.openstreetmap.org/%s/%d", p.OSMType, p.OSMID)
}

type rawPlace struct {
	OSMType     string            `json:"osm_type"`
	OSMID       int64             `json:"osm_id"`
	Lat         string            `json:"lat"`
	Lon         string            `json:"lon"`
	Category    string            `json:"category"`
	Class       string            `json:"class"` // format=json
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Address     map[string]string `json:"address"`
	ExtraTags   map[string]string `json:"extratags"`
	NameDetails map[string]string `json:"namedetails"`
}

// ParsePlaces decodes a jsonv2 search answer.
func ParsePlaces(b []byte) ([]Place, error) {
	var raw []rawPlace
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("réponse Nominatim invalide : %w", err)
	}
	out := make([]Place, 0, len(raw))
	for _, r := range raw {
		p := Place{OSMType: r.OSMType, OSMID: r.OSMID, Category: r.Category, Type: r.Type, Name: strings.TrimSpace(r.Name), DisplayName: r.DisplayName}
		if p.Category == "" {
			p.Category = r.Class
		}
		p.Lat, _ = strconv.ParseFloat(r.Lat, 64)
		p.Lng, _ = strconv.ParseFloat(r.Lon, 64)
		for _, k := range []string{"phone", "contact:phone", "contact:mobile", "mobile"} {
			if v := strings.TrimSpace(r.ExtraTags[k]); v != "" {
				p.Phone = v
				break
			}
		}
		a := r.Address
		p.HouseNumber, p.Road, p.Postcode = strings.TrimSpace(a["house_number"]), strings.TrimSpace(a["road"]), strings.TrimSpace(a["postcode"])
		if p.Road == "" {
			p.Road = strings.TrimSpace(a["pedestrian"])
		}
		for _, k := range []string{"city", "town", "village", "municipality", "suburb"} {
			if v := strings.TrimSpace(a[k]); v != "" {
				p.Locality = v
				break
			}
		}
		seen := map[string]bool{}
		add := func(n string) {
			n = strings.TrimSpace(n)
			if n != "" && !seen[n] {
				seen[n] = true
				p.Names = append(p.Names, n)
			}
		}
		add(p.Name)
		for _, k := range []string{"name", "name:fr", "brand", "official_name", "alt_name", "short_name", "old_name"} {
			for _, n := range strings.Split(r.NameDetails[k], ";") {
				add(n)
			}
		}
		if p.Name == "" && len(p.Names) > 0 {
			p.Name = p.Names[0]
		}
		out = append(out, p)
	}
	return out, nil
}
