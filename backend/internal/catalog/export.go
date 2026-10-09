package catalog

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// UncategorizedName holds the items that have no category when exporting.
const UncategorizedName = "Autres"

// ExportAll returns every restaurant (active or not) with its menu, in the
// import format, sorted by name. Importing the result restores the catalogue.
func ExportAll(app core.App) ([]RestaurantImport, error) {
	recs, err := app.FindRecordsByFilter(Restaurants, "", "name,slug", 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]RestaurantImport, 0, len(recs))
	for _, r := range recs {
		in, err := exportRecord(app, r)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, nil
}

// ExportOne returns the restaurant with this slug in the import format, or
// (nil, nil) when it does not exist.
func ExportOne(app core.App, slug string) (*RestaurantImport, error) {
	r, err := app.FindFirstRecordByData(Restaurants, "slug", slug)
	if err != nil {
		return nil, nil //nolint:nilerr // not found
	}
	in, err := exportRecord(app, r)
	if err != nil {
		return nil, err
	}
	return &in, nil
}

func exportRecord(app core.App, r *core.Record) (RestaurantImport, error) {
	active := r.GetBool("active")
	in := RestaurantImport{
		Slug:        r.GetString("slug"),
		Name:        r.GetString("name"),
		Description: r.GetString("description"),
		Emoji:       r.GetString("emoji"),
		CoverURL:    r.GetString("cover_url"),
		Address:     r.GetString("address"),
		Lat:         r.GetFloat("lat"),
		Lng:         r.GetFloat("lng"),
		Phone:       r.GetString("phone"),
		Rating:      r.GetFloat("rating"),
		RatingCount: r.GetInt("rating_count"),
		PriceLevel:  r.GetInt("price_level"),
		EtaMin:      r.GetInt("eta_min"),
		EtaMax:      r.GetInt("eta_max"),
		DeliveryFee: r.GetInt("delivery_fee"),
		MinOrder:    r.GetInt("min_order"),
		Active:      &active,
		PartialMenu: r.GetBool("partial_menu"),
		GeoApprox:   r.GetBool("geo_approx"),
		Cuisines:    []string{},
		Providers:   []providers.Link{},
		Categories:  []CategoryImport{},
	}
	_ = r.UnmarshalJSONField("cuisines", &in.Cuisines)
	_ = r.UnmarshalJSONField("providers", &in.Providers)
	in.Cuisines = orEmpty(in.Cuisines)
	in.Providers = orEmpty(in.Providers)

	cats, err := app.FindRecordsByFilter(MenuCategories, "restaurant = {:r}", "position,created", 0, 0, dbx.Params{"r": r.Id})
	if err != nil {
		return in, err
	}
	items, err := app.FindRecordsByFilter(MenuItems, "restaurant = {:r}", "position,created", 0, 0, dbx.Params{"r": r.Id})
	if err != nil {
		return in, err
	}
	known := map[string]bool{}
	for _, c := range cats {
		known[c.Id] = true
	}
	byCat := map[string][]ItemImport{}
	var orphans []ItemImport
	for _, it := range items {
		available := it.GetBool("available")
		item := ItemImport{
			Name:         it.GetString("name"),
			Description:  it.GetString("description"),
			Price:        it.GetInt("price"),
			Emoji:        it.GetString("emoji"),
			Tags:         []string{},
			OptionGroups: []domain.OptionGroup{},
			Popular:      it.GetBool("popular"),
			Available:    &available,
		}
		_ = it.UnmarshalJSONField("tags", &item.Tags)
		_ = it.UnmarshalJSONField("option_groups", &item.OptionGroups)
		item.Tags = orEmpty(item.Tags)
		item.OptionGroups = orEmpty(item.OptionGroups)
		if cat := it.GetString("category"); known[cat] {
			byCat[cat] = append(byCat[cat], item)
		} else {
			orphans = append(orphans, item)
		}
	}
	for _, c := range cats {
		in.Categories = append(in.Categories, CategoryImport{Name: c.GetString("name"), Items: orEmpty(byCat[c.Id])})
	}
	if len(orphans) > 0 {
		in.Categories = append(in.Categories, CategoryImport{Name: UncategorizedName, Items: orphans})
	}
	return in, nil
}
