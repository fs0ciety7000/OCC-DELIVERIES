package app

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

type teamResp struct {
	Team struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		Code              string `json:"code"`
		MyRole            string `json:"myRole"`
		MemberCount       int    `json:"memberCount"`
		UsualTime         string `json:"usualTime"`
		DefaultCandidates []struct {
			ID string `json:"id"`
		} `json:"defaultCandidates"`
		Members []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Role    string `json:"role"`
			IsGuest bool   `json:"isGuest"`
		} `json:"members"`
		ActiveParty *struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			IsMember bool   `json:"isMember"`
		} `json:"activeParty"`
	} `json:"team"`
	AlreadyMember bool `json:"alreadyMember"`
}

// newTeam creates a team owned by owner (collection API) and returns id, code.
func (e *env) newTeam(owner user, fields map[string]any) (string, string) {
	e.t.Helper()
	body := map[string]any{"name": "OCC Mons — midi", "address": "Rue de Nimy 7, 7000 Mons", "usual_time": "12h15", "usual_days": []string{"fri", "mon"}}
	for k, v := range fields {
		body[k] = v
	}
	r := e.expect(200, "POST", "/api/collections/teams/records", owner.token, body).m(e.t)
	return r["id"].(string), r["code"].(string)
}

func (e *env) team(id, token string) teamResp {
	e.t.Helper()
	var out teamResp
	e.expect(200, "GET", "/api/occ/teams/"+id, token, nil).json(e.t, &out)
	return out
}

func TestTeamRulesAndMembership(t *testing.T) {
	e := newEnv(t)
	pizza, _ := e.testRestaurants()
	alice, bob, carol, mallory := e.user("Alice"), e.user("Bob"), e.user("Carol"), e.user("Mallory")

	tid, code := e.newTeam(alice, map[string]any{
		"default_candidates": []string{pizza}, "color": "#e4572e",
		// server managed: ignored / overwritten
		"members": []string{mallory.id()}, "owner": mallory.id(), "code": "AAAAAAAA",
	})
	rec, err := e.app.FindRecordById(colTeams, tid)
	if err != nil {
		t.Fatal(err)
	}
	if rec.GetString("owner") != alice.id() || len(rec.GetStringSlice("members")) != 1 || code == "AAAAAAAA" || len(code) != 8 {
		t.Fatalf("server fields not forced: owner=%s members=%v code=%s", rec.GetString("owner"), rec.GetStringSlice("members"), code)
	}
	if rec.GetString("usual_time") != "12:15" || strings.Join(rec.GetStringSlice("usual_days"), ",") != "mon,fri" || rec.GetString("color") != "#E4572E" {
		t.Fatalf("fields not normalised: %v %v %v", rec.GetString("usual_time"), rec.GetStringSlice("usual_days"), rec.GetString("color"))
	}
	e.expect(400, "POST", "/api/collections/teams/records", alice.token, map[string]any{"name": "x"})
	e.expect(400, "POST", "/api/collections/teams/records", alice.token, map[string]any{"name": "Équipe", "usual_time": "midi"})
	e.expect(400, "POST", "/api/collections/teams/records", "", map[string]any{"name": "Équipe"})

	// non-member: no read (members.id ?= rule), no detail, no update
	e.expect(404, "GET", "/api/collections/teams/records/"+tid, carol.token, nil)
	if r := e.expect(200, "GET", "/api/collections/teams/records", carol.token, nil); !strings.Contains(string(r.body), `"totalItems":0`) {
		t.Fatalf("carol lists teams: %s", r.body)
	}
	e.expect(403, "GET", "/api/occ/teams/"+tid, carol.token, nil)
	e.expect(404, "PATCH", "/api/collections/teams/records/"+tid, carol.token, map[string]any{"name": "Pirate"})
	e.expect(200, "GET", "/api/collections/teams/records/"+tid, alice.token, nil)

	// join by the fixed link (idempotent)
	e.expect(400, "POST", "/api/occ/teams/join", bob.token, map[string]any{"code": "ABC"})
	e.expect(404, "POST", "/api/occ/teams/join", bob.token, map[string]any{"code": "ABCDEFGH"})
	var j teamResp
	e.expect(200, "POST", "/api/occ/teams/join", bob.token, map[string]any{"code": strings.ToLower(code)}).json(t, &j)
	if j.AlreadyMember || j.Team.MyRole != "member" || j.Team.MemberCount != 2 {
		t.Fatalf("join: %+v", j)
	}
	e.expect(200, "POST", "/api/occ/teams/join", bob.token, map[string]any{"code": code}).json(t, &j)
	if !j.AlreadyMember {
		t.Fatal("second join should be idempotent")
	}
	e.expect(200, "POST", "/api/occ/teams/join", carol.token, map[string]any{"code": code})
	e.expect(200, "GET", "/api/collections/teams/records/"+tid, bob.token, nil)

	// a member cannot update; members/code/owner immutable; only the owner picks admins
	e.expect(404, "PATCH", "/api/collections/teams/records/"+tid, bob.token, map[string]any{"name": "Bob team"})
	for field, val := range map[string]any{"members": []string{alice.id()}, "code": "ABCDEFGH", "owner": bob.id(), "last_party": ""} {
		if field == "last_party" {
			continue
		}
		r := e.expect(400, "PATCH", "/api/collections/teams/records/"+tid, alice.token, map[string]any{field: val})
		if !strings.Contains(string(r.body), "ne peut pas être modifié") {
			t.Fatalf("%s: %s", field, r.body)
		}
	}
	e.expect(400, "PATCH", "/api/collections/teams/records/"+tid, alice.token, map[string]any{"admins": []string{mallory.id()}})
	e.expect(200, "PATCH", "/api/collections/teams/records/"+tid, alice.token, map[string]any{"admins": []string{bob.id()}})
	if d := e.team(tid, bob.token); d.Team.MyRole != "admin" || d.Team.Members[0].ID != alice.id() || d.Team.Members[0].Role != "owner" {
		t.Fatalf("bob admin: %+v", d.Team)
	}
	// admin (bob) edits settings but cannot change admins
	e.expect(200, "PATCH", "/api/collections/teams/records/"+tid, bob.token, map[string]any{"name": "OCC Mons — soir"})
	e.expect(403, "PATCH", "/api/collections/teams/records/"+tid, bob.token, map[string]any{"admins": []string{bob.id(), carol.id()}})

	// remove: admin removes a member, not the owner; members cannot
	e.expect(400, "DELETE", path("/api/occ/teams/%s/members/%s", tid, alice.id()), bob.token, nil)
	e.expect(400, "DELETE", path("/api/occ/teams/%s/members/%s", tid, bob.id()), carol.token, nil)
	e.expect(200, "DELETE", path("/api/occ/teams/%s/members/%s", tid, carol.id()), bob.token, nil)
	e.expect(403, "GET", "/api/occ/teams/"+tid, carol.token, nil)

	// leave (not the owner); admin rights dropped with the membership
	e.expect(400, "POST", path("/api/occ/teams/%s/leave", tid), alice.token, nil)
	e.expect(200, "POST", path("/api/occ/teams/%s/leave", tid), bob.token, nil)
	rec, _ = e.app.FindRecordById(colTeams, tid)
	if len(rec.GetStringSlice("admins")) != 0 || len(rec.GetStringSlice("members")) != 1 {
		t.Fatalf("after leave: admins=%v members=%v", rec.GetStringSlice("admins"), rec.GetStringSlice("members"))
	}

	// new invite code: managers only, old link dead
	var nc struct {
		Code string `json:"code"`
	}
	e.expect(200, "POST", path("/api/occ/teams/%s/code", tid), alice.token, nil).json(t, &nc)
	if nc.Code == code || len(nc.Code) != 8 {
		t.Fatalf("new code %q", nc.Code)
	}
	e.expect(404, "POST", "/api/occ/teams/join", bob.token, map[string]any{"code": code})

	// my teams
	var mine struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	e.expect(200, "GET", "/api/occ/me/teams", alice.token, nil).json(t, &mine)
	if len(mine.Items) != 1 || mine.Items[0].ID != tid {
		t.Fatalf("my teams: %+v", mine)
	}
	e.expect(200, "GET", "/api/occ/me/teams", carol.token, nil).json(t, &mine)
	if len(mine.Items) != 0 {
		t.Fatalf("carol teams: %+v", mine)
	}

	// archived: no join, no launch; no hard delete
	e.expect(200, "PATCH", "/api/collections/teams/records/"+tid, alice.token, map[string]any{"archived": true})
	e.expect(400, "POST", "/api/occ/teams/join", carol.token, map[string]any{"code": nc.Code})
	e.expect(400, "POST", path("/api/occ/teams/%s/launch", tid), alice.token, nil)
	e.expect(403, "DELETE", "/api/collections/teams/records/"+tid, alice.token, nil)
}

func TestTeamPartyLaunchAndOneTapJoin(t *testing.T) {
	e := newEnv(t)
	pizza, burger := e.testRestaurants()
	alice, bob, carol, dave := e.user("Alice"), e.user("Bob"), e.user("Carol"), e.user("Dave")
	tid, code := e.newTeam(alice, map[string]any{"default_candidates": []string{pizza, burger}, "default_split": "proportional"})
	for _, u := range []user{bob, carol} {
		e.expect(200, "POST", "/api/occ/teams/join", u.token, map[string]any{"code": code})
	}
	// a deactivated default candidate does not block the launch
	b, _ := e.app.FindRecordById(colRestaurants, burger)
	b.Set("active", false)
	if err := e.app.Save(b); err != nil {
		t.Fatal(err)
	}

	rec := &recordingNotifier{}
	SetTeamNotifier(rec)
	defer SetTeamNotifier(nil)

	e.expect(403, "POST", path("/api/occ/teams/%s/launch", tid), dave.token, nil)
	var l struct {
		Party struct {
			ID              string   `json:"id"`
			Title           string   `json:"title"`
			Team            string   `json:"team"`
			Host            string   `json:"host"`
			DeliveryAddress string   `json:"delivery_address"`
			Candidates      []string `json:"candidates"`
			SplitMode       string   `json:"split_mode"`
			Status          string   `json:"status"`
			Code            string   `json:"code"`
		} `json:"party"`
		Created bool `json:"created"`
	}
	e.expect(200, "POST", path("/api/occ/teams/%s/launch", tid), bob.token, nil).json(t, &l)
	p := l.Party
	if !l.Created || p.Team != tid || p.Host != bob.id() || p.DeliveryAddress != "Rue de Nimy 7, 7000 Mons" ||
		len(p.Candidates) != 1 || p.Candidates[0] != pizza || p.SplitMode != "proportional" || p.Status != "lobby" ||
		!(strings.HasPrefix(p.Title, "Midi du ") || strings.HasPrefix(p.Title, "Soirée du ")) {
		t.Fatalf("launched party: %+v", l)
	}
	if len(rec.calls) != 1 || rec.calls[0].party != p.ID || strings.Join(rec.calls[0].recipients, ",") != alice.id()+","+carol.id() &&
		strings.Join(rec.calls[0].recipients, ",") != carol.id()+","+alice.id() {
		t.Fatalf("notifier: %+v", rec.calls)
	}
	team, _ := e.app.FindRecordById(colTeams, tid)
	if team.GetString("last_party") != p.ID || team.GetDateTime("last_launch_at").IsZero() {
		t.Fatal("team launch not recorded (realtime toast)")
	}
	pm, err := e.app.FindFirstRecordByFilter(colPartyMembers, "party = {:p} && user = {:u}", map[string]any{"p": p.ID, "u": bob.id()})
	if err != nil || pm.GetString("role") != "host" {
		t.Fatalf("host party member: %v", err)
	}

	// launching again returns the active party (and joins the caller)
	e.expect(200, "POST", path("/api/occ/teams/%s/launch", tid), alice.token, nil).json(t, &l)
	if l.Created || l.Party.ID != p.ID {
		t.Fatalf("relaunch: %+v", l)
	}

	// lobby: carol (team member) not there yet
	var pt struct {
		Team *struct {
			ID string `json:"id"`
		} `json:"team"`
		Missing []struct {
			ID string `json:"id"`
		} `json:"missing"`
	}
	e.expect(200, "GET", path("/api/occ/parties/%s/team", p.ID), bob.token, nil).json(t, &pt)
	if pt.Team == nil || pt.Team.ID != tid || len(pt.Missing) != 1 || pt.Missing[0].ID != carol.id() {
		t.Fatalf("party team: %+v", pt)
	}
	e.expect(403, "GET", path("/api/occ/parties/%s/team", p.ID), carol.token, nil)

	// team page: active party highlighted, carol not member yet
	d := e.team(tid, carol.token)
	if d.Team.ActiveParty == nil || d.Team.ActiveParty.ID != p.ID || d.Team.ActiveParty.IsMember {
		t.Fatalf("active party: %+v", d.Team.ActiveParty)
	}

	// one-tap join: team members only, no code
	e.expect(403, "POST", path("/api/occ/parties/%s/join", p.ID), dave.token, nil)
	var jr struct {
		AlreadyMember bool `json:"alreadyMember"`
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/join", p.ID), carol.token, nil).json(t, &jr)
	if jr.AlreadyMember {
		t.Fatal("carol was not a member")
	}
	e.expect(200, "GET", "/api/collections/parties/records/"+p.ID, carol.token, nil)
	e.expect(200, "POST", path("/api/occ/parties/%s/join", p.ID), carol.token, nil).json(t, &jr)
	if !jr.AlreadyMember {
		t.Fatal("second one-tap join should be idempotent")
	}
	e.expect(200, "GET", path("/api/occ/parties/%s/team", p.ID), bob.token, nil).json(t, &pt)
	if len(pt.Missing) != 0 {
		t.Fatalf("missing after joins: %+v", pt.Missing)
	}
	// a party without team: no one-tap join
	solo := e.expect(200, "POST", "/api/collections/parties/records", dave.token, map[string]any{"title": "Solo"}).m(t)
	e.expect(403, "POST", path("/api/occ/parties/%s/join", solo["id"]), carol.token, nil)
	// team is immutable on the party
	e.expect(400, "PATCH", "/api/collections/parties/records/"+p.ID, bob.token, map[string]any{"team": ""})

	// party created from the sheet « Pour l'équipe … »: defaults, members only
	e.expect(403, "POST", "/api/collections/parties/records", dave.token, map[string]any{"title": "Intrus", "team": tid})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", p.ID), bob.token, map[string]any{"to": "cancelled"})
	sheet := e.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{
		"title": "Soirée pizza", "team": tid, "delivery_address": "Salle B",
	}).m(t)
	if sheet["team"] != tid || sheet["delivery_address"] != "Salle B" || len(sheet["candidates"].([]any)) != 1 || sheet["split_mode"] != "proportional" {
		t.Fatalf("sheet party: %v", sheet)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("sheet launch not notified: %d", len(rec.calls))
	}

	// history of the team: both parties, newest first
	var hist struct {
		TotalItems int `json:"totalItems"`
		Items      []struct {
			Party struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"party"`
			IsMember bool `json:"isMember"`
		} `json:"items"`
	}
	e.expect(200, "GET", path("/api/occ/teams/%s/parties", tid), carol.token, nil).json(t, &hist)
	if hist.TotalItems != 2 || hist.Items[0].Party.ID != sheet["id"] || hist.Items[0].IsMember || !hist.Items[1].IsMember {
		t.Fatalf("team history: %+v", hist)
	}
	e.expect(403, "GET", path("/api/occ/teams/%s/parties", tid), dave.token, nil)
}

type notifierCall struct {
	team, party string
	recipients  []string
}

type recordingNotifier struct{ calls []notifierCall }

func (r *recordingNotifier) TeamPartyLaunched(_ core.App, team, party *core.Record, recipients []string) error {
	r.calls = append(r.calls, notifierCall{team: team.Id, party: party.Id, recipients: recipients})
	return nil
}
