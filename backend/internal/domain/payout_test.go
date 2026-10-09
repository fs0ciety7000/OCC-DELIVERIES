package domain

import "testing"

func TestNormalizeRevolutTag(t *testing.T) {
	cases := []struct {
		in, want string
		err      bool
	}{
		{"", "", false},
		{"  ", "", false},
		{"bob", "bob", false},
		{"@Bob", "bob", false},
		{"revolut.me/bob", "bob", false},
		{"https://revolut.me/bob", "bob", false},
		{"https://www.revolut.me/@bob/eur5/note", "bob", false},
		{"https://revolut.me/john.doe_42?x=1", "john.doe_42", false},
		{"https://paypal.me/bob", "", true},
		{"https://revolut.me/", "", true},
		{"b", "", true},
		{"bob smith", "", true},
		{"bob<script>", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeRevolutTag(c.in)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("%q → %q, %v (want %q, err=%v)", c.in, got, err, c.want, c.err)
		}
	}
}

func TestNormalizePayPalMe(t *testing.T) {
	cases := []struct {
		in, want string
		err      bool
	}{
		{"", "", false},
		{"JDoe", "JDoe", false},
		{"@jdoe", "jdoe", false},
		{"paypal.me/jdoe", "jdoe", false},
		{"https://www.paypal.me/jdoe/12EUR", "jdoe", false},
		{"https://www.paypal.com/paypalme/jdoe", "jdoe", false},
		{"https://paypal.com/be/home", "", true},
		{"https://revolut.me/jdoe", "", true},
		{"j doe", "", true},
	}
	for _, c := range cases {
		got, err := NormalizePayPalMe(c.in)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("%q → %q, %v (want %q, err=%v)", c.in, got, err, c.want, c.err)
		}
	}
}

func TestNormalizePaymentLink(t *testing.T) {
	cases := []struct {
		in, want string
		err      bool
	}{
		{"", "", false},
		{"https://wise.com/pay/business/acme", "https://wise.com/pay/business/acme", false},
		{"revolut.me/bob", "https://revolut.me/bob", false},
		{"http://Lydia-App.com/collect/x", "https://lydia-app.com/collect/x", false},
		{"javascript:alert(1)", "", true},
		{"mailto:bob@x.be", "", true},
		{"pas un lien", "", true},
		{"localhost", "", true},
	}
	for _, c := range cases {
		got, err := NormalizePaymentLink(c.in)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("%q → %q, %v (want %q, err=%v)", c.in, got, err, c.want, c.err)
		}
	}
}

func TestSplitPayoutLink(t *testing.T) {
	cases := []struct {
		name string
		in   PayoutHandles
		want PayoutHandles
	}{
		{"revolut moved", PayoutHandles{Link: "https://revolut.me/Bob"}, PayoutHandles{RevolutTag: "bob"}},
		{"paypal moved", PayoutHandles{Link: "https://paypal.me/jdoe/5EUR"}, PayoutHandles{PayPalMe: "jdoe"}},
		{"paypal.com moved", PayoutHandles{Link: "https://www.paypal.com/paypalme/jdoe"}, PayoutHandles{PayPalMe: "jdoe"}},
		{"same tag dropped", PayoutHandles{RevolutTag: "bob", Link: "https://revolut.me/bob"}, PayoutHandles{RevolutTag: "bob"}},
		{"other tag kept", PayoutHandles{RevolutTag: "alice", Link: "https://revolut.me/bob"}, PayoutHandles{RevolutTag: "alice", Link: "https://revolut.me/bob"}},
		{"generic kept", PayoutHandles{Link: "https://wise.com/pay/business/acme"}, PayoutHandles{Link: "https://wise.com/pay/business/acme"}},
		{"empty", PayoutHandles{}, PayoutHandles{}},
	}
	for _, c := range cases {
		if got := SplitPayoutLink(c.in); got != c.want {
			t.Errorf("%s: %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestLinkBuilders(t *testing.T) {
	if got := RevolutPaymentLink("bob", 1240, "OCC K7M2QX Alice"); got != "https://revolut.me/bob?amount=1240&currency=EUR&note=OCC+K7M2QX+Alice" {
		t.Errorf("revolut %q", got)
	}
	if got := RevolutPaymentLink("bob", 0, "x"); got != "https://revolut.me/bob" {
		t.Errorf("revolut no amount %q", got)
	}
	if got := PayPalMePaymentLink("jdoe", 1240); got != "https://paypal.me/jdoe/12.40EUR" {
		t.Errorf("paypal %q", got)
	}
	cases := []struct {
		link      string
		amount    int
		want      string
		prefilled bool
	}{
		{"https://wise.com/pay/business/acme", 1240, "https://wise.com/pay/business/acme?amount=12.40&currency=EUR&description=OCC+K7M2QX+Alice", true},
		{"https://paypal.me/bob", 1400, "https://paypal.me/bob/14.00EUR", true},
		{"https://revolut.me/bob", 500, "https://revolut.me/bob?amount=500&currency=EUR&note=OCC+K7M2QX+Alice", true},
		{"https://wise.com/pay/me/bob", 1240, "https://wise.com/pay/me/bob", false},
		{"https://lydia-app.com/collect/abc", 1240, "https://lydia-app.com/collect/abc", false},
		{"https://paypal.me/bob", 0, "https://paypal.me/bob", false},
		{"", 1240, "", false},
	}
	for _, c := range cases {
		got, pre := LinkWithAmount(c.link, c.amount, "OCC K7M2QX Alice")
		if got != c.want || pre != c.prefilled {
			t.Errorf("%q → %q %v, want %q %v", c.link, got, pre, c.want, c.prefilled)
		}
	}
}

func TestPaymentLinks(t *testing.T) {
	links := PaymentLinks(PayoutHandles{RevolutTag: "bob", PayPalMe: "bobm", Link: "https://www.lydia-app.com/collect/x"}, 1240, "OCC AB Alice")
	if len(links) != 3 {
		t.Fatalf("%d links", len(links))
	}
	want := []PaymentLink{
		{Kind: "revolut", Label: "Revolut", URL: "https://revolut.me/bob?amount=1240&currency=EUR&note=OCC+AB+Alice", AmountPrefilled: true},
		{Kind: "paypal", Label: "PayPal", URL: "https://paypal.me/bobm/12.40EUR", AmountPrefilled: true},
		{Kind: "link", Label: "lydia-app.com", URL: "https://www.lydia-app.com/collect/x", AmountPrefilled: false},
	}
	for i := range want {
		if links[i] != want[i] {
			t.Errorf("%d: %+v want %+v", i, links[i], want[i])
		}
	}
	if got := PaymentLinks(PayoutHandles{}, 1240, "x"); got == nil || len(got) != 0 {
		t.Errorf("empty handles: %#v", got)
	}
}
