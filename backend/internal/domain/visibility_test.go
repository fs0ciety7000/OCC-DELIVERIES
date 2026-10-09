package domain

import "testing"

func TestValidateMinMenuItems(t *testing.T) {
	tests := []struct {
		n    int
		ok   bool
		name string
	}{
		{0, true, "disabled"},
		{1, true, "lowest"},
		{10, true, "default"},
		{100, true, "highest"},
		{-1, false, "negative"},
		{101, false, "too high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMinMenuItems(tt.n)
			if (err == nil) != tt.ok {
				t.Fatalf("ValidateMinMenuItems(%d) = %v", tt.n, err)
			}
			if err != nil && !IsDomainError(err) {
				t.Fatal("expected a business error")
			}
		})
	}
}

func TestHiddenIncomplete(t *testing.T) {
	tests := []struct {
		name       string
		items, min int
		hidden     bool
	}{
		{"filter disabled, empty menu", 0, 0, false},
		{"filter disabled, small menu", 3, 0, false},
		{"below threshold", 9, 10, true},
		{"empty menu", 0, 10, true},
		{"exactly the threshold", 10, 10, false},
		{"above threshold", 52, 10, false},
		{"threshold 1 hides empty menus only", 0, 1, true},
		{"threshold 1 keeps one item", 1, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HiddenIncomplete(tt.items, tt.min); got != tt.hidden {
				t.Fatalf("HiddenIncomplete(%d, %d) = %v", tt.items, tt.min, got)
			}
		})
	}
}
