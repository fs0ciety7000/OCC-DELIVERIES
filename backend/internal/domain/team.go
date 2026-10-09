package domain

import (
	"crypto/rand"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Permanent team rooms (« Salon d'équipe »). Pure rules: the PocketBase glue
// lives in internal/app/teams.go.

// TeamCodeLength is the length of a team invite code (/e/:code). Longer than
// a party code: the link is permanent, so it must be harder to guess.
const TeamCodeLength = 8

// Team name bounds (runes).
const (
	TeamNameMin = 2
	TeamNameMax = 80
)

// Weekdays are the values of teams.usual_days, Monday first.
var Weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// weekdayFR names the days for party titles (time.Weekday order: Sunday = 0).
var weekdayFR = [...]string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}

// GenerateTeamCode returns a random team invite code (crypto/rand).
func GenerateTeamCode() (string, error) { return generateTeamCode(rand.Reader) }

func generateTeamCode(r io.Reader) (string, error) {
	var b strings.Builder
	for b.Len() < TeamCodeLength {
		c, err := generateCode(r)
		if err != nil {
			return "", err
		}
		b.WriteString(c)
	}
	return b.String()[:TeamCodeLength], nil
}

// IsValidTeamCode reports whether s is a well formed team code.
func IsValidTeamCode(s string) bool {
	if len(s) != TeamCodeLength {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune(CodeAlphabet, rune(s[i])) {
			return false
		}
	}
	return true
}

// NormalizeTeamName collapses the spaces and checks the length.
func NormalizeTeamName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(s)
	if n < TeamNameMin {
		return "", Errf("Donne un nom à l'équipe (%d caractères minimum).", TeamNameMin)
	}
	if n > TeamNameMax {
		return "", Errf("Le nom de l'équipe est trop long (%d caractères maximum).", TeamNameMax)
	}
	return s, nil
}

var usualTimeRe = regexp.MustCompile(`^(\d{1,2})\s*(?:[:hH.]\s*(\d{2})?)?$`)

// NormalizeUsualTime accepts "12:30", "12h30", "12h", "9.05", "1230" and
// returns "HH:MM" ("" stays ""), the usual order time at Europe/Brussels.
func NormalizeUsualTime(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if len(s) == 4 && strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) < 0 {
		s = s[:2] + ":" + s[2:]
	}
	m := usualTimeRe.FindStringSubmatch(s)
	if m == nil {
		return "", Errf("Heure invalide (format attendu : 12:15).")
	}
	h, _ := strconv.Atoi(m[1])
	mi := 0
	if m[2] != "" {
		mi, _ = strconv.Atoi(m[2])
	}
	if h > 23 || mi > 59 {
		return "", Errf("Heure invalide (format attendu : 12:15).")
	}
	return twoDigits(h) + ":" + twoDigits(mi), nil
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// NormalizeDays keeps the known weekdays, without duplicates, Monday first.
func NormalizeDays(days []string) ([]string, error) {
	out := []string{}
	for _, d := range Weekdays {
		for _, in := range days {
			if strings.EqualFold(strings.TrimSpace(in), d) {
				out = append(out, d)
				break
			}
		}
	}
	for _, in := range days {
		if !slices.Contains(Weekdays, strings.ToLower(strings.TrimSpace(in))) {
			return nil, Errf("Jour inconnu : « %s ».", in)
		}
	}
	return out, nil
}

// DailyTitle is the default title of a team party launched at t (t must be
// in the team time zone): « Midi du lundi », « Soirée du lundi » from 16:00.
func DailyTitle(t time.Time) string {
	day := weekdayFR[t.Weekday()]
	if t.Hour() >= 16 {
		return "Soirée du " + day
	}
	return "Midi du " + day
}

// CheckTeamAdmins: every admin must be a member and none may be a guest.
func CheckTeamAdmins(members, admins, guests []string) error {
	for _, a := range admins {
		if !slices.Contains(members, a) {
			return Errf("Un·e admin de l'équipe doit d'abord en être membre.")
		}
		if slices.Contains(guests, a) {
			return Errf("Un·e invité·e ne peut pas être admin de l'équipe : il ou elle doit d'abord créer un compte.")
		}
	}
	return nil
}

// TeamRole is the role of a user in a team.
type TeamRole string

const (
	TeamOwner  TeamRole = "owner"
	TeamAdmin  TeamRole = "admin"
	TeamMember TeamRole = "member"
	TeamNone   TeamRole = ""
)

// RoleInTeam returns the role of user in the team.
func RoleInTeam(owner string, admins, members []string, user string) TeamRole {
	switch {
	case user == "":
		return TeamNone
	case user == owner:
		return TeamOwner
	case slices.Contains(admins, user):
		return TeamAdmin
	case slices.Contains(members, user):
		return TeamMember
	}
	return TeamNone
}

// CanManage reports whether the role may edit the team settings.
func (r TeamRole) CanManage() bool { return r == TeamOwner || r == TeamAdmin }

// CheckRemoveMember enforces who can remove whom: the owner can never be
// removed; only the owner removes an admin; owner and admins remove members.
func CheckRemoveMember(actor, target TeamRole) error {
	switch {
	case target == TeamNone:
		return Errf("Cette personne ne fait pas partie de l'équipe.")
	case target == TeamOwner:
		return Errf("Le ou la propriétaire de l'équipe ne peut pas en être retiré·e.")
	case !actor.CanManage():
		return Errf("Seuls le ou la propriétaire et les admins de l'équipe peuvent retirer un membre.")
	case target == TeamAdmin && actor != TeamOwner:
		return Errf("Seul·e le ou la propriétaire peut retirer un·e admin.")
	}
	return nil
}

// ------------------------------------------------------------------ guests

// Guests are accounts without e-mail or password, created from an invite
// link with a first name only (POST /api/occ/guest).

const (
	// GuestNameMax bounds the first name of a guest (runes).
	GuestNameMax = 40
	// GuestTokenDays is the validity of a guest token (renewed on refresh).
	GuestTokenDays = 30
	// GuestInactiveDays: guests without activity for this long are anonymised.
	GuestInactiveDays = 60
	guestDomain       = "guest.occ.invalid"
)

// GuestEmail is the placeholder (undeliverable, RFC 2606) e-mail of a guest.
func GuestEmail(userID string) string {
	return "guest-" + strings.ToLower(userID) + "@" + guestDomain
}

// IsPlaceholderEmail reports whether the address is a guest or anonymised
// placeholder (never a real mailbox).
func IsPlaceholderEmail(email string) bool {
	e := strings.ToLower(strings.TrimSpace(email))
	return strings.HasSuffix(e, ".invalid")
}

// NormalizeGuestName trims the first name, collapses spaces and checks it
// contains at least one letter.
func NormalizeGuestName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || strings.IndexFunc(s, unicode.IsLetter) < 0 {
		return "", Errf("Indique ton prénom pour que les collègues te reconnaissent.")
	}
	if utf8.RuneCountInString(s) > GuestNameMax {
		return "", Errf("Le prénom est trop long (%d caractères maximum).", GuestNameMax)
	}
	return s, nil
}

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// NormalizeColor returns "#RRGGBB" in upper case, "" for an empty value.
func NormalizeColor(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !hexColorRe.MatchString(s) {
		return "", Errf("Couleur invalide (format attendu : #E4572E).")
	}
	return strings.ToUpper(s), nil
}

// GuestInactive reports whether a guest whose last activity is lastActivity
// must be cleaned up at now.
func GuestInactive(lastActivity, now time.Time) bool {
	return !lastActivity.IsZero() && now.Sub(lastActivity) >= GuestInactiveDays*24*time.Hour
}
