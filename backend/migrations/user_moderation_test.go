package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestUserModerationMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range UserModerationFields {
		f := users.Fields.GetByName(name)
		if f == nil || !f.GetHidden() {
			t.Fatalf("field %s missing or not hidden: %#v", name, f)
		}
	}
	if users.DeleteRule != nil {
		t.Fatalf("users delete rule must be superuser only, got %q", *users.DeleteRule)
	}

	// backfill: a password account vs an OAuth2-linked one
	mk := func(email string) *core.Record {
		r := core.NewRecord(users)
		r.SetEmail(email)
		r.SetPassword("password123")
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	plain, linked := mk("plain@example.com"), mk("linked@example.com")
	ea := core.NewExternalAuth(app)
	ea.SetCollectionRef(users.Id)
	ea.SetRecordRef(linked.Id)
	ea.SetProvider("google")
	ea.SetProviderId("g-1")
	if err := app.Save(ea); err != nil {
		t.Fatal(err)
	}
	if err := upUserModeration(app); err != nil { // idempotent re-run
		t.Fatal(err)
	}
	for _, c := range []struct {
		r    *core.Record
		want bool
	}{{plain, true}, {linked, false}} {
		got, err := app.FindRecordById("users", c.r.Id)
		if err != nil {
			t.Fatal(err)
		}
		if got.GetBool("password_set") != c.want {
			t.Errorf("%s password_set=%v, want %v", got.Email(), got.GetBool("password_set"), c.want)
		}
	}
}
