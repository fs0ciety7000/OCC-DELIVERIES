package domain

import "strings"

// Payer guard (ADR 0003, update 4): a member can only be designated payer
// when colleagues will be able to reimburse them with a usable method —
// a valid IBAN (EPC transfer QR, preferred), a Revolut tag, a PayPal.me name
// or another payment link. Cash and « later » never count: they need no
// payout profile, so they cannot prove anything.

// CodePayerNoPayout is the machine-readable code of the refusal
// (`data.payer.code` of the API error), read by the SPA.
const CodePayerNoPayout = "payer_no_payout"

// PayoutFields are the raw payout_profiles values of a member.
type PayoutFields struct {
	IBAN        string
	RevolutTag  string
	PayPalMe    string
	PaymentLink string
}

// PayoutAvailabilityOf tells which reimbursement methods a payout profile
// really offers: the IBAN must pass mod-97, the handles must normalize
// (profiles saved before the structured handles are normalized again), and
// a revolut.me / PayPal.me link pasted in the free field counts as that
// handle (same rules as the per-payment links).
func PayoutAvailabilityOf(f PayoutFields) PayoutAvailability {
	var a PayoutAvailability
	if iban := NormalizeIBAN(f.IBAN); iban != "" && ValidateIBAN(iban) == nil {
		a.IBAN = true
	}
	h := PayoutHandles{}
	h.RevolutTag, _ = NormalizeRevolutTag(f.RevolutTag)
	h.PayPalMe, _ = NormalizePayPalMe(f.PayPalMe)
	h.Link, _ = NormalizePaymentLink(f.PaymentLink)
	h = SplitPayoutLink(h)
	a.Revolut = h.RevolutTag != ""
	a.PayPal = h.PayPalMe != ""
	a.Link = h.Link != ""
	return a
}

// CanReceive reports whether at least one usable reimbursement method is
// available (cash / later excluded).
func (a PayoutAvailability) CanReceive() bool {
	return a.IBAN || a.Revolut || a.PayPal || a.Link
}

// LinkKinds lists the link methods available, in display order
// (« revolut », « paypal », « link »); never nil.
func (a PayoutAvailability) LinkKinds() []string {
	out := []string{}
	if a.Revolut {
		out = append(out, MethodRevolut)
	}
	if a.PayPal {
		out = append(out, MethodPayPal)
	}
	if a.Link {
		out = append(out, MethodLink)
	}
	return out
}

// PayoutStatus is what other members may know about someone's payout
// profile: booleans and method kinds only, never the IBAN nor the handles.
type PayoutStatus struct {
	Ready bool     `json:"ready"`
	IBAN  bool     `json:"iban"`
	Links []string `json:"links"`
}

// StatusOf turns an availability into the public PayoutStatus.
func StatusOf(a PayoutAvailability) PayoutStatus {
	return PayoutStatus{Ready: a.CanReceive(), IBAN: a.IBAN, Links: a.LinkKinds()}
}

// PayoutRequired reports whether the payer must be able to receive money:
// only when at least one other member has something to reimburse (a payer
// who ordered alone owes nothing to anyone). debtors are the members with
// at least one item.
func PayoutRequired(payer string, debtors []string) bool {
	for _, d := range debtors {
		if d != "" && d != payer {
			return true
		}
	}
	return false
}

// PayerNoPayoutMessage is the French refusal shown when the chosen payer
// has no usable reimbursement method.
func PayerNoPayoutMessage(self bool, name string) string {
	if self {
		return "Ajoute ton IBAN (ou Revolut / PayPal) dans ton profil avant de valider la commande."
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Ce membre"
	}
	return name + " n'a encore renseigné aucun moyen de remboursement."
}

// CheckPayer validates the payer designation: nil when allowed.
func CheckPayer(payer string, debtors []string, payout PayoutAvailability, self bool, name string) error {
	if !PayoutRequired(payer, debtors) || payout.CanReceive() {
		return nil
	}
	return &Error{Msg: PayerNoPayoutMessage(self, name)}
}
