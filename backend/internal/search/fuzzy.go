package search

import (
	"sort"
	"strings"
)

// Typo tolerance: a query term that is not the prefix of any indexed term is
// replaced by the indexed terms closest to it (Damerau–Levenshtein distance,
// 1 for 4–6 letters, 2 from 7 letters), closest first, then the longest
// common prefix, then the most frequent. « piza » → « pizza », « ramne » →
// « ramen », « pizzas » → « pizza ». Shorter terms are never corrected.

// VocabTerm is an indexed term with the number of documents containing it.
type VocabTerm struct {
	Term string
	Docs int
}

// Vocab is the sorted list of indexed terms.
type Vocab []VocabTerm

// NewVocab sorts and dedupes terms (doc counts added up).
func NewVocab(terms []VocabTerm) Vocab {
	byTerm := map[string]int{}
	for _, t := range terms {
		if t.Term != "" {
			byTerm[t.Term] += t.Docs
		}
	}
	v := make(Vocab, 0, len(byTerm))
	for t, n := range byTerm {
		v = append(v, VocabTerm{Term: t, Docs: n})
	}
	sort.Slice(v, func(i, j int) bool { return v[i].Term < v[j].Term })
	return v
}

// HasPrefix reports whether an indexed term starts with p.
func (v Vocab) HasPrefix(p string) bool {
	i := sort.Search(len(v), func(i int) bool { return v[i].Term >= p })
	return i < len(v) && strings.HasPrefix(v[i].Term, p)
}

// maxDistance is the accepted edit distance for a term of n runes (0 = never corrected).
func maxDistance(n int) int {
	switch {
	case n >= 7:
		return 2
	case n >= 4:
		return 1
	default:
		return 0
	}
}

// maxAlternatives bounds the corrections kept per term.
const maxAlternatives = 3

// Corrections returns the indexed terms closest to term, best first (nil
// when term is a known prefix, too short, or nothing is close enough). A
// term is also compared to the prefixes of longer indexed terms so that a
// typo in a beginning (« ramn » for « ramen ») is still found.
func (v Vocab) Corrections(term string) []string {
	if v.HasPrefix(term) {
		return nil
	}
	tr := []rune(term)
	maxD := maxDistance(len(tr))
	if maxD == 0 {
		return nil
	}
	type cand struct {
		term   string
		d      int
		prefix int
		docs   int
	}
	var cands []cand
	for _, vt := range v {
		vr := []rune(vt.Term)
		if len(vr) < len(tr)-maxD {
			continue
		}
		score := -1
		if d := distance(tr, vr, maxD); d <= maxD {
			score = d * 2
		} else if len(vr) > len(tr) {
			// a typo in the beginning of a longer word (prefix search):
			// ranked after a whole-word correction of the same distance
			if pd := distance(tr, vr[:len(tr)], maxD); pd <= maxD {
				score = pd*2 + 1
			}
		}
		if score >= 0 {
			cands = append(cands, cand{vt.Term, score, commonPrefix(tr, vr), vt.Docs})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].d != cands[j].d {
			return cands[i].d < cands[j].d
		}
		// typos rarely hit the first letters: « piza » → « pizza » before « pita »
		if cands[i].prefix != cands[j].prefix {
			return cands[i].prefix > cands[j].prefix
		}
		if cands[i].docs != cands[j].docs {
			return cands[i].docs > cands[j].docs
		}
		return cands[i].term < cands[j].term
	})
	// prefix corrections (odd scores) only when no whole word is close enough
	if len(cands) > 0 && cands[0].d%2 == 0 {
		whole := cands[:0]
		for _, c := range cands {
			if c.d%2 == 0 {
				whole = append(whole, c)
			}
		}
		cands = whole
	}
	out := []string{}
	for _, c := range cands {
		out = append(out, c.term)
		if len(out) == maxAlternatives {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// distance is the optimal string alignment (Damerau–Levenshtein) distance
// between a and b, or a value > maxD as soon as it exceeds maxD.
func distance(a, b []rune, maxD int) int {
	if d := len(a) - len(b); d > maxD || -d > maxD {
		return maxD + 1
	}
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
			rowMin = min(rowMin, cur[j])
		}
		if rowMin > maxD {
			return maxD + 1
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}

func commonPrefix(a, b []rune) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}
