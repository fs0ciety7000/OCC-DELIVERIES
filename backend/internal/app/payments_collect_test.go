package app

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// « Encaisser » : the payer gets one QR payload per debtor, with each exact
// amount; nobody else can read them (not the host, not a debtor, not a
// non-member).
func TestPaymentsCollectQR(t *testing.T) {
	e := newEnv(t)
	pid, code, pizza, _, alice, bob, carol := setupParty(t, e)
	dave := e.user("Dave")
	e.expect(200, "POST", "/api/occ/parties/join", carol.token, map[string]any{"code": code})
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "ordering", "restaurant": pizza})
	tira := e.menuItem(pizza, "Tiramisu")
	for _, u := range []user{alice, bob, carol} {
		e.expect(200, "POST", "/api/collections/order_items/records", u.token, map[string]any{"party": pid, "user": u.id(), "menu_item": tira, "quantity": 1})
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/transition", pid), alice.token, map[string]any{"to": "review"})
	collect := path("/api/occ/parties/%s/payments/qr", pid)
	// not open yet
	e.expect(400, "GET", collect, bob.token, nil)

	e.expect(200, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{
		"user": bob.id(), "holder_name": "Bob Martin", "iban": "be71 0961 2345 6769",
		"revolut_tag": "bobm", "paypal_me": "bob",
	})
	// bob (a guest of alice's party, not the host) advanced the money
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": bob.id()})

	var out struct {
		Beneficiary string  `json:"beneficiary"`
		IBAN        *string `json:"iban"`
		Items       []struct {
			Payment string `json:"payment"`
			Debtor  struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"debtor"`
			Amount    int     `json:"amount"`
			Status    string  `json:"status"`
			Reference string  `json:"reference"`
			EPC       *string `json:"epc"`
			Links     []struct {
				Kind            string `json:"kind"`
				URL             string `json:"url"`
				AmountPrefilled bool   `json:"amountPrefilled"`
			} `json:"links"`
		} `json:"items"`
	}
	e.expect(200, "GET", collect, bob.token, nil).json(t, &out)
	if out.Beneficiary != "Bob Martin" || out.IBAN == nil || *out.IBAN != "BE71096123456769" {
		t.Fatalf("header: %+v", out)
	}
	if len(out.Items) != 2 || out.Items[0].Debtor.Name != "Alice" || out.Items[1].Debtor.Name != "Carol" {
		t.Fatalf("items (payer excluded, sorted by name): %+v", out.Items)
	}
	for _, it := range out.Items {
		if it.Status != "pending" || it.Amount <= 0 || it.EPC == nil {
			t.Fatalf("item: %+v", it)
		}
		amount := fmt.Sprintf("%d.%02d", it.Amount/100, it.Amount%100)
		if !strings.Contains(*it.EPC, "\nEUR"+amount+"\n") || !strings.HasSuffix(*it.EPC, "\n"+it.Reference) || !strings.Contains(it.Reference, it.Debtor.Name) {
			t.Fatalf("epc %q for %+v", *it.EPC, it)
		}
		if len(it.Links) != 2 || it.Links[0].Kind != "revolut" || !it.Links[0].AmountPrefilled ||
			!strings.Contains(it.Links[0].URL, "amount="+strconv.Itoa(it.Amount)+"&") || it.Links[1].URL != "https://paypal.me/bob/"+amount+"EUR" {
			t.Fatalf("links: %+v", it.Links)
		}
	}

	// confirmation is reflected
	e.expect(200, "POST", path("/api/occ/payments/%s/action", out.Items[0].Payment), bob.token, map[string]any{"action": "confirm"})
	e.expect(200, "GET", collect, bob.token, nil).json(t, &out)
	if out.Items[0].Status != "confirmed" {
		t.Fatalf("status: %+v", out.Items[0])
	}

	// authz: host (not payer), debtor, non-member, anonymous
	e.expect(403, "GET", collect, alice.token, nil)
	e.expect(403, "GET", collect, carol.token, nil)
	e.expect(403, "GET", collect, dave.token, nil)
	e.expect(401, "GET", collect, "", nil)
	e.expect(404, "GET", "/api/occ/parties/nope/payments/qr", bob.token, nil)
}
