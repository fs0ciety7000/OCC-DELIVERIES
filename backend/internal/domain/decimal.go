package domain

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	decimalRe   = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	euroInfixRe = regexp.MustCompile(`(\d)\s*€\s*(\d)`)
)

// normalizeDecimal turns human input (« 12,50 € », « 1 234,5 », « 1.234,50 »,
// « 12.5 ») into a dot-decimal string. The last of ',' / '.' is the decimal
// separator when both appear; the other one is a thousands separator.
func normalizeDecimal(s string) string {
	s = strings.TrimSpace(s)
	s = euroInfixRe.ReplaceAllString(s, "$1,$2") // « 12€50 »
	s = strings.NewReplacer("€", "", "EUR", "", "eur", "", " ", "", "\u00a0", "", "\u202f", "", "'", "").Replace(s)
	lastComma, lastDot := strings.LastIndex(s, ","), strings.LastIndex(s, ".")
	switch {
	case lastComma >= 0 && lastDot >= 0:
		if lastComma > lastDot {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	case lastComma >= 0:
		s = strings.Replace(s, ",", ".", 1)
	}
	return s
}

// ParseEuros parses a euro amount written by a human (French or English
// decimals, optional « € ») and returns integer cents. It rejects negative
// amounts and more than two decimals.
func ParseEuros(s string) (int, error) {
	n := normalizeDecimal(s)
	if n == "" {
		return 0, Errf("Prix manquant.")
	}
	if !decimalRe.MatchString(n) {
		return 0, Errf("Prix invalide : « %s » (ex. 12,50).", strings.TrimSpace(s))
	}
	if strings.HasPrefix(n, "-") {
		return 0, Errf("Prix négatif : « %s ».", strings.TrimSpace(s))
	}
	intPart, frac, _ := strings.Cut(n, ".")
	if len(frac) > 2 {
		return 0, Errf("Prix invalide : « %s » (deux décimales maximum).", strings.TrimSpace(s))
	}
	for len(frac) < 2 {
		frac += "0"
	}
	euros, err := strconv.Atoi(intPart)
	if err != nil || euros > 100000 {
		return 0, Errf("Prix invalide : « %s ».", strings.TrimSpace(s))
	}
	cents, _ := strconv.Atoi(frac)
	return euros*100 + cents, nil
}

// ParseDecimal parses a number written with a French or English decimal
// separator (« 50,4542 » or « 50.4542 »).
func ParseDecimal(s string) (float64, error) {
	n := normalizeDecimal(s)
	if !decimalRe.MatchString(n) {
		return 0, Errf("Nombre invalide : « %s ».", strings.TrimSpace(s))
	}
	return strconv.ParseFloat(n, 64)
}

// ParseYesNo parses a French/English boolean (« oui », « x », « 1 », « true »…).
// An empty value is false.
func ParseYesNo(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "0", "non", "no", "n", "false", "faux", "f":
		return false, nil
	case "1", "oui", "o", "yes", "y", "true", "vrai", "v", "x", "✓":
		return true, nil
	}
	return false, Errf("Valeur oui/non invalide : « %s ».", strings.TrimSpace(s))
}

// SplitList splits « veggie|spicy », « veggie, spicy » or « veggie;spicy » into
// trimmed, lower-cased, de-duplicated values.
func SplitList(s string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '|' || r == ',' || r == ';' }) {
		v := strings.ToLower(strings.TrimSpace(part))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
