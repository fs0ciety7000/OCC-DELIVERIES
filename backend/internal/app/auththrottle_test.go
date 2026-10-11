package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tests"
)

// post sends a JSON POST from remote ("ip") with extra headers.
func (e *env) post(url, remote string, hdr map[string]string, body string) resp {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote + ":4242"
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return resp{status: rec.Code, body: rec.Body.Bytes(), header: rec.Header()}
}

func (e *env) loginFrom(remote string, hdr map[string]string, identity, password string) resp {
	e.t.Helper()
	return e.post("/api/collections/users/auth-with-password", remote, hdr,
		fmt.Sprintf(`{"identity":%q,"password":%q}`, identity, password))
}

func assertTooMany(t *testing.T, r resp) {
	t.Helper()
	if r.status != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d %s", r.status, r.body)
	}
	if !strings.Contains(string(r.body), "Trop de tentatives, réessaie dans ") {
		t.Fatalf("429 message: %s", r.body)
	}
	if s, err := strconv.Atoi(r.header.Get("Retry-After")); err != nil || s < 1 {
		t.Fatalf("Retry-After = %q", r.header.Get("Retry-After"))
	}
}

func TestLoginIdentityLimit(t *testing.T) {
	e := newEnv(t)
	alice := e.user("Alice")
	email := alice.rec.Email()
	for i := 0; i < 8; i++ {
		// a different IP each time: only the identity limit applies
		if r := e.loginFrom(fmt.Sprintf("198.51.100.%d", i+1), nil, email, "wrong-password"); r.status != 400 {
			t.Fatalf("attempt %d: %d %s", i, r.status, r.body)
		}
	}
	// normalised identity (case, spaces): same counter; even the right password waits
	assertTooMany(t, e.loginFrom("198.51.100.50", nil, "  ALICE@example.com ", "password123"))
	assertTooMany(t, e.loginFrom("198.51.100.51", nil, email, "password123"))
	// other identities are not affected
	bob := e.user("Bob")
	if r := e.loginFrom("198.51.100.52", nil, bob.rec.Email(), "password123"); r.status != 200 {
		t.Fatalf("bob: %d %s", r.status, r.body)
	}
	// superusers are protected too
	e.superuser()
	for i := 0; i < 8; i++ {
		r := e.post("/api/collections/_superusers/auth-with-password", fmt.Sprintf("192.0.2.%d", i+10), nil, `{"identity":"root@example.com","password":"nope"}`)
		if r.status != 400 {
			t.Fatalf("superuser attempt %d: %d", i, r.status)
		}
	}
	assertTooMany(t, e.post("/api/collections/_superusers/auth-with-password", "192.0.2.99", nil, `{"identity":"root@example.com","password":"password123456"}`))
}

func TestLoginSuccessResetsIdentity(t *testing.T) {
	e := newEnv(t)
	alice := e.user("Alice")
	email := alice.rec.Email()
	for i := 0; i < 7; i++ {
		e.loginFrom("198.51.100.1", nil, email, "wrong-password")
	}
	if r := e.loginFrom("198.51.100.1", nil, email, "password123"); r.status != 200 {
		t.Fatalf("login: %d %s", r.status, r.body)
	}
	for i := 0; i < 8; i++ {
		if r := e.loginFrom("198.51.100.1", nil, email, "wrong-password"); r.status != 400 {
			t.Fatalf("after reset, attempt %d: %d %s", i, r.status, r.body)
		}
	}
	assertTooMany(t, e.loginFrom("198.51.100.1", nil, email, "wrong-password"))
}

func TestLoginIPLimit(t *testing.T) {
	e := newEnv(t)
	alice := e.user("Alice")
	// successful logins give their IP attempt back (colleagues behind one office IP)
	for i := 0; i < 30; i++ {
		if r := e.loginFrom("203.0.113.7", nil, alice.rec.Email(), "password123"); r.status != 200 {
			t.Fatalf("success %d: %d %s", i, r.status, r.body)
		}
	}
	for i := 0; i < 20; i++ {
		// spoofed forwarding headers from a direct client change nothing
		hdr := map[string]string{"X-Forwarded-For": fmt.Sprintf("10.9.8.%d", i), "CF-Connecting-IP": fmt.Sprintf("1.1.1.%d", i), clientIPHeader: fmt.Sprintf("8.8.8.%d", i)}
		if r := e.loginFrom("203.0.113.7", hdr, fmt.Sprintf("nobody%d@example.com", i), "x"); r.status != 400 {
			t.Fatalf("attempt %d: %d %s", i, r.status, r.body)
		}
	}
	assertTooMany(t, e.loginFrom("203.0.113.7", map[string]string{"X-Forwarded-For": "4.4.4.4"}, "other@example.com", "x"))
	// the IP limit is checked before the password: even a right one waits
	assertTooMany(t, e.loginFrom("203.0.113.7", nil, alice.rec.Email(), "password123"))
	if r := e.loginFrom("198.51.100.9", nil, "other@example.com", "x"); r.status != 400 {
		t.Fatalf("other ip: %d", r.status)
	}
}

func TestClientIPBehindProxies(t *testing.T) {
	cfg := testConfig
	cfg.Security = securityConfigFromEnv(func(string) string { return "" }, cfg.PublicURL) // default trust: private proxies + Cloudflare
	e := newEnvWith(t, cfg, nil)

	// Cloudflare → Traefik (10.0.1.5): CF-Connecting-IP is the client, one bucket each
	for i := 0; i < 25; i++ {
		hdr := map[string]string{"X-Forwarded-For": fmt.Sprintf("198.51.100.%d, 172.70.1.2", i), "CF-Connecting-IP": fmt.Sprintf("198.51.100.%d", i)}
		if r := e.loginFrom("10.0.1.5", hdr, fmt.Sprintf("n%d@example.com", i), "x"); r.status != 400 {
			t.Fatalf("cloudflare client %d: %d %s", i, r.status, r.body)
		}
	}
	// origin reached without Cloudflare: forged CF-Connecting-IP ignored, the
	// hop seen by Traefik is the client
	for i := 0; i < 20; i++ {
		hdr := map[string]string{"X-Forwarded-For": "203.0.113.66", "CF-Connecting-IP": fmt.Sprintf("1.1.1.%d", i)}
		if r := e.loginFrom("10.0.1.5", hdr, fmt.Sprintf("m%d@example.com", i), "x"); r.status != 400 {
			t.Fatalf("direct %d: %d", i, r.status)
		}
	}
	assertTooMany(t, e.loginFrom("10.0.1.5", map[string]string{"X-Forwarded-For": "203.0.113.66", "CF-Connecting-IP": "9.9.9.9"}, "z@example.com", "x"))

	// PocketBase's RealIP (logs, built-in limiter) reads our resolved IP only
	if got := e.app.Settings().TrustedProxy.Headers; len(got) != 1 || got[0] != clientIPHeader {
		t.Fatalf("trusted proxy headers = %v", got)
	}
}

func TestRateLimitsSurviveRestart(t *testing.T) {
	ta, err := tests.NewTestApp(templateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer ta.Cleanup()
	Register(ta, testConfig)
	e1 := serveEnv(t, ta)
	alice := e1.user("Alice")
	for i := 0; i < 8; i++ {
		e1.loginFrom("198.51.100.1", nil, alice.rec.Email(), "wrong")
	}
	assertTooMany(t, e1.loginFrom("198.51.100.1", nil, alice.rec.Email(), "wrong"))
	// guest creation limit (10 / h / IP) too
	for i := 0; i < guestCreatePerHour; i++ {
		e1.doIP("203.0.113.7", "POST", "/api/occ/guest", "", `{"name":"x","partyCode":"ZZZZZZ"}`)
	}
	if err := ta.ClearBootstrap(); err != nil { // closes the database (restart)
		t.Fatal(err)
	}

	ta2, err := tests.NewTestApp(ta.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ta2.Cleanup()
	Register(ta2, testConfig)
	e2 := serveEnv(t, ta2)
	assertTooMany(t, e2.loginFrom("198.51.100.1", nil, alice.rec.Email(), "password123"))
	r := e2.doIP("203.0.113.7", "POST", "/api/occ/guest", "", `{"name":"x","partyCode":"ZZZZZZ"}`)
	if r.status != http.StatusTooManyRequests || r.header.Get("Retry-After") == "" {
		t.Fatalf("guest limit after restart: %d %v", r.status, r.header)
	}
	// keys are hashed: no IP / e-mail in the table
	var n int
	if err := ta2.DB().NewQuery("SELECT COUNT(*) FROM occ_rate_limits WHERE key LIKE '%example.com%' OR key LIKE '%203.0.113.7%'").Row(&n); err != nil || n != 0 {
		t.Fatalf("raw values stored: %d %v", n, err)
	}
}

func TestMailRequestLimits(t *testing.T) {
	e := newEnv(t)
	alice := e.user("Alice")
	body := fmt.Sprintf(`{"email":%q}`, alice.rec.Email())
	for i := 0; i < 10; i++ {
		if r := e.post("/api/collections/users/request-password-reset", "203.0.113.8", nil, body); r.status != 204 {
			t.Fatalf("reset %d: %d %s", i, r.status, r.body)
		}
	}
	assertTooMany(t, e.post("/api/collections/users/request-verification", "203.0.113.8", nil, body))
	// e-mail change: per account
	for i := 0; i < 3; i++ {
		req := fmt.Sprintf(`{"newEmail":"alice%d@example.org"}`, i)
		if r := e.post("/api/collections/users/request-email-change", fmt.Sprintf("198.51.100.%d", i), map[string]string{"Authorization": alice.token}, req); r.status != 204 {
			t.Fatalf("email change %d: %d %s", i, r.status, r.body)
		}
	}
	assertTooMany(t, e.post("/api/collections/users/request-email-change", "198.51.100.77", map[string]string{"Authorization": alice.token}, `{"newEmail":"alice9@example.org"}`))
}

func TestRateLimitCleanup(t *testing.T) {
	e := newEnv(t)
	e.loginFrom("198.51.100.1", nil, "x@example.com", "x")
	count := func() int {
		var n int
		if err := e.app.DB().NewQuery("SELECT COUNT(*) FROM occ_rate_limits").Row(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count() != 2 { // IP + identity
		t.Fatalf("rows = %d", count())
	}
	if n, err := cleanupRateLimits(e.app, time.Now()); err != nil || n != 0 {
		t.Fatalf("early cleanup removed %d (%v)", n, err)
	}
	if n, err := cleanupRateLimits(e.app, time.Now().Add(16*time.Minute)); err != nil || n != 2 || count() != 0 {
		t.Fatalf("cleanup removed %d (%v)", n, err)
	}
}
