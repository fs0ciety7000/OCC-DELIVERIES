package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestCollectModeMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)

	parties, err := app.FindCollectionByNameOrId("parties")
	if err != nil {
		t.Fatal(err)
	}
	f, ok := parties.Fields.GetByName("collect_mode").(*core.SelectField)
	if !ok || len(f.Values) != 2 || f.Required {
		t.Fatalf("collect_mode field: %#v", parties.Fields.GetByName("collect_mode"))
	}

	// a party already paying before the migration keeps the transfer behaviour
	if err := downCollectMode(app); err != nil {
		t.Fatal(err)
	}
	users, _ := app.FindCollectionByNameOrId("users")
	u := core.NewRecord(users)
	u.SetEmail("c@example.com")
	u.SetPassword("password123")
	u.Set("name", "C")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	parties, _ = app.FindCollectionByNameOrId("parties")
	mk := func(code, status, payer string) *core.Record {
		r := core.NewRecord(parties)
		r.Load(map[string]any{"code": code, "host": u.Id, "members": []string{u.Id}, "status": status, "payer": payer, "split_mode": "equal"})
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	paying := mk("PAYAAA", "paying", u.Id)
	lobby := mk("LBBAAA", "lobby", "")
	for run := 0; run < 2; run++ { // idempotent
		if err := upCollectMode(app); err != nil {
			t.Fatal(err)
		}
	}
	for r, want := range map[*core.Record]string{paying: "transfer", lobby: ""} {
		got, err := app.FindRecordById("parties", r.Id)
		if err != nil {
			t.Fatal(err)
		}
		if got.GetString("collect_mode") != want {
			t.Fatalf("%s: collect_mode = %q, want %q", r.GetString("code"), got.GetString("collect_mode"), want)
		}
	}
}
