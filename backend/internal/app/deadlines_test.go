package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

type deadlineFixture struct {
	*pushEnv
	pid               string
	pizza, burger     string
	alice, bob, carol user
	base              time.Time
}

// newDeadlineFixture: Alice hosts, Bob and Carol joined, 2 candidates; each
// member has a push subscription named after them.
func newDeadlineFixture(t *testing.T) *deadlineFixture {
	pe := newPushEnv(t, nil)
	f := &deadlineFixture{pushEnv: pe, base: time.Date(2026, 10, 9, 9, 30, 0, 0, time.UTC)}
	pe.clock.set(f.base)
	f.pizza, f.burger = pe.testRestaurants()
	f.alice, f.bob, f.carol = pe.user("Alice"), pe.user("Bob"), pe.user("Carol")
	for _, u := range []user{f.alice, f.bob, f.carol} {
		pe.subscribe(u, strings.ToLower(u.rec.GetString("name")))
	}
	party := pe.expect(200, "POST", "/api/collections/parties/records", f.alice.token, map[string]any{"title": "Midi", "candidates": []string{f.pizza, f.burger}}).m(t)
	f.pid = party["id"].(string)
	for _, u := range []user{f.bob, f.carol} {
		pe.expect(200, "POST", "/api/occ/parties/join", u.token, map[string]any{"code": party["code"]})
	}
	pe.flush()
	return f
}

func (f *deadlineFixture) at(d time.Duration) { f.clock.set(f.base.Add(d)) }

func (f *deadlineFixture) tick() int {
	n := f.h.deadlines.tick()
	f.h.push.wait()
	return n
}

func (f *deadlineFixture) party() map[string]any {
	return f.expect(200, "GET", "/api/collections/parties/records/"+f.pid, f.alice.token, nil).m(f.t)
}

func (f *deadlineFixture) events() []domain.AutoEvent {
	p, err := f.app.FindRecordById(colParties, f.pid)
	if err != nil {
		f.t.Fatal(err)
	}
	return readAutoEvents(p)
}

func (f *deadlineFixture) setDeadline(field string, at time.Time) {
	f.expect(200, "PATCH", "/api/collections/parties/records/"+f.pid, f.alice.token, map[string]any{field: at.UTC().Format(types.DefaultDateLayout)})
}

func TestDeadlineVoteRemindersAndAutoClose(t *testing.T) {
	f := newDeadlineFixture(t)
	end := f.base.Add(10 * time.Minute) // 11:40 Brussels
	f.setDeadline("voting_ends_at", end)
	f.expect(200, "POST", path("/api/occ/parties/%s/transition", f.pid), f.alice.token, map[string]any{"to": "voting"})
	f.expect(200, "POST", "/api/collections/votes/records", f.bob.token, map[string]any{"party": f.pid, "user": f.bob.id(), "restaurant": f.burger})
	f.flush()

	steps := []struct {
		name   string
		at     time.Duration
		acted  int
		pushes string
		status string
	}{
		{"too early", 7 * time.Minute, 0, "", "voting"},
		{"reminder to those who did not vote", 8 * time.Minute, 1, "alice: Plus que 2 minutes pour voter ⏳|carol: Plus que 2 minutes pour voter ⏳", "voting"},
		{"reminder only once", 9 * time.Minute, 0, "", "voting"},
		{"auto close at the deadline", 10*time.Minute + 20*time.Second, 1, "alice: On commande chez Test Burger 🍽️|bob: On commande chez Test Burger 🍽️|carol: On commande chez Test Burger 🍽️", "ordering"},
		{"nothing more", 11 * time.Minute, 0, "", "ordering"},
	}
	for _, s := range steps {
		f.at(s.at)
		if n := f.tick(); n != s.acted {
			t.Fatalf("%s: acted %d, want %d", s.name, n, s.acted)
		}
		if got := strings.Join(f.rec.take(), "|"); got != s.pushes {
			t.Fatalf("%s: pushes %q, want %q", s.name, got, s.pushes)
		}
		if st := f.party()["status"]; st != s.status {
			t.Fatalf("%s: status %v", s.name, st)
		}
	}
	p := f.party()
	if p["restaurant"] != f.burger {
		t.Fatalf("winner: %v", p["restaurant"])
	}
	ev := f.events()
	if len(ev) != 2 || ev[0].Kind != "reminder_vote" || ev[1].Kind != "vote_closed" || ev[1].Text != "Vote clôturé automatiquement à 11:40 — Test Burger" {
		t.Fatalf("auto events: %+v", ev)
	}
	if _, ok := p["auto_state"]; ok {
		t.Fatal("auto_state must be hidden")
	}
	// auto_events cannot be forged by the host
	f.expect(400, "PATCH", "/api/collections/parties/records/"+f.pid, f.alice.token, map[string]any{"auto_events": []any{}})
}

func TestDeadlineVoteExtensionThenHost(t *testing.T) {
	f := newDeadlineFixture(t)
	end := f.base.Add(5 * time.Minute)
	f.setDeadline("voting_ends_at", end)
	f.expect(200, "POST", path("/api/occ/parties/%s/transition", f.pid), f.alice.token, map[string]any{"to": "voting"})
	f.flush()

	f.at(5 * time.Minute)
	f.tick()
	if got := strings.Join(f.rec.take(), "|"); got != "alice: Personne n'a voté : +5 minutes" {
		t.Fatalf("extension: %q", got)
	}
	p := f.party()
	if p["status"] != "voting" || !strings.HasPrefix(p["voting_ends_at"].(string), "2026-10-09 09:40:00") {
		t.Fatalf("extended deadline: %v %v", p["status"], p["voting_ends_at"])
	}
	// the new deadline gets its own reminder (everybody: no vote yet)
	f.at(8 * time.Minute)
	f.tick()
	if got := f.rec.take(); len(got) != 3 {
		t.Fatalf("reminder after extension: %v", got)
	}
	f.at(10 * time.Minute)
	f.tick()
	if got := strings.Join(f.rec.take(), "|"); got != "alice: À toi de décider" {
		t.Fatalf("give up: %q", got)
	}
	f.at(11 * time.Minute)
	if n := f.tick(); n != 0 {
		t.Fatal("give up only once")
	}
	if f.party()["status"] != "voting" {
		t.Fatal("still voting: the host decides")
	}
	kinds := []string{}
	for _, e := range f.events() {
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, ",") != "vote_extended,reminder_vote,vote_needs_host" {
		t.Fatalf("events: %v", kinds)
	}
}

func TestDeadlineOrderingAutoClose(t *testing.T) {
	f := newDeadlineFixture(t)
	f.expect(200, "POST", path("/api/occ/parties/%s/transition", f.pid), f.alice.token, map[string]any{"to": "ordering", "restaurant": f.pizza})
	end := f.base.Add(15 * time.Minute)
	f.setDeadline("ordering_ends_at", end)
	tiramisu := f.menuItem(f.pizza, "Tiramisu")
	for _, u := range []user{f.alice, f.bob} {
		f.expect(200, "POST", "/api/collections/order_items/records", u.token, map[string]any{"party": f.pid, "user": u.id(), "menu_item": tiramisu, "quantity": 1})
	}
	f.expect(200, "POST", path("/api/occ/parties/%s/ready", f.pid), f.alice.token, map[string]any{"ready": true})
	f.flush()

	f.at(13*time.Minute + 30*time.Second)
	f.tick()
	if got := strings.Join(f.rec.take(), "|"); got != "bob: Plus que 2 minutes pour commander ⏳|carol: Plus que 2 minutes pour commander ⏳" {
		t.Fatalf("order reminders: %q", got)
	}
	f.at(15 * time.Minute)
	f.tick()
	got := f.rec.take()
	if strings.Join(got, "|") != "alice: Le récap est prêt 🧾|bob: Heure limite atteinte|bob: Le récap est prêt 🧾|carol: Heure limite atteinte|carol: Le récap est prêt 🧾" {
		t.Fatalf("ordering close: %v", got)
	}
	p := f.party()
	if p["status"] != "review" || p["delivery_fee"].(float64) != 299 {
		t.Fatalf("review: %v", p)
	}
	// not ready member kept their items, still not ready
	if n, _ := f.app.CountRecords(colOrderItems, dbx.HashExp{"party": f.pid, "user": f.bob.id()}); n != 1 {
		t.Fatal("bob's cart kept")
	}
	ev := f.events()
	if last := ev[len(ev)-1]; last.Kind != "ordering_closed" || !strings.Contains(last.Text, "à 11:45 (2 personnes n'étaient pas prêtes") {
		t.Fatalf("ordering event: %+v", last)
	}
}

func TestDeadlineDisabledAndEmpty(t *testing.T) {
	f := newDeadlineFixture(t)
	f.expect(200, "POST", path("/api/occ/parties/%s/transition", f.pid), f.alice.token, map[string]any{"to": "ordering", "restaurant": f.pizza})
	f.setDeadline("ordering_ends_at", f.base.Add(time.Minute))
	f.flush()

	// empty carts at the deadline: the host is told once, the party stays open
	f.at(2 * time.Minute)
	f.tick()
	if got := strings.Join(f.rec.take(), "|"); got != "alice: À toi de décider" {
		t.Fatalf("empty: %q", got)
	}
	f.at(3 * time.Minute)
	if n := f.tick(); n != 0 || f.party()["status"] != "ordering" {
		t.Fatal("empty carts: once, still ordering")
	}

	// the host turns auto-close off: an item + a new deadline → nothing happens
	f.expect(200, "PATCH", "/api/collections/parties/records/"+f.pid, f.alice.token, map[string]any{"auto_close_disabled": true})
	f.expect(200, "POST", "/api/collections/order_items/records", f.bob.token, map[string]any{"party": f.pid, "user": f.bob.id(), "menu_item": f.menuItem(f.pizza, "Tiramisu"), "quantity": 1})
	f.setDeadline("ordering_ends_at", f.base.Add(5*time.Minute))
	f.flush()
	f.at(6 * time.Minute)
	f.tick()
	if f.party()["status"] != "ordering" {
		t.Fatal("auto close disabled")
	}
	if got := f.rec.take(); len(got) != 0 {
		t.Fatalf("no reminder after the deadline: %v", got)
	}

	// a member cannot change the host's switch
	f.expect(404, "PATCH", "/api/collections/parties/records/"+f.pid, f.bob.token, map[string]any{"auto_close_disabled": false})
}

// TestDeadlineRestartSafe: a fresh scheduler (restart) reads the flags from
// the database and never repeats a reminder.
func TestDeadlineRestartSafe(t *testing.T) {
	f := newDeadlineFixture(t)
	f.setDeadline("voting_ends_at", f.base.Add(10*time.Minute))
	f.expect(200, "POST", path("/api/occ/parties/%s/transition", f.pid), f.alice.token, map[string]any{"to": "voting"})
	f.flush()
	f.at(8*time.Minute + 30*time.Second)
	f.tick()
	if got := f.rec.take(); len(got) != 3 {
		t.Fatalf("reminders: %v", got)
	}
	restarted := newDeadlineScheduler(f.app, f.h.push, f.clock.now)
	f.at(9 * time.Minute)
	if n := restarted.tick(); n != 0 {
		t.Fatal("restart must not repeat the reminder")
	}
	p, _ := f.app.FindRecordById(colParties, f.pid)
	var flags domain.DeadlineFlags
	_ = json.Unmarshal([]byte(p.GetString("auto_state")), &flags)
	if flags.VoteRemindedFor != "2026-10-09T09:40:00Z" {
		t.Fatalf("flags: %+v", flags)
	}
}

// TestClientKeyDedupe: the offline outbox replays an action with the same
// client_key → one row, same response.
func TestClientKeyDedupe(t *testing.T) {
	e := newEnv(t)
	pizza, burger := e.testRestaurants()
	alice, bob := e.user("Alice"), e.user("Bob")
	party := e.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{"title": "Midi", "candidates": []string{pizza, burger}}).m(t)
	pid := party["id"].(string)
	e.expect(200, "POST", "/api/occ/parties/join", bob.token, map[string]any{"code": party["code"]})

	// votes: replay by key, and the same restaurant twice → the existing vote
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "voting"})
	vote := map[string]any{"party": pid, "user": bob.id(), "restaurant": pizza, "client_key": "v-1"}
	v1 := e.expect(200, "POST", "/api/collections/votes/records", bob.token, vote).m(t)
	v2 := e.expect(200, "POST", "/api/collections/votes/records", bob.token, vote).m(t)
	vote["client_key"] = "v-2"
	v3 := e.expect(200, "POST", "/api/collections/votes/records", bob.token, vote).m(t)
	if v1["id"] != v2["id"] || v1["id"] != v3["id"] {
		t.Fatalf("vote replay: %v %v %v", v1["id"], v2["id"], v3["id"])
	}
	if n, _ := e.app.CountRecords(colVotes, dbx.HashExp{"party": pid}); n != 1 {
		t.Fatalf("votes: %d", n)
	}
	// someone else's key does not leak their vote
	e.expect(200, "POST", "/api/collections/votes/records", alice.token, map[string]any{"party": pid, "user": alice.id(), "restaurant": pizza, "client_key": "v-1"})
	if n, _ := e.app.CountRecords(colVotes, dbx.HashExp{"party": pid}); n != 2 {
		t.Fatalf("votes: %d", n)
	}
	e.expect(400, "POST", "/api/collections/votes/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "restaurant": burger, "client_key": "bad key!"})

	// order items
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering"})
	item := map[string]any{"party": pid, "user": bob.id(), "menu_item": e.menuItem(pizza, "Tiramisu"), "quantity": 2, "client_key": "oi-123"}
	i1 := e.expect(200, "POST", "/api/collections/order_items/records", bob.token, item).m(t)
	item["quantity"] = 5 // a replay never changes the stored line
	i2 := e.expect(200, "POST", "/api/collections/order_items/records", bob.token, item).m(t)
	if i1["id"] != i2["id"] || i2["quantity"].(float64) != 2 || i2["total"].(float64) != 1200 {
		t.Fatalf("item replay: %v / %v", i1, i2)
	}
	if n, _ := e.app.CountRecords(colOrderItems, dbx.HashExp{"party": pid}); n != 1 {
		t.Fatalf("items: %d", n)
	}
	// without key: two lines
	delete(item, "client_key")
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, item)
	if n, _ := e.app.CountRecords(colOrderItems, dbx.HashExp{"party": pid}); n != 2 {
		t.Fatalf("items: %d", n)
	}
	// the key cannot be changed afterwards
	e.expect(400, "PATCH", "/api/collections/order_items/records/"+i1["id"].(string), bob.token, map[string]any{"client_key": "other"})
}
