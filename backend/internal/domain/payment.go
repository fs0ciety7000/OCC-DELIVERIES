package domain

import "strings"

// Payment methods.
const (
	MethodQR         = "qr"
	MethodWero       = "wero"
	MethodBancontact = "bancontact"
	MethodLink       = "link"
	MethodCash       = "cash"
	MethodLater      = "later"
	MethodSelf       = "self"
)

// PaymentMethods lists every payment method value.
var PaymentMethods = []string{MethodQR, MethodWero, MethodBancontact, MethodLink, MethodCash, MethodLater, MethodSelf}

// Payment statuses.
const (
	PaymentPending   = "pending"
	PaymentDeclared  = "declared"
	PaymentConfirmed = "confirmed"
)

// PaymentActions.
const (
	ActionDeclare = "declare"
	ActionConfirm = "confirm"
	ActionReset   = "reset"
)

// DeclareStatus returns the status a payment gets when the debtor declares
// it with the given method.
func DeclareStatus(method string) (string, error) {
	switch method {
	case MethodQR, MethodWero, MethodBancontact, MethodLink, MethodCash:
		return PaymentDeclared, nil
	case MethodLater:
		return PaymentPending, nil
	}
	return "", Errf("Moyen de paiement invalide : %q.", method)
}

// PayoutAvailability describes what the payer filled in their payout profile.
type PayoutAvailability struct {
	IBAN       bool
	Wero       bool
	Bancontact bool
	Link       bool
}

// AvailableMethods returns the reimbursement methods offered to debtors, in
// recommended display order: wero, bancontact, qr, link, then cash and later.
func AvailableMethods(a PayoutAvailability) []string {
	out := []string{}
	if a.Wero {
		out = append(out, MethodWero)
	}
	if a.Bancontact {
		out = append(out, MethodBancontact)
	}
	if a.IBAN {
		out = append(out, MethodQR)
	}
	if a.Link {
		out = append(out, MethodLink)
	}
	return append(out, MethodCash, MethodLater)
}

// PaymentReference builds the transfer communication "OCC <code> <name>" (≤ 140 chars).
func PaymentReference(code, name string) string {
	name = cleanLine(name)
	if name == "" {
		name = "Membre"
	}
	return truncateRunes(strings.TrimSpace("OCC "+code+" "+name), epcMaxRemittance)
}
