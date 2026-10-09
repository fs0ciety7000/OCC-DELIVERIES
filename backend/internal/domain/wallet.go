package domain

import (
	"regexp"
	"strings"
)

var (
	e164Re  = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)
	emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
)

// NormalizePhone converts a mobile number to E.164.
// Spaces, dots, dashes, slashes and parentheses are removed; a leading "00"
// becomes "+"; a leading "0" becomes "+32" (Belgium by default).
func NormalizePhone(s string) (string, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '.', '-', '/', '(', ')', '\t', ' ':
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	switch {
	case strings.HasPrefix(s, "00"):
		s = "+" + s[2:]
	case strings.HasPrefix(s, "0"):
		s = "+32" + s[1:]
	}
	if !e164Re.MatchString(s) {
		return "", Errf("Numéro de mobile invalide (format attendu : +32470123456).")
	}
	return s, nil
}

// NormalizeWeroID accepts either a mobile number (→ E.164) or an e-mail
// address (→ lowercased).
func NormalizeWeroID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if strings.Contains(s, "@") {
		e := strings.ToLower(s)
		if !emailRe.MatchString(e) || len(e) > 254 {
			return "", Errf("Identifiant Wero invalide : e-mail incorrect.")
		}
		return e, nil
	}
	p, err := NormalizePhone(s)
	if err != nil {
		return "", Errf("Identifiant Wero invalide : indiquez un mobile (+32470123456) ou un e-mail.")
	}
	return p, nil
}
