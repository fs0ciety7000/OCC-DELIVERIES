package app

import (
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// ------------------------------------------------------------ env → settings

func TestMailConfigFromEnv(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	mc := mailConfigFromEnv(env(nil))
	if mc.Host != "" || mc.Port != 587 || mc.TLS || mc.FromName != "OCC Deliveries" {
		t.Fatalf("defaults: %+v", mc)
	}
	mc = mailConfigFromEnv(env(map[string]string{
		"OCC_SMTP_HOST": " smtp-relay.brevo.com ", "OCC_SMTP_PORT": "465", "OCC_SMTP_USERNAME": "bot@fs0ciety.org",
		"OCC_SMTP_PASSWORD": " s3cret ",
	}))
	if mc.Host != "smtp-relay.brevo.com" || mc.Port != 465 || !mc.TLS || mc.From != "bot@fs0ciety.org" || mc.Password != " s3cret " {
		t.Fatalf("465: %+v", mc)
	}
	mc = mailConfigFromEnv(env(map[string]string{
		"OCC_SMTP_HOST": "smtp.gmail.com", "OCC_SMTP_PORT": "abc", "OCC_SMTP_TLS": "true", "OCC_SMTP_USERNAME": "login",
		"OCC_MAIL_FROM": "noreply@fs0ciety.org", "OCC_MAIL_FROM_NAME": "OCC",
	}))
	if mc.Port != 587 || !mc.TLS || mc.From != "noreply@fs0ciety.org" || mc.FromName != "OCC" {
		t.Fatalf("explicit: %+v", mc)
	}

	// settings: untouched without a host (except appURL)
	s := &core.Settings{}
	s.Meta.SenderAddress = "admin@ui.example"
	if !applyMailSettings(s, MailConfig{}, "https://eat.fs0ciety.org/") || s.Meta.AppURL != "https://eat.fs0ciety.org" {
		t.Fatalf("appURL: %+v", s.Meta)
	}
	if s.SMTP.Enabled || s.Meta.SenderAddress != "admin@ui.example" {
		t.Fatal("SMTP must stay untouched without OCC_SMTP_HOST")
	}
	full := MailConfig{Host: "smtp.example.com", Port: 587, Username: "u", Password: "p", From: "noreply@fs0ciety.org", FromName: "OCC Deliveries"}
	if !applyMailSettings(s, full, "https://eat.fs0ciety.org") {
		t.Fatal("expected a change")
	}
	if !s.SMTP.Enabled || s.SMTP.Host != "smtp.example.com" || s.SMTP.Password != "p" || s.SMTP.TLS ||
		s.Meta.SenderAddress != "noreply@fs0ciety.org" || s.Meta.SenderName != "OCC Deliveries" {
		t.Fatalf("applied: %+v %+v", s.SMTP, s.Meta)
	}
	if applyMailSettings(s, full, "https://eat.fs0ciety.org") {
		t.Fatal("second application must be a no-op")
	}
}

func TestGoogleOAuthFromEnv(t *testing.T) {
	g := googleConfigFromEnv(func(k string) string {
		return map[string]string{"OCC_GOOGLE_CLIENT_ID": " id.apps.googleusercontent.com ", "OCC_GOOGLE_CLIENT_SECRET": "sec"}[k]
	})
	if !g.Enabled() || g.ClientID != "id.apps.googleusercontent.com" {
		t.Fatalf("%+v", g)
	}

	col := core.NewAuthCollection("users")
	col.Fields.Add(&core.TextField{Name: "name"}, &core.FileField{Name: "avatar"})
	if applyGoogleOAuth(col, GoogleConfig{ClientID: "only-id"}) || col.OAuth2.Enabled {
		t.Fatal("half a configuration must not touch the collection")
	}
	if !applyGoogleOAuth(col, g) {
		t.Fatal("expected a change")
	}
	p, ok := col.OAuth2.GetProviderConfig("google")
	if !col.OAuth2.Enabled || !ok || p.ClientId != g.ClientID || p.ClientSecret != "sec" {
		t.Fatalf("provider: %+v", col.OAuth2)
	}
	if col.OAuth2.MappedFields.Name != "name" || col.OAuth2.MappedFields.AvatarURL != "avatar" {
		t.Fatalf("mapped fields: %+v", col.OAuth2.MappedFields)
	}
	if applyGoogleOAuth(col, g) {
		t.Fatal("idempotent")
	}
	g.ClientSecret = "rotated"
	if !applyGoogleOAuth(col, g) || len(col.OAuth2.Providers) != 1 {
		t.Fatalf("secret rotation: %+v", col.OAuth2.Providers)
	}
}

func TestGoogleOAuthAppliedOnServe(t *testing.T) {
	cfg := testConfig
	cfg.Google = GoogleConfig{ClientID: "cid", ClientSecret: "csecret"}
	e := newEnvWith(t, cfg, nil)
	var methods struct {
		OAuth2 struct {
			Enabled   bool `json:"enabled"`
			Providers []struct {
				Name string `json:"name"`
			} `json:"providers"`
		} `json:"oauth2"`
	}
	e.expect(200, "GET", "/api/collections/users/auth-methods", "", nil).json(t, &methods)
	if !methods.OAuth2.Enabled || len(methods.OAuth2.Providers) != 1 || methods.OAuth2.Providers[0].Name != "google" {
		t.Fatalf("auth methods: %+v", methods)
	}

	// without env, an existing configuration (made in /_/) is kept
	e2 := newEnvWith(t, testConfig, func(app *tests.TestApp) {
		users, _ := app.FindCollectionByNameOrId(colUsers)
		applyGoogleOAuth(users, GoogleConfig{ClientID: "ui", ClientSecret: "ui"})
		if err := app.Save(users); err != nil {
			t.Fatal(err)
		}
	})
	users, _ := e2.app.FindCollectionByNameOrId(colUsers)
	if p, ok := users.OAuth2.GetProviderConfig("google"); !ok || p.ClientId != "ui" {
		t.Fatalf("admin UI configuration lost: %+v", users.OAuth2)
	}
	// templates point to the SPA
	if !strings.Contains(users.ResetPasswordTemplate.Body, "{APP_URL}/auth/reinitialiser/{TOKEN}") {
		t.Fatal("reset template not installed")
	}
}

// ------------------------------------------------------------- moderation

func (e *env) superuser() string {
	e.t.Helper()
	col, err := e.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		e.t.Fatal(err)
	}
	su := core.NewRecord(col)
	su.SetEmail("root@example.com")
	su.SetPassword("password123456")
	if err := e.app.Save(su); err != nil {
		e.t.Fatal(err)
	}
	tok, err := su.NewAuthToken()
	if err != nil {
		e.t.Fatal(err)
	}
	return tok
}

func (e *env) reload(id string) *core.Record {
	e.t.Helper()
	r, err := e.app.FindRecordById(colUsers, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

func login(e *env, email, password string) resp {
	return e.do("POST", "/api/collections/users/auth-with-password", "", map[string]any{"identity": email, "password": password})
}

func TestAdminUserActionsAuthz(t *testing.T) {
	e := newEnv(t)
	boss, alice := e.admin("Boss"), e.user("Alice")
	routes := []struct{ method, url string }{
		{"POST", "/api/occ/admin/users/" + alice.id() + "/ban"},
		{"POST", "/api/occ/admin/users/" + alice.id() + "/unban"},
		{"POST", "/api/occ/admin/users/" + alice.id() + "/logout"},
		{"POST", "/api/occ/admin/users/" + alice.id() + "/password-reset"},
		{"DELETE", "/api/occ/admin/users/" + alice.id()},
		{"GET", "/api/occ/admin/mail"},
		{"POST", "/api/occ/admin/mail/test"},
	}
	for _, r := range routes {
		e.expect(401, r.method, r.url, "", map[string]any{})
		e.expect(403, r.method, r.url, alice.token, map[string]any{})
	}
	// an admin never acts on their own account here
	if msg := e.expect(400, "POST", "/api/occ/admin/users/"+boss.id()+"/ban", boss.token, map[string]any{}).m(t)["message"]; !strings.Contains(msg.(string), "propre compte") {
		t.Fatalf("self ban: %v", msg)
	}
	e.expect(400, "DELETE", "/api/occ/admin/users/"+boss.id(), boss.token, nil)
	e.expect(400, "PATCH", "/api/occ/admin/users/"+boss.id()+"/role", boss.token, map[string]any{"role": "user"})
	e.expect(404, "POST", "/api/occ/admin/users/nope/ban", boss.token, map[string]any{})
	e.expect(400, "POST", "/api/occ/admin/users/"+alice.id()+"/ban", boss.token, map[string]any{"reason": strings.Repeat("x", 301)})

	// last active admin: even a superuser cannot ban / demote / delete it
	su := e.superuser()
	for _, r := range []struct{ method, url string }{
		{"POST", "/api/occ/admin/users/" + boss.id() + "/ban"},
		{"DELETE", "/api/occ/admin/users/" + boss.id()},
	} {
		if msg := e.expect(400, r.method, r.url, su, map[string]any{}).m(t)["message"]; !strings.Contains(msg.(string), "dernier") {
			t.Fatalf("%s %s: %v", r.method, r.url, msg)
		}
	}
	e.expect(400, "PATCH", "/api/occ/admin/users/"+boss.id()+"/role", su, map[string]any{"role": "user"})
	// with a second admin it works
	chef := e.admin("Chef")
	e.expect(200, "POST", "/api/occ/admin/users/"+boss.id()+"/ban", chef.token, map[string]any{"reason": "test"})
}

func TestBannedUserIsLockedOut(t *testing.T) {
	e := newEnv(t)
	boss, alice, bob := e.admin("Boss"), e.user("Alice"), e.user("Bob")
	e.expect(200, "GET", "/api/occ/me/history", alice.token, nil)

	var out struct{ User adminUser }
	e.expect(200, "POST", "/api/occ/admin/users/"+alice.id()+"/ban", boss.token, map[string]any{"reason": "  spam  "}).json(t, &out)
	if !out.User.Banned || out.User.BannedReason != "spam" || out.User.BannedAt == "" {
		t.Fatalf("ban response: %+v", out.User)
	}
	e.expect(400, "POST", "/api/occ/admin/users/"+alice.id()+"/ban", boss.token, map[string]any{})

	// the issued token is dead (tokenKey rotated)…
	e.expect(401, "GET", "/api/occ/me/history", alice.token, nil)
	// …and signing in again is refused with a French message
	r := login(e, "alice@example.com", "password123")
	if r.status != 403 || !strings.Contains(string(r.body), "Compte suspendu") {
		t.Fatalf("login of a banned user: %d %s", r.status, r.body)
	}
	// hidden from other users' reads
	view := e.expect(200, "GET", "/api/collections/users/records/"+alice.id(), bob.token, nil).m(t)
	if _, leaked := view["banned"]; leaked {
		t.Fatalf("banned field visible: %v", view)
	}

	// a ban written behind the hooks' back (raw SQL) is still enforced on
	// an already issued token, everywhere
	tok, _ := bob.rec.NewAuthToken()
	if _, err := e.app.DB().Update(colUsers, dbx.Params{fieldBanned: true}, dbx.HashExp{"id": bob.id()}).Execute(); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"/api/occ/me/history", "/api/collections/parties/records", "/api/collections/users/auth-refresh"} {
		method := "GET"
		if strings.HasSuffix(u, "auth-refresh") {
			method = "POST"
		}
		r := e.do(method, u, tok, nil)
		if r.status != 403 || !strings.Contains(string(r.body), "Compte suspendu") {
			t.Fatalf("%s with a banned token: %d %s", u, r.status, r.body)
		}
	}

	// unban: alice can sign in again
	e.expect(200, "POST", "/api/occ/admin/users/"+alice.id()+"/unban", boss.token, nil)
	e.expect(400, "POST", "/api/occ/admin/users/"+alice.id()+"/unban", boss.token, nil)
	if r := login(e, "alice@example.com", "password123"); r.status != 200 {
		t.Fatalf("login after unban: %d %s", r.status, r.body)
	}

	// forced logout
	tok2, _ := e.reload(alice.id()).NewAuthToken()
	e.expect(200, "GET", "/api/occ/me/history", tok2, nil)
	e.expect(200, "POST", "/api/occ/admin/users/"+alice.id()+"/logout", boss.token, nil)
	e.expect(401, "GET", "/api/occ/me/history", tok2, nil)
}

func TestAdminUsersListFilters(t *testing.T) {
	e := newEnv(t)
	boss, alice, bob := e.admin("Boss"), e.user("Alice"), e.user("Bob")
	e.expect(200, "POST", "/api/occ/admin/users/"+alice.id()+"/ban", boss.token, map[string]any{})
	bob.rec.SetVerified(true)
	if err := e.app.Save(bob.rec); err != nil {
		t.Fatal(err)
	}
	type list struct {
		TotalItems int         `json:"totalItems"`
		Items      []adminUser `json:"items"`
	}
	get := func(query string) list {
		var l list
		e.expect(200, "GET", "/api/occ/admin/users?"+query, boss.token, nil).json(t, &l)
		return l
	}
	if l := get("status=banned"); l.TotalItems != 1 || l.Items[0].ID != alice.id() || !l.Items[0].Banned {
		t.Fatalf("banned: %+v", l)
	}
	for _, it := range get("status=unverified").Items {
		if it.Verified {
			t.Fatalf("verified user in the unverified filter: %+v", it)
		}
	}
	if l := get("role=admin"); l.TotalItems != 1 || l.Items[0].ID != boss.id() {
		t.Fatalf("admins: %+v", l)
	}
	if l := get("q=BOB"); l.TotalItems != 1 || l.Items[0].Email != "bob@example.com" || !l.Items[0].PasswordSet || l.Items[0].Providers == nil {
		t.Fatalf("search: %+v", l)
	}
	if l := get("q=%25"); l.TotalItems != 0 {
		t.Fatalf("LIKE wildcard not escaped: %+v", l)
	}
	if l := get("perPage=1&page=2"); len(l.Items) != 1 || l.TotalItems < 3 {
		t.Fatalf("pagination: %+v", l)
	}
}

// ---------------------------------------------------------- anonymisation

func TestDeleteAccountKeepsHistory(t *testing.T) {
	e := newEnv(t)
	boss := e.admin("Boss")
	pid, _, pizza, _, alice, bob, _ := setupParty(t, e)
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	tira := e.menuItem(pizza, "Tiramisu")
	for _, u := range []user{alice, bob} {
		e.expect(200, "POST", "/api/collections/order_items/records", u.token, map[string]any{"party": pid, "user": u.id(), "menu_item": tira, "quantity": 2})
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": alice.id()})
	e.expect(200, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{"user": bob.id(), "holder_name": "Bob"})

	var before, after domain.Summary
	e.expect(200, "GET", path("/api/occ/parties/%s/summary", pid), alice.token, nil).json(t, &before)

	// alice (payer, not refunded) cannot delete herself yet; bob types the word
	e.expect(400, "POST", "/api/occ/me/delete", alice.token, map[string]any{"confirm": "SUPPRIMER"})
	e.expect(400, "POST", "/api/occ/me/delete", bob.token, map[string]any{"confirm": "oui"})
	// admin deletion instead (same anonymisation)
	var out struct{ User adminUser }
	e.expect(200, "DELETE", "/api/occ/admin/users/"+bob.id(), boss.token, nil).json(t, &out)
	if !out.User.Deleted || !out.User.Banned || out.User.Name != domain.DeletedAccountName || out.User.Email != domain.AnonymizedEmail(bob.id()) {
		t.Fatalf("anonymised: %+v", out.User)
	}

	e.expect(200, "GET", path("/api/occ/parties/%s/summary", pid), alice.token, nil).json(t, &after)
	if after.GrandTotal != before.GrandTotal || len(after.Participants) != len(before.Participants) {
		t.Fatalf("history changed: %d → %d", before.GrandTotal, after.GrandTotal)
	}
	found := false
	for _, p := range after.Participants {
		if p.User.ID == bob.id() {
			found = p.User.Name == domain.DeletedAccountName && p.Total > 0
		}
	}
	if !found {
		t.Fatalf("bob's share lost or not anonymised: %+v", after.Participants)
	}
	if n, _ := e.app.CountRecords(colPayoutProfiles, dbx.HashExp{"user": bob.id()}); n != 0 {
		t.Fatal("payout profile kept")
	}
	if r := login(e, "bob@example.com", "password123"); r.status == 200 {
		t.Fatal("deleted account can still sign in")
	}
	e.expect(401, "GET", "/api/occ/me/history", bob.token, nil)
	e.expect(400, "POST", "/api/occ/admin/users/"+bob.id()+"/unban", boss.token, nil)
	e.expect(400, "DELETE", "/api/occ/admin/users/"+bob.id(), boss.token, nil)
	// no hard delete through the collection API anymore
	e.expect(403, "DELETE", "/api/collections/users/records/"+alice.id(), alice.token, nil)

	// self-service: carol, nothing pending
	carol := e.user("Carol2")
	e.expect(204, "POST", "/api/occ/me/delete", carol.token, map[string]any{"confirm": " supprimer "})
	if r := e.reload(carol.id()); r.GetString("name") != domain.DeletedAccountName || !isDeleted(r) {
		t.Fatalf("self delete: %v", r)
	}
}

// ------------------------------------------------------------------ mails

func enableTestSMTP(e *env) {
	s := e.app.Settings()
	s.SMTP.Enabled = true
	s.SMTP.Host = "smtp.example.com"
	s.SMTP.Port = 587
	s.Meta.SenderAddress = "noreply@fs0ciety.org"
	s.Meta.SenderName = "OCC Deliveries"
}

func TestMailWithoutSMTP(t *testing.T) {
	e := newEnv(t)
	boss, alice := e.admin("Boss"), e.user("Alice")
	for _, u := range []string{"/api/occ/admin/mail/test", "/api/occ/admin/users/" + alice.id() + "/password-reset"} {
		r := e.expect(400, "POST", u, boss.token, nil)
		if !strings.Contains(string(r.body), "OCC_SMTP") {
			t.Fatalf("%s: %s", u, r.body)
		}
	}
	if cfg := e.expect(200, "GET", "/api/occ/config", "", nil).m(t); cfg["mailEnabled"] != false {
		t.Fatalf("config: %v", cfg)
	}
	if st := e.expect(200, "GET", "/api/occ/admin/mail", boss.token, nil).m(t); st["enabled"] != false {
		t.Fatalf("status: %v", st)
	}
	if e.app.TestMailer.TotalSend() != 0 {
		t.Fatal("no mail expected")
	}
}

func TestMailTemplatesPointToSPA(t *testing.T) {
	e := newEnv(t)
	enableTestSMTP(e)
	boss, alice := e.admin("Boss"), e.user("Alice")
	mailer := e.app.TestMailer

	e.expect(200, "POST", "/api/occ/admin/mail/test", boss.token, nil)
	if m := mailer.LastMessage(); m.To[0].Address != "boss@example.com" || !strings.Contains(m.Subject, "E-mail de test") || strings.Contains(m.HTML, "{APP_") {
		t.Fatalf("test mail: %s / %s", m.Subject, m.HTML)
	}
	if cfg := e.expect(200, "GET", "/api/occ/config", "", nil).m(t); cfg["mailEnabled"] != true {
		t.Fatalf("config: %v", cfg)
	}

	check := func(what, subject, route string) {
		t.Helper()
		m := mailer.LastMessage()
		if !strings.Contains(m.Subject, subject) {
			t.Errorf("%s subject %q", what, m.Subject)
		}
		if !strings.Contains(m.HTML, "http://localhost:8090"+route) || strings.Contains(m.HTML, "/_/") || strings.Contains(m.HTML, "{TOKEN}") {
			t.Errorf("%s body does not link to %s: %s", what, route, m.HTML)
		}
	}

	// admin reset link
	e.expect(200, "POST", "/api/occ/admin/users/"+alice.id()+"/password-reset", boss.token, nil)
	check("reset", "Réinitialise ton mot de passe", "/auth/reinitialiser/")

	// password sign-up sends the verification e-mail
	n := mailer.TotalSend()
	e.expect(200, "POST", "/api/collections/users/records", "", map[string]any{
		"email": "dora@example.com", "password": "password123", "passwordConfirm": "password123", "name": "Dora",
	})
	if mailer.TotalSend() != n+1 || mailer.LastMessage().To[0].Address != "dora@example.com" {
		t.Fatalf("verification not sent on sign-up (%d mails)", mailer.TotalSend())
	}
	check("verification", "Confirme ton adresse e-mail", "/auth/verifier/")

	// e-mail change (PocketBase endpoint, our template)
	e.expect(204, "POST", "/api/collections/users/request-email-change", alice.token, map[string]any{"newEmail": "alice.new@example.com"})
	check("email change", "nouvelle adresse e-mail", "/auth/changer-email/")
	if mailer.LastMessage().To[0].Address != "alice.new@example.com" {
		t.Fatal("email change sent to the wrong address")
	}

	// self-service reset (PocketBase endpoint) marks the password as chosen
	n = mailer.TotalSend()
	e.expect(204, "POST", "/api/collections/users/request-password-reset", "", map[string]any{"email": "alice@example.com"})
	for i := 0; i < 200 && mailer.TotalSend() == n; i++ { // sent in background by PocketBase
		time.Sleep(10 * time.Millisecond)
	}
	check("self reset", "Réinitialise", "/auth/reinitialiser/")
	link := mailer.LastMessage().HTML
	i := strings.Index(link, "/auth/reinitialiser/") + len("/auth/reinitialiser/")
	token := link[i : i+strings.IndexAny(link[i:], `"<`)]
	fresh := e.reload(alice.id())
	fresh.Set(fieldPasswordSet, false)
	if err := e.app.Save(fresh); err != nil {
		t.Fatal(err)
	}
	e.expect(204, "POST", "/api/collections/users/confirm-password-reset", "", map[string]any{
		"token": token, "password": "nouveau-mdp-42", "passwordConfirm": "nouveau-mdp-42",
	})
	if !e.reload(alice.id()).GetBool(fieldPasswordSet) {
		t.Fatal("password_set not updated by the reset")
	}
	if r := login(e, "alice@example.com", "nouveau-mdp-42"); r.status != 200 {
		t.Fatalf("login with the new password: %d %s", r.status, r.body)
	}
}

// ------------------------------------------------------ me/account, unlink

func TestAccountProvidersAndUnlink(t *testing.T) {
	e := newEnv(t)
	alice := e.user("Alice")
	users, _ := e.app.FindCollectionByNameOrId(colUsers)
	ea := core.NewExternalAuth(e.app)
	ea.SetCollectionRef(users.Id)
	ea.SetRecordRef(alice.id())
	ea.SetProvider("google")
	ea.SetProviderId("g-alice")
	if err := e.app.Save(ea); err != nil {
		t.Fatal(err)
	}
	var acc accountInfo
	e.expect(200, "GET", "/api/occ/me/account", alice.token, nil).json(t, &acc)
	if !acc.PasswordSet || len(acc.Providers) != 1 || acc.Providers[0].Provider != "google" || acc.MailEnabled {
		t.Fatalf("account: %+v", acc)
	}
	e.expect(401, "GET", "/api/occ/me/account", "", nil)

	// Google-only account (random password): unlinking would lock it out
	fresh := e.reload(alice.id())
	fresh.Set(fieldPasswordSet, false)
	if err := e.app.Save(fresh); err != nil {
		t.Fatal(err)
	}
	e.expect(400, "DELETE", "/api/occ/me/providers/google", alice.token, nil)
	e.expect(400, "DELETE", "/api/collections/_externalAuths/records/"+ea.Id, alice.token, nil)

	// after choosing a password (profile update with oldPassword)…
	tok := alice.token
	e.expect(200, "PATCH", "/api/collections/users/records/"+alice.id(), tok, map[string]any{
		"oldPassword": "password123", "password": "encore-mieux-1", "passwordConfirm": "encore-mieux-1",
	})
	if !e.reload(alice.id()).GetBool(fieldPasswordSet) {
		t.Fatal("password_set not updated on password change")
	}
	tok2, _ := e.reload(alice.id()).NewAuthToken()
	e.expect(200, "DELETE", "/api/occ/me/providers/google", tok2, nil)
	e.expect(404, "DELETE", "/api/occ/me/providers/google", tok2, nil)
}

func TestNameFromEmail(t *testing.T) {
	for in, want := range map[string]string{
		"alice.dupont@x.be": "Alice Dupont",
		"bob@x.be":          "Bob",
		"@x.be":             "Collègue",
		"j_doe+occ@x.be":    "J Doe Occ",
	} {
		if got := nameFromEmail(in); got != want {
			t.Errorf("%s → %q, want %q", in, got, want)
		}
	}
}
