package domain

import "time"

// ReminderLead is how long before a deadline the reminder is sent.
const ReminderLead = 2 * time.Minute

// VoteExtension is the extra time given once when nobody voted.
const VoteExtension = 5 * time.Minute

// DeadlineFlags are the scheduler's memory for a party (parties.auto_state):
// reminders and decisions are keyed by the deadline they were made for, so
// that a new deadline set by the host starts afresh and a restart never
// repeats an action.
type DeadlineFlags struct {
	VoteRemindedFor  string `json:"voteRemindedFor,omitempty"`
	OrderRemindedFor string `json:"orderRemindedFor,omitempty"`
	VoteExtended     bool   `json:"voteExtended,omitempty"`
	VoteGaveUpFor    string `json:"voteGaveUpFor,omitempty"`
	OrderGaveUpFor   string `json:"orderGaveUpFor,omitempty"`
}

// DeadlineState is what the scheduler knows about a party at a given time.
type DeadlineState struct {
	Status            string
	VotingEndsAt      time.Time // zero = no deadline
	OrderingEndsAt    time.Time
	AutoCloseDisabled bool
	Votes             int // votes cast in the party
	Items             int // order items in the party
	Flags             DeadlineFlags
}

// DeadlineAction is what the scheduler must do now.
type DeadlineAction string

// Scheduler actions.
const (
	ActNone          DeadlineAction = ""
	ActRemindVote    DeadlineAction = "remind_vote"
	ActRemindOrder   DeadlineAction = "remind_order"
	ActCloseVote     DeadlineAction = "close_vote"     // voting → ordering (winner)
	ActExtendVote    DeadlineAction = "extend_vote"    // no vote: +5 min, once
	ActGiveUpVote    DeadlineAction = "give_up_vote"   // still no vote after the extension: host decides
	ActCloseOrdering DeadlineAction = "close_ordering" // ordering → review
	ActGiveUpOrder   DeadlineAction = "give_up_order"  // empty carts at the deadline: host decides
)

// DeadlineKey identifies a deadline in the flags (UTC, second precision).
func DeadlineKey(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// PlanDeadline returns the single action due at now (ActNone if nothing).
// It is pure and idempotent: once the matching flag is recorded (or the
// status changed), the same action is never returned again.
func PlanDeadline(s DeadlineState, now time.Time) DeadlineAction {
	switch s.Status {
	case StatusVoting:
		end := s.VotingEndsAt
		if end.IsZero() {
			return ActNone
		}
		key := DeadlineKey(end)
		if !now.Before(end) {
			if s.AutoCloseDisabled {
				return ActNone
			}
			if s.Votes > 0 {
				return ActCloseVote
			}
			if !s.Flags.VoteExtended {
				return ActExtendVote
			}
			if s.Flags.VoteGaveUpFor != key {
				return ActGiveUpVote
			}
			return ActNone
		}
		if !now.Before(end.Add(-ReminderLead)) && s.Flags.VoteRemindedFor != key {
			return ActRemindVote
		}
	case StatusOrdering:
		end := s.OrderingEndsAt
		if end.IsZero() {
			return ActNone
		}
		key := DeadlineKey(end)
		if !now.Before(end) {
			if s.AutoCloseDisabled {
				return ActNone
			}
			if s.Items > 0 {
				return ActCloseOrdering
			}
			if s.Flags.OrderGaveUpFor != key {
				return ActGiveUpOrder
			}
			return ActNone
		}
		if !now.Before(end.Add(-ReminderLead)) && s.Flags.OrderRemindedFor != key {
			return ActRemindOrder
		}
	}
	return ActNone
}

// AutoEvent is an entry of parties.auto_events, shown in the party.
type AutoEvent struct {
	Kind string `json:"kind"`
	At   string `json:"at"` // RFC 3339, UTC
	Text string `json:"text"`
}

// MaxAutoEvents bounds parties.auto_events.
const MaxAutoEvents = 20

// AppendAutoEvent adds e and keeps the last MaxAutoEvents entries.
func AppendAutoEvent(list []AutoEvent, e AutoEvent) []AutoEvent {
	list = append(list, e)
	if len(list) > MaxAutoEvents {
		list = list[len(list)-MaxAutoEvents:]
	}
	return list
}
