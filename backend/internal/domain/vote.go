package domain

import (
	"sort"
	"strings"
)

// Ranked vote (ADR 0005 « Vote par classement »).
//
// Each member ranks the candidates they like: 1 = favourite. Points per
// ballot line follow a truncated Borda count:
//
//	points = K − rank + 1   (at least 1)
//
// where K is the number of candidates. With 4 candidates a 1st choice is
// worth 4 points, a 2nd 3, a 3rd 2, a 4th 1; unranked restaurants get 0.
// Ranking a restaurant therefore always helps it, and never more than the
// restaurants ranked above it.
//
// The winner has the most points; ties → more 1st choices, then more voters
// (distinct members who ranked it), then the better rating, then the name
// (case-insensitive), then the id.

// MaxBallot is the longest ranking a member can submit (= the maximum
// number of candidates of a party).
const MaxBallot = 20

// RankedVote is one line of a member's ballot.
type RankedVote struct {
	User       string
	Restaurant string
	Rank       int
}

// BallotPoints returns the points of a ballot line of rank `rank` when
// `candidates` restaurants are in the running (0 for an invalid rank).
func BallotPoints(candidates, rank int) int {
	if rank < 1 {
		return 0
	}
	if p := candidates - rank + 1; p > 1 {
		return p
	}
	return 1
}

// Standing is the score of one candidate.
type Standing struct {
	Restaurant   string `json:"restaurant"`
	Points       int    `json:"points"`
	FirstChoices int    `json:"firstChoices"`
	Voters       int    `json:"voters"`
}

// Tally is the live result of a vote, computed by the server only.
type Tally struct {
	// Candidates is K: the points of a 1st choice.
	Candidates int `json:"candidates"`
	// Voters is the number of members with at least one ranked restaurant.
	Voters int `json:"voters"`
	// Standings: every candidate, best first (the winner rule).
	Standings []Standing `json:"standings"`
	// Winner is the restaurant elected if the vote closed now ("" when there
	// is no candidate). Without any vote it falls back to rating, then name.
	Winner string `json:"winner"`
}

// ComputeTally scores the candidates from the ranked votes. Votes for a
// restaurant that is not a candidate (no longer active…) score nothing but
// their voter still counts as having voted.
func ComputeTally(candidates []Candidate, votes []RankedVote) Tally {
	k := len(candidates)
	byID := make(map[string]*Standing, k)
	voterSets := make(map[string]map[string]bool, k)
	out := Tally{Candidates: k, Standings: make([]Standing, 0, k)}
	for _, c := range candidates {
		byID[c.ID] = &Standing{Restaurant: c.ID}
		voterSets[c.ID] = map[string]bool{}
	}
	voters := map[string]bool{}
	for _, v := range votes {
		voters[v.User] = true
		s := byID[v.Restaurant]
		if s == nil {
			continue
		}
		s.Points += BallotPoints(k, v.Rank)
		if v.Rank == 1 {
			s.FirstChoices++
		}
		voterSets[v.Restaurant][v.User] = true
	}
	out.Voters = len(voters)

	cs := make([]Candidate, len(candidates))
	copy(cs, candidates)
	for _, c := range cs {
		byID[c.ID].Voters = len(voterSets[c.ID])
	}
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		sa, sb := byID[a.ID], byID[b.ID]
		if sa.Points != sb.Points {
			return sa.Points > sb.Points
		}
		if sa.FirstChoices != sb.FirstChoices {
			return sa.FirstChoices > sb.FirstChoices
		}
		if sa.Voters != sb.Voters {
			return sa.Voters > sb.Voters
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
	for _, c := range cs {
		out.Standings = append(out.Standings, *byID[c.ID])
	}
	if len(cs) > 0 {
		out.Winner = cs[0].ID
	}
	return out
}

// ElectWinner returns the candidate elected by the ranked votes ("" when
// there is no candidate). Same rule for the host and the deadline scheduler.
func ElectWinner(candidates []Candidate, votes []RankedVote) string {
	return ComputeTally(candidates, votes).Winner
}

// ValidateBallot checks a member's ranking (restaurant ids, favourite
// first) against the party candidates: candidates only, no duplicate, at
// most MaxBallot lines. An empty ranking withdraws the ballot.
func ValidateBallot(ranking, candidates []string) error {
	if len(ranking) > MaxBallot {
		return Errf("Ton classement compte au plus %d restaurants.", MaxBallot)
	}
	allowed := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		allowed[c] = true
	}
	seen := make(map[string]bool, len(ranking))
	for _, id := range ranking {
		if !allowed[id] {
			return Errf("Ce restaurant ne fait pas partie des candidats.")
		}
		if seen[id] {
			return Errf("Un restaurant ne peut apparaître qu'une fois dans ton classement.")
		}
		seen[id] = true
	}
	return nil
}
