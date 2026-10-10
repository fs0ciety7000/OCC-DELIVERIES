package domain

import "strings"

// Payer guard (ADR 0003, update 4). With the payer, the host chooses how the
// payer wants to be reimbursed (parties.collect_mode):
//   - "transfer": EPC transfer QR, Revolut, PayPal.me or another link — the
//     payer must then have a usable method: a valid IBAN (preferred), a
//     Revolut tag, a PayPal.me name or another payment link;
//   - "cash": cash (or « later ») only — never blocked, even with an empty
//     payout profile; debtors cannot declare any other method.

// Collect modes (parties.collect_mode).
const (
	CollectTransfer = "transfer"
	CollectCash     = "cash"
)

// NormalizeCollectMode validates the requested mode; empty means transfer
// (the historical behaviour).
func NormalizeCollectMode(s string) (string, error) {
	switch strings.TrimSpace(s) {
	case "", CollectTransfer:
		return CollectTransfer, nil
	case CollectCash:
		return CollectCash, nil
	}
	return "", Errf("Mode de remboursement invalide : %q (transfer ou cash).", s)
}

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

// CheckPayer validates the payer designation: nil when allowed (cash mode,
// nothing to reimburse, or a usable method).
func CheckPayer(mode, payer string, debtors []string, payout PayoutAvailability, self bool, name string) error {
	if mode == CollectCash || !PayoutRequired(payer, debtors) || payout.CanReceive() {
		return nil
	}
	return &Error{Msg: PayerNoPayoutMessage(self, name)}
}

// MethodsFor returns the reimbursement methods offered to debtors for the
// party's collect mode: cash mode only offers cash and later.
func MethodsFor(mode string, a PayoutAvailability) []string {
	if mode == CollectCash {
		return []string{MethodCash, MethodLater}
	}
	return AvailableMethods(a)
}

// DeclareStatusFor is DeclareStatus restricted by the collect mode: in cash
// mode only cash and later can be declared.
func DeclareStatusFor(mode, method string) (string, error) {
	switch method {
	case MethodQR, MethodRevolut, MethodPayPal, MethodLink:
		if mode != CollectCash {
			break
		}
		return "", Errf("Le payeur a demandé un remboursement en espèces : déclare « espèces » ou « plus tard ».")
	}
	return DeclareStatus(method)
}
