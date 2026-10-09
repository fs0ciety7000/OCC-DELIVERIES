package menusync

import (
	"math"
	"testing"
)

func TestCleanStoreName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CTR Chicken Mons (Independant)", "CTR Chicken"},
		{"Altaj (Quaregnon)", "Altaj"},
		{"Burger King - Quaregnon", "Burger King"},
		{"Snack Pitta Grill Akropolis (Mons)", "Snack Pitta Grill Akropolis"},
		{"Donroll’s (Mons)", "Donroll’s"},
		{"Tomo - Mons", "Tomo"},
		{"BAGEL CITY (Indépendant) (Mons)", "Bagel City"},
		{"Pizza Mons (Independant)", "Pizza Mons"}, // a generic word alone is not a name
		{"(Independant)", "(Independant)"},
		{"  Le   Comptoir  ", "Le Comptoir"},
	}
	for _, c := range cases {
		if got := CleanStoreName(c.in, "mons"); got != c.want {
			t.Errorf("CleanStoreName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNameMatch(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"CTR Chicken Mons (Independant)", "CTR Chicken", NameSameKey},
		{"Donroll’s (Mons)", "Donroll's", NameSameKey},
		{"Snack Pitta Grill Akropolis (Mons)", "Snack Pitta Grec Akropolis", NameTokenSet},
		{"Le Baalbeck", "Baalbeck Libanais", NameContained},
		{"Sushi Shop Mons", "Sushi Shop", NameSameKey},
		// generic words alone never match
		{"Pizza Hut", "Pizza Roma", NameNoMatch},
		{"Pizzeria", "Pizzeria Milano", NameNoMatch},
		{"Burger King", "Burger House", NameNoMatch},
		// short contained key
		{"Tomo", "Tomo Ramen", NameNoMatch},
		{"Shop", "Sushi Shop", NameNoMatch},
		// different places
		{"Le Petit Café", "Petit Paris", NameNoMatch},
		{"Akropolis", "Olympos", NameNoMatch},
		{"", "Tomo", NameNoMatch},
	}
	for _, c := range cases {
		if got := NameMatch(c.a, c.b, "mons"); got != c.want {
			t.Errorf("NameMatch(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := NameMatch(c.b, c.a, "mons"); got != c.want {
			t.Errorf("NameMatch(%q, %q) (swapped) = %d, want %d", c.b, c.a, got, c.want)
		}
	}
}

func TestTokenSetRatio(t *testing.T) {
	cases := []struct {
		a, b []string
		want float64
	}{
		{[]string{"snack", "pitta", "grill", "akropolis"}, []string{"snack", "pitta", "grec", "akropolis"}, 48.0 / 53},
		{[]string{"a", "b"}, []string{"b", "a"}, 1},
		{[]string{"tomo"}, []string{"tomo", "ramen"}, 1},
		{[]string{"abc"}, []string{"xyz"}, 0},
	}
	for _, c := range cases {
		if got := TokenSetRatio(c.a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("TokenSetRatio(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	if r := Ratio("", ""); r != 1 {
		t.Errorf("Ratio empty = %v", r)
	}
	if r := Ratio("kitten", "sitting"); math.Abs(r-2*4.0/13) > 1e-9 {
		t.Errorf("Ratio kitten/sitting = %v", r)
	}
}
