package app

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/notify"
)

const deadlinesJobID = "occDeadlines"

// deadlineScheduler enforces the « Fin du vote » / « Fin de la commande »
// deadlines: reminders 2 minutes before, automatic transitions at the
// deadline. It runs every minute (PocketBase cron); every decision is
// recorded in parties.auto_state, so ticks are idempotent and survive
// restarts.
type deadlineScheduler struct {
	app  core.App
	push *pushService
	now  func() time.Time
	mu   sync.Mutex
}

func newDeadlineScheduler(app core.App, push *pushService, now func() time.Time) *deadlineScheduler {
	if now == nil {
		now = time.Now
	}
	return &deadlineScheduler{app: app, push: push, now: now}
}

func (d *deadlineScheduler) bind() {
	d.app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		d.app = se.App
		if err := se.App.Cron().Add(deadlinesJobID, "* * * * *", func() { d.tick() }); err != nil {
			return err
		}
		return se.Next()
	})
}

// tick processes every voting / ordering party with a deadline; it returns
// the number of actions taken.
func (d *deadlineScheduler) tick() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	parties, err := d.app.FindAllRecords(colParties,
		dbx.Or(
			dbx.And(dbx.HashExp{"status": domain.StatusVoting}, dbx.NewExp("voting_ends_at != ''")),
			dbx.And(dbx.HashExp{"status": domain.StatusOrdering}, dbx.NewExp("ordering_ends_at != ''")),
		))
	if err != nil {
		d.app.Logger().Warn("échéances : lecture des commandes", "error", err)
		return 0
	}
	n := 0
	for _, p := range parties {
		acted, err := d.process(p.Id, now)
		if err != nil {
			d.app.Logger().Warn("échéances : action impossible", "party", p.Id, "error", err)
			continue
		}
		if acted {
			n++
		}
	}
	return n
}

func readFlags(p *core.Record) domain.DeadlineFlags {
	var f domain.DeadlineFlags
	if raw := strings.TrimSpace(p.GetString("auto_state")); raw != "" && raw != "null" {
		_ = json.Unmarshal([]byte(raw), &f)
	}
	return f
}

func readAutoEvents(p *core.Record) []domain.AutoEvent {
	var list []domain.AutoEvent
	if raw := strings.TrimSpace(p.GetString("auto_events")); raw != "" && raw != "null" {
		_ = json.Unmarshal([]byte(raw), &list)
	}
	return list
}

type pendingPush struct {
	users []string
	msg   notify.Message
}

// process re-reads the party in a transaction, plans and applies the due
// action, then sends the notifications once committed.
func (d *deadlineScheduler) process(partyID string, now time.Time) (bool, error) {
	var out []pendingPush
	acted := false
	err := d.app.RunInTransaction(func(tx core.App) error {
		out, acted = nil, false
		p, err := tx.FindRecordById(colParties, partyID)
		if err != nil {
			return nil // deleted meanwhile
		}
		flags := readFlags(p)
		st := domain.DeadlineState{
			Status:            p.GetString("status"),
			VotingEndsAt:      p.GetDateTime("voting_ends_at").Time(),
			OrderingEndsAt:    p.GetDateTime("ordering_ends_at").Time(),
			AutoCloseDisabled: p.GetBool("auto_close_disabled"),
			Flags:             flags,
		}
		votes, err := tx.FindAllRecords(colVotes, dbx.HashExp{"party": p.Id})
		if err != nil {
			return err
		}
		items, err := tx.FindAllRecords(colOrderItems, dbx.HashExp{"party": p.Id})
		if err != nil {
			return err
		}
		st.Votes, st.Items = len(votes), len(items)

		action := domain.PlanDeadline(st, now)
		if action == domain.ActNone {
			return nil
		}
		info := partyInfo(p)
		members := p.GetStringSlice("members")
		host := p.GetString("host")
		events := readAutoEvents(p)
		event := func(kind, text string) {
			events = domain.AppendAutoEvent(events, domain.AutoEvent{Kind: kind, At: now.UTC().Format(time.RFC3339), Text: text})
		}

		switch action {
		case domain.ActRemindVote:
			flags.VoteRemindedFor = domain.DeadlineKey(st.VotingEndsAt)
			voted := map[string]bool{}
			for _, v := range votes {
				voted[v.GetString("user")] = true
			}
			var late []string
			for _, m := range members {
				if !voted[m] {
					late = append(late, m)
				}
			}
			event("reminder_vote", "Rappel envoyé : fin du vote à "+notify.Clock(st.VotingEndsAt))
			out = append(out, pendingPush{late, notify.ReminderVote(info, st.VotingEndsAt)})

		case domain.ActRemindOrder:
			flags.OrderRemindedFor = domain.DeadlineKey(st.OrderingEndsAt)
			pms, err := partyMembers(tx, p.Id)
			if err != nil {
				return err
			}
			has := itemsByUser(items)
			var withItems, without []string
			for _, pm := range pms {
				if pm.GetBool("ready") {
					continue
				}
				if u := pm.GetString("user"); has[u] {
					withItems = append(withItems, u)
				} else {
					without = append(without, u)
				}
			}
			event("reminder_order", "Rappel envoyé : fin de la commande à "+notify.Clock(st.OrderingEndsAt))
			out = append(out,
				pendingPush{withItems, notify.ReminderOrder(info, st.OrderingEndsAt, true)},
				pendingPush{without, notify.ReminderOrder(info, st.OrderingEndsAt, false)})

		case domain.ActCloseVote:
			if err := applyTransition(tx, p, domain.StatusOrdering, ""); err != nil {
				if p, err = autoCloseFailed(tx, p.Id, err, event); err != nil {
					return err
				}
				out = append(out, pendingPush{[]string{host}, notify.NeedsHost(info, "Le vote de "+partyLabel(p)+" n'a pas pu être clôturé automatiquement : à toi de choisir.")})
				break
			}
			name := ""
			if r, err := tx.FindRecordById(colRestaurants, p.GetString("restaurant")); err == nil {
				name = " — " + r.GetString("name")
			}
			event("vote_closed", "Vote clôturé automatiquement à "+notify.Clock(st.VotingEndsAt)+name)
			// the status hook notifies everybody (auto context)

		case domain.ActExtendVote:
			end := now.Add(domain.VoteExtension).Truncate(time.Second)
			flags.VoteExtended = true
			dt, err := types.ParseDateTime(end)
			if err != nil {
				return err
			}
			p.Set("voting_ends_at", dt)
			event("vote_extended", "Aucun vote à "+notify.Clock(st.VotingEndsAt)+" : vote prolongé jusqu'à "+notify.Clock(end))
			out = append(out, pendingPush{[]string{host}, notify.VoteExtended(info, end)})

		case domain.ActGiveUpVote:
			flags.VoteGaveUpFor = domain.DeadlineKey(st.VotingEndsAt)
			event("vote_needs_host", "Toujours aucun vote à "+notify.Clock(st.VotingEndsAt)+" : l'hôte choisit le resto")
			out = append(out, pendingPush{[]string{host}, notify.NeedsHost(info,
				"Personne n'a voté pour "+partyLabel(p)+" : clôture le vote ou impose un resto.")})

		case domain.ActCloseOrdering:
			pms, err := partyMembers(tx, p.Id)
			if err != nil {
				return err
			}
			if err := applyTransition(tx, p, domain.StatusReview, ""); err != nil {
				if p, err = autoCloseFailed(tx, p.Id, err, event); err != nil {
					return err
				}
				out = append(out, pendingPush{[]string{host}, notify.NeedsHost(info, "La commande "+partyLabel(p)+" n'a pas pu être clôturée automatiquement : passe au récap toi-même.")})
				break
			}
			has := itemsByUser(items)
			var notReady []string
			lateWithItems, lateEmpty := []string{}, []string{}
			for _, pm := range pms {
				if pm.GetBool("ready") {
					continue
				}
				u := pm.GetString("user")
				notReady = append(notReady, u)
				if has[u] {
					lateWithItems = append(lateWithItems, u)
				} else {
					lateEmpty = append(lateEmpty, u)
				}
			}
			text := "Commande clôturée automatiquement à " + notify.Clock(st.OrderingEndsAt)
			switch len(notReady) {
			case 0:
			case 1:
				text += " (1 personne n'était pas prête : panier gardé tel quel)"
			default:
				text += " (" + itoaInt(len(notReady)) + " personnes n'étaient pas prêtes : paniers gardés tels quels)"
			}
			event("ordering_closed", text)
			out = append(out,
				pendingPush{lateWithItems, notify.NotReadyLocked(info, true)},
				pendingPush{lateEmpty, notify.NotReadyLocked(info, false)})

		case domain.ActGiveUpOrder:
			flags.OrderGaveUpFor = domain.DeadlineKey(st.OrderingEndsAt)
			event("ordering_needs_host", "Heure limite atteinte à "+notify.Clock(st.OrderingEndsAt)+" sans aucun article : la commande reste ouverte")
			out = append(out, pendingPush{[]string{host}, notify.NeedsHost(info,
				"Heure limite atteinte pour "+partyLabel(p)+", mais aucun panier n'est rempli.")})
		}

		p.Set("auto_state", flags)
		p.Set("auto_events", events)
		if err := tx.SaveWithContext(autoContext(), p); err != nil {
			return err
		}
		acted = true
		return nil
	})
	if err != nil {
		return false, err
	}
	for _, o := range out {
		d.push.send(o.users, o.msg)
	}
	return acted, nil
}

// autoCloseFailed discards the partial changes of a failed automatic
// transition and turns auto-close off for the party (no retry every minute).
func autoCloseFailed(tx core.App, partyID string, cause error, event func(kind, text string)) (*core.Record, error) {
	p, err := tx.FindRecordById(colParties, partyID)
	if err != nil {
		return nil, err
	}
	p.Set("auto_close_disabled", true)
	event("auto_failed", "Clôture automatique impossible : "+strings.TrimSuffix(cause.Error(), "."))
	return p, nil
}

func itemsByUser(items []*core.Record) map[string]bool {
	has := map[string]bool{}
	for _, it := range items {
		has[it.GetString("user")] = true
	}
	return has
}

func partyLabel(p *core.Record) string {
	if t := strings.TrimSpace(p.GetString("title")); t != "" {
		return "« " + t + " »"
	}
	return "la commande " + p.GetString("code")
}

func itoaInt(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
