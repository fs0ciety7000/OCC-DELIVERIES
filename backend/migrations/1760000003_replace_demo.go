package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
)

func init() {
	m.Register(func(app core.App) error {
		_, err := ReplaceDemo(app)
		return err
	}, nil)
}

// ReplaceDemoResult summarizes what ReplaceDemo did.
type ReplaceDemoResult struct {
	Imported    int
	Deleted     int
	Deactivated int
	Skipped     []string
}

// ReplaceDemo upserts the real restaurants of RealDataFile, then removes the
// fictional demo restaurants (deactivating those referenced by a party).
// With no valid real restaurant it does nothing at all.
func ReplaceDemo(app core.App) (ReplaceDemoResult, error) {
	var res ReplaceDemoResult
	list, problems := RealRestaurants()
	res.Skipped = problems
	for _, p := range problems {
		app.Logger().Warn("real restaurant data skipped", "problem", p)
	}
	if len(list) == 0 {
		return res, nil
	}

	realSlugs := map[string]bool{}
	for _, in := range list {
		if _, _, err := catalog.Import(app, in); err != nil {
			return res, err
		}
		realSlugs[in.Slug] = true
		res.Imported++
	}

	for _, demo := range DemoRestaurants() {
		if realSlugs[demo.Slug] {
			continue // the real data reuses this slug: keep the imported version
		}
		rec, err := app.FindFirstRecordByData(catalog.Restaurants, "slug", demo.Slug)
		if err != nil {
			continue
		}
		used, err := app.CountRecords("parties", dbx.Or(dbx.HashExp{"restaurant": rec.Id}, dbx.Like("candidates", rec.Id)))
		if err != nil {
			return res, err
		}
		if used > 0 {
			if rec.GetBool("active") {
				rec.Set("active", false)
				if err := app.Save(rec); err != nil {
					return res, err
				}
			}
			res.Deactivated++
			continue
		}
		if err := app.Delete(rec); err != nil {
			return res, err
		}
		res.Deleted++
	}
	return res, nil
}
