package domain

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

var pizzaGroups = []OptionGroup{
	{ID: "size", Name: "Taille", Min: 1, Max: 1, Choices: []OptionChoice{
		{ID: "m", Name: "Moyenne", Price: 0},
		{ID: "l", Name: "Large", Price: 300},
	}},
	{ID: "extras", Name: "Suppléments", Min: 0, Max: 3, Choices: []OptionChoice{
		{ID: "cheese", Name: "Fromage", Price: 150},
		{ID: "ham", Name: "Jambon", Price: 200},
		{ID: "olives", Name: "Olives", Price: 100},
		{ID: "egg", Name: "Œuf", Price: 100},
	}},
	{ID: "sauce", Name: "Sauce", Min: 0, Max: 0, Choices: []OptionChoice{
		{ID: "chili", Name: "Piment", Price: 0},
		{ID: "garlic", Name: "Ail", Price: 50},
	}},
}

func TestPriceOptions(t *testing.T) {
	tests := []struct {
		name      string
		sel       []SelectedOption
		wantPrice int
		wantLabel string
		wantKey   string
		wantErr   string
	}{
		{"size only", []SelectedOption{{Group: "size", Choices: []string{"m"}}}, 1000, "Moyenne", "size=m", ""},
		{"large + extras in definition order",
			[]SelectedOption{{Group: "extras", Choices: []string{"ham", "cheese"}}, {Group: "size", Choices: []string{"l"}}},
			1650, "Large, Fromage, Jambon", "size=l;extras=cheese,ham", ""},
		{"unbounded max", []SelectedOption{{Group: "size", Choices: []string{"m"}}, {Group: "sauce", Choices: []string{"garlic", "chili"}}},
			1050, "Moyenne, Piment, Ail", "size=m;sauce=chili,garlic", ""},
		{"missing required", nil, 0, "", "", "Choisissez une option pour « Taille »"},
		{"too many in size", []SelectedOption{{Group: "size", Choices: []string{"m", "l"}}}, 0, "", "", "Au maximum 1"},
		{"too many extras", []SelectedOption{{Group: "size", Choices: []string{"m"}}, {Group: "extras", Choices: []string{"cheese", "ham", "olives", "egg"}}}, 0, "", "", "Au maximum 3"},
		{"unknown group", []SelectedOption{{Group: "size", Choices: []string{"m"}}, {Group: "nope", Choices: []string{"x"}}}, 0, "", "", "Groupe d'options inconnu"},
		{"unknown choice", []SelectedOption{{Group: "size", Choices: []string{"xl"}}}, 0, "", "", "Option inconnue"},
		{"duplicate choice", []SelectedOption{{Group: "size", Choices: []string{"m"}}, {Group: "extras", Choices: []string{"cheese", "cheese"}}}, 0, "", "", "Option en double"},
		{"duplicate group", []SelectedOption{{Group: "size", Choices: []string{"m"}}, {Group: "size", Choices: []string{"m"}}}, 0, "", "", "Groupe d'options en double"},
		{"empty extras group ignored", []SelectedOption{{Group: "size", Choices: []string{"l"}}, {Group: "extras", Choices: nil}}, 1300, "Large", "size=l", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PriceOptions(1000, pizzaGroups, tt.sel)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error %q, got %v", tt.wantErr, err)
				}
				if !IsDomainError(err) {
					t.Fatalf("expected domain error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.UnitPrice != tt.wantPrice || got.Label != tt.wantLabel || got.Key != tt.wantKey {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestPriceOptionsNoGroups(t *testing.T) {
	got, err := PriceOptions(850, nil, nil)
	if err != nil || got.UnitPrice != 850 || got.Label != "" {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := PriceOptions(-1, nil, nil); err == nil {
		t.Fatal("negative base price should fail")
	}
}

func TestValidateOptionGroups(t *testing.T) {
	if err := ValidateOptionGroups(pizzaGroups); err != nil {
		t.Fatal(err)
	}
	bad := [][]OptionGroup{
		{{ID: "", Choices: []OptionChoice{{ID: "a"}}}},
		{{ID: "a", Choices: []OptionChoice{{ID: "x"}}}, {ID: "a", Choices: []OptionChoice{{ID: "y"}}}},
		{{ID: "a"}},
		{{ID: "a", Min: 2, Max: 1, Choices: []OptionChoice{{ID: "x"}, {ID: "y"}}}},
		{{ID: "a", Min: 3, Choices: []OptionChoice{{ID: "x"}}}},
		{{ID: "a", Choices: []OptionChoice{{ID: "x"}, {ID: "x"}}}},
		{{ID: "a", Choices: []OptionChoice{{ID: "x", Price: -5}}}},
		{{ID: "a", Min: -1, Choices: []OptionChoice{{ID: "x"}}}},
	}
	for i, g := range bad {
		if err := ValidateOptionGroups(g); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func sum(m map[string]int) int {
	s := 0
	for _, v := range m {
		s += v
	}
	return s
}

func TestSplitFees(t *testing.T) {
	tests := []struct {
		name  string
		total int
		mode  string
		ps    []Share
		want  map[string]int
	}{
		{"equal exact", 300, SplitEqual, []Share{{"a", 1}, {"b", 1}, {"c", 1}}, map[string]int{"a": 100, "b": 100, "c": 100}},
		{"equal remainder stable by id", 100, SplitEqual, []Share{{"c", 1}, {"a", 1}, {"b", 1}}, map[string]int{"a": 34, "b": 33, "c": 33}},
		{"equal 2 cents over 3", 2, SplitEqual, []Share{{"b", 0}, {"c", 0}, {"a", 0}}, map[string]int{"a": 1, "b": 1, "c": 0}},
		{"proportional", 299, SplitProportional, []Share{{"a", 1000}, {"b", 2000}, {"c", 1500}},
			map[string]int{"a": 66, "b": 133, "c": 100}},
		{"proportional all zero falls back to equal", 10, SplitProportional, []Share{{"a", 0}, {"b", 0}}, map[string]int{"a": 5, "b": 5}},
		{"zero total", 0, SplitEqual, []Share{{"a", 1}}, map[string]int{"a": 0}},
		{"single", 499, SplitProportional, []Share{{"z", 1}}, map[string]int{"z": 499}},
		{"empty", 499, SplitEqual, nil, map[string]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitFees(tt.total, tt.mode, tt.ps)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("got %v want %v", got, tt.want)
				}
			}
			if len(tt.ps) > 0 && sum(got) != tt.total {
				t.Fatalf("sum %d != %d", sum(got), tt.total)
			}
		})
	}
}

func TestSplitFeesSumProperty(t *testing.T) {
	ids := []string{"u1", "u2", "u3", "u4", "u5", "u6", "u7"}
	for total := 0; total < 2000; total += 37 {
		for n := 1; n <= len(ids); n++ {
			var ps []Share
			for i := 0; i < n; i++ {
				ps = append(ps, Share{ID: ids[i], Subtotal: 100*i + 17*total%250})
			}
			for _, mode := range []string{SplitEqual, SplitProportional} {
				if got := sum(SplitFees(total, mode, ps)); got != total {
					t.Fatalf("total=%d n=%d mode=%s sum=%d", total, n, mode, got)
				}
			}
		}
	}
}

func TestFormat(t *testing.T) {
	cases := map[int][2]string{0: {"0.00", "0,00 €"}, 5: {"0.05", "0,05 €"}, 1250: {"12.50", "12,50 €"}, -199: {"-1.99", "-1,99 €"}, 123456: {"1234.56", "1234,56 €"}}
	for in, want := range cases {
		if got := FormatAmount(in); got != want[0] {
			t.Errorf("FormatAmount(%d)=%q", in, got)
		}
		if got := FormatEUR(in); got != want[1] {
			t.Errorf("FormatEUR(%d)=%q", in, got)
		}
	}
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name string
		in   TransitionInput
		ok   bool
	}{
		{"lobby→voting needs 2 candidates", TransitionInput{From: StatusLobby, To: StatusVoting, Candidates: 1}, false},
		{"lobby→voting ok", TransitionInput{From: StatusLobby, To: StatusVoting, Candidates: 2}, true},
		{"lobby→ordering needs restaurant", TransitionInput{From: StatusLobby, To: StatusOrdering}, false},
		{"lobby→ordering ok", TransitionInput{From: StatusLobby, To: StatusOrdering, RequestedRestaurant: "r"}, true},
		{"voting→ordering by election", TransitionInput{From: StatusVoting, To: StatusOrdering, Candidates: 2}, true},
		{"voting→ordering imposed candidate", TransitionInput{From: StatusVoting, To: StatusOrdering, Candidates: 2, RequestedRestaurant: "r", RequestedIsCandidate: true}, true},
		{"voting→ordering imposed non candidate", TransitionInput{From: StatusVoting, To: StatusOrdering, Candidates: 2, RequestedRestaurant: "r"}, false},
		{"ordering→review empty", TransitionInput{From: StatusOrdering, To: StatusReview}, false},
		{"ordering→review ok", TransitionInput{From: StatusOrdering, To: StatusReview, Items: 1}, true},
		{"review→ordering reopen", TransitionInput{From: StatusReview, To: StatusOrdering}, true},
		{"review→paying only via payer", TransitionInput{From: StatusReview, To: StatusPaying}, false},
		{"paying→closed manual", TransitionInput{From: StatusPaying, To: StatusClosed}, true},
		{"ordering→closed", TransitionInput{From: StatusOrdering, To: StatusClosed}, false},
		{"lobby→review", TransitionInput{From: StatusLobby, To: StatusReview, Items: 3}, false},
		{"voting→lobby", TransitionInput{From: StatusVoting, To: StatusLobby}, false},
		{"cancel from paying", TransitionInput{From: StatusPaying, To: StatusCancelled}, true},
		{"cancel from lobby", TransitionInput{From: StatusLobby, To: StatusCancelled}, true},
		{"cancel closed", TransitionInput{From: StatusClosed, To: StatusCancelled}, false},
		{"cancel cancelled", TransitionInput{From: StatusCancelled, To: StatusCancelled}, false},
		{"unknown target", TransitionInput{From: StatusLobby, To: "party"}, false},
		{"same", TransitionInput{From: StatusOrdering, To: StatusOrdering}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransition(tt.in)
			if (err == nil) != tt.ok {
				t.Fatalf("ok=%v err=%v", tt.ok, err)
			}
		})
	}
}

func TestElectWinner(t *testing.T) {
	cands := []Candidate{
		{ID: "r1", Name: "Pizza", Rating: 4.2},
		{ID: "r2", Name: "Burger", Rating: 4.6},
		{ID: "r3", Name: "Avocado", Rating: 4.6},
	}
	tests := []struct {
		name  string
		votes map[string]int
		want  string
	}{
		{"most votes", map[string]int{"r1": 3, "r2": 1}, "r1"},
		{"tie → rating", map[string]int{"r1": 2, "r2": 2}, "r2"},
		{"tie rating → name", map[string]int{"r2": 1, "r3": 1}, "r3"},
		{"no votes → best rating then name", nil, "r3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ElectWinner(cands, tt.votes); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
	if ElectWinner(nil, nil) != "" {
		t.Fatal("empty candidates should give empty winner")
	}
}

func TestGenerateCode(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		c, err := GenerateCode()
		if err != nil {
			t.Fatal(err)
		}
		if !IsValidCode(c) {
			t.Fatalf("invalid code %q", c)
		}
		if strings.ContainsAny(c, "01OIL") {
			t.Fatalf("ambiguous char in %q", c)
		}
		seen[c] = true
	}
	if len(seen) < 495 {
		t.Fatalf("too many collisions: %d unique", len(seen))
	}
	if _, err := generateCode(bytes.NewReader(nil)); err == nil {
		t.Fatal("expected error on exhausted reader")
	}
}

func TestNormalizeCode(t *testing.T) {
	cases := map[string]string{" k7m-2qx ": "K7M2QX", "abc def": "ABCDEF"}
	for in, want := range cases {
		if got := NormalizeCode(in); got != want {
			t.Errorf("%q → %q", in, got)
		}
	}
	for _, bad := range []string{"K7M2Q", "K7M2QX1", "K0M2QX", "K7M2QI"} {
		if IsValidCode(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestIBAN(t *testing.T) {
	tests := []struct {
		in, norm string
		ok       bool
	}{
		{"BE71 0961 2345 6769", "BE71096123456769", true},
		{"be71-0961-2345-6769", "BE71096123456769", true},
		{"FR14 2004 1010 0505 0001 3M02 606", "FR1420041010050500013M02606", true},
		{"NL91ABNA0417164300", "NL91ABNA0417164300", true},
		{"DE89 3704 0044 0532 0130 00", "DE89370400440532013000", true},
		{"BE72 0961 2345 6769", "BE72096123456769", false}, // bad check digits
		{"BE71 0961 2345 676", "BE7109612345676", false},   // bad length
		{"1234", "1234", false},                            // bad format
		{"BE71 0961 2345 67@9", "BE710961234567@9", false}, // bad char
	}
	for _, tt := range tests {
		n := NormalizeIBAN(tt.in)
		if n != tt.norm {
			t.Errorf("normalize %q → %q", tt.in, n)
		}
		if err := ValidateIBAN(n); (err == nil) != tt.ok {
			t.Errorf("validate %q: ok=%v err=%v", n, tt.ok, err)
		}
	}
	if b, err := NormalizeBIC(" gebabebb "); err != nil || b != "GEBABEBB" {
		t.Errorf("bic: %q %v", b, err)
	}
	if _, err := NormalizeBIC("GEB"); err == nil {
		t.Error("short BIC should fail")
	}
}

func TestBuildEPC(t *testing.T) {
	got, err := BuildEPC(EPCParams{Name: "Bob Martin", IBAN: "BE71 0961 2345 6769", Amount: 1400, Remittance: "OCC K7M2QX Alice"})
	if err != nil {
		t.Fatal(err)
	}
	want := "BCD\n002\n1\nSCT\n\nBob Martin\nBE71096123456769\nEUR14.00\n\n\nOCC K7M2QX Alice"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}

	long := strings.Repeat("é", 100)
	got, err = BuildEPC(EPCParams{BIC: "gebabebb", Name: long, IBAN: "BE71096123456769", Amount: 1250, Remittance: strings.Repeat("x", 200) + "\nline"})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 11 || lines[4] != "GEBABEBB" || lines[7] != "EUR12.50" {
		t.Fatalf("bad payload %q", got)
	}
	if n := len([]rune(lines[5])); n != 70 {
		t.Fatalf("name len %d", n)
	}
	if n := len([]rune(lines[10])); n != 140 {
		t.Fatalf("remittance len %d", n)
	}

	for _, p := range []EPCParams{
		{Name: "", IBAN: "BE71096123456769", Amount: 100},
		{Name: "Bob", IBAN: "BE72096123456769", Amount: 100},
		{Name: "Bob", IBAN: "BE71096123456769", Amount: 0},
		{Name: "Bob", IBAN: "BE71096123456769", Amount: 100, BIC: "XX"},
	} {
		if _, err := BuildEPC(p); err == nil {
			t.Errorf("expected error for %+v", p)
		}
	}
}

func TestHaversine(t *testing.T) {
	if d := HaversineKm(50.4542, 3.9567, 50.4542, 3.9567); d != 0 {
		t.Fatalf("same point %f", d)
	}
	// Mons → Brussels ≈ 53 km
	d := HaversineKm(50.4542, 3.9567, 50.8467, 4.3525)
	if math.Abs(d-51.5) > 3 {
		t.Fatalf("Mons-Bruxelles %f", d)
	}
	// 1° latitude ≈ 111.2 km
	if d := HaversineKm(0, 0, 1, 0); math.Abs(d-111.19) > 0.1 {
		t.Fatalf("1 degree %f", d)
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"0470 12 34 56", "+32470123456", true},
		{"0470.12.34.56", "+32470123456", true},
		{"0470-12-34-56", "+32470123456", true},
		{"0032 470 12 34 56", "+32470123456", true},
		{"+33 6 12 34 56 78", "+33612345678", true},
		{"+32 (0)470", "", false},
		{"", "", true},
		{"12345", "", false},
		{"+0470123456", "", false},
		{"abc", "", false},
		{"+3247012345678901", "", false},
	}
	for _, tt := range tests {
		got, err := NormalizePhone(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("%q → %q, %v (want %q ok=%v)", tt.in, got, err, tt.want, tt.ok)
		}
	}
}

func TestNormalizeWeroID(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"0470 12 34 56", "+32470123456", true},
		{" Bob.Martin@Example.BE ", "bob.martin@example.be", true},
		{"", "", true},
		{"bob@", "", false},
		{"bob@example", "", false},
		{"pas un numero", "", false},
	}
	for _, tt := range tests {
		got, err := NormalizeWeroID(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("%q → %q, %v (want %q ok=%v)", tt.in, got, err, tt.want, tt.ok)
		}
	}
}

func TestPaymentHelpers(t *testing.T) {
	for _, m := range []string{MethodQR, MethodRevolut, MethodPayPal, MethodWero, MethodBancontact, MethodLink, MethodCash} {
		if s, err := DeclareStatus(m); err != nil || s != PaymentDeclared {
			t.Errorf("%s → %s %v", m, s, err)
		}
	}
	if s, err := DeclareStatus(MethodLater); err != nil || s != PaymentPending {
		t.Errorf("later → %s %v", s, err)
	}
	for _, m := range []string{MethodSelf, "", "bitcoin"} {
		if _, err := DeclareStatus(m); err == nil {
			t.Errorf("%q should be rejected", m)
		}
	}

	all := AvailableMethods(PayoutAvailability{IBAN: true, Revolut: true, PayPal: true, Wero: true, Bancontact: true, Link: true})
	if strings.Join(all, ",") != "qr,revolut,paypal,link,wero,bancontact,cash,later" {
		t.Errorf("order: %v", all)
	}
	none := AvailableMethods(PayoutAvailability{})
	if strings.Join(none, ",") != "cash,later" {
		t.Errorf("none: %v", none)
	}
	if got := AvailableMethods(PayoutAvailability{IBAN: true}); strings.Join(got, ",") != "qr,cash,later" {
		t.Errorf("iban: %v", got)
	}

	if r := PaymentReference("K7M2QX", "Alice"); r != "OCC K7M2QX Alice" {
		t.Errorf("ref %q", r)
	}
	if r := PaymentReference("K7M2QX", " "); r != "OCC K7M2QX Membre" {
		t.Errorf("ref %q", r)
	}
	if r := PaymentReference("K7M2QX", strings.Repeat("a", 300)); len([]rune(r)) != 140 {
		t.Errorf("ref len %d", len(r))
	}
}

func TestBuildSummary(t *testing.T) {
	members := []MemberInfo{
		{User: UserInfo{ID: "alice", Name: "Alice"}, Ready: true},
		{User: UserInfo{ID: "bob", Name: "Bob"}, Ready: true},
		{User: UserInfo{ID: "carol", Name: "Carol"}, Ready: false}, // no items
	}
	items := []ItemInfo{
		{ID: "i1", UserID: "alice", MenuItem: "marg", Name: "Margherita", OptionsLabel: "Large", OptionsKey: "size=l", Quantity: 1, UnitPrice: 1250, Total: 1250},
		{ID: "i2", UserID: "bob", MenuItem: "marg", Name: "Margherita", OptionsLabel: "Large", OptionsKey: "size=l", Note: "sans oignon", Quantity: 2, UnitPrice: 1250, Total: 2500},
		{ID: "i3", UserID: "bob", MenuItem: "tira", Name: "Tiramisu", Quantity: 1, UnitPrice: 750, Total: 750},
		{ID: "i4", UserID: "alice", MenuItem: "marg", Name: "Margherita", OptionsLabel: "Moyenne", OptionsKey: "size=m", Quantity: 1, UnitPrice: 950, Total: 950},
	}
	in := SummaryInput{
		PartyID: "p1", Status: StatusReview, SplitMode: "",
		Restaurant:  &RestaurantInfo{ID: "r1", Name: "Luigi", MinOrder: 1500, DeliveryFee: 299},
		DeliveryFee: 299, ServiceFee: 100, Tip: 0,
		Members: members, Items: items,
	}
	s := BuildSummary(in)

	if s.ItemsSubtotal != 5450 || s.SharedFees != 399 || s.GrandTotal != 5849 {
		t.Fatalf("totals %+v", s)
	}
	if s.SplitMode != SplitEqual || s.Currency != "EUR" {
		t.Fatalf("defaults %+v", s)
	}
	total := 0
	for _, p := range s.Participants {
		total += p.Total
	}
	if total != s.GrandTotal {
		t.Fatalf("Σ total %d != %d", total, s.GrandTotal)
	}
	a, _ := s.ParticipantByID("alice")
	b, _ := s.ParticipantByID("bob")
	c, _ := s.ParticipantByID("carol")
	if a.SharedFees != 200 || b.SharedFees != 199 || c.SharedFees != 0 || c.Total != 0 {
		t.Fatalf("shares a=%d b=%d c=%d", a.SharedFees, b.SharedFees, c.SharedFees)
	}
	if len(s.Consolidated) != 3 {
		t.Fatalf("consolidated %+v", s.Consolidated)
	}
	first := s.Consolidated[0]
	if first.Quantity != 3 || first.Total != 3750 || len(first.Notes) != 1 || first.Notes[0] != "sans oignon" {
		t.Fatalf("merge %+v", first)
	}
	if !s.MinOrderReached || !s.AllReady {
		t.Fatalf("flags %+v", s)
	}

	// proportional split, not ready, min order not reached
	in.SplitMode = SplitProportional
	in.Members[0].Ready = false
	in.Items = items[:1]
	in.Restaurant.MinOrder = 2000
	s = BuildSummary(in)
	if s.AllReady || s.MinOrderReached {
		t.Fatalf("flags %+v", s)
	}
	a, _ = s.ParticipantByID("alice")
	if a.Total != s.GrandTotal || s.GrandTotal != 1250+399 {
		t.Fatalf("single participant %+v", s)
	}

	// nobody ordered
	in.Items = nil
	s = BuildSummary(in)
	if s.GrandTotal != 0 || s.AllReady || s.MinOrderReached || len(s.Consolidated) != 0 {
		t.Fatalf("empty %+v", s)
	}

	// no restaurant yet
	in.Restaurant = nil
	s = BuildSummary(in)
	if s.Restaurant != nil || s.MinOrderReached {
		t.Fatalf("no restaurant %+v", s)
	}

	// orphan line from a user that is not a member any more
	in.Items = []ItemInfo{{ID: "x", UserID: "ghost", MenuItem: "m", Name: "X", Quantity: 1, UnitPrice: 100, Total: 100}}
	s = BuildSummary(in)
	if s.GrandTotal != 100+399 {
		t.Fatalf("orphan %+v", s)
	}
}
