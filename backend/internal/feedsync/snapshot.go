package feedsync

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// The Uber Eats snapshot (docs/adr/0002-providers.md, update 3): the lead
// retrieves the Uber Eats restaurants delivering to the office through the
// official Uber Eats connector of a Claude session (the server cannot call
// it) and commits them to migrations/data/mons_ubereats.json. The source
// "ubereats-snapshot" reads that embedded file — no network — and its
// restaurants are reconciled with special rules (see reconcileSnapshot):
// only a few sample items are known, so a restaurant it creates carries
// partial_menu = true until a real feed supplies a full menu.

// ProviderUberEatsSnapshot is the sync_sources.provider of the snapshot.
const ProviderUberEatsSnapshot = "ubereats-snapshot"

// PreviewCategory is the menu category holding the snapshot sample items.
const PreviewCategory = "Aperçu"

// FullMenuMinItems is the number of items a feed must supply to replace the
// partial menu of a snapshot restaurant.
const FullMenuMinItems = 5

// SnapshotEtaSpread is added to the snapshot ETA to get eta_max (minutes).
const SnapshotEtaSpread = 15

// SnapshotEntry is one restaurant of the snapshot file.
type SnapshotEntry struct {
	Name        string         `json:"name"`
	URL         string         `json:"url"`
	Rating      float64        `json:"rating"`
	RatingCount int            `json:"rating_count"`
	EtaMin      int            `json:"eta_min"`
	Categories  []string       `json:"categories"`
	Promo       string         `json:"promo"`
	Lat         float64        `json:"lat"`
	Lng         float64        `json:"lng"`
	Address     string         `json:"address"`
	GeoApprox   bool           `json:"geo_approx"`
	Items       []SnapshotItem `json:"items"`
	CheckedAt   string         `json:"checked_at"`
}

// SnapshotItem is a sample item (price in cents).
type SnapshotItem struct {
	Name  string `json:"name"`
	Price int    `json:"price"`
}

// HasGeo reports whether the entry has real (not approximate) coordinates.
func (e SnapshotEntry) HasGeo() bool { return !e.GeoApprox && (e.Lat != 0 || e.Lng != 0) }

// Key is the source key of the restaurants created from the entry.
func (e SnapshotEntry) Key() string { return ProviderUberEatsSnapshot + ":" + menusync.NormURL(e.URL) }

// cleanUberEatsURL validates an Uber Eats store URL and drops its query
// string and fragment ("" when invalid).
func cleanUberEatsURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "ubereats.com" && !strings.HasSuffix(host, ".ubereats.com") {
		return ""
	}
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimSuffix(u.String(), "/")
}

// ParseSnapshot decodes the snapshot file. Invalid entries (no name, no Uber
// Eats URL, duplicate URL) are skipped and reported in problems; items
// without a name or a positive price are dropped. Numbers are clamped (note
// 0–5, counts ≥ 0). An empty file yields nothing; malformed JSON is an error.
func ParseSnapshot(data []byte, city string) (entries []SnapshotEntry, problems []string, err error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil, nil
	}
	var raw []SnapshotEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("fichier Uber Eats illisible (JSON invalide) : %w", err)
	}
	seen := map[string]bool{}
	for i, e := range raw {
		e.Name = menusync.CleanStoreName(e.Name, city)
		e.URL = cleanUberEatsURL(e.URL)
		switch {
		case e.Name == "":
			problems = append(problems, fmt.Sprintf("restaurant n° %d : nom manquant", i+1))
			continue
		case e.URL == "":
			problems = append(problems, fmt.Sprintf("restaurant n° %d (%s) : lien Uber Eats invalide", i+1, e.Name))
			continue
		case seen[menusync.NormURL(e.URL)]:
			problems = append(problems, fmt.Sprintf("restaurant n° %d (%s) : lien Uber Eats en double", i+1, e.Name))
			continue
		}
		seen[menusync.NormURL(e.URL)] = true
		e.Rating = min(max(e.Rating, 0), 5)
		e.RatingCount = max(e.RatingCount, 0)
		e.EtaMin = max(e.EtaMin, 0)
		if e.Lat < -90 || e.Lat > 90 || e.Lng < -180 || e.Lng > 180 {
			e.Lat, e.Lng = 0, 0
		}
		e.Address = menusync.CleanText(e.Address)
		e.Promo = menusync.CleanText(e.Promo)
		if _, err := time.Parse(time.DateOnly, e.CheckedAt); err != nil {
			e.CheckedAt = ""
		}
		var items []SnapshotItem
		for _, it := range e.Items {
			it.Name = menusync.CleanText(it.Name)
			if it.Name == "" || it.Price <= 0 || slices.ContainsFunc(items, func(o SnapshotItem) bool { return menusync.Fold(o.Name) == menusync.Fold(it.Name) }) {
				continue
			}
			items = append(items, it)
		}
		e.Items = items
		entries = append(entries, e)
	}
	return entries, problems, nil
}

// ReadSnapshot reads the snapshot for a source row (no network).
func ReadSnapshot(s Source, data []byte, logf func(string, ...any)) ([]SnapshotEntry, SourceResult) {
	start := time.Now()
	res := SourceResult{ID: s.ID, Label: s.Label, Provider: s.Provider, URL: s.URL}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	logf("== %s (%s)", s.Label, s.Provider)
	city := cmp.Or(strings.ToLower(strings.TrimSpace(s.City)), "mons")
	entries, problems, err := ParseSnapshot(data, city)
	for _, p := range problems {
		logf("  ignoré : %s", p)
	}
	res.Restaurants = len(entries)
	switch {
	case err != nil:
		res.Status, res.Message = StatusFailed, err.Error()
	case len(entries) == 0:
		// an empty file is a normal state (nothing collected yet)
		res.Status, res.Message = StatusOK, "fichier vide (0 restaurant)"
	default:
		res.Status, res.Message = StatusOK, fmt.Sprintf("%d restaurant", len(entries))
		if len(entries) > 1 {
			res.Message += "s"
		}
		res.Message += " (instantané, sans requête)"
	}
	res.DurationMs = time.Since(start).Milliseconds()
	logf("== %s : %s — %s", s.Label, res.Status, res.Message)
	return entries, res
}

// SplitSources separates the network sources from the snapshot ones.
func SplitSources(srcs []Source) (network, snapshot []Source) {
	for _, s := range srcs {
		if s.Provider == ProviderUberEatsSnapshot {
			snapshot = append(snapshot, s)
		} else {
			network = append(network, s)
		}
	}
	return network, snapshot
}

// ---------------------------------------------------------------- emoji

var emojiRules = []struct {
	re    *regexp.Regexp
	emoji string
}{
	{regexp.MustCompile(`pizz`), "🍕"},
	{regexp.MustCompile(`burger|smash`), "🍔"},
	{regexp.MustCompile(`sushi|japon|maki`), "🍣"},
	{regexp.MustCompile(`ramen|nouille|noodle|\bpho\b|thai|vietnam`), "🍜"},
	{regexp.MustCompile(`kebab|pitta|pita|grec|libanais|durum|shawarma|turc|falafel`), "🥙"},
	{regexp.MustCompile(`tacos|mexic|burrito`), "🌮"},
	{regexp.MustCompile(`indien|curry`), "🍛"},
	{regexp.MustCompile(`chinois|wok|asiat|dim sum`), "🥡"},
	{regexp.MustCompile(`poulet|chicken|wings`), "🍗"},
	{regexp.MustCompile(`frite|friterie`), "🍟"},
	{regexp.MustCompile(`poke|salade|healthy|bowl|vegan|vegetarien`), "🥗"},
	{regexp.MustCompile(`sandwich|bagel|panini|snack`), "🥪"},
	{regexp.MustCompile(`pates|pasta|italien`), "🍝"},
	{regexp.MustCompile(`bubble|boba`), "🧋"},
	{regexp.MustCompile(`boulang|patiss|viennois|petit-dejeuner|brunch`), "🥐"},
	{regexp.MustCompile(`glace|dessert|gaufre|crepe`), "🍨"},
	{regexp.MustCompile(`cafe|coffee`), "☕"},
	{regexp.MustCompile(`grill|bbq|steak|viande`), "🥩"},
}

// GuessEmoji picks a visual for a restaurant from its cuisines, then its
// name (🍽️ by default).
func GuessEmoji(cuisines []string, name string) string {
	for _, text := range append(slices.Clone(cuisines), name) {
		t := menusync.Fold(text)
		for _, r := range emojiRules {
			if r.re.MatchString(t) {
				return r.emoji
			}
		}
	}
	return "🍽️"
}

// ---------------------------------------------------------------- reconciliation

// asFeed converts an entry into a feed record (used to create or refresh the
// partial restaurants owned by the snapshot). Approximate coordinates are
// left out: they never replace stored ones.
func (e SnapshotEntry) asFeed() menusync.Restaurant {
	m := menusync.Restaurant{Source: ProviderUberEatsSnapshot, SourceURLs: []string{e.URL}}
	m.Name, m.Address, m.Rating, m.RatingCount = e.Name, e.Address, e.Rating, e.RatingCount
	m.Cuisines = menusync.NormalizeCuisines(e.Categories)
	if e.EtaMin > 0 {
		m.EtaMin, m.EtaMax = e.EtaMin, e.EtaMin+SnapshotEtaSpread
	}
	if e.HasGeo() {
		m.Lat, m.Lng = e.Lat, e.Lng
	}
	m.Providers = []providers.Link{{ID: providers.UberEats, URL: e.URL}}
	m.Origins = []menusync.Origin{{Source: ProviderUberEatsSnapshot, URL: e.URL, CheckedAt: e.CheckedAt}}
	if len(e.Items) > 0 {
		c := catalog.CategoryImport{Name: PreviewCategory}
		for _, it := range e.Items {
			c.Items = append(c.Items, catalog.ItemImport{Name: it.Name, Price: it.Price})
		}
		m.Categories = []catalog.CategoryImport{c}
	}
	return m
}

// candidate is a restaurant the snapshot can match: a stored one (with its
// planned state when a feed already updated it) or one created by a feed in
// this run.
type candidate struct {
	stored *Restaurant     // nil = created in this run
	plan   *RestaurantPlan // the plan already writing it (nil = none)
}

func (c candidate) state() *Restaurant {
	if c.plan != nil {
		return c.plan.Restaurant
	}
	return c.stored
}

func ubereatsURL(r *Restaurant) string {
	for _, l := range r.Providers {
		if l.ID == providers.UberEats && l.URL != "" {
			return l.URL
		}
	}
	return ""
}

func hasGeo(r *Restaurant) bool { return !r.GeoApprox && (r.Lat != 0 || r.Lng != 0) }

// snapshotPlaceOK applies the geo check of a name match: only when both
// sides have real coordinates, within SameNameMaxKm.
func snapshotPlaceOK(r *Restaurant, lat, lng float64, geo bool) (bool, float64) {
	if !geo || !hasGeo(r) {
		return true, 0
	}
	d := domain.HaversineKm(r.Lat, r.Lng, lat, lng)
	return d <= menusync.SameNameMaxKm, d
}

// matchSnapshot matches every entry to a candidate: Uber Eats link (or
// snapshot key) first for all entries, then loose name + place.
func matchSnapshot(cands []candidate, entries []SnapshotEntry, city string) []int {
	matches := make([]int, len(entries))
	taken := make([]bool, len(cands))
	for i := range matches {
		matches[i] = -1
	}
	for i, e := range entries {
		key := menusync.NormURL(e.URL)
		for ci, c := range cands {
			r := c.state()
			if taken[ci] {
				continue
			}
			if u := ubereatsURL(r); (u != "" && menusync.NormURL(u) == key) || r.SourceKey == e.Key() {
				matches[i], taken[ci] = ci, true
				break
			}
		}
	}
	for i, e := range entries {
		if matches[i] >= 0 {
			continue
		}
		best, bestScore, bestD := -1, 0, 0.0
		for ci, c := range cands {
			if taken[ci] {
				continue
			}
			r := c.state()
			// a restaurant already bound to another Uber Eats store is not this one
			if u := ubereatsURL(r); u != "" && menusync.NormURL(u) != menusync.NormURL(e.URL) {
				continue
			}
			score := menusync.NameMatch(r.Name, e.Name, city)
			if score == menusync.NameNoMatch {
				continue
			}
			ok, d := snapshotPlaceOK(r, e.Lat, e.Lng, e.HasGeo())
			if !ok {
				continue
			}
			if best < 0 || score > bestScore || (score == bestScore && d < bestD) {
				best, bestScore, bestD = ci, score, d
			}
		}
		if best >= 0 {
			matches[i], taken[best] = best, true
		}
	}
	return matches
}

// mergeInto folds q (computed from p's planned state) into p.
func (p *RestaurantPlan) mergeInto(q *RestaurantPlan) {
	p.Restaurant = q.Restaurant
	p.Save = p.Save || q.Save
	p.NewCategories = append(p.NewCategories, q.NewCategories...)
	p.Items = append(p.Items, q.Items...)
	p.Changes = append(p.Changes, q.Changes...)
	st := q.Stats
	if p.Stats.RestaurantsUpdated > 0 || p.Stats.RestaurantsCreated > 0 {
		st.RestaurantsUpdated = 0
	}
	p.Stats.Add(st)
}

// reconcileSnapshot applies the snapshot after the feeds (see Reconcile).
// It returns the ids of the stored restaurants the snapshot lists (never
// marked stale).
func reconcileSnapshot(plan *Plan, existing []*Restaurant, o Options, slugs map[string]bool) map[string]bool {
	seen := map[string]bool{}
	if len(o.Snapshot) == 0 {
		return seen
	}
	byStored := map[string]*RestaurantPlan{}
	var cands []candidate
	for _, p := range plan.Restaurants {
		if p.Restaurant.ID != "" {
			byStored[p.Restaurant.ID] = p
		}
	}
	for _, r := range existing {
		cands = append(cands, candidate{stored: r, plan: byStored[r.ID]})
	}
	for _, p := range plan.Restaurants {
		if p.Restaurant.ID == "" {
			cands = append(cands, candidate{plan: p})
		}
	}

	matches := matchSnapshot(cands, o.Snapshot, o.City)
	for i, e := range o.Snapshot {
		ci := matches[i]
		if ci < 0 {
			plan.Restaurants = append(plan.Restaurants, createFromSnapshot(e, o, slugs))
			continue
		}
		c := cands[ci]
		cur := c.state()
		plan.SnapshotMatched++
		if c.stored != nil {
			seen[c.stored.ID] = true
		}
		if cur.Name != e.Name {
			plan.Log = append(plan.Log, fmt.Sprintf("≈ Uber Eats : « %s » reconnu comme « %s »", e.Name, cur.Name))
		}
		if cur.Locked {
			plan.Locked++
			plan.Log = append(plan.Log, fmt.Sprintf("= %s : verrouillé, ignoré (Uber Eats)", cur.Name))
			continue
		}
		var q *RestaurantPlan
		if cur.PartialMenu && c.stored != nil {
			// the snapshot owns its menu: refresh the preview and the live fields
			full := *cur
			full.Categories, full.Items = c.stored.Categories, c.stored.Items
			fe := e.asFeed()
			fe.Clean(o.City)
			q = updateRestaurant(&full, fe, o)
		} else {
			q = fillFromSnapshot(cur, e)
		}
		switch {
		case c.plan != nil:
			c.plan.mergeInto(q)
		case !q.Empty():
			plan.Restaurants = append(plan.Restaurants, q)
		}
	}
	return seen
}

// fillFromSnapshot completes a restaurant whose menu comes from elsewhere:
// Uber Eats link added, note / number of reviews / delay filled when
// missing. The menu is never touched.
func fillFromSnapshot(cur *Restaurant, e SnapshotEntry) *RestaurantPlan {
	r := cur.clone()
	p := &RestaurantPlan{Restaurant: r}
	var what []string
	if ubereatsURL(r) == "" {
		r.Providers = append(r.Providers, providers.Link{ID: providers.UberEats, URL: e.URL})
		what = append(what, "lien Uber Eats")
	}
	if r.Rating == 0 && e.Rating > 0 {
		r.Rating = e.Rating
		what = append(what, "note")
	}
	if r.RatingCount == 0 && e.RatingCount > 0 {
		r.RatingCount = e.RatingCount
		if !slices.Contains(what, "note") {
			what = append(what, "nombre d'avis")
		}
	}
	if r.EtaMin == 0 && e.EtaMin > 0 {
		r.EtaMin = e.EtaMin
		r.EtaMax = max(r.EtaMax, e.EtaMin+SnapshotEtaSpread)
		what = append(what, "délai")
	}
	if len(what) > 0 {
		p.Save = true
		p.Stats.RestaurantsUpdated = 1
		p.Changes = append(p.Changes, fmt.Sprintf("%s : complété par Uber Eats (%s)", r.Name, strings.Join(what, ", ")))
	}
	return p
}

// createFromSnapshot creates an active restaurant with a partial menu (the
// sample items in the « Aperçu » category).
func createFromSnapshot(e SnapshotEntry, o Options, slugs map[string]bool) *RestaurantPlan {
	fe := e.asFeed()
	fe.Clean(o.City)
	p := createRestaurant(fe, o, slugs)
	r := p.Restaurant
	r.PartialMenu = true
	r.PriceLevel = 2
	r.Emoji = GuessEmoji(r.Cuisines, r.Name)
	r.Lat, r.Lng, r.GeoApprox = e.Lat, e.Lng, e.GeoApprox
	if e.Lat == 0 && e.Lng == 0 {
		r.Lat, r.Lng = o.defaultLocation()
		r.GeoApprox = true
	}
	p.Changes = []string{fmt.Sprintf("%s : nouveau restaurant Uber Eats, carte partielle (%d plats en aperçu)", r.Name, len(p.Items))}
	return p
}
