package app

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/notify"
)

// recPusher records every push (plaintext payload) instead of sending it.
type recPusher struct {
	mu   sync.Mutex
	sent []sentPush
}

type sentPush struct {
	endpoint string
	payload  notify.Payload
	opts     notify.PushOptions
}

func (r *recPusher) Push(_ context.Context, sub notify.Subscription, payload []byte, opts notify.PushOptions) (int, error) {
	var p notify.Payload
	_ = json.Unmarshal(payload, &p)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, sentPush{sub.Endpoint, p, opts})
	if strings.HasSuffix(sub.Endpoint, "/gone") {
		return http.StatusGone, nil
	}
	return http.StatusCreated, nil
}

// take returns and clears the pushes, as "endpoint-suffix: title" sorted.
func (r *recPusher) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.sent))
	for _, s := range r.sent {
		out = append(out, s.endpoint[strings.LastIndex(s.endpoint, "/")+1:]+": "+s.payload.Title)
	}
	r.sent = nil
	sort.Strings(out)
	return out
}

func (r *recPusher) last() sentPush {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sent[len(r.sent)-1]
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

type pushEnv struct {
	*env
	h     *handlers
	rec   *recPusher
	clock *clock
}

func newPushEnv(t *testing.T, mutate func(*PushConfig)) *pushEnv {
	t.Helper()
	ta, err := tests.NewTestApp(templateDir)
	if err != nil {
		t.Fatal(err)
	}
	rec := &recPusher{}
	clk := &clock{t: time.Now()}
	cfg := testConfig
	cfg.Push = PushConfig{Enabled: true, Subject: "mailto:noreply@occ.test", Pusher: rec, Now: clk.now}
	if mutate != nil {
		mutate(&cfg.Push)
	}
	h := register(ta, cfg)
	t.Cleanup(ta.Cleanup)
	return &pushEnv{env: serveEnv(t, ta), h: h, rec: rec, clock: clk}
}

func browserSubscription(t *testing.T, endpoint string) map[string]any {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	return map[string]any{"endpoint": endpoint, "keys": map[string]any{
		"p256dh": base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()),
		"auth":   base64.RawURLEncoding.EncodeToString(secret),
	}}
}

// subscribe registers a device for u at https://push.test/<name>.
func (pe *pushEnv) subscribe(u user, name string) {
	pe.t.Helper()
	pe.expect(200, "POST", "/api/occ/push/subscribe", u.token, browserSubscription(pe.t, "https://push.test/"+name))
}

func (pe *pushEnv) flush() []string {
	pe.h.push.wait()
	return pe.rec.take()
}

func TestPushSubscriptionEndpoints(t *testing.T) {
	pe := newPushEnv(t, nil)
	alice, bob := pe.user("Alice"), pe.user("Bob")

	// public key: generated once, persisted, never the private key
	r := pe.expect(200, "GET", "/api/occ/push/public-key", "", nil).m(t)
	pub, _ := r["publicKey"].(string)
	if r["enabled"] != true || len(pub) < 80 || strings.Contains(string(pe.do("GET", "/api/occ/push/public-key", "", nil).body), "private") {
		t.Fatalf("public key: %v", r)
	}
	again, err := resolveVAPID(pe.app, PushConfig{Subject: "mailto:x@y.z"})
	if err != nil || again.Public != pub || again.Private == "" {
		t.Fatalf("keys must survive restarts: %v %v", again.Public, err)
	}
	if n, _ := pe.app.CountRecords(colServerSecrets); n != 1 {
		t.Fatalf("server_secrets rows: %d", n)
	}
	// server_secrets is superuser only
	pe.expect(403, "GET", "/api/collections/server_secrets/records", alice.token, nil)

	// validation
	pe.expect(401, "POST", "/api/occ/push/subscribe", "", browserSubscription(t, "https://push.test/a"))
	pe.expect(400, "POST", "/api/occ/push/subscribe", alice.token, browserSubscription(t, "http://push.test/a"))
	bad := browserSubscription(t, "https://push.test/a")
	bad["keys"].(map[string]any)["p256dh"] = "AAAA"
	pe.expect(400, "POST", "/api/occ/push/subscribe", alice.token, bad)
	bad = browserSubscription(t, "https://push.test/a")
	bad["keys"].(map[string]any)["auth"] = "!!"
	pe.expect(400, "POST", "/api/occ/push/subscribe", alice.token, bad)

	// test without device → 400 in French
	if r := pe.expect(400, "POST", "/api/occ/push/test", alice.token, nil); !strings.Contains(string(r.body), "Aucun appareil") {
		t.Fatalf("test without device: %s", r.body)
	}

	pe.subscribe(alice, "alice-phone")
	pe.subscribe(alice, "alice-phone") // idempotent
	if n, _ := pe.app.CountRecords(colPushSubscriptions, dbx.HashExp{"user": alice.id()}); n != 1 {
		t.Fatalf("subscriptions: %d", n)
	}
	// owner-only rules; no client writes through the collection
	if list := pe.expect(200, "GET", "/api/collections/push_subscriptions/records", bob.token, nil).m(t); list["totalItems"].(float64) != 0 {
		t.Fatalf("bob sees alice's devices: %v", list)
	}
	if list := pe.expect(200, "GET", "/api/collections/push_subscriptions/records", alice.token, nil).m(t); list["totalItems"].(float64) != 1 {
		t.Fatalf("alice devices: %v", list)
	}
	pe.expect(403, "POST", "/api/collections/push_subscriptions/records", alice.token, map[string]any{
		"user": alice.id(), "endpoint": "https://evil.test/x", "p256dh": "x", "auth": "y",
	})

	pe.expect(200, "POST", "/api/occ/push/test", alice.token, nil)
	if got := pe.flush(); len(got) != 1 || got[0] != "alice-phone: Notifications activées 🔔" {
		t.Fatalf("test push: %v", got)
	}

	// the same browser logs in as Bob: the endpoint follows the account
	pe.subscribe(bob, "alice-phone")
	if n, _ := pe.app.CountRecords(colPushSubscriptions, dbx.HashExp{"user": bob.id()}); n != 1 {
		t.Fatal("endpoint should move to bob")
	}
	pe.expect(400, "POST", "/api/occ/push/test", alice.token, nil)

	// prefs
	p := pe.expect(200, "GET", "/api/occ/push/prefs", bob.token, nil).m(t)
	if p["devices"].(float64) != 1 || p["prefs"].(map[string]any)["reminders"] != true {
		t.Fatalf("prefs: %v", p)
	}
	pe.expect(400, "PATCH", "/api/occ/push/prefs", bob.token, map[string]any{})
	p = pe.expect(200, "PATCH", "/api/occ/push/prefs", bob.token, map[string]any{"reminders": false}).m(t)
	if pr := p["prefs"].(map[string]any); pr["reminders"] != false || pr["party"] != true {
		t.Fatalf("prefs patch: %v", p)
	}
	// notify_prefs is hidden: not exposed, not writable through the collection
	u := pe.expect(200, "GET", "/api/collections/users/records/"+bob.id(), bob.token, nil).m(t)
	if _, ok := u["notify_prefs"]; ok {
		t.Fatal("notify_prefs must be hidden")
	}

	// unsubscribe: only one's own endpoint
	if r := pe.expect(200, "DELETE", "/api/occ/push/subscribe", alice.token, map[string]any{"endpoint": "https://push.test/alice-phone"}).m(t); r["deleted"].(float64) != 0 {
		t.Fatal("alice cannot delete bob's endpoint")
	}
	pe.expect(200, "DELETE", "/api/occ/push/subscribe", bob.token, map[string]any{"endpoint": "https://push.test/alice-phone"})
	if n, _ := pe.app.CountRecords(colPushSubscriptions); n != 0 {
		t.Fatalf("left: %d", n)
	}
}

func TestPushDisabled(t *testing.T) {
	e := newEnv(t) // testConfig: push disabled
	alice := e.user("Alice")
	if r := e.expect(200, "GET", "/api/occ/push/public-key", "", nil).m(t); r["enabled"] != false || r["publicKey"] != "" {
		t.Fatalf("disabled: %v", r)
	}
	e.expect(400, "POST", "/api/occ/push/subscribe", alice.token, browserSubscription(t, "https://push.test/a"))
}

// TestPushGoneCleanup sends through the real Web Push transport to a fake
// push service answering 410 Gone: the subscription is deleted.
func TestPushGoneCleanup(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") != "aes128gcm" || !strings.HasPrefix(r.Header.Get("Authorization"), "vapid ") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/gone") {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	keys, err := notify.GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	pe := newPushEnv(t, func(c *PushConfig) {
		c.PublicKey, c.PrivateKey = keys.Public, keys.Private
		c.Pusher = notify.WebPusher{Keys: notify.Keys{Public: keys.Public, Private: keys.Private, Subject: c.Subject}, Client: srv.Client()}
	})
	alice := pe.user("Alice")
	if r := pe.expect(200, "GET", "/api/occ/push/public-key", "", nil).m(t); r["publicKey"] != keys.Public {
		t.Fatal("configured key must be used")
	}
	pe.expect(200, "POST", "/api/occ/push/subscribe", alice.token, browserSubscription(t, srv.URL+"/ok"))
	pe.expect(200, "POST", "/api/occ/push/subscribe", alice.token, browserSubscription(t, srv.URL+"/gone"))
	pe.expect(200, "POST", "/api/occ/push/test", alice.token, nil)
	pe.h.push.wait()

	subs, err := pe.app.FindAllRecords(colPushSubscriptions)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || !strings.HasSuffix(subs[0].GetString("endpoint"), "/ok") || subs[0].GetString("last_ok") == "" {
		t.Fatalf("after 410: %d subscriptions", len(subs))
	}
}

// TestPushPartyEvents walks a party and checks who is notified of what.
func TestPushPartyEvents(t *testing.T) {
	pe := newPushEnv(t, nil)
	pizza, burger := pe.testRestaurants()
	alice, bob, carol, dave := pe.user("Alice"), pe.user("Bob"), pe.user("Carol"), pe.user("Dave")
	for _, u := range []user{alice, bob, carol, dave} {
		pe.subscribe(u, strings.ToLower(u.rec.GetString("name")))
	}

	party := pe.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{"title": "Midi"}).m(t)
	pid, code := party["id"].(string), party["code"].(string)
	pe.expect(200, "POST", "/api/occ/parties/join", bob.token, map[string]any{"code": code})
	pe.expect(200, "POST", "/api/occ/parties/join", carol.token, map[string]any{"code": code}) // throttled
	pe.clock.set(pe.clock.now().Add(3 * time.Minute))
	pe.expect(200, "POST", "/api/occ/parties/join", dave.token, map[string]any{"code": code})
	if got := pe.flush(); strings.Join(got, "|") != "alice: Bob a rejoint la commande 👋|alice: Dave et 1 autre personne ont rejoint" {
		t.Fatalf("joined: %v", got)
	}

	// Dave mutes « party » notifications
	pe.expect(200, "PATCH", "/api/occ/push/prefs", dave.token, map[string]any{"party": false})

	pe.expect(200, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"candidates": []string{pizza, burger}})
	pe.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "voting"})
	if got := pe.flush(); strings.Join(got, "|") != "bob: Le vote est ouvert 🗳️|carol: Le vote est ouvert 🗳️" {
		t.Fatalf("voting (actor and muted excluded): %v", got)
	}

	pe.expect(200, "PUT", path("/api/occ/parties/%s/ballot", pid), bob.token, map[string]any{"ranking": []string{pizza}})
	pe.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering"})
	if got := pe.flush(); strings.Join(got, "|") != "bob: On commande chez Test Pizza 🍽️|carol: On commande chez Test Pizza 🍽️" {
		t.Fatalf("ordering: %v", got)
	}

	tiramisu := pe.menuItem(pizza, "Tiramisu")
	for _, u := range []user{alice, bob, carol} {
		pe.expect(200, "POST", "/api/collections/order_items/records", u.token, map[string]any{"party": pid, "user": u.id(), "menu_item": tiramisu, "quantity": 1})
	}
	for _, u := range []user{bob, carol, dave} {
		pe.expect(200, "POST", path("/api/occ/parties/%s/ready", pid), u.token, map[string]any{"ready": true})
	}
	pe.expect(200, "POST", path("/api/occ/parties/%s/ready", pid), alice.token, map[string]any{"ready": true}) // host is last: no « tout le monde »
	if got := pe.flush(); len(got) != 0 {
		t.Fatalf("host last ready: %v", got)
	}
	pe.expect(200, "POST", path("/api/occ/parties/%s/ready", pid), bob.token, map[string]any{"ready": false})
	pe.expect(200, "POST", path("/api/occ/parties/%s/ready", pid), bob.token, map[string]any{"ready": true})
	if got := pe.flush(); strings.Join(got, "|") != "alice: Tout le monde est prêt ✅" {
		t.Fatalf("all ready: %v", got)
	}

	pe.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})
	pe.flush()
	pe.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": bob.id()})
	got := pe.flush()
	// Carol's share: 7,00 € ± the delivery-fee cent, which goes to whichever member sorts first
	// (ids created in the same second) — read it from the server instead of hard-coding it.
	payRec, err := pe.app.FindFirstRecordByFilter(colPayments, "party = {:p} && debtor = {:d}", dbx.Params{"p": pid, "d": carol.id()})
	if err != nil {
		t.Fatal(err)
	}
	carolOwes := domain.FormatEUR(payRec.GetInt("amount"))
	if a := payRec.GetInt("amount"); a != 699 && a != 700 {
		t.Fatalf("carol amount = %d", a)
	}
	if strings.Join(got, "|") != "bob: C'est toi qui paies 💳|carol: Paiement : tu dois "+carolOwes {
		t.Fatalf("paying: %v", got)
	}

	var pay struct {
		ID string `json:"id"`
	}
	pay.ID = payRec.Id
	pe.expect(200, "POST", path("/api/occ/payments/%s/action", pay.ID), carol.token, map[string]any{"action": "declare", "method": "revolut"})
	pe.h.push.wait()
	if l := pe.rec.last(); !strings.HasSuffix(l.endpoint, "/bob") || l.payload.Body != "Carol a déclaré t'avoir remboursé "+carolOwes+" (Revolut). Pense à confirmer." || l.payload.URL != "/party/"+pid {
		t.Fatalf("declared: %+v", l)
	}
	pe.rec.take()
	pe.expect(200, "POST", path("/api/occ/payments/%s/action", pay.ID), bob.token, map[string]any{"action": "confirm"})
	got = pe.flush()
	if len(got) < 1 || got[len(got)-1] != "carol: Remboursement confirmé ✅" {
		t.Fatalf("confirmed: %v", got)
	}
}

// TestPushPrefsFilter: a muted category is never pushed.
func TestPushPrefsFilter(t *testing.T) {
	pe := newPushEnv(t, nil)
	alice := pe.user("Alice")
	pe.subscribe(alice, "alice")
	pe.expect(200, "PATCH", "/api/occ/push/prefs", alice.token, map[string]any{"party": false, "payments": false, "reminders": false})
	pe.h.push.send([]string{alice.id()}, notify.AllReady(notify.Party{ID: "p1", Title: "x"}))
	pe.h.push.send([]string{alice.id()}, notify.PaymentConfirmed(notify.Party{ID: "p1"}, "Bob", 100))
	if got := pe.flush(); len(got) != 0 {
		t.Fatalf("muted: %v", got)
	}
	pe.h.push.send([]string{alice.id()}, notify.Test()) // system: always
	if got := pe.flush(); len(got) != 1 {
		t.Fatalf("system: %v", got)
	}
}

// TestPushTeamLaunched: the TeamNotifier hook pushes to the recipients, prefs respected.
func TestPushTeamLaunched(t *testing.T) {
	pe := newPushEnv(t, nil)
	alice, bob := pe.user("Alice"), pe.user("Bob")
	pe.subscribe(alice, "alice")
	pe.subscribe(bob, "bob")
	pe.expect(200, "PATCH", "/api/occ/push/prefs", bob.token, map[string]any{"party": false})
	if teamNotifier != pe.h.push {
		t.Fatal("register must install the push service as TeamNotifier")
	}
	party := pe.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{"title": "Midi du lundi"}).m(t)
	p, _ := pe.app.FindRecordById(colParties, party["id"].(string))
	team := newRecord(p.Collection()) // only GetString("name") is read
	team.Set("name", "Compta")
	if err := pe.h.push.TeamPartyLaunched(pe.app, team, p, []string{alice.id(), bob.id()}); err != nil {
		t.Fatal(err)
	}
	if got := pe.flush(); strings.Join(got, "|") != "alice: Compta : la commande du jour est lancée" {
		t.Fatalf("team launched: %v", got)
	}
}
