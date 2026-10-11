package domain

import (
	"slices"
	"strings"
	"testing"
)

func TestClientIP(t *testing.T) {
	prod := ProxyTrust{Proxies: DefaultTrustedProxies(), Cloudflare: CloudflareRanges()}
	noCF := ProxyTrust{Proxies: DefaultTrustedProxies()}
	none := ProxyTrust{}
	tests := []struct {
		name   string
		trust  ProxyTrust
		remote string
		xff    []string
		cf     string
		want   string
	}{
		{"direct, no header", prod, "203.0.113.7:4242", nil, "", "203.0.113.7"},
		{"direct, spoofed headers ignored", prod, "203.0.113.7:4242", []string{"1.2.3.4"}, "5.6.7.8", "203.0.113.7"},
		{"traefik only", prod, "10.0.1.5:5000", []string{"198.51.100.9"}, "", "198.51.100.9"},
		{"traefik keeps client XFF: rightmost wins", prod, "10.0.1.5:5000", []string{"1.2.3.4, 198.51.100.9"}, "", "198.51.100.9"},
		{"cloudflare → traefik", prod, "10.0.1.5:5000", []string{"198.51.100.9, 172.70.1.2"}, "198.51.100.9", "198.51.100.9"},
		{"cloudflare → traefik, IPv6 client", prod, "10.0.1.5:5000", []string{"2a02:1810::1, 2606:4700:10::ac43:1"}, "2a02:1810::1", "2a02:1810::1"},
		{"origin hit directly with forged CF header", prod, "10.0.1.5:5000", []string{"203.0.113.66"}, "1.1.1.1", "203.0.113.66"},
		{"forged XFF prefix + forged CF header, direct to traefik", prod, "10.0.1.5:5000", []string{"172.70.1.2, 203.0.113.66"}, "1.1.1.1", "203.0.113.66"},
		{"cloudflare without CF header: XFF entry", prod, "10.0.1.5:5000", []string{"198.51.100.9, 172.70.1.2"}, "", "198.51.100.9"},
		{"cloudflare connects directly", prod, "172.70.1.2:443", nil, "198.51.100.9", "198.51.100.9"},
		{"cloudflare not trusted", noCF, "10.0.1.5:5000", []string{"198.51.100.9, 172.70.1.2"}, "198.51.100.9", "172.70.1.2"},
		{"no trusted proxy at all", none, "10.0.1.5:5000", []string{"198.51.100.9"}, "198.51.100.9", "10.0.1.5"},
		{"several XFF headers are concatenated", prod, "127.0.0.1:1", []string{"203.0.113.1", "10.0.0.2"}, "", "203.0.113.1"},
		{"garbage XFF stops the walk", prod, "10.0.1.5:5000", []string{"nope"}, "", "10.0.1.5"},
		{"only proxies: last proxy", prod, "10.0.1.5:5000", []string{"10.0.0.9"}, "", "10.0.0.9"},
		{"IPv4-mapped peer", prod, "[::ffff:203.0.113.7]:80", nil, "", "203.0.113.7"},
		{"invalid remote kept as is", prod, "pipe", nil, "", "pipe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.trust.ClientIP(tt.remote, tt.xff, tt.cf); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePrefixes(t *testing.T) {
	tests := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"", []string{}, false},
		{"none", []string{}, false},
		{"10.0.0.0/8, 172.18.0.5", []string{"10.0.0.0/8", "172.18.0.5/32"}, false},
		{"10.1.2.3/8 ::1", []string{"10.0.0.0/8", "::1/128"}, false},
		{"10.0.0.0/33", nil, true},
		{"abc", nil, true},
	}
	for _, tt := range tests {
		got, err := ParsePrefixes(tt.in)
		if (err != nil) != tt.wantErr {
			t.Fatalf("ParsePrefixes(%q) err = %v", tt.in, err)
		}
		if tt.wantErr {
			continue
		}
		var s []string
		for _, p := range got {
			s = append(s, p.String())
		}
		if s == nil {
			s = []string{}
		}
		if !slices.Equal(s, tt.want) {
			t.Fatalf("ParsePrefixes(%q) = %v, want %v", tt.in, s, tt.want)
		}
	}
}

func TestIsLocalHost(t *testing.T) {
	for h, want := range map[string]bool{
		"localhost": true, "127.0.0.1": true, "[::1]": true, "app.localhost": true,
		"eat.fs0ciety.org": false, "10.0.0.1": false, "": false,
	} {
		if got := IsLocalHost(h); got != want {
			t.Errorf("IsLocalHost(%q) = %v", h, got)
		}
	}
}

func TestInlineScriptHashes(t *testing.T) {
	html := []byte(`<html><head><script>alert(1)</script>
<script type="module" src="/assets/index.js"></script>
<script type="application/ld+json">{"a":1}</script>
<SCRIPT type='text/javascript'>alert(1)</SCRIPT>
<script type="module">import "/x.js"</script></head></html>`)
	got := InlineScriptHashes(html)
	// alert(1) → well known hash (deduplicated), and the inline module
	want := []string{"sha256-bhHHL3z2vDgxUt0W3dWQOrprscmda2Y5pLsLg4GF+pI=", "sha256-gGJyvGDuDCRuOVZq69RBtnlPiQTuvDF7Op4nIlpJlnI="}
	if len(got) != 2 || got[0] != want[0] {
		t.Fatalf("hashes = %v", got)
	}
	if len(InlineScriptHashes([]byte(`<script src="/theme-init.js"></script>`))) != 0 {
		t.Fatal("external script hashed")
	}
}

func TestCSP(t *testing.T) {
	o := CSPOptions{ScriptHashes: []string{"sha256-abc"}, ConnectSrc: []string{"https://nominatim.openstreetmap.org"}, ReportURI: "/api/occ/csp-report"}
	app := o.AppCSP()
	for _, part := range []string{
		"default-src 'self'", "script-src 'self' 'sha256-abc'", "style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob: https:", "connect-src 'self' https://nominatim.openstreetmap.org;",
		"frame-ancestors 'none'", "object-src 'none'", "base-uri 'self'", "form-action 'self'",
		"report-uri /api/occ/csp-report",
	} {
		if !strings.Contains(app, part) {
			t.Errorf("app CSP lacks %q: %s", part, app)
		}
	}
	if strings.Contains(app, "upgrade-insecure-requests") || strings.Contains(app, "unsafe-eval") {
		t.Errorf("app CSP: %s", app)
	}
	o.UpgradeInsecure = true
	if !strings.Contains(o.AppCSP(), "upgrade-insecure-requests") {
		t.Error("upgrade-insecure-requests missing")
	}
	if w := o.WorkerCSP(); !strings.Contains(w, "connect-src 'self' https://nominatim.openstreetmap.org https:;") || strings.Contains(o.AppCSP(), "openstreetmap.org https:") {
		t.Errorf("worker CSP: %s", w)
	}
}
