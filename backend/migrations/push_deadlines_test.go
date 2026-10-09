package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestPushDeadlinesMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)

	ps, err := app.FindCollectionByNameOrId(PushSubscriptionsCollection)
	if err != nil {
		t.Fatal(err)
	}
	owner := `user = @request.auth.id`
	if ps.ListRule == nil || *ps.ListRule != owner || ps.ViewRule == nil || *ps.ViewRule != owner ||
		ps.DeleteRule == nil || *ps.DeleteRule != owner || ps.CreateRule != nil || ps.UpdateRule != nil {
		t.Fatalf("push_subscriptions rules: %v %v %v %v %v", ps.ListRule, ps.ViewRule, ps.CreateRule, ps.UpdateRule, ps.DeleteRule)
	}
	for _, f := range []string{"user", "endpoint", "p256dh", "auth", "user_agent", "last_ok", "failures", "created"} {
		if ps.Fields.GetByName(f) == nil {
			t.Fatalf("push_subscriptions.%s missing", f)
		}
	}
	ss, err := app.FindCollectionByNameOrId(ServerSecretsCollection)
	if err != nil {
		t.Fatal(err)
	}
	if ss.ListRule != nil || ss.ViewRule != nil || ss.CreateRule != nil || ss.UpdateRule != nil || ss.DeleteRule != nil {
		t.Fatal("server_secrets must be superuser only")
	}
	if f := ss.Fields.GetByName("value"); f == nil || !f.GetHidden() {
		t.Fatal("server_secrets.value must be hidden")
	}
	users, _ := app.FindCollectionByNameOrId("users")
	if f := users.Fields.GetByName("notify_prefs"); f == nil || !f.GetHidden() {
		t.Fatal("users.notify_prefs must be hidden")
	}
	parties, _ := app.FindCollectionByNameOrId("parties")
	for _, f := range []string{"auto_close_disabled", "auto_events", "auto_state"} {
		if parties.Fields.GetByName(f) == nil {
			t.Fatalf("parties.%s missing", f)
		}
	}
	if !parties.Fields.GetByName("auto_state").GetHidden() {
		t.Fatal("auto_state must be hidden")
	}

	// unique endpoint, unique (user, client_key) when set
	u := core.NewRecord(users)
	u.SetEmail("a@example.com")
	u.SetPassword("password123")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	mk := func() error {
		r := core.NewRecord(ps)
		r.Load(map[string]any{"user": u.Id, "endpoint": "https://push.test/1", "p256dh": "k", "auth": "a"})
		return app.Save(r)
	}
	if err := mk(); err != nil {
		t.Fatal(err)
	}
	if err := mk(); err == nil {
		t.Fatal("endpoint must be unique")
	}
	for _, name := range []string{"order_items", "votes"} {
		col, _ := app.FindCollectionByNameOrId(name)
		if col.Fields.GetByName("client_key") == nil {
			t.Fatalf("%s.client_key missing", name)
		}
		if idx := col.GetIndex("idx_" + name + "_client_key"); idx == "" {
			t.Fatalf("%s client_key index missing", name)
		}
	}

	// existing open parties keep the indicative deadlines; idempotent re-run
	p := core.NewRecord(parties)
	p.Load(map[string]any{"code": "ABCDEF", "host": u.Id, "members": []string{u.Id}, "status": "ordering"})
	if err := app.Save(p); err != nil {
		t.Fatal(err)
	}
	parties.Fields.RemoveByName("auto_close_disabled")
	parties.Fields.RemoveByName("auto_events")
	parties.Fields.RemoveByName("auto_state")
	if err := app.Save(parties); err != nil {
		t.Fatal(err)
	}
	if err := upPushDeadlines(app); err != nil {
		t.Fatal(err)
	}
	if err := upPushDeadlines(app); err != nil {
		t.Fatal(err)
	}
	got, err := app.FindRecordById("parties", p.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.GetBool("auto_close_disabled") {
		t.Fatal("existing open party must keep indicative deadlines")
	}
}
