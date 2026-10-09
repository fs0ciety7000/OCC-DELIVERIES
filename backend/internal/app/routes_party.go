package app

import (
	"slices"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

var joinableStatuses = []string{domain.StatusLobby, domain.StatusVoting, domain.StatusOrdering, domain.StatusReview}

var leavableStatuses = []string{domain.StatusLobby, domain.StatusVoting, domain.StatusOrdering}

func (h *handlers) join(e *core.RequestEvent) error {
	var body struct {
		Code string `json:"code"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	code := domain.NormalizeCode(body.Code)
	if !domain.IsValidCode(code) {
		return badRequest("Code de commande invalide.")
	}

	var party *core.Record
	err := e.App.RunInTransaction(func(tx core.App) error {
		p, err := tx.FindFirstRecordByData(colParties, "code", code)
		if err != nil {
			return notFound("Aucune commande ne correspond à ce code.")
		}
		party = p
		if isMember(p, e.Auth.Id) {
			return nil // idempotent
		}
		if !slices.Contains(joinableStatuses, p.GetString("status")) {
			return badRequest("Cette commande n'accepte plus de nouveaux participants.")
		}
		p.Set("members+", e.Auth.Id)
		if err := tx.Save(p); err != nil {
			return err
		}
		col, err := tx.FindCollectionByNameOrId(colPartyMembers)
		if err != nil {
			return err
		}
		pm := core.NewRecord(col)
		pm.Load(map[string]any{"party": p.Id, "user": e.Auth.Id, "role": "member", "ready": false})
		return tx.Save(pm)
	})
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"party": party})
}

func (h *handlers) leave(e *core.RequestEvent) error {
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	if isHost(party, e.Auth.Id) {
		return badRequest("L'hôte ne peut pas quitter sa commande : annulez-la plutôt.")
	}
	if !slices.Contains(leavableStatuses, party.GetString("status")) {
		return badRequest("Il est trop tard pour quitter cette commande.")
	}
	uid := e.Auth.Id
	err = e.App.RunInTransaction(func(tx core.App) error {
		for _, col := range []string{colVotes, colOrderItems, colPartyMembers} {
			recs, err := tx.FindAllRecords(col, dbx.HashExp{"party": party.Id, "user": uid})
			if err != nil {
				return err
			}
			for _, r := range recs {
				if err := tx.Delete(r); err != nil {
					return err
				}
			}
		}
		p, err := tx.FindRecordById(colParties, party.Id)
		if err != nil {
			return err
		}
		p.Set("members-", uid)
		return tx.Save(p)
	})
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"ok": true})
}

func (h *handlers) transition(e *core.RequestEvent) error {
	var body struct {
		To         string `json:"to"`
		Restaurant string `json:"restaurant"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	party, err := partyForHost(e)
	if err != nil {
		return err
	}

	err = e.App.RunInTransaction(func(tx core.App) error {
		p, err := tx.FindRecordById(colParties, party.Id)
		if err != nil {
			return err
		}
		from := p.GetString("status")
		candidates := p.GetStringSlice("candidates")
		nItems, err := tx.CountRecords(colOrderItems, dbx.HashExp{"party": p.Id})
		if err != nil {
			return err
		}

		in := domain.TransitionInput{
			From: from, To: body.To,
			Candidates:           len(candidates),
			RequestedRestaurant:  body.Restaurant,
			RequestedIsCandidate: slices.Contains(candidates, body.Restaurant),
			Items:                int(nItems),
		}
		if err := domain.ValidateTransition(in); err != nil {
			return toAPIError(err)
		}

		switch {
		case body.To == domain.StatusOrdering && (from == domain.StatusLobby || from == domain.StatusVoting):
			restID := body.Restaurant
			if restID == "" {
				restID, err = electWinner(tx, p.Id, candidates)
				if err != nil {
					return err
				}
			}
			rest, err := tx.FindRecordById(colRestaurants, restID)
			if err != nil || !rest.GetBool("active") {
				return badRequest("Restaurant introuvable ou indisponible.")
			}
			p.Set("restaurant", rest.Id)
		case body.To == domain.StatusReview:
			rest, err := tx.FindRecordById(colRestaurants, p.GetString("restaurant"))
			if err != nil {
				return badRequest("Aucun restaurant choisi.")
			}
			p.Set("delivery_fee", rest.GetInt("delivery_fee"))
		case from == domain.StatusReview && body.To == domain.StatusOrdering:
			// saved one by one (small set) so that realtime subscribers are notified
			pms, err := partyMembers(tx, p.Id)
			if err != nil {
				return err
			}
			for _, pm := range pms {
				if !pm.GetBool("ready") {
					continue
				}
				pm.Set("ready", false)
				if err := tx.Save(pm); err != nil {
					return err
				}
			}
		case body.To == domain.StatusClosed:
			p.Set("closed_at", types.NowDateTime())
		}

		p.Set("status", body.To)
		if err := tx.Save(p); err != nil {
			return err
		}
		party = p
		return nil
	})
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"party": party})
}

func electWinner(app core.App, partyID string, candidates []string) (string, error) {
	recs, err := app.FindRecordsByIds(colRestaurants, candidates)
	if err != nil {
		return "", err
	}
	cands := make([]domain.Candidate, 0, len(recs))
	for _, r := range recs {
		if r.GetBool("active") {
			cands = append(cands, domain.Candidate{ID: r.Id, Name: r.GetString("name"), Rating: r.GetFloat("rating")})
		}
	}
	votes, err := app.FindAllRecords(colVotes, dbx.HashExp{"party": partyID})
	if err != nil {
		return "", err
	}
	counts := map[string]int{}
	for _, v := range votes {
		counts[v.GetString("restaurant")]++
	}
	winner := domain.ElectWinner(cands, counts)
	if winner == "" {
		return "", badRequest("Aucun restaurant candidat disponible.")
	}
	return winner, nil
}

func (h *handlers) ready(e *core.RequestEvent) error {
	var body struct {
		Ready bool `json:"ready"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	if party.GetString("status") != domain.StatusOrdering {
		return badRequest("On ne peut se déclarer prêt que pendant la prise de commande.")
	}
	pm, err := e.App.FindFirstRecordByFilter(colPartyMembers, "party = {:p} && user = {:u}", dbx.Params{"p": party.Id, "u": e.Auth.Id})
	if err != nil {
		return errNotMember()
	}
	pm.Set("ready", body.Ready)
	if err := e.App.Save(pm); err != nil {
		return err
	}
	return ok(e, map[string]any{"member": pm})
}

func (h *handlers) summary(e *core.RequestEvent) error {
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	s, _, err := buildSummary(e.App, party)
	if err != nil {
		return err
	}
	return ok(e, s)
}

func (h *handlers) dispatch(e *core.RequestEvent) error {
	var body struct {
		Method string `json:"method"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	party, err := partyForHost(e)
	if err != nil {
		return err
	}
	if st := party.GetString("status"); st != domain.StatusReview && st != domain.StatusPaying {
		return badRequest("La commande doit être validée (récapitulatif) avant d'être envoyée.")
	}
	prov, found := providers.Get(body.Method)
	if !found || (providers.IsPlatform(body.Method) && !h.cfg.providerEnabled(body.Method)) {
		return badRequest("Méthode d'envoi indisponible.")
	}

	s, rest, err := buildSummary(e.App, party)
	if err != nil {
		return err
	}
	d := prov.Dispatch(providerRestaurant(rest), s)

	providerField := "manual"
	if providers.IsPlatform(body.Method) {
		providerField = body.Method
	}
	party.Set("provider", providerField)
	party.Set("dispatch", map[string]any{
		"method": d.Method,
		"at":     time.Now().UTC().Format(time.RFC3339),
		"url":    d.URL,
	})
	if err := e.App.Save(party); err != nil {
		return err
	}
	return ok(e, map[string]any{"party": party, "dispatch": d})
}
