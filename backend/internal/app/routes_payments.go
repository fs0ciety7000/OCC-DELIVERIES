package app

import (
	"context"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

func (h *handlers) payer(e *core.RequestEvent) error {
	var body struct {
		Payer       string `json:"payer"`
		CollectMode string `json:"collectMode"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	mode, err := domain.NormalizeCollectMode(body.CollectMode)
	if err != nil {
		return toAPIError(err)
	}
	party, err := partyForHost(e)
	if err != nil {
		return err
	}
	if !isMember(party, body.Payer) {
		return badRequest("Le payeur doit faire partie de la commande.")
	}

	var created []*core.Record
	err = e.App.RunInTransaction(func(tx core.App) error {
		p, err := tx.FindRecordById(colParties, party.Id)
		if err != nil {
			return err
		}
		switch p.GetString("status") {
		case domain.StatusReview:
		case domain.StatusPaying:
			existing, err := tx.FindAllRecords(colPayments, dbx.HashExp{"party": p.Id})
			if err != nil {
				return err
			}
			for _, pay := range existing {
				if pay.GetString("method") != domain.MethodSelf && pay.GetString("status") == domain.PaymentConfirmed {
					return badRequest("Des remboursements sont déjà confirmés : impossible de recalculer les parts.")
				}
			}
			for _, pay := range existing {
				if err := tx.Delete(pay); err != nil {
					return err
				}
			}
		default:
			return badRequest("Le payeur se désigne une fois le récapitulatif validé.")
		}

		s, _, err := buildSummary(tx, p)
		if err != nil {
			return err
		}
		// ADR 0003, update 4: outside cash mode, the payer must be reimbursable
		// (IBAN, Revolut, PayPal, link).
		if err := checkPayer(tx, s, mode, body.Payer, e.Auth.Id); err != nil {
			return err
		}
		col, err := tx.FindCollectionByNameOrId(colPayments)
		if err != nil {
			return err
		}
		now := types.NowDateTime()
		for _, part := range s.Participants {
			if len(part.Items) == 0 {
				continue
			}
			pay := core.NewRecord(col)
			pay.Load(map[string]any{
				"party":     p.Id,
				"debtor":    part.User.ID,
				"creditor":  body.Payer,
				"amount":    part.Total,
				"status":    domain.PaymentPending,
				"reference": domain.PaymentReference(p.GetString("code"), part.User.Name),
			})
			if part.User.ID == body.Payer {
				pay.Set("method", domain.MethodSelf)
				pay.Set("status", domain.PaymentConfirmed)
				pay.Set("confirmed_at", now)
			}
			if err := tx.Save(pay); err != nil {
				return err
			}
			created = append(created, pay)
		}
		if len(created) == 0 {
			return badRequest("Aucun article commandé : rien à rembourser.")
		}

		p.Set("payer", body.Payer)
		p.Set("collect_mode", mode)
		p.Set("status", domain.StatusPaying)
		if err := tx.SaveWithContext(actorContext(e), p); err != nil {
			return err
		}
		if _, err := closeIfAllConfirmed(actorContext(e), tx, p); err != nil {
			return err
		}
		party = p
		return nil
	})
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"party": party, "payments": created})
}

// closeIfAllConfirmed closes the party when every payment is confirmed.
func closeIfAllConfirmed(ctx context.Context, app core.App, party *core.Record) (bool, error) {
	if party.GetString("status") != domain.StatusPaying {
		return false, nil
	}
	pays, err := app.FindAllRecords(colPayments, dbx.HashExp{"party": party.Id})
	if err != nil {
		return false, err
	}
	if len(pays) == 0 {
		return false, nil
	}
	for _, p := range pays {
		if p.GetString("status") != domain.PaymentConfirmed {
			return false, nil
		}
	}
	party.Set("status", domain.StatusClosed)
	party.Set("closed_at", types.NowDateTime())
	return true, app.SaveWithContext(ctx, party)
}

// paymentForMember loads a payment and its party, checking membership.
func paymentForMember(e *core.RequestEvent) (*core.Record, *core.Record, error) {
	pay, err := e.App.FindRecordById(colPayments, e.Request.PathValue("id"))
	if err != nil {
		return nil, nil, notFound("Paiement introuvable.")
	}
	party, err := findParty(e.App, pay.GetString("party"))
	if err != nil {
		return nil, nil, err
	}
	if !isMember(party, e.Auth.Id) {
		return nil, nil, errNotMember()
	}
	return pay, party, nil
}

func (h *handlers) paymentAction(e *core.RequestEvent) error {
	var body struct {
		Action string `json:"action"`
		Method string `json:"method"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	pay, party, err := paymentForMember(e)
	if err != nil {
		return err
	}
	if party.GetString("status") != domain.StatusPaying {
		return badRequest("Les remboursements ne sont pas (ou plus) ouverts pour cette commande.")
	}
	uid := e.Auth.Id
	isDebtor := pay.GetString("debtor") == uid
	isCreditorOrHost := pay.GetString("creditor") == uid || isHost(party, uid)

	err = e.App.RunInTransaction(func(tx core.App) error {
		switch body.Action {
		case domain.ActionDeclare:
			if !isDebtor {
				return forbidden("Seul le débiteur peut déclarer son paiement.")
			}
			if pay.GetString("status") == domain.PaymentConfirmed {
				return badRequest("Ce paiement est déjà confirmé.")
			}
			status, err := domain.DeclareStatusFor(party.GetString("collect_mode"), body.Method)
			if err != nil {
				return toAPIError(err)
			}
			pay.Set("method", body.Method)
			pay.Set("status", status)
			if status == domain.PaymentDeclared {
				pay.Set("declared_at", types.NowDateTime())
			} else {
				pay.Set("declared_at", "")
			}
		case domain.ActionConfirm:
			if !isCreditorOrHost {
				return forbidden("Seul le payeur ou l'hôte peut confirmer un paiement.")
			}
			pay.Set("status", domain.PaymentConfirmed)
			pay.Set("confirmed_at", types.NowDateTime())
		case domain.ActionReset:
			if !isCreditorOrHost {
				return forbidden("Seul le payeur ou l'hôte peut réinitialiser un paiement.")
			}
			pay.Set("status", domain.PaymentPending)
			pay.Set("declared_at", "")
			pay.Set("confirmed_at", "")
		default:
			return badRequest("Action inconnue.")
		}
		if err := tx.SaveWithContext(actorContext(e), pay); err != nil {
			return err
		}
		if body.Action == domain.ActionConfirm {
			p, err := tx.FindRecordById(colParties, party.Id)
			if err != nil {
				return err
			}
			_, err = closeIfAllConfirmed(actorContext(e), tx, p)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"payment": pay})
}

type paymentQR struct {
	// CollectMode: "cash" → no EPC / links, methods = cash, later (ADR 0003, update 4).
	CollectMode string               `json:"collectMode"`
	Amount      int                  `json:"amount"`
	Reference   string               `json:"reference"`
	Beneficiary string               `json:"beneficiary"`
	EPC         *string              `json:"epc"`
	IBAN        *string              `json:"iban"`
	Links       []domain.PaymentLink `json:"links"`
	Methods     []string             `json:"methods"`
}

func payoutProfile(app core.App, userID string) *core.Record {
	p, err := app.FindFirstRecordByData(colPayoutProfiles, "user", userID)
	if err != nil {
		return nil
	}
	return p
}

// payoutData is the creditor's payout profile, loaded once and turned into
// per-payment QR payloads (EPC + links with the exact amount).
type payoutData struct {
	beneficiary string
	iban, bic   string
	handles     domain.PayoutHandles
}

func loadPayout(app core.App, creditorID string) payoutData {
	var d payoutData
	if u, err := app.FindRecordById(colUsers, creditorID); err == nil {
		d.beneficiary = userInfo(u).Name
	}
	prof := payoutProfile(app, creditorID)
	if prof == nil {
		return d
	}
	if holder := prof.GetString("holder_name"); holder != "" {
		d.beneficiary = holder
	}
	d.iban, d.bic = prof.GetString("iban"), prof.GetString("bic")
	// Normalized again: profiles saved before the structured handles.
	d.handles = domain.PayoutHandles{Link: prof.GetString("payment_link")}
	d.handles.RevolutTag, _ = domain.NormalizeRevolutTag(prof.GetString("revolut_tag"))
	d.handles.PayPalMe, _ = domain.NormalizePayPalMe(prof.GetString("paypal_me"))
	return d
}

// qrFor builds the PaymentQR of one payment (amount in cents, reference).
// In cash mode nothing of the payout profile is used.
func (d payoutData) qrFor(mode string, amount int, reference string) paymentQR {
	out := paymentQR{
		CollectMode: collectModeOf(mode),
		Amount:      amount,
		Reference:   reference,
		Beneficiary: d.beneficiary,
		Links:       []domain.PaymentLink{},
	}
	if out.CollectMode == domain.CollectCash {
		out.Methods = domain.MethodsFor(domain.CollectCash, domain.PayoutAvailability{})
		return out
	}
	if d.iban != "" {
		epc, err := domain.BuildEPC(domain.EPCParams{
			BIC: d.bic, Name: d.beneficiary, IBAN: d.iban,
			Amount: amount, Remittance: reference,
		})
		if err == nil {
			out.EPC = &epc
			iban := domain.NormalizeIBAN(d.iban)
			out.IBAN = &iban
		}
	}
	out.Links = domain.PaymentLinks(domain.SplitPayoutLink(d.handles), amount, reference)
	avail := domain.PayoutAvailability{IBAN: out.EPC != nil}
	for _, l := range out.Links {
		switch l.Kind {
		case domain.LinkRevolut:
			avail.Revolut = true
		case domain.LinkPayPal:
			avail.PayPal = true
		case domain.LinkGeneric:
			avail.Link = true
		}
	}
	out.Methods = domain.AvailableMethods(avail)
	return out
}

func (h *handlers) paymentQR(e *core.RequestEvent) error {
	pay, party, err := paymentForMember(e)
	if err != nil {
		return err
	}
	out := loadPayout(e.App, pay.GetString("creditor")).qrFor(party.GetString("collect_mode"), pay.GetInt("amount"), pay.GetString("reference"))
	return ok(e, out)
}

// collectItem is one debtor's share as seen by the payer (« Encaisser »).
type collectItem struct {
	Payment   string               `json:"payment"`
	Debtor    domain.UserInfo      `json:"debtor"`
	Amount    int                  `json:"amount"`
	Status    string               `json:"status"`
	Method    string               `json:"method"`
	Reference string               `json:"reference"`
	EPC       *string              `json:"epc"`
	Links     []domain.PaymentLink `json:"links"`
}

type collectQR struct {
	CollectMode string        `json:"collectMode"`
	Beneficiary string        `json:"beneficiary"`
	IBAN        *string       `json:"iban"`
	Items       []collectItem `json:"items"`
}

// paymentsCollectQR returns, to the payer only, the QR payloads of every
// debtor's share (EPC069-12 + prefilled links with each exact amount): the
// payer presents them from his own phone. Same builders as /payments/{id}/qr;
// only the payer's own payout data is involved.
func (h *handlers) paymentsCollectQR(e *core.RequestEvent) error {
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	switch party.GetString("status") {
	case domain.StatusPaying, domain.StatusClosed:
	default:
		return badRequest("Les remboursements ne sont pas encore ouverts pour cette commande.")
	}
	payerID := party.GetString("payer")
	if payerID == "" || payerID != e.Auth.Id {
		return forbidden("Seul le payeur peut afficher les QR d'encaissement.")
	}
	pays, err := e.App.FindAllRecords(colPayments, dbx.HashExp{"party": party.Id, "creditor": payerID})
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(pays))
	for _, p := range pays {
		ids = append(ids, p.GetString("debtor"))
	}
	users := map[string]domain.UserInfo{}
	if recs, err := e.App.FindRecordsByIds(colUsers, ids); err == nil {
		for _, u := range recs {
			users[u.Id] = userInfo(u)
		}
	}

	data := loadPayout(e.App, payerID)
	mode := collectModeOf(party.GetString("collect_mode"))
	out := collectQR{CollectMode: mode, Beneficiary: data.beneficiary, Items: []collectItem{}}
	for _, p := range pays {
		debtor := p.GetString("debtor")
		if debtor == payerID || p.GetString("method") == domain.MethodSelf {
			continue
		}
		q := data.qrFor(mode, p.GetInt("amount"), p.GetString("reference"))
		if out.IBAN == nil {
			out.IBAN = q.IBAN
		}
		u, found := users[debtor]
		if !found {
			u = domain.UserInfo{ID: debtor, Name: "Membre"}
		}
		out.Items = append(out.Items, collectItem{
			Payment: p.Id, Debtor: u, Amount: p.GetInt("amount"),
			Status: p.GetString("status"), Method: p.GetString("method"),
			Reference: p.GetString("reference"), EPC: q.EPC, Links: q.Links,
		})
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		return strings.ToLower(out.Items[i].Debtor.Name) < strings.ToLower(out.Items[j].Debtor.Name)
	})
	return ok(e, out)
}

// collectModeOf reads parties.collect_mode ("" = transfer: parties
// designated before the field existed, see migration 1760000022).
func collectModeOf(stored string) string {
	if stored == domain.CollectCash {
		return domain.CollectCash
	}
	return domain.CollectTransfer
}
