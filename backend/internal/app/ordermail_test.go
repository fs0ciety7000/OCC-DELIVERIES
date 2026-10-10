package app

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

type orderMailEnv struct {
	*env
	h *handlers
}

func newOrderMailEnv(t *testing.T, smtp bool) *orderMailEnv {
	t.Helper()
	ta, err := tests.NewTestApp(templateDir)
	if err != nil {
		t.Fatal(err)
	}
	h := register(ta, testConfig)
	t.Cleanup(ta.Cleanup)
	e := &orderMailEnv{env: serveEnv(t, ta), h: h}
	if smtp {
		enableTestSMTP(e.env)
	}
	return e
}

// orderUntilReview: party at pizza, each user orders the given items
// (menu item name → quantity), then the host moves to review.
func (e *orderMailEnv) orderUntilReview(title string, host user, orders map[*user][]map[string]any, others ...user) (pid string, pizza string) {
	t := e.t
	t.Helper()
	pizza, _ = e.testRestaurants()
	party := e.expect(200, "POST", "/api/collections/parties/records", host.token, map[string]any{"title": title, "delivery_address": "Rue de Nimy 7, 7000 Mons"}).m(t)
	pid, code := party["id"].(string), party["code"].(string)
	joined := map[string]bool{host.id(): true}
	for u := range orders {
		if !joined[u.id()] {
			e.expect(200, "POST", "/api/occ/parties/join", u.token, map[string]any{"code": code})
			joined[u.id()] = true
		}
	}
	for _, u := range others {
		e.expect(200, "POST", "/api/occ/parties/join", u.token, map[string]any{"code": code})
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), host.token, map[string]any{"to": "ordering", "restaurant": pizza})
	for u, items := range orders {
		for _, it := range items {
			body := map[string]any{"party": pid, "user": u.id()}
			for k, v := range it {
				if k == "item" {
					body["menu_item"] = e.menuItem(pizza, v.(string))
					continue
				}
				body[k] = v
			}
			e.expect(200, "POST", "/api/collections/order_items/records", u.token, body)
		}
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), host.token, map[string]any{"to": "review"})
	return pid, pizza
}

func (e *orderMailEnv) summary(pid string, u user) domain.Summary {
	var s domain.Summary
	e.expect(200, "GET", path("/api/occ/parties/%s/summary", pid), u.token, nil).json(e.t, &s)
	return s
}

func participant(s domain.Summary, id string) domain.Participant {
	for _, p := range s.Participants {
		if p.User.ID == id {
			return p
		}
	}
	return domain.Participant{}
}

// personalBlock is the highlighted « Ta commande » part of the HTML body.
func personalBlock(t *testing.T, html string) string {
	t.Helper()
	i, j := strings.Index(html, "Ta commande"), strings.Index(html, "Commande complète")
	if i < 0 || j < i {
		t.Fatalf("no personal block in %s", html)
	}
	return html[i:j]
}

func TestOrderMailOnPaying(t *testing.T) {
	e := newOrderMailEnv(t, true)
	alice, bob, carol, dave, erin := e.user("Alice"), e.user("Bob"), e.user("Carol"), e.user("Dave"), e.user("Erin")
	// Erin opts out of the e-mail
	if p := e.expect(200, "PATCH", "/api/occ/push/prefs", erin.token, map[string]any{"emails": false}).m(t)["prefs"].(map[string]any); p["emails"] != false || p["party"] != true {
		t.Fatalf("prefs: %v", p)
	}
	if p := e.expect(200, "GET", "/api/occ/push/prefs", alice.token, nil).m(t); p["mail"] != true || p["prefs"].(map[string]any)["emails"] != true {
		t.Fatalf("get prefs: %v", p)
	}

	pid, _ := e.orderUntilReview("Midi <b>& co", alice, map[*user][]map[string]any{
		&alice: {{"item": "Tiramisu", "quantity": 1}},
		&bob:   {{"item": "Margherita", "quantity": 1, "selected_options": []map[string]any{{"group": "size", "choices": []string{"l"}}, {"group": "extras", "choices": []string{"cheese"}}}}},
		&carol: {{"item": "Tiramisu", "quantity": 2, "note": "sans cacao <svp>"}},
		&erin:  {{"item": "Tiramisu", "quantity": 1}},
	}, dave)
	if e.app.TestMailer.TotalSend() != 0 {
		t.Fatalf("mails before paying: %d", e.app.TestMailer.TotalSend())
	}

	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": bob.id()})
	e.h.orderMail.wait()
	s := e.summary(pid, alice)

	msgs := e.app.TestMailer.Messages()
	byTo := map[string]int{}
	for _, m := range msgs {
		byTo[m.To[0].Address]++
	}
	if len(msgs) != 3 || byTo["alice@example.com"] != 1 || byTo["bob@example.com"] != 1 || byTo["carol@example.com"] != 1 {
		t.Fatalf("recipients (no Dave: nothing ordered, no Erin: opted out): %v", byTo)
	}

	for _, m := range msgs {
		if m.Subject != "Bon de commande — Midi <b>& co (Test Pizza)" {
			t.Fatalf("subject: %q", m.Subject)
		}
		if strings.Contains(m.HTML, "<b>&") || !strings.Contains(m.HTML, "Midi &lt;b&gt;&amp; co") {
			t.Fatal("title not escaped")
		}
		if !strings.Contains(m.HTML, "/party/"+pid) || !strings.Contains(m.Text, "/party/"+pid) {
			t.Fatal("party link missing")
		}
		if strings.Contains(m.HTML, "<svp>") || !strings.Contains(m.HTML, "sans cacao &lt;svp&gt;") {
			t.Fatal("note not escaped in the full order")
		}
		// full order: every person who ordered, grand total, restaurant details
		for _, want := range []string{"Alice", "Bob", "Carol", "Erin", "Commande complète", eurHTML(s.GrandTotal), "65 00 00 00", "Rue de Nimy 7, 7000 Mons"} {
			if !strings.Contains(m.HTML, want) {
				t.Fatalf("%s: %q missing", m.To[0].Address, want)
			}
		}
		if strings.Contains(m.HTML, "Dave") || strings.Contains(m.HTML, "{{") {
			t.Fatalf("%s: Dave (no items) or raw template in mail", m.To[0].Address)
		}
	}

	for _, m := range msgs {
		block := personalBlock(t, m.HTML)
		switch m.To[0].Address {
		case "carol@example.com":
			c := participant(s, carol.id())
			if !strings.Contains(block, "2&nbsp;×") || !strings.Contains(block, "Tiramisu") || strings.Contains(block, "Margherita") {
				t.Fatalf("carol block: %s", block)
			}
			if !strings.Contains(block, "Montant à rembourser à <strong") || !strings.Contains(block, eurHTML(c.Total)) || !strings.Contains(block, eurHTML(c.SharedFees)) || !strings.Contains(block, eurHTML(1200)) {
				t.Fatalf("carol amounts (%d): %s", c.Total, block)
			}
			if !strings.Contains(m.Text, "MONTANT À REMBOURSER À BOB : "+domain.FormatEUR(c.Total)) {
				t.Fatalf("carol text: %s", m.Text)
			}
			if !strings.Contains(m.HTML, "Ta part : "+domain.FormatEUR(c.Total)+" à rembourser à Bob.") {
				t.Fatal("carol preheader")
			}
		case "alice@example.com":
			a := participant(s, alice.id())
			if !strings.Contains(block, "Tiramisu") || strings.Contains(block, "Margherita") || !strings.Contains(block, eurHTML(a.Total)) {
				t.Fatalf("alice block: %s", block)
			}
		case "bob@example.com":
			b := participant(s, bob.id())
			owed := s.GrandTotal - b.Total
			if !strings.Contains(block, "Margherita") || !strings.Contains(block, "Large") || strings.Contains(block, "Tiramisu") {
				t.Fatalf("bob block: %s", block)
			}
			if !strings.Contains(block, "Tu as avancé") || !strings.Contains(block, eurHTML(s.GrandTotal)) ||
				!strings.Contains(block, "3 collègues te doivent "+domain.FormatEUR(owed)+".") || strings.Contains(block, "Montant à rembourser") {
				t.Fatalf("bob payer block (owed %d): %s", owed, block)
			}
			if !strings.Contains(m.Text, "Tu as avancé "+domain.FormatEUR(s.GrandTotal)) {
				t.Fatalf("bob text: %s", m.Text)
			}
		}
	}

	// never resent: payer changed again, party updated, closed
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": alice.id()})
	e.expect(200, "POST", path("/api/occ/parties/%s/dispatch", pid), alice.token, map[string]any{"method": "phone"})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "closed"})
	e.h.orderMail.wait()
	if n := e.app.TestMailer.TotalSend(); n != 3 {
		t.Fatalf("resent: %d mails", n)
	}
	p, _ := e.app.FindRecordById(colParties, pid)
	if p.GetDateTime(colPartiesOrderMailField).IsZero() {
		t.Fatal("order_mail_sent_at not set")
	}
	if e.h.orderMail.claim(pid) {
		t.Fatal("claimed twice")
	}
	// hidden: never exposed to the client
	if r := e.expect(200, "GET", "/api/collections/parties/records/"+pid, alice.token, nil); strings.Contains(string(r.body), colPartiesOrderMailField) {
		t.Fatal("order_mail_sent_at exposed")
	}
}

func TestOrderMailGuestsAndSolo(t *testing.T) {
	e := newOrderMailEnv(t, true)
	alice, bob := e.user("Alice"), e.user("Bob")
	pizza, _ := e.testRestaurants()
	party := e.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{"title": "Midi"}).m(t)
	pid, code := party["id"].(string), party["code"].(string)
	lea, _ := e.guest("Léa", map[string]any{"partyCode": code})
	e.expect(200, "POST", "/api/occ/parties/join", bob.token, map[string]any{"code": code})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	tiramisu := e.menuItem(pizza, "Tiramisu")
	for _, u := range []user{alice, lea, bob} {
		e.expect(200, "POST", "/api/collections/order_items/records", u.token, map[string]any{"party": pid, "user": u.id(), "menu_item": tiramisu, "quantity": 1})
	}
	// Bob is suspended before the order is validated
	bob.rec = e.reload(bob.id())
	bob.rec.Set("banned", true)
	if err := e.app.Save(bob.rec); err != nil {
		t.Fatal(err)
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": alice.id()})
	e.h.orderMail.wait()
	msgs := e.app.TestMailer.Messages()
	if len(msgs) != 1 || msgs[0].To[0].Address != "alice@example.com" {
		t.Fatalf("only Alice (Léa is a guest, Bob is suspended): %d mails", len(msgs))
	}
	s := e.summary(pid, alice)
	owed := participant(s, lea.id()).Total + participant(s, bob.id()).Total
	if !strings.Contains(msgs[0].HTML, "2 collègues te doivent "+domain.FormatEUR(owed)+".") {
		t.Fatalf("payer line: %s", personalBlock(t, msgs[0].HTML))
	}

	// solo: only the payer ordered → closed in the same transaction, one mail
	party = e.expect(200, "POST", "/api/collections/parties/records", alice.token, map[string]any{"title": "Solo"}).m(t)
	solo := party["id"].(string)
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", solo), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	e.expect(200, "POST", "/api/collections/order_items/records", alice.token, map[string]any{"party": solo, "user": alice.id(), "menu_item": tiramisu, "quantity": 1})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", solo), alice.token, map[string]any{"to": "review"})
	r := e.expect(200, "POST", path("/api/occ/parties/%s/payer", solo), alice.token, map[string]any{"payer": alice.id()}).m(t)
	if r["party"].(map[string]any)["status"] != "closed" {
		t.Fatalf("solo not closed: %v", r["party"])
	}
	e.h.orderMail.wait()
	msgs = e.app.TestMailer.Messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1].Subject, "Solo") || !strings.Contains(msgs[1].HTML, "Personne ne te doit rien") {
		t.Fatalf("solo: %d mails", len(msgs))
	}
}

func TestOrderMailWithoutSMTP(t *testing.T) {
	e := newOrderMailEnv(t, false)
	alice, bob := e.user("Alice"), e.user("Bob")
	pid, _ := e.orderUntilReview("Midi", alice, map[*user][]map[string]any{
		&alice: {{"item": "Tiramisu", "quantity": 1}},
		&bob:   {{"item": "Tiramisu", "quantity": 1}},
	})
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": bob.id()})
	e.h.orderMail.wait()
	if n := e.app.TestMailer.TotalSend(); n != 0 {
		t.Fatalf("mails without SMTP: %d", n)
	}
	if p, _ := e.app.FindRecordById(colParties, pid); !p.GetDateTime(colPartiesOrderMailField).IsZero() {
		t.Fatal("claimed without SMTP")
	}
}

// ------------------------------------------------------------- rendering

func sampleOrderMail() orderMailData {
	items := func(name string, qty, unit int, opts, note string) domain.SummaryItem {
		return domain.SummaryItem{ID: name, Name: name, Quantity: qty, UnitPrice: unit, Total: qty * unit, OptionsLabel: opts, Note: note}
	}
	d := orderMailData{
		PartyID: "p1", Title: "Midi \"du\" <vendredi>", Code: "ABC234", Restaurant: "Pizza & Co",
		RestaurantPhone: "+32 65 00 00 00", RestaurantAddress: "Rue X 12, 7000 Mons", Provider: "Uber Eats",
		SplitEqual: true, PayerID: "bob", PayerName: "Bob",
		People: []orderMailPerson{
			{ID: "alice", Name: "Alice", Items: []domain.SummaryItem{items("Margherita", 1, 1000, "Large, Fromage", "")}, Subtotal: 1000, SharedFees: 240, Total: 1240},
			{ID: "bob", Name: "Bob", Items: []domain.SummaryItem{items("Calzone", 2, 1200, "", "bien cuite")}, Subtotal: 2400, SharedFees: 240, Total: 2640, IsPayer: true},
		},
		ItemsSubtotal: 3400, DeliveryFee: 299, ServiceFee: 81, Tip: 100, SharedFees: 480, GrandTotal: 3880,
		owedBy: map[string]int{"alice": 1240}, OwedToPayer: 1240, Debtors: 1,
	}
	d.Advanced = d.GrandTotal
	return d
}

func TestRenderOrderMail(t *testing.T) {
	base := sampleOrderMail()

	d := base.forRecipient("alice", "https://eat.example")
	if d.PartyURL != "https://eat.example/party/p1" || d.Owed != 1240 || d.IsPayer || d.Me.Name != "Alice" {
		t.Fatalf("recipient data: %+v", d)
	}
	if got := orderMailSubject(d); got != `Bon de commande — Midi "du" <vendredi> (Pizza & Co)` {
		t.Fatalf("subject: %q", got)
	}
	h, err := renderOrderMailHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Ta part : 12,40 € à rembourser à Bob.",                // preheader
		"Midi &#34;du&#34; &lt;vendredi&gt;",                   // escaped title
		"Pizza &amp; Co · code ABC234 · envoyée via Uber Eats", // meta line
		"Montant à rembourser à <strong style=\"color:#17151A\">Bob</strong>",
		"12,40 €", "10,00 €", "2,40 €", "38,80 €", "2,99 €", "0,81 €", "1,00 €",
		`href="https://eat.example/party/p1"`, "Rembourser Bob",
		"Alice <span style=\"font-weight:400;color:#5F5B66\">(toi)</span>",
		"Bob <span style=\"font-size:12px;font-weight:700;color:#B93A17\">· a payé</span>",
		"Tél. &#43;32 65 00 00 00", "Rue X 12, 7000 Mons", "« bien cuite »",
		`href="https://eat.example"`, "Développé par OCC MONS Studios",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("html: %q missing", want)
		}
	}
	block := personalBlock(t, h)
	if !strings.Contains(block, "Margherita") || !strings.Contains(block, "Large, Fromage") || strings.Contains(block, "Calzone") {
		t.Errorf("alice block: %s", block)
	}
	if strings.Contains(h, "[soft]") || strings.Contains(h, "<vendredi>") {
		t.Error("unreplaced token or unescaped text")
	}

	txt := renderOrderMailText(d)
	for _, want := range []string{
		`BON DE COMMANDE — Midi "du" <vendredi>`, "1 × Margherita (Large, Fromage) — 10,00 €",
		"Ma part des frais : 2,40 €", "MONTANT À REMBOURSER À BOB : 12,40 €", "https://eat.example/party/p1",
		"Bob · a payé", "  2 × Calzone — 24,00 €", "     « bien cuite »", "Total de la commande : 38,80 €",
		"Livraison : 2,99 €", "Pourboire : 1,00 €",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("text: %q missing in\n%s", want, txt)
		}
	}

	// payer variant
	p := base.forRecipient("bob", "https://eat.example")
	h, err = renderOrderMailHTML(p)
	if err != nil {
		t.Fatal(err)
	}
	block = personalBlock(t, h)
	for _, want := range []string{"Tu as avancé", "38,80 €", "1 collègue te doit 12,40 €.", "Calzone"} {
		if !strings.Contains(block, want) {
			t.Errorf("payer block: %q missing", want)
		}
	}
	if strings.Contains(block, "Montant à rembourser") || !strings.Contains(h, "Suivre les remboursements") ||
		!strings.Contains(h, "Tu as avancé 38,80 € ; 1 collègue te doit 12,40 €.") {
		t.Errorf("payer html: %s", block)
	}
	p.Debtors, p.OwedToPayer = 2, 2480
	if got := p.payerLine(); got != "2 collègues te doivent 24,80 €." {
		t.Errorf("payer line: %q", got)
	}
	p.Debtors = 0
	if got := p.preheader(); got != "Tu as avancé 38,80 € ; personne ne te doit rien." {
		t.Errorf("solo preheader: %q", got)
	}
}
