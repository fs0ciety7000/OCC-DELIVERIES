package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Permanent team rooms and guests without account:
//   - users.is_guest: account created from an invite link with a first name
//     only (POST /api/occ/guest). Readable (badges « Invité », guest UI) but
//     never writable by a client (hook); guests cannot create parties, teams
//     or a payout profile (rules below + hooks);
//   - teams: « Salon d'équipe » with a permanent invite link /e/:code;
//   - parties.team: the team a party was launched for (immutable).

func init() {
	m.Register(upTeamsGuests, downTeamsGuests)
}

// Rules written by this migration (exported for the tests).
const (
	TeamsMembersRule = `members.id ?= @request.auth.id || @request.auth.role = "admin"`
	TeamsCreateRule  = `@request.auth.id != "" && @request.auth.is_guest = false`
	TeamsUpdateRule  = `owner = @request.auth.id || admins.id ?= @request.auth.id`
	notGuest         = ` && @request.auth.is_guest = false`
)

func upTeamsGuests(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	if users.Fields.GetByName("is_guest") == nil {
		users.Fields.Add(&core.BoolField{Name: "is_guest"})
		if err := app.Save(users); err != nil {
			return err
		}
	}
	restaurants, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		return err
	}
	parties, err := app.FindCollectionByNameOrId("parties")
	if err != nil {
		return err
	}

	teams, _ := app.FindCollectionByNameOrId("teams")
	if teams == nil {
		teams = core.NewBaseCollection("teams")
		teams.Fields.Add(
			&core.TextField{Name: "name", Required: true, Max: 80},
			&core.TextField{Name: "code", Required: true, Min: 8, Max: 8, Pattern: `^[A-HJ-NP-Z2-9]{8}$`},
			&core.RelationField{Name: "owner", CollectionId: users.Id, MaxSelect: 1, Required: true},
			&core.RelationField{Name: "admins", CollectionId: users.Id, MaxSelect: 999},
			&core.RelationField{Name: "members", CollectionId: users.Id, MaxSelect: 999},
			&core.TextField{Name: "address", Max: 300},
			&core.NumberField{Name: "lat", Min: fptr(-90), Max: fptr(90)},
			&core.NumberField{Name: "lng", Min: fptr(-180), Max: fptr(180)},
			&core.TextField{Name: "usual_time", Max: 5, Pattern: `^(([01][0-9]|2[0-3]):[0-5][0-9])?$`},
			&core.SelectField{Name: "usual_days", MaxSelect: 7, Values: []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}},
			&core.RelationField{Name: "default_candidates", CollectionId: restaurants.Id, MaxSelect: 20},
			&core.SelectField{Name: "default_split", MaxSelect: 1, Values: []string{"equal", "proportional"}},
			&core.TextField{Name: "emoji", Max: 16},
			&core.TextField{Name: "color", Max: 7, Pattern: `^(#[0-9A-Fa-f]{6})?$`},
			&core.BoolField{Name: "archived"},
			// server: last party launched for the team (realtime notification)
			&core.RelationField{Name: "last_party", CollectionId: parties.Id, MaxSelect: 1},
			&core.DateField{Name: "last_launch_at"},
		)
		autodates(teams)
		teams.AddIndex("idx_teams_code", true, "code", "")
		if err := app.Save(teams); err != nil {
			return err
		}
	}
	teams.ListRule = ptr(TeamsMembersRule)
	teams.ViewRule = ptr(TeamsMembersRule)
	teams.CreateRule = ptr(TeamsCreateRule)
	teams.UpdateRule = ptr(TeamsUpdateRule)
	teams.DeleteRule = nil // archive instead (keeps the parties history)

	if err := app.Save(teams); err != nil {
		return err
	}
	if parties.Fields.GetByName("team") == nil {
		parties.Fields.Add(&core.RelationField{Name: "team", CollectionId: teams.Id, MaxSelect: 1})
		parties.AddIndex("idx_parties_team", false, "team", "")
	}
	parties.CreateRule = ptr(`@request.auth.id != ""` + notGuest)
	if err := app.Save(parties); err != nil {
		return err
	}

	pp, err := app.FindCollectionByNameOrId("payout_profiles")
	if err != nil {
		return err
	}
	pp.CreateRule = ptr(`@request.auth.id != "" && user = @request.auth.id` + notGuest)
	pp.UpdateRule = ptr(`user = @request.auth.id` + notGuest)
	return app.Save(pp)
}

func downTeamsGuests(app core.App) error {
	if pp, err := app.FindCollectionByNameOrId("payout_profiles"); err == nil {
		pp.CreateRule = ptr(`@request.auth.id != "" && user = @request.auth.id`)
		pp.UpdateRule = ptr(`user = @request.auth.id`)
		if err := app.Save(pp); err != nil {
			return err
		}
	}
	if parties, err := app.FindCollectionByNameOrId("parties"); err == nil {
		parties.Fields.RemoveByName("team")
		parties.RemoveIndex("idx_parties_team")
		parties.CreateRule = ptr(`@request.auth.id != ""`)
		if err := app.Save(parties); err != nil {
			return err
		}
	}
	if teams, err := app.FindCollectionByNameOrId("teams"); err == nil {
		if err := app.Delete(teams); err != nil {
			return err
		}
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	users.Fields.RemoveByName("is_guest")
	return app.Save(users)
}
