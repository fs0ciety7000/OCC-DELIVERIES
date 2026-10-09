package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// An Uber Eats preview (partial_menu) created for a store whose platform name
// carried a neighbouring town (« Altaj (Quaregnon) ») duplicates the full
// restaurant of the feeds (« Altaj »). Fold such previews into the full
// restaurant: its Uber Eats link and missing rating move over, the preview is
// deleted (or hidden when a party already uses it).

func init() {
	m.Register(mergePartialDuplicates, func(app core.App) error { return nil })
}

func mergePartialDuplicates(app core.App) error {
	all, err := app.FindAllRecords("restaurants")
	if err != nil {
		return err
	}
	byKey := map[string][]*core.Record{}
	for _, r := range all {
		if !r.GetBool("partial_menu") {
			k := menusync.StoreNameKey(r.GetString("name"), "Mons")
			byKey[k] = append(byKey[k], r)
		}
	}
	for _, p := range all {
		if !p.GetBool("partial_menu") {
			continue
		}
		full := byKey[menusync.StoreNameKey(p.GetString("name"), "Mons")]
		if len(full) != 1 {
			continue
		}
		target := full[0]
		var links, have []providers.Link
		_ = p.UnmarshalJSONField("providers", &links)
		_ = target.UnmarshalJSONField("providers", &have)
		for _, l := range links {
			exists := false
			for _, h := range have {
				exists = exists || h.ID == l.ID
			}
			if !exists {
				have = append(have, l)
			}
		}
		target.Set("providers", have)
		if target.GetFloat("rating") == 0 {
			target.Set("rating", p.GetFloat("rating"))
			target.Set("rating_count", p.GetInt("rating_count"))
		}
		if err := app.Save(target); err != nil {
			return err
		}
		used, _ := app.CountRecords("parties", dbx.Or(dbx.HashExp{"restaurant": p.Id}, dbx.Like("candidates", p.Id)))
		if used > 0 {
			p.Set("active", false)
			if err := app.Save(p); err != nil {
				return err
			}
			continue
		}
		if err := app.Delete(p); err != nil {
			return err
		}
	}
	return nil
}
