package domain

import (
	"testing"
	"time"
)

func TestPlanDeadline(t *testing.T) {
	end := time.Date(2026, 10, 9, 9, 45, 0, 0, time.UTC)
	key := DeadlineKey(end)
	before := func(d time.Duration) time.Time { return end.Add(-d) }

	cases := []struct {
		name string
		s    DeadlineState
		now  time.Time
		want DeadlineAction
	}{
		{"no deadline", DeadlineState{Status: StatusVoting}, end, ActNone},
		{"vote: too early", DeadlineState{Status: StatusVoting, VotingEndsAt: end}, before(3 * time.Minute), ActNone},
		{"vote: reminder window", DeadlineState{Status: StatusVoting, VotingEndsAt: end}, before(2 * time.Minute), ActRemindVote},
		{"vote: reminder once", DeadlineState{Status: StatusVoting, VotingEndsAt: end, Flags: DeadlineFlags{VoteRemindedFor: key}}, before(time.Minute), ActNone},
		{"vote: reminder again after a new deadline", DeadlineState{Status: StatusVoting, VotingEndsAt: end, Flags: DeadlineFlags{VoteRemindedFor: "2026-10-09T09:30:00Z"}}, before(time.Minute), ActRemindVote},
		{"vote: close with votes", DeadlineState{Status: StatusVoting, VotingEndsAt: end, Votes: 2}, end, ActCloseVote},
		{"vote: close long after (restart)", DeadlineState{Status: StatusVoting, VotingEndsAt: end, Votes: 1, Flags: DeadlineFlags{VoteRemindedFor: key}}, end.Add(time.Hour), ActCloseVote},
		{"vote: auto close disabled, reminder still sent", DeadlineState{Status: StatusVoting, VotingEndsAt: end, AutoCloseDisabled: true}, before(time.Minute), ActRemindVote},
		{"vote: auto close disabled", DeadlineState{Status: StatusVoting, VotingEndsAt: end, AutoCloseDisabled: true, Votes: 3}, end, ActNone},
		{"vote: no vote → extend once", DeadlineState{Status: StatusVoting, VotingEndsAt: end}, end, ActExtendVote},
		{"vote: no vote after extension → host", DeadlineState{Status: StatusVoting, VotingEndsAt: end, Flags: DeadlineFlags{VoteExtended: true}}, end, ActGiveUpVote},
		{"vote: gave up once", DeadlineState{Status: StatusVoting, VotingEndsAt: end, Flags: DeadlineFlags{VoteExtended: true, VoteGaveUpFor: key}}, end.Add(time.Minute), ActNone},
		{"vote: votes after giving up → close", DeadlineState{Status: StatusVoting, VotingEndsAt: end, Votes: 1, Flags: DeadlineFlags{VoteExtended: true, VoteGaveUpFor: key}}, end.Add(time.Minute), ActCloseVote},
		{"ordering deadline ignored while voting", DeadlineState{Status: StatusVoting, OrderingEndsAt: end}, end, ActNone},
		{"order: reminder", DeadlineState{Status: StatusOrdering, OrderingEndsAt: end}, before(90 * time.Second), ActRemindOrder},
		{"order: reminder once", DeadlineState{Status: StatusOrdering, OrderingEndsAt: end, Flags: DeadlineFlags{OrderRemindedFor: key}}, before(30 * time.Second), ActNone},
		{"order: close", DeadlineState{Status: StatusOrdering, OrderingEndsAt: end, Items: 4}, end.Add(time.Second), ActCloseOrdering},
		{"order: empty → host once", DeadlineState{Status: StatusOrdering, OrderingEndsAt: end}, end, ActGiveUpOrder},
		{"order: empty, already told", DeadlineState{Status: StatusOrdering, OrderingEndsAt: end, Flags: DeadlineFlags{OrderGaveUpFor: key}}, end, ActNone},
		{"order: disabled", DeadlineState{Status: StatusOrdering, OrderingEndsAt: end, Items: 1, AutoCloseDisabled: true}, end, ActNone},
		{"review: nothing", DeadlineState{Status: StatusReview, OrderingEndsAt: end, Items: 1}, end, ActNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PlanDeadline(c.s, c.now); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestAppendAutoEvent(t *testing.T) {
	var list []AutoEvent
	for i := 0; i < MaxAutoEvents+5; i++ {
		list = AppendAutoEvent(list, AutoEvent{Kind: "k", Text: string(rune('a' + i%26))})
	}
	if len(list) != MaxAutoEvents || list[len(list)-1].Text != string(rune('a'+(MaxAutoEvents+4)%26)) {
		t.Fatalf("bounded list: %d %v", len(list), list[len(list)-1])
	}
	if DeadlineKey(time.Time{}) != "" {
		t.Fatal("zero deadline key")
	}
}
