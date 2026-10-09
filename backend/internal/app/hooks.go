package app

import (
	"crypto/rand"
	"math/big"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// avatarPalette holds pleasant, readable avatar colors.
var avatarPalette = []string{
	"#E4572E", "#F3A712", "#29335C", "#669BBC", "#2A9D8F", "#E76F51",
	"#8E7DBE", "#3D5A80", "#D1495B", "#00798C", "#6A994E", "#BC6C25",
}

func randomColor() string {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(avatarPalette))))
	if err != nil {
		return avatarPalette[0]
	}
	return avatarPalette[n.Int64()]
}

// partyProtectedFields can only be changed through /api/occ/* endpoints.
var partyProtectedFields = []string{"code", "host", "members", "status", "restaurant", "payer", "dispatch", "closed_at"}

// partyFeeFields are editable by the host only before payments exist.
var partyFeeFields = []string{"delivery_fee", "service_fee", "tip", "split_mode"}

// inTx runs fn (which must call e.Next()) inside a transaction bound to the event.
func inTx(e *core.RecordRequestEvent, fn func(tx core.App) error) error {
	original := e.App
	defer func() { e.App = original }()
	return e.App.RunInTransaction(func(tx core.App) error {
		e.App = tx
		return fn(tx)
	})
}

func bindHooks(app core.App) {
	// ------------------------------------------------------------- users
	app.OnRecordCreate(colUsers).BindFunc(func(e *core.RecordEvent) error {
		if strings.TrimSpace(e.Record.GetString("color")) == "" {
			e.Record.Set("color", randomColor())
		}
		return e.Next()
	})

	// ----------------------------------------------------------- parties
	app.OnRecordCreateRequest(colParties).BindFunc(onPartyCreate)
	app.OnRecordUpdateRequest(colParties).BindFunc(onPartyUpdate)

	// ------------------------------------------------------- order_items
	app.OnRecordCreateRequest(colOrderItems).BindFunc(onOrderItemUpsert)
	app.OnRecordUpdateRequest(colOrderItems).BindFunc(onOrderItemUpsert)
	app.OnRecordDeleteRequest(colOrderItems).BindFunc(func(e *core.RecordRequestEvent) error {
		return inTx(e, func(tx core.App) error {
			if err := e.Next(); err != nil {
				return err
			}
			return resetReady(tx, e.Record.GetString("party"), e.Record.GetString("user"))
		})
	})

	// ------------------------------------------------------------- votes
	app.OnRecordCreateRequest(colVotes).BindFunc(onVoteCreate)

	// --------------------------------------------------- payout_profiles
	app.OnRecordCreateRequest(colPayoutProfiles).BindFunc(onPayoutProfileUpsert)
	app.OnRecordUpdateRequest(colPayoutProfiles).BindFunc(onPayoutProfileUpsert)
}

func uniqueCode(app core.App) (string, error) {
	for i := 0; i < 20; i++ {
		code, err := domain.GenerateCode()
		if err != nil {
			return "", err
		}
		n, err := app.CountRecords(colParties, dbx.HashExp{"code": code})
		if err != nil {
			return "", err
		}
		if n == 0 {
			return code, nil
		}
	}
	return "", badRequest("Impossible de générer un code de commande, réessayez.")
}

func validateCandidates(app core.App, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	recs, err := app.FindRecordsByIds(colRestaurants, ids)
	if err != nil {
		return err
	}
	if len(recs) != len(ids) {
		return badRequest("Restaurant candidat introuvable.")
	}
	for _, r := range recs {
		if !r.GetBool("active") {
			return badRequest("Le restaurant « " + r.GetString("name") + " » n'est pas disponible.")
		}
	}
	return nil
}

func validateFees(r *core.Record) error {
	for _, f := range []string{"delivery_fee", "service_fee", "tip"} {
		if r.GetInt(f) < 0 {
			return badRequest("Les frais ne peuvent pas être négatifs.")
		}
	}
	return nil
}

func onPartyCreate(e *core.RecordRequestEvent) error {
	r := e.Record
	switch {
	case e.Auth != nil && e.Auth.Collection().Name == colUsers:
		r.Set("host", e.Auth.Id)
	case e.HasSuperuserAuth():
		// superusers (admin UI) must provide an explicit host
	default:
		return forbidden("Connexion requise.")
	}
	host := r.GetString("host")
	if host == "" {
		return badRequest("L'hôte de la commande est requis.")
	}

	code, err := uniqueCode(e.App)
	if err != nil {
		return err
	}
	r.Set("code", code)
	r.Set("members", []string{host})
	r.Set("status", domain.StatusLobby)
	r.Set("restaurant", "")
	r.Set("payer", "")
	r.Set("dispatch", nil)
	r.Set("closed_at", "")
	if r.GetString("split_mode") == "" {
		r.Set("split_mode", domain.SplitEqual)
	}
	if err := validateFees(r); err != nil {
		return err
	}
	if err := validateCandidates(e.App, r.GetStringSlice("candidates")); err != nil {
		return err
	}

	return inTx(e, func(tx core.App) error {
		if err := e.Next(); err != nil {
			return err
		}
		col, err := tx.FindCollectionByNameOrId(colPartyMembers)
		if err != nil {
			return err
		}
		pm := core.NewRecord(col)
		pm.Load(map[string]any{"party": r.Id, "user": host, "role": "host", "ready": false})
		return tx.Save(pm)
	})
}

func sameValue(a, b *core.Record, field string) bool {
	switch a.Get(field).(type) {
	case []string:
		return slices.Equal(a.GetStringSlice(field), b.GetStringSlice(field))
	case types.JSONRaw:
		return strings.TrimSpace(a.GetString(field)) == strings.TrimSpace(b.GetString(field))
	}
	return a.GetString(field) == b.GetString(field)
}

func onPartyUpdate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	r, orig := e.Record, e.Record.Original()

	for _, f := range partyProtectedFields {
		if !sameValue(r, orig, f) {
			return badRequest("Le champ « " + f + " » ne peut pas être modifié directement.")
		}
	}

	status := orig.GetString("status")
	if status == domain.StatusClosed || status == domain.StatusCancelled {
		return badRequest("Cette commande est terminée et ne peut plus être modifiée.")
	}
	if !sameValue(r, orig, "candidates") {
		if status != domain.StatusLobby {
			return badRequest("Les candidats ne sont modifiables que dans le salon (avant le vote).")
		}
		if err := validateCandidates(e.App, r.GetStringSlice("candidates")); err != nil {
			return err
		}
	}
	for _, f := range partyFeeFields {
		if !sameValue(r, orig, f) && status == domain.StatusPaying {
			return badRequest("Les frais ne sont plus modifiables une fois le payeur désigné.")
		}
	}
	if r.GetString("split_mode") == "" {
		r.Set("split_mode", domain.SplitEqual)
	}
	if err := validateFees(r); err != nil {
		return err
	}
	return e.Next()
}

// resetReady marks the member as not ready (any cart change invalidates "ready").
func resetReady(app core.App, partyID, userID string) error {
	pm, err := app.FindFirstRecordByFilter(colPartyMembers, "party = {:p} && user = {:u}", dbx.Params{"p": partyID, "u": userID})
	if err != nil {
		return nil // not a member any more: nothing to reset
	}
	if !pm.GetBool("ready") {
		return nil
	}
	pm.Set("ready", false)
	return app.Save(pm)
}

func onOrderItemUpsert(e *core.RecordRequestEvent) error {
	r := e.Record
	isNew := r.IsNew()

	if !isNew && !e.HasSuperuserAuth() {
		orig := r.Original()
		if r.GetString("party") != orig.GetString("party") || r.GetString("user") != orig.GetString("user") {
			return badRequest("Impossible de déplacer un article vers une autre commande ou un autre membre.")
		}
	}

	party, err := e.App.FindRecordById(colParties, r.GetString("party"))
	if err != nil {
		return badRequest("Commande introuvable.")
	}
	if party.GetString("status") != domain.StatusOrdering {
		return badRequest("La commande n'accepte plus de modifications du panier.")
	}
	if !isMember(party, r.GetString("user")) {
		return forbidden("Vous ne faites pas partie de cette commande.")
	}

	menuItem, err := e.App.FindRecordById(colMenuItems, r.GetString("menu_item"))
	if err != nil || menuItem.GetString("restaurant") != party.GetString("restaurant") {
		return badRequest("Cet article n'appartient pas au restaurant choisi.")
	}
	if !menuItem.GetBool("available") {
		return badRequest("Cet article n'est plus disponible.")
	}

	qty := r.GetInt("quantity")
	if isNew && qty == 0 {
		qty = 1
	}
	if qty < 1 || qty > 20 {
		return badRequest("La quantité doit être comprise entre 1 et 20.")
	}
	note := strings.TrimSpace(r.GetString("note"))
	if utf8.RuneCountInString(note) > 200 {
		return badRequest("La remarque ne peut pas dépasser 200 caractères.")
	}

	var groups []domain.OptionGroup
	if err := menuItem.UnmarshalJSONField("option_groups", &groups); err != nil {
		return err
	}
	var selected []domain.SelectedOption
	if raw := strings.TrimSpace(r.GetString("selected_options")); raw != "" && raw != "null" {
		if err := r.UnmarshalJSONField("selected_options", &selected); err != nil {
			return badRequest("Options sélectionnées invalides.")
		}
	}
	priced, err := domain.PriceOptions(menuItem.GetInt("price"), groups, selected)
	if err != nil {
		return toAPIError(err)
	}

	sel := priced.Selected
	if sel == nil {
		sel = []domain.SelectedOption{}
	}
	r.Set("quantity", qty)
	r.Set("note", note)
	r.Set("selected_options", sel)
	r.Set("name", menuItem.GetString("name"))
	r.Set("options_label", priced.Label)
	r.Set("unit_price", priced.UnitPrice)
	r.Set("total", priced.UnitPrice*qty)

	return inTx(e, func(tx core.App) error {
		if err := e.Next(); err != nil {
			return err
		}
		return resetReady(tx, r.GetString("party"), r.GetString("user"))
	})
}

func onVoteCreate(e *core.RecordRequestEvent) error {
	party, err := e.App.FindRecordById(colParties, e.Record.GetString("party"))
	if err != nil {
		return badRequest("Commande introuvable.")
	}
	if party.GetString("status") != domain.StatusVoting {
		return badRequest("Le vote n'est pas ouvert.")
	}
	if !slices.Contains(party.GetStringSlice("candidates"), e.Record.GetString("restaurant")) {
		return badRequest("Ce restaurant ne fait pas partie des candidats.")
	}
	return e.Next()
}

func onPayoutProfileUpsert(e *core.RecordRequestEvent) error {
	r := e.Record
	if !r.IsNew() && !e.HasSuperuserAuth() && r.GetString("user") != r.Original().GetString("user") {
		return badRequest("Impossible de changer le propriétaire du profil.")
	}

	if iban := domain.NormalizeIBAN(r.GetString("iban")); iban != "" {
		if err := domain.ValidateIBAN(iban); err != nil {
			return toAPIError(err)
		}
		r.Set("iban", iban)
	} else {
		r.Set("iban", "")
	}

	bic, err := domain.NormalizeBIC(r.GetString("bic"))
	if err != nil {
		return toAPIError(err)
	}
	r.Set("bic", bic)

	wero, err := domain.NormalizeWeroID(r.GetString("wero_id"))
	if err != nil {
		return toAPIError(err)
	}
	r.Set("wero_id", wero)

	phone, err := domain.NormalizePhone(r.GetString("bancontact_phone"))
	if err != nil {
		return toAPIError(domain.Errf("Numéro Bancontact Pay invalide (format attendu : +32470123456)."))
	}
	r.Set("bancontact_phone", phone)
	r.Set("holder_name", strings.TrimSpace(r.GetString("holder_name")))

	revtag, err := domain.NormalizeRevolutTag(r.GetString("revolut_tag"))
	if err != nil {
		return toAPIError(err)
	}
	paypal, err := domain.NormalizePayPalMe(r.GetString("paypal_me"))
	if err != nil {
		return toAPIError(err)
	}
	link, err := domain.NormalizePaymentLink(r.GetString("payment_link"))
	if err != nil {
		return toAPIError(err)
	}
	// A revolut.me / PayPal.me link pasted in the free field becomes a handle
	// (so the per-payment link gets the amount).
	h := domain.SplitPayoutLink(domain.PayoutHandles{RevolutTag: revtag, PayPalMe: paypal, Link: link})
	r.Set("revolut_tag", h.RevolutTag)
	r.Set("paypal_me", h.PayPalMe)
	r.Set("payment_link", h.Link)

	return e.Next()
}
