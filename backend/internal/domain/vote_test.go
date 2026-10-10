package domain

import (
	"reflect"
	"testing"
)

func TestBallotPoints(t *testing.T) {
	tests := []struct {
		k, rank, want int
	}{
		{4, 1, 4}, {4, 2, 3}, {4, 3, 2}, {4, 4, 1},
		{4, 5, 1}, // candidate list shrank: a ranked restaurant keeps 1 point
		{3, 0, 0}, {3, -1, 0},
		{2, 1, 2}, {2, 2, 1},
	}
	for _, tt := range tests {
		if got := BallotPoints(tt.k, tt.rank); got != tt.want {
			t.Errorf("BallotPoints(%d, %d) = %d, want %d", tt.k, tt.rank, got, tt.want)
		}
	}
}

func TestComputeTally(t *testing.T) {
	// the user's example: Pitta Shop 1st, Akropolis 2nd, Buga Ramen 3rd
	pitta := Candidate{ID: "pitta", Name: "Pitta Shop", Rating: 4.1}
	akro := Candidate{ID: "akro", Name: "Akropolis", Rating: 4.5}
	buga := Candidate{ID: "buga", Name: "Buga Ramen", Rating: 4.5}
	pizza := Candidate{ID: "pizza", Name: "Pizza", Rating: 3.9}
	cands := []Candidate{pitta, akro, buga, pizza}
	v := func(user, rest string, rank int) RankedVote {
		return RankedVote{User: user, Restaurant: rest, Rank: rank}
	}

	tests := []struct {
		name       string
		votes      []RankedVote
		wantOrder  []string
		wantPoints []int
		voters     int
	}{
		{
			"one full ballot: 4/3/2 points, unranked 0",
			[]RankedVote{v("ana", "pitta", 1), v("ana", "akro", 2), v("ana", "buga", 3)},
			[]string{"pitta", "akro", "buga", "pizza"}, []int{4, 3, 2, 0}, 1,
		},
		{
			"a strong consensus 2nd choice beats a divisive 1st",
			[]RankedVote{
				v("ana", "pitta", 1), v("ana", "akro", 2),
				v("ben", "pizza", 1), v("ben", "akro", 2),
				v("cat", "pitta", 1), v("cat", "akro", 2),
			},
			// akro 9, pitta 8, pizza 4
			[]string{"akro", "pitta", "pizza", "buga"}, []int{9, 8, 4, 0}, 3,
		},
		{
			"points tie → more first choices",
			[]RankedVote{
				v("ana", "buga", 1),                       // buga 4
				v("ben", "akro", 2), v("ben", "pizza", 1), // akro 3, pizza 4
				v("cat", "akro", 4), // akro 3+1 = 4
			},
			// buga 4 (1 first), pizza 4 (1 first), akro 4 (0 first) → buga/pizza by voters (1/1) → rating buga
			[]string{"buga", "pizza", "akro", "pitta"}, []int{4, 4, 4, 0}, 3,
		},
		{
			"points and firsts tie → more voters",
			[]RankedVote{
				v("ana", "pitta", 3), v("ben", "pitta", 3), // pitta 2+2 = 4, 2 voters
				v("cat", "pizza", 1), // pizza 4, 1 first
				v("dan", "akro", 1),  // akro 4, 1 first
			},
			// pizza & akro: 4 pts 1 first 1 voter → rating akro; pitta 4 pts 0 first
			[]string{"akro", "pizza", "pitta", "buga"}, []int{4, 4, 4, 0}, 4,
		},
		{
			"no vote → rating, then name",
			nil,
			[]string{"akro", "buga", "pitta", "pizza"}, []int{0, 0, 0, 0}, 0,
		},
		{
			"vote for a non-candidate: no points, but the voter counts",
			[]RankedVote{v("ana", "gone", 1), v("ana", "pizza", 2)},
			[]string{"pizza", "akro", "buga", "pitta"}, []int{3, 0, 0, 0}, 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeTally(cands, tt.votes)
			var order []string
			var points []int
			for _, s := range got.Standings {
				order = append(order, s.Restaurant)
				points = append(points, s.Points)
			}
			if !reflect.DeepEqual(order, tt.wantOrder) || !reflect.DeepEqual(points, tt.wantPoints) {
				t.Fatalf("order %v points %v, want %v %v", order, points, tt.wantOrder, tt.wantPoints)
			}
			if got.Winner != tt.wantOrder[0] || got.Voters != tt.voters || got.Candidates != 4 {
				t.Fatalf("winner %q voters %d candidates %d", got.Winner, got.Voters, got.Candidates)
			}
			if w := ElectWinner(cands, tt.votes); w != got.Winner {
				t.Fatalf("ElectWinner %q ≠ tally winner %q", w, got.Winner)
			}
		})
	}

	t.Run("first choices and voters per candidate", func(t *testing.T) {
		got := ComputeTally(cands, []RankedVote{v("ana", "pitta", 1), v("ben", "pitta", 2), v("cat", "pitta", 1)})
		if s := got.Standings[0]; s.Restaurant != "pitta" || s.Points != 11 || s.FirstChoices != 2 || s.Voters != 3 {
			t.Fatalf("standing %+v", s)
		}
	})
	t.Run("no candidate", func(t *testing.T) {
		got := ComputeTally(nil, []RankedVote{v("ana", "pitta", 1)})
		if got.Winner != "" || len(got.Standings) != 0 || got.Standings == nil {
			t.Fatalf("empty tally %+v", got)
		}
	})
	t.Run("same name → id", func(t *testing.T) {
		a, b := Candidate{ID: "b", Name: "Same"}, Candidate{ID: "a", Name: "same"}
		if w := ElectWinner([]Candidate{a, b}, nil); w != "a" {
			t.Fatalf("got %s", w)
		}
	})
}

func TestValidateBallot(t *testing.T) {
	cands := []string{"r1", "r2", "r3"}
	long := make([]string, MaxBallot+1)
	for i := range long {
		long[i] = "r1"
	}
	tests := []struct {
		name    string
		ranking []string
		ok      bool
	}{
		{"empty = withdraw", nil, true},
		{"partial", []string{"r2"}, true},
		{"full", []string{"r3", "r1", "r2"}, true},
		{"not a candidate", []string{"r1", "rX"}, false},
		{"duplicate", []string{"r1", "r2", "r1"}, false},
		{"too long", long, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBallot(tt.ranking, cands)
			if (err == nil) != tt.ok {
				t.Fatalf("ok=%v err=%v", tt.ok, err)
			}
			if err != nil && !IsDomainError(err) {
				t.Fatalf("expected a domain error, got %T", err)
			}
		})
	}
}
