package menusync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// DeliverooBase is the Belgian site root.
const DeliverooBase = "https://deliveroo.be"

// DeliverooCityListing maps a -city value to its public listing page.
var DeliverooCityListing = map[string]string{
	"mons": DeliverooBase + "/fr/restaurants/brussels/mons-center?fulfillment_method=DELIVERY&geohash=u0fz40u6z12k",
}

var nextDataRe = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__" type="application/json"[^>]*>(.*?)</script>`)

// NextData extracts the Next.js __NEXT_DATA__ JSON of a page.
func NextData(page []byte) ([]byte, error) {
	m := nextDataRe.FindSubmatch(page)
	if m == nil {
		return nil, errors.New("__NEXT_DATA__ introuvable dans la page")
	}
	return m[1], nil
}

// ListingEntry is a restaurant card of a Deliveroo listing page.
type ListingEntry struct {
	ID         string
	Name       string
	Href       string  // "/menu/Brussels/mons-center/baalbeck?geohash=…"
	DistanceKm float64 // -1 when unknown
	// Base is the site root of the listing ("" = DeliverooBase).
	Base string
}

func (e ListingEntry) base() string {
	if e.Base != "" {
		return e.Base
	}
	return DeliverooBase
}

// Path returns the href without its query string.
func (e ListingEntry) Path() string {
	p, _, _ := strings.Cut(e.Href, "?")
	return p
}

// MenuURL is the page to fetch. Only the delivery geohash is kept from the
// card link (fees depend on it); scheduling params (day, time…) are dropped.
func (e ListingEntry) MenuURL() string {
	u := e.base() + "/fr" + e.Path()
	if _, q, ok := strings.Cut(e.Href, "?"); ok {
		if v, err := url.ParseQuery(q); err == nil && v.Get("geohash") != "" {
			u += "?geohash=" + url.QueryEscape(v.Get("geohash"))
		}
	}
	return u
}

// PageURL is the canonical public restaurant page (provider link).
func (e ListingEntry) PageURL() string { return e.base() + "/fr" + e.Path() }

var kmRe = regexp.MustCompile(`([\d.,]+)\s*km`)

// ParseDeliverooListing returns the unique restaurant cards of a listing page.
func ParseDeliverooListing(page []byte) ([]ListingEntry, error) {
	raw, err := NextData(page)
	if err != nil {
		return nil, err
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("__NEXT_DATA__ invalide : %w", err)
	}
	byPath := map[string]*ListingEntry{}
	var order []string
	add := func(params map[string]any, distance string) {
		href, _ := params["restaurant_href"].(string)
		if !strings.HasPrefix(href, "/menu/") {
			return
		}
		e := ListingEntry{Href: href, DistanceKm: -1}
		e.Name, _ = params["restaurant_name"].(string)
		e.ID, _ = params["restaurant_id"].(string)
		if m := kmRe.FindStringSubmatch(distance); m != nil {
			if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
				e.DistanceKm = v
			}
		}
		if prev, ok := byPath[e.Path()]; ok {
			if prev.DistanceKm < 0 {
				prev.DistanceKm = e.DistanceKm
			}
			return
		}
		byPath[e.Path()] = &e
		order = append(order, e.Path())
	}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if tap, ok := t["partner-card.on-tap"].(map[string]any); ok {
				if action, ok := tap["action"].(map[string]any); ok {
					if params, ok := action["parameters"].(map[string]any); ok {
						d, _ := t["distance-presentational.content"].(string)
						add(params, d)
					}
				}
			}
			if _, ok := t["restaurant_href"]; ok {
				add(t, "")
			}
			for _, c := range t {
				walk(c)
			}
		case []any:
			for _, c := range t {
				walk(c)
			}
		}
	}
	walk(root)
	out := make([]ListingEntry, 0, len(order))
	for _, p := range order {
		out = append(out, *byPath[p])
	}
	return out, nil
}

// --- menu page -----------------------------------------------------------

type drPrice struct {
	Fractional int `json:"fractional"`
}

type drSpan struct {
	Text string `json:"text"`
}

type drLine struct {
	Spans []drSpan `json:"spans"`
}

func (l drLine) texts() []string {
	var out []string
	for _, s := range l.Spans {
		if t := CleanText(s.Text); t != "" && t != "·" {
			out = append(out, t)
		}
	}
	return out
}

type drMenuPage struct {
	Props struct {
		InitialState struct {
			MenuPage struct {
				Menu struct {
					Metas struct {
						Root struct {
							Restaurant struct {
								ID           string `json:"id"`
								Name         string `json:"name"`
								MenuDisabled bool   `json:"menuDisabled"`
								Location     struct {
									Address struct {
										Address1 string `json:"address1"`
									} `json:"address"`
								} `json:"location"`
								Links struct {
									Self struct {
										Href string `json:"href"`
									} `json:"self"`
								} `json:"links"`
							} `json:"restaurant"`
							Metatags struct {
								Image             string `json:"image"`
								DescriptionSocial string `json:"descriptionSocial"`
							} `json:"metatags"`
							Categories []struct {
								ID   string `json:"id"`
								Name string `json:"name"`
							} `json:"categories"`
							Items []struct {
								ID               string   `json:"id"`
								CategoryID       string   `json:"categoryId"`
								Name             string   `json:"name"`
								Description      string   `json:"description"`
								Price            drPrice  `json:"price"`
								Available        bool     `json:"available"`
								Popular          bool     `json:"popular"`
								ModifierGroupIDs []string `json:"modifierGroupIds"`
							} `json:"items"`
							ModifierGroups []struct {
								ID           string `json:"id"`
								Name         string `json:"name"`
								MinSelection int    `json:"minSelection"`
								MaxSelection int    `json:"maxSelection"`
								Options      []struct {
									ID        string  `json:"id"`
									Name      string  `json:"name"`
									Price     drPrice `json:"price"`
									Available bool    `json:"available"`
								} `json:"modifierOptions"`
							} `json:"modifierGroups"`
						} `json:"root"`
					} `json:"metas"`
					Header struct {
						Title      string `json:"title"`
						HeaderTags struct {
							Lines []drLine `json:"lines"`
						} `json:"headerTags"`
						HeaderInfoRows []struct {
							Lines []drLine `json:"lines"`
						} `json:"headerInfoRows"`
					} `json:"header"`
					LayoutGroups []struct {
						Layouts []struct {
							Header any `json:"header"`
							Blocks []struct {
								Map *struct {
									Pins []struct {
										Lat float64 `json:"lat"`
										Lon float64 `json:"lon"`
									} `json:"pins"`
								} `json:"map"`
								Lines   []drLine `json:"lines"`
								Actions []struct {
									Lines []drLine `json:"lines"`
								} `json:"actions"`
							} `json:"blocks"`
						} `json:"layouts"`
					} `json:"layoutGroups"`
				} `json:"menu"`
			} `json:"menuPage"`
		} `json:"initialState"`
	} `json:"props"`
}

var (
	minOrderRe  = regexp.MustCompile(`(?i)montant min[^0-9]*([\d.,]+)`)
	feeRe       = regexp.MustCompile(`(?i)([\d.,]+)\s*€\s*de livraison`)
	freeFeeRe   = regexp.MustCompile(`(?i)livraison (offerte|gratuite)|frais de livraison offerts`)
	etaRangeRe  = regexp.MustCompile(`(\d+)\s*[-–]\s*(\d+)\s*min`)
	infoTagRe   = regexp.MustCompile(`(?i)\d\s*km\b|€|\bmin\b|livraison|ouvre|ferm|\d{1,2}:\d{2}`)
	ratingRe    = regexp.MustCompile(`^(\d(?:[.,]\d)?)\b`)
	ratingCntRe = regexp.MustCompile(`(\d[\d\s.]*)\+?\s*avis|\((\d+)\+?\)`)
	phoneRe     = regexp.MustCompile(`^\+?[\d\s./-]{8,}$`)
	brusselsRe  = regexp.MustCompile(`(?i),?\s*\bbrussels\b`)
	brusselsPC  = regexp.MustCompile(`\b1\d{3}\b`)
)

// ParseDeliverooMenu parses a restaurant menu page. pageURL is the public
// restaurant page used as provider link.
func ParseDeliverooMenu(page []byte, pageURL string) (Restaurant, error) {
	raw, err := NextData(page)
	if err != nil {
		return Restaurant{}, err
	}
	var p drMenuPage
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&p); err != nil {
		return Restaurant{}, fmt.Errorf("__NEXT_DATA__ invalide : %w", err)
	}
	menu := p.Props.InitialState.MenuPage.Menu
	meta := menu.Metas.Root
	name := CleanText(meta.Restaurant.Name)
	if name == "" {
		name = CleanText(menu.Header.Title)
	}
	if name == "" {
		return Restaurant{}, errors.New("page menu sans restaurant")
	}

	r := Restaurant{SourceURLs: []string{pageURL}}
	r.Slug = Slugify(name)
	r.Name = name
	r.CoverURL = meta.Metatags.Image
	r.Address = cleanDeliverooAddress(meta.Restaurant.Location.Address.Address1)
	r.Providers = []providers.Link{{ID: providers.Deliveroo, URL: pageURL}}

	// header tags: a cuisines line ("Libanais · Mezzes") and an info line
	// ("À 0.6 km · Ouvre à 17:30 · Montant min. de 5,00 € · 4,95 € de livraison");
	// the cuisines line is missing on some pages
	for _, line := range menu.Header.HeaderTags.Lines {
		texts := line.texts()
		if !slices.ContainsFunc(texts, infoTagRe.MatchString) {
			for _, t := range texts {
				r.Cuisines = appendUnique(r.Cuisines, strings.ToLower(t))
			}
			continue
		}
		for _, t := range texts {
			if m := minOrderRe.FindStringSubmatch(t); m != nil {
				r.MinOrder, _ = ParseEuroCents(m[1])
			}
			if m := feeRe.FindStringSubmatch(t); m != nil {
				r.DeliveryFee, _ = ParseEuroCents(m[1])
			}
			if freeFeeRe.MatchString(t) {
				r.DeliveryFee = 0
			}
			if m := etaRangeRe.FindStringSubmatch(t); m != nil {
				r.EtaMin, _ = strconv.Atoi(m[1])
				r.EtaMax, _ = strconv.Atoi(m[2])
			}
		}
	}
	// (the "environ 32 minutes" of the social description is the same
	// template value on every page: not used)

	// rating rows: "4.6 Excellent" then "(500+)" / "Voir 12 avis"
	for _, row := range menu.Header.HeaderInfoRows {
		var texts []string
		for _, l := range row.Lines {
			texts = append(texts, l.texts()...)
		}
		if len(texts) == 0 {
			continue
		}
		m := ratingRe.FindStringSubmatch(texts[0])
		if m == nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		if err != nil || v <= 0 || v > 5 {
			continue
		}
		r.Rating = v
		for _, t := range texts {
			if c := ratingCntRe.FindStringSubmatch(t); c != nil {
				s := c[1] + c[2]
				s = strings.NewReplacer(" ", "", ".", "").Replace(s)
				r.RatingCount, _ = strconv.Atoi(s)
			}
		}
		break
	}

	// "Lieu" (map pin + address) and "Coordonnées" (phone) blocks
	for _, g := range menu.LayoutGroups {
		for _, l := range g.Layouts {
			for _, b := range l.Blocks {
				if b.Map != nil && len(b.Map.Pins) > 0 && r.Lat == 0 {
					r.Lat, r.Lng = b.Map.Pins[0].Lat, b.Map.Pins[0].Lon
					for _, line := range b.Lines {
						if t := line.texts(); len(t) > 0 && r.Address == "" {
							r.Address = cleanDeliverooAddress(t[0])
						}
					}
				}
				if h, _ := l.Header.(string); h == "Coordonnées" && r.Phone == "" {
					lines := slices.Clone(b.Lines)
					for _, a := range b.Actions {
						lines = append(lines, a.Lines...)
					}
					for _, line := range lines {
						for _, t := range line.texts() {
							if phoneRe.MatchString(t) && r.Phone == "" {
								r.Phone = t
							}
						}
					}
				}
			}
		}
	}

	// option groups
	groups := map[string]domain.OptionGroup{}
	for _, g := range meta.ModifierGroups {
		og := domain.OptionGroup{ID: g.ID, Name: CleanText(g.Name), Min: g.MinSelection, Max: g.MaxSelection}
		seen := map[string]bool{}
		for _, o := range g.Options {
			if !o.Available || o.ID == "" || seen[o.ID] {
				continue
			}
			seen[o.ID] = true
			og.Choices = append(og.Choices, domain.OptionChoice{ID: o.ID, Name: CleanText(o.Name), Price: max(o.Price.Fractional, 0)})
		}
		groups[g.ID] = normalizeGroup(og)
	}

	cats := map[string]int{}
	for _, c := range meta.Categories {
		cats[c.ID] = len(r.Categories)
		r.Categories = append(r.Categories, catalog.CategoryImport{Name: CleanText(c.Name)})
	}
	for _, it := range meta.Items {
		ci, ok := cats[it.CategoryID]
		if !ok {
			continue // items only reachable as modifier options
		}
		item := catalog.ItemImport{
			Name:        CleanText(it.Name),
			Description: CleanText(it.Description),
			Price:       max(it.Price.Fractional, 0),
			Popular:     it.Popular,
		}
		if item.Name == "" {
			continue
		}
		if strings.EqualFold(item.Description, item.Name) {
			item.Description = ""
		}
		// a closed restaurant reports every item unavailable: only trust the
		// flag while the menu is open
		if !it.Available && !meta.Restaurant.MenuDisabled {
			f := false
			item.Available = &f
		}
		for _, gid := range it.ModifierGroupIDs {
			if g, ok := groups[gid]; ok && len(g.Choices) > 0 {
				item.OptionGroups = append(item.OptionGroups, g)
			}
		}
		r.Categories[ci].Items = append(r.Categories[ci].Items, item)
	}
	r.Categories = dropEmptyCategories(r.Categories)
	return r, nil
}

// normalizeGroup makes a platform option group valid for our model.
func normalizeGroup(g domain.OptionGroup) domain.OptionGroup {
	g.Min = max(g.Min, 0)
	if g.Min > len(g.Choices) {
		g.Min = len(g.Choices)
	}
	if g.Max < 0 {
		g.Max = 0
	}
	if g.Max > 0 && g.Max < g.Min {
		g.Max = g.Min
	}
	if g.Max >= len(g.Choices) && g.Max > 1 {
		g.Max = 0 // no effective bound
	}
	if g.Name == "" {
		g.Name = "Options"
	}
	return g
}

func dropEmptyCategories(cs []catalog.CategoryImport) []catalog.CategoryImport {
	out := cs[:0]
	for _, c := range cs {
		if c.Name != "" && len(c.Items) > 0 {
			out = append(out, c)
		}
	}
	return out
}

func cleanDeliverooAddress(a string) string {
	a = CleanText(a)
	// Deliveroo files Belgian zones under the "Brussels" city: drop it
	// unless the postcode is a Brussels one.
	if brusselsRe.MatchString(a) && !brusselsPC.MatchString(a) {
		a = brusselsRe.ReplaceAllString(a, "")
	}
	return strings.Trim(CleanText(a), ", ")
}

// SortEntriesByDistance sorts listing entries, known distances first.
func SortEntriesByDistance(es []ListingEntry) {
	sort.SliceStable(es, func(i, j int) bool {
		di, dj := es[i].DistanceKm, es[j].DistanceKm
		if (di < 0) != (dj < 0) {
			return dj < 0
		}
		return di < dj
	})
}
