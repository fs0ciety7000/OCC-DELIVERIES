package app

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Ranked vote (ADR 0005): a member's ballot is the ordered list of the
// candidates they like, stored as one `votes` row per restaurant with its
// `rank`. The ballot is only written here, whole, in a transaction.

func (h *handlers) voteRoutes(g *router.RouterGroup[*core.RequestEvent], user *hookHandler) {
	g.PUT("/parties/{id}/ballot", h.putBallot).Bind(user)
	g.GET("/parties/{id}/tally", h.getTally).Bind(user)
}

// ballot keys: the vote rows carry `<clientKey>_<rank>` (client_key ≤ 64)
var ballotKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,56}$`)

// putBallot: PUT /api/occ/parties/{id}/ballot {"ranking": [restaurantId…], "clientKey"?}
// replaces the member's whole ballot (empty ranking = withdraw). Idempotent:
// the same ranking (or a replay of the same client key) writes nothing.
func (h *handlers) putBallot(e *core.RequestEvent) error {
	var body struct {
		Ranking   []string `json:"ranking"`
		ClientKey string   `json:"clientKey"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	key := strings.TrimSpace(body.ClientKey)
	if key != "" && !ballotKeyRe.MatchString(key) {
		return badRequest("Clé d'idempotence invalide.")
	}
	ranking := make([]string, 0, len(body.Ranking))
	for _, id := range body.Ranking {
		ranking = append(ranking, strings.TrimSpace(id))
	}
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	uid := e.Auth.Id

	var tally domain.Tally
	err = e.App.RunInTransaction(func(tx core.App) error {
		p, err := tx.FindRecordById(colParties, party.Id)
		if err != nil {
			return err
		}
		if p.GetString("status") != domain.StatusVoting {
			return badRequest("Le vote n'est pas ouvert.")
		}
		if !isMember(p, uid) {
			return errNotMember()
		}
		candidates := p.GetStringSlice("candidates")
		if err := domain.ValidateBallot(ranking, candidates); err != nil {
			return toAPIError(err)
		}

		mine, err := tx.FindRecordsByFilter(colVotes, "party = {:p} && user = {:u}", "rank", 0, 0, dbx.Params{"p": p.Id, "u": uid})
		if err != nil {
			return err
		}
		current := make([]string, 0, len(mine))
		for _, v := range mine {
			current = append(current, v.GetString("restaurant"))
		}
		replay := key != "" && len(mine) > 0 && mine[0].GetString("client_key") == key+"_1"
		if !replay && !slices.Equal(current, ranking) {
			// whole ballot replaced: delete then insert (the unique rank index
			// forbids swapping ranks row by row)
			for _, v := range mine {
				if err := tx.Delete(v); err != nil {
					return err
				}
			}
			col, err := tx.FindCollectionByNameOrId(colVotes)
			if err != nil {
				return err
			}
			for i, rest := range ranking {
				v := core.NewRecord(col)
				v.Load(map[string]any{"party": p.Id, "user": uid, "restaurant": rest, "rank": i + 1})
				if key != "" {
					v.Set("client_key", key+"_"+strconv.Itoa(i+1))
				}
				if err := tx.SaveWithContext(actorContext(e), v); err != nil {
					return err
				}
			}
		}
		tally, err = partyTally(tx, p.Id, candidates)
		return err
	})
	if err != nil {
		return err
	}

	ballot, err := myBallot(e.App, party.Id, uid)
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"ballot": ballot, "tally": tally})
}

// getTally: GET /api/occ/parties/{id}/tally — live standings (any status:
// after the vote it shows how the restaurant was elected).
func (h *handlers) getTally(e *core.RequestEvent) error {
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	tally, err := partyTally(e.App, party.Id, party.GetStringSlice("candidates"))
	if err != nil {
		return err
	}
	return ok(e, tally)
}

func myBallot(app core.App, partyID, userID string) ([]string, error) {
	recs, err := app.FindRecordsByFilter(colVotes, "party = {:p} && user = {:u}", "rank", 0, 0, dbx.Params{"p": partyID, "u": userID})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.GetString("restaurant"))
	}
	return out, nil
}

// partyTally scores the active candidates of a party from its ranked votes.
func partyTally(app core.App, partyID string, candidates []string) (domain.Tally, error) {
	recs, err := app.FindRecordsByIds(colRestaurants, candidates)
	if err != nil {
		return domain.Tally{}, err
	}
	// keep the candidates order (stable display), active restaurants only
	byID := make(map[string]*core.Record, len(recs))
	for _, r := range recs {
		byID[r.Id] = r
	}
	cands := make([]domain.Candidate, 0, len(recs))
	for _, id := range candidates {
		if r := byID[id]; r != nil && r.GetBool("active") {
			cands = append(cands, domain.Candidate{ID: r.Id, Name: r.GetString("name"), Rating: r.GetFloat("rating")})
		}
	}
	votes, err := app.FindAllRecords(colVotes, dbx.HashExp{"party": partyID})
	if err != nil {
		return domain.Tally{}, err
	}
	ranked := make([]domain.RankedVote, 0, len(votes))
	for _, v := range votes {
		ranked = append(ranked, domain.RankedVote{User: v.GetString("user"), Restaurant: v.GetString("restaurant"), Rank: v.GetInt("rank")})
	}
	return domain.ComputeTally(cands, ranked), nil
}
