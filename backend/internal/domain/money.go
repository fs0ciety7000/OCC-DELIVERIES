// Package domain contains the pure business logic of OCC Deliveries.
//
// It must never import PocketBase: everything here is plain Go, deterministic
// and covered by table tests. All amounts are integer cents (EUR).
package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Error is a user facing business error (French message).
type Error struct {
	Msg string
}

func (e *Error) Error() string { return e.Msg }

// Errf builds a business error with a formatted French message.
func Errf(format string, args ...any) error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}

// IsDomainError reports whether err is (or wraps) a business error.
func IsDomainError(err error) bool {
	var de *Error
	return errors.As(err, &de)
}

// OptionChoice is one selectable choice of an option group.
type OptionChoice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}

// OptionGroup describes a group of options of a menu item (size, extras…).
// Max <= 0 means "no upper bound".
type OptionGroup struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Min     int            `json:"min"`
	Max     int            `json:"max"`
	Choices []OptionChoice `json:"choices"`
}

// SelectedOption is what the client sends: the chosen choice ids of a group.
type SelectedOption struct {
	Group   string   `json:"group"`
	Choices []string `json:"choices"`
}

// PricedOptions is the server side result of an option selection.
type PricedOptions struct {
	UnitPrice int
	Label     string
	// Selected is the normalized selection (definition order, empty groups removed).
	Selected []SelectedOption
	// Key is a canonical representation used to merge identical lines.
	Key string
}

// ValidateOptionGroups checks the structural validity of option groups
// (used when importing menus).
func ValidateOptionGroups(groups []OptionGroup) error {
	seen := map[string]bool{}
	for _, g := range groups {
		if strings.TrimSpace(g.ID) == "" {
			return Errf("Groupe d'options sans identifiant.")
		}
		if seen[g.ID] {
			return Errf("Groupe d'options en double : %q.", g.ID)
		}
		seen[g.ID] = true
		if len(g.Choices) == 0 {
			return Errf("Le groupe %q n'a aucun choix.", g.ID)
		}
		if g.Min < 0 {
			return Errf("Minimum négatif pour le groupe %q.", g.ID)
		}
		if g.Max > 0 && g.Min > g.Max {
			return Errf("Minimum supérieur au maximum pour le groupe %q.", g.ID)
		}
		if g.Min > len(g.Choices) {
			return Errf("Le groupe %q exige plus de choix qu'il n'en propose.", g.ID)
		}
		choices := map[string]bool{}
		for _, c := range g.Choices {
			if strings.TrimSpace(c.ID) == "" {
				return Errf("Choix sans identifiant dans le groupe %q.", g.ID)
			}
			if choices[c.ID] {
				return Errf("Choix en double %q dans le groupe %q.", c.ID, g.ID)
			}
			if c.Price < 0 {
				return Errf("Prix négatif pour le choix %q.", c.ID)
			}
			choices[c.ID] = true
		}
	}
	return nil
}

// PriceOptions validates a selection against the option groups of a menu item
// and computes the unit price (base + choices) and a human readable label.
func PriceOptions(basePrice int, groups []OptionGroup, selected []SelectedOption) (PricedOptions, error) {
	if basePrice < 0 {
		return PricedOptions{}, Errf("Prix de base invalide.")
	}

	byGroup := map[string][]string{}
	for _, s := range selected {
		if _, dup := byGroup[s.Group]; dup {
			return PricedOptions{}, Errf("Groupe d'options en double : %q.", s.Group)
		}
		byGroup[s.Group] = s.Choices
	}

	known := map[string]bool{}
	for _, g := range groups {
		known[g.ID] = true
	}
	for gid := range byGroup {
		if !known[gid] {
			return PricedOptions{}, Errf("Groupe d'options inconnu : %q.", gid)
		}
	}

	res := PricedOptions{UnitPrice: basePrice}
	var labels []string

	for _, g := range groups {
		chosen := byGroup[g.ID]
		if len(chosen) < g.Min {
			if g.Min == 1 {
				return PricedOptions{}, Errf("Choisissez une option pour « %s ».", g.Name)
			}
			return PricedOptions{}, Errf("Choisissez au moins %d options pour « %s ».", g.Min, g.Name)
		}
		if g.Max > 0 && len(chosen) > g.Max {
			return PricedOptions{}, Errf("Au maximum %d option(s) pour « %s ».", g.Max, g.Name)
		}
		if len(chosen) == 0 {
			continue
		}

		picked := map[string]bool{}
		for _, cid := range chosen {
			if picked[cid] {
				return PricedOptions{}, Errf("Option en double %q pour « %s ».", cid, g.Name)
			}
			picked[cid] = true
		}

		var ids []string
		for _, c := range g.Choices { // definition order
			if !picked[c.ID] {
				continue
			}
			delete(picked, c.ID)
			res.UnitPrice += c.Price
			labels = append(labels, c.Name)
			ids = append(ids, c.ID)
		}
		if len(picked) > 0 {
			unknown := make([]string, 0, len(picked))
			for cid := range picked {
				unknown = append(unknown, cid)
			}
			sort.Strings(unknown)
			return PricedOptions{}, Errf("Option inconnue %q pour « %s ».", unknown[0], g.Name)
		}

		res.Selected = append(res.Selected, SelectedOption{Group: g.ID, Choices: ids})
	}

	res.Label = strings.Join(labels, ", ")
	res.Key = SelectionKey(res.Selected)
	return res, nil
}

// SelectionKey returns the canonical key of a (normalized) selection, used to
// merge identical lines in the consolidated cart.
func SelectionKey(sel []SelectedOption) string {
	keys := make([]string, 0, len(sel))
	for _, s := range sel {
		if len(s.Choices) == 0 {
			continue
		}
		keys = append(keys, s.Group+"="+strings.Join(s.Choices, ","))
	}
	return strings.Join(keys, ";")
}

// SplitMode values.
const (
	SplitEqual        = "equal"
	SplitProportional = "proportional"
)

// Share is one participant's weight in a fee split.
type Share struct {
	ID       string
	Subtotal int
}

// SplitFees splits `total` cents between the given participants so that the
// returned shares sum exactly to total, using the largest remainder method.
//
// equal: identical weights; proportional: weighted by subtotal (falls back to
// equal when all subtotals are zero). Ties on the remainder are broken by
// ascending id, so the result is stable.
func SplitFees(total int, mode string, participants []Share) map[string]int {
	out := make(map[string]int, len(participants))
	if len(participants) == 0 {
		return out
	}

	ps := make([]Share, len(participants))
	copy(ps, participants)
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].ID < ps[j].ID })

	weights := make([]int64, len(ps))
	var sumW int64
	for i, p := range ps {
		w := int64(1)
		if mode == SplitProportional {
			w = int64(max(p.Subtotal, 0))
		}
		weights[i] = w
		sumW += w
	}
	if sumW == 0 {
		for i := range weights {
			weights[i] = 1
		}
		sumW = int64(len(weights))
	}

	sign := int64(1)
	t := int64(total)
	if t < 0 {
		sign, t = -1, -t
	}

	type rem struct {
		idx int
		r   int64
	}
	rems := make([]rem, len(ps))
	var allocated int64
	base := make([]int64, len(ps))
	for i, w := range weights {
		base[i] = t * w / sumW
		rems[i] = rem{idx: i, r: t * w % sumW}
		allocated += base[i]
	}
	sort.SliceStable(rems, func(i, j int) bool { return rems[i].r > rems[j].r })
	for k := int64(0); k < t-allocated; k++ {
		base[rems[k].idx]++
	}

	for i, p := range ps {
		out[p.ID] = int(sign * base[i])
	}
	return out
}

// FormatAmount formats cents as "12.50" (dot separator, no currency).
func FormatAmount(cents int) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// FormatEUR formats cents for French display: "12,50 €".
func FormatEUR(cents int) string {
	return strings.Replace(FormatAmount(cents), ".", ",", 1) + " €"
}
