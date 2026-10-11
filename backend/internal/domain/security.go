package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

// ------------------------------------------------------------- client IP

// cloudflareRanges are the published Cloudflare edge ranges
// (https://www.cloudflare.com/ips-v4, /ips-v6, checked 2026-10-11).
var cloudflareRanges = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
}

// defaultProxyRanges: loopback and private networks, i.e. a reverse proxy on
// the same host / Docker network (Coolify's Traefik).
var defaultProxyRanges = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128", "fc00::/7",
}

func mustPrefixes(list []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

// CloudflareRanges returns the Cloudflare edge ranges.
func CloudflareRanges() []netip.Prefix { return mustPrefixes(cloudflareRanges) }

// DefaultTrustedProxies returns the loopback and private ranges.
func DefaultTrustedProxies() []netip.Prefix { return mustPrefixes(defaultProxyRanges) }

// ParsePrefixes parses a comma / space separated list of CIDR ranges or
// single addresses; "none" (or "-") is an explicit empty list.
func ParsePrefixes(s string) ([]netip.Prefix, error) {
	out := []netip.Prefix{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\n' }) {
		if f == "none" || f == "-" {
			continue
		}
		if strings.Contains(f, "/") {
			p, err := netip.ParsePrefix(f)
			if err != nil {
				return nil, fmt.Errorf("plage invalide %q", f)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(f)
		if err != nil {
			return nil, fmt.Errorf("adresse invalide %q", f)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// ProxyTrust decides which forwarding headers are believed.
type ProxyTrust struct {
	// Proxies are the reverse proxies that connect to the server directly
	// (their X-Forwarded-For entry is believed).
	Proxies []netip.Prefix
	// Cloudflare are the Cloudflare edge ranges: when the hop that reached
	// our proxy is one of them, CF-Connecting-IP is the client. Empty =
	// CF-Connecting-IP is never read.
	Cloudflare []netip.Prefix
}

func inAny(a netip.Addr, list []netip.Prefix) bool {
	for _, p := range list {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func parseIP(s string) (netip.Addr, bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap().WithZone(""), true
}

// ClientIP resolves the client address of a request from the TCP peer
// (remoteAddr, "ip:port"), its X-Forwarded-For header values (in order) and
// CF-Connecting-IP. Headers are only believed hop by hop, from the right:
// a trusted proxy's X-Forwarded-For entry gives the previous hop; a
// Cloudflare edge hop gives CF-Connecting-IP; the first other hop is the
// client. A client connecting directly (or to the proxy without Cloudflare)
// cannot spoof its address with these headers.
func (t ProxyTrust) ClientIP(remoteAddr string, xff []string, cfConnectingIP string) string {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	cur, ok := parseIP(host)
	if !ok {
		return host
	}
	var hops []string
	for _, v := range xff {
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hops = append(hops, h)
			}
		}
	}
	i := len(hops) - 1
	for {
		if inAny(cur, t.Cloudflare) {
			if cf, ok := parseIP(cfConnectingIP); ok {
				return cf.String()
			}
			// no header: Cloudflare appended the client to X-Forwarded-For
		} else if !inAny(cur, t.Proxies) {
			return cur.String()
		}
		if i < 0 {
			return cur.String()
		}
		next, ok := parseIP(hops[i])
		if !ok {
			return cur.String()
		}
		cur, i = next, i-1
	}
}

// IsLocalHost reports whether host (without port) is a loopback name/address.
func IsLocalHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	a, ok := parseIP(h)
	return ok && a.IsLoopback()
}

// ------------------------------------------------- Content-Security-Policy

// CSPOptions are the variable parts of the policies.
type CSPOptions struct {
	// ScriptHashes are the "sha256-…" sources of the inline scripts of the
	// SPA index (InlineScriptHashes).
	ScriptHashes []string
	// ConnectSrc are the extra origins the pages call (fetch / XHR / SSE).
	ConnectSrc []string
	// UpgradeInsecure adds upgrade-insecure-requests (HTTPS deployments).
	UpgradeInsecure bool
	// ReportURI receives the violation reports ("" = none).
	ReportURI string
}

func joinCSP(directives [][]string) string {
	parts := make([]string, 0, len(directives))
	for _, d := range directives {
		parts = append(parts, strings.Join(d, " "))
	}
	return strings.Join(parts, "; ")
}

func (o CSPOptions) build(connect []string) string {
	script := append([]string{"script-src", "'self'"}, quoteHashes(o.ScriptHashes)...)
	d := [][]string{
		{"default-src", "'self'"},
		script,
		// motion / sonner write style attributes and <style> elements
		{"style-src", "'self'", "'unsafe-inline'"},
		// restaurant covers (cover_url) come from many hosts
		{"img-src", "'self'", "data:", "blob:", "https:"},
		{"font-src", "'self'"},
		append([]string{"connect-src", "'self'"}, connect...),
		{"worker-src", "'self'"},
		{"manifest-src", "'self'"},
		{"frame-src", "'none'"},
		{"frame-ancestors", "'none'"},
		{"base-uri", "'self'"},
		{"form-action", "'self'"},
		{"object-src", "'none'"},
	}
	if o.UpgradeInsecure {
		d = append(d, []string{"upgrade-insecure-requests"})
	}
	if o.ReportURI != "" {
		d = append(d, []string{"report-uri", o.ReportURI})
	}
	return joinCSP(d)
}

func quoteHashes(h []string) []string {
	out := make([]string, 0, len(h))
	for _, s := range h {
		out = append(out, "'"+s+"'")
	}
	return out
}

// AppCSP is the policy of the SPA pages (and the static files).
func (o CSPOptions) AppCSP() string { return o.build(o.ConnectSrc) }

// WorkerCSP is the policy of the service worker script: it also fetches the
// remote restaurant covers it caches (connect-src https:).
func (o CSPOptions) WorkerCSP() string {
	return o.build(append(append([]string{}, o.ConnectSrc...), "https:"))
}

// APICSP is the policy of the JSON API responses (nothing to load or run).
const APICSP = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// AdminCSP is the only directive set on PocketBase's admin UI (/_/), which
// keeps its own needs (inline styles, scripts…).
const AdminCSP = "frame-ancestors 'self'"

// PermissionsPolicy denies the powerful features the app does not use;
// geolocation (« près de moi »), Web Share, Wake Lock (« Présenter »),
// clipboard and WebAuthn stay available to our own origin (default).
const PermissionsPolicy = "accelerometer=(), bluetooth=(), browsing-topics=(), camera=(), display-capture=(), " +
	"geolocation=(self), gyroscope=(), hid=(), magnetometer=(), microphone=(), midi=(), payment=(), serial=(), usb=()"

// HSTS is the Strict-Transport-Security value of HTTPS deployments.
const HSTS = "max-age=31536000; includeSubDomains"

var inlineScriptRe = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script\s*>`)
var scriptTypeRe = regexp.MustCompile(`(?i)\btype\s*=\s*["']?([^"'\s>]+)`)
var scriptSrcRe = regexp.MustCompile(`(?i)\bsrc\s*=`)

// InlineScriptHashes returns the CSP hash sources ("sha256-<base64>") of the
// executable inline scripts of an HTML document (no src, JavaScript type).
func InlineScriptHashes(html []byte) []string {
	out := []string{}
	for _, m := range inlineScriptRe.FindAllSubmatch(html, -1) {
		attrs, body := m[1], m[2]
		if scriptSrcRe.Match(attrs) {
			continue
		}
		if t := scriptTypeRe.FindSubmatch(attrs); t != nil {
			switch strings.ToLower(string(t[1])) {
			case "module", "text/javascript", "application/javascript":
			default:
				continue // data blocks (JSON, templates) are not executed
			}
		}
		sum := sha256.Sum256(body)
		h := "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
}
