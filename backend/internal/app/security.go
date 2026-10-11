package app

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// HTTP security headers (CSP, HSTS, Permissions-Policy…) on every response
// of our server, and the client IP resolution behind Coolify's proxy and
// Cloudflare (domain.ProxyTrust). See docs/ARCHITECTURE.md « Sécurité HTTP ».

const (
	// clientIPHeader carries the resolved client IP to PocketBase: it is the
	// only trusted proxy header in the settings (RealIP: logs, built-in rate
	// limiter, auth alerts) and is always overwritten by our middleware, so a
	// client value is never believed.
	clientIPHeader = "X-Occ-Client-Ip"
	clientIPKey    = "occClientIP"
	cspReportPath  = "/api/occ/csp-report"
	cspReportMax   = 16 << 10
)

// cspReportLimit: violation reports logged per IP (the rest is dropped).
var cspReportLimit = newRateLimiter("csp-report", 20, time.Minute)

// SecurityConfig is read from OCC_CSP_REPORT_ONLY, OCC_TRUSTED_PROXIES,
// OCC_TRUST_CLOUDFLARE, OCC_PUBLIC_DIR and OCC_PUBLIC_URL.
type SecurityConfig struct {
	// PublicDir holds the SPA (its index.html inline scripts are hashed).
	PublicDir string
	// ReportOnly sends Content-Security-Policy-Report-Only instead of
	// Content-Security-Policy (safety valve).
	ReportOnly bool
	// Trust decides which forwarding headers give the client IP.
	Trust domain.ProxyTrust
	// TrustErr: invalid OCC_TRUSTED_PROXIES → startup refused.
	TrustErr error
	// Secure: HTTPS deployment (OCC_PUBLIC_URL https, not localhost) → HSTS
	// and upgrade-insecure-requests (never for a localhost request).
	Secure bool
	// ConnectSrc: origins the pages call from the browser.
	ConnectSrc []string
}

func securityConfigFromEnv(getenv func(string) string, publicURL string) SecurityConfig {
	cfg := SecurityConfig{
		PublicDir:  strings.TrimSpace(getenv("OCC_PUBLIC_DIR")),
		ReportOnly: boolValue(getenv("OCC_CSP_REPORT_ONLY"), false),
		// admin geocoding (frontend/src/features/admin/geocode.ts)
		ConnectSrc: []string{"https://nominatim.openstreetmap.org"},
	}
	if cfg.PublicDir == "" {
		cfg.PublicDir = "./pb_public"
	}
	if raw := strings.TrimSpace(getenv("OCC_TRUSTED_PROXIES")); raw != "" {
		cfg.Trust.Proxies, cfg.TrustErr = domain.ParsePrefixes(raw)
	} else {
		cfg.Trust.Proxies = domain.DefaultTrustedProxies()
	}
	if boolValue(getenv("OCC_TRUST_CLOUDFLARE"), true) {
		cfg.Trust.Cloudflare = domain.CloudflareRanges()
	}
	if u, err := url.Parse(publicURL); err == nil && u.Scheme == "https" && !domain.IsLocalHost(u.Hostname()) {
		cfg.Secure = true
	}
	return cfg
}

func boolValue(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "oui", "on":
		return true
	case "0", "false", "no", "non", "off":
		return false
	}
	return def
}

type securityState struct {
	cfg       SecurityConfig
	cspHeader string
	app       string // SPA policy (HTTP / localhost)
	appTLS    string // SPA policy with upgrade-insecure-requests
	worker    string // service worker policy
	workerTLS string
}

func newSecurityState(cfg SecurityConfig, scriptHashes []string) *securityState {
	s := &securityState{cfg: cfg, cspHeader: "Content-Security-Policy"}
	if cfg.ReportOnly {
		s.cspHeader = "Content-Security-Policy-Report-Only"
	}
	o := domain.CSPOptions{ScriptHashes: scriptHashes, ConnectSrc: cfg.ConnectSrc, ReportURI: cspReportPath}
	s.app, s.worker = o.AppCSP(), o.WorkerCSP()
	// upgrade-insecure-requests is ignored (with a console warning) in a
	// report-only policy
	o.UpgradeInsecure = !cfg.ReportOnly
	s.appTLS, s.workerTLS = o.AppCSP(), o.WorkerCSP()
	return s
}

// indexScriptHashes hashes the inline scripts of the served index.html (the
// theme bootstrap lives in /theme-init.js; this keeps a future inline
// script working without a CSP change).
func indexScriptHashes(dir string) []string {
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return nil
	}
	return domain.InlineScriptHashes(b)
}

func bindSecurity(app core.App, cfg SecurityConfig) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if cfg.TrustErr != nil {
			return errors.New("OCC_TRUSTED_PROXIES invalide : " + cfg.TrustErr.Error())
		}
		if err := applyTrustedProxySettings(se.App); err != nil {
			return err
		}
		s := newSecurityState(cfg, indexScriptHashes(cfg.PublicDir))
		se.Router.Bind(&hook.Handler[*core.RequestEvent]{
			Id: "occClientIP",
			// before PocketBase's activity logger and rate limiter
			Priority: apis.DefaultActivityLoggerMiddlewarePriority - 1,
			Func: func(e *core.RequestEvent) error {
				ip := cfg.Trust.ClientIP(e.Request.RemoteAddr, e.Request.Header.Values("X-Forwarded-For"), e.Request.Header.Get("CF-Connecting-IP"))
				e.Request.Header.Set(clientIPHeader, ip)
				e.Set(clientIPKey, ip)
				return e.Next()
			},
		})
		se.Router.Bind(&hook.Handler[*core.RequestEvent]{
			Id: "occSecurityHeaders",
			// after PocketBase's securityHeaders (overrides X-Frame-Options)
			Priority: apis.DefaultSecurityHeadersMiddlewarePriority + 1,
			Func:     s.headers,
		})
		se.Router.POST(cspReportPath, cspReport)
		return se.Next()
	})
}

// applyTrustedProxySettings makes PocketBase's RealIP read our resolved IP.
func applyTrustedProxySettings(app core.App) error {
	s := app.Settings()
	want := []string{clientIPHeader}
	if slices.Equal(s.TrustedProxy.Headers, want) && !s.TrustedProxy.UseLeftmostIP {
		return nil
	}
	s.TrustedProxy.Headers = want
	s.TrustedProxy.UseLeftmostIP = false
	return app.Save(s)
}

// clientIP is the resolved client address of a request (never a raw
// forwarding header).
func clientIP(e *core.RequestEvent) string {
	if v, ok := e.Get(clientIPKey).(string); ok && v != "" {
		return v
	}
	return e.RemoteIP()
}

func requestHost(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		return h
	}
	return r.Host
}

func (s *securityState) headers(e *core.RequestEvent) error {
	h := e.Response.Header()
	p := e.Request.URL.Path
	secure := s.cfg.Secure && !domain.IsLocalHost(requestHost(e.Request))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Permissions-Policy", domain.PermissionsPolicy)
	h.Set("X-XSS-Protection", "0") // legacy auditor off (CSP instead)
	if secure {
		h.Set("Strict-Transport-Security", domain.HSTS)
	}
	switch {
	case p == "/_" || strings.HasPrefix(p, "/_/"):
		// PocketBase admin UI: its own needs, only anti-framing
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Content-Security-Policy", domain.AdminCSP)
	case strings.HasPrefix(p, "/api/files/"):
		// PocketBase adds its sandbox CSP to served files when none is set
		h.Set("Cross-Origin-Resource-Policy", "same-site")
	case strings.HasPrefix(p, "/api/"):
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		h.Set(s.cspHeader, domain.APICSP)
	default:
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		switch {
		case p == "/sw.js" && secure:
			h.Set(s.cspHeader, s.workerTLS)
		case p == "/sw.js":
			h.Set(s.cspHeader, s.worker)
		case secure:
			h.Set(s.cspHeader, s.appTLS)
		default:
			h.Set(s.cspHeader, s.app)
		}
	}
	return e.Next()
}

// cspReport: POST /api/occ/csp-report — violation reports (report-uri
// format "application/csp-report", or Reporting API arrays), logged as
// warnings; 20 per minute and per IP, the rest is dropped. Always 204.
func cspReport(e *core.RequestEvent) error {
	if ok, _ := cspReportLimit.allow(e.App, clientIP(e), time.Now()); !ok {
		return e.NoContent(http.StatusNoContent)
	}
	body, err := io.ReadAll(io.LimitReader(e.Request.Body, cspReportMax))
	if err != nil {
		return e.NoContent(http.StatusNoContent)
	}
	for _, r := range parseCSPReports(body) {
		e.App.Logger().Warn("CSP violation",
			"document", r["document"], "directive", r["directive"], "blocked", r["blocked"],
			"source", r["source"], "line", r["line"], "disposition", r["disposition"], "ip", clientIP(e))
	}
	return e.NoContent(http.StatusNoContent)
}

// parseCSPReports normalises both report formats (at most 10 entries, values
// truncated).
func parseCSPReports(body []byte) []map[string]string {
	pick := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k]; ok && v != nil {
				var s string
				switch t := v.(type) {
				case string:
					s = t
				default:
					b, _ := json.Marshal(t)
					s = string(b)
				}
				if len(s) > 300 {
					s = s[:300]
				}
				return s
			}
		}
		return ""
	}
	norm := func(m map[string]any) map[string]string {
		return map[string]string{
			"document":    pick(m, "document-uri", "documentURL"),
			"directive":   pick(m, "effective-directive", "effectiveDirective", "violated-directive"),
			"blocked":     pick(m, "blocked-uri", "blockedURL"),
			"source":      pick(m, "source-file", "sourceFile"),
			"line":        pick(m, "line-number", "lineNumber"),
			"disposition": pick(m, "disposition"),
		}
	}
	var out []map[string]string
	var legacy struct {
		Report map[string]any `json:"csp-report"`
	}
	if json.Unmarshal(body, &legacy) == nil && legacy.Report != nil {
		return append(out, norm(legacy.Report))
	}
	var list []struct {
		Type string         `json:"type"`
		Body map[string]any `json:"body"`
	}
	if json.Unmarshal(body, &list) == nil {
		for _, r := range list {
			if r.Body != nil && (r.Type == "" || r.Type == "csp-violation") && len(out) < 10 {
				out = append(out, norm(r.Body))
			}
		}
	}
	return out
}
