package app

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/enrich"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
)

// OpenStreetMap enrichment at the end of a synchronisation run (see
// internal/enrich): active, unlocked restaurants missing a phone, an address
// or real coordinates are looked up in Nominatim; only empty fields are
// filled, and restaurants.enriched_from records the attribution (ODbL).

func (s *syncer) newEnrichClient() *enrich.Client {
	dir := s.cfg.CacheDir
	if dir == "" {
		dir = filepath.Join(s.app.DataDir(), "menusync-cache")
	}
	if s.cfg.Enrich.NewClient != nil {
		return s.cfg.Enrich.NewClient(dir)
	}
	return enrich.NewClient(dir)
}

// enrichRun completes the restaurants (only those of scope when not nil)
// and returns the number of restaurants written and their change lines.
func (s *syncer) enrichRun(ctx context.Context, lg *runLog, city string, scope map[string]bool) (int, []string) {
	app := s.app
	recs, err := app.FindRecordsByFilter(colRestaurants, "active = true && locked = false", "name", 0, 0)
	if err != nil {
		lg.logf("! OpenStreetMap : %v", err)
		return 0, nil
	}
	var targets []enrich.Target
	for _, r := range recs {
		if scope != nil && !scope[r.Id] {
			continue
		}
		targets = append(targets, enrich.Target{
			ID: r.Id, Name: r.GetString("name"), Address: r.GetString("address"), Phone: r.GetString("phone"),
			Lat: r.GetFloat("lat"), Lng: r.GetFloat("lng"), GeoApprox: r.GetBool("geo_approx"),
		})
	}
	area := enrich.Area{Lat: s.cfg.DefaultLat, Lng: s.cfg.DefaultLng, City: city, Label: s.cfg.DefaultLabel}
	if area.Lat == 0 && area.Lng == 0 {
		c, ok := menusync.Cities[city]
		if !ok {
			c = menusync.Cities["mons"]
		}
		area.Lat, area.Lng = c.Lat, c.Lng
	}
	c := s.newEnrichClient()
	c.Logf = lg.logf
	lg.logf("OpenStreetMap : recherche des coordonnées manquantes")
	res := c.Enrich(ctx, targets, enrich.Options{Area: area, MaxLookups: s.cfg.Enrich.MaxLookups, Logf: lg.logf})
	today := time.Now().In(s.loc).Format(time.DateOnly)
	n := 0
	var changes []string
	for _, p := range res.Patches {
		applied, err := applyEnrichment(app, p, today)
		if err != nil {
			lg.logf("! %s : %v", p.Name, err)
			continue
		}
		if !applied.Empty() {
			n++
			changes = append(changes, applied.Change())
		}
	}
	lg.logf("OpenStreetMap : %d à compléter, %d recherchés (%d requêtes, %d depuis le cache), %d complétés, %d reportés à une prochaine exécution",
		res.Candidates, res.Looked, c.Network, c.Cached, n, res.Deferred)
	if res.Stopped != "" {
		lg.logf("OpenStreetMap : arrêt anticipé (%s)", res.Stopped)
	}
	return n, changes
}

// applyEnrichment writes a patch in a transaction, re-checking that the
// restaurant is still active, unlocked, and that each field is still empty
// (an admin or a feed may have filled it meanwhile). It returns what was
// actually written.
func applyEnrichment(app core.App, p enrich.Patch, today string) (enrich.Patch, error) {
	applied := enrich.Patch{ID: p.ID, Name: p.Name, URL: p.URL}
	err := app.RunInTransaction(func(tx core.App) error {
		rec, err := tx.FindRecordById(colRestaurants, p.ID)
		if err != nil {
			return err
		}
		if rec.GetBool("locked") || !rec.GetBool("active") {
			return nil
		}
		if p.Phone != "" && strings.TrimSpace(rec.GetString("phone")) == "" {
			rec.Set("phone", p.Phone)
			applied.Phone = p.Phone
			applied.Fields = append(applied.Fields, enrich.FieldPhone)
		}
		if p.Address != "" && strings.TrimSpace(rec.GetString("address")) == "" {
			rec.Set("address", truncate(p.Address, 300))
			applied.Address = p.Address
			applied.Fields = append(applied.Fields, enrich.FieldAddress)
		}
		if p.SetGeo && (rec.GetBool("geo_approx") || (rec.GetFloat("lat") == 0 && rec.GetFloat("lng") == 0)) {
			rec.Set("lat", p.Lat)
			rec.Set("lng", p.Lng)
			rec.Set("geo_approx", false)
			applied.Lat, applied.Lng, applied.SetGeo = p.Lat, p.Lng, true
			applied.Fields = append(applied.Fields, enrich.FieldGeo)
		}
		if applied.Empty() {
			return nil
		}
		a := enrich.Attribution{Provider: enrich.Provider, URL: p.URL, CheckedAt: today}
		var prev enrich.Attribution
		if raw := strings.TrimSpace(rec.GetString("enriched_from")); raw != "" && raw != "null" && rec.UnmarshalJSONField("enriched_from", &prev) == nil {
			a.Fields = prev.Fields
		}
		for _, f := range applied.Fields {
			if !slices.Contains(a.Fields, f) {
				a.Fields = append(a.Fields, f)
			}
		}
		rec.Set("enriched_from", a)
		return tx.Save(rec)
	})
	if err != nil {
		return enrich.Patch{}, err
	}
	return applied, nil
}
