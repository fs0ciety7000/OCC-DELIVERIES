package menusync

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Loose name matching used to recognise the restaurants of the Uber Eats
// snapshot (store names such as « CTR Chicken Mons (Independant) ») among
// the stored ones (« CTR Chicken »).

// storeLabelRe matches a trailing platform label: « (Independant) »,
// « (Indépendant) », « [Independent] ».
var storeLabelRe = regexp.MustCompile(`(?i)\s*[(\[]\s*(ind[eé]pendante?|independent|franchise)\s*[)\]]\s*$`)

// MinContainedKey is the minimal length of a name key contained in another
// one for the two names to match (« donrolls » in « donrollssushi »).
const MinContainedKey = 6

// MinTokenSetRatio is the minimal token-set similarity of two names.
const MinTokenSetRatio = 0.8

// CleanStoreName normalizes a store name as published by a delivery
// platform: platform labels (« (Independant) ») and city suffixes
// (« (Mons) », « - Mons », a trailing « Mons ») removed, whitespace
// collapsed, ALL-CAPS names title-cased (see NormalizeName).
func CleanStoreName(name, city string) string {
	name = CleanText(name)
	for {
		next := storeLabelRe.ReplaceAllString(name, "")
		if utf8.RuneCountInString(strings.TrimSpace(next)) < 3 {
			next = name
		}
		next = NormalizeName(next, city)
		if next == name {
			return name
		}
		name = next
	}
}

// StoreNameKey is the comparison key of a store name (CleanStoreName, then
// accents, case, punctuation and stop words ignored).
func StoreNameKey(name, city string) string {
	return strings.Join(nameTokens(CleanStoreName(name, city)), "")
}

func hasSignificant(tokens []string) bool {
	return slices.ContainsFunc(tokens, func(t string) bool { return !genericNameWords[t] })
}

// Name match strengths returned by NameMatch (higher is stronger).
const (
	NameNoMatch   = 0
	NameTokenSet  = 1 // token-set similarity ≥ MinTokenSetRatio
	NameContained = 2 // one key contains the other (≥ MinContainedKey chars)
	NameSameKey   = 3 // same key
)

// NameMatch compares two store names loosely and returns the strength of the
// match: same key, one key containing the other (at least MinContainedKey
// characters, not only generic words such as « pizza »), or a token-set
// similarity ≥ MinTokenSetRatio whose common words are significant and long
// enough (« Snack Pitta Grill Akropolis » ~ « Snack Pitta Grec Akropolis »).
func NameMatch(a, b, city string) int {
	ta, tb := nameTokens(CleanStoreName(a, city)), nameTokens(CleanStoreName(b, city))
	ka, kb := strings.Join(ta, ""), strings.Join(tb, "")
	if ka == "" || kb == "" {
		return NameNoMatch
	}
	if ka == kb {
		return NameSameKey
	}
	short, long, shortTokens := ka, kb, ta
	if len(short) > len(long) {
		short, long, shortTokens = kb, ka, tb
	}
	if len(short) >= MinContainedKey && strings.Contains(long, short) && hasSignificant(shortTokens) {
		return NameContained
	}
	common := intersect(ta, tb)
	if hasSignificant(common) && len(strings.Join(common, "")) >= MinContainedKey && TokenSetRatio(ta, tb) >= MinTokenSetRatio {
		return NameTokenSet
	}
	return NameNoMatch
}

func uniqueSorted(ts []string) []string {
	out := slices.Clone(ts)
	slices.Sort(out)
	return slices.Compact(out)
}

func intersect(a, b []string) []string {
	var out []string
	for _, t := range uniqueSorted(a) {
		if slices.Contains(b, t) {
			out = append(out, t)
		}
	}
	return out
}

func minus(a, b []string) []string {
	var out []string
	for _, t := range uniqueSorted(a) {
		if !slices.Contains(b, t) {
			out = append(out, t)
		}
	}
	return out
}

// TokenSetRatio is the token-set similarity of two token lists (0–1), as
// popularised by fuzzywuzzy: the sorted common words are compared with each
// side's « common + own words », and the best ratio is kept. A list included
// in the other scores 1.
func TokenSetRatio(a, b []string) float64 {
	common := strings.Join(intersect(a, b), " ")
	join := func(rest []string) string {
		return strings.TrimSpace(common + " " + strings.Join(rest, " "))
	}
	t1, t2 := join(minus(a, b)), join(minus(b, a))
	return max(Ratio(common, t1), Ratio(common, t2), Ratio(t1, t2))
}

// Ratio is the similarity of two strings (0–1): 2 × longest common
// subsequence / total length (the indel similarity).
func Ratio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	total := len(ra) + len(rb)
	if total == 0 {
		return 1
	}
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			switch {
			case ra[i-1] == rb[j-1]:
				cur[j] = prev[j-1] + 1
			case prev[j] >= cur[j-1]:
				cur[j] = prev[j]
			default:
				cur[j] = cur[j-1]
			}
		}
		prev, cur = cur, prev
	}
	return 2 * float64(prev[len(rb)]) / float64(total)
}
