package domain

import (
	"strings"
	"testing"
)

func TestPayoutAvailabilityOf(t *testing.T) {
	cases := []struct {
		name  string
		in    PayoutFields
		want  PayoutStatus
		ready bool
	}{
		{"empty", PayoutFields{}, PayoutStatus{Links: []string{}}, false},
		{"valid iban (spaces, lowercase)", PayoutFields{IBAN: "be71 0961 2345 6769"}, PayoutStatus{Ready: true, IBAN: true, Links: []string{}}, true},
		{"invalid iban checksum", PayoutFields{IBAN: "BE71096123456760"}, PayoutStatus{Links: []string{}}, false},
		{"garbage iban", PayoutFields{IBAN: "hello"}, PayoutStatus{Links: []string{}}, false},
		{"revolut only", PayoutFields{RevolutTag: "@Bob"}, PayoutStatus{Ready: true, Links: []string{"revolut"}}, true},
		{"invalid revtag", PayoutFields{RevolutTag: "x"}, PayoutStatus{Links: []string{}}, false},
		{"paypal only", PayoutFields{PayPalMe: "paypal.me/bob"}, PayoutStatus{Ready: true, Links: []string{"paypal"}}, true},
		{"generic link", PayoutFields{PaymentLink: "lydia-app.com/collect/bob"}, PayoutStatus{Ready: true, Links: []string{"link"}}, true},
		{"revolut link in free field", PayoutFields{PaymentLink: "https://revolut.me/bob"}, PayoutStatus{Ready: true, Links: []string{"revolut"}}, true},
		{"unsafe link", PayoutFields{PaymentLink: "javascript:alert(1)"}, PayoutStatus{Links: []string{}}, false},
		{"everything", PayoutFields{IBAN: "BE71096123456769", RevolutTag: "bob", PayPalMe: "bob", PaymentLink: "https://wise.com/pay/business/bob"},
			PayoutStatus{Ready: true, IBAN: true, Links: []string{"revolut", "paypal", "link"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := PayoutAvailabilityOf(c.in)
			got := StatusOf(a)
			if got.Ready != c.want.Ready || got.IBAN != c.want.IBAN || strings.Join(got.Links, ",") != strings.Join(c.want.Links, ",") || got.Links == nil {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
			if a.CanReceive() != c.ready {
				t.Fatalf("CanReceive = %v", a.CanReceive())
			}
		})
	}
}

func TestCheckPayer(t *testing.T) {
	none := PayoutAvailability{}
	iban := PayoutAvailability{IBAN: true}
	revolut := PayoutAvailability{Revolut: true}
	cases := []struct {
		name    string
		payer   string
		debtors []string
		payout  PayoutAvailability
		self    bool
		who     string
		wantErr string
	}{
		{"self without method", "a", []string{"a", "b"}, none, true, "Alice", "Ajoute ton IBAN (ou Revolut / PayPal) dans ton profil avant de valider la commande."},
		{"other without method", "b", []string{"a", "b"}, none, false, "Bob", "Bob n'a encore renseigné aucun moyen de remboursement."},
		{"other without name", "b", []string{"a"}, none, false, "  ", "Ce membre n'a encore renseigné aucun moyen de remboursement."},
		{"payer who did not order still needs a method", "c", []string{"a", "b"}, none, false, "Carol", "Carol n'a encore renseigné"},
		{"iban", "b", []string{"a", "b"}, iban, false, "Bob", ""},
		{"revolut only", "b", []string{"a", "b"}, revolut, false, "Bob", ""},
		{"payer ordered alone: nothing to reimburse", "a", []string{"a"}, none, true, "Alice", ""},
		{"no debtors at all", "a", nil, none, true, "Alice", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckPayer(c.payer, c.debtors, c.payout, c.self, c.who)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), c.wantErr) || !IsDomainError(err) {
				t.Fatalf("err = %v, want %q", err, c.wantErr)
			}
		})
	}
}
