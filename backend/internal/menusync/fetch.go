package menusync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// UserAgent identifies the tool (mandatory, never spoofed).
const UserAgent = "OCC-Deliveries-menusync/1.0 (+https://eat.fs0ciety.org)"

// robotsToken is the product token matched against robots.txt groups.
const robotsToken = "OCC-Deliveries-menusync"

// MinDelay is the minimum pause between two network requests.
const MinDelay = 2500 * time.Millisecond

const maxBody = 32 << 20

// ErrBlocked means the site answered 403 or an anti-bot challenge page. The
// run must stop: we never try to get around it.
type ErrBlocked struct {
	URL    string
	Reason string
}

func (e *ErrBlocked) Error() string {
	return fmt.Sprintf("accès refusé par %s (%s) : arrêt, aucun contournement n'est tenté", e.URL, e.Reason)
}

// ErrDisallowed means robots.txt forbids the URL; it was not requested.
type ErrDisallowed struct{ URL string }

func (e *ErrDisallowed) Error() string {
	return fmt.Sprintf("URL interdite par robots.txt, non demandée : %s", e.URL)
}

// HTTPError is any other non-2xx answer.
type HTTPError struct {
	URL    string
	Status int
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d pour %s", e.Status, e.URL) }

// IsBlocked reports whether err (or a wrapped error) is an ErrBlocked.
func IsBlocked(err error) bool {
	var b *ErrBlocked
	return errors.As(err, &b)
}

// Request is one HTTP request.
type Request struct {
	Method string // GET by default
	URL    string
	Body   []byte // JSON body for POST
	Accept string
	// Sanitize, if set, rewrites the body before it is cached or returned
	// (used to drop personal data some APIs leak).
	Sanitize func([]byte) ([]byte, error)
}

// Fetcher performs polite, sequential, cached HTTP requests.
type Fetcher struct {
	Client   *http.Client
	Delay    time.Duration // clamped to MinDelay
	Jitter   time.Duration
	CacheDir string // "" disables the cache
	// CacheTTL makes cached answers older than this be fetched again
	// (0 = cached answers never expire).
	CacheTTL time.Duration
	// MaxRetryWait caps Retry-After / backoff waits.
	MaxRetryWait time.Duration
	Logf         func(format string, args ...any)

	// Network counts the requests actually sent; Cached the cache hits.
	Network, Cached int

	// Sleep replaces the real pauses (tests only: the delays are still
	// computed, never skipped in production).
	Sleep func(context.Context, time.Duration) error

	last   time.Time
	robots map[string]*Robots
}

// NewFetcher returns a Fetcher with sane defaults.
func NewFetcher(cacheDir string) *Fetcher {
	return &Fetcher{
		Client:       &http.Client{Timeout: 45 * time.Second},
		Delay:        MinDelay,
		Jitter:       1500 * time.Millisecond,
		CacheDir:     cacheDir,
		MaxRetryWait: 2 * time.Minute,
	}
}

func (f *Fetcher) logf(format string, args ...any) {
	if f.Logf != nil {
		f.Logf(format, args...)
	}
}

// Get fetches a URL with GET.
func (f *Fetcher) Get(ctx context.Context, u string) ([]byte, error) {
	return f.Do(ctx, Request{URL: u})
}

// Do performs a request: robots.txt check, cache lookup, polite wait, at most
// one retry on 429/5xx (Retry-After honoured), hard stop on 403/challenge.
func (f *Fetcher) Do(ctx context.Context, req Request) ([]byte, error) {
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	pu, err := url.Parse(req.URL)
	if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" {
		return nil, fmt.Errorf("URL invalide : %q", req.URL)
	}
	rb, err := f.robotsFor(ctx, pu)
	if err != nil {
		return nil, err
	}
	if !rb.Allowed(pu.RequestURI()) {
		return nil, &ErrDisallowed{URL: req.URL}
	}
	return f.cachedDo(ctx, req, pu)
}

func (f *Fetcher) robotsFor(ctx context.Context, pu *url.URL) (*Robots, error) {
	key := pu.Scheme + "://" + pu.Host
	if r, ok := f.robots[key]; ok {
		return r, nil
	}
	body, err := f.cachedDo(ctx, Request{Method: http.MethodGet, URL: key + "/robots.txt", Accept: "text/plain"}, &url.URL{Scheme: pu.Scheme, Host: pu.Host, Path: "/robots.txt"})
	var r *Robots
	var he *HTTPError
	switch {
	case err == nil:
		r = ParseRobots(body, robotsToken)
	case errors.As(err, &he) && he.Status >= 400 && he.Status < 500:
		r = &Robots{} // no robots.txt: everything allowed (RFC 9309 §2.3.1.3)
	default:
		return nil, fmt.Errorf("robots.txt de %s illisible : %w", pu.Host, err)
	}
	if f.robots == nil {
		f.robots = map[string]*Robots{}
	}
	f.robots[key] = r
	return r, nil
}

func (f *Fetcher) cachePath(req Request, pu *url.URL) string {
	if f.CacheDir == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(req.Method + " " + req.URL + "\n"))
	h.Write(req.Body)
	return filepath.Join(f.CacheDir, pu.Host, hex.EncodeToString(h.Sum(nil))[:40])
}

func (f *Fetcher) cachedDo(ctx context.Context, req Request, pu *url.URL) ([]byte, error) {
	cp := f.cachePath(req, pu)
	if cp != "" {
		if st, err := os.Stat(cp); err == nil && (f.CacheTTL <= 0 || time.Since(st.ModTime()) < f.CacheTTL) {
			if b, err := os.ReadFile(cp); err == nil {
				f.Cached++
				return b, nil
			}
		}
	}
	b, err := f.network(ctx, req)
	if err != nil {
		return nil, err
	}
	if req.Sanitize != nil {
		if b, err = req.Sanitize(b); err != nil {
			return nil, fmt.Errorf("%s : %w", req.URL, err)
		}
	}
	if cp != "" {
		if err := os.MkdirAll(filepath.Dir(cp), 0o755); err == nil {
			_ = os.WriteFile(cp, b, 0o644)
		}
	}
	return b, nil
}

func (f *Fetcher) wait(ctx context.Context, d time.Duration) error {
	if f.Sleep != nil {
		return f.Sleep(ctx, d)
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

func (f *Fetcher) politeWait(ctx context.Context) error {
	delay := max(f.Delay, MinDelay)
	if f.Jitter > 0 {
		delay += rand.N(f.Jitter)
	}
	if f.last.IsZero() {
		return nil
	}
	return f.wait(ctx, time.Until(f.last.Add(delay)))
}

func (f *Fetcher) network(ctx context.Context, req Request) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		if err := f.politeWait(ctx); err != nil {
			return nil, err
		}
		status, header, body, err := f.send(ctx, req)
		f.last = time.Now()
		if err != nil {
			if attempt == 0 {
				f.logf("  erreur réseau (%v), nouvel essai dans %s", err, f.backoff())
				if err := f.wait(ctx, f.backoff()); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		if reason := challengeReason(status, header, body); reason != "" {
			return nil, &ErrBlocked{URL: req.URL, Reason: reason}
		}
		switch {
		case status >= 200 && status < 300:
			return body, nil
		case (status == http.StatusTooManyRequests || status >= 500) && attempt == 0:
			d := retryAfter(header.Get("Retry-After"), time.Now())
			if d <= 0 {
				d = f.backoff()
			}
			if f.MaxRetryWait > 0 && d > f.MaxRetryWait {
				return nil, fmt.Errorf("HTTP %d pour %s : Retry-After de %s, trop long, arrêt", status, req.URL, d)
			}
			f.logf("  HTTP %d, nouvel essai dans %s", status, d.Round(time.Second))
			if err := f.wait(ctx, d); err != nil {
				return nil, err
			}
			continue
		default:
			return nil, &HTTPError{URL: req.URL, Status: status}
		}
	}
}

func (f *Fetcher) backoff() time.Duration {
	return 4 * max(f.Delay, MinDelay)
}

func (f *Fetcher) send(ctx context.Context, req Request) (int, http.Header, []byte, error) {
	var rd io.Reader
	if req.Body != nil {
		rd = bytes.NewReader(req.Body)
	}
	hr, err := http.NewRequestWithContext(ctx, req.Method, req.URL, rd)
	if err != nil {
		return 0, nil, nil, err
	}
	hr.Header.Set("User-Agent", UserAgent)
	accept := req.Accept
	if accept == "" {
		accept = "text/html,application/json;q=0.9,*/*;q=0.8"
	}
	hr.Header.Set("Accept", accept)
	hr.Header.Set("Accept-Language", "fr-BE,fr;q=0.9")
	if req.Body != nil {
		hr.Header.Set("Content-Type", "application/json")
	}
	f.Network++
	f.logf("  %s %s", req.Method, req.URL)
	res, err := f.Client.Do(hr)
	if err != nil {
		return 0, nil, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return 0, nil, nil, err
	}
	return res.StatusCode, res.Header, body, nil
}

// retryAfter parses a Retry-After header (seconds or HTTP date).
func retryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return t.Sub(now)
	}
	return 0
}

// challengeMarkers are signs of an anti-bot interstitial (Cloudflare,
// PerimeterX/HUMAN, DataDome). Seeing one stops the run.
var challengeMarkers = []string{
	"<title>just a moment...</title>",
	"cf-chl",
	"/cdn-cgi/challenge-platform",
	"cf-browser-verification",
	"px-captcha",
	"access to this page has been denied",
	"captcha-delivery.com",
}

// challengeReason returns why a response looks blocked, or "".
func challengeReason(status int, header http.Header, body []byte) string {
	if status == http.StatusForbidden {
		return "HTTP 403"
	}
	if header.Get("Cf-Mitigated") == "challenge" {
		return "défi Cloudflare (cf-mitigated)"
	}
	head := body
	if len(head) > 64<<10 {
		head = head[:64<<10]
	}
	lower := bytes.ToLower(head)
	for _, m := range challengeMarkers {
		if bytes.Contains(lower, []byte(m)) {
			return "page anti-robot détectée (" + m + ")"
		}
	}
	return ""
}
