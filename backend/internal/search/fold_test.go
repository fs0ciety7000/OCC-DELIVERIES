package search

import (
	"reflect"
	"strings"
	"testing"
)

func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"Râmen":             "ramen",
		"RAMEN":             "ramen",
		"Crème brûlée":      "creme brulee",
		"Bœuf bourguignon":  "boeuf bourguignon",
		"ÆSIR straße":       "aesir strasse",
		"Poké bowl":         "poke bowl",
		"Ça c'est l'été !":  "ca c'est l'ete !",
		"Pizza 4 fromages":  "pizza 4 fromages",
		"":                  "",
		"Ñoquis à l'ancien": "noquis a l'ancien",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTerms(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"râmen", []string{"ramen"}},
		{"  RAM  ", []string{"ram"}},
		{"poké-bowl saumon", []string{"poke", "bowl", "saumon"}},
		{`"pizza" OR * NEAR( ^col:`, []string{"pizza", "or", "near", "col"}}, // FTS syntax neutralised
		{"pizza pizza", []string{"pizza"}},
		{"a b c d e f g h", []string{"a", "b", "c", "d", "e", "f"}},
		{"!!!", []string{}},
	}
	for _, c := range cases {
		if got := Terms(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Terms(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	long := Terms(strings.Repeat("x", 100))
	if len(long) != 1 || len([]rune(long[0])) != maxTermLen {
		t.Fatalf("long term not truncated: %v", long)
	}
}

func TestSnippet(t *testing.T) {
	desc := "Bouillon de porc mijoté douze heures, nouilles fraîches, œuf mariné, chashu et oignons nouveaux. Servi bien chaud."
	got := Snippet(desc, []string{"oeuf"}, 60)
	if !strings.Contains(got, "œuf mariné") {
		t.Fatalf("snippet without the match (accents kept): %q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("cut snippet without ellipsis: %q", got)
	}
	if len([]rune(got)) > 62 {
		t.Fatalf("snippet too long (%d): %q", len([]rune(got)), got)
	}
	// match at the start
	if got := Snippet("Ramen miso épicé", []string{"ramen"}, 90); got != "Ramen miso épicé" {
		t.Fatalf("short text changed: %q", got)
	}
	// no match → beginning of the text
	if got := Snippet(desc, []string{"zzz"}, 20); !strings.HasPrefix(got, "Bouillon") || !strings.HasSuffix(got, "…") {
		t.Fatalf("no-match snippet: %q", got)
	}
	// words only: « ram » does not match inside « caramel »
	if got := Snippet("Caramel beurre salé, puis ramequin", []string{"ram"}, 14); !strings.Contains(got, "ramequin") {
		t.Fatalf("matched inside a word: %q", got)
	}
	if Snippet("", []string{"x"}, 10) != "" {
		t.Fatal("empty text")
	}
}

func TestVocabCorrections(t *testing.T) {
	v := NewVocab([]VocabTerm{
		{"ramen", 12}, {"pizza", 40}, {"pizzeria", 3}, {"margherita", 9}, {"sushi", 7},
		{"poke", 5}, {"burger", 8}, {"ramequin", 1}, {"pizza", 2},
	})
	if len(v) != 8 {
		t.Fatalf("not deduped: %v", v)
	}
	cases := []struct {
		term string
		want []string
	}{
		{"ram", nil},                 // known prefix: no correction
		{"pizz", nil},                // known prefix
		{"piza", []string{"pizza"}},  // deletion
		{"ramne", []string{"ramen"}}, // transposition
		{"pizzas", []string{"pizza"}},
		{"magherita", []string{"margherita"}}, // 9 letters: distance 2 allowed
		{"sus", nil},                          // known prefix
		{"suhi", []string{"sushi"}},
		{"bxr", nil}, // too short to be corrected
		{"xyzzyx", nil},
	}
	for _, c := range cases {
		if got := v.Corrections(c.term); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Corrections(%q) = %q, want %q", c.term, got, c.want)
		}
	}
	// a typo in the beginning of a longer word
	if got := v.Corrections("burgr"); len(got) == 0 || got[0] != "burger" {
		t.Errorf("burgr → %q", got)
	}
	// equal distance: the longest common prefix wins over frequency
	pv := NewVocab([]VocabTerm{{"pita", 50}, {"pizza", 2}})
	if got := pv.Corrections("piza"); len(got) != 2 || got[0] != "pizza" {
		t.Errorf("piza → %q", got)
	}
	if got := v.Corrections("margh"); got != nil {
		t.Errorf("known prefix corrected: %q", got)
	}
	if got := v.Corrections("margj"); len(got) == 0 || got[0] != "margherita" {
		t.Errorf("prefix typo margj → %q", got)
	}
}

func TestDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"ramen", "ramen", 0}, {"ramen", "ramne", 1}, {"piza", "pizza", 1},
		{"abc", "xyz", 2}, // capped at maxD+1
		{"", "ab", 2},
	} {
		if got := distance([]rune(c.a), []rune(c.b), 1); min(got, 2) != min(c.want, 2) {
			t.Errorf("distance(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestMatchExpr(t *testing.T) {
	got := matchExpr([]string{"ram", "piza"}, [][]string{nil, {"pizza", "pizze"}})
	want := `"ram"* AND ("pizza"* OR "pizze"*)`
	if got != want {
		t.Fatalf("matchExpr = %s, want %s", got, want)
	}
}

func TestActions(t *testing.T) {
	ids := func(a []Action) []string {
		out := []string{}
		for _, x := range a {
			out = append(out, x.ID)
		}
		return out
	}
	if got := ids(actions(nil, Params{})); !reflect.DeepEqual(got, []string{"new-party", "restaurants"}) {
		t.Fatalf("anonymous actions %v", got)
	}
	if got := ids(actions(nil, Params{UserID: "u", Admin: true})); len(got) != 4 {
		t.Fatalf("admin actions %v", got)
	}
	if got := ids(actions(Terms("comm"), Params{UserID: "u"})); !reflect.DeepEqual(got, []string{"new-party", "my-orders"}) {
		t.Fatalf("« comm » actions %v", got)
	}
	if got := ids(actions(Terms("admin"), Params{UserID: "u"})); len(got) != 0 {
		t.Fatalf("admin action leaked to a user: %v", got)
	}
}
