package domain

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

// EPCParams are the inputs of an EPC069-12 (SEPA credit transfer QR) payload.
type EPCParams struct {
	BIC        string
	Name       string
	IBAN       string
	Amount     int // cents
	Remittance string
}

// EPC limits.
const (
	epcMaxName       = 70
	epcMaxRemittance = 140
	epcMaxAmount     = 99999999999 // EUR 999 999 999.99
)

// BuildEPC builds an EPC069-12 payload (version 002, UTF-8, SCT).
// Lines: BCD, 002, 1, SCT, BIC, name, IBAN, amount, purpose, structured ref, remittance.
func BuildEPC(p EPCParams) (string, error) {
	name := truncateRunes(cleanLine(p.Name), epcMaxName)
	if name == "" {
		return "", Errf("Nom du bénéficiaire manquant.")
	}
	iban := NormalizeIBAN(p.IBAN)
	if err := ValidateIBAN(iban); err != nil {
		return "", err
	}
	bic, err := NormalizeBIC(p.BIC)
	if err != nil {
		return "", err
	}
	if p.Amount < 1 || p.Amount > epcMaxAmount {
		return "", Errf("Montant invalide pour un virement.")
	}
	rem := truncateRunes(cleanLine(p.Remittance), epcMaxRemittance)

	lines := []string{
		"BCD", "002", "1", "SCT",
		bic,
		name,
		iban,
		"EUR" + FormatAmount(p.Amount),
		"", // purpose
		"", // structured reference
		rem,
	}
	return strings.Join(lines, "\n"), nil
}

// PaymentLinkWithAmount returns the payer's payment link; for paypal.me links
// with only a username, the amount is appended (".../bob/14.00EUR").
func PaymentLinkWithAmount(link string, amount int) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	u, err := url.Parse(link)
	if err != nil || u.Host == "" {
		return link
	}
	host := strings.ToLower(strings.TrimPrefix(u.Host, "www."))
	if host != "paypal.me" || amount <= 0 {
		return link
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) != 1 || segs[0] == "" {
		return link
	}
	u.Path = "/" + segs[0] + "/" + FormatAmount(amount) + "EUR"
	u.RawQuery = ""
	return u.String()
}

func cleanLine(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n]))
}

// TruncateRunes is the exported helper used for references/labels.
func TruncateRunes(s string, n int) string { return truncateRunes(s, n) }
