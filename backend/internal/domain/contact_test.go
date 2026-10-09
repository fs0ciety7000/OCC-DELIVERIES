package domain

import "testing"

func TestNormalizeRestaurantPhone(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"065352964", "+3265352964", true},
		{"065 35 29 64", "+3265352964", true},
		{"065/35.29.64", "+3265352964", true},
		{"065 / 35 29 64", "+3265352964", true},
		{"+32 (0)65 35 29 64", "+3265352964", true},
		{"+32 065 35 29 64", "+3265352964", true},
		{"0032 65 35 29 64", "+3265352964", true},
		{"32 65 35 29 64", "+3265352964", true},
		{"+32484158003", "+32484158003", true},
		{"0495466512", "+32495466512", true},
		{"0495 46 65 12", "+32495466512", true},
		{"Tél. : 065 35 29 64", "+3265352964", true},
		{"tel:+3265352964", "+3265352964", true},
		{"02 123 45 67", "+3221234567", true},
		{"0800 12 345", "+3280012345", true},
		{"065 35 29 64 / 0475 12 34 56", "+3265352964", true},
		{"pas de téléphone ; 0475 12 34 56", "+32475123456", true},
		{"+33 3 27 12 34 56", "+33327123456", true},
		{"0000000000", "", false},
		{"0999999999", "", false},
		{"12345", "", false},
		{"065 35", "", false},
		{"0565352964", "", false}, // 9 digits, not a mobile
		{"n/a", "", false},
		{"", "", false},
		{"+3265352964", "+3265352964", true}, // idempotent
	}
	for _, c := range cases {
		got, ok := NormalizeRestaurantPhone(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("NormalizeRestaurantPhone(%q) = %q, %v ; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestFormatPhone(t *testing.T) {
	cases := map[string]string{
		"+3265352964":  "+32 65 35 29 64",
		"+32495466512": "+32 495 46 65 12",
		"+3221234567":  "+32 2 123 45 67",
		"+3242223344":  "+32 4 222 33 44",
		"+3280012345":  "+32 800 12 345",
		"+33327123456": "+33327123456",
		"065352964":    "065352964",
		"":             "",
	}
	for in, want := range cases {
		if got := FormatPhone(in); got != want {
			t.Errorf("FormatPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAddress(t *testing.T) {
	cases := []struct{ in, want string }{
		{"24 Rue de la Clef, 7000", "Rue de la Clef 24, 7000 Mons"},
		{"14 rue de la clef 7000", "Rue de la Clef 14, 7000 Mons"},
		{"11 Rue d’Enghien, 7000 Mons", "Rue d’Enghien 11, 7000 Mons"},
		{"Rue d'Enghien 11, 7000 Mons, Belgique", "Rue d'Enghien 11, 7000 Mons"},
		{"Rue d'Enghien 11, 7000 Mons", "Rue d'Enghien 11, 7000 Mons"},
		{"Av. de l'Enseignement 1, 7330 Saint-Ghislain, Belgium", "Av. de l'Enseignement 1, 7330 Saint-Ghislain"},
		{"7 Rue Léopold II, 7000 Mons", "Rue Léopold II 7, 7000 Mons"},
		{"Rue de Nimy 1 7000 Mons", "Rue de Nimy 1, 7000 Mons"},
		{"RUE DE NIMY 1, 7000 MONS", "Rue de Nimy 1, 7000 Mons"},
		{"Grand-Place 2, B-7000 Mons", "Grand-Place 2, 7000 Mons"},
		{"Grand-Place 2, BE-7000", "Grand-Place 2, 7000 Mons"},
		{"Route de Wallonie 4, 7011", "Route de Wallonie 4, 7011 Ghlin"},
		{"Rue de Flénu 10, 7012 Flénu", "Rue de Flénu 10, 7012 Flénu"},
		{"Chaussée du Roeulx 40, 7020", "Chaussée du Roeulx 40, 7020 Nimy"},
		{"Rue X 12, 7033", "Rue X 12, 7033 Cuesmes"},
		{"Rue Grande 3, 7390", "Rue Grande 3, 7390 Quaregnon"},
		{"Rue Grande 3, Quaregnon", "Rue Grande 3, 7390 Quaregnon"},
		{"Rue de la Clef 24, Mons", "Rue de la Clef 24, 7000 Mons"},
		{"24, Rue de la Clef, 7000 Mons", "Rue de la Clef 24, 7000 Mons"},
		{"12-14 rue des Fripiers, 7000 Mons", "Rue des Fripiers 12-14, 7000 Mons"},
		{"24A Rue de la Clef, 7000 Mons", "Rue de la Clef 24A, 7000 Mons"},
		{"14 rue de la clef 7000, Brussels", "Rue de la Clef 14, 7000 Mons"},
		{"Rue de la Loi 16, 1000 Brussels", "Rue de la Loi 16, 1000 Brussels"},
		{"Chaussée de Binche 1050", "Chaussée de Binche 1050"},
		{"1050 Chaussée de Binche, 7000 Mons", "Chaussée de Binche 1050, 7000 Mons"},
		{"Rue Inconnue 5, 4000 Liège", "Rue Inconnue 5, 4000 Liège"},
		{"7000 Mons", "7000 Mons"},
		{"7000", "7000 Mons"},
		{"Mons", "Mons"},
		{"  Rue   de  Nimy 1 ,  7000  Mons ", "Rue de Nimy 1, 7000 Mons"},
		{"Galerie du Centre, Rue X 3, 7000 Mons", "Galerie du Centre, Rue X 3, 7000 Mons"},
		{"", ""},
	}
	for _, c := range cases {
		got := NormalizeAddress(c.in)
		if got != c.want {
			t.Errorf("NormalizeAddress(%q) = %q, want %q", c.in, got, c.want)
		}
		if again := NormalizeAddress(got); again != got {
			t.Errorf("NormalizeAddress not idempotent: %q → %q", got, again)
		}
	}
}

func TestPostcodeTable(t *testing.T) {
	for pc, want := range map[string]string{"7000": "Mons", "7012": "Jemappes", "7022": "Hyon", "7080": "Frameries", "9999": ""} {
		if got := LocalityForPostcode(pc); got != want {
			t.Errorf("LocalityForPostcode(%s) = %q", pc, got)
		}
	}
	for loc, want := range map[string]string{"mons": "7000", "Flenu": "7012", "HAVRÉ": "7021", "Atlantis": ""} {
		if got := PostcodeForLocality(loc); got != want {
			t.Errorf("PostcodeForLocality(%s) = %q", loc, got)
		}
	}
}
