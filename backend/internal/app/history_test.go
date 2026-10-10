package app

import (
	"net/url"
	"testing"
)

type historyResp struct {
	Page       int `json:"page"`
	PerPage    int `json:"perPage"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
	Items      []struct {
		ID         string `json:"id"`
		Code       string `json:"code"`
		Title      string `json:"title"`
		Status     string `json:"status"`
		Created    string `json:"created"`
		Restaurant *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"restaurant"`
		Host        struct{ Name string } `json:"host"`
		IsHost      bool                  `json:"isHost"`
		MemberCount int                   `json:"memberCount"`
		Items       []struct {
			MenuItem     string `json:"menuItem"`
			Name         string `json:"name"`
			OptionsLabel string `json:"optionsLabel"`
			Quantity     int    `json:"quantity"`
			UnitPrice    int    `json:"unitPrice"`
			Total        int    `json:"total"`
		} `json:"items"`
		Subtotal   int `json:"subtotal"`
		SharedFees int `json:"sharedFees"`
		Total      int `json:"total"`
		GrandTotal int `json:"grandTotal"`
		Payer      *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"payer"`
		Payment *struct {
			Method string `json:"method"`
			Status string `json:"status"`
			Amount int    `json:"amount"`
		} `json:"payment"`
	} `json:"items"`
}

// newOrderingParty creates a party hosted by host, joined by guests, ordering at restaurant.
func (e *env) newOrderingParty(host user, restaurant, title string, guests ...user) string {
	e.t.Helper()
	p := e.expect(200, "POST", "/api/collections/parties/records", host.token, map[string]any{"title": title}).m(e.t)
	for _, g := range guests {
		e.expect(200, "POST", "/api/occ/parties/join", g.token, map[string]any{"code": p["code"]})
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", p["id"]), host.token, map[string]any{"to": "ordering", "restaurant": restaurant})
	return p["id"].(string)
}

func TestHistoryAndStats(t *testing.T) {
	e := newEnv(t)
	pizza, burger := e.testRestaurants()
	alice, bob, carol, dave := e.user("Alice"), e.user("Bob"), e.user("Carol"), e.user("Dave")
	marg, tira := e.menuItem(pizza, "Margherita"), e.menuItem(pizza, "Tiramisu")

	e.expect(401, "GET", "/api/occ/me/history", "", nil)
	e.expect(401, "GET", "/api/occ/me/stats", "", nil)

	// Party 1 : Alice (hôte) + Bob, pizza, Bob paie.
	p1 := e.newOrderingParty(alice, pizza, "Midi pizza", bob)
	e.expect(200, "POST", "/api/collections/order_items/records", alice.token, map[string]any{
		"party": p1, "user": alice.id(), "menu_item": marg, "quantity": 2,
		"selected_options": []map[string]any{{"group": "size", "choices": []string{"l"}}, {"group": "extras", "choices": []string{"cheese"}}},
	}) // 2 × 14,50
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": p1, "user": bob.id(), "menu_item": marg, "selected_options": []map[string]any{{"group": "size", "choices": []string{"l"}}},
	}) // 13,00
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{"party": p1, "user": bob.id(), "menu_item": tira}) // 6,00
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", p1), alice.token, map[string]any{"to": "review"})
	e.expect(200, "PATCH", "/api/collections/parties/records/"+p1, alice.token, map[string]any{"tip": 100})
	e.payout(bob)
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", p1), alice.token, map[string]any{"payer": bob.id()})

	// Party 2 : Carol seule (burger) — jamais visible des autres.
	p2 := e.newOrderingParty(carol, burger, "Carol solo")
	e.expect(200, "POST", "/api/collections/order_items/records", carol.token, map[string]any{"party": p2, "user": carol.id(), "menu_item": e.menuItem(burger, "Cheeseburger")})

	// Party 3 : Bob hôte, Alice invitée, encore en salon (plus récente).
	p3 := e.expect(200, "POST", "/api/collections/parties/records", bob.token, map[string]any{"title": "Demain"}).m(t)
	e.expect(200, "POST", "/api/occ/parties/join", alice.token, map[string]any{"code": p3["code"]})

	// Totaux de référence : le récap serveur.
	var sum struct {
		GrandTotal   int `json:"grandTotal"`
		Participants []struct {
			User                        struct{ ID string }
			Subtotal, SharedFees, Total int
		} `json:"participants"`
	}
	e.expect(200, "GET", path("/api/occ/parties/%s/summary", p1), alice.token, nil).json(t, &sum)
	want := map[string][3]int{}
	for _, p := range sum.Participants {
		want[p.User.ID] = [3]int{p.Subtotal, p.SharedFees, p.Total}
	}
	if sum.GrandTotal != 2900+1300+600+299+100 {
		t.Fatalf("grand total %d", sum.GrandTotal)
	}

	var h historyResp
	e.expect(200, "GET", "/api/occ/me/history", alice.token, nil).json(t, &h)
	if h.TotalItems != 2 || len(h.Items) != 2 || h.Page != 1 || h.PerPage != 10 || h.TotalPages != 1 {
		t.Fatalf("alice history: %+v", h)
	}
	if h.Items[0].ID != p3["id"] || h.Items[1].ID != p1 {
		t.Fatalf("newest first expected: %s, %s", h.Items[0].ID, h.Items[1].ID)
	}
	if h.Items[0].Restaurant != nil || len(h.Items[0].Items) != 0 || h.Items[0].IsHost || h.Items[0].Host.Name != "Bob" || h.Items[0].Status != "lobby" {
		t.Fatalf("lobby entry: %+v", h.Items[0])
	}
	a := h.Items[1]
	w := want[alice.id()]
	if a.Status != "paying" || !a.IsHost || a.MemberCount != 2 || a.Restaurant == nil || a.Restaurant.ID != pizza || a.Restaurant.Name != "Test Pizza" {
		t.Fatalf("alice entry: %+v", a)
	}
	if a.Subtotal != 2900 || a.Subtotal != w[0] || a.SharedFees != w[1] || a.Total != w[2] || a.GrandTotal != sum.GrandTotal {
		t.Fatalf("alice totals %d/%d/%d (grand %d), want %v (grand %d)", a.Subtotal, a.SharedFees, a.Total, a.GrandTotal, w, sum.GrandTotal)
	}
	if len(a.Items) != 1 || a.Items[0].Name != "Margherita" || a.Items[0].OptionsLabel != "Large, Fromage" || a.Items[0].Quantity != 2 ||
		a.Items[0].UnitPrice != 1450 || a.Items[0].Total != 2900 || a.Items[0].MenuItem != marg {
		t.Fatalf("alice items: %+v", a.Items)
	}
	if a.Payer == nil || a.Payer.ID != bob.id() || a.Payment == nil || a.Payment.Amount != a.Total || a.Payment.Status != "pending" {
		t.Fatalf("alice payer/payment: %+v %+v", a.Payer, a.Payment)
	}

	// Bob : son propre panier, sa part « self » confirmée ; Σ parts = total.
	e.expect(200, "GET", "/api/occ/me/history", bob.token, nil).json(t, &h)
	if h.TotalItems != 2 {
		t.Fatalf("bob history: %d", h.TotalItems)
	}
	b := h.Items[1]
	if b.ID != p1 || b.Subtotal != 1900 || len(b.Items) != 2 || b.Total != want[bob.id()][2] || b.Payment == nil || b.Payment.Method != "self" || b.Payment.Status != "confirmed" {
		t.Fatalf("bob entry: %+v", b)
	}
	if a.Total+b.Total != sum.GrandTotal {
		t.Fatalf("Σ parts %d ≠ %d", a.Total+b.Total, sum.GrandTotal)
	}

	// Carol ne voit que sa party ; Dave rien.
	e.expect(200, "GET", "/api/occ/me/history", carol.token, nil).json(t, &h)
	if h.TotalItems != 1 || h.Items[0].ID != p2 {
		t.Fatalf("carol history: %+v", h)
	}
	e.expect(200, "GET", "/api/occ/me/history", dave.token, nil).json(t, &h)
	if h.TotalItems != 0 || len(h.Items) != 0 || h.Items == nil {
		t.Fatalf("dave history: %+v", h)
	}

	// Stats : seules les commandes passées (review/paying/closed avec articles) comptent.
	var st struct {
		Orders             int
		TotalSpent         int
		FavoriteRestaurant *struct{ ID, Name string }
		FavoriteDish       *struct {
			Name     string
			Quantity int
		}
	}
	e.expect(200, "GET", "/api/occ/me/stats", alice.token, nil).json(t, &st)
	if st.Orders != 1 || st.TotalSpent != a.Total || st.FavoriteRestaurant == nil || st.FavoriteRestaurant.ID != pizza ||
		st.FavoriteDish == nil || st.FavoriteDish.Name != "Margherita" || st.FavoriteDish.Quantity != 2 {
		t.Fatalf("alice stats: %+v", st)
	}
	e.expect(200, "GET", "/api/occ/me/stats", carol.token, nil).json(t, &st)
	if st.Orders != 0 || st.TotalSpent != 0 || st.FavoriteRestaurant != nil {
		t.Fatalf("carol stats (party still ordering): %+v", st)
	}

	// Rejoindre une party dont on est déjà membre (même en paiement) : idempotent, signalé.
	var jr struct {
		AlreadyMember bool `json:"alreadyMember"`
	}
	code := e.expect(200, "GET", "/api/collections/parties/records/"+p1, bob.token, nil).m(t)["code"].(string)
	e.expect(200, "POST", "/api/occ/parties/join", bob.token, map[string]any{"code": code}).json(t, &jr)
	if !jr.AlreadyMember {
		t.Fatal("alreadyMember expected")
	}
	e.expect(400, "POST", "/api/occ/parties/join", dave.token, map[string]any{"code": code})

	// Le filtre « mes commandes en cours » côté front : members.id ?= (members ?= ne matche jamais).
	q := url.Values{"filter": {"members.id ?= '" + bob.id() + "' && status != 'closed' && status != 'cancelled'"}}
	var list struct {
		TotalItems int `json:"totalItems"`
	}
	e.expect(200, "GET", "/api/collections/parties/records?"+q.Encode(), bob.token, nil).json(t, &list)
	if list.TotalItems != 2 {
		t.Fatalf("bob active parties via members.id: %d", list.TotalItems)
	}
	q = url.Values{"filter": {"members.id ?= '" + bob.id() + "'"}}
	e.expect(200, "GET", "/api/collections/parties/records?"+q.Encode(), carol.token, nil).json(t, &list)
	if list.TotalItems != 0 {
		t.Fatalf("carol must not list bob's parties: %d", list.TotalItems)
	}
}

func TestHistoryPagination(t *testing.T) {
	e := newEnv(t)
	dave := e.user("Dave")
	for i := 0; i < 12; i++ {
		e.expect(200, "POST", "/api/collections/parties/records", dave.token, map[string]any{"title": path("P%02d", i)})
	}
	var h historyResp
	e.expect(200, "GET", "/api/occ/me/history?page=1", dave.token, nil).json(t, &h)
	if h.TotalItems != 12 || h.TotalPages != 2 || len(h.Items) != 10 {
		t.Fatalf("page 1: total %d pages %d len %d", h.TotalItems, h.TotalPages, len(h.Items))
	}
	seen := map[string]bool{}
	for _, it := range h.Items {
		seen[it.ID] = true
	}
	e.expect(200, "GET", "/api/occ/me/history?page=2&perPage=10", dave.token, nil).json(t, &h)
	if len(h.Items) != 2 || h.Page != 2 {
		t.Fatalf("page 2: %+v", h)
	}
	for _, it := range h.Items {
		if seen[it.ID] {
			t.Fatalf("duplicate across pages: %s", it.ID)
		}
	}
	e.expect(200, "GET", "/api/occ/me/history?perPage=500", dave.token, nil).json(t, &h)
	if h.PerPage != 50 || len(h.Items) != 12 {
		t.Fatalf("perPage capped: %+v", h.PerPage)
	}
}

func TestReorder(t *testing.T) {
	e := newEnv(t)
	pizza, burger := e.testRestaurants()
	alice, bob, carol := e.user("Alice"), e.user("Bob"), e.user("Carol")
	marg, tira := e.menuItem(pizza, "Margherita"), e.menuItem(pizza, "Tiramisu")

	p1 := e.newOrderingParty(alice, pizza, "Hier", bob)
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{
		"party": p1, "user": bob.id(), "menu_item": marg, "quantity": 2, "note": "bien cuite",
		"selected_options": []map[string]any{{"group": "size", "choices": []string{"l"}}},
	})
	e.expect(200, "POST", "/api/collections/order_items/records", bob.token, map[string]any{"party": p1, "user": bob.id(), "menu_item": tira})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", p1), alice.token, map[string]any{"to": "review"})

	// Le tiramisu n'est plus disponible ; la margherita change de prix (le serveur recalcule).
	tr, _ := e.app.FindRecordById(colMenuItems, tira)
	tr.Set("available", false)
	if err := e.app.Save(tr); err != nil {
		t.Fatal(err)
	}
	mr, _ := e.app.FindRecordById(colMenuItems, marg)
	mr.Set("price", 1100)
	if err := e.app.Save(mr); err != nil {
		t.Fatal(err)
	}

	p2 := e.newOrderingParty(alice, pizza, "Aujourd'hui", bob, carol)
	var prev struct {
		Source *struct{ PartyID string } `json:"source"`
		Items  []struct {
			MenuItem  string `json:"menuItem"`
			Name      string `json:"name"`
			Quantity  int    `json:"quantity"`
			UnitPrice int    `json:"unitPrice"`
			Available bool   `json:"available"`
			Reason    string `json:"reason"`
		} `json:"items"`
	}
	e.expect(200, "GET", path("/api/occ/parties/%s/reorder", p2), bob.token, nil).json(t, &prev)
	if prev.Source == nil || prev.Source.PartyID != p1 || len(prev.Items) != 2 {
		t.Fatalf("preview: %+v", prev)
	}
	if !prev.Items[0].Available || prev.Items[0].UnitPrice != 1400 || prev.Items[0].Quantity != 2 || prev.Items[1].Available || prev.Items[1].Reason == "" {
		t.Fatalf("preview lines: %+v", prev.Items)
	}
	// Carol n'a jamais commandé ici ; un non-membre est refusé.
	e.expect(200, "GET", path("/api/occ/parties/%s/reorder", p2), carol.token, nil).json(t, &prev)
	if prev.Source != nil || len(prev.Items) != 0 {
		t.Fatalf("carol preview: %+v", prev)
	}
	dave := e.user("Dave")
	e.expect(403, "GET", path("/api/occ/parties/%s/reorder", p2), dave.token, nil)
	e.expect(403, "POST", path("/api/occ/parties/%s/reorder", p2), dave.token, nil)
	e.expect(404, "POST", path("/api/occ/parties/%s/reorder", p2), carol.token, nil)

	e.expect(200, "POST", path("/api/occ/parties/%s/ready", p2), bob.token, map[string]any{"ready": true})
	var res struct {
		Added []struct {
			Name     string
			Quantity int
		}
		Skipped []struct{ Name, Reason string }
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/reorder", p2), bob.token, map[string]any{"unit_price": 1}).json(t, &res)
	if len(res.Added) != 1 || res.Added[0].Name != "Margherita" || res.Added[0].Quantity != 2 || len(res.Skipped) != 1 || res.Skipped[0].Name != "Tiramisu" {
		t.Fatalf("reorder result: %+v", res)
	}
	items, _ := orderItems(e.app, p2)
	if len(items) != 1 || items[0].GetString("user") != bob.id() || items[0].GetInt("unit_price") != 1400 || items[0].GetInt("total") != 2800 ||
		items[0].GetString("options_label") != "Large" || items[0].GetString("note") != "bien cuite" {
		t.Fatalf("created lines: %v", items)
	}
	pm, _ := e.app.FindFirstRecordByFilter(colPartyMembers, "party={:p} && user={:u}", map[string]any{"p": p2, "u": bob.id()})
	if pm.GetBool("ready") {
		t.Fatal("reorder must reset ready")
	}

	// Autre restaurant : rien à reprendre ; hors prise de commande : refusé.
	p3 := e.newOrderingParty(bob, burger, "Burger", alice)
	e.expect(200, "GET", path("/api/occ/parties/%s/reorder", p3), bob.token, nil).json(t, &prev)
	if prev.Source != nil {
		t.Fatalf("burger preview: %+v", prev)
	}
	e.expect(400, "POST", path("/api/occ/parties/%s/reorder", p1), bob.token, nil)
}
