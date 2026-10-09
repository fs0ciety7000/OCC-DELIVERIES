package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/auth"
	"golang.org/x/oauth2"
)

// fakeOAuth is an OAuth2 provider answering without any network call.
type fakeOAuth struct {
	auth.BaseProvider
	user *auth.AuthUser
}

func (p *fakeOAuth) FetchToken(string, ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: "x"}, nil
}

func (p *fakeOAuth) FetchAuthUser(*oauth2.Token) (*auth.AuthUser, error) {
	if p.user == nil {
		return nil, errors.New("no user")
	}
	return p.user, nil
}

// TestOAuth2SignUpAndLinking drives the real PocketBase OAuth2 endpoint
// with a fake provider configured like Google (same mapped fields).
func TestOAuth2SignUpAndLinking(t *testing.T) {
	const provider = "occfake"
	var current *auth.AuthUser
	auth.Providers[provider] = func() auth.Provider { return &fakeOAuth{user: current} }
	t.Cleanup(func() { delete(auth.Providers, provider) })

	cfg := testConfig
	cfg.AdminEmails = []string{"chef@example.com"}
	e := newEnvWith(t, cfg, func(app *tests.TestApp) {
		users, _ := app.FindCollectionByNameOrId(colUsers)
		applyGoogleOAuth(users, GoogleConfig{ClientID: "id", ClientSecret: "secret"})
		users.OAuth2.Providers[0].Name = provider
		if err := app.Save(users); err != nil {
			t.Fatal(err)
		}
	})
	signIn := func(u *auth.AuthUser) resp {
		current = u
		return e.do("POST", "/api/collections/users/auth-with-oauth2", "", map[string]any{
			"provider": provider, "code": "c", "redirectURL": "http://localhost:8090/api/oauth2-redirect",
		})
	}
	type authResp struct {
		Token  string `json:"token"`
		Record struct {
			ID string `json:"id"`
		} `json:"record"`
		Meta struct {
			IsNew bool `json:"isNew"`
		} `json:"meta"`
	}

	// 1. new account: Google name, bootstrap admin role, verified, no password of its own
	var r authResp
	if res := signIn(&auth.AuthUser{Id: "g-chef", Email: "chef@example.com", Name: "Chef Cuistot"}); res.status != 200 {
		t.Fatalf("sign-up: %d %s", res.status, res.body)
	} else {
		res.json(t, &r)
	}
	chef := e.reload(r.Record.ID)
	if !r.Meta.IsNew || chef.GetString("name") != "Chef Cuistot" || chef.GetString("role") != roleAdmin || !chef.Verified() ||
		chef.GetBool(fieldPasswordSet) || len(chef.GetString("color")) != 7 {
		t.Fatalf("new oauth account: %v", chef)
	}
	if e.app.TestMailer.TotalSend() != 0 {
		t.Fatal("no verification e-mail for a Google account")
	}

	// 2. no Google name → derived from the e-mail
	signIn(&auth.AuthUser{Id: "g-dora", Email: "dora.durand@example.com"}).json(t, &r)
	if got := e.reload(r.Record.ID).GetString("name"); got != "Dora Durand" {
		t.Fatalf("default name %q", got)
	}

	// 3. existing verified password account with the same e-mail: linked, password kept
	alice := e.user("Alice")
	alice.rec.SetVerified(true)
	if err := e.app.Save(alice.rec); err != nil {
		t.Fatal(err)
	}
	signIn(&auth.AuthUser{Id: "g-alice", Email: "alice@example.com", Name: "Alice G."}).json(t, &r)
	a := e.reload(alice.id())
	if r.Record.ID != alice.id() || r.Meta.IsNew || !a.ValidatePassword("password123") || !a.GetBool(fieldPasswordSet) || a.GetString("name") != "Alice" {
		t.Fatalf("link to verified account: %+v %v", r, a)
	}

	// 4. existing UNverified account: PocketBase links it but resets its
	// password (takeover guard) → password_set false
	bob := e.user("Bob")
	signIn(&auth.AuthUser{Id: "g-bob", Email: "bob@example.com"}).json(t, &r)
	b := e.reload(bob.id())
	if r.Record.ID != bob.id() || !b.Verified() || b.ValidatePassword("password123") || b.GetBool(fieldPasswordSet) {
		t.Fatalf("link to unverified account: %v", b)
	}

	// 5. suspended account: refused, by link or by e-mail
	boss := e.admin("Boss")
	e.expect(200, "POST", "/api/occ/admin/users/"+alice.id()+"/ban", boss.token, map[string]any{})
	res := signIn(&auth.AuthUser{Id: "g-alice", Email: "alice@example.com"})
	if res.status != 403 || !strings.Contains(string(res.body), "Compte suspendu") {
		t.Fatalf("banned oauth: %d %s", res.status, res.body)
	}
	carol := e.user("Carol")
	carol.rec.Set(fieldBanned, true)
	if err := e.app.Save(carol.rec); err != nil {
		t.Fatal(err)
	}
	if res := signIn(&auth.AuthUser{Id: "g-carol", Email: "carol@example.com"}); res.status != 403 {
		t.Fatalf("banned by e-mail: %d %s", res.status, res.body)
	}
	if eas, _ := e.app.FindAllExternalAuthsByRecord(e.reload(carol.id())); len(eas) != 0 {
		t.Fatal("a suspended account must not get linked")
	}
}
