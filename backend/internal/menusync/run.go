package menusync

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// City is a search centre.
type City struct {
	Lat, Lng float64
}

// Cities known by -city.
var Cities = map[string]City{
	"mons": {Lat: 50.4542, Lng: 3.9567},
}

// Options drive a fetch run.
type Options struct {
	City     string
	Center   City
	RadiusKm float64
	Limit    int  // 0 = no limit
	Details  bool // fetch per-product details (weloveat supplements)
	URLs     []string
	// ListingURL replaces the Deliveroo city listing page (its host is then
	// used for the menu pages too).
	ListingURL string
	// APIBase replaces the weloveat API root (https://api.weloveat.be/api/).
	APIBase string
	Today   string // menu_checked_at (YYYY-MM-DD)
	Logf    func(format string, args ...any)
}

func (o Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

func (o Options) within(lat, lng float64) bool {
	if o.RadiusKm <= 0 || (lat == 0 && lng == 0) {
		return true
	}
	return domain.HaversineKm(o.Center.Lat, o.Center.Lng, lat, lng) <= o.RadiusKm
}

// Run fetches a source. On an ErrBlocked it returns the restaurants already
// collected together with the error.
func Run(ctx context.Context, f *Fetcher, source string, o Options) ([]Restaurant, error) {
	var out []Restaurant
	var err error
	switch source {
	case SourceDeliveroo:
		out, err = runDeliveroo(ctx, f, o)
	case SourceWeloveat:
		out, err = runWeloveat(ctx, f, o)
	case SourceTakeawaySite:
		out, err = runPages(ctx, f, o, ParseTakeawaySite)
	case SourceJSONLD:
		out, err = runPages(ctx, f, o, ParseJSONLD)
	default:
		return nil, fmt.Errorf("source inconnue %q (deliveroo, weloveat, takeaway-site, jsonld)", source)
	}
	// the same place can be listed twice by a platform (two brands, two links)
	if len(out) > 1 {
		var st MergeStats
		for i := range out {
			out[i].Source = source
		}
		out, st = MergeIn(out, nil, o.City)
		for _, g := range st.Merged {
			o.logf("doublon fusionné : %s", strings.Join(g.Sources, " | "))
		}
	}
	for i := range out {
		r := &out[i]
		r.Source = source
		if r.MenuCheckedAt == "" {
			r.MenuCheckedAt = o.Today
		}
		if len(r.Origins) == 0 && len(r.SourceURLs) > 0 {
			r.Origins = []Origin{{Source: source, URL: r.SourceURLs[0], CheckedAt: r.MenuCheckedAt}}
		}
		for j := range r.Origins {
			r.Origins[j].Source, r.Origins[j].CheckedAt = source, r.MenuCheckedAt
		}
		r.Clean(o.City)
		r.Slug = Slugify(r.Name)
		r.Normalize()
	}
	return out, err
}

// Normalize replaces nil slices by empty ones (stable JSON output).
func (r *Restaurant) Normalize() {
	if r.Cuisines == nil {
		r.Cuisines = []string{}
	}
	if r.Providers == nil {
		r.Providers = []providers.Link{}
	}
	if r.Categories == nil {
		r.Categories = []catalog.CategoryImport{}
	}
	if r.SourceURLs == nil {
		r.SourceURLs = []string{}
	}
	for ci := range r.Categories {
		for ii := range r.Categories[ci].Items {
			it := &r.Categories[ci].Items[ii]
			if it.Tags == nil {
				it.Tags = []string{}
			}
			if it.OptionGroups == nil {
				it.OptionGroups = []domain.OptionGroup{}
			}
		}
	}
}

func keep(o Options, r Restaurant) error {
	if err := r.RestaurantImport.Validate(); err != nil {
		return err
	}
	if r.ItemCount() == 0 {
		return errors.New("aucun plat lisible sur la page (fermé, épicerie à sous-catégories ou carte vide)")
	}
	if !o.within(r.Lat, r.Lng) {
		return fmt.Errorf("à %.1f km du centre (> %.0f km)", domain.HaversineKm(o.Center.Lat, o.Center.Lng, r.Lat, r.Lng), o.RadiusKm)
	}
	return nil
}

func runDeliveroo(ctx context.Context, f *Fetcher, o Options) ([]Restaurant, error) {
	listing := o.ListingURL
	if listing == "" {
		var ok bool
		if listing, ok = DeliverooCityListing[o.City]; !ok {
			return nil, fmt.Errorf("ville %q inconnue pour Deliveroo", o.City)
		}
	}
	base, err := siteRoot(listing)
	if err != nil {
		return nil, err
	}
	page, err := f.Get(ctx, listing)
	if err != nil {
		return nil, err
	}
	entries, err := ParseDeliverooListing(page)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Base = base
	}
	SortEntriesByDistance(entries)
	o.logf("Deliveroo : %d restaurants dans la liste", len(entries))
	var out []Restaurant
	for _, e := range entries {
		if o.Limit > 0 && len(out) >= o.Limit {
			break
		}
		if o.RadiusKm > 0 && e.DistanceKm > o.RadiusKm {
			o.logf("- %s : ignoré, %.1f km", e.Name, e.DistanceKm)
			continue
		}
		page, err := f.Get(ctx, e.MenuURL())
		if err != nil {
			if IsBlocked(err) || errors.Is(err, context.Canceled) {
				return out, err
			}
			o.logf("- %s : %v", e.Name, err)
			continue
		}
		r, err := ParseDeliverooMenu(page, e.PageURL())
		if err == nil {
			err = keep(o, r)
		}
		if err != nil {
			o.logf("- %s : ignoré (%v)", e.Name, err)
			continue
		}
		o.logf("+ %s : %d plats, %d avec options", r.Name, r.ItemCount(), r.ItemsWithOptions())
		out = append(out, r)
	}
	return out, nil
}

func runWeloveat(ctx context.Context, f *Fetcher, o Options) ([]Restaurant, error) {
	city, ok := WeloveatCities[o.City]
	if !ok {
		return nil, fmt.Errorf("ville %q inconnue pour weloveat", o.City)
	}
	api := WeloveatAPI
	if o.APIBase != "" {
		api = strings.TrimSuffix(o.APIBase, "/") + "/"
	}
	b, err := f.Do(ctx, Request{Method: "POST", URL: api + weloveatSearchPath, Body: WeloveatSearchBody(city), Accept: "application/json", Sanitize: SanitizeWeloveat})
	if err != nil {
		return nil, err
	}
	shops, err := ParseWeloveatSearch(b)
	if err != nil {
		return nil, err
	}
	o.logf("weloveat : %d établissements dans la recherche", len(shops))
	var out []Restaurant
	for _, s := range shops {
		if o.Limit > 0 && len(out) >= o.Limit {
			break
		}
		if !o.within(s.Lat, s.Lng) {
			o.logf("- %s : ignoré (hors rayon)", s.Name)
			continue
		}
		b, err := f.Do(ctx, Request{Method: "POST", URL: api + weloveatCataloguePath, Body: WeloveatCatalogueBody(s.Slug, city), Accept: "application/json", Sanitize: SanitizeWeloveat})
		if err != nil {
			if IsBlocked(err) || errors.Is(err, context.Canceled) {
				return out, err
			}
			o.logf("- %s : %v", s.Name, err)
			continue
		}
		r, refs, err := ParseWeloveatCatalogue(b, WeloveatPageURL(s.Slug))
		if err != nil {
			o.logf("- %s : %v", s.Name, err)
			continue
		}
		// the catalogue answer omits coordinates: take them from the search
		if r.Lat == 0 && r.Lng == 0 {
			r.Lat, r.Lng = s.Lat, s.Lng
		}
		if r.Address == "" {
			r.Address = s.Address
		}
		if r.Phone == "" {
			r.Phone = s.Phone
		}
		if o.Details {
			for _, ref := range refs {
				pb, err := f.Do(ctx, Request{URL: api + weloveatProductPath + ref.Slug, Accept: "application/json", Sanitize: SanitizeWeloveat})
				if err != nil {
					if IsBlocked(err) || errors.Is(err, context.Canceled) {
						return out, err
					}
					o.logf("  produit %s : %v", ref.Slug, err)
					continue
				}
				groups, err := ParseWeloveatProduct(pb)
				if err != nil {
					o.logf("  produit %s : %v", ref.Slug, err)
					continue
				}
				r.Categories[ref.Category].Items[ref.Item].OptionGroups = groups
			}
		}
		if err := keep(o, r); err != nil {
			o.logf("- %s : ignoré (%v)", s.Name, err)
			continue
		}
		o.logf("+ %s : %d plats, %d avec options", r.Name, r.ItemCount(), r.ItemsWithOptions())
		out = append(out, r)
	}
	return out, nil
}

func runPages(ctx context.Context, f *Fetcher, o Options, parse func([]byte, string) (Restaurant, error)) ([]Restaurant, error) {
	if len(o.URLs) == 0 {
		return nil, errors.New("aucune URL : utilisez -url ou -urls fichier.txt")
	}
	var out []Restaurant
	for _, u := range o.URLs {
		if o.Limit > 0 && len(out) >= o.Limit {
			break
		}
		page, err := f.Get(ctx, u)
		if err != nil {
			if IsBlocked(err) || errors.Is(err, context.Canceled) {
				return out, err
			}
			o.logf("- %s : %v", u, err)
			continue
		}
		r, err := parse(page, u)
		if err == nil {
			err = keep(o, r)
		}
		if err != nil {
			o.logf("- %s : ignoré (%v)", u, err)
			continue
		}
		o.logf("+ %s : %d plats", r.Name, r.ItemCount())
		out = append(out, r)
	}
	return out, nil
}

func siteRoot(u string) (string, error) {
	pu, err := url.Parse(u)
	if err != nil || pu.Host == "" || (pu.Scheme != "https" && pu.Scheme != "http") {
		return "", fmt.Errorf("URL invalide : %q", u)
	}
	return pu.Scheme + "://" + pu.Host, nil
}

// Today returns the current date in Brussels as YYYY-MM-DD.
func Today() string {
	loc, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("2006-01-02")
}
