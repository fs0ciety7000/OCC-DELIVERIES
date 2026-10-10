package notify

import (
	"encoding/json"
	"strings"
)

// Prefs are a user's notification preferences (users.notify_prefs). Every
// category is on by default: a missing key or an empty value means « on ».
type Prefs struct {
	Party     bool `json:"party"`
	Payments  bool `json:"payments"`
	Reminders bool `json:"reminders"`
	// Emails: the « bon de commande » e-mail sent when the order is validated
	// (not a push category: it applies even without a subscribed device).
	Emails bool `json:"emails"`
}

// DefaultPrefs has every category on.
func DefaultPrefs() Prefs { return Prefs{Party: true, Payments: true, Reminders: true, Emails: true} }

// ParsePrefs reads the stored JSON; null, invalid JSON or missing keys fall
// back to « on ».
func ParsePrefs(raw []byte) Prefs {
	p := DefaultPrefs()
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return p
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return p
	}
	read := func(key string, dst *bool) {
		if v, ok := m[key].(bool); ok {
			*dst = v
		}
	}
	read("party", &p.Party)
	read("payments", &p.Payments)
	read("reminders", &p.Reminders)
	read("emails", &p.Emails)
	return p
}

// Allows reports whether a message of category c may be sent.
func (p Prefs) Allows(c Category) bool {
	switch c {
	case CategoryParty:
		return p.Party
	case CategoryPayments:
		return p.Payments
	case CategoryReminders:
		return p.Reminders
	}
	return true
}

// FilterByPrefs keeps the users whose preferences allow category c.
func FilterByPrefs(users []string, prefs map[string]Prefs, c Category) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		p, ok := prefs[u]
		if !ok {
			p = DefaultPrefs()
		}
		if p.Allows(c) {
			out = append(out, u)
		}
	}
	return out
}
