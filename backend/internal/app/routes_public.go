package app

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

func (h *handlers) health(e *core.RequestEvent) error {
	return ok(e, map[string]any{"status": "ok", "version": h.cfg.Version})
}

type providerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	Enabled bool   `json:"enabled"`
}

func (h *handlers) config(e *core.RequestEvent) error {
	list := make([]providerInfo, 0, len(providers.Platforms))
	for _, id := range providers.Platforms {
		p, _ := providers.Get(id)
		list = append(list, providerInfo{ID: id, Name: p.Name(), Color: p.Color(), Enabled: h.cfg.providerEnabled(id)})
	}
	return ok(e, map[string]any{
		"currency": "EUR",
		"defaultLocation": map[string]any{
			"lat": h.cfg.DefaultLat, "lng": h.cfg.DefaultLng, "label": h.cfg.DefaultLabel,
		},
		"providers": list,
	})
}

// fold lowercases and strips accents for forgiving searches.
func fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	return strings.ToLower(out)
}

func queryFloat(e *core.RequestEvent, key string, def float64) float64 {
	v, err := strconv.ParseFloat(e.Request.URL.Query().Get(key), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return def
	}
	return v
}

func (h *handlers) nearby(e *core.RequestEvent) error {
	lat := queryFloat(e, "lat", h.cfg.DefaultLat)
	lng := queryFloat(e, "lng", h.cfg.DefaultLng)
	radius := queryFloat(e, "radiusKm", 5)
	if radius <= 0 || radius > 100 {
		radius = 5
	}
	q := fold(strings.TrimSpace(e.Request.URL.Query().Get("q")))
	cuisine := fold(strings.TrimSpace(e.Request.URL.Query().Get("cuisine")))

	recs, err := e.App.FindAllRecords(catalog.Restaurants, dbx.HashExp{"active": true})
	if err != nil {
		return err
	}

	type hit struct {
		data   map[string]any
		dist   float64
		name   string
		approx bool
	}
	hits := []hit{}
	for _, r := range recs {
		var cuisines []string
		_ = r.UnmarshalJSONField("cuisines", &cuisines)
		if cuisine != "" {
			found := false
			for _, c := range cuisines {
				if fold(c) == cuisine {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if q != "" {
			hay := fold(r.GetString("name") + " " + r.GetString("description") + " " + strings.Join(cuisines, " "))
			if !strings.Contains(hay, q) {
				continue
			}
		}
		d := domain.HaversineKm(lat, lng, r.GetFloat("lat"), r.GetFloat("lng"))
		if d > radius {
			continue
		}
		data := r.PublicExport()
		data["distanceKm"] = math.Round(d*100) / 100
		hits = append(hits, hit{data: data, dist: d, name: r.GetString("name"), approx: r.GetBool("geo_approx")})
	}
	// approximate positions (Uber Eats snapshot) come after the real ones
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].approx != hits[j].approx {
			return !hits[i].approx
		}
		if hits[i].dist != hits[j].dist {
			return hits[i].dist < hits[j].dist
		}
		return hits[i].name < hits[j].name
	})
	items := make([]map[string]any, 0, len(hits))
	for _, x := range hits {
		items = append(items, x.data)
	}
	return ok(e, map[string]any{"items": items})
}
