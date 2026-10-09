package domain

import (
	"slices"
	"testing"
)

func TestParseEuros(t *testing.T) {
	ok := map[string]int{
		"12,50": 1250, "12.50": 1250, "12,5": 1250, "12": 1200, "0": 0, "0,99": 99,
		" 12,50 € ": 1250, "€12.50": 1250, "1 234,50": 123450, "1.234,50": 123450,
		"1,234.50": 123450, "1\u00a0234,5": 123450, "9,9 EUR": 990, "12€50": 1250,
	}
	for in, want := range ok {
		got, err := ParseEuros(in)
		if err != nil || got != want {
			t.Errorf("ParseEuros(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "-1", "12,505", "1,2,3", "999999"} {
		if _, err := ParseEuros(in); err == nil {
			t.Errorf("ParseEuros(%q): expected an error", in)
		} else if !IsDomainError(err) {
			t.Errorf("ParseEuros(%q): not a domain error", in)
		}
	}
}

func TestParseDecimal(t *testing.T) {
	for in, want := range map[string]float64{"50,4542": 50.4542, "3.9567": 3.9567, "-0,5": -0.5, "4": 4} {
		if got, err := ParseDecimal(in); err != nil || got != want {
			t.Errorf("ParseDecimal(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseDecimal("nord"); err == nil {
		t.Error("expected an error")
	}
}

func TestParseYesNoAndSplitList(t *testing.T) {
	for in, want := range map[string]bool{"": false, "oui": true, "X": true, "1": true, "non": false, "TRUE": true, "0": false} {
		if got, err := ParseYesNo(in); err != nil || got != want {
			t.Errorf("ParseYesNo(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseYesNo("peut-être"); err == nil {
		t.Error("expected an error")
	}
	if got := SplitList(" Veggie | spicy,veggie;; new "); !slices.Equal(got, []string{"veggie", "spicy", "new"}) {
		t.Errorf("SplitList = %v", got)
	}
	if got := SplitList(""); got == nil || len(got) != 0 {
		t.Errorf("SplitList empty = %#v", got)
	}
}
