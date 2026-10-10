package app

import (
	"net/http"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/notify"
)

// Payer guard (ADR 0003, update 4): the payer must have a usable
// reimbursement method (IBAN preferred, Revolut, PayPal.me, other link)
// before being designated. Other members only learn booleans and method
// kinds (GET /parties/{id}/payout-readiness), never the IBAN or handles.

// payoutAskWindow: one « ajoute ton IBAN » request per party and target.
const payoutAskWindow = 10 * time.Minute

// codedError is a PocketBase « safe error item »: it is serialized as
// `data.<field> = { code, message }` in the API error body.
type codedError struct {
	code, msg string
}

func (c codedError) Error() string { return c.msg }
func (c codedError) Code() string  { return c.code }

// errPayerNoPayout is the 409 returned when the chosen payer cannot be
// reimbursed: `{ status: 409, message, data: { payer: { code: "payer_no_payout", message } } }`.
func errPayerNoPayout(self bool, name string) error {
	msg := domain.PayerNoPayoutMessage(self, name)
	return apis.NewApiError(http.StatusConflict, msg, map[string]error{
		"payer": codedError{code: domain.CodePayerNoPayout, msg: msg},
	})
}

func payoutFields(prof *core.Record) domain.PayoutFields {
	if prof == nil {
		return domain.PayoutFields{}
	}
	return domain.PayoutFields{
		IBAN:        prof.GetString("iban"),
		RevolutTag:  prof.GetString("revolut_tag"),
		PayPalMe:    prof.GetString("paypal_me"),
		PaymentLink: prof.GetString("payment_link"),
	}
}

// payoutAvailability reads the user's payout profile (none → nothing).
func payoutAvailability(app core.App, userID string) domain.PayoutAvailability {
	return domain.PayoutAvailabilityOf(payoutFields(payoutProfile(app, userID)))
}

// checkPayer refuses a payer who cannot be reimbursed while colleagues owe
// them something (s = summary of the party being validated).
func checkPayer(app core.App, s domain.Summary, payer, actor string) error {
	debtors := make([]string, 0, len(s.Participants))
	for _, p := range s.Participants {
		if len(p.Items) > 0 {
			debtors = append(debtors, p.User.ID)
		}
	}
	if !domain.PayoutRequired(payer, debtors) {
		return nil
	}
	if payoutAvailability(app, payer).CanReceive() {
		return nil
	}
	return errPayerNoPayout(payer == actor, userName(app, payer))
}

// memberPayout is one line of GET /parties/{id}/payout-readiness.
type memberPayout struct {
	User   string              `json:"user"`
	Guest  bool                `json:"guest"`
	Payout domain.PayoutStatus `json:"payout"`
}

type payoutReadiness struct {
	Members    []memberPayout `json:"members"`
	ReadyCount int            `json:"readyCount"`
	Total      int            `json:"total"`
}

// GET /api/occ/parties/{id}/payout-readiness (members): who could be
// designated payer. Booleans and method kinds only.
func (h *handlers) payoutReadiness(e *core.RequestEvent) error {
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	members := party.GetStringSlice("members")
	profiles := map[string]*core.Record{}
	if len(members) > 0 {
		ids := make([]any, 0, len(members))
		for _, m := range members {
			ids = append(ids, m)
		}
		recs, err := e.App.FindAllRecords(colPayoutProfiles, dbx.In("user", ids...))
		if err != nil {
			return err
		}
		for _, r := range recs {
			profiles[r.GetString("user")] = r
		}
	}
	guests := map[string]bool{}
	if users, err := e.App.FindRecordsByIds(colUsers, members); err == nil {
		for _, u := range users {
			guests[u.Id] = u.GetBool(fieldIsGuest)
		}
	}
	out := payoutReadiness{Members: make([]memberPayout, 0, len(members)), Total: len(members)}
	for _, m := range members {
		st := domain.StatusOf(domain.PayoutAvailabilityOf(payoutFields(profiles[m])))
		if st.Ready {
			out.ReadyCount++
		}
		out.Members = append(out.Members, memberPayout{User: m, Guest: guests[m], Payout: st})
	}
	return ok(e, out)
}

// POST /api/occ/parties/{id}/payout-request { user } (members): asks a
// colleague to fill in a reimbursement method (push « payments » + in-app
// toast). One request per party and target every 10 minutes.
func (h *handlers) payoutRequest(e *core.RequestEvent) error {
	var body struct {
		User string `json:"user"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	switch party.GetString("status") {
	case domain.StatusClosed, domain.StatusCancelled:
		return badRequest("Cette commande est terminée.")
	}
	if body.User == e.Auth.Id {
		return badRequest("Ajoute directement ton IBAN (ou Revolut / PayPal) dans ton profil.")
	}
	if !isMember(party, body.User) {
		return badRequest("Ce collègue ne fait pas partie de la commande.")
	}
	target, err := e.App.FindRecordById(colUsers, body.User)
	if err != nil {
		return badRequest("Ce collègue ne fait pas partie de la commande.")
	}
	name := userInfo(target).Name
	if target.GetBool(fieldIsGuest) {
		return badRequest(name + " est invité·e : il faut d'abord créer son compte pour renseigner un IBAN.")
	}
	if payoutAvailability(e.App, body.User).CanReceive() {
		return badRequest(name + " a déjà renseigné un moyen de remboursement.")
	}
	if !h.payoutAsk.Allow(party.Id+":"+body.User, h.push.now()) {
		return apis.NewTooManyRequestsError("Déjà demandé à "+name+" il y a moins de 10 minutes.", nil)
	}
	h.push.send([]string{body.User}, notify.PayoutRequest(partyInfo(party), userName(e.App, e.Auth.Id)))
	return ok(e, map[string]any{"sent": true})
}
