package migrations

import (
	"testing"

	"github.com/pocketbase/dbx"
)

func TestRankedVotesMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)

	votes, err := app.FindCollectionByNameOrId("votes")
	if err != nil {
		t.Fatal(err)
	}
	if votes.CreateRule != nil || votes.UpdateRule != nil || votes.DeleteRule != nil {
		t.Fatal("votes must be written by the server only (PUT /ballot)")
	}
	if votes.ListRule == nil || votes.ViewRule == nil {
		t.Fatal("members must still read the votes")
	}
	if votes.Fields.GetByName("rank") == nil || votes.GetIndex(votesRankIndex) == "" {
		t.Fatal("votes.rank and its unique index are required")
	}

	// back to the pre-migration state (no unique rank) to insert legacy rows
	votes.RemoveIndex(votesRankIndex)
	if err := app.Save(votes); err != nil {
		t.Fatal(err)
	}

	// legacy approval votes (rank 0) → ranked by creation order per member;
	// an already ranked ballot is left alone
	ins := func(id, party, user, rest, created string, rank int) {
		t.Helper()
		if _, err := app.DB().Insert("votes", dbx.Params{
			"id": id, "party": party, "user": user, "restaurant": rest, "client_key": "",
			"rank": rank, "created": created, "updated": created,
		}).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	ins("vote00000000001", "p1", "ana", "r2", "2026-10-09 10:00:02.000Z", 0)
	ins("vote00000000002", "p1", "ana", "r1", "2026-10-09 10:00:01.000Z", 0)
	ins("vote00000000003", "p1", "ana", "r3", "2026-10-09 10:00:02.000Z", 0)
	ins("vote00000000004", "p1", "ben", "r3", "2026-10-09 10:00:00.000Z", 0)
	ins("vote00000000005", "p2", "ana", "r1", "2026-10-09 10:00:00.000Z", 0)
	ins("vote00000000006", "p3", "cat", "r1", "2026-10-09 10:00:05.000Z", 2)
	ins("vote00000000007", "p3", "cat", "r2", "2026-10-09 10:00:00.000Z", 1)

	for run := 0; run < 2; run++ { // idempotent
		if err := upRankedVotes(app); err != nil {
			t.Fatal(err)
		}
		want := map[string]int{
			"vote00000000002": 1, "vote00000000001": 2, "vote00000000003": 3, // same instant → id
			"vote00000000004": 1, "vote00000000005": 1,
			"vote00000000006": 2, "vote00000000007": 1,
		}
		for id, rank := range want {
			var got int
			if err := app.DB().NewQuery("SELECT rank FROM votes WHERE id = {:id}").Bind(dbx.Params{"id": id}).Row(&got); err != nil {
				t.Fatal(err)
			}
			if got != rank {
				t.Fatalf("run %d: %s rank %d, want %d", run, id, got, rank)
			}
		}
	}

	// ranks are unique per (party, user)
	if _, err := app.DB().Insert("votes", dbx.Params{
		"id": "vote00000000008", "party": "p2", "user": "ana", "restaurant": "r2", "client_key": "", "rank": 1,
	}).Execute(); err == nil {
		t.Fatal("duplicate rank must be refused")
	}
}
