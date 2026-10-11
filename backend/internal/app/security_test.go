package app

import (
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// headerEnv serves fake SPA (/{path...}) and admin (/_/{path...}) routes,
// like main.go / PocketBase do in production.
func headerEnv(t *testing.T, cfg Config) *env {
	t.Helper()
	return newEnvWith(t, cfg, func(ta *tests.TestApp) {
		ta.OnServe().BindFunc(func(se *core.ServeEvent) error {
			se.Router.GET("/_/{path...}", func(e *core.RequestEvent) error { return e.HTML(200, "admin") })
			se.Router.GET("/{path...}", func(e *core.RequestEvent) error { return e.HTML(200, "spa") })
			se.Router.GET("/api/occ/test-realip", func(e *core.RequestEvent) error { return e.String(200, e.RealIP()) })
			return se.Next()
		})
	})
}

func (e *env) get(url, host string, hdr map[string]string) resp {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	if host != "" {
		req.Host = host
	}
	req.RemoteAddr = "203.0.113.7:4242"
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return resp{status: rec.Code, body: rec.Body.Bytes(), header: rec.Header()}
}

func securityTestConfig(t *testing.T, publicURL string, env map[string]string) Config {
	t.Helper()
	dir := t.TempDir()
	index := `<!doctype html><html><head><script>console.log("inline")</script><script src="/theme-init.js"></script></head></html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig
	cfg.PublicURL = publicURL
	cfg.Security = securityConfigFromEnv(func(k string) string {
		if k == "OCC_PUBLIC_DIR" {
			return dir
		}
		return env[k]
	}, publicURL)
	return cfg
}

func TestSecurityHeadersSPA(t *testing.T) {
	e := headerEnv(t, securityTestConfig(t, "https://eat.example.org", nil))
	r := e.get("/party/abc", "eat.example.org", nil)
	csp := r.header.Get("Content-Security-Policy")
	inline := domain.InlineScriptHashes([]byte(`<script>console.log("inline")</script>`))[0]
	for _, want := range []string{
		"default-src 'self'", "script-src 'self' '" + inline + "';", "frame-ancestors 'none'", "object-src 'none'",
		"connect-src 'self' https://nominatim.openstreetmap.org;", "upgrade-insecure-requests", "report-uri /api/occ/csp-report",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("SPA CSP lacks %q: %s", want, csp)
		}
	}
	want := map[string]string{
		"Strict-Transport-Security":    "max-age=31536000; includeSubDomains",
		"Referrer-Policy":              "strict-origin-when-cross-origin",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Cross-Origin-Resource-Policy": "same-site",
		"Cross-Origin-Opener-Policy":   "same-origin", // PocketBase's, kept (OAuth popup + realtime)
	}
	for k, v := range want {
		if got := r.header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if pp := r.header.Get("Permissions-Policy"); !strings.Contains(pp, "geolocation=(self)") || !strings.Contains(pp, "camera=()") {
		t.Errorf("Permissions-Policy = %q", pp)
	}
	if r.header.Get("Content-Security-Policy-Report-Only") != "" {
		t.Error("report-only header set")
	}

	// service worker: also fetches remote covers
	if sw := e.get("/sw.js", "eat.example.org", nil).header.Get("Content-Security-Policy"); !strings.Contains(sw, "openstreetmap.org https:;") {
		t.Errorf("sw CSP: %s", sw)
	}
	// API: nothing to load
	api := e.get("/api/occ/health", "eat.example.org", nil)
	if got := api.header.Get("Content-Security-Policy"); got != domain.APICSP {
		t.Errorf("API CSP = %q", got)
	}
	// PocketBase admin UI: no strict CSP, anti-framing only
	adm := e.get("/_/", "eat.example.org", nil)
	if got := adm.header.Get("Content-Security-Policy"); got != "frame-ancestors 'self'" {
		t.Errorf("admin CSP = %q", got)
	}
	if adm.header.Get("X-Frame-Options") != "SAMEORIGIN" || adm.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("admin headers: %v", adm.header)
	}
	// files keep PocketBase's sandbox CSP (set only when missing)
	if got := e.get("/api/files/x/y/z.png", "eat.example.org", nil).header.Get("Content-Security-Policy"); got != "" {
		t.Errorf("files CSP preset: %q", got)
	}

	// localhost request on the same deployment: no HSTS / upgrade
	loc := e.get("/", "localhost:8090", nil)
	if loc.header.Get("Strict-Transport-Security") != "" || strings.Contains(loc.header.Get("Content-Security-Policy"), "upgrade-insecure-requests") {
		t.Errorf("localhost: %v", loc.header)
	}
}

func TestSecurityHeadersHTTPAndReportOnly(t *testing.T) {
	e := headerEnv(t, securityTestConfig(t, "http://localhost:8090", nil))
	r := e.get("/", "localhost:8090", nil)
	if r.header.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS over http")
	}
	if csp := r.header.Get("Content-Security-Policy"); csp == "" || strings.Contains(csp, "upgrade-insecure-requests") {
		t.Errorf("http CSP: %q", csp)
	}

	ro := headerEnv(t, securityTestConfig(t, "https://eat.example.org", map[string]string{"OCC_CSP_REPORT_ONLY": "1"}))
	r = ro.get("/", "eat.example.org", nil)
	if r.header.Get("Content-Security-Policy") != "" {
		t.Errorf("enforced CSP in report-only mode: %q", r.header.Get("Content-Security-Policy"))
	}
	if csp := r.header.Get("Content-Security-Policy-Report-Only"); !strings.Contains(csp, "default-src 'self'") || strings.Contains(csp, "upgrade-insecure-requests") {
		t.Errorf("report-only CSP: %q", csp)
	}
	if r.header.Get("Strict-Transport-Security") == "" {
		t.Error("HSTS missing")
	}
}

func TestPocketBaseRealIPNotSpoofable(t *testing.T) {
	e := headerEnv(t, securityTestConfig(t, "http://localhost:8090", nil))
	r := e.get("/api/occ/test-realip", "", map[string]string{clientIPHeader: "1.2.3.4", "X-Forwarded-For": "5.6.7.8", "CF-Connecting-IP": "9.9.9.9"})
	if string(r.body) != "203.0.113.7" {
		t.Fatalf("RealIP = %q", r.body)
	}
}

func TestCSPReportEndpoint(t *testing.T) {
	e := newEnv(t)
	legacy := `{"csp-report":{"document-uri":"https://eat.example.org/","violated-directive":"script-src-elem","blocked-uri":"inline","line-number":3}}`
	if r := e.doRaw("POST", "/api/occ/csp-report", "", "application/csp-report", strings.NewReader(legacy)); r.status != 204 {
		t.Fatalf("legacy: %d %s", r.status, r.body)
	}
	modern := `[{"type":"csp-violation","body":{"documentURL":"https://eat.example.org/","effectiveDirective":"img-src","blockedURL":"http://x/y.png"}}]`
	if r := e.doRaw("POST", "/api/occ/csp-report", "", "application/reports+json", strings.NewReader(modern)); r.status != 204 {
		t.Fatalf("reports+json: %d", r.status)
	}
	got := parseCSPReports([]byte(legacy))
	if len(got) != 1 || got[0]["directive"] != "script-src-elem" || got[0]["line"] != "3" {
		t.Fatalf("legacy parse: %v", got)
	}
	if got := parseCSPReports([]byte(modern)); len(got) != 1 || got[0]["blocked"] != "http://x/y.png" {
		t.Fatalf("modern parse: %v", got)
	}
	if got := parseCSPReports([]byte("nope")); len(got) != 0 {
		t.Fatalf("garbage: %v", got)
	}
	// rate limited, silently
	for i := 0; i < 25; i++ {
		if r := e.doRaw("POST", "/api/occ/csp-report", "", "application/csp-report", strings.NewReader(legacy)); r.status != 204 {
			t.Fatalf("report %d: %d", i, r.status)
		}
	}
}

// The SQL statement of rateLimiter.allow mirrors domain.Limit.Step.
func TestRateLimiterSQLMatchesDomain(t *testing.T) {
	e := newEnv(t)
	rng := rand.New(rand.NewPCG(1, 2))
	for _, l := range []rateLimiter{newRateLimiter("t1", 3, time.Minute), newRateLimiter("t2", 1, 10*time.Minute), newRateLimiter("t3", 20, 15*time.Minute)} {
		now := time.UnixMilli(1_760_000_000_000)
		var tat int64
		for i := 0; i < 300; i++ {
			now = now.Add(time.Duration(rng.IntN(int(l.limit.Window.Milliseconds()/2))) * time.Millisecond / 4)
			next, want, wantRetry := l.limit.Step(tat, now.UnixMilli())
			got, retry := l.allow(e.app, "k", now)
			if got != want || retry != wantRetry {
				t.Fatalf("%s step %d: sql (%v, %v), domain (%v, %v)", l.name, i, got, retry, want, wantRetry)
			}
			tat = next
		}
	}
}
