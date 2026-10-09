package domain

import (
	"regexp"
	"strings"
)

// ibanLengths holds the expected IBAN length for common SEPA countries.
// Countries not listed only get the generic checks.
var ibanLengths = map[string]int{
	"BE": 16, "FR": 27, "NL": 18, "LU": 20, "DE": 22, "ES": 24, "IT": 27,
	"PT": 25, "AT": 20, "IE": 22, "CH": 21, "GB": 22, "MC": 27, "FI": 18,
	"DK": 18, "SE": 24, "NO": 15, "PL": 28,
}

var (
	ibanFormat = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
	bicFormat  = regexp.MustCompile(`^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$`)
)

// NormalizeIBAN removes spaces/dashes and uppercases.
func NormalizeIBAN(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '\t':
			return -1
		}
		return r
	}, s)
}

// ValidateIBAN checks format, country length and the ISO 7064 mod-97 checksum.
// The input must already be normalized.
func ValidateIBAN(iban string) error {
	if !ibanFormat.MatchString(iban) {
		return Errf("IBAN invalide : format incorrect.")
	}
	if l, ok := ibanLengths[iban[:2]]; ok && len(iban) != l {
		return Errf("IBAN invalide : longueur incorrecte pour %s (%d caractères attendus).", iban[:2], l)
	}
	rearranged := iban[4:] + iban[:4]
	rem := 0
	for _, r := range rearranged {
		var v int
		switch {
		case r >= '0' && r <= '9':
			v = int(r - '0')
			rem = (rem*10 + v) % 97
		default:
			v = int(r-'A') + 10
			rem = (rem*100 + v) % 97
		}
	}
	if rem != 1 {
		return Errf("IBAN invalide : clé de contrôle incorrecte.")
	}
	return nil
}

// NormalizeBIC uppercases and strips spaces; returns an error on bad format.
func NormalizeBIC(s string) (string, error) {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if s == "" {
		return "", nil
	}
	if !bicFormat.MatchString(s) {
		return "", Errf("BIC invalide.")
	}
	return s, nil
}
