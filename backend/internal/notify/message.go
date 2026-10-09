// Package notify builds the Web Push notifications of OCC Deliveries (pure,
// French copy, deep links) and sends them through a small worker pool.
// It never imports PocketBase: the app package wires the store and the
// events (internal/app/push.go).
package notify

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Category is a user preference bucket (users.notify_prefs).
type Category string

// Preference categories.
const (
	CategoryParty     Category = "party"
	CategoryPayments  Category = "payments"
	CategoryReminders Category = "reminders"
	// CategorySystem is always delivered (test notification).
	CategorySystem Category = "system"
)

// Notification kinds (payload "kind", used by the service worker / UI).
const (
	KindStatus          = "status"
	KindAllReady        = "all_ready"
	KindMemberJoined    = "member_joined"
	KindPaymentDeclared = "payment_declared"
	KindPaymentConfirm  = "payment_confirmed"
	KindReminderVote    = "reminder_vote"
	KindReminderOrder   = "reminder_order"
	KindAutoClosed      = "auto_closed"
	KindVoteExtended    = "vote_extended"
	KindNeedsHost       = "needs_host"
	KindTest            = "test"
)

// Urgency values (RFC 8030).
const (
	UrgencyNormal = "normal"
	UrgencyHigh   = "high"
)

// Message is one notification, before it is sent to a user's devices.
type Message struct {
	Kind     string
	Category Category
	Title    string
	Body     string
	// URL is the deep link opened on click (path of the SPA, e.g. /party/{id}).
	URL string
	// Tag groups notifications on the device (one per party): a newer one replaces the older.
	Tag string
	// Topic collapses undelivered messages at the push service (≤ 32 URL-safe chars).
	Topic    string
	PartyID  string
	Urgency  string
	TTL      time.Duration
	Renotify bool
}

// Payload is the JSON read by the service worker (frontend/src/pwa/sw.js).
type Payload struct {
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	URL      string `json:"url"`
	Tag      string `json:"tag"`
	PartyID  string `json:"partyId,omitempty"`
	Renotify bool   `json:"renotify"`
	Icon     string `json:"icon"`
	Badge    string `json:"badge"`
	TS       int64  `json:"ts"`
}

// Icons served by the SPA (frontend/public/icons).
const (
	IconURL  = "/icons/icon-192.png"
	BadgeURL = "/icons/badge-72.png"
)

// maxBody keeps the encrypted payload far below the 4 KB push limit.
const (
	maxTitle = 120
	maxBody  = 400
)

// JSON encodes the payload sent to the browser.
func (m Message) JSON(now time.Time) []byte {
	b, _ := json.Marshal(Payload{
		Kind: m.Kind, Title: clip(m.Title, maxTitle), Body: clip(m.Body, maxBody),
		URL: m.URL, Tag: m.Tag, PartyID: m.PartyID, Renotify: m.Renotify,
		Icon: IconURL, Badge: BadgeURL, TS: now.UnixMilli(),
	})
	return b
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// Party is what the builders need to know about a party.
type Party struct {
	ID    string
	Title string
	Code  string
}

func (p Party) name() string {
	if t := strings.TrimSpace(p.Title); t != "" {
		return "« " + t + " »"
	}
	return "la commande " + p.Code
}

func (p Party) url() string { return "/party/" + p.ID }

func (p Party) base(kind string, cat Category) Message {
	return Message{
		Kind: kind, Category: cat, URL: p.url(), Tag: "party-" + p.ID, Topic: Topic(p.ID),
		PartyID: p.ID, Urgency: UrgencyNormal, TTL: 2 * time.Hour, Renotify: true,
	}
}

// Topic is the collapse key of a party (Topic header: ≤ 32 chars of the
// URL-safe base64 alphabet).
func Topic(partyID string) string {
	var b strings.Builder
	b.WriteString("party-")
	for _, r := range partyID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

// Clock formats t as « 11:45 » in Brussels time.
func Clock(t time.Time) string {
	return t.In(Brussels).Format("15:04")
}

// Brussels is the time zone of the deadlines and their messages.
var Brussels = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		return time.UTC
	}
	return loc
}()

func until(deadline time.Time) string {
	if deadline.IsZero() {
		return ""
	}
	return " avant " + Clock(deadline)
}

// StatusInput describes a party status change.
type StatusInput struct {
	Party      Party
	Status     string
	Restaurant string // name of the chosen restaurant (ordering)
	Deadline   time.Time
	// Auto is true when the scheduler made the transition.
	Auto bool
}

// StatusChanged is the notification sent to members when a party changes
// status (paying is personal, see Paying). ok is false when the status is
// not worth a notification.
func StatusChanged(in StatusInput) (Message, bool) {
	p := in.Party
	m := p.base(KindStatus, CategoryParty)
	switch in.Status {
	case domain.StatusVoting:
		m.Title = "Le vote est ouvert 🗳️"
		m.Body = "Vote pour tes restos préférés pour " + p.name() + until(in.Deadline) + "."
	case domain.StatusOrdering:
		if in.Restaurant != "" {
			m.Title = "On commande chez " + in.Restaurant + " 🍽️"
		} else {
			m.Title = "À vos paniers 🍽️"
		}
		m.Body = "Compose ton panier pour " + p.name() + until(in.Deadline) + "."
		if in.Auto {
			m.Body = "Vote clôturé automatiquement. " + m.Body
		}
	case domain.StatusReview:
		m.Title = "Le récap est prêt 🧾"
		m.Body = "Les paniers de " + p.name() + " sont verrouillés : vérifie ta commande."
		if in.Auto {
			m.Body = "Heure limite atteinte : les paniers de " + p.name() + " sont verrouillés."
		}
	case domain.StatusClosed:
		m.Title = "Commande clôturée ✅"
		m.Body = capitalize(p.name()) + " est terminée. Merci à tous !"
		m.Urgency = "low"
	case domain.StatusCancelled:
		m.Title = "Commande annulée"
		m.Body = capitalize(p.name()) + " a été annulée."
	default:
		return Message{}, false
	}
	return m, true
}

// Paying is the personal « paiement » notification: debtors learn what they
// owe, the payer what they will get back.
func Paying(p Party, payerName string, amount int, isPayer bool, owedToPayer int) Message {
	m := p.base(KindStatus, CategoryPayments)
	if isPayer {
		m.Title = "C'est toi qui paies 💳"
		if owedToPayer > 0 {
			m.Body = "Tes collègues vont te rembourser " + domain.FormatEUR(owedToPayer) + " pour " + p.name() + "."
		} else {
			m.Body = "Tu as avancé l'argent pour " + p.name() + "."
		}
		return m
	}
	m.Title = "Paiement : tu dois " + domain.FormatEUR(amount)
	m.Body = "Rembourse " + payerName + " pour " + p.name() + "."
	m.Urgency = UrgencyHigh
	return m
}

// AllReady tells the host that every member is ready.
func AllReady(p Party) Message {
	m := p.base(KindAllReady, CategoryParty)
	m.Title = "Tout le monde est prêt ✅"
	m.Body = "Tu peux passer au récap de " + p.name() + "."
	m.Urgency = UrgencyHigh
	return m
}

// MemberJoined tells the host who joined (others is the number of extra
// people who joined since the last notification).
func MemberJoined(p Party, name string, others int) Message {
	m := p.base(KindMemberJoined, CategoryParty)
	if name == "" {
		name = "Quelqu'un"
	}
	switch {
	case others == 1:
		m.Title = name + " et 1 autre personne ont rejoint"
	case others > 1:
		m.Title = name + " et " + itoa(others) + " autres personnes ont rejoint"
	default:
		m.Title = name + " a rejoint la commande 👋"
	}
	m.Body = p.name()
	m.Renotify = false
	m.TTL = 30 * time.Minute
	return m
}

// MethodLabel is the French label of a reimbursement method.
func MethodLabel(method string) string {
	switch method {
	case domain.MethodQR:
		return "virement"
	case domain.MethodRevolut:
		return "Revolut"
	case domain.MethodPayPal:
		return "PayPal"
	case domain.MethodLink:
		return "lien de paiement"
	case domain.MethodWero:
		return "Wero"
	case domain.MethodBancontact:
		return "Bancontact Pay"
	case domain.MethodCash:
		return "espèces"
	}
	return ""
}

// PaymentDeclared tells the payer that a debtor says they paid.
func PaymentDeclared(p Party, debtor string, amount int, method string) Message {
	m := p.base(KindPaymentDeclared, CategoryPayments)
	m.Title = "Remboursement déclaré 💸"
	m.Body = debtor + " a déclaré t'avoir remboursé " + domain.FormatEUR(amount)
	if l := MethodLabel(method); l != "" {
		m.Body += " (" + l + ")"
	}
	m.Body += ". Pense à confirmer."
	m.Tag = "payment-" + p.ID
	return m
}

// PaymentConfirmed tells the debtor that the payer confirmed.
func PaymentConfirmed(p Party, by string, amount int) Message {
	m := p.base(KindPaymentConfirm, CategoryPayments)
	m.Title = "Remboursement confirmé ✅"
	m.Body = by + " a confirmé ton remboursement de " + domain.FormatEUR(amount) + "."
	m.Tag = "payment-" + p.ID
	return m
}

// ReminderVote is sent ~2 minutes before the end of the vote to members who
// have not voted yet.
func ReminderVote(p Party, deadline time.Time) Message {
	m := p.base(KindReminderVote, CategoryReminders)
	m.Title = "Plus que 2 minutes pour voter ⏳"
	m.Body = "Le vote de " + p.name() + " se termine à " + Clock(deadline) + "."
	m.Urgency = UrgencyHigh
	m.TTL = 3 * time.Minute
	return m
}

// ReminderOrder is sent ~2 minutes before the end of the ordering to members
// who are not ready.
func ReminderOrder(p Party, deadline time.Time, hasItems bool) Message {
	m := p.base(KindReminderOrder, CategoryReminders)
	m.Title = "Plus que 2 minutes pour commander ⏳"
	if hasItems {
		m.Body = "Valide ton panier (« Je suis prêt·e ») avant " + Clock(deadline) + " — " + p.name() + "."
	} else {
		m.Body = "Les paniers de " + p.name() + " se ferment à " + Clock(deadline) + "."
	}
	m.Urgency = UrgencyHigh
	m.TTL = 3 * time.Minute
	return m
}

// VoteExtended tells the host that nobody voted and the vote got 5 more minutes.
func VoteExtended(p Party, deadline time.Time) Message {
	m := p.base(KindVoteExtended, CategoryParty)
	m.Title = "Personne n'a voté : +5 minutes"
	m.Body = "Le vote de " + p.name() + " est prolongé jusqu'à " + Clock(deadline) + "."
	m.Urgency = UrgencyHigh
	return m
}

// NeedsHost tells the host that the scheduler could not close on its own.
func NeedsHost(p Party, reason string) Message {
	m := p.base(KindNeedsHost, CategoryParty)
	m.Title = "À toi de décider"
	m.Body = reason
	m.Urgency = UrgencyHigh
	return m
}

// NotReadyLocked tells a member who was not ready that the ordering closed
// automatically and their cart was kept as is.
func NotReadyLocked(p Party, hasItems bool) Message {
	m := p.base(KindAutoClosed, CategoryReminders)
	m.Title = "Heure limite atteinte"
	if hasItems {
		m.Body = "Tu n'avais pas validé ton panier : il a été gardé tel quel pour " + p.name() + "."
	} else {
		m.Body = "Les paniers de " + p.name() + " sont fermés sans commande de ta part."
	}
	return m
}

// KindTeamLaunched: a team party was launched.
const KindTeamLaunched = "team_launched"

// TeamLaunched invites the team members to the party of the day.
func TeamLaunched(p Party, team string) Message {
	m := p.base(KindTeamLaunched, CategoryParty)
	if strings.TrimSpace(team) == "" {
		team = "Ton équipe"
	}
	m.Title = strings.TrimSpace(team) + " : la commande du jour est lancée"
	m.Body = "Rejoins " + p.name() + "."
	m.Urgency = UrgencyHigh
	return m
}

// Test is the « Envoyer un test » notification.
func Test() Message {
	return Message{
		Kind: KindTest, Category: CategorySystem, Title: "Notifications activées 🔔",
		Body: "Tu recevras ici les étapes de tes commandes groupées.", URL: "/profile",
		Tag: "occ-test", Topic: "occ-test", Urgency: UrgencyNormal, TTL: 10 * time.Minute, Renotify: true,
	}
}

// Recipients returns members without the actor (deduplicated, order kept).
func Recipients(members []string, actor string) []string {
	out := make([]string, 0, len(members))
	seen := map[string]bool{}
	for _, m := range members {
		if m == "" || m == actor || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

func capitalize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
