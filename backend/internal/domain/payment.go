package domain

import "strings"

// Payment methods.
const (
	MethodQR         = "qr"
	MethodWero       = "wero"
	MethodBancontact = "bancontact"
	MethodRevolut    = "revolut"
	MethodPayPal     = "paypal"
	MethodLink       = "link"
	MethodCash       = "cash"
	MethodLater      = "later"
	MethodSelf       = "self"
)

// PaymentMethods lists every payment method value.
var PaymentMethods = []string{MethodQR, MethodRevolut, MethodPayPal, MethodLink, MethodWero, MethodBancontact, MethodCash, MethodLater, MethodSelf}

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
	case MethodQR, MethodRevolut, MethodPayPal, MethodLink, MethodWero, MethodBancontact, MethodCash:
		return PaymentDeclared, nil
	case MethodLater:
		return PaymentPending, nil
	}
	return "", Errf("Moyen de paiement invalide : %q.", method)
}

// PayoutAvailability describes what the payer filled in their payout profile.
type PayoutAvailability struct {
	IBAN       bool
	Revolut    bool
	PayPal     bool
	Link       bool
	Wero       bool
	Bancontact bool
}

// AvailableMethods returns the reimbursement methods offered to debtors, in
// order of usefulness: the EPC transfer QR (amount + communication
// prefilled in any bank app), the wallet links (amount prefilled), the
// free link, Wero / Bancontact Pay (amount typed by hand), then cash and
// later.
func AvailableMethods(a PayoutAvailability) []string {
	out := []string{}
	for _, m := range []struct {
		on     bool
		method string
	}{
		{a.IBAN, MethodQR}, {a.Revolut, MethodRevolut}, {a.PayPal, MethodPayPal}, {a.Link, MethodLink},
		{a.Wero, MethodWero}, {a.Bancontact, MethodBancontact},
	} {
		if m.on {
			out = append(out, m.method)
		}
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
