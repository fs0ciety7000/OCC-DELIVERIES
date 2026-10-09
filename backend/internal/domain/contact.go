package domain

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Restaurant contact details (phone, postal address) normalization. The
// sources publish them in many shapes (« 065/35.29.64 », « +32 (0)65 … »,
// « 24 Rue de la Clef, 7000 ») ; we store one canonical form:
//   - phone: E.164 (« +3265352964 »), formatted for display by FormatPhone
//     (and its TypeScript twin in frontend/src/lib/format.ts);
//   - address: « Rue de la Clef 24, 7000 Mons » (street + house number,
//     postcode + locality, locality inferred from the postcode around Mons).

// ---------------------------------------------------------------- phone

// phoneRunRe finds a phone-like run of characters inside a free text
// (« Tél. : 065 35 29 64 »).
var phoneRunRe = regexp.MustCompile(`\+?\d[\d\s.()/\-\x{00a0}\x{202f}]{5,}\d`)

// phoneSplitRe separates several numbers given in one field.
var phoneSplitRe = regexp.MustCompile(`(?i)\s*(?:;|,|\||\bou\b|\bor\b|\s/\s|\s-\s)\s*`)

// NormalizeRestaurantPhone returns the E.164 form of a restaurant phone
// number (landline or mobile; Belgium by default: a leading 0 becomes +32,
// 00 becomes +, « +32 (0)65 » drops the trunk 0). When the field holds
// several numbers, the first valid one is kept. ok is false for junk
// (too short / long, repeated digit, letters only…).
func NormalizeRestaurantPhone(s string) (string, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "tel:"))
	if s == "" {
		return "", false
	}
	// the whole field first (« 065 / 35 29 64 »), then each part of a list
	// (« 065 35 29 64 / 0475 12 34 56 »)
	if p, ok := canonicalPhone(phoneRunRe.FindString(s)); ok {
		return p, true
	}
	for _, part := range phoneSplitRe.Split(s, -1) {
		run := phoneRunRe.FindString(part)
		if run == "" {
			continue
		}
		if p, ok := canonicalPhone(run); ok {
			return p, true
		}
	}
	return "", false
}

func canonicalPhone(run string) (string, bool) {
	if run == "" {
		return "", false
	}
	run = strings.ReplaceAll(run, "(0)", "")
	plus := strings.HasPrefix(run, "+")
	var b strings.Builder
	for _, r := range run {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case plus:
	case strings.HasPrefix(d, "00"):
		d = d[2:]
	case strings.HasPrefix(d, "0"):
		d = "32" + d[1:]
	case strings.HasPrefix(d, "32") && (len(d) == 10 || len(d) == 11):
		// « 32 65 35 29 64 » without the plus
	default:
		return "", false
	}
	if strings.HasPrefix(d, "320") {
		d = "32" + d[3:] // « +32 065 … »: trunk prefix written twice
	}
	if d == "" || d[0] == '0' || len(d) < 8 || len(d) > 15 {
		return "", false
	}
	if strings.Count(d[2:], d[len(d)-1:]) == len(d)-2 {
		return "", false // « 0000000000 », « 0999999999 »
	}
	if strings.HasPrefix(d, "32") {
		n := d[2:]
		switch {
		case len(n) == 8: // landline, 0800 / 0900 numbers
		case len(n) == 9 && n[0] == '4': // mobile
		default:
			return "", false
		}
	}
	return "+" + d, true
}

// beOneDigitZones are the Belgian landline zones with a 1-digit prefix
// (Brussels, Antwerp, Liège, Ghent); the others have 2 digits.
var beOneDigitZones = "2349"

// FormatPhone formats an E.164 number for display: « +32 65 35 29 64 »,
// « +32 475 12 34 56 », « +32 2 123 45 67 », « +32 800 12 345 ». Other
// countries and unknown shapes are returned unchanged.
func FormatPhone(e164 string) string {
	if !strings.HasPrefix(e164, "+32") || len(e164) < 11 {
		return e164
	}
	n := e164[3:]
	for _, r := range n {
		if r < '0' || r > '9' {
			return e164
		}
	}
	groups := func(prefix string, rest string, sizes ...int) string {
		parts := []string{"+32", prefix}
		for _, s := range sizes {
			if len(rest) < s {
				break
			}
			parts = append(parts, rest[:s])
			rest = rest[s:]
		}
		if rest != "" {
			parts = append(parts, rest)
		}
		return strings.Join(parts, " ")
	}
	switch {
	case len(n) == 9 && n[0] == '4':
		return groups(n[:3], n[3:], 2, 2, 2)
	case len(n) == 8 && (strings.HasPrefix(n, "800") || strings.HasPrefix(n, "90") || strings.HasPrefix(n, "70") || strings.HasPrefix(n, "78")):
		return groups(n[:3], n[3:], 2, 3)
	case len(n) == 8 && strings.ContainsRune(beOneDigitZones, rune(n[0])):
		return groups(n[:1], n[1:], 3, 2, 2)
	case len(n) == 8:
		return groups(n[:2], n[2:], 2, 2, 2)
	}
	return e164
}

// ---------------------------------------------------------------- address

// postcodeLocalities lists the localities of the postcodes around Mons
// (the first one is used when the source gives no locality).
var postcodeLocalities = map[string][]string{
	"7000": {"Mons"},
	"7010": {"SHAPE"},
	"7011": {"Ghlin"},
	"7012": {"Jemappes", "Flénu"},
	"7020": {"Nimy", "Maisières"},
	"7021": {"Havré"},
	"7022": {"Hyon", "Harmignies", "Harveng", "Mesvin", "Nouvelles"},
	"7024": {"Ciply"},
	"7030": {"Saint-Symphorien"},
	"7031": {"Villers-Saint-Ghislain"},
	"7032": {"Spiennes"},
	"7033": {"Cuesmes"},
	"7034": {"Obourg", "Saint-Denis"},
	"7040": {"Quévy", "Asquillies", "Aulnois", "Blaregnies", "Bougnies", "Genly", "Goegnies-Chaussée", "Quévy-le-Grand", "Quévy-le-Petit"},
	"7041": {"Givry", "Havay"},
	"7050": {"Jurbise", "Erbaut", "Erbisœul", "Herchies", "Masnuy-Saint-Jean"},
	"7060": {"Soignies"},
	"7080": {"Frameries", "Eugies", "La Bouverie", "Noirchain", "Sars-la-Bruyère"},
	"7090": {"Braine-le-Comte"},
	"7100": {"La Louvière"},
	"7130": {"Binche"},
	"7300": {"Boussu"},
	"7301": {"Hornu"},
	"7320": {"Bernissart"},
	"7330": {"Saint-Ghislain"},
	"7331": {"Baudour"},
	"7332": {"Sirault", "Neufmaison"},
	"7333": {"Tertre"},
	"7334": {"Hautrage", "Villerot"},
	"7340": {"Colfontaine", "Wasmes", "Pâturages", "Warquignies"},
	"7350": {"Hensies"},
	"7370": {"Dour"},
	"7380": {"Quiévrain"},
	"7387": {"Honnelles"},
	"7390": {"Quaregnon", "Wasmuel"},
	"7500": {"Tournai"},
	"6000": {"Charleroi"},
	"1000": {"Bruxelles"},
}

// LocalityForPostcode returns the main locality of a postcode of the Mons
// area ("" when unknown).
func LocalityForPostcode(pc string) string {
	if l := postcodeLocalities[pc]; len(l) > 0 {
		return l[0]
	}
	return ""
}

// PostcodeForLocality returns the postcode of a locality of the table
// (accents and case ignored; "" when unknown).
func PostcodeForLocality(loc string) string {
	k := foldKey(loc)
	if k == "" {
		return ""
	}
	for pc, ls := range postcodeLocalities {
		for _, l := range ls {
			if foldKey(l) == k {
				return pc
			}
		}
	}
	return ""
}

var (
	countrySuffixRe = regexp.MustCompile(`(?i)[\s,\-–]*\(?\b(belgique|belgium|belgi[eë]|belgien|be)\b\)?\.?\s*$`)
	// « 7000 Mons », « B-7000 Mons », « BE-7000 »
	pcFirstRe = regexp.MustCompile(`(?i)^(?:b(?:e)?\s?-?\s?)?([1-9]\d{3})\b\s*(.*)$`)
	// « … 7000 », « … 7000 Mons » at the end of a part
	pcLastRe = regexp.MustCompile(`(?i)^(.*?\S)\s+(?:b(?:e)?-)?([1-9]\d{3})(?:\s+(\p{L}[\p{L}\s'’\-.]*))?$`)
	// a leading house number: « 24 Rue de la Clef », « 24A, rue … », « 12-14 rue … »
	leadNumRe = regexp.MustCompile(`^(\d{1,4}\s?[a-zA-Z]?(?:\s?[-/]\s?\d{1,4}[a-zA-Z]?)?)\s*,?\s+(\p{L}.*)$`)
	onlyNumRe = regexp.MustCompile(`^\d{1,4}\s?[a-zA-Z]?(?:\s?[-/]\s?\d{1,4}[a-zA-Z]?)?$`)
	anyNumRe  = regexp.MustCompile(`\d`)
	// streetWordRe: « 1050 Chaussée de … » starts with a house number, not a postcode
	streetWordRe = regexp.MustCompile(`(?i)^(rue|avenue|av\.?|chauss[ée]e|ch\.|place|pl\.|boulevard|bd|route|rte|chemin|square|quai|galerie|all[ée]e|impasse|dr[èe]ve|grand[- ]place|sentier|clos|cit[ée]|rampe|parvis|esplanade)\b`)
)

// junkLocalities are « localities » some platforms put in addresses.
var junkLocalities = map[string]bool{"brussels": true, "bruxelles": true, "belgique": true, "belgium": true, "monscenter": true, "monscentre": true}

// NormalizeAddress rewrites a postal address as « Street Number, Postcode
// Locality »: country removed, leading house number moved after the street,
// all-lowercase / all-uppercase words title-cased, locality inferred from the
// postcode (and the postcode from a known locality). Idempotent; an address
// it cannot read is returned cleaned but otherwise unchanged.
func NormalizeAddress(s string) string {
	s = cleanSpaces(s)
	for {
		next := strings.TrimSpace(countrySuffixRe.ReplaceAllString(s, ""))
		next = strings.TrimRight(next, " ,")
		if next == s || next == "" {
			break
		}
		s = next
	}
	if s == "" {
		return ""
	}
	var parts []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	// postcode + locality: the last part that starts or ends with a postcode
	pc, loc := "", ""
	for i := len(parts) - 1; i >= 0; i-- {
		p := parts[i]
		if m := pcFirstRe.FindStringSubmatch(p); m != nil && !anyNumRe.MatchString(m[2]) && !streetWordRe.MatchString(m[2]) {
			pc, loc = m[1], strings.TrimSpace(m[2])
			parts = append(parts[:i], parts[i+1:]...)
			break
		}
		if m := pcLastRe.FindStringSubmatch(p); m != nil {
			street := strings.TrimSpace(m[1])
			// the 4-digit number is a postcode when the street already has a
			// house number, a locality follows it, or it is a known postcode
			if !anyNumRe.MatchString(street) && m[3] == "" && LocalityForPostcode(m[2]) == "" {
				continue
			}
			pc, loc = m[2], strings.TrimSpace(m[3])
			parts[i] = street
			break
		}
	}
	if pc == "" && len(parts) > 1 {
		// « Rue X 12, Mons »: a known locality without postcode
		last := parts[len(parts)-1]
		if p := PostcodeForLocality(last); p != "" && !anyNumRe.MatchString(last) {
			pc, loc = p, last
			parts = parts[:len(parts)-1]
		}
	}
	loc = strings.Trim(loc, " .-")
	if junkLocalities[foldKey(loc)] && !strings.HasPrefix(pc, "1") {
		loc = ""
	}
	if loc == "" {
		loc = LocalityForPostcode(pc)
	} else {
		loc = fixCase(loc, true)
	}

	// parts repeating the locality, or platform « localities » (Brussels)
	kept := parts[:0]
	for _, p := range parts {
		k := foldKey(p)
		if (junkLocalities[k] && !strings.HasPrefix(pc, "1")) || (pc != "" && k == foldKey(loc)) {
			continue
		}
		kept = append(kept, p)
	}
	parts = kept

	// street: « 24, Rue X » → « Rue X 24 » ; « 24 rue x » → « Rue X 24 »
	if len(parts) >= 2 && onlyNumRe.MatchString(parts[0]) {
		parts = append([]string{parts[1] + " " + parts[0]}, parts[2:]...)
	}
	for i, p := range parts {
		if m := leadNumRe.FindStringSubmatch(p); m != nil && !anyNumRe.MatchString(m[2]) {
			p = m[2] + " " + strings.ReplaceAll(m[1], " ", "")
		}
		parts[i] = fixCase(p, false)
	}
	street := strings.Join(parts, ", ")
	place := strings.TrimSpace(pc + " " + loc)
	switch {
	case street == "":
		return place
	case place == "":
		return street
	}
	return street + ", " + place
}

func cleanSpaces(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '\u00a0' || r == '\u202f' {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func foldKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'à', 'â', 'ä', 'á':
			r = 'a'
		case 'é', 'è', 'ê', 'ë':
			r = 'e'
		case 'î', 'ï':
			r = 'i'
		case 'ô', 'ö':
			r = 'o'
		case 'û', 'ü', 'ù':
			r = 'u'
		case 'ç':
			r = 'c'
		case 'œ':
			b.WriteString("oe")
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// lowerParticles stay in lower case inside a title-cased street name.
var lowerParticles = map[string]bool{
	"de": true, "du": true, "des": true, "la": true, "le": true, "les": true, "l": true, "d": true,
	"aux": true, "au": true, "et": true, "sur": true, "en": true, "à": true, "sous": true, "lez": true,
}

// fixCase title-cases a street / locality written entirely in lower or
// upper case (« rue de la clef » → « Rue de la Clef », « MONS » → « Mons »)
// and upper-cases the first letter otherwise.
func fixCase(s string, locality bool) string {
	hasLower, hasUpper := false, false
	for _, r := range s {
		hasLower = hasLower || unicode.IsLower(r)
		hasUpper = hasUpper || unicode.IsUpper(r)
	}
	if hasLower && hasUpper {
		return upperFirst(s)
	}
	if !hasLower && !hasUpper {
		return s
	}
	var b strings.Builder
	word := 0
	start := true // at the start of a word
	runes := []rune(strings.ToLower(s))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if !unicode.IsLetter(r) {
			b.WriteRune(r)
			if r != '.' {
				start = true
			}
			continue
		}
		if !start {
			b.WriteRune(r)
			continue
		}
		start = false
		j := i
		for j < len(runes) && unicode.IsLetter(runes[j]) {
			j++
		}
		w := string(runes[i:j])
		particle := lowerParticles[w] && word > 0 && !locality
		if locality && word > 0 && (w == "le" || w == "la" || w == "les" || w == "lez" || w == "sur" || w == "en") {
			particle = true
		}
		if particle {
			b.WriteString(w)
		} else {
			b.WriteString(upperFirst(w))
		}
		word++
		i = j - 1
	}
	return b.String()
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError || !unicode.IsLower(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// PhoneOrRaw returns the E.164 form of a phone, or the trimmed input when it
// cannot be read (imports never lose what an admin typed).
func PhoneOrRaw(s string) string {
	if p, ok := NormalizeRestaurantPhone(s); ok {
		return p
	}
	return strings.TrimSpace(s)
}
