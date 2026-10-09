package domain

import (
	"sort"
	"strings"
)

// Party statuses.
const (
	StatusLobby     = "lobby"
	StatusVoting    = "voting"
	StatusOrdering  = "ordering"
	StatusReview    = "review"
	StatusPaying    = "paying"
	StatusClosed    = "closed"
	StatusCancelled = "cancelled"
)

// Statuses lists every party status in lifecycle order.
var Statuses = []string{StatusLobby, StatusVoting, StatusOrdering, StatusReview, StatusPaying, StatusClosed, StatusCancelled}

// IsValidStatus reports whether s is a known status.
func IsValidStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

// TransitionInput carries the facts needed to validate a transition.
type TransitionInput struct {
	From string
	To   string
	// Candidates is the number of restaurants submitted to the vote.
	Candidates int
	// RequestedRestaurant is the restaurant explicitly chosen by the host ("" if none).
	RequestedRestaurant string
	// RequestedIsCandidate tells whether RequestedRestaurant is in the candidates.
	RequestedIsCandidate bool
	// Items is the number of order items in the party.
	Items int
}

// ValidateTransition checks a host initiated transition against the state machine.
// Note: review → paying is only possible through the payer endpoint.
func ValidateTransition(in TransitionInput) error {
	if !IsValidStatus(in.To) {
		return Errf("Statut cible inconnu : %q.", in.To)
	}
	if in.From == in.To {
		return Errf("La commande est déjà dans ce statut.")
	}

	if in.To == StatusCancelled {
		if in.From == StatusClosed || in.From == StatusCancelled {
			return Errf("Une commande clôturée ou annulée ne peut plus être annulée.")
		}
		return nil
	}

	switch {
	case in.From == StatusLobby && in.To == StatusVoting:
		if in.Candidates < 2 {
			return Errf("Il faut au moins 2 restaurants candidats pour lancer le vote.")
		}
		return nil
	case in.From == StatusLobby && in.To == StatusOrdering:
		if in.RequestedRestaurant == "" {
			return Errf("Choisissez un restaurant pour passer directement à la commande.")
		}
		return nil
	case in.From == StatusVoting && in.To == StatusOrdering:
		if in.RequestedRestaurant != "" && !in.RequestedIsCandidate {
			return Errf("Le restaurant imposé doit faire partie des candidats.")
		}
		if in.RequestedRestaurant == "" && in.Candidates == 0 {
			return Errf("Aucun restaurant candidat.")
		}
		return nil
	case in.From == StatusOrdering && in.To == StatusReview:
		if in.Items < 1 {
			return Errf("Le panier est vide : ajoutez au moins un article.")
		}
		return nil
	case in.From == StatusReview && in.To == StatusOrdering:
		return nil
	case in.From == StatusReview && in.To == StatusPaying:
		return Errf("Désignez le payeur pour passer au remboursement.")
	case in.From == StatusPaying && in.To == StatusClosed:
		return nil
	}
	return Errf("Transition impossible de « %s » vers « %s ».", in.From, in.To)
}

// Candidate is a restaurant submitted to the vote.
type Candidate struct {
	ID     string
	Name   string
	Rating float64
}

// ElectWinner returns the candidate with the most (approval) votes.
// Ties are broken by higher rating, then by name (case-insensitive), then id.
// votes maps restaurant id → number of votes. Returns "" when no candidate.
func ElectWinner(candidates []Candidate, votes map[string]int) string {
	if len(candidates) == 0 {
		return ""
	}
	cs := make([]Candidate, len(candidates))
	copy(cs, candidates)
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if votes[a.ID] != votes[b.ID] {
			return votes[a.ID] > votes[b.ID]
		}
		if a.Rating != b.Rating {
			return a.Rating > b.Rating
		}
		an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if an != bn {
			return an < bn
		}
		return a.ID < b.ID
	})
	return cs[0].ID
}
