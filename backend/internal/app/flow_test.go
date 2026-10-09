package app

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

type partyResp struct {
	Party struct {
		ID         string         `json:"id"`
		Code       string         `json:"code"`
		Host       string         `json:"host"`
		Members    []string       `json:"members"`
		Status     string         `json:"status"`
		Restaurant string         `json:"restaurant"`
		Payer      string         `json:"payer"`
		Provider   string         `json:"provider"`
		Dispatch   map[string]any `json:"dispatch"`
		ClosedAt   string         `json:"closed_at"`
	} `json:"party"`
}

func TestHappyPath(t *testing.T) {
	e := newEnv(t)
	pizza, burger := e.testRestaurants()
	alice, bob, carol := e.user("Alice"), e.user("Bob"), e.user("Carol")

	// --- create: forced fields are ignored --------------------------------
	r := e.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{
		"title": "Midi du vendredi", "code": "AAAAAA", "status": "closed", "host": bob.id(),
		"members": []string{bob.id(), carol.id()}, "restaurant": pizza,
	})
	party := r.m(t)
	pid := party["id"].(string)
	code := party["code"].(string)
	if party["host"] != alice.id() || party["status"] != "lobby" || code == "AAAAAA" || !domain.IsValidCode(code) || party["restaurant"] != "" {
		t.Fatalf("create did not force fields: %v", party)
	}
	if m := party["members"].([]any); len(m) != 1 || m[0] != alice.id() {
		t.Fatalf("members: %v", m)
	}
	if n, _ := e.app.CountRecords(colPartyMembers, dbx.HashExp{"party": pid, "role": "host", "user": alice.id()}); n != 1 {
		t.Fatal("host party_member not created")
	}

	// --- join by code (case/space insensitive, idempotent) ------------------
	var jr partyResp
	e.expect(200, "POST", "/api/occ/parties/join", bob.token, map[string]any{"code": " " + strings.ToLower(code) + " "}).json(t, &jr)
	if !slices.Contains(jr.Party.Members, bob.id()) {
		t.Fatalf("bob not member: %v", jr.Party.Members)
	}
	e.expect(200, "POST", "/api/occ/parties/join", bob.token, map[string]any{"code": code})
	if n, _ := e.app.CountRecords(colPartyMembers, dbx.HashExp{"party": pid}); n != 2 {
		t.Fatalf("expected 2 party members, got %d", n)
	}
	e.expect(404, "POST", "/api/occ/parties/join", carol.token, map[string]any{"code": "ZZZZZZ"})
	e.expect(400, "POST", "/api/occ/parties/join", carol.token, map[string]any{"code": "nope"})

	// --- candidates (lobby only) -------------------------------------------
	e.expect(200, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"candidates": []string{pizza, burger}})

	// --- voting ------------------------------------------------------------
	e.expect(400, "POST", "/api/collections/votes/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "restaurant": pizza})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "voting"})
	e.expect(400, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"candidates": []string{pizza}})

	e.expect(200, "POST", "/api/collections/votes/records", bob.token, map[string]any{"party": pid, "user": bob.id(), "restaurant": pizza})
	e.expect(200, "POST", "/api/collections/votes/records", alice.token, map[string]any{"party": pid, "user": alice.id(), "restaurant": pizza})
	e.expect(200, "POST", "/api/collections/votes/records", alice.token, map[string]any{"party": pid, "user": alice.id(), "restaurant": burger})
	// burger has the best rating but pizza has more votes
	var tr partyResp
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering"}).json(t, &tr)
	if tr.Party.Status != "ordering" || tr.Party.Restaurant != pizza {
		t.Fatalf("election: %+v", tr.Party)
	}

	// --- ordering: server side prices --------------------------------------
	marg := e.menuItem(pizza, "Margherita")
	tira := e.menuItem(pizza, "Tiramisu")
	r = e.expect(200, "POST", "/api/collections/order_items/records", alice.token, map[string]any{
		"party": pid, "user": alice.id(), "menu_item": marg, "quantity": 2,
		"selected_options": []map[string]any{{"group": "size", "choices": []string{"l"}}, {"group": "extras", "choices": []string{"cheese"}}},
		"unit_price":       1, "total": 1, "name": "Pirate", "options_label": "gratuit",
	})
	item := r.m(t)
	if item["unit_price"] != float64(1450) || item["total"] != float64(2900) || item["name"] != "Margherita" || item["options_label"] != "Large, Fromage" {
		t.Fatalf("server pricing not applied: %v", item)
	}
	aliceItem := item["id"].(string)

	r = e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": pid, "user": bob.id(), "menu_item": marg, "quantity": 1, "note": "sans oignon",
		"selected_options": []map[string]any{{"group": "size", "choices": []string{"l"}}},
	})
	bobItem := r.m(t)["id"].(string)

	// ready then a cart change resets it
	e.expect(200, "POST", path("/api/occ/parties/%s/ready", pid), bob.token, map[string]any{"ready": true})
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": pid, "user": bob.id(), "menu_item": tira,
	})
	pm, _ := e.app.FindFirstRecordByFilter(colPartyMembers, "party={:p} && user={:u}", dbx.Params{"p": pid, "u": bob.id()})
	if pm.GetBool("ready") {
		t.Fatal("ready should be reset after a cart change")
	}

	// invalid lines
	e.expect(400, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": pid, "user": bob.id(), "menu_item": e.menuItem(burger, "Cheeseburger"), "quantity": 1,
	})
	e.expect(400, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": pid, "user": bob.id(), "menu_item": e.menuItem(pizza, "Calzone"), "quantity": 1,
	})
	e.expect(400, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": pid, "user": bob.id(), "menu_item": marg, "quantity": 1,
	}) // missing required size
	e.expect(400, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": pid, "user": bob.id(), "menu_item": tira, "quantity": 21,
	})
	e.expect(400, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": pid, "user": bob.id(), "menu_item": tira, "note": strings.Repeat("x", 201),
	})
	// someone else's line / impersonation
	e.expect(404, "PATCH", "/api/collections/order_items/records/"+aliceItem, bob.token, map[string]any{"quantity": 5})
	e.expect(400, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": pid, "user": alice.id(), "menu_item": tira,
	})
	e.expect(400, "POST", "/api/collections/order_items/records", carol.token, map[string]any{
		"party": pid, "user": carol.id(), "menu_item": tira,
	})

	// update recomputes the total
	r = e.expect(200, "PATCH", "/api/collections/order_items/records/"+bobItem, bob.token, map[string]any{"quantity": 2, "total": 5})
	if r.m(t)["total"] != float64(2600) {
		t.Fatalf("update total: %s", r.body)
	}
	e.expect(200, "PATCH", "/api/collections/order_items/records/"+bobItem, bob.token, map[string]any{"quantity": 1})

	e.expect(200, "POST", path("/api/occ/parties/%s/ready", pid), bob.token, map[string]any{"ready": true})
	e.expect(200, "POST", path("/api/occ/parties/%s/ready", pid), alice.token, map[string]any{"ready": true})

	// --- review ------------------------------------------------------------
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})
	pr, _ := e.app.FindRecordById(colParties, pid)
	if pr.GetInt("delivery_fee") != 299 {
		t.Fatalf("delivery fee snapshot: %d", pr.GetInt("delivery_fee"))
	}
	e.expect(400, "POST", path("/api/occ/parties/%s/ready", pid), bob.token, map[string]any{"ready": false})
	e.expect(200, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"service_fee": 100})

	var s domain.Summary
	e.expect(200, "GET", path("/api/occ/parties/%s/summary", pid), bob.token, nil).json(t, &s)
	// alice 2×1450 = 2900 ; bob 1300 + 600 = 1900 ; fees 299 + 100
	if s.ItemsSubtotal != 4800 || s.SharedFees != 399 || s.GrandTotal != 5199 || !s.AllReady || !s.MinOrderReached {
		t.Fatalf("summary totals: %+v", s)
	}
	sum := 0
	for _, p := range s.Participants {
		sum += p.Total
	}
	if sum != s.GrandTotal {
		t.Fatalf("Σ participant totals %d != grandTotal %d", sum, s.GrandTotal)
	}
	if len(s.Consolidated) != 3 || s.Restaurant == nil || s.Restaurant.ID != pizza {
		t.Fatalf("consolidated: %+v", s.Consolidated)
	}
	aliceShare, _ := s.ParticipantByID(alice.id())
	bobShare, _ := s.ParticipantByID(bob.id())
	if aliceShare.SharedFees+bobShare.SharedFees != 399 || aliceShare.Subtotal != 2900 || bobShare.Subtotal != 1900 {
		t.Fatalf("shares: %+v %+v", aliceShare, bobShare)
	}

	// --- dispatch ----------------------------------------------------------
	var dr struct {
		partyResp
		Dispatch struct {
			Method       string   `json:"method"`
			URL          string   `json:"url"`
			CartText     string   `json:"cartText"`
			Instructions []string `json:"instructions"`
		} `json:"dispatch"`
	}
	e.expect(403, "POST", path("/api/occ/parties/%s/dispatch", pid), bob.token, map[string]any{"method": "ubereats"})
	e.expect(400, "POST", path("/api/occ/parties/%s/dispatch", pid), alice.token, map[string]any{"method": "glovo"})
	// weloveat is a known platform but disabled in the test config
	e.expect(400, "POST", path("/api/occ/parties/%s/dispatch", pid), alice.token, map[string]any{"method": "weloveat"})
	e.expect(200, "POST", path("/api/occ/parties/%s/dispatch", pid), alice.token, map[string]any{"method": "deliveroo"}).json(t, &dr)
	if dr.Dispatch.URL != "https://deliveroo.be/fr/" || dr.Party.Provider != "deliveroo" || dr.Party.Dispatch["method"] != "deliveroo" {
		t.Fatalf("deliveroo dispatch: %+v", dr)
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/dispatch", pid), alice.token, map[string]any{"method": "ubereats"}).json(t, &dr)
	if dr.Dispatch.URL != "https://www.ubereats.com/be/store/test-pizza" || !strings.Contains(dr.Dispatch.CartText, "2 × Margherita (Large, Fromage)") ||
		dr.Party.Provider != "ubereats" || dr.Party.Dispatch["method"] != "ubereats" {
		t.Fatalf("dispatch: %+v", dr)
	}

	// --- export ------------------------------------------------------------
	for _, f := range []string{"csv", "txt", "json"} {
		r := e.expect(200, "GET", path("/api/occ/parties/%s/export?format=%s", pid, f), bob.token, nil)
		if !strings.Contains(r.header.Get("Content-Disposition"), "attachment") || !strings.Contains(string(r.body), "Margherita") {
			t.Fatalf("export %s: %v %s", f, r.header, r.body)
		}
	}
	e.expect(400, "GET", path("/api/occ/parties/%s/export?format=pdf", pid), bob.token, nil)
	e.expect(403, "GET", path("/api/occ/parties/%s/export?format=csv", pid), carol.token, nil)

	// --- payout profile of bob (the payer) ---------------------------------
	e.expect(400, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{"user": bob.id(), "iban": "BE72 0961 2345 6769"})
	e.expect(400, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{"user": bob.id(), "bancontact_phone": "123"})
	e.expect(400, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{"user": bob.id(), "revolut_tag": "bob smith"})
	e.expect(400, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{"user": bob.id(), "payment_link": "javascript:alert(1)"})
	r = e.expect(200, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{
		"user": bob.id(), "holder_name": "Bob Martin", "iban": "be71 0961 2345 6769",
		"wero_id": "0470 12 34 56", "bancontact_phone": "0032.470.12.34.56",
		"revolut_tag": "https://revolut.me/BobM", "payment_link": "paypal.me/bob",
	})
	prof := r.m(t)
	if prof["iban"] != "BE71096123456769" || prof["wero_id"] != "+32470123456" || prof["bancontact_phone"] != "+32470123456" ||
		prof["revolut_tag"] != "bobm" || prof["paypal_me"] != "bob" || prof["payment_link"] != "" {
		t.Fatalf("profile normalization: %v", prof)
	}
	e.expect(400, "POST", "/api/collections/payout_profiles/records", carol.token, map[string]any{"user": bob.id()})

	// --- payer -------------------------------------------------------------
	var payr struct {
		partyResp
		Payments []struct {
			ID        string `json:"id"`
			Debtor    string `json:"debtor"`
			Creditor  string `json:"creditor"`
			Amount    int    `json:"amount"`
			Method    string `json:"method"`
			Status    string `json:"status"`
			Reference string `json:"reference"`
		} `json:"payments"`
	}
	e.expect(403, "POST", path("/api/occ/parties/%s/payer", pid), bob.token, map[string]any{"payer": bob.id()})
	e.expect(400, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": carol.id()})
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": bob.id()}).json(t, &payr)
	if payr.Party.Status != "paying" || payr.Party.Payer != bob.id() || len(payr.Payments) != 2 {
		t.Fatalf("payer: %+v", payr)
	}
	var alicePay string
	total := 0
	for _, p := range payr.Payments {
		total += p.Amount
		switch p.Debtor {
		case alice.id():
			alicePay = p.ID
			if p.Status != "pending" || p.Amount != aliceShare.Total || p.Reference != "OCC "+code+" Alice" || p.Creditor != bob.id() {
				t.Fatalf("alice payment: %+v", p)
			}
		case bob.id():
			if p.Method != "self" || p.Status != "confirmed" || p.Amount != bobShare.Total {
				t.Fatalf("bob payment: %+v", p)
			}
		}
	}
	if total != s.GrandTotal {
		t.Fatalf("payments sum %d != %d", total, s.GrandTotal)
	}
	// fees frozen once paying
	e.expect(400, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"tip": 500})
	// recompute allowed while no third-party payment is confirmed
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": bob.id()}).json(t, &payr)
	for _, p := range payr.Payments {
		if p.Debtor == alice.id() {
			alicePay = p.ID
		}
	}

	// --- payment QR --------------------------------------------------------
	profRec, _ := e.app.FindFirstRecordByData(colPayoutProfiles, "user", bob.id())
	e.attachFile(profRec, "wero_qr", "wero.png", pngBytes(t))

	var qr struct {
		Amount      int                  `json:"amount"`
		Reference   string               `json:"reference"`
		Beneficiary string               `json:"beneficiary"`
		EPC         *string              `json:"epc"`
		IBAN        *string              `json:"iban"`
		Links       []domain.PaymentLink `json:"links"`
		Wero        *struct {
			ID    string `json:"id"`
			HasQR bool   `json:"hasQr"`
		} `json:"wero"`
		Bancontact *struct {
			Phone string `json:"phone"`
			HasQR bool   `json:"hasQr"`
		} `json:"bancontact"`
		Methods []string `json:"methods"`
	}
	e.expect(200, "GET", path("/api/occ/payments/%s/qr", alicePay), alice.token, nil).json(t, &qr)
	wantEPC := "BCD\n002\n1\nSCT\n\nBob Martin\nBE71096123456769\nEUR" + domain.FormatAmount(aliceShare.Total) + "\n\n\nOCC " + code + " Alice"
	if qr.EPC == nil || *qr.EPC != wantEPC || qr.Beneficiary != "Bob Martin" || qr.Amount != aliceShare.Total {
		t.Fatalf("qr: %+v", qr)
	}
	if qr.IBAN == nil || *qr.IBAN != "BE71096123456769" {
		t.Fatalf("iban: %v", qr.IBAN)
	}
	wantLinks := []domain.PaymentLink{
		{Kind: "revolut", Label: "Revolut", AmountPrefilled: true,
			URL: fmt.Sprintf("https://revolut.me/bobm?amount=%d&currency=EUR&note=OCC+%s+Alice", aliceShare.Total, code)},
		{Kind: "paypal", Label: "PayPal", AmountPrefilled: true,
			URL: "https://paypal.me/bob/" + domain.FormatAmount(aliceShare.Total) + "EUR"},
	}
	if len(qr.Links) != len(wantLinks) || qr.Links[0] != wantLinks[0] || qr.Links[1] != wantLinks[1] {
		t.Fatalf("links: %+v", qr.Links)
	}
	if qr.Wero == nil || qr.Wero.ID != "+32470123456" || !qr.Wero.HasQR || qr.Bancontact == nil || qr.Bancontact.HasQR {
		t.Fatalf("wallets: %+v %+v", qr.Wero, qr.Bancontact)
	}
	if strings.Join(qr.Methods, ",") != "qr,revolut,paypal,wero,bancontact,cash,later" {
		t.Fatalf("methods: %v", qr.Methods)
	}
	e.expect(403, "GET", path("/api/occ/payments/%s/qr", alicePay), carol.token, nil)

	// legacy profile (free revolut.me link stored before the structured handles)
	if _, err := e.app.DB().Update(colPayoutProfiles, dbx.Params{"revolut_tag": "", "paypal_me": "", "payment_link": "https://revolut.me/legacy"}, dbx.HashExp{"user": bob.id()}).Execute(); err != nil {
		t.Fatal(err)
	}
	e.expect(200, "GET", path("/api/occ/payments/%s/qr", alicePay), alice.token, nil).json(t, &qr)
	if len(qr.Links) != 1 || qr.Links[0].Kind != "revolut" || !qr.Links[0].AmountPrefilled ||
		!strings.HasPrefix(qr.Links[0].URL, fmt.Sprintf("https://revolut.me/legacy?amount=%d&", aliceShare.Total)) {
		t.Fatalf("legacy links: %+v", qr.Links)
	}
	if strings.Join(qr.Methods, ",") != "qr,revolut,wero,bancontact,cash,later" {
		t.Fatalf("legacy methods: %v", qr.Methods)
	}

	// wallet QR image
	wr := e.expect(200, "GET", path("/api/occ/payments/%s/wallet-qr/wero", alicePay), alice.token, nil)
	if wr.header.Get("Content-Type") != "image/png" || wr.header.Get("Cache-Control") != "private, max-age=300" || len(wr.body) == 0 {
		t.Fatalf("wallet qr headers: %v", wr.header)
	}
	e.expect(404, "GET", path("/api/occ/payments/%s/wallet-qr/bancontact", alicePay), alice.token, nil)
	e.expect(404, "GET", path("/api/occ/payments/%s/wallet-qr/paypal", alicePay), alice.token, nil)
	e.expect(403, "GET", path("/api/occ/payments/%s/wallet-qr/wero", alicePay), carol.token, nil)
	e.expect(401, "GET", path("/api/occ/payments/%s/wallet-qr/wero", alicePay), "", nil)
	// the protected file is not reachable through the public files API
	if r := e.do("GET", path("/api/files/%s/%s/%s", colPayoutProfiles, profRec.Id, profRec.GetString("wero_qr")), alice.token, nil); r.status == 200 {
		t.Fatal("protected wallet QR must not be publicly served")
	}

	// --- declare / confirm -------------------------------------------------
	e.expect(403, "POST", path("/api/occ/payments/%s/action", alicePay), bob.token, map[string]any{"action": "declare", "method": "qr"})
	e.expect(400, "POST", path("/api/occ/payments/%s/action", alicePay), alice.token, map[string]any{"action": "declare", "method": "self"})
	e.expect(400, "POST", path("/api/occ/payments/%s/action", alicePay), alice.token, map[string]any{"action": "pay"})

	var ar struct {
		Payment map[string]any `json:"payment"`
	}
	e.expect(200, "POST", path("/api/occ/payments/%s/action", alicePay), alice.token, map[string]any{"action": "declare", "method": "later"}).json(t, &ar)
	if ar.Payment["status"] != "pending" || ar.Payment["method"] != "later" {
		t.Fatalf("declare later: %v", ar.Payment)
	}
	e.expect(200, "POST", path("/api/occ/payments/%s/action", alicePay), alice.token, map[string]any{"action": "declare", "method": "wero"}).json(t, &ar)
	if ar.Payment["status"] != "declared" || ar.Payment["method"] != "wero" || ar.Payment["declared_at"] == "" {
		t.Fatalf("declare wero: %v", ar.Payment)
	}
	e.expect(200, "POST", path("/api/occ/payments/%s/action", alicePay), bob.token, map[string]any{"action": "reset"}).json(t, &ar)
	if ar.Payment["status"] != "pending" {
		t.Fatalf("reset: %v", ar.Payment)
	}
	// new wallet methods are stored as such (payments.method select values)
	for _, m := range []string{"paypal", "revolut"} {
		e.expect(200, "POST", path("/api/occ/payments/%s/action", alicePay), alice.token, map[string]any{"action": "declare", "method": m}).json(t, &ar)
		if ar.Payment["status"] != "declared" || ar.Payment["method"] != m {
			t.Fatalf("declare %s: %v", m, ar.Payment)
		}
	}
	e.expect(200, "POST", path("/api/occ/payments/%s/action", alicePay), alice.token, map[string]any{"action": "declare", "method": "qr"})
	e.expect(200, "POST", path("/api/occ/payments/%s/action", alicePay), bob.token, map[string]any{"action": "confirm"}).json(t, &ar)
	if ar.Payment["status"] != "confirmed" {
		t.Fatalf("confirm: %v", ar.Payment)
	}

	// --- all confirmed: party closed ----------------------------------------
	pr, _ = e.app.FindRecordById(colParties, pid)
	if pr.GetString("status") != "closed" || pr.GetDateTime("closed_at").IsZero() {
		t.Fatalf("party should be closed: %s", pr.GetString("status"))
	}
	e.expect(400, "POST", path("/api/occ/payments/%s/action", alicePay), bob.token, map[string]any{"action": "reset"})
	e.expect(400, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"title": "trop tard"})
	e.expect(http.StatusOK, "GET", path("/api/occ/parties/%s/summary", pid), alice.token, nil)
}
