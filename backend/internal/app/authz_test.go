package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tests"
)

// setupParty creates a party hosted by alice with bob as member, in ordering
// on the test pizza restaurant.
func setupParty(t *testing.T, e *env) (pid, code, pizza, burger string, alice, bob, carol user) {
	t.Helper()
	pizza, burger = e.testRestaurants()
	alice, bob, carol = e.user("Alice"), e.user("Bob"), e.user("Carol")
	p := e.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{
		"title": "Test", "candidates": []string{pizza, burger},
	}).m(t)
	pid, code = p["id"].(string), p["code"].(string)
	e.expect(200, "POST", "/api/occ/parties/join", bob.token, map[string]any{"code": code})
	return
}

func TestAuthz(t *testing.T) {
	e := newEnv(t)
	pid, _, pizza, _, alice, bob, carol := setupParty(t, e)

	// non-member cannot view the party, its members or its summary
	e.expect(404, "GET", "/api/collections/parties/records/"+pid, carol.token, nil)
	if r := e.expect(200, "GET", "/api/collections/parties/records", carol.token, nil); !strings.Contains(string(r.body), `"totalItems":0`) {
		t.Fatalf("carol lists parties: %s", r.body)
	}
	if r := e.expect(200, "GET", "/api/collections/party_members/records", carol.token, nil); !strings.Contains(string(r.body), `"totalItems":0`) {
		t.Fatalf("carol lists members: %s", r.body)
	}
	e.expect(200, "GET", "/api/collections/parties/records/"+pid, bob.token, nil)
	e.expect(403, "GET", path("/api/occ/parties/%s/summary", pid), carol.token, nil)
	e.expect(401, "GET", path("/api/occ/parties/%s/summary", pid), "", nil)
	e.expect(404, "GET", "/api/occ/parties/unknownid/summary", bob.token, nil)

	// non-host cannot transition nor update the party
	e.expect(403, "POST", path("/api/occ/parties/%s/transition", pid), bob.token, map[string]any{"to": "voting"})
	e.expect(403, "POST", path("/api/occ/parties/%s/transition", pid), carol.token, map[string]any{"to": "voting"})
	e.expect(404, "PATCH", "/api/collections/parties/records/"+pid, bob.token, map[string]any{"title": "pirate"})

	// protected fields cannot be changed by the host either
	for field, val := range map[string]any{
		"status": "closed", "code": "ABCDEF", "host": bob.id(), "members": []string{alice.id()},
		"restaurant": pizza, "payer": bob.id(), "dispatch": map[string]any{"method": "x"}, "closed_at": "2026-01-01 00:00:00.000Z",
	} {
		r := e.expect(400, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{field: val})
		if !strings.Contains(string(r.body), "ne peut pas être modifié") {
			t.Fatalf("%s: %s", field, r.body)
		}
	}
	e.expect(200, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"title": "Nouveau titre", "notes": "3e étage"})
	e.expect(400, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"tip": -5})

	// party_members / payments are server-only
	e.expect(403, "POST", "/api/collections/party_members/records", carol.token, map[string]any{"party": pid, "user": carol.id(), "role": "member"})
	e.expect(403, "POST", "/api/collections/payments/records", alice.token, map[string]any{"party": pid, "debtor": bob.id(), "creditor": alice.id(), "status": "confirmed"})

	// vote outside the voting phase
	e.expect(400, "POST", "/api/collections/votes/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "restaurant": pizza})

	// voting: candidates only, members only, own votes only
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "voting"})
	seedRest, err := e.app.FindFirstRecordByData(colRestaurants, "slug", "la-bella-nonna")
	if err != nil {
		t.Fatal(err)
	}
	r := e.expect(400, "POST", "/api/collections/votes/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "restaurant": seedRest.Id})
	if !strings.Contains(string(r.body), "candidats") {
		t.Fatalf("non-candidate vote: %s", r.body)
	}
	e.expect(400, "POST", "/api/collections/votes/records", carol.token, map[string]any{"party": pid, "user": carol.id(), "restaurant": pizza})
	e.expect(400, "POST", "/api/collections/votes/records", bob.token, map[string]any{"party": pid, "user": alice.id(), "restaurant": pizza})
	v := e.expect(200, "POST", "/api/collections/votes/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "restaurant": pizza}).m(t)
	e.expect(400, "POST", "/api/collections/votes/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "restaurant": pizza}) // unique
	e.expect(403, "PATCH", "/api/collections/votes/records/"+v["id"].(string), bob.token, map[string]any{"restaurant": pizza})
	e.expect(404, "DELETE", "/api/collections/votes/records/"+v["id"].(string), alice.token, nil)

	// review → paying only through /payer ; imposed restaurant must be a candidate
	e.expect(400, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": seedRest.Id})
	e.expect(400, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	e.expect(400, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"}) // empty cart

	// ready: members only
	e.expect(403, "POST", path("/api/occ/parties/%s/ready", pid), carol.token, map[string]any{"ready": true})

	// payer before review
	e.expect(400, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": alice.id()})

	// host cannot leave; bob leaves and his items/votes go away
	tira := e.menuItem(pizza, "Tiramisu")
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "menu_item": tira})
	e.expect(400, "POST", path("/api/occ/parties/%s/leave", pid), alice.token, nil)
	e.expect(403, "POST", path("/api/occ/parties/%s/leave", pid), carol.token, nil)
	e.expect(200, "POST", path("/api/occ/parties/%s/leave", pid), bob.token, nil)
	if n, _ := e.app.CountRecords(colOrderItems, dbx.HashExp{"party": pid}); n != 0 {
		t.Fatalf("bob's items should be deleted, %d left", n)
	}
	if n, _ := e.app.CountRecords(colVotes, dbx.HashExp{"party": pid}); n != 0 {
		t.Fatalf("bob's votes should be deleted, %d left", n)
	}
	e.expect(404, "GET", "/api/collections/parties/records/"+pid, bob.token, nil)

	// cancel, then nothing else is possible
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "cancelled"})
	e.expect(400, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "cancelled"})
	e.expect(400, "POST", "/api/occ/parties/join", carol.token, map[string]any{"code": e.partyCode(pid)})
	e.expect(400, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"title": "x"})
}

func (e *env) partyCode(pid string) string {
	p, err := e.app.FindRecordById(colParties, pid)
	if err != nil {
		e.t.Fatal(err)
	}
	return p.GetString("code")
}

func TestPayerRecomputeLockedAfterConfirmation(t *testing.T) {
	e := newEnv(t)
	pid, _, pizza, _, alice, bob, carol := setupParty(t, e)
	e.expect(200, "POST", "/api/occ/parties/join", carol.token, map[string]any{"code": e.partyCode(pid)})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	tira := e.menuItem(pizza, "Tiramisu")
	for _, u := range []user{alice, bob, carol} {
		e.expect(200, "POST", "/api/collections/order_items/records", u.token, map[string]any{"party": pid, "user": u.id(), "menu_item": tira, "quantity": 1})
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})
	// split 299 between 3: 100 + 100 + 99, total = 3×600 + 299
	var payr struct {
		Payments []struct {
			ID     string `json:"id"`
			Debtor string `json:"debtor"`
			Amount int    `json:"amount"`
		} `json:"payments"`
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": alice.id()}).json(t, &payr)
	sum := 0
	var bobPay string
	for _, p := range payr.Payments {
		sum += p.Amount
		if p.Debtor == bob.id() {
			bobPay = p.ID
		}
	}
	if len(payr.Payments) != 3 || sum != 3*600+299 {
		t.Fatalf("payments: %+v", payr)
	}

	// carol (member, not creditor/host) cannot confirm bob's payment
	e.expect(403, "POST", path("/api/occ/payments/%s/action", bobPay), carol.token, map[string]any{"action": "confirm"})
	// host confirms -> recompute is now locked
	e.expect(200, "POST", path("/api/occ/payments/%s/action", bobPay), alice.token, map[string]any{"action": "confirm"})
	e.expect(400, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": bob.id()})

	// manual close by the host
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "closed"})
}

func TestReopenResetsReady(t *testing.T) {
	e := newEnv(t)
	pid, _, pizza, _, alice, bob, _ := setupParty(t, e)
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	tira := e.menuItem(pizza, "Tiramisu")
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "menu_item": tira})
	e.expect(200, "POST", path("/api/occ/parties/%s/ready", pid), bob.token, map[string]any{"ready": true})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering"})
	if n, _ := e.app.CountRecords(colPartyMembers, dbx.HashExp{"party": pid, "ready": true}); n != 0 {
		t.Fatalf("ready flags not reset: %d", n)
	}
}

func TestUsersColorAndVisibility(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.user("Alice"), e.user("Bob")
	if c := alice.rec.GetString("color"); len(c) != 7 || c[0] != '#' {
		t.Fatalf("color not assigned: %q", c)
	}
	e.expect(200, "GET", "/api/collections/users/records/"+bob.id(), alice.token, nil)
	e.expect(404, "GET", "/api/collections/users/records/"+bob.id(), "", nil)
	e.expect(404, "PATCH", "/api/collections/users/records/"+bob.id(), alice.token, map[string]any{"name": "x"})
}

func TestWalletQRAuthz(t *testing.T) {
	e := newEnv(t)
	pid, _, pizza, _, alice, bob, carol := setupParty(t, e)
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	tira := e.menuItem(pizza, "Tiramisu")
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "menu_item": tira})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})

	// alice is the payer with only a Bancontact QR (no id, no IBAN)
	prof := e.expect(200, "POST", "/api/collections/payout_profiles/records", alice.token, map[string]any{"user": alice.id()}).m(t)
	rec, _ := e.app.FindRecordById(colPayoutProfiles, prof["id"].(string))
	e.attachFile(rec, "bancontact_qr", "bc.png", pngBytes(t))
	// other users cannot read the private profile
	e.expect(404, "GET", "/api/collections/payout_profiles/records/"+rec.Id, bob.token, nil)

	var payr struct {
		Payments []struct {
			ID string `json:"id"`
		} `json:"payments"`
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": alice.id()}).json(t, &payr)
	payID := payr.Payments[0].ID

	var qr map[string]any
	e.expect(200, "GET", path("/api/occ/payments/%s/qr", payID), bob.token, nil).json(t, &qr)
	if qr["epc"] != nil || qr["link"] != nil || qr["wero"] != nil {
		t.Fatalf("unexpected payout data: %v", qr)
	}
	bc := qr["bancontact"].(map[string]any)
	if bc["phone"] != "" || bc["hasQr"] != true {
		t.Fatalf("bancontact: %v", bc)
	}
	if m := qr["methods"].([]any); len(m) != 3 || m[0] != "bancontact" {
		t.Fatalf("methods: %v", m)
	}

	r := e.expect(200, "GET", path("/api/occ/payments/%s/wallet-qr/bancontact", payID), bob.token, nil)
	if r.header.Get("Content-Type") != "image/png" || !strings.HasPrefix(r.header.Get("Cache-Control"), "private") {
		t.Fatalf("headers: %v", r.header)
	}
	e.expect(404, "GET", path("/api/occ/payments/%s/wallet-qr/wero", payID), bob.token, nil)
	e.expect(403, "GET", path("/api/occ/payments/%s/wallet-qr/bancontact", payID), carol.token, nil)
	e.expect(404, "GET", "/api/occ/payments/nope/wallet-qr/bancontact", bob.token, nil)
}

// Public endpoints, exercised through the PocketBase tests.ApiScenario runner.
func TestPublicEndpoints(t *testing.T) {
	factory := func(t testing.TB) *tests.TestApp { return newTestApp(t) }

	scenarios := []tests.ApiScenario{
		{
			Name: "health", Method: http.MethodGet, URL: "/api/occ/health",
			ExpectedStatus: 200, ExpectedContent: []string{`"status":"ok"`, `"version":"test"`},
			TestAppFactory: factory,
		},
		{
			Name: "config", Method: http.MethodGet, URL: "/api/occ/config",
			ExpectedStatus:  200,
			ExpectedContent: []string{`"currency":"EUR"`, `"label":"Mons"`, `"id":"ubereats"`, `"id":"takeaway"`, `"enabled":true`},
			TestAppFactory:  factory,
		},
		{
			Name: "nearby default location", Method: http.MethodGet, URL: "/api/occ/restaurants/nearby",
			ExpectedStatus: 200, ExpectedContent: []string{`"distanceKm"`, `"la-bella-nonna"`, `"friterie-du-beffroi"`},
			TestAppFactory: factory,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				var out struct {
					Items []struct {
						Slug string  `json:"slug"`
						Dist float64 `json:"distanceKm"`
					} `json:"items"`
				}
				if err := jsonDecode(res, &out); err != nil {
					t.Fatal(err)
				}
				if len(out.Items) < 8 {
					t.Fatalf("expected the seeded restaurants, got %d", len(out.Items))
				}
				for i := 1; i < len(out.Items); i++ {
					if out.Items[i].Dist < out.Items[i-1].Dist {
						t.Fatal("not sorted by distance")
					}
					if out.Items[i].Dist > 3.5 {
						t.Fatalf("%s too far: %f", out.Items[i].Slug, out.Items[i].Dist)
					}
				}
			},
		},
		{
			Name: "nearby cuisine filter (accent insensitive)", Method: http.MethodGet,
			URL:            "/api/occ/restaurants/nearby?lat=50.454&lng=3.952&cuisine=thai",
			ExpectedStatus: 200, ExpectedContent: []string{`"petit-bangkok"`}, NotExpectedContent: []string{`"la-bella-nonna"`},
			TestAppFactory: factory,
		},
		{
			Name: "nearby text search", Method: http.MethodGet,
			URL:            "/api/occ/restaurants/nearby?lat=50.454&lng=3.952&q=libanaise",
			ExpectedStatus: 200, ExpectedContent: []string{`"les-jardins-du-cedre"`}, NotExpectedContent: []string{`"maison-hanami"`},
			TestAppFactory: factory,
		},
		{
			Name: "nearby out of radius", Method: http.MethodGet,
			URL:            "/api/occ/restaurants/nearby?lat=50.85&lng=4.35&radiusKm=5",
			ExpectedStatus: 200, ExpectedContent: []string{`"items":[]`},
			TestAppFactory: factory,
		},
		{
			Name: "join requires auth", Method: http.MethodPost, URL: "/api/occ/parties/join",
			Body: strings.NewReader(`{"code":"ABCDEF"}`), ExpectedStatus: 401, ExpectedContent: []string{`"status":401`},
			TestAppFactory: factory,
		},
		{
			Name: "admin import requires superuser", Method: http.MethodPost, URL: "/api/occ/admin/import",
			Body: strings.NewReader(`{"slug":"x","name":"X"}`), ExpectedStatus: 401, ExpectedContent: []string{`"status":401`},
			TestAppFactory: factory,
		},
	}
	for _, s := range scenarios {
		s.Test(t)
	}
}

func TestAdminImport(t *testing.T) {
	e := newEnv(t)
	col, _ := e.app.FindCollectionByNameOrId("_superusers")
	su := newRecord(col)
	su.SetEmail("admin@example.com")
	su.SetPassword("password123456")
	if err := e.app.Save(su); err != nil {
		t.Fatal(err)
	}
	tok, _ := su.NewAuthToken()

	body := map[string]any{
		"slug": "chez-test", "name": "Chez Test", "lat": 50.45, "lng": 3.95, "delivery_fee": 250,
		"categories": []map[string]any{{"name": "Plats", "items": []map[string]any{
			{"name": "Plat 1", "price": 1000},
			{"name": "Plat 2", "price": 1100, "option_groups": []map[string]any{{"id": "s", "name": "Sauce", "min": 0, "max": 1, "choices": []map[string]any{{"id": "a", "name": "A", "price": 0}}}}},
		}}},
	}
	r := e.expect(200, "POST", "/api/occ/admin/import", tok, body).m(t)
	if r["items"] != float64(2) {
		t.Fatalf("import: %v", r)
	}
	// upsert: same slug, menu replaced
	body["categories"] = []map[string]any{{"name": "Plats", "items": []map[string]any{{"name": "Plat 3", "price": 900}}}}
	r2 := e.expect(200, "POST", "/api/occ/admin/import", tok, body).m(t)
	if r2["restaurant"] != r["restaurant"] || r2["items"] != float64(1) {
		t.Fatalf("upsert: %v vs %v", r2, r)
	}
	if n, _ := e.app.CountRecords(colMenuItems, dbx.HashExp{"restaurant": r["restaurant"]}); n != 1 {
		t.Fatalf("menu not replaced: %d", n)
	}
	e.expect(400, "POST", "/api/occ/admin/import", tok, map[string]any{"slug": "Bad Slug", "name": "x"})
	e.expect(400, "POST", "/api/occ/admin/import", tok, map[string]any{"slug": "ok", "name": "x",
		"categories": []map[string]any{{"name": "c", "items": []map[string]any{{"name": "i", "price": 100,
			"option_groups": []map[string]any{{"id": "g", "name": "G", "min": 2, "max": 1, "choices": []map[string]any{{"id": "a"}}}}}}}}})
	alice := e.user("Alice")
	e.expect(403, "POST", "/api/occ/admin/import", alice.token, body)
}
