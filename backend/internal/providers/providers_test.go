package providers

import (
	"strings"
	"testing"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

func sampleSummary() domain.Summary {
	return domain.BuildSummary(domain.SummaryInput{
		PartyID:     "p1",
		Status:      domain.StatusReview,
		Restaurant:  &domain.RestaurantInfo{ID: "r1", Name: "Chez Luigi", MinOrder: 1000, DeliveryFee: 299},
		DeliveryFee: 299,
		Tip:         100,
		Members:     []domain.MemberInfo{{User: domain.UserInfo{ID: "a"}}, {User: domain.UserInfo{ID: "b"}}},
		Items: []domain.ItemInfo{
			{ID: "1", UserID: "a", MenuItem: "m", Name: "Margherita", OptionsLabel: "Large", OptionsKey: "size=l", Quantity: 1, UnitPrice: 1250, Total: 1250},
			{ID: "2", UserID: "b", MenuItem: "m", Name: "Margherita", OptionsLabel: "Large", OptionsKey: "size=l", Note: "sans oignon", Quantity: 1, UnitPrice: 1250, Total: 1250},
		},
	})
}

func TestCartText(t *testing.T) {
	txt := CartText(Restaurant{Name: "Chez Luigi"}, sampleSummary())
	for _, want := range []string{
		"Commande groupée — Chez Luigi",
		"2 × Margherita (Large) — 25,00 €",
		"• sans oignon",
		"Livraison : 2,99 €",
		"Pourboire : 1,00 €",
		"Total estimé : 28,99 €",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in\n%s", want, txt)
		}
	}
}

func TestDispatch(t *testing.T) {
	r := Restaurant{
		ID: "r1", Name: "Chez Luigi", Phone: "065 12 34 56",
		Providers: []Link{
			{ID: UberEats, URL: "https://www.ubereats.com/be/store/luigi"},
			{ID: Deliveroo, URL: "https://deliveroo.be/fr/menu/Brussels/mons-center/luigi"},
			{ID: Weloveat, URL: "https://weloveat.be/luigi"},
		},
	}
	s := sampleSummary()

	tests := []struct {
		id       string
		wantURL  string
		wantText string
	}{
		{UberEats, "https://www.ubereats.com/be/store/luigi", "commande groupée"},
		{Takeaway, "https://www.takeaway.com/be-fr", "Takeaway.com"},
		{Deliveroo, "https://deliveroo.be/fr/menu/Brussels/mons-center/luigi", "Deliveroo"},
		{Weloveat, "https://weloveat.be/luigi", "weloveat.be"},
		{Export, "/api/occ/parties/p1/export?format=txt", "Téléchargez"},
		{Phone, "tel:065123456", "065 12 34 56"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			p, ok := Get(tt.id)
			if !ok {
				t.Fatal("not registered")
			}
			if p.ID() != tt.id || p.Name() == "" || !strings.HasPrefix(p.Color(), "#") {
				t.Fatalf("bad metadata %s %s %s", p.ID(), p.Name(), p.Color())
			}
			d := p.Dispatch(r, s)
			if d.Method != tt.id || d.URL != tt.wantURL {
				t.Fatalf("got %+v", d)
			}
			if d.CartText == "" || len(d.Instructions) < 3 {
				t.Fatalf("incomplete dispatch %+v", d)
			}
			if !strings.Contains(strings.Join(d.Instructions, "\n"), tt.wantText) {
				t.Fatalf("instructions miss %q: %v", tt.wantText, d.Instructions)
			}
		})
	}

	if _, ok := Get("glovo"); ok {
		t.Fatal("unexpected provider")
	}
	if len(IDs()) != 6 || !IsPlatform(UberEats) || !IsPlatform(Deliveroo) || !IsPlatform(Weloveat) || IsPlatform(Export) {
		t.Fatal("registry")
	}
	// without a restaurant link, the new platforms fall back to their home page
	if d := (deliveroo{}).Dispatch(Restaurant{Name: "X"}, s); d.URL != "https://deliveroo.be/fr/" {
		t.Fatalf("deliveroo fallback: %q", d.URL)
	}
	if d := (weloveat{}).Dispatch(Restaurant{Name: "X"}, s); d.URL != "https://weloveat.be/restaurants" {
		t.Fatalf("weloveat fallback: %q", d.URL)
	}
	if d := (phone{}).Dispatch(Restaurant{Name: "X"}, s); d.URL != "" {
		t.Fatalf("phone without number: %q", d.URL)
	}
}
