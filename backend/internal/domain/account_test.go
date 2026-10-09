package domain

import (
	"strings"
	"testing"
)

func TestAnonymizedEmail(t *testing.T) {
	got := AnonymizedEmail("AbC123")
	if got != "deleted-abc123@occ.invalid" {
		t.Fatalf("got %q", got)
	}
	cases := []struct {
		in   string
		want bool
	}{
		{got, true},
		{" Deleted-x@OCC.invalid ", true},
		{"deleted-x@example.com", false},
		{"alice@occ.invalid", false},
		{"", false},
	}
	for _, c := range cases {
		if IsAnonymizedEmail(c.in) != c.want {
			t.Errorf("IsAnonymizedEmail(%q) != %v", c.in, c.want)
		}
	}
}

func TestNormalizeBanReason(t *testing.T) {
	cases := []struct {
		in, want string
		err      bool
	}{
		{"", "", false},
		{"  spam   répété \n ", "spam répété", false},
		{strings.Repeat("é", MaxBanReason), strings.Repeat("é", MaxBanReason), false},
		{strings.Repeat("a", MaxBanReason+1), "", true},
	}
	for _, c := range cases {
		got, err := NormalizeBanReason(c.in)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("%q → %q, %v", c.in, got, err)
		}
	}
}

func TestCheckDeleteConfirmation(t *testing.T) {
	for _, ok := range []string{"SUPPRIMER", " supprimer ", "Supprimer"} {
		if err := CheckDeleteConfirmation(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "oui", "SUPPRIME", "SUPPRIMER!"} {
		if err := CheckDeleteConfirmation(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCheckModeration(t *testing.T) {
	user := ModerationTarget{ID: "u2"}
	admin := ModerationTarget{ID: "u2", IsAdmin: true}
	cases := []struct {
		name   string
		action ModerationAction
		actor  string
		target ModerationTarget
		admins int
		err    string
	}{
		{"ban a member", ActionBan, "u1", user, 1, ""},
		{"delete a member", ActionDelete, "u1", user, 1, ""},
		{"ban self", ActionBan, "u2", user, 2, "propre compte"},
		{"delete self", ActionDelete, "u2", admin, 2, "depuis ton profil"},
		{"demote self", ActionDemote, "u2", admin, 2, "propres droits"},
		{"ban one of two admins", ActionBan, "u1", admin, 2, ""},
		{"ban the last admin (superuser)", ActionBan, "su", admin, 1, "dernier compte administrateur"},
		{"demote the last admin", ActionDemote, "su", admin, 1, "dernier"},
		{"delete a banned last admin", ActionDelete, "su", ModerationTarget{ID: "u2", IsAdmin: true, Banned: true}, 0, ""},
		{"already deleted", ActionBan, "u1", ModerationTarget{ID: "u2", Deleted: true}, 1, "déjà été supprimé"},
	}
	for _, c := range cases {
		err := CheckModeration(c.action, c.actor, c.target, c.admins)
		switch {
		case c.err == "" && err != nil:
			t.Errorf("%s: unexpected %v", c.name, err)
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s: got %v, want %q", c.name, err, c.err)
		}
	}
}

func TestCheckSelfDelete(t *testing.T) {
	cases := []struct {
		name                      string
		admin                     bool
		admins, hosted, unsettled int
		err                       string
	}{
		{"member, nothing pending", false, 1, 0, 0, ""},
		{"hosts an open order", false, 1, 1, 0, "hôte"},
		{"payer not refunded yet", false, 1, 0, 2, "rembourser"},
		{"last admin", true, 1, 0, 0, "dernier"},
		{"one of two admins", true, 2, 0, 0, ""},
	}
	for _, c := range cases {
		err := CheckSelfDelete(c.admin, c.admins, c.hosted, c.unsettled)
		switch {
		case c.err == "" && err != nil:
			t.Errorf("%s: unexpected %v", c.name, err)
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s: got %v, want %q", c.name, err, c.err)
		}
	}
}
