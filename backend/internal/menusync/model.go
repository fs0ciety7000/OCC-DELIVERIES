// Package menusync retrieves restaurant + menu feeds from public pages of
// delivery platforms and restaurant websites, and turns them into the
// RestaurantImport format accepted by POST /api/occ/admin/import.
//
// Politeness is mandatory (see Fetcher): identified User-Agent, sequential
// requests with a delay, robots.txt enforced, on-disk cache, and a hard stop
// on 403 / anti-bot challenge pages. It never tries to bypass a protection.
//
// Parsing functions are pure (bytes in, structs out) and tested on trimmed
// fixtures under testdata/.
package menusync

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
)

// Source ids.
const (
	SourceDeliveroo    = "deliveroo"
	SourceWeloveat     = "weloveat"
	SourceTakeawaySite = "takeaway-site"
	SourceJSONLD       = "jsonld"
	SourceExisting     = "existing"
)

// Restaurant is a RestaurantImport plus provenance keys (ignored by the import).
type Restaurant struct {
	catalog.RestaurantImport
	// Source is the menusync source that produced the record ("" = existing data).
	Source        string   `json:"source,omitempty"`
	SourceURLs    []string `json:"source_urls"`
	MenuCheckedAt string   `json:"menu_checked_at,omitempty"`
	// Origins lists every source record merged into this one.
	Origins []Origin `json:"origins,omitempty"`
}

// Origin is one source record of a (merged) restaurant.
type Origin struct {
	Source    string `json:"source"`
	URL       string `json:"url"`
	CheckedAt string `json:"checked_at,omitempty"`
}

// SourceKey is the stable id of the record: "<source>:<normalized url>" of
// the source its menu comes from ("" when unknown).
func (r Restaurant) SourceKey() string {
	pick := func(o Origin) string { return o.Source + ":" + NormURL(o.URL) }
	for _, o := range r.Origins {
		if o.Source == r.Source && o.URL != "" {
			return pick(o)
		}
	}
	for _, o := range r.Origins {
		if o.URL != "" && o.Source != SourceExisting {
			return pick(o)
		}
	}
	if r.Source != "" && r.Source != SourceExisting && len(r.SourceURLs) > 0 {
		return r.Source + ":" + NormURL(r.SourceURLs[0])
	}
	return ""
}

// ItemCount returns the number of menu items.
func (r Restaurant) ItemCount() int {
	n := 0
	for _, c := range r.Categories {
		n += len(c.Items)
	}
	return n
}

// ItemsWithOptions returns the number of items having at least one option group.
func (r Restaurant) ItemsWithOptions() int {
	n := 0
	for _, c := range r.Categories {
		for _, it := range c.Items {
			if len(it.OptionGroups) > 0 {
				n++
			}
		}
	}
	return n
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Fold lowercases and strips accents.
func Fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	return strings.ToLower(out)
}

// Slugify turns a name into a merge-friendly slug ("Ô Sando" → "o-sando").
func Slugify(s string) string {
	s = strings.NewReplacer("’", "", "'", "", "&", " et ", "œ", "oe", "Œ", "oe").Replace(s)
	s = strings.Trim(nonSlug.ReplaceAllString(Fold(s), "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}

var spaces = regexp.MustCompile(`\s+`)

// CleanText collapses whitespace (incl. NBSP) and trims.
func CleanText(s string) string {
	s = strings.ReplaceAll(s, " ", " ")
	s = strings.ReplaceAll(s, " ", " ")
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}

var priceRe = regexp.MustCompile(`(\d+(?:[ .]\d{3})*)(?:[.,](\d{1,2}))?`)

// ParseEuroCents parses "€ 16,00", "16.5", "4,95 €" into cents. ok=false if
// no number was found.
func ParseEuroCents(s string) (int, bool) {
	m := priceRe.FindStringSubmatch(strings.ReplaceAll(s, " ", " "))
	if m == nil {
		return 0, false
	}
	units, err := strconv.Atoi(strings.NewReplacer(" ", "", ".", "").Replace(m[1]))
	if err != nil {
		return 0, false
	}
	cents := 0
	if m[2] != "" {
		frac := m[2]
		if len(frac) == 1 {
			frac += "0"
		}
		cents, _ = strconv.Atoi(frac)
	}
	return units*100 + cents, true
}

// EurosToCents converts a float amount in euros to cents.
func EurosToCents(v float64) int {
	return int(math.Round(v * 100))
}

func appendUnique(list []string, vs ...string) []string {
	for _, v := range vs {
		if v == "" {
			continue
		}
		found := false
		for _, x := range list {
			if x == v {
				found = true
				break
			}
		}
		if !found {
			list = append(list, v)
		}
	}
	return list
}
