package migrations

import (
	"slices"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Uber Eats snapshot (docs/adr/0002-providers.md, update 3): restaurants
// retrieved through the Uber Eats connector of a Claude session, committed to
// data/mons_ubereats.json and read by the sync source "ubereats-snapshot".
//   - restaurants.partial_menu: only a few sample items are known;
//   - restaurants.geo_approx: lat / lng are approximate (office location);
//   - sync_sources.provider gains "ubereats-snapshot", with one seeded row.

// SourceUberEatsSnapshot is the sync_sources.provider of the snapshot.
const SourceUberEatsSnapshot = "ubereats-snapshot"

// UberEatsSnapshotLabel is the label of the seeded snapshot source.
const UberEatsSnapshotLabel = "Uber Eats (instantané connecteur)"

// UberEatsSnapshotPriority is the priority of the seeded snapshot source
// (after the feeds: their full menus win).
const UberEatsSnapshotPriority = 70

func init() {
	m.Register(upUberEatsSnapshot, downUberEatsSnapshot)
}

func upUberEatsSnapshot(app core.App) error {
	rest, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		return err
	}
	for _, name := range []string{"partial_menu", "geo_approx"} {
		if rest.Fields.GetByName(name) == nil {
			rest.Fields.Add(&core.BoolField{Name: name})
		}
	}
	if err := app.Save(rest); err != nil {
		return err
	}

	src, err := app.FindCollectionByNameOrId("sync_sources")
	if err != nil {
		return err
	}
	if f, ok := src.Fields.GetByName("provider").(*core.SelectField); ok && !slices.Contains(f.Values, SourceUberEatsSnapshot) {
		f.Values = append(f.Values, SourceUberEatsSnapshot)
		if err := app.Save(src); err != nil {
			return err
		}
	}
	n, err := app.CountRecords("sync_sources", dbx.HashExp{"provider": SourceUberEatsSnapshot})
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	r := core.NewRecord(src)
	r.Load(map[string]any{
		"provider": SourceUberEatsSnapshot, "label": UberEatsSnapshotLabel, "url": "", "city": "mons",
		"priority": UberEatsSnapshotPriority, "enabled": true, "options": false,
	})
	return app.Save(r)
}

func downUberEatsSnapshot(app core.App) error {
	if src, err := app.FindCollectionByNameOrId("sync_sources"); err == nil {
		recs, err := app.FindAllRecords("sync_sources", dbx.HashExp{"provider": SourceUberEatsSnapshot})
		if err != nil {
			return err
		}
		for _, r := range recs {
			if err := app.Delete(r); err != nil {
				return err
			}
		}
		if f, ok := src.Fields.GetByName("provider").(*core.SelectField); ok {
			f.Values = slices.DeleteFunc(slices.Clone(f.Values), func(v string) bool { return v == SourceUberEatsSnapshot })
			if err := app.Save(src); err != nil {
				return err
			}
		}
	}
	if rest, err := app.FindCollectionByNameOrId("restaurants"); err == nil {
		rest.Fields.RemoveByName("partial_menu")
		rest.Fields.RemoveByName("geo_approx")
		return app.Save(rest)
	}
	return nil
}
