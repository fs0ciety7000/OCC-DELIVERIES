package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/auth"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

type guestResp struct {
	Token  string `json:"token"`
	Record struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Color   string `json:"color"`
		IsGuest bool   `json:"is_guest"`
	} `json:"record"`
	Party *struct {
		ID string `json:"id"`
	} `json:"party"`
	Team *struct {
		ID string `json:"id"`
	} `json:"team"`
}

// doIP performs a JSON request from a given client IP.
func (e *env) doIP(ip, method, url, token, body string) resp {
	e.t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":4242"
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return resp{status: rec.Code, body: rec.Body.Bytes(), header: rec.Header()}
}

// guest creates a guest through the endpoint (party or team code).
func (e *env) guest(name string, body map[string]any) (user, guestResp) {
	e.t.Helper()
	b := map[string]any{"name": name}
	for k, v := range body {
		b[k] = v
	}
	var g guestResp
	e.expect(200, "POST", "/api/occ/guest", "", b).json(e.t, &g)
	return user{rec: e.reload(g.Record.ID), token: g.Token}, g
}

func TestGuestJoinAndGuards(t *testing.T) {
	e := newEnv(t)
	pid, code, pizza, _, alice, bob, _ := setupParty(t, e)

	// a valid code is required
	e.expect(400, "POST", "/api/occ/guest", "", map[string]any{"name": "Léa"})
	e.expect(400, "POST", "/api/occ/guest", "", map[string]any{"name": "Léa", "partyCode": "ABC"})
	e.expect(404, "POST", "/api/occ/guest", "", map[string]any{"name": "Léa", "partyCode": "ZZZZZZ"})
	e.expect(400, "POST", "/api/occ/guest", "", map[string]any{"name": "  ", "partyCode": code})
	e.expect(400, "POST", "/api/occ/guest", "", map[string]any{"name": "Léa", "partyCode": code, "teamCode": "ABCDEFGH"})
	e.expect(400, "POST", "/api/occ/guest", bob.token, map[string]any{"name": "Léa", "partyCode": code})

	lea, g := e.guest(" Léa ", map[string]any{"partyCode": strings.ToLower(code), "color": "#2a9d8f"})
	if !g.Record.IsGuest || g.Record.Name != "Léa" || g.Record.Color != "#2A9D8F" || g.Party == nil || g.Party.ID != pid {
		t.Fatalf("guest response: %+v", g)
	}
	r := lea.rec
	if !domain.IsPlaceholderEmail(r.Email()) || r.Email() != domain.GuestEmail(r.Id) || r.Verified() || r.GetBool(fieldPasswordSet) || r.GetString("role") != roleUser {
		t.Fatalf("guest record: email=%s verified=%v role=%s", r.Email(), r.Verified(), r.GetString("role"))
	}

	// the token works: member of the party, can see it and its summary
	e.expect(200, "GET", "/api/collections/parties/records/"+pid, lea.token, nil)
	e.expect(200, "GET", path("/api/occ/parties/%s/summary", pid), lea.token, nil)
	// refresh: a new (static) guest token
	var ref struct {
		Token  string `json:"token"`
		Record struct {
			ID string `json:"id"`
		} `json:"record"`
	}
	e.expect(200, "POST", "/api/collections/users/auth-refresh", lea.token, nil).json(t, &ref)
	if ref.Token == "" || ref.Record.ID != r.Id {
		t.Fatalf("refresh: %+v", ref)
	}
	e.expect(200, "GET", "/api/collections/parties/records/"+pid, ref.Token, nil)

	// guests can vote / order / mark ready
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "voting"})
	e.expect(200, "PUT", path("/api/occ/parties/%s/ballot", pid), lea.token, map[string]any{"ranking": []string{pizza}})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	e.expect(200, "POST", "/api/collections/order_items/records", lea.token, map[string]any{"party": pid, "user": r.Id, "menu_item": e.menuItem(pizza, "Tiramisu")})
	e.expect(200, "POST", path("/api/occ/parties/%s/ready", pid), lea.token, map[string]any{"ready": true})

	// ... but cannot: create a party or a team, save a payout profile, use /admin
	e.expect(400, "POST", "/api/collections/parties/records", lea.token, map[string]any{"title": "Invitée"})
	e.expect(400, "POST", "/api/collections/teams/records", lea.token, map[string]any{"name": "Équipe Léa"})
	e.expect(400, "POST", "/api/collections/payout_profiles/records", lea.token, map[string]any{"user": r.Id, "iban": "BE71096123456769"})
	e.expect(403, "GET", "/api/occ/admin/stats", lea.token, nil)
	e.expect(403, "GET", "/api/occ/admin/users", lea.token, nil)
	// nor flag / unflag herself, nor take a role
	e.expect(403, "PATCH", "/api/collections/users/records/"+r.Id, lea.token, map[string]any{"is_guest": false})
	e.expect(403, "PATCH", "/api/collections/users/records/"+r.Id, lea.token, map[string]any{"role": "admin"})
	e.expect(200, "PATCH", "/api/collections/users/records/"+r.Id, lea.token, map[string]any{"name": "Léa D."})
	// a regular sign-up can never be a guest
	reg := e.expect(200, "POST", "/api/collections/users/records", "", map[string]any{
		"email": "eve@example.com", "password": "password123", "passwordConfirm": "password123", "name": "Eve", "is_guest": true,
	}).m(t)
	if reg["is_guest"] != false {
		t.Fatalf("sign-up flagged guest: %v", reg)
	}
	// an admin cannot promote a guest
	admin := e.admin("Root")
	e.expect(400, "PATCH", path("/api/occ/admin/users/%s/role", r.Id), admin.token, map[string]any{"role": "admin"})
	// admin list: badge + filter
	var list struct {
		TotalItems int `json:"totalItems"`
		Items      []struct {
			ID      string `json:"id"`
			IsGuest bool   `json:"isGuest"`
		} `json:"items"`
	}
	e.expect(200, "GET", "/api/occ/admin/users?status=guest", admin.token, nil).json(t, &list)
	if list.TotalItems != 1 || list.Items[0].ID != r.Id || !list.Items[0].IsGuest {
		t.Fatalf("guest filter: %+v", list)
	}

	// the guest appears in the summary
	var s struct {
		Participants []struct {
			User struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"user"`
			Total int `json:"total"`
		} `json:"participants"`
	}
	e.expect(200, "GET", path("/api/occ/parties/%s/summary", pid), alice.token, nil).json(t, &s)
	found := false
	for _, p := range s.Participants {
		if p.User.ID == r.Id && p.User.Name == "Léa D." && p.Total > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("guest missing from the summary: %+v", s)
	}

	// closed party: no more guests
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "cancelled"})
	e.expect(400, "POST", "/api/occ/guest", "", map[string]any{"name": "Tom", "partyCode": code})
}

func TestGuestTeamJoinAndInvitePreview(t *testing.T) {
	e := newEnv(t)
	alice := e.user("Alice")
	tid, code := e.newTeam(alice, nil)

	var pv struct {
		Kind     string `json:"kind"`
		Title    string `json:"title"`
		Joinable bool   `json:"joinable"`
	}
	e.expect(200, "GET", "/api/occ/invites/"+code, "", nil).json(t, &pv)
	if pv.Kind != "team" || pv.Title != "OCC Mons — midi" || !pv.Joinable {
		t.Fatalf("team preview: %+v", pv)
	}
	e.expect(404, "GET", "/api/occ/invites/ABCDEFGH", "", nil)
	e.expect(400, "GET", "/api/occ/invites/nope", "", nil)

	tom, g := e.guest("Tom", map[string]any{"teamCode": code})
	if g.Team == nil || g.Team.ID != tid {
		t.Fatalf("team guest: %+v", g)
	}
	d := e.team(tid, tom.token)
	if d.Team.MemberCount != 2 || !d.Team.Members[1].IsGuest {
		t.Fatalf("team members: %+v", d.Team.Members)
	}
	// a guest member cannot launch the party of the day nor become team admin
	e.expect(403, "POST", path("/api/occ/teams/%s/launch", tid), tom.token, nil)
	e.expect(400, "PATCH", "/api/collections/teams/records/"+tid, alice.token, map[string]any{"admins": []string{tom.id()}})
	// ... but joins the team party in one tap
	var l struct {
		Party struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		} `json:"party"`
	}
	e.expect(200, "POST", path("/api/occ/teams/%s/launch", tid), alice.token, nil).json(t, &l)
	e.expect(200, "POST", path("/api/occ/parties/%s/join", l.Party.ID), tom.token, nil)

	e.expect(200, "GET", "/api/occ/invites/"+l.Party.Code, "", nil).json(t, &pv)
	if pv.Kind != "party" || !pv.Joinable {
		t.Fatalf("party preview: %+v", pv)
	}
}

func TestGuestRateLimit(t *testing.T) {
	e := newEnv(t)
	_, code, _, _, _, _, _ := setupParty(t, e)
	body := `{"name":"Spam","partyCode":"` + code + `"}`
	for i := 0; i < guestCreatePerHour; i++ {
		if r := e.doIP("203.0.113.7", "POST", "/api/occ/guest", "", body); r.status != 200 {
			t.Fatalf("attempt %d: %d %s", i, r.status, r.body)
		}
	}
	r := e.doIP("203.0.113.7", "POST", "/api/occ/guest", "", body)
	if r.status != http.StatusTooManyRequests || !strings.Contains(string(r.body), "Trop de tentatives") {
		t.Fatalf("expected 429, got %d %s", r.status, r.body)
	}
	// invalid codes count too (no brute force)
	if r := e.doIP("203.0.113.7", "POST", "/api/occ/guest", "", `{"name":"x","partyCode":"ZZZZZZ"}`); r.status != 429 {
		t.Fatalf("invalid code after limit: %d", r.status)
	}
	// another IP is not affected
	if r := e.doIP("198.51.100.9", "POST", "/api/occ/guest", "", body); r.status != 200 {
		t.Fatalf("other ip: %d %s", r.status, r.body)
	}
}

func TestGuestUpgradeKeepsHistory(t *testing.T) {
	e := newEnv(t)
	pizza, _ := e.testRestaurants()
	alice := e.user("Alice")
	pid := e.newOrderingParty(alice, pizza, "Midi pizza")
	lea, _ := e.guest("Léa", map[string]any{"partyCode": e.partyCode(pid)})
	e.expect(200, "POST", "/api/collections/order_items/records", lea.token, map[string]any{"party": pid, "user": lea.id(), "menu_item": e.menuItem(pizza, "Tiramisu")})

	// validations
	e.expect(401, "POST", "/api/occ/me/upgrade", "", map[string]any{"email": "lea@occ.be", "password": "password123"})
	e.expect(400, "POST", "/api/occ/me/upgrade", lea.token, map[string]any{"email": "pas-un-email", "password": "password123"})
	e.expect(400, "POST", "/api/occ/me/upgrade", lea.token, map[string]any{"email": "x@guest.occ.invalid", "password": "password123"})
	e.expect(400, "POST", "/api/occ/me/upgrade", lea.token, map[string]any{"email": "alice@example.com", "password": "password123"})
	e.expect(400, "POST", "/api/occ/me/upgrade", lea.token, map[string]any{"email": "lea@occ.be", "password": "court"})
	e.expect(400, "POST", "/api/occ/me/upgrade", lea.token, map[string]any{"email": "lea@occ.be", "password": "password123", "passwordConfirm": "autre12345"})
	e.expect(400, "POST", "/api/occ/me/upgrade", alice.token, map[string]any{"email": "alice3@occ.be", "password": "password123"})

	var up struct {
		Token  string `json:"token"`
		Record struct {
			ID      string `json:"id"`
			IsGuest bool   `json:"is_guest"`
		} `json:"record"`
	}
	e.expect(200, "POST", "/api/occ/me/upgrade", lea.token, map[string]any{"email": " Lea@OCC.be ", "password": "password123", "passwordConfirm": "password123"}).json(t, &up)
	if up.Record.ID != lea.id() || up.Record.IsGuest || up.Token == "" {
		t.Fatalf("upgrade: %+v", up)
	}
	u := e.reload(lea.id())
	if u.Email() != "lea@occ.be" || u.GetBool(fieldIsGuest) || !u.GetBool(fieldPasswordSet) || u.Verified() {
		t.Fatalf("upgraded record: email=%s guest=%v pwset=%v verified=%v", u.Email(), u.GetBool(fieldIsGuest), u.GetBool(fieldPasswordSet), u.Verified())
	}
	// same record: the order and the history are kept; password login works
	if r := login(e, "lea@occ.be", "password123"); r.status != 200 {
		t.Fatalf("login after upgrade: %d %s", r.status, r.body)
	}
	var h historyResp
	e.expect(200, "GET", "/api/occ/me/history", up.Token, nil).json(t, &h)
	if h.TotalItems != 1 || h.Items[0].ID != pid || len(h.Items[0].Items) != 1 {
		t.Fatalf("history after upgrade: %+v", h)
	}
	// now a regular account: payout profile and party creation allowed
	e.expect(200, "POST", "/api/collections/payout_profiles/records", up.Token, map[string]any{"user": lea.id(), "iban": "BE71096123456769"})
	e.expect(200, "POST", "/api/collections/parties/records", up.Token, map[string]any{"title": "Mon midi"})
	e.expect(400, "POST", "/api/occ/me/upgrade", up.Token, map[string]any{"email": "lea2@occ.be", "password": "password123"})
}

func TestGuestCleanup(t *testing.T) {
	e := newEnv(t)
	pizza, _ := e.testRestaurants()
	alice := e.user("Alice")
	pid := e.newOrderingParty(alice, pizza, "Ancien midi")
	tid, tcode := e.newTeam(alice, nil)
	old, _ := e.guest("Ancien", map[string]any{"partyCode": e.partyCode(pid)})
	e.expect(200, "POST", "/api/occ/teams/join", old.token, map[string]any{"code": tcode})
	e.expect(200, "POST", "/api/collections/order_items/records", old.token, map[string]any{"party": pid, "user": old.id(), "menu_item": e.menuItem(pizza, "Tiramisu")})
	recent, _ := e.guest("Récent", map[string]any{"teamCode": tcode})

	// age every activity of the old guest by 61 days
	past := types.NowDateTime().Add(-61 * 24 * time.Hour).String()
	for _, q := range []string{
		"UPDATE users SET created = {:d}, updated = {:d} WHERE id = {:u}",
		"UPDATE party_members SET created = {:d} WHERE user = {:u}",
		"UPDATE order_items SET updated = {:d} WHERE user = {:u}",
	} {
		if _, err := e.app.DB().NewQuery(q).Bind(map[string]any{"d": past, "u": old.id()}).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	n, err := cleanupGuests(e.app, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("cleanup: n=%d err=%v", n, err)
	}
	o := e.reload(old.id())
	if !isDeleted(o) || o.GetString("name") != domain.DeletedAccountName || !o.GetBool(fieldBanned) {
		t.Fatalf("old guest not anonymised: %v", o)
	}
	if isDeleted(e.reload(recent.id())) || isDeleted(e.reload(alice.id())) {
		t.Fatal("active accounts must be kept")
	}
	team, _ := e.app.FindRecordById(colTeams, tid)
	if isTeamMember(team, old.id()) || !isTeamMember(team, recent.id()) {
		t.Fatalf("team members after cleanup: %v", team.GetStringSlice("members"))
	}
	// history and totals kept
	items, err := e.app.FindAllRecords(colOrderItems)
	if err != nil || len(items) != 1 {
		t.Fatalf("order lines kept: %d %v", len(items), err)
	}
	// old token rejected; idempotent second run
	e.expect(401, "GET", "/api/occ/me/history", old.token, nil)
	if n, err := cleanupGuests(e.app, time.Now()); err != nil || n != 0 {
		t.Fatalf("second cleanup: %d %v", n, err)
	}
	// the cron job is registered on serve
	if !hasCronJob(e.app, guestCleanupJobID) {
		t.Fatal("guest cleanup cron job not registered")
	}
}

func hasCronJob(app core.App, id string) bool {
	for _, j := range app.Cron().Jobs() {
		if j.Id() == id {
			return true
		}
	}
	return false
}

// TestGuestUpgradeWithGoogle: a signed-in guest links Google → same record,
// regular verified account with the Google e-mail.
func TestGuestUpgradeWithGoogle(t *testing.T) {
	const provider = "occfakeguest"
	var current *auth.AuthUser
	auth.Providers[provider] = func() auth.Provider { return &fakeOAuth{user: current} }
	t.Cleanup(func() { delete(auth.Providers, provider) })
	e := newEnvWith(t, testConfig, func(app *tests.TestApp) {
		users, _ := app.FindCollectionByNameOrId(colUsers)
		applyGoogleOAuth(users, GoogleConfig{ClientID: "id", ClientSecret: "secret"})
		users.OAuth2.Providers[0].Name = provider
		if err := app.Save(users); err != nil {
			t.Fatal(err)
		}
	})
	signIn := func(token string, u *auth.AuthUser) resp {
		current = u
		return e.do("POST", "/api/collections/users/auth-with-oauth2", token, map[string]any{
			"provider": provider, "code": "c", "redirectURL": "http://localhost:8090/api/oauth2-redirect",
		})
	}
	alice := e.user("Alice")
	_, code := e.newTeam(alice, nil)
	lea, _ := e.guest("Léa", map[string]any{"teamCode": code})

	// Google address already used by another account: refused, guest unchanged
	if r := signIn(lea.token, &auth.AuthUser{Id: "g-x", Email: "alice@example.com"}); r.status != 400 {
		t.Fatalf("taken e-mail: %d %s", r.status, r.body)
	}
	if !e.reload(lea.id()).GetBool(fieldIsGuest) {
		t.Fatal("guest flag lost on a refused upgrade")
	}
	r := signIn(lea.token, &auth.AuthUser{Id: "g-lea", Email: "lea@gmail.com", Name: "Léa Google"})
	if r.status != 200 {
		t.Fatalf("google upgrade: %d %s", r.status, r.body)
	}
	u := e.reload(lea.id())
	if u.GetBool(fieldIsGuest) || u.Email() != "lea@gmail.com" || !u.Verified() || u.GetString("name") != "Léa" {
		t.Fatalf("upgraded by google: guest=%v email=%s verified=%v name=%s", u.GetBool(fieldIsGuest), u.Email(), u.Verified(), u.GetString("name"))
	}
	// next Google sign-in (no token) lands on the same record
	var ar struct {
		Record struct {
			ID string `json:"id"`
		} `json:"record"`
	}
	signIn("", &auth.AuthUser{Id: "g-lea", Email: "lea@gmail.com"}).json(t, &ar)
	if ar.Record.ID != lea.id() {
		t.Fatalf("google sign-in after upgrade: %s", ar.Record.ID)
	}
}
