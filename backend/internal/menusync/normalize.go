package menusync

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// MaxCuisines is the number of cuisine tags kept per restaurant.
const MaxCuisines = 4

// --- restaurant names ------------------------------------------------------

// acronyms stay upper-case when an ALL-CAPS name is title-cased.
var acronyms = map[string]bool{"bbq": true, "kfc": true, "xl": true, "xxl": true, "ob": true, "usa": true, "ny": true, "bx": true, "ctr": true}

// smallWords stay lower-case inside a title-cased French name.
var smallWords = map[string]bool{
	"de": true, "du": true, "des": true, "la": true, "le": true, "les": true, "et": true,
	"à": true, "au": true, "aux": true, "en": true, "sur": true, "of": true, "and": true,
}

var (
	parenSuffixRe = regexp.MustCompile(`\s*\(([^()]*)\)\s*$`)
	dashPartRe    = regexp.MustCompile(`\s+[-–—|]\s+`)
)

// NormalizeName cleans a restaurant name as published by a platform:
// whitespace collapsed, platform/city suffixes removed (" - Mons - Mons
// Center", " (MON)", a trailing " Mons" repeating the city) and ALL-CAPS
// names title-cased (French small words lower-case, short acronyms kept).
// city is the search city ("mons"); "" disables the suffix rules.
func NormalizeName(name, city string) string {
	name = CleanText(name)
	if name == "" {
		return ""
	}
	if c := Fold(strings.TrimSpace(city)); c != "" {
		name = stripCitySuffix(name, c)
	}
	if isAllCaps(name) {
		name = TitleCase(name)
	}
	return name
}

// isCityPart reports whether a (folded) name part only names the city or one
// of its delivery zones: "mons", "mons center", "mon", "mons-nord"…
func isCityPart(part, city string) bool {
	p := strings.Trim(nonSlug.ReplaceAllString(Fold(part), " "), " ")
	if p == "" {
		return false
	}
	if p == city || (len(p) >= 3 && strings.HasPrefix(city, p)) {
		return true
	}
	rest, ok := strings.CutPrefix(p, city+" ")
	if !ok {
		return false
	}
	switch rest {
	case "center", "centre", "centrum", "north", "nord", "south", "sud", "east", "est", "west", "ouest", "ville":
		return true
	}
	return false
}

func stripCitySuffix(name, city string) string {
	keep := func(s string) bool {
		return utf8.RuneCountInString(strings.TrimSpace(s)) >= 3 && !danglingName(s)
	}
	for {
		changed := false
		if m := parenSuffixRe.FindStringSubmatchIndex(name); m != nil {
			inner := name[m[2]:m[3]]
			if isCityPart(inner, city) && keep(name[:m[0]]) {
				name, changed = strings.TrimSpace(name[:m[0]]), true
			}
		}
		if locs := dashPartRe.FindAllStringIndex(name, -1); len(locs) > 0 {
			last := locs[len(locs)-1]
			if isCityPart(name[last[1]:], city) && keep(name[:last[0]]) {
				name, changed = strings.TrimSpace(name[:last[0]]), true
			}
		}
		if i := strings.LastIndexByte(name, ' '); i > 0 && Fold(name[i+1:]) == city && keep(name[:i]) {
			name, changed = strings.TrimSpace(name[:i]), true
		}
		if !changed {
			return strings.Trim(name, " -–—|,")
		}
	}
}

// nameConnectors end a name that still expects a complement: removing the
// city after them leaves "O'Tacos Centre ville de".
var nameConnectors = map[string]bool{
	"de": true, "du": true, "des": true, "d": true, "a": true, "au": true, "aux": true,
	"la": true, "le": true, "les": true, "l": true, "of": true, "in": true, "en": true, "sur": true,
}

// danglingName reports whether a name left once the city is removed would be
// truncated or meaningless: it ends with a connector ("… ville de") or is a
// single generic word ("Pitta", "Pizza", "Snack").
func danglingName(s string) bool {
	words := strings.Fields(strings.Trim(nonSlug.ReplaceAllString(Fold(s), " "), " "))
	if len(words) == 0 {
		return true
	}
	if nameConnectors[words[len(words)-1]] {
		return true
	}
	return len(words) == 1 && genericNameWords[words[0]]
}

// TruncatedName reports whether the stored name looks like a truncated form
// of the name now published by the source ("Pitta" for "Pitta Mons",
// "O'Tacos Centre ville de" for "O'Tacos Centre ville de Mons"), so that the
// synchronisation may repair it.
func TruncatedName(stored, published string) bool {
	s, p := Fold(strings.TrimSpace(stored)), Fold(strings.TrimSpace(published))
	return s != "" && s != p && strings.HasPrefix(p, s) && danglingName(stored)
}

func isAllCaps(s string) bool {
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters >= 2 && letters == upper
}

func hasVowel(s string) bool {
	return strings.ContainsAny(Fold(s), "aeiouy")
}

// TitleCase title-cases a French name: "L’ARÈNE SANDWICHERIE" → "L’Arène
// Sandwicherie", "PIZZA DE LA GARE" → "Pizza de la Gare", "BBQ HOME" →
// "BBQ Home".
func TitleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		bare := strings.Trim(w, "()[]\"«».,!:;*")
		switch {
		case acronyms[bare] || (len(bare) >= 2 && len(bare) <= 4 && !hasVowel(bare) && isLetters(bare)):
			words[i] = strings.Replace(w, bare, strings.ToUpper(bare), 1)
		case i > 0 && smallWords[bare] && strings.Trim(words[i-1], "-–—|:") != "":
			// stays lower-case
		default:
			words[i] = capitalizeParts(w)
		}
	}
	return strings.Join(words, " ")
}

func isLetters(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}

// capitalizeParts upper-cases the first letter of a word, the letter
// following a hyphen and the one following an elided article ("l’arène" →
// "L’Arène", "pitta-grec" → "Pitta-Grec", but "season’s" → "Season’s").
func capitalizeParts(w string) string {
	rs := []rune(w)
	up, seg := true, 0 // seg = letters since the last separator
	for i, r := range rs {
		if unicode.IsLetter(r) {
			if up {
				rs[i] = unicode.ToUpper(r)
			}
			up = false
			seg++
			continue
		}
		switch r {
		case '\'', '’':
			up = seg == 1 // l’, d’, o’
		case '-', '(', '"', '«':
			up = true
		}
		seg = 0
	}
	return string(rs)
}

// NameKey is the comparison key of a restaurant name: accents, case,
// punctuation and city suffixes ignored ("BAGEL CITY" == "Bagel City").
func NameKey(name, city string) string {
	return strings.Join(nameTokens(NormalizeName(name, city)), "")
}

// --- cuisines ----------------------------------------------------------------

// cuisineSynonyms maps folded tags to their canonical spelling.
var cuisineSynonyms = map[string]string{
	"burgers": "burger", "hamburger": "burger", "hamburgers": "burger",
	"pizzas":    "pizza",
	"sandwichs": "sandwich", "sandwiches": "sandwich", "sandwicherie": "sandwich",
	"thailandais": "thaï", "thai": "thaï", "thaïlandais": "thaï", "thailand": "thaï",
	"poke": "poké", "poke bowl": "poké", "poke bowls": "poké", "pokes": "poké",
	"fast food": "fast-food", "fastfood": "fast-food", "fast-food": "fast-food",
	"sushis":  "sushi",
	"salades": "salade",
	"bagels":  "bagel",
	"grill":   "grillades", "grills": "grillades",
	"kebabs":       "kebab",
	"boulangeries": "boulangerie",
	"pates":        "pâtes",
	"vegetarien":   "végétarien", "vegetarian": "végétarien",
	"italian": "italien", "japanese": "japonais", "chinese": "chinois", "indian": "indien",
	"libanaise": "libanais", "mexicaine": "mexicain",
	"dessert": "desserts",
	"boisson": "boissons", "drinks": "boissons",
}

// genericCuisines are dropped when the restaurant has other cuisines.
var genericCuisines = map[string]bool{
	"boissons": true, "desserts": true, "légumes": true, "chill": true, "snacks": true, "autres": true, "divers": true,
}

// NormalizeCuisine returns the canonical form of one cuisine tag.
func NormalizeCuisine(c string) string {
	c = strings.ToLower(CleanText(c))
	if c == "" {
		return ""
	}
	if v, ok := cuisineSynonyms[Fold(c)]; ok {
		return v
	}
	if v, ok := cuisineSynonyms[c]; ok {
		return v
	}
	return c
}

// NormalizeCuisines lower-cases, unifies synonyms and plurals, drops generic
// tags (drinks, desserts…) when there are other cuisines, dedupes and keeps
// at most MaxCuisines tags, in their original order.
func NormalizeCuisines(in []string) []string {
	var all []string
	for _, c := range in {
		all = appendUnique(all, NormalizeCuisine(c))
	}
	specific := slices.DeleteFunc(slices.Clone(all), func(c string) bool { return genericCuisines[c] })
	if len(specific) > 0 {
		all = specific
	}
	if len(all) > MaxCuisines {
		all = all[:MaxCuisines]
	}
	if all == nil {
		all = []string{}
	}
	return all
}

// --- menu items --------------------------------------------------------------

// CleanMenu tidies a menu: names and descriptions trimmed with whitespace
// collapsed, zero-price items dropped unless they carry option groups (the
// price then lives in the options), identical items of a category deduped,
// same-name categories merged, empty categories dropped.
func CleanMenu(cats []catalog.CategoryImport) []catalog.CategoryImport {
	out := make([]catalog.CategoryImport, 0, len(cats))
	byName := map[string]int{}
	for _, c := range cats {
		c.Name = CleanText(c.Name)
		if c.Name == "" {
			continue
		}
		idx, ok := byName[Fold(c.Name)]
		if !ok {
			idx = len(out)
			byName[Fold(c.Name)] = idx
			out = append(out, catalog.CategoryImport{Name: c.Name, Items: []catalog.ItemImport{}})
		}
		for _, it := range c.Items {
			it.Name = CleanText(it.Name)
			it.Description = CleanText(it.Description)
			if it.Name == "" {
				continue
			}
			if strings.EqualFold(it.Description, it.Name) {
				it.Description = ""
			}
			if it.Price <= 0 && len(it.OptionGroups) == 0 {
				continue
			}
			if slices.ContainsFunc(out[idx].Items, func(o catalog.ItemImport) bool { return sameItem(o, it) }) {
				continue
			}
			out[idx].Items = append(out[idx].Items, it)
		}
	}
	return dropEmptyCategories(out)
}

func sameItem(a, b catalog.ItemImport) bool {
	return Fold(a.Name) == Fold(b.Name) && a.Price == b.Price && Fold(a.Description) == Fold(b.Description) &&
		optionsEqual(a.OptionGroups, b.OptionGroups)
}

func optionsEqual(a, b []domain.OptionGroup) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || len(a[i].Choices) != len(b[i].Choices) {
			return false
		}
	}
	return true
}

// Clean applies every data-quality rule to a restaurant: name, cuisines and
// menu (see NormalizeName, NormalizeCuisines, CleanMenu).
func (r *Restaurant) Clean(city string) {
	r.Name = NormalizeName(r.Name, city)
	r.Cuisines = NormalizeCuisines(r.Cuisines)
	r.Categories = CleanMenu(r.Categories)
	r.Address = CleanText(r.Address)
	r.Description = CleanText(r.Description)
	r.Phone = CleanText(r.Phone)
	r.normalizeContact()
	// a cover must be an absolute web URL ("?width=1200…" without host is a
	// platform placeholder)
	if c := strings.TrimSpace(r.CoverURL); !strings.HasPrefix(c, "https://") && !strings.HasPrefix(c, "http://") {
		r.CoverURL = ""
	}
}
