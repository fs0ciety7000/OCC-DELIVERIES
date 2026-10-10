package domain

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestResolveRelyingParty(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		rpID      string
		origins   []string
		wantID    string
		want      []string
		err       string
	}{
		{"prod", "https://eat.fs0ciety.org", "", nil, "eat.fs0ciety.org", []string{"https://eat.fs0ciety.org"}, ""},
		{"prod trailing slash + case", "https://EAT.fs0ciety.org/", "", nil, "eat.fs0ciety.org", []string{"https://eat.fs0ciety.org"}, ""},
		{"dev default", "http://localhost:8090", "", nil, "localhost",
			[]string{"http://localhost:8090", "http://localhost:5173", "http://localhost:4173"}, ""},
		{"parent rp id", "https://eat.fs0ciety.org", "fs0ciety.org", nil, "fs0ciety.org", []string{"https://eat.fs0ciety.org"}, ""},
		{"explicit origins", "https://eat.fs0ciety.org", "fs0ciety.org", []string{"https://eat.fs0ciety.org", " https://www.fs0ciety.org ", ""},
			"fs0ciety.org", []string{"https://eat.fs0ciety.org", "https://www.fs0ciety.org"}, ""},
		{"origins without public url", "", "example.com", []string{"https://example.com"}, "example.com", []string{"https://example.com"}, ""},
		{"origin outside rp", "https://eat.fs0ciety.org", "", []string{"https://evil.example"}, "", nil, "hors du domaine"},
		{"suffix trick", "https://notfs0ciety.org", "fs0ciety.org", nil, "", nil, "hors du domaine"},
		{"http outside localhost", "http://eat.fs0ciety.org", "", nil, "", nil, "https obligatoire"},
		{"ip address", "http://127.0.0.1:8090", "", nil, "", nil, "RP ID invalide"},
		{"rp id with port", "https://eat.fs0ciety.org", "eat.fs0ciety.org:443", nil, "", nil, "RP ID invalide"},
		{"bad public url", "eat.fs0ciety.org", "", nil, "", nil, "OCC_PUBLIC_URL"},
		{"bad origin", "https://eat.fs0ciety.org", "", []string{"ftp://eat.fs0ciety.org"}, "", nil, "OCC_WEBAUTHN_ORIGINS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp, err := ResolveRelyingParty(tt.publicURL, tt.rpID, tt.origins)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("want error %q, got %v (%+v)", tt.err, err, rp)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if rp.ID != tt.wantID || !slices.Equal(rp.Origins, tt.want) {
				t.Fatalf("got %+v, want %s %v", rp, tt.wantID, tt.want)
			}
		})
	}
}

func TestOriginsFor(t *testing.T) {
	dev := RelyingParty{ID: "localhost", Origins: []string{"http://localhost:8090"}}
	prod := RelyingParty{ID: "eat.fs0ciety.org", Origins: []string{"https://eat.fs0ciety.org"}}
	tests := []struct {
		rp     RelyingParty
		origin string
		want   []string
	}{
		{dev, "", []string{"http://localhost:8090"}},
		{dev, "http://localhost:8090", []string{"http://localhost:8090"}},
		{dev, "http://localhost:8102", []string{"http://localhost:8090", "http://localhost:8102"}},
		{dev, "http://127.0.0.1:8102", []string{"http://localhost:8090"}},
		{dev, "http://localhost.evil.example", []string{"http://localhost:8090"}},
		{prod, "http://localhost:5173", []string{"https://eat.fs0ciety.org"}},
	}
	for _, tt := range tests {
		if got := tt.rp.OriginsFor(tt.origin); !slices.Equal(got, tt.want) {
			t.Errorf("%s OriginsFor(%q) = %v, want %v", tt.rp.ID, tt.origin, got, tt.want)
		}
	}
	// the configured slice is never modified
	if len(dev.Origins) != 1 {
		t.Fatalf("origins mutated: %v", dev.Origins)
	}
}

func TestPasskeyName(t *testing.T) {
	long := strings.Repeat("é", 70)
	tests := []struct{ name, aaguid, want string }{
		{"  Mon   iPhone ", "", "Mon iPhone"},
		{"", "fbfc3007-154e-4ecc-8c0b-6e020557d7bd", "Trousseau iCloud"},
		{"", "EA9B8D66-4D01-1D21-3CE4-B6B48CB575D4", "Gestionnaire de mots de passe Google"},
		{"   ", "00000000-0000-0000-0000-000000000000", DefaultPasskeyName},
		{"Clé YubiKey", "fbfc3007-154e-4ecc-8c0b-6e020557d7bd", "Clé YubiKey"},
		{long, "", strings.Repeat("é", PasskeyNameMax)},
	}
	for _, tt := range tests {
		if got := PasskeyName(tt.name, tt.aaguid); got != tt.want {
			t.Errorf("PasskeyName(%q, %q) = %q, want %q", tt.name, tt.aaguid, got, tt.want)
		}
	}
	if _, err := ValidatePasskeyRename("  "); err == nil || !IsDomainError(err) {
		t.Fatalf("empty rename: %v", err)
	}
	if n, err := ValidatePasskeyRename(" Pixel  8 "); err != nil || n != "Pixel 8" {
		t.Fatalf("rename: %q %v", n, err)
	}
}

func TestChallengeStore(t *testing.T) {
	t0 := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	s := NewChallengeStore[string](5*time.Minute, 3)

	s.Put("a", "A", t0)
	if v, ok := s.Take("a", t0.Add(time.Minute)); !ok || v != "A" {
		t.Fatalf("take a = %q %v", v, ok)
	}
	if _, ok := s.Take("a", t0.Add(time.Minute)); ok {
		t.Fatal("a challenge must be single use")
	}

	s.Put("b", "B", t0)
	if _, ok := s.Take("b", t0.Add(5*time.Minute)); ok {
		t.Fatal("expired challenge accepted")
	}
	if s.Len() != 0 {
		t.Fatalf("expired entry not removed by Take: %d", s.Len())
	}
	if _, ok := s.Take("missing", t0); ok {
		t.Fatal("unknown challenge accepted")
	}

	// bounded: expired entries go first, then the oldest
	s.Put("old", "1", t0)
	s.Put("c", "2", t0.Add(4*time.Minute))
	s.Put("d", "3", t0.Add(4*time.Minute+time.Second))
	s.Put("e", "4", t0.Add(6*time.Minute)) // "old" expired → dropped
	if s.Len() != 3 {
		t.Fatalf("len = %d", s.Len())
	}
	if _, ok := s.Take("old", t0.Add(6*time.Minute)); ok {
		t.Fatal("old should have been dropped")
	}
	s.Put("f", "5", t0.Add(6*time.Minute)) // full, nothing expired → oldest ("c") dropped
	if _, ok := s.Take("c", t0.Add(6*time.Minute)); ok {
		t.Fatal("oldest entry should have been dropped")
	}
	for _, k := range []string{"d", "e", "f"} {
		if _, ok := s.Take(k, t0.Add(6*time.Minute)); !ok {
			t.Fatalf("%s missing", k)
		}
	}
}
