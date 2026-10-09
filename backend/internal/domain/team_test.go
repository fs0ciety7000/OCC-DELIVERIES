package domain

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTeamCode(t *testing.T) {
	for i := 0; i < 50; i++ {
		c, err := GenerateTeamCode()
		if err != nil {
			t.Fatal(err)
		}
		if !IsValidTeamCode(c) {
			t.Fatalf("invalid generated code %q", c)
		}
	}
	if _, err := generateTeamCode(bytes.NewReader(nil)); err == nil {
		t.Fatal("expected an error from an empty reader")
	}
	tests := []struct {
		in   string
		want bool
	}{
		{"K7M2QXAB", true}, {"K7M2QX", false}, {"K7M2QXA0", false}, {"k7m2qxab", false}, {"", false},
	}
	for _, tt := range tests {
		if got := IsValidTeamCode(tt.in); got != tt.want {
			t.Errorf("IsValidTeamCode(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeTeamName(t *testing.T) {
	tests := []struct{ in, want, err string }{
		{"  OCC   Mons — midi ", "OCC Mons — midi", ""},
		{"A", "", "minimum"},
		{"", "", "minimum"},
		{strings.Repeat("é", 81), "", "trop long"},
		{strings.Repeat("é", 80), strings.Repeat("é", 80), ""},
	}
	for _, tt := range tests {
		got, err := NormalizeTeamName(tt.in)
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("NormalizeTeamName(%q) err = %v, want %q", tt.in, err, tt.err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("NormalizeTeamName(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestNormalizeUsualTime(t *testing.T) {
	tests := []struct {
		in, want string
		err      bool
	}{
		{"", "", false}, {"12:30", "12:30", false}, {"12h30", "12:30", false}, {"12h", "12:00", false},
		{"9.05", "09:05", false}, {"1230", "12:30", false}, {" 7 ", "07:00", false}, {"12 h 15", "12:15", false},
		{"24:00", "", true}, {"12:60", "", true}, {"midi", "", true}, {"12:3", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeUsualTime(tt.in)
		if (err != nil) != tt.err || got != tt.want {
			t.Errorf("NormalizeUsualTime(%q) = %q, %v; want %q (err %v)", tt.in, got, err, tt.want, tt.err)
		}
	}
}

func TestNormalizeDays(t *testing.T) {
	tests := []struct {
		in   []string
		want []string
		err  bool
	}{
		{nil, []string{}, false},
		{[]string{"fri", "MON", "mon", " wed "}, []string{"mon", "wed", "fri"}, false},
		{[]string{"mon", "lundi"}, nil, true},
	}
	for _, tt := range tests {
		got, err := NormalizeDays(tt.in)
		if (err != nil) != tt.err || (!tt.err && !slices.Equal(got, tt.want)) {
			t.Errorf("NormalizeDays(%v) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
}

func TestDailyTitle(t *testing.T) {
	brussels, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	tests := []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 10, 12, 11, 0, 0, 0, brussels), "Midi du lundi"},
		{time.Date(2026, 10, 16, 15, 59, 0, 0, brussels), "Midi du vendredi"},
		{time.Date(2026, 10, 16, 18, 0, 0, 0, brussels), "Soirée du vendredi"},
		{time.Date(2026, 10, 18, 9, 0, 0, 0, brussels), "Midi du dimanche"},
	}
	for _, tt := range tests {
		if got := DailyTitle(tt.at); got != tt.want {
			t.Errorf("DailyTitle(%v) = %q, want %q", tt.at, got, tt.want)
		}
	}
}

func TestTeamRoles(t *testing.T) {
	members := []string{"own", "adm", "mem", "gst"}
	admins := []string{"adm"}
	tests := []struct {
		user string
		want TeamRole
	}{
		{"own", TeamOwner}, {"adm", TeamAdmin}, {"mem", TeamMember}, {"out", TeamNone}, {"", TeamNone},
	}
	for _, tt := range tests {
		if got := RoleInTeam("own", admins, members, tt.user); got != tt.want {
			t.Errorf("RoleInTeam(%q) = %q, want %q", tt.user, got, tt.want)
		}
	}
	if !TeamOwner.CanManage() || !TeamAdmin.CanManage() || TeamMember.CanManage() || TeamNone.CanManage() {
		t.Error("CanManage mismatch")
	}

	adminTests := []struct {
		admins []string
		err    string
	}{
		{nil, ""}, {[]string{"adm"}, ""}, {[]string{"out"}, "membre"}, {[]string{"gst"}, "invité"},
	}
	for _, tt := range adminTests {
		err := CheckTeamAdmins(members, tt.admins, []string{"gst"})
		if (tt.err == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tt.err)) {
			t.Errorf("CheckTeamAdmins(%v) = %v, want %q", tt.admins, err, tt.err)
		}
	}

	removeTests := []struct {
		actor, target TeamRole
		ok            bool
	}{
		{TeamOwner, TeamAdmin, true}, {TeamOwner, TeamMember, true}, {TeamAdmin, TeamMember, true},
		{TeamAdmin, TeamAdmin, false}, {TeamAdmin, TeamOwner, false}, {TeamMember, TeamMember, false},
		{TeamOwner, TeamNone, false},
	}
	for _, tt := range removeTests {
		if err := CheckRemoveMember(tt.actor, tt.target); (err == nil) != tt.ok {
			t.Errorf("CheckRemoveMember(%q, %q) = %v, want ok=%v", tt.actor, tt.target, err, tt.ok)
		}
	}
}

func TestGuestHelpers(t *testing.T) {
	if got := GuestEmail("AbC123"); got != "guest-abc123@guest.occ.invalid" {
		t.Errorf("GuestEmail = %q", got)
	}
	for _, e := range []string{GuestEmail("x"), AnonymizedEmail("y"), " X@Y.INVALID "} {
		if !IsPlaceholderEmail(e) {
			t.Errorf("IsPlaceholderEmail(%q) = false", e)
		}
	}
	if IsPlaceholderEmail("lea@occ.be") {
		t.Error("real address flagged as placeholder")
	}

	nameTests := []struct{ in, want, err string }{
		{"  Léa  ", "Léa", ""}, {"Jean  Marc", "Jean Marc", ""}, {"", "", "prénom"}, {"1234", "", "prénom"},
		{strings.Repeat("a", 41), "", "trop long"},
	}
	for _, tt := range nameTests {
		got, err := NormalizeGuestName(tt.in)
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("NormalizeGuestName(%q) err = %v, want %q", tt.in, err, tt.err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("NormalizeGuestName(%q) = %q, %v", tt.in, got, err)
		}
	}

	colorTests := []struct {
		in, want string
		err      bool
	}{{"", "", false}, {"#e4572e", "#E4572E", false}, {"red", "", true}, {"#FFF", "", true}}
	for _, tt := range colorTests {
		got, err := NormalizeColor(tt.in)
		if (err != nil) != tt.err || got != tt.want {
			t.Errorf("NormalizeColor(%q) = %q, %v", tt.in, got, err)
		}
	}

	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	inactiveTests := []struct {
		last time.Time
		want bool
	}{
		{now.AddDate(0, 0, -61), true}, {now.AddDate(0, 0, -60), true}, {now.AddDate(0, 0, -59), false},
		{now, false}, {time.Time{}, false},
	}
	for _, tt := range inactiveTests {
		if got := GuestInactive(tt.last, now); got != tt.want {
			t.Errorf("GuestInactive(%v) = %v, want %v", tt.last, got, tt.want)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	l := NewRateLimiter(3, time.Minute)
	t0 := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	steps := []struct {
		key  string
		at   time.Duration
		want bool
	}{
		{"a", 0, true}, {"a", time.Second, true}, {"a", 2 * time.Second, true},
		{"a", 3 * time.Second, false}, // 4th within the window
		{"b", 3 * time.Second, true},  // other key
		{"a", 59 * time.Second, false},
		{"a", 60 * time.Second, true}, // first hit expired
		{"a", 61 * time.Second, true},
		{"a", 61 * time.Second, false},
	}
	for i, s := range steps {
		if got := l.Allow(s.key, t0.Add(s.at)); got != s.want {
			t.Fatalf("step %d (%s at %v): got %v, want %v", i, s.key, s.at, got, s.want)
		}
	}
	// gc drops idle keys
	for i := 0; i < 300; i++ {
		l.Allow("c", t0.Add(time.Hour))
	}
	if _, found := l.hits["b"]; found {
		t.Error("idle key not garbage collected")
	}
}
