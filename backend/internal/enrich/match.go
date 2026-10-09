package enrich

import (
	"context"
	"fmt"
	"strings"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
)

// Matching limits.
const (
	// MaxAreaKm: a place farther than this from the default location is not
	// one of our restaurants.
	MaxAreaKm = 12.0
	// MaxShiftKm: a restaurant with real coordinates only matches a place
	// this close to them.
	MaxShiftKm = 1.5
	// SameSpotKm: homonymous places closer than this are one place mapped
	// twice (node + building); farther apart, the match is ambiguous.
	SameSpotKm = 0.3
)

// Provider is the attribution provider id stored in restaurants.enriched_from.
const Provider = "osm"

// Fields filled by the enrichment (restaurants.enriched_from.fields).
const (
	FieldPhone   = "phone"
	FieldAddress = "address"
	FieldGeo     = "geo"
)

// Target is a stored restaurant considered for enrichment.
type Target struct {
	ID        string
	Name      string
	Address   string
	Phone     string
	Lat, Lng  float64
	GeoApprox bool
}

// HasGeo reports whether the restaurant has real coordinates.
func (t Target) HasGeo() bool { return !t.GeoApprox && (t.Lat != 0 || t.Lng != 0) }

// Needs reports whether something is missing (phone, address or real coordinates).
func (t Target) Needs() bool {
	return strings.TrimSpace(t.Phone) == "" || strings.TrimSpace(t.Address) == "" || !t.HasGeo()
}

// Area is the search area (OCC_DEFAULT_LAT / LNG, city).
type Area struct {
	Lat, Lng float64
	City     string // menusync city key ("mons")
	Label    string // locality used in queries when the address has none ("Mons")
}

// Query is the free-form Nominatim query of a target: its cleaned store
// name and its locality (from the address postcode, else the area label).
func Query(t Target, a Area) string {
	name := menusync.CleanStoreName(t.Name, a.City)
	loc := addressLocality(t.Address)
	if loc == "" {
		loc = a.Label
	}
	if loc == "" {
		loc = "Mons"
	}
	return name + ", " + loc
}

// addressLocality returns the locality of a normalized address (« …, 7012
// Jemappes » → « Jemappes »).
func addressLocality(addr string) string {
	n := domain.NormalizeAddress(addr)
	i := strings.LastIndex(n, ", ")
	last := n
	if i >= 0 {
		last = n[i+2:]
	}
	if len(last) > 5 && last[4] == ' ' && isDigits(last[:4]) {
		return strings.TrimSpace(last[5:])
	}
	return ""
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// foodTypes are the OSM tags accepted as a restaurant (value = needs a
// strong name match: a bar is only accepted under the very same name).
var foodTypes = map[string]map[string]bool{
	"amenity": {"restaurant": false, "fast_food": false, "cafe": false, "ice_cream": false, "food_court": false, "bar": true, "pub": true, "biergarten": true},
	"shop":    {"bakery": false, "pastry": false, "deli": false, "confectionery": false, "ice_cream": false},
}

// Address is the normalized postal address of the place ("" without a street).
func (p Place) Address() string {
	if p.Road == "" {
		return ""
	}
	street := strings.TrimSpace(p.Road + " " + p.HouseNumber)
	loc := domain.LocalityForPostcode(p.Postcode)
	if loc == "" {
		loc = p.Locality
	}
	place := strings.TrimSpace(p.Postcode + " " + loc)
	if place == "" {
		return domain.NormalizeAddress(street)
	}
	return domain.NormalizeAddress(street + ", " + place)
}

// nameStrength is the best menusync.NameMatch of the target against the
// names of the place.
func nameStrength(t Target, p Place, city string) int {
	best := menusync.NameNoMatch
	for _, n := range p.Names {
		best = max(best, menusync.NameMatch(t.Name, n, city))
	}
	return best
}

// streetCompatible reports whether the place lies on the street of the
// known address (some significant street word in common). Unknown streets
// are compatible.
func streetCompatible(addr string, p Place) bool {
	if strings.TrimSpace(addr) == "" || p.Road == "" {
		return true
	}
	street := addr
	if i := strings.Index(addr, ","); i >= 0 {
		street = addr[:i]
	}
	words := func(s string) map[string]bool {
		all := strings.FieldsFunc(menusync.Fold(s), func(r rune) bool { return !(r >= 'a' && r <= 'z') })
		out := map[string]bool{}
		for _, w := range all {
			if len(w) > 2 && !streetFiller[w] {
				out[w] = true
			}
		}
		if len(out) == 0 { // « Rue de la Chaussée »: the filler words are the name
			for _, w := range all {
				if len(w) > 2 {
					out[w] = true
				}
			}
		}
		return out
	}
	a, b := words(street), words(p.Road)
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for w := range a {
		if b[w] {
			return true
		}
	}
	return false
}

var streetFiller = map[string]bool{
	"rue": true, "des": true, "les": true, "avenue": true, "chaussee": true, "place": true, "boulevard": true,
	"route": true, "chemin": true, "square": true, "quai": true, "galerie": true, "allee": true, "impasse": true,
}

// Pick chooses the place describing the target, or returns why none does.
// A place is accepted when it is a food amenity / shop, its name matches
// (menusync.NameMatch on every OSM name; bars and pubs need the same or a
// contained name), it lies within MaxAreaKm of the area centre, within
// MaxShiftKm of the target's real coordinates if any, and on the street of
// its known address. Homonymous candidates far apart are ambiguous (none).
func Pick(t Target, places []Place, a Area) (Place, string) {
	if len(places) == 0 {
		return Place{}, "aucun résultat"
	}
	type cand struct {
		p        Place
		strength int
		dist     float64 // to the target (real coordinates) or the area centre
	}
	var cands []cand
	reason := "aucun établissement de restauration à ce nom"
	for _, p := range places {
		strong, food := foodTypes[p.Category][p.Type]
		if !food {
			continue
		}
		s := nameStrength(t, p, a.City)
		if s == menusync.NameNoMatch || (strong && s < menusync.NameContained) {
			continue
		}
		if p.Lat == 0 && p.Lng == 0 {
			continue
		}
		dArea := domain.HaversineKm(a.Lat, a.Lng, p.Lat, p.Lng)
		if dArea > MaxAreaKm {
			reason = fmt.Sprintf("homonyme trop loin (%.1f km)", dArea)
			continue
		}
		d := dArea
		if t.HasGeo() {
			d = domain.HaversineKm(t.Lat, t.Lng, p.Lat, p.Lng)
			if d > MaxShiftKm {
				reason = fmt.Sprintf("homonyme à %.1f km de la position connue", d)
				continue
			}
		}
		if !streetCompatible(t.Address, p) {
			reason = "homonyme dans une autre rue (" + p.Road + ")"
			continue
		}
		cands = append(cands, cand{p: p, strength: s, dist: d})
	}
	if len(cands) == 0 {
		return Place{}, reason
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.strength > best.strength || (c.strength == best.strength && t.HasGeo() && c.dist < best.dist) {
			best = c
		}
	}
	if !t.HasGeo() {
		// without a position of our own, two homonyms far apart (two
		// branches of a chain) cannot be told apart
		for _, c := range cands {
			if c.strength == best.strength && domain.HaversineKm(c.p.Lat, c.p.Lng, best.p.Lat, best.p.Lng) > SameSpotKm {
				return Place{}, "plusieurs établissements homonymes, ambigu"
			}
		}
	}
	return best.p, ""
}

// Patch is what the enrichment writes on one restaurant (empty fields only).
type Patch struct {
	ID       string
	Name     string
	Phone    string // E.164, "" = unchanged
	Address  string // normalized, "" = unchanged
	Lat, Lng float64
	SetGeo   bool // real coordinates replace missing / approximate ones
	Fields   []string
	URL      string // OSM page of the place (attribution)
}

// Empty reports whether the patch writes nothing.
func (p Patch) Empty() bool { return len(p.Fields) == 0 }

// Fill returns the patch filling the target's empty fields from the place:
// phone (E.164, junk ignored), address (normalized), real coordinates.
// Non-empty values are never replaced.
func Fill(t Target, p Place) Patch {
	out := Patch{ID: t.ID, Name: t.Name, URL: p.URL()}
	if strings.TrimSpace(t.Phone) == "" {
		if ph, ok := domain.NormalizeRestaurantPhone(p.Phone); ok {
			out.Phone = ph
			out.Fields = append(out.Fields, FieldPhone)
		}
	}
	if strings.TrimSpace(t.Address) == "" {
		if ad := p.Address(); ad != "" {
			out.Address = ad
			out.Fields = append(out.Fields, FieldAddress)
		}
	}
	if !t.HasGeo() && (p.Lat != 0 || p.Lng != 0) {
		out.Lat, out.Lng, out.SetGeo = p.Lat, p.Lng, true
		out.Fields = append(out.Fields, FieldGeo)
	}
	return out
}

// Change is the human readable line of a patch (sync_runs.changes).
func (p Patch) Change() string {
	var what []string
	for _, f := range p.Fields {
		switch f {
		case FieldPhone:
			what = append(what, "téléphone ajouté")
		case FieldAddress:
			what = append(what, "adresse ajoutée")
		case FieldGeo:
			what = append(what, "position précisée")
		}
	}
	return fmt.Sprintf("%s — %s (OpenStreetMap)", p.Name, strings.Join(what, ", "))
}

// Attribution is restaurants.enriched_from: where the filled fields come from.
type Attribution struct {
	Provider  string   `json:"provider"`
	URL       string   `json:"url"`
	Fields    []string `json:"fields"`
	CheckedAt string   `json:"checked_at"`
}

// Options drive Enrich.
type Options struct {
	Area Area
	// MaxLookups caps the network lookups (cache hits are not counted);
	// 0 = DefaultMaxLookups.
	MaxLookups int
	Logf       func(format string, args ...any)
}

// Result of an enrichment pass.
type Result struct {
	Patches []Patch
	// Candidates needed something; Looked were searched (cache or network);
	// Deferred were left for a later run (lookup cap reached).
	Candidates, Looked, Deferred int
	// Stopped is the reason the pass stopped early ("" = completed).
	Stopped string
}

// Enrich looks up the targets missing a phone, an address or real
// coordinates, and returns the patches to apply. Network lookups are capped
// (MaxLookups) and spaced (MinInterval); a refusal stops the pass.
func (c *Client) Enrich(ctx context.Context, targets []Target, o Options) Result {
	logf := func(format string, args ...any) {
		if o.Logf != nil {
			o.Logf(format, args...)
		}
	}
	maxLookups := o.MaxLookups
	if maxLookups <= 0 {
		maxLookups = DefaultMaxLookups
	}
	var res Result
	for _, t := range targets {
		if !t.Needs() || strings.TrimSpace(t.Name) == "" {
			continue
		}
		res.Candidates++
		if ctx.Err() != nil {
			res.Stopped = "interrompu"
			break
		}
		if res.Stopped != "" {
			res.Deferred++
			continue
		}
		q := Query(t, o.Area)
		if c.Network >= maxLookups && !c.IsCached(q) {
			res.Deferred++
			continue
		}
		places, err := c.Search(ctx, q)
		if err != nil {
			if IsStopped(err) || ctx.Err() != nil {
				res.Stopped = err.Error()
				res.Deferred++
				logf("! %s", err)
				continue
			}
			logf("- %s : %v", t.Name, err)
			continue
		}
		res.Looked++
		p, why := Pick(t, places, o.Area)
		if why != "" {
			logf("- %s : %s", t.Name, why)
			continue
		}
		patch := Fill(t, p)
		if patch.Empty() {
			logf("= %s : reconnu (%s), rien à compléter", t.Name, p.URL())
			continue
		}
		logf("+ %s : %s", t.Name, strings.TrimSuffix(strings.TrimPrefix(patch.Change(), t.Name+" — "), " (OpenStreetMap)"))
		res.Patches = append(res.Patches, patch)
	}
	return res
}
