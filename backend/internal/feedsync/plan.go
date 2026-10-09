// Package feedsync reconciles the restaurant feeds read by menusync with the
// catalogue stored in the database. The reconciliation itself (Reconcile) is
// pure: it takes a snapshot of the stored restaurants and the merged feed,
// and returns the records to write plus statistics and human readable
// changes. The PocketBase glue (loading the snapshot, transactions,
// scheduling) lives in internal/app.
//
// Rules (docs/ARCHITECTURE.md « Synchronisation automatique ») :
//   - a feed restaurant is matched to a stored one by source key, then by a
//     platform link / source page, then by name + location (menusync.SameRestaurant);
//   - locked records (edited by an admin) are never modified;
//   - curated fields are preserved: a non-empty value is never replaced by an
//     empty one, and address / coordinates / phone / description / emoji /
//     name / cuisines already set are kept;
//   - items missing from the feed become unavailable (never deleted) and come
//     back when the feed lists them again;
//   - a restaurant no source returns anymore is marked stale (never deleted);
//   - the Uber Eats snapshot (snapshot.go) is applied after the feeds: it adds
//     links / fills missing notes, or creates restaurants with a partial menu
//     that a feed supplying a full menu (≥ FullMenuMinItems items) replaces.
package feedsync

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// SourceRef is one feed a record comes from (restaurants.sources / menu_items.sources).
type SourceRef struct {
	Provider  string `json:"provider"`
	URL       string `json:"url"`
	CheckedAt string `json:"checked_at"`
}

// Restaurant is a stored restaurant with its menu.
type Restaurant struct {
	ID          string
	SourceKey   string
	Slug        string
	Name        string
	Description string
	Emoji       string
	CoverURL    string
	Address     string
	Phone       string
	Cuisines    []string
	Lat, Lng    float64
	Rating      float64
	RatingCount int
	PriceLevel  int
	EtaMin      int
	EtaMax      int
	DeliveryFee int
	MinOrder    int
	Providers   []providers.Link
	Sources     []SourceRef
	Locked      bool
	Active      bool
	StaleSince  string // "" = not stale
	// PartialMenu: only a few sample items are known (Uber Eats snapshot).
	PartialMenu bool
	// GeoApprox: Lat / Lng are approximate (default office location).
	GeoApprox bool

	Categories []*Category
	Items      []*Item
}

// Category is a stored menu category.
type Category struct {
	ID       string // "" = to create
	Name     string
	Position int
}

// Item is a stored menu item.
type Item struct {
	ID           string // "" = to create
	SourceKey    string
	CategoryID   string
	CategoryName string // set when the category is created by the plan
	Name         string
	Description  string
	Price        int
	OptionGroups []domain.OptionGroup
	Popular      bool
	Available    bool
	Locked       bool
	Position     int
	Sources      []SourceRef
}

// Stats are the counters of a run (sync_runs.stats).
type Stats struct {
	RestaurantsCreated int `json:"restaurants_created"`
	RestaurantsUpdated int `json:"restaurants_updated"`
	RestaurantsStale   int `json:"restaurants_stale"`
	ItemsCreated       int `json:"items_created"`
	ItemsUpdated       int `json:"items_updated"`
	ItemsPriceChanged  int `json:"items_price_changed"`
	ItemsUnavailable   int `json:"items_unavailable"`
	// RestaurantsEnriched: restaurants completed from OpenStreetMap
	// (phone / address / position, internal/enrich) at the end of the run.
	RestaurantsEnriched int `json:"restaurants_enriched"`
}

// Add sums two stats.
func (s *Stats) Add(o Stats) {
	s.RestaurantsCreated += o.RestaurantsCreated
	s.RestaurantsUpdated += o.RestaurantsUpdated
	s.RestaurantsStale += o.RestaurantsStale
	s.ItemsCreated += o.ItemsCreated
	s.ItemsUpdated += o.ItemsUpdated
	s.ItemsPriceChanged += o.ItemsPriceChanged
	s.ItemsUnavailable += o.ItemsUnavailable
	s.RestaurantsEnriched += o.RestaurantsEnriched
}

// RestaurantPlan is what to write for one restaurant (one transaction).
type RestaurantPlan struct {
	// Restaurant is the desired state; ID "" means create.
	Restaurant *Restaurant
	// Save is true when the restaurant record itself must be written.
	Save bool
	// NewCategories are created first; Items refer to them by CategoryName.
	NewCategories []*Category
	// Items to write (ID "" = create).
	Items []*Item
	// Stats and Changes of this restaurant (applied only if the write succeeds).
	Stats   Stats
	Changes []string
}

// Empty reports whether the plan writes nothing.
func (p *RestaurantPlan) Empty() bool {
	return !p.Save && len(p.NewCategories) == 0 && len(p.Items) == 0
}

// Plan is the result of Reconcile.
type Plan struct {
	Restaurants []*RestaurantPlan
	// Matched counts feed restaurants matched to a stored one.
	Matched int
	// Locked counts feed restaurants matched to a locked one (skipped).
	Locked int
	// SnapshotMatched counts snapshot restaurants matched to a known one.
	SnapshotMatched int
	Log             []string
}

// Options tune Reconcile.
type Options struct {
	Now  time.Time
	City string
	// Incomplete holds the providers whose fetch failed or was blocked in
	// this run: their absent restaurants are not marked stale.
	Incomplete map[string]bool
	// Snapshot is the Uber Eats snapshot read in this run (nil = none).
	Snapshot []SnapshotEntry
	// DefaultLat / DefaultLng locate the snapshot restaurants without
	// coordinates (OCC_DEFAULT_LAT / LNG; the city centre when 0).
	DefaultLat, DefaultLng float64
	// Scoped marks a run limited to some sources (one source run from the
	// admin): nothing is marked stale, the provenance of a matched
	// restaurant is extended (never replaced), and a restaurant whose
	// stored sources include a provider ranked before the feed one (in
	// Priority) keeps its menu.
	Scoped   bool
	Priority []string
}

// outranked reports whether a stored restaurant has a source preferred to
// provider in a scoped run.
func (o Options) outranked(cur *Restaurant, provider string) bool {
	if !o.Scoped || len(o.Priority) == 0 {
		return false
	}
	rank := func(p string) int {
		if i := slices.Index(o.Priority, p); i >= 0 {
			return i
		}
		return len(o.Priority)
	}
	return slices.ContainsFunc(cur.Sources, func(s SourceRef) bool { return rank(s.Provider) < rank(provider) })
}

func (o Options) today() string { return o.Now.Format(time.DateOnly) }

func (o Options) defaultLocation() (float64, float64) {
	if o.DefaultLat != 0 || o.DefaultLng != 0 {
		return o.DefaultLat, o.DefaultLng
	}
	c, ok := menusync.Cities[o.City]
	if !ok {
		c = menusync.Cities["mons"]
	}
	return c.Lat, c.Lng
}

// Reconcile computes the writes that bring the stored catalogue in line with
// the merged feed. existing is not modified.
func Reconcile(existing []*Restaurant, feed []menusync.Restaurant, o Options) Plan {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	var plan Plan
	claimed := map[string]bool{} // stored restaurant id → matched
	slugs := map[string]bool{}
	for _, r := range existing {
		slugs[r.Slug] = true
	}

	feed = slices.Clone(feed)
	for i := range feed {
		feed[i].Clean(o.City)
	}
	// two passes: stable source keys first, so that a fuzzy match never
	// steals the restaurant another feed record is bound to
	matches := make([]*Restaurant, len(feed))
	for i, in := range feed {
		if r := matchByKey(existing, in, claimed); r != nil {
			matches[i], claimed[r.ID] = r, true
		}
	}
	for i, in := range feed {
		if matches[i] == nil && in.Name != "" {
			if r := matchByPlace(existing, in, claimed, o.City); r != nil {
				matches[i], claimed[r.ID] = r, true
			}
		}
	}

	for i, in := range feed {
		if in.Name == "" {
			continue
		}
		if cur := matches[i]; cur != nil {
			plan.Matched++
			if cur.Locked {
				plan.Locked++
				plan.Log = append(plan.Log, fmt.Sprintf("= %s : verrouillé, ignoré", cur.Name))
				continue
			}
			if p := updateRestaurant(cur, in, o); !p.Empty() {
				plan.Restaurants = append(plan.Restaurants, p)
			}
			continue
		}
		if in.ItemCount() == 0 {
			continue
		}
		plan.Restaurants = append(plan.Restaurants, createRestaurant(in, o, slugs))
	}

	seen := reconcileSnapshot(&plan, existing, o, slugs)

	for _, r := range existing {
		if o.Scoped {
			break // a partial run cannot tell what disappeared
		}
		if claimed[r.ID] || seen[r.ID] || r.Locked || r.StaleSince != "" || len(r.Sources) == 0 {
			continue
		}
		if slices.ContainsFunc(r.Sources, func(s SourceRef) bool { return o.Incomplete[s.Provider] }) {
			continue
		}
		cp := r.clone()
		cp.StaleSince = o.Now.UTC().Format("2006-01-02 15:04:05.000Z")
		plan.Restaurants = append(plan.Restaurants, &RestaurantPlan{
			Restaurant: cp, Save: true,
			Stats:   Stats{RestaurantsStale: 1},
			Changes: []string{fmt.Sprintf("%s : plus proposé par aucune source (obsolète)", r.Name)},
		})
	}
	return plan
}

// ---------------------------------------------------------------- matching

func feedKeys(in menusync.Restaurant) []string {
	keys := []string{}
	if k := in.SourceKey(); k != "" {
		keys = append(keys, k)
	}
	for _, o := range in.Origins {
		if o.URL != "" {
			keys = append(keys, o.Source+":"+menusync.NormURL(o.URL))
		}
	}
	return keys
}

func (r *Restaurant) asFeed() menusync.Restaurant {
	m := menusync.Restaurant{}
	m.Name, m.Address, m.Providers = r.Name, r.Address, r.Providers
	if !r.GeoApprox {
		m.Lat, m.Lng = r.Lat, r.Lng
	}
	for _, s := range r.Sources {
		m.SourceURLs = append(m.SourceURLs, s.URL)
	}
	return m
}

// matchByKey finds the unclaimed stored restaurant bound to the same source key.
func matchByKey(existing []*Restaurant, in menusync.Restaurant, claimed map[string]bool) *Restaurant {
	keys := feedKeys(in)
	for _, r := range existing {
		if !claimed[r.ID] && r.SourceKey != "" && slices.Contains(keys, r.SourceKey) {
			return r
		}
	}
	return nil
}

// matchByPlace finds the unclaimed stored restaurant with the same platform
// link / source page, or the same name at the same place (nearest first).
// A partial-menu restaurant (Uber Eats snapshot, often without a real
// position) is also recognised by a loose name match (menusync.NameMatch).
func matchByPlace(existing []*Restaurant, in menusync.Restaurant, claimed map[string]bool, city string) *Restaurant {
	var best *Restaurant
	bestD := 1e9
	for _, r := range existing {
		if claimed[r.ID] {
			continue
		}
		if !menusync.SameRestaurant(r.asFeed(), in) {
			if !r.PartialMenu || menusync.NameMatch(r.Name, in.Name, city) == menusync.NameNoMatch {
				continue
			}
			if ok, _ := snapshotPlaceOK(r, in.Lat, in.Lng, in.Lat != 0 || in.Lng != 0); !ok {
				continue
			}
		}
		d := 0.0
		if hasGeo(r) && (in.Lat != 0 || in.Lng != 0) {
			d = domain.HaversineKm(r.Lat, r.Lng, in.Lat, in.Lng)
		}
		if best == nil || d < bestD {
			best, bestD = r, d
		}
	}
	return best
}

// ---------------------------------------------------------------- creation

func origins(in menusync.Restaurant, o Options) []SourceRef {
	out := []SourceRef{}
	for _, x := range in.Origins {
		if x.URL == "" || x.Source == menusync.SourceExisting {
			continue
		}
		out = append(out, SourceRef{Provider: x.Source, URL: x.URL, CheckedAt: cmp.Or(x.CheckedAt, o.today())})
	}
	return out
}

func menuSources(in menusync.Restaurant, all []SourceRef) []SourceRef {
	for _, s := range all {
		if s.Provider == in.Source {
			return []SourceRef{s}
		}
	}
	if len(all) > 0 {
		return all[:1]
	}
	return []SourceRef{}
}

func uniqueSlug(base string, used map[string]bool) string {
	if base == "" {
		base = "restaurant"
	}
	s := base
	for n := 2; used[s]; n++ {
		s = fmt.Sprintf("%s-%d", base, n)
	}
	used[s] = true
	return s
}

func createRestaurant(in menusync.Restaurant, o Options, slugs map[string]bool) *RestaurantPlan {
	srcs := origins(in, o)
	r := &Restaurant{
		SourceKey: in.SourceKey(), Slug: uniqueSlug(menusync.Slugify(in.Name), slugs), Name: in.Name,
		Description: in.Description, Emoji: in.Emoji, CoverURL: in.CoverURL, Address: in.Address, Phone: in.Phone,
		Cuisines: menusync.NormalizeCuisines(in.Cuisines), Lat: in.Lat, Lng: in.Lng, Rating: in.Rating,
		RatingCount: in.RatingCount, PriceLevel: cmp.Or(in.PriceLevel, 2), EtaMin: in.EtaMin, EtaMax: in.EtaMax,
		DeliveryFee: in.DeliveryFee, MinOrder: in.MinOrder, Providers: cleanLinks(in.Providers), Sources: srcs, Active: true,
	}
	if r.EtaMin > r.EtaMax {
		r.EtaMax = r.EtaMin
	}
	p := &RestaurantPlan{Restaurant: r, Save: true, Stats: Stats{RestaurantsCreated: 1}}
	isrc := menuSources(in, srcs)
	for _, fi := range feedItems(in) {
		if !slices.ContainsFunc(p.NewCategories, func(c *Category) bool { return c.Name == fi.category }) {
			p.NewCategories = append(p.NewCategories, &Category{Name: fi.category, Position: len(p.NewCategories)})
		}
		it := &Item{SourceKey: fi.key, CategoryName: fi.category, Position: fi.position, Sources: isrc}
		applyFeedItem(it, fi.item)
		p.Items = append(p.Items, it)
		p.Stats.ItemsCreated++
	}
	p.Changes = append(p.Changes, fmt.Sprintf("%s : nouveau restaurant (%d plats)", r.Name, len(p.Items)))
	return p
}

func cleanLinks(in []providers.Link) []providers.Link {
	out := []providers.Link{}
	for _, l := range in {
		if l.URL != "" && providers.IsPlatform(l.ID) && !slices.ContainsFunc(out, func(x providers.Link) bool { return x.ID == l.ID }) {
			out = append(out, l)
		}
	}
	return out
}

// ---------------------------------------------------------------- menu items

type feedItem struct {
	key      string
	category string
	position int
	item     catalog.ItemImport
}

// ItemKey is the stable id of a menu item inside its restaurant.
func ItemKey(name string) string { return menusync.Slugify(name) }

// feedItems flattens the feed menu with stable, unique item keys: the name
// slug, suffixed by the category (then a counter) when a name repeats.
func feedItems(in menusync.Restaurant) []feedItem {
	var out []feedItem
	count := map[string]int{}
	for _, c := range in.Categories {
		for _, it := range c.Items {
			count[ItemKey(it.Name)]++
		}
	}
	used := map[string]bool{}
	for _, c := range in.Categories {
		for i, it := range c.Items {
			key := ItemKey(it.Name)
			if count[key] > 1 {
				key += "--" + menusync.Slugify(c.Name)
			}
			base := key
			for n := 2; used[key]; n++ {
				key = fmt.Sprintf("%s-%d", base, n)
			}
			used[key] = true
			out = append(out, feedItem{key: key, category: c.Name, position: i, item: it})
		}
	}
	return out
}

func validGroups(gs []domain.OptionGroup) []domain.OptionGroup {
	if len(gs) == 0 || domain.ValidateOptionGroups(gs) != nil {
		return []domain.OptionGroup{}
	}
	return gs
}

func applyFeedItem(it *Item, in catalog.ItemImport) {
	it.Name = in.Name
	it.Description = in.Description
	it.Price = in.Price
	it.OptionGroups = validGroups(in.OptionGroups)
	it.Popular = in.Popular
	it.Available = in.Available == nil || *in.Available
}

func groupsJSON(gs []domain.OptionGroup) string {
	if len(gs) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(gs)
	return string(b)
}

// FormatEuros formats cents the French way: 1450 → "14,50 €".
func FormatEuros(cents int) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s%d,%02d €", sign, cents/100, cents%100)
}

// ---------------------------------------------------------------- update

func (r *Restaurant) clone() *Restaurant {
	cp := *r
	cp.Cuisines = slices.Clone(r.Cuisines)
	cp.Providers = slices.Clone(r.Providers)
	cp.Sources = slices.Clone(r.Sources)
	cp.Categories = nil
	cp.Items = nil
	return &cp
}

func (it *Item) clone() *Item {
	cp := *it
	cp.OptionGroups = slices.Clone(it.OptionGroups)
	cp.Sources = slices.Clone(it.Sources)
	return &cp
}

func fill(dst *string, v string) bool {
	if strings.TrimSpace(*dst) == "" && strings.TrimSpace(v) != "" {
		*dst = strings.TrimSpace(v)
		return true
	}
	return false
}

func setPositive(dst *int, v int) bool {
	if v > 0 && *dst != v {
		*dst = v
		return true
	}
	return false
}

func updateRestaurant(cur *Restaurant, in menusync.Restaurant, o Options) *RestaurantPlan {
	r := cur.clone()
	p := &RestaurantPlan{Restaurant: r}
	var what []string
	note := func(changed bool, label string) {
		if changed && !slices.Contains(what, label) {
			what = append(what, label)
		}
	}

	// a partial menu (Uber Eats snapshot) is replaced by the first feed
	// supplying a full one; a smaller feed menu leaves it alone
	fromSnapshot := in.Source == ProviderUberEatsSnapshot
	takeover := cur.PartialMenu && !fromSnapshot && in.ItemCount() >= FullMenuMinItems
	keepMenu := cur.PartialMenu && !fromSnapshot && !takeover
	if !fromSnapshot && o.outranked(cur, in.Source) {
		// scoped run of a less preferred source: the menu stays
		takeover, keepMenu = false, true
	}

	// provenance: always refreshed (checked_at), not counted as a change
	srcs := origins(in, o)
	if keepMenu || fromSnapshot || o.Scoped {
		// the menu still comes from the snapshot: snapshot key, the refs of
		// both the snapshot and the feeds listing the restaurant
		if k := in.SourceKey(); (fromSnapshot || (o.Scoped && !keepMenu)) && k != "" && r.SourceKey != k {
			r.SourceKey = k
			p.Save = true
		}
		if u := unionSources(r.Sources, srcs); !sourcesEqual(r.Sources, u) {
			r.Sources = u
			p.Save = true
		}
	} else {
		if k := in.SourceKey(); k != "" && r.SourceKey != k {
			r.SourceKey = k
			p.Save = true
		}
		if !sourcesEqual(r.Sources, srcs) && len(srcs) > 0 {
			r.Sources = srcs
			p.Save = true
		}
	}
	if takeover {
		r.PartialMenu = false
		p.Save = true
		p.Changes = append(p.Changes, fmt.Sprintf("%s : carte complète reçue (%d plats), fin de l'aperçu Uber Eats", r.Name, in.ItemCount()))
		note(true, "carte complète")
	}
	if r.StaleSince != "" {
		r.StaleSince = ""
		p.Save = true
		p.Changes = append(p.Changes, fmt.Sprintf("%s : de nouveau proposé par une source", r.Name))
		note(true, "réapparu")
	}

	// a name truncated by an older cleaning rule is repaired from the source
	if menusync.TruncatedName(r.Name, in.Name) {
		p.Changes = append(p.Changes, fmt.Sprintf("%s : nom corrigé en « %s »", r.Name, in.Name))
		r.Name = in.Name
		note(true, "nom")
	}

	// curated fields: only filled when empty
	note(fill(&r.Description, in.Description), "description")
	note(fill(&r.Emoji, in.Emoji), "emoji")
	note(fill(&r.CoverURL, in.CoverURL), "image")
	note(fill(&r.Address, in.Address), "adresse")
	note(fill(&r.Phone, in.Phone), "téléphone")
	if (r.GeoApprox || (r.Lat == 0 && r.Lng == 0)) && (in.Lat != 0 || in.Lng != 0) {
		// a real position replaces an approximate one
		if r.GeoApprox && strings.TrimSpace(in.Address) != "" {
			r.Address = strings.TrimSpace(in.Address)
		}
		r.Lat, r.Lng, r.GeoApprox = in.Lat, in.Lng, false
		note(true, "coordonnées")
	}
	if len(r.Cuisines) == 0 && len(in.Cuisines) > 0 {
		r.Cuisines = menusync.NormalizeCuisines(in.Cuisines)
		note(len(r.Cuisines) > 0, "cuisines")
	}
	for _, l := range cleanLinks(in.Providers) {
		if !slices.ContainsFunc(r.Providers, func(x providers.Link) bool { return x.ID == l.ID }) {
			r.Providers = append(r.Providers, l)
			note(true, "liens")
		}
	}
	// live fields: follow the feed when it has a value — except the snapshot
	// when a feed also lists the restaurant (the feed wins, the snapshot only
	// fills what is missing)
	if fromSnapshot && slices.ContainsFunc(r.Sources, func(s SourceRef) bool { return s.Provider != ProviderUberEatsSnapshot }) {
		if r.Rating == 0 && in.Rating > 0 {
			r.Rating, r.RatingCount = in.Rating, max(r.RatingCount, in.RatingCount)
			note(true, "note")
		}
		if r.EtaMin == 0 && in.EtaMin > 0 {
			r.EtaMin, r.EtaMax = in.EtaMin, max(r.EtaMax, in.EtaMax)
			note(true, "délai")
		}
	} else {
		if in.Rating > 0 && (in.Rating != r.Rating || (in.RatingCount > 0 && in.RatingCount != r.RatingCount)) {
			r.Rating = in.Rating
			if in.RatingCount > 0 {
				r.RatingCount = in.RatingCount
			}
			note(true, "note")
		}
		note(setPositive(&r.DeliveryFee, in.DeliveryFee), "frais de livraison")
		note(setPositive(&r.MinOrder, in.MinOrder), "minimum")
		if in.EtaMin > 0 && in.EtaMax >= in.EtaMin && (in.EtaMin != r.EtaMin || in.EtaMax != r.EtaMax) {
			r.EtaMin, r.EtaMax = in.EtaMin, in.EtaMax
			note(true, "délai")
		}
	}
	restChanged := len(what) > 0
	if restChanged {
		p.Save = true
		if len(p.Changes) == 0 {
			p.Changes = append(p.Changes, fmt.Sprintf("%s : infos mises à jour (%s)", r.Name, strings.Join(what, ", ")))
		}
	}

	itemsChanged := false
	if !keepMenu {
		itemsChanged = reconcileItems(cur, r, in, srcs, p)
	}
	if restChanged || itemsChanged {
		p.Stats.RestaurantsUpdated = 1
	}
	return p
}

func sourcesEqual(a, b []SourceRef) bool {
	return slices.Equal(a, b)
}

// unionSources returns a with the refs of b added or refreshed (same
// provider and URL).
func unionSources(a, b []SourceRef) []SourceRef {
	out := slices.Clone(a)
	for _, s := range b {
		i := slices.IndexFunc(out, func(x SourceRef) bool {
			return x.Provider == s.Provider && menusync.NormURL(x.URL) == menusync.NormURL(s.URL)
		})
		if i >= 0 {
			out[i] = s
		} else {
			out = append(out, s)
		}
	}
	return out
}

// reconcileItems plans the menu writes of a matched restaurant. It reports
// whether any item changed.
func reconcileItems(cur, r *Restaurant, in menusync.Restaurant, srcs []SourceRef, p *RestaurantPlan) bool {
	feed := feedItems(in)
	if len(feed) == 0 {
		return false // nothing readable: never empty a menu
	}
	isrc := menuSources(in, srcs)

	catByName := map[string]string{} // folded name → id
	catName := map[string]string{}   // id → name
	maxPos := -1
	for _, c := range cur.Categories {
		if _, ok := catByName[menusync.Fold(c.Name)]; !ok {
			catByName[menusync.Fold(c.Name)] = c.ID
		}
		catName[c.ID] = c.Name
		maxPos = max(maxPos, c.Position)
	}
	newCat := map[string]bool{}
	categoryFor := func(name string) (id, created string) {
		if id, ok := catByName[menusync.Fold(name)]; ok {
			return id, ""
		}
		if !newCat[menusync.Fold(name)] {
			newCat[menusync.Fold(name)] = true
			maxPos++
			p.NewCategories = append(p.NewCategories, &Category{Name: name, Position: maxPos})
		}
		return "", name
	}

	claimed := map[string]bool{}
	bySource := map[string]*Item{}
	for _, it := range cur.Items {
		if it.SourceKey != "" {
			bySource[it.SourceKey] = it
		}
	}
	findByName := func(fi feedItem) *Item {
		var best *Item
		for _, it := range cur.Items {
			if claimed[it.ID] || ItemKey(it.Name) != ItemKey(fi.item.Name) {
				continue
			}
			if it.SourceKey != "" && it.SourceKey != fi.key && bySource[it.SourceKey] == it && feedHasKey(feed, it.SourceKey) {
				continue // belongs to another feed item
			}
			if best == nil || menusync.Fold(catName[it.CategoryID]) == menusync.Fold(fi.category) {
				best = it
			}
		}
		return best
	}

	changed := false
	rname := r.Name
	// price and availability lines are always listed; new items and minor
	// updates are summed up per restaurant when there are many
	var newLines, minorLines []string
	defer func() {
		p.Changes = append(p.Changes, summarize(newLines, rname, "nouveaux plats")...)
		p.Changes = append(p.Changes, summarize(minorLines, rname, "autres plats mis à jour (description, options, catégorie…)")...)
	}()
	for _, fi := range feed {
		it := bySource[fi.key]
		if it != nil && claimed[it.ID] {
			it = nil
		}
		if it == nil {
			it = findByName(fi)
		}
		if it == nil {
			n := &Item{SourceKey: fi.key, Position: fi.position, Sources: isrc}
			n.CategoryID, n.CategoryName = categoryFor(fi.category)
			applyFeedItem(n, fi.item)
			p.Items = append(p.Items, n)
			p.Stats.ItemsCreated++
			newLines = append(newLines, fmt.Sprintf("%s — %s : nouveau plat (%s)", rname, n.Name, FormatEuros(n.Price)))
			changed = true
			continue
		}
		claimed[it.ID] = true
		if it.Locked {
			continue
		}
		nx := it.clone()
		var what []string
		save := false
		if nx.SourceKey != fi.key {
			nx.SourceKey, save = fi.key, true
		}
		if fi.item.Price != nx.Price {
			p.Changes = append(p.Changes, fmt.Sprintf("%s — %s : %s → %s", rname, nx.Name, FormatEuros(nx.Price), FormatEuros(fi.item.Price)))
			nx.Price = fi.item.Price
			p.Stats.ItemsPriceChanged++
			what = append(what, "prix")
		}
		if d := fi.item.Description; d != "" && d != nx.Description {
			nx.Description = d
			what = append(what, "description")
		}
		if gs := validGroups(fi.item.OptionGroups); len(gs) > 0 && groupsJSON(gs) != groupsJSON(nx.OptionGroups) {
			nx.OptionGroups = gs
			what = append(what, "options")
		}
		avail := fi.item.Available == nil || *fi.item.Available
		if avail != nx.Available {
			nx.Available = avail
			if avail {
				p.Changes = append(p.Changes, fmt.Sprintf("%s — %s : de nouveau disponible", rname, nx.Name))
			} else {
				p.Changes = append(p.Changes, fmt.Sprintf("%s — %s : indisponible", rname, nx.Name))
				p.Stats.ItemsUnavailable++
			}
			what = append(what, "disponibilité")
		}
		if fi.item.Popular != nx.Popular {
			nx.Popular = fi.item.Popular
			what = append(what, "populaire")
		}
		if menusync.Fold(catName[nx.CategoryID]) != menusync.Fold(fi.category) {
			nx.CategoryID, nx.CategoryName = categoryFor(fi.category)
			what = append(what, "catégorie")
		}
		if len(what) > 0 {
			nx.Sources = isrc
			p.Stats.ItemsUpdated++
			changed = true
			if !slices.Contains(what, "prix") && !slices.Contains(what, "disponibilité") {
				minorLines = append(minorLines, fmt.Sprintf("%s — %s : mis à jour (%s)", rname, nx.Name, strings.Join(what, ", ")))
			}
		}
		if save || len(what) > 0 {
			p.Items = append(p.Items, nx)
		}
	}

	// stored items the feed no longer lists → unavailable
	for _, it := range cur.Items {
		if claimed[it.ID] || it.Locked || !it.Available {
			continue
		}
		nx := it.clone()
		nx.Available = false
		p.Items = append(p.Items, nx)
		p.Stats.ItemsUnavailable++
		p.Stats.ItemsUpdated++
		p.Changes = append(p.Changes, fmt.Sprintf("%s — %s : retiré de la carte (indisponible)", rname, nx.Name))
		changed = true
	}
	return changed
}

// maxDetailedLines is the number of similar lines listed one by one per
// restaurant before they are summed up.
const maxDetailedLines = 10

func summarize(lines []string, restaurant, what string) []string {
	if len(lines) <= maxDetailedLines {
		return lines
	}
	return []string{fmt.Sprintf("%s : %d %s", restaurant, len(lines), what)}
}

func feedHasKey(feed []feedItem, key string) bool {
	return slices.ContainsFunc(feed, func(f feedItem) bool { return f.key == key })
}
