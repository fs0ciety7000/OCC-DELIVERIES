package notify

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

var party = Party{ID: "abc123def456ghi", Title: "Midi du vendredi", Code: "K7M2QX"}

func TestStatusMessages(t *testing.T) {
	deadline := time.Date(2026, 10, 9, 9, 45, 0, 0, time.UTC) // 11:45 in Brussels (CEST)
	cases := []struct {
		in        StatusInput
		ok        bool
		title     string
		bodyParts []string
	}{
		{StatusInput{Party: party, Status: domain.StatusVoting, Deadline: deadline}, true, "Le vote est ouvert 🗳️", []string{"« Midi du vendredi »", "avant 11:45"}},
		{StatusInput{Party: party, Status: domain.StatusOrdering, Restaurant: "Pizza Nonna"}, true, "On commande chez Pizza Nonna 🍽️", []string{"Compose ton panier"}},
		{StatusInput{Party: party, Status: domain.StatusOrdering, Auto: true}, true, "À vos paniers 🍽️", []string{"Vote clôturé automatiquement"}},
		{StatusInput{Party: party, Status: domain.StatusReview}, true, "Le récap est prêt 🧾", []string{"verrouillés"}},
		{StatusInput{Party: Party{ID: "x", Code: "K7M2QX"}, Status: domain.StatusClosed}, true, "Commande clôturée ✅", []string{"La commande K7M2QX est terminée"}},
		{StatusInput{Party: party, Status: domain.StatusCancelled}, true, "Commande annulée", []string{"« Midi du vendredi » a été annulée"}},
		{StatusInput{Party: party, Status: domain.StatusLobby}, false, "", nil},
		{StatusInput{Party: party, Status: domain.StatusPaying}, false, "", nil},
	}
	for _, c := range cases {
		m, ok := StatusChanged(c.in)
		if ok != c.ok {
			t.Fatalf("%s: ok=%v", c.in.Status, ok)
		}
		if !ok {
			continue
		}
		if m.Title != c.title {
			t.Errorf("%s: title %q, want %q", c.in.Status, m.Title, c.title)
		}
		for _, part := range c.bodyParts {
			if !strings.Contains(m.Body, part) {
				t.Errorf("%s: body %q lacks %q", c.in.Status, m.Body, part)
			}
		}
		if m.URL != "/party/"+c.in.Party.ID || m.Tag != "party-"+c.in.Party.ID || m.Category != CategoryParty {
			t.Errorf("%s: link/tag/category %q %q %q", c.in.Status, m.URL, m.Tag, m.Category)
		}
	}
}

func TestPersonalMessages(t *testing.T) {
	m := Paying(party, "Bob", 1240, false, 0)
	if m.Title != "Paiement : tu dois 12,40 €" || !strings.Contains(m.Body, "Rembourse Bob") || m.Category != CategoryPayments {
		t.Fatalf("paying debtor: %+v", m)
	}
	m = Paying(party, "Bob", 0, true, 3610)
	if !strings.Contains(m.Body, "36,10 €") {
		t.Fatalf("paying payer: %+v", m)
	}
	m = PaymentDeclared(party, "Bob", 1240, domain.MethodWero)
	if m.Body != "Bob a déclaré t'avoir remboursé 12,40 € (Wero). Pense à confirmer." {
		t.Fatalf("declared: %q", m.Body)
	}
	m = PaymentConfirmed(party, "Bob", 1240)
	if m.Body != "Bob a confirmé ton remboursement de 12,40 €." {
		t.Fatalf("confirmed: %q", m.Body)
	}
	if MemberJoined(party, "Alice", 0).Title != "Alice a rejoint la commande 👋" ||
		MemberJoined(party, "Alice", 2).Title != "Alice et 2 autres personnes ont rejoint" {
		t.Fatal("joined titles")
	}
	if ReminderVote(party, time.Date(2026, 1, 9, 10, 45, 0, 0, time.UTC)).Body != "Le vote de « Midi du vendredi » se termine à 11:45." {
		t.Fatal("winter time reminder")
	}
}

func TestPayloadAndTopic(t *testing.T) {
	m := AllReady(party)
	var p Payload
	if err := json.Unmarshal(m.JSON(time.Unix(10, 0)), &p); err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindAllReady || p.URL != "/party/"+party.ID || p.Icon != IconURL || p.Badge != BadgeURL || !p.Renotify || p.TS != 10000 || p.PartyID != party.ID {
		t.Fatalf("payload: %+v", p)
	}
	long := Message{Title: strings.Repeat("é", 300), Body: strings.Repeat("a", 1000)}
	if err := json.Unmarshal(long.JSON(time.Now()), &p); err != nil {
		t.Fatal(err)
	}
	if len([]rune(p.Title)) != maxTitle || len([]rune(p.Body)) != maxBody {
		t.Fatalf("clip: %d %d", len([]rune(p.Title)), len([]rune(p.Body)))
	}
	if Topic("abc/def!") != "party-abcdef" || len(Topic(strings.Repeat("a", 50))) != 32 {
		t.Fatal("topic")
	}
}

func TestPrefsAndRecipients(t *testing.T) {
	cases := []struct {
		raw  string
		want Prefs
	}{
		{"", DefaultPrefs()},
		{"null", DefaultPrefs()},
		{"{oops", DefaultPrefs()},
		{`{"party":false}`, Prefs{Party: false, Payments: true, Reminders: true}},
		{`{"party":true,"payments":false,"reminders":false}`, Prefs{Party: true}},
		{`{"payments":"no"}`, DefaultPrefs()},
	}
	for _, c := range cases {
		if got := ParsePrefs([]byte(c.raw)); got != c.want {
			t.Errorf("%q: %+v, want %+v", c.raw, got, c.want)
		}
	}
	prefs := map[string]Prefs{"a": {Party: false, Payments: true, Reminders: true}, "b": {Party: true, Reminders: false}}
	if got := FilterByPrefs([]string{"a", "b", "c"}, prefs, CategoryParty); strings.Join(got, ",") != "b,c" {
		t.Fatalf("party filter: %v", got)
	}
	if got := FilterByPrefs([]string{"a", "b", "c"}, prefs, CategoryReminders); strings.Join(got, ",") != "a,c" {
		t.Fatalf("reminders filter: %v", got)
	}
	if got := FilterByPrefs([]string{"a", "b"}, prefs, CategorySystem); len(got) != 2 {
		t.Fatalf("system always: %v", got)
	}
	if got := Recipients([]string{"host", "a", "", "a", "b"}, "host"); strings.Join(got, ",") != "a,b" {
		t.Fatalf("actor excluded: %v", got)
	}
}

// ------------------------------------------------------------ sender

type memStore struct {
	mu        sync.Mutex
	subs      []Subscription
	prefs     map[string]Prefs
	delivered []string
	deleted   []string
	failures  map[string]int
}

func (s *memStore) Targets(users []string, c Category) ([]Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	allowed := map[string]bool{}
	for _, u := range FilterByPrefs(users, s.prefs, c) {
		allowed[u] = true
	}
	var out []Subscription
	for _, sub := range s.subs {
		if allowed[sub.UserID] {
			out = append(out, sub)
		}
	}
	return out, nil
}

func (s *memStore) Delivered(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delivered = append(s.delivered, id)
}

func (s *memStore) Failed(id string, gone bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gone {
		s.deleted = append(s.deleted, id)
		return
	}
	if s.failures == nil {
		s.failures = map[string]int{}
	}
	s.failures[id]++
}

func browserKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	return base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(secret)
}

// TestSenderWebPush posts real encrypted messages to fake push services:
// 201 = delivered, 410 / 404 = subscription removed, 500 = failure counted.
func TestSenderWebPush(t *testing.T) {
	type hit struct {
		path    string
		headers http.Header
		size    int
	}
	var mu sync.Mutex
	var hits []hit
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		mu.Lock()
		hits = append(hits, hit{r.URL.Path, r.Header.Clone(), n})
		mu.Unlock()
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusCreated)
		case "/gone":
			w.WriteHeader(http.StatusGone)
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	keys, err := GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	keys.Subject = "mailto:noreply@occ.test"
	sub := func(id, user, path string) Subscription {
		p, a := browserKeys(t)
		return Subscription{ID: id, UserID: user, Endpoint: srv.URL + path, P256dh: p, Auth: a}
	}
	store := &memStore{
		subs: []Subscription{
			sub("s1", "alice", "/ok"), sub("s2", "alice", "/gone"), sub("s3", "bob", "/missing"),
			sub("s4", "bob", "/boom"), sub("s5", "carol", "/ok"),
		},
		prefs: map[string]Prefs{"carol": {Party: false, Payments: true, Reminders: true}},
	}
	s := NewSender(store, WebPusher{Keys: keys, Client: srv.Client()}, Options{Workers: 2})
	defer s.Close()

	msg, _ := StatusChanged(StatusInput{Party: party, Status: domain.StatusReview})
	if err := s.Send([]string{"alice", "bob", "carol"}, msg); err != nil {
		t.Fatal(err)
	}
	s.Wait()

	if strings.Join(store.delivered, ",") != "s1" {
		t.Fatalf("delivered: %v", store.delivered)
	}
	if len(store.deleted) != 2 || store.failures["s4"] != 1 {
		t.Fatalf("deleted %v failures %v", store.deleted, store.failures)
	}
	if len(hits) != 4 { // carol muted the party category
		t.Fatalf("hits: %d", len(hits))
	}
	for _, h := range hits {
		if h.headers.Get("Content-Encoding") != "aes128gcm" || h.headers.Get("Topic") != Topic(party.ID) ||
			h.headers.Get("Urgency") != "normal" || h.headers.Get("TTL") != "7200" ||
			!strings.HasPrefix(h.headers.Get("Authorization"), "vapid t=") || h.size < 100 {
			t.Fatalf("push request: %v (%d bytes)", h.headers, h.size)
		}
	}
}

type countingPusher struct {
	mu sync.Mutex
	n  int
}

func (c *countingPusher) Push(ctx context.Context, sub Subscription, payload []byte, opts PushOptions) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return http.StatusCreated, nil
}

func TestSenderQueueAndClose(t *testing.T) {
	store := &memStore{subs: []Subscription{{ID: "s1", UserID: "u"}}}
	p := &countingPusher{}
	s := NewSender(store, p, Options{Workers: 1, QueueSize: 1000})
	for i := 0; i < 50; i++ {
		_ = s.Send([]string{"u"}, Test())
	}
	_ = s.Send(nil, Test()) // ignored
	s.Close()
	if p.n != 50 {
		t.Fatalf("pushed %d", p.n)
	}
	if err := s.Send([]string{"u"}, Test()); err != nil { // after close: silently dropped
		t.Fatal(err)
	}
	var nilSender *Sender
	if nilSender.Send([]string{"u"}, Test()) != nil {
		t.Fatal("nil sender")
	}
	nilSender.Wait()
	nilSender.Close()
}

func TestTeamLaunched(t *testing.T) {
	m := TeamLaunched(Party{ID: "p9", Title: "Midi du lundi"}, "Compta")
	if m.Title != "Compta : la commande du jour est lancée" || m.Body != "Rejoins « Midi du lundi »." || m.URL != "/party/p9" || m.Category != CategoryParty {
		t.Fatalf("team launched: %+v", m)
	}
}
