// Package search is the global search of OCC Deliveries: SQLite FTS5 indexes
// of the restaurants and menu items (migration 1760000017), kept in sync by
// record hooks and explicit Reindex calls, queried by GET /api/occ/search.
//
// Text is folded in Go (lower case, accents and ligatures removed) both when
// indexing and when querying, so « Râmen », « RAMEN » and « ramen » match the
// same terms whatever the SQLite tokenizer does.
package search

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ligatures are not decomposed by NFD: fold them explicitly.
var ligatures = map[rune]string{
	'œ': "oe", 'Œ': "oe", 'æ': "ae", 'Æ': "ae", 'ß': "ss", 'ﬁ': "fi", 'ﬂ': "fl",
}

// foldRune returns the folded form of one rune (possibly empty or several runes).
func foldRune(r rune) string {
	if l, ok := ligatures[r]; ok {
		return l
	}
	if r < unicode.MaxASCII {
		return string(unicode.ToLower(r))
	}
	var b strings.Builder
	for _, d := range norm.NFD.String(string(r)) {
		if unicode.Is(unicode.Mn, d) {
			continue
		}
		b.WriteRune(unicode.ToLower(d))
	}
	return b.String()
}

// Fold lower-cases s and strips accents / ligatures: « Bœuf Râpé » → « boeuf rape ».
func Fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteString(foldRune(r))
	}
	return b.String()
}

// isWordRune reports whether a folded rune belongs to a search term.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// MaxTerms bounds the number of query terms.
const MaxTerms = 6

// maxTermLen bounds the length of one query term (in runes).
const maxTermLen = 32

// Terms splits a query into folded search terms (letters and digits only),
// without duplicates, at most MaxTerms.
func Terms(q string) []string {
	out := []string{}
	for _, w := range strings.FieldsFunc(Fold(q), func(r rune) bool { return !isWordRune(r) }) {
		if rs := []rune(w); len(rs) > maxTermLen {
			w = string(rs[:maxTermLen])
		}
		dup := false
		for _, o := range out {
			if o == w {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, w)
		}
		if len(out) == MaxTerms {
			break
		}
	}
	return out
}

// Words returns the folded words of s (for matching names in Go).
func Words(s string) []string {
	return strings.FieldsFunc(Fold(s), func(r rune) bool { return !isWordRune(r) })
}

// foldedWithOffsets folds s and returns, for each byte of the folded string,
// the byte offset of the original rune it comes from (len = len(folded)+1).
func foldedWithOffsets(s string) (string, []int) {
	var b strings.Builder
	offs := make([]int, 0, len(s)+1)
	for i, r := range s {
		f := foldRune(r)
		b.WriteString(f)
		for range len(f) {
			offs = append(offs, i)
		}
	}
	offs = append(offs, len(s))
	return b.String(), offs
}

// snippetRadius is the number of runes kept around the first match.
const snippetRadius = 40

// Snippet returns a short excerpt of text (original accents kept) around the
// first occurrence of one of the folded terms, with « … » when cut. Without
// any match it returns the beginning of the text. Plain text: the client
// highlights the terms itself.
func Snippet(text string, terms []string, maxRunes int) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}
	folded, offs := foldedWithOffsets(text)
	start := -1
	for _, t := range terms {
		if t == "" {
			continue
		}
		if i := indexWordPrefix(folded, t); i >= 0 && (start < 0 || i < start) {
			start = i
		}
	}
	runes := []rune(text)
	if start < 0 {
		if len(runes) <= maxRunes {
			return text
		}
		return strings.TrimSpace(string(runes[:maxRunes])) + "…"
	}
	// convert the byte offset of the match to a rune index in text
	orig := offs[start]
	ri := len([]rune(text[:orig]))
	from := max(ri-min(snippetRadius, maxRunes/3), 0)
	to := min(from+maxRunes, len(runes))
	if to-from < maxRunes {
		from = max(to-maxRunes, 0)
	}
	// do not cut words at the edges
	for from > 0 && from < ri && runes[from-1] != ' ' {
		from++
	}
	for to < len(runes) && to > ri && runes[to-1] != ' ' && runes[to] != ' ' {
		to--
	}
	out := strings.TrimSpace(string(runes[from:to]))
	if from > 0 {
		out = "…" + out
	}
	if to < len(runes) {
		out += "…"
	}
	return out
}

// indexWordPrefix returns the byte index of the first word of folded that
// starts with term (-1 if none).
func indexWordPrefix(folded, term string) int {
	for off := 0; off < len(folded); {
		i := strings.Index(folded[off:], term)
		if i < 0 {
			return -1
		}
		i += off
		if i == 0 || !isWordRune(lastRune(folded[:i])) {
			return i
		}
		off = i + len(term)
	}
	return -1
}

func lastRune(s string) rune {
	r := []rune(s)
	if len(r) == 0 {
		return ' '
	}
	return r[len(r)-1]
}
