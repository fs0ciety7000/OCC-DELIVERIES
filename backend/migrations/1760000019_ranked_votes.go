package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Ranked vote (ADR 0005): every vote gets a `rank` (1 = favourite) and a
// member's ranks are unique per party. Votes are written only through
// `PUT /api/occ/parties/{id}/ballot` (the whole ballot, in a transaction),
// so the collection create / delete rules become superuser only.
//
// Legacy approval votes (parties still voting) are ranked by creation order
// per member: the first restaurant liked becomes the 1st choice.

func init() {
	m.Register(upRankedVotes, downRankedVotes)
}

const votesRankIndex = "idx_votes_party_user_rank"

func upRankedVotes(app core.App) error {
	votes, err := app.FindCollectionByNameOrId("votes")
	if err != nil {
		return err
	}
	if votes.Fields.GetByName("rank") == nil {
		votes.Fields.Add(&core.NumberField{Name: "rank", OnlyInt: true, Min: fptr(1), Max: fptr(20), Required: true})
	}
	votes.CreateRule = nil
	votes.UpdateRule = nil
	votes.DeleteRule = nil
	if err := app.Save(votes); err != nil {
		return err
	}

	// rank legacy votes by creation order, only in groups (party, user) that
	// still hold unranked rows; negative first so that the unique index (if
	// already there, re-run) never sees a transient duplicate
	if _, err := app.DB().NewQuery(`UPDATE votes SET rank = -(
			SELECT COUNT(*) FROM votes v2
			WHERE v2.party = votes.party AND v2.user = votes.user
			  AND (v2.created < votes.created OR (v2.created = votes.created AND v2.id <= votes.id)))
		WHERE EXISTS (SELECT 1 FROM votes v3 WHERE v3.party = votes.party AND v3.user = votes.user AND v3.rank < 1)`).Execute(); err != nil {
		return err
	}
	if _, err := app.DB().NewQuery(`UPDATE votes SET rank = -rank WHERE rank < 0`).Execute(); err != nil {
		return err
	}

	if votes.GetIndex(votesRankIndex) == "" {
		votes.AddIndex(votesRankIndex, true, "party, user, rank", "")
		return app.Save(votes)
	}
	return nil
}

func downRankedVotes(app core.App) error {
	votes, err := app.FindCollectionByNameOrId("votes")
	if err != nil {
		return err
	}
	votes.RemoveIndex(votesRankIndex)
	votes.Fields.RemoveByName("rank")
	votes.CreateRule = ptr(`user = @request.auth.id && party.members.id ?= @request.auth.id && party.status = "voting"`)
	votes.DeleteRule = ptr(`user = @request.auth.id && party.status = "voting"`)
	return app.Save(votes)
}
