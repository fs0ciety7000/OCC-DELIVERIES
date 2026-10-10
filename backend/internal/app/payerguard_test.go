package app

import (
	"strings"
	"testing"
)

type readinessResp struct {
	Members []struct {
		User   string `json:"user"`
		Guest  bool   `json:"guest"`
		Payout struct {
			Ready bool     `json:"ready"`
			IBAN  bool     `json:"iban"`
			Links []string `json:"links"`
		} `json:"payout"`
	} `json:"members"`
	ReadyCount int `json:"readyCount"`
	Total      int `json:"total"`
}

type payerErr struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Payer struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"payer"`
	} `json:"data"`
}

// reviewParty: alice hosts, bob and carol join, all three order, review.
func reviewParty(t *testing.T, e *env) (pid string, alice, bob, carol user) {
	t.Helper()
	pid, code, pizza, _, alice, bob, carol := setupParty(t, e)
	e.expect(200, "POST", "/api/occ/parties/join", carol.token, map[string]any{"code": code})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	tira := e.menuItem(pizza, "Tiramisu")
	for _, u := range []user{alice, bob, carol} {
		e.expect(200, "POST", "/api/collections/order_items/records", u.token, map[string]any{"party": pid, "user": u.id(), "menu_item": tira, "quantity": 1})
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})
	return pid, alice, bob, carol
}

func TestPayerGuard(t *testing.T) {
	e := newEnv(t)
	pid, alice, bob, carol := reviewParty(t, e)
	dave := e.user("Dave")
	payer := path("/api/occ/parties/%s/payer", pid)
	readiness := path("/api/occ/parties/%s/payout-readiness", pid)

	// non-member: forbidden; anonymous: 401
	e.expect(403, "GET", readiness, dave.token, nil)
	e.expect(401, "GET", readiness, "", nil)

	var rd readinessResp
	e.expect(200, "GET", readiness, bob.token, nil).json(t, &rd)
	if rd.Total != 3 || rd.ReadyCount != 0 || len(rd.Members) != 3 {
		t.Fatalf("readiness: %+v", rd)
	}

	// blocked: self (transfer by default) then someone else
	var pe payerErr
	e.expect(409, "POST", payer, alice.token, map[string]any{"payer": alice.id()}).json(t, &pe)
	if pe.Data.Payer.Code != "payer_no_payout" || pe.Message != "Ajoute ton IBAN (ou Revolut / PayPal) dans ton profil avant de valider la commande." {
		t.Fatalf("self: %+v", pe)
	}
	e.expect(409, "POST", payer, alice.token, map[string]any{"payer": bob.id(), "collectMode": "transfer"}).json(t, &pe)
	if pe.Data.Payer.Code != "payer_no_payout" || pe.Message != "Bob n'a encore renseigné aucun moyen de remboursement." {
		t.Fatalf("other: %+v", pe)
	}
	e.expect(400, "POST", payer, alice.token, map[string]any{"payer": bob.id(), "collectMode": "wero"})
	// nothing happened
	if p, _ := e.app.FindRecordById(colParties, pid); p.GetString("status") != "review" || p.GetString("payer") != "" {
		t.Fatalf("party changed: %s %s", p.GetString("status"), p.GetString("payer"))
	}

	// allowed with Revolut only
	e.expect(200, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{"user": bob.id(), "revolut_tag": "@bobm"})
	r := e.expect(200, "POST", payer, alice.token, map[string]any{"payer": bob.id()}).m(t)
	if party := r["party"].(map[string]any); party["status"] != "paying" || party["collect_mode"] != "transfer" {
		t.Fatalf("party: %v", party)
	}

	// changing the payer while paying is guarded too; then allowed with an IBAN
	e.expect(409, "POST", payer, alice.token, map[string]any{"payer": carol.id()})
	e.expect(200, "POST", "/api/collections/payout_profiles/records", carol.token, map[string]any{"user": carol.id(), "holder_name": "Carol Secret", "iban": "BE71 0961 2345 6769"})
	raw := e.expect(200, "GET", readiness, alice.token, nil)
	raw.json(t, &rd)
	for _, leak := range []string{"BE71", "096123456769", "bobm", "Carol Secret", "holder"} {
		if strings.Contains(string(raw.body), leak) {
			t.Fatalf("readiness leaks %q: %s", leak, raw.body)
		}
	}
	got := map[string]string{}
	for _, m := range rd.Members {
		got[m.User] = strings.Join(m.Payout.Links, ",")
		if m.Payout.IBAN {
			got[m.User] += "+iban"
		}
		if m.Payout.Links == nil {
			t.Fatalf("links must be [] not null: %s", raw.body)
		}
	}
	if rd.ReadyCount != 2 || got[alice.id()] != "" || got[bob.id()] != "revolut" || got[carol.id()] != "+iban" {
		t.Fatalf("readiness after: %+v / %v", rd, got)
	}
	e.expect(200, "POST", payer, alice.token, map[string]any{"payer": carol.id()})

	// the payer cannot be set around the endpoint
	e.expect(400, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"payer": alice.id()})
	e.expect(400, "PATCH", "/api/collections/parties/records/"+pid, alice.token, map[string]any{"collect_mode": "cash"})
}

func TestPayerGuardCashMode(t *testing.T) {
	e := newOrderMailEnv(t, true)
	pid, alice, bob, _ := reviewParty(t, e.env)
	payer := path("/api/occ/parties/%s/payer", pid)

	// cash: allowed with an empty profile
	var res struct {
		Party struct {
			Status      string `json:"status"`
			CollectMode string `json:"collect_mode"`
		} `json:"party"`
		Payments []struct {
			ID     string `json:"id"`
			Debtor string `json:"debtor"`
		} `json:"payments"`
	}
	e.expect(200, "POST", payer, alice.token, map[string]any{"payer": alice.id(), "collectMode": "cash"}).json(t, &res)
	if res.Party.Status != "paying" || res.Party.CollectMode != "cash" {
		t.Fatalf("cash: %+v", res.Party)
	}
	var bobPay string
	for _, p := range res.Payments {
		if p.Debtor == bob.id() {
			bobPay = p.ID
		}
	}

	// the debtor only gets cash / later, and cannot declare anything else
	var qr struct {
		CollectMode string   `json:"collectMode"`
		EPC         *string  `json:"epc"`
		Links       []any    `json:"links"`
		Methods     []string `json:"methods"`
	}
	e.expect(200, "GET", path("/api/occ/payments/%s/qr", bobPay), bob.token, nil).json(t, &qr)
	if qr.CollectMode != "cash" || qr.EPC != nil || len(qr.Links) != 0 || strings.Join(qr.Methods, ",") != "cash,later" {
		t.Fatalf("qr: %+v", qr)
	}
	for _, m := range []string{"qr", "revolut", "paypal", "link"} {
		r := e.expect(400, "POST", path("/api/occ/payments/%s/action", bobPay), bob.token, map[string]any{"action": "declare", "method": m})
		if !strings.Contains(string(r.body), "espèces") {
			t.Fatalf("declare %s: %s", m, r.body)
		}
	}
	e.expect(200, "POST", path("/api/occ/payments/%s/action", bobPay), bob.token, map[string]any{"action": "declare", "method": "later"})
	e.expect(200, "POST", path("/api/occ/payments/%s/action", bobPay), bob.token, map[string]any{"action": "declare", "method": "cash"})

	// « Encaisser »: amounts, no QR
	var col struct {
		CollectMode string `json:"collectMode"`
		Items       []struct {
			EPC   *string `json:"epc"`
			Links []any   `json:"links"`
		} `json:"items"`
	}
	e.expect(200, "GET", path("/api/occ/parties/%s/payments/qr", pid), alice.token, nil).json(t, &col)
	if col.CollectMode != "cash" || len(col.Items) != 2 || col.Items[0].EPC != nil || len(col.Items[0].Links) != 0 {
		t.Fatalf("collect: %+v", col)
	}

	// order e-mail: « à rembourser en espèces à Alice »
	e.h.orderMail.wait()
	found := false
	for _, m := range e.app.TestMailer.Messages() {
		if m.To[0].Address == "bob@example.com" {
			found = true
			if !strings.Contains(m.HTML, "Montant à rembourser en espèces à") || !strings.Contains(m.Text, "EN ESPÈCES À ALICE") {
				t.Fatalf("bob mail: %s", m.Text)
			}
		}
	}
	if !found {
		t.Fatal("no mail for bob")
	}
}

func TestPayerGuardSoloNeedsNothing(t *testing.T) {
	e := newEnv(t)
	pizza, _ := e.testRestaurants()
	dave := e.user("Dave")
	p := e.expect(200, "POST", "/api/collections/parties/records", dave.token, map[string]any{"title": "Solo"}).m(t)
	pid := p["id"].(string)
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), dave.token, map[string]any{"to": "ordering", "restaurant": pizza})
	e.expect(200, "POST", "/api/collections/order_items/records", dave.token, map[string]any{"party": pid, "user": dave.id(), "menu_item": e.menuItem(pizza, "Tiramisu")})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), dave.token, map[string]any{"to": "review"})
	// nobody owes the payer anything: transfer mode without profile is fine
	r := e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), dave.token, map[string]any{"payer": dave.id()}).m(t)
	if r["party"].(map[string]any)["status"] != "closed" {
		t.Fatalf("solo: %v", r["party"])
	}
}

func TestPayoutRequest(t *testing.T) {
	pe := newPushEnv(t, nil)
	pid, alice, bob, carol := reviewParty(t, pe.env)
	dave := pe.user("Dave")
	ask := path("/api/occ/parties/%s/payout-request", pid)
	pe.subscribe(carol, "carol")
	pe.expect(200, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{"user": bob.id(), "iban": "BE71096123456769"})
	pe.flush()

	pe.expect(403, "POST", ask, dave.token, map[string]any{"user": carol.id()})
	pe.expect(400, "POST", ask, alice.token, map[string]any{"user": alice.id()}) // self
	pe.expect(400, "POST", ask, alice.token, map[string]any{"user": dave.id()})  // not a member
	if r := pe.expect(400, "POST", ask, alice.token, map[string]any{"user": bob.id()}); !strings.Contains(string(r.body), "déjà renseigné") {
		t.Fatalf("bob ready: %s", r.body)
	}

	pe.expect(200, "POST", ask, alice.token, map[string]any{"user": carol.id()})
	pe.h.push.wait()
	l := pe.rec.last()
	if !strings.HasSuffix(l.endpoint, "/carol") || l.payload.Kind != "payout_request" || l.payload.Title != "Ajoute ton IBAN 💳" ||
		l.payload.URL != "/party/"+pid+"?iban=1" || !strings.HasPrefix(l.payload.Body, "Alice aimerait que tu puisses avancer") {
		t.Fatalf("push: %+v", l)
	}
	pe.rec.take()
	// once per 10 minutes per party and target (bob may not re-ask either)
	pe.expect(429, "POST", ask, alice.token, map[string]any{"user": carol.id()})
	pe.expect(429, "POST", ask, bob.token, map[string]any{"user": carol.id()})
	pe.clock.set(pe.clock.now().Add(11 * 60e9))
	pe.expect(200, "POST", ask, bob.token, map[string]any{"user": carol.id()})

	// muted « payments » category: no push (the in-app toast still goes)
	pe.expect(200, "PATCH", "/api/occ/push/prefs", carol.token, map[string]any{"payments": false})
	pe.clock.set(pe.clock.now().Add(11 * 60e9))
	pe.flush()
	pe.expect(200, "POST", ask, alice.token, map[string]any{"user": carol.id()})
	if got := pe.flush(); len(got) != 0 {
		t.Fatalf("muted: %v", got)
	}
}
