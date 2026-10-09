package menusync

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// DefaultPriority is the default menu source preference of Merge.
var DefaultPriority = []string{SourceTakeawaySite, SourceDeliveroo, SourceWeloveat, SourceJSONLD, SourceExisting}

// SameRestaurantMaxKm is the max distance between two records of the same
// restaurant (150 m).
const SameRestaurantMaxKm = 0.150

// MergeGroup describes one output restaurant built from several records.
type MergeGroup struct {
	Slug    string
	Sources []string // "source:name" of each merged record
}

// MergeStats summarises a merge.
type MergeStats struct {
	In, Out int
	Merged  []MergeGroup // only groups of 2+ records
}

// Duplicates is the number of records folded into another one.
func (s MergeStats) Duplicates() int { return s.In - s.Out }

// Merge dedupes restaurants across sources: same provider link, or similar
// name and (coordinates within 150 m or same street + number). Each output
// keeps all provider links and source URLs, its menu from the best source in
// priority, curated fields (slug, name, description, emoji, price level,
// address, active) from the "existing" record when there is one, and every
// other field from the first source in priority that has it. Nothing is
// invented: a field missing everywhere stays empty.
func Merge(in []Restaurant, priority []string) ([]Restaurant, MergeStats) {
	if len(priority) == 0 {
		priority = DefaultPriority
	}
	rank := func(src string) int {
		if i := slices.Index(priority, src); i >= 0 {
			return i
		}
		return len(priority)
	}
	recs := slices.Clone(in)
	sort.SliceStable(recs, func(i, j int) bool { return rank(recs[i].Source) < rank(recs[j].Source) })

	var clusters [][]Restaurant
	for _, r := range recs {
		placed := false
		for ci := range clusters {
			if slices.ContainsFunc(clusters[ci], func(o Restaurant) bool { return SameRestaurant(o, r) }) {
				clusters[ci] = append(clusters[ci], r)
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, []Restaurant{r})
		}
	}

	stats := MergeStats{In: len(in)}
	out := make([]Restaurant, 0, len(clusters))
	for _, c := range clusters {
		m := mergeCluster(c)
		out = append(out, m)
		if len(c) > 1 {
			g := MergeGroup{Slug: m.Slug}
			for _, r := range c {
				g.Sources = append(g.Sources, fmt.Sprintf("%s:%s", sourceOr(r.Source), r.Name))
			}
			stats.Merged = append(stats.Merged, g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	// unique slugs (two distinct places can share a name)
	seen := map[string]int{}
	for i := range out {
		s := out[i].Slug
		if n := seen[s]; n > 0 {
			out[i].Slug = fmt.Sprintf("%s-%d", s, n+1)
		}
		seen[s]++
	}
	stats.Out = len(out)
	return out, stats
}

func sourceOr(s string) string {
	if s == "" {
		return SourceExisting
	}
	return s
}

// mergeCluster merges records already sorted by source priority.
func mergeCluster(c []Restaurant) Restaurant {
	curated := slices.Clone(c)
	sort.SliceStable(curated, func(i, j int) bool {
		return (sourceOr(curated[i].Source) == SourceExisting) && (sourceOr(curated[j].Source) != SourceExisting)
	})

	var m Restaurant
	// menu: first record (priority order) with items
	menuFrom := c[0]
	for _, r := range c {
		if r.ItemCount() > 0 {
			menuFrom = r
			break
		}
	}
	m.Categories = menuFrom.Categories
	m.MenuCheckedAt = menuFrom.MenuCheckedAt
	m.Source = sourceOr(menuFrom.Source)

	for _, r := range curated {
		setStr(&m.Slug, r.Slug)
		setStr(&m.Name, r.Name)
		setStr(&m.Description, r.Description)
		setStr(&m.Emoji, r.Emoji)
		setStr(&m.Address, r.Address)
		if m.PriceLevel == 0 {
			m.PriceLevel = r.PriceLevel
		}
		if m.Active == nil && r.Active != nil {
			a := *r.Active
			m.Active = &a
		}
	}
	bestRating := -1
	for _, r := range c {
		setStr(&m.CoverURL, r.CoverURL)
		setStr(&m.Phone, r.Phone)
		if m.Lat == 0 && m.Lng == 0 && (r.Lat != 0 || r.Lng != 0) {
			m.Lat, m.Lng = r.Lat, r.Lng
		}
		if r.Rating > 0 && r.RatingCount > bestRating {
			m.Rating, m.RatingCount, bestRating = r.Rating, r.RatingCount, r.RatingCount
		}
		if m.EtaMin == 0 && m.EtaMax == 0 {
			m.EtaMin, m.EtaMax = r.EtaMin, r.EtaMax
		}
		if m.DeliveryFee == 0 {
			m.DeliveryFee = r.DeliveryFee
		}
		if m.MinOrder == 0 {
			m.MinOrder = r.MinOrder
		}
		for _, p := range r.Providers {
			if p.URL != "" && !slices.ContainsFunc(m.Providers, func(x providers.Link) bool { return x.ID == p.ID }) {
				m.Providers = append(m.Providers, p)
			}
		}
		m.SourceURLs = appendUnique(m.SourceURLs, r.SourceURLs...)
	}
	for _, r := range curated {
		for _, cu := range r.Cuisines {
			m.Cuisines = appendUnique(m.Cuisines, strings.ToLower(cu))
		}
	}
	return m
}

func setStr(dst *string, v string) {
	if *dst == "" {
		*dst = strings.TrimSpace(v)
	}
}

// SameRestaurant reports whether two records describe the same place.
func SameRestaurant(a, b Restaurant) bool {
	for _, pa := range a.Providers {
		for _, pb := range b.Providers {
			if pa.ID == pb.ID && pa.URL != "" && normURL(pa.URL) == normURL(pb.URL) {
				return true
			}
		}
	}
	if !SimilarNames(a.Name, b.Name) {
		return false
	}
	aGeo, bGeo := a.Lat != 0 || a.Lng != 0, b.Lat != 0 || b.Lng != 0
	if aGeo && bGeo {
		if domain.HaversineKm(a.Lat, a.Lng, b.Lat, b.Lng) <= SameRestaurantMaxKm {
			return true
		}
	}
	return SameStreetAddress(a.Address, b.Address)
}

var takeawayPathRe = regexp.MustCompile(`/menu/([a-z0-9-]+)`)

func normURL(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	u, _, _ = strings.Cut(u, "?")
	u = strings.TrimSuffix(u, "/")
	for _, p := range []string{"https://", "http://", "www."} {
		u = strings.TrimPrefix(u, p)
	}
	// takeaway.com/be/menu/x and takeaway.com/be-fr/menu/x are the same store
	if strings.HasPrefix(u, "takeaway.com/") {
		if m := takeawayPathRe.FindStringSubmatch(u); m != nil {
			return "takeaway.com/menu/" + m[1]
		}
	}
	return u
}

var nameStop = map[string]bool{
	"mons": true, "restaurant": true, "resto": true, "center": true, "centre": true,
	"the": true, "le": true, "la": true, "les": true, "l": true, "s": true, "be": true, "et": true, "and": true,
}

func nameTokens(s string) []string {
	s = strings.NewReplacer("’", "", "'", "", "&", " ").Replace(Fold(s))
	var out []string
	for _, t := range nonSlug.Split(s, -1) {
		if t != "" && !nameStop[t] {
			out = append(out, t)
		}
	}
	return out
}

// SimilarNames compares names loosely: same significant words, one name's
// words included in the other's ("Tomo" ~ "TOMO RAMEN"), or same letters
// once spaces are removed ("Donrolls" ~ "DON ROLL'S").
func SimilarNames(a, b string) bool {
	ta, tb := nameTokens(a), nameTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	if strings.Join(ta, "") == strings.Join(tb, "") {
		return true
	}
	subset := func(x, y []string) bool {
		for _, t := range x {
			if !slices.Contains(y, t) {
				return false
			}
		}
		return true
	}
	return subset(ta, tb) || subset(tb, ta)
}

var (
	houseNumRe   = regexp.MustCompile(`\b(\d{1,4}[a-z]?)\b`)
	postcodeRe   = regexp.MustCompile(`\b\d{4}\b`)
	streetFiller = map[string]bool{
		"rue": true, "de": true, "du": true, "des": true, "la": true, "le": true, "l": true, "d": true,
		"chaussee": true, "chau": true, "place": true, "pl": true, "avenue": true, "av": true,
		"route": true, "rte": true, "boulevard": true, "bd": true, "mons": true,
		"belgique": true, "belgium": true, "brussels": true, "b": true,
	}
)

// SameStreetAddress compares two postal addresses: same house number and at
// least one significant street word in common.
func SameStreetAddress(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	parse := func(s string) (map[string]bool, map[string]bool) {
		s = strings.NewReplacer("’", " ", "'", " ").Replace(Fold(s))
		s = postcodeRe.ReplaceAllString(s, " ")
		nums, words := map[string]bool{}, map[string]bool{}
		for _, m := range houseNumRe.FindAllString(s, -1) {
			nums[m] = true
		}
		for _, w := range nonSlug.Split(s, -1) {
			if len(w) > 2 && !streetFiller[w] && !houseNumRe.MatchString(w) {
				words[w] = true
			}
		}
		return nums, words
	}
	na, wa := parse(a)
	nb, wb := parse(b)
	common := func(x, y map[string]bool) bool {
		for k := range x {
			if y[k] {
				return true
			}
		}
		return false
	}
	return common(na, nb) && common(wa, wb)
}
