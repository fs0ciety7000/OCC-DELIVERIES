package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestTeamsGuestsMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	f := users.Fields.GetByName("is_guest")
	if f == nil || f.Type() != core.FieldTypeBool || f.GetHidden() {
		t.Fatalf("users.is_guest missing, not a bool or hidden: %#v", f)
	}

	teams, err := app.FindCollectionByNameOrId("teams")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"name", "code", "owner", "admins", "members", "address", "lat", "lng", "usual_time",
		"usual_days", "default_candidates", "default_split", "emoji", "color", "archived", "last_party", "last_launch_at"} {
		if teams.Fields.GetByName(name) == nil {
			t.Errorf("teams.%s missing", name)
		}
	}
	rules := []struct {
		name string
		got  *string
		want string
	}{
		{"list", teams.ListRule, TeamsMembersRule},
		{"view", teams.ViewRule, TeamsMembersRule},
		{"create", teams.CreateRule, TeamsCreateRule},
		{"update", teams.UpdateRule, TeamsUpdateRule},
	}
	for _, r := range rules {
		if r.got == nil || *r.got != r.want {
			t.Errorf("teams %s rule = %v, want %q", r.name, r.got, r.want)
		}
	}
	if teams.DeleteRule != nil {
		t.Errorf("teams delete rule must be nil, got %q", *teams.DeleteRule)
	}

	parties, err := app.FindCollectionByNameOrId("parties")
	if err != nil {
		t.Fatal(err)
	}
	rel, ok := parties.Fields.GetByName("team").(*core.RelationField)
	if !ok || rel.CollectionId != teams.Id || rel.MaxSelect != 1 {
		t.Fatalf("parties.team: %#v", parties.Fields.GetByName("team"))
	}
	if lp, ok := teams.Fields.GetByName("last_party").(*core.RelationField); !ok || lp.CollectionId != parties.Id {
		t.Fatalf("teams.last_party: %#v", teams.Fields.GetByName("last_party"))
	}
	if parties.CreateRule == nil || *parties.CreateRule != `@request.auth.id != "" && @request.auth.is_guest = false` {
		t.Errorf("parties create rule = %v", parties.CreateRule)
	}
	pp, err := app.FindCollectionByNameOrId("payout_profiles")
	if err != nil {
		t.Fatal(err)
	}
	if pp.UpdateRule == nil || *pp.UpdateRule != `user = @request.auth.id && @request.auth.is_guest = false` {
		t.Errorf("payout_profiles update rule = %v", pp.UpdateRule)
	}

	// idempotent re-run, then down + up
	if err := upTeamsGuests(app); err != nil {
		t.Fatal(err)
	}
	if err := downTeamsGuests(app); err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindCollectionByNameOrId("teams"); err == nil {
		t.Fatal("teams still present after down")
	}
	if err := upTeamsGuests(app); err != nil {
		t.Fatal(err)
	}
}
