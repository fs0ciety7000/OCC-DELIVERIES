package app

import (
	"context"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

func (h *handlers) payer(e *core.RequestEvent) error {
	var body struct {
		Payer string `json:"payer"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
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
			status, err := domain.DeclareStatus(body.Method)
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

type weroInfo struct {
	ID    string `json:"id"`
	HasQR bool   `json:"hasQr"`
}

type bancontactInfo struct {
	Phone string `json:"phone"`
	HasQR bool   `json:"hasQr"`
}

type paymentQR struct {
	Amount      int                  `json:"amount"`
	Reference   string               `json:"reference"`
	Beneficiary string               `json:"beneficiary"`
	EPC         *string              `json:"epc"`
	IBAN        *string              `json:"iban"`
	Links       []domain.PaymentLink `json:"links"`
	Wero        *weroInfo            `json:"wero"`
	Bancontact  *bancontactInfo      `json:"bancontact"`
	Methods     []string             `json:"methods"`
}

func payoutProfile(app core.App, userID string) *core.Record {
	p, err := app.FindFirstRecordByData(colPayoutProfiles, "user", userID)
	if err != nil {
		return nil
	}
	return p
}

func (h *handlers) paymentQR(e *core.RequestEvent) error {
	pay, _, err := paymentForMember(e)
	if err != nil {
		return err
	}
	creditorID := pay.GetString("creditor")
	out := paymentQR{
		Amount:    pay.GetInt("amount"),
		Reference: pay.GetString("reference"),
		Links:     []domain.PaymentLink{},
	}
	if u, err := e.App.FindRecordById(colUsers, creditorID); err == nil {
		out.Beneficiary = userInfo(u).Name
	}

	if prof := payoutProfile(e.App, creditorID); prof != nil {
		if holder := prof.GetString("holder_name"); holder != "" {
			out.Beneficiary = holder
		}
		if iban := prof.GetString("iban"); iban != "" {
			epc, err := domain.BuildEPC(domain.EPCParams{
				BIC: prof.GetString("bic"), Name: out.Beneficiary, IBAN: iban,
				Amount: out.Amount, Remittance: out.Reference,
			})
			if err == nil {
				out.EPC = &epc
				iban = domain.NormalizeIBAN(iban)
				out.IBAN = &iban
			}
		}
		// Normalized again: profiles saved before the structured handles.
		h := domain.PayoutHandles{Link: prof.GetString("payment_link")}
		h.RevolutTag, _ = domain.NormalizeRevolutTag(prof.GetString("revolut_tag"))
		h.PayPalMe, _ = domain.NormalizePayPalMe(prof.GetString("paypal_me"))
		out.Links = domain.PaymentLinks(domain.SplitPayoutLink(h), out.Amount, out.Reference)
		if id, qr := prof.GetString("wero_id"), prof.GetString("wero_qr"); id != "" || qr != "" {
			out.Wero = &weroInfo{ID: id, HasQR: qr != ""}
		}
		if phone, qr := prof.GetString("bancontact_phone"), prof.GetString("bancontact_qr"); phone != "" || qr != "" {
			out.Bancontact = &bancontactInfo{Phone: phone, HasQR: qr != ""}
		}
	}
	avail := domain.PayoutAvailability{IBAN: out.EPC != nil, Wero: out.Wero != nil, Bancontact: out.Bancontact != nil}
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
	return ok(e, out)
}

var walletQRFields = map[string]string{"wero": "wero_qr", "bancontact": "bancontact_qr"}

func (h *handlers) walletQR(e *core.RequestEvent) error {
	field, known := walletQRFields[e.Request.PathValue("kind")]
	if !known {
		return notFound("Type de QR inconnu.")
	}
	pay, _, err := paymentForMember(e)
	if err != nil {
		return err
	}
	prof := payoutProfile(e.App, pay.GetString("creditor"))
	if prof == nil || prof.GetString(field) == "" {
		return notFound("Le payeur n'a pas fourni de QR.")
	}
	name := prof.GetString(field)

	fsys, err := e.App.NewFilesystem()
	if err != nil {
		return err
	}
	defer fsys.Close()

	key := prof.BaseFilesPath() + "/" + name
	e.Response.Header().Set("Cache-Control", "private, max-age=300")
	if err := fsys.Serve(e.Response, e.Request, key, name); err != nil {
		e.Response.Header().Del("Cache-Control")
		return notFound("QR introuvable.")
	}
	return nil
}
