package migrations

import (
	"bufio"
	"bytes"
	_ "embed"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Automatic menu synchronisation (docs/adr/0002-providers.md, update of
// 2026-10-09): reconciliation fields on restaurants / menu_items, the
// sync_sources and sync_runs collections, and the default Mons sources.

//go:embed data/mons_takeaway_sites.txt
var takeawaySitesTxt []byte

// Default sources.
const (
	DefaultDeliverooListing = "https://deliveroo.be/fr/restaurants/brussels/mons-center?fulfillment_method=DELIVERY&geohash=u0fz40u6z12k"
	DefaultWeloveatAPI      = "https://api.weloveat.be/api/"
)

// SyncProviders are the values of sync_sources.provider.
var SyncProviders = []string{"deliveroo", "weloveat", "takeaway-site", "jsonld"}

// SyncRunStatuses are the values of sync_runs.status.
var SyncRunStatuses = []string{"running", "success", "partial", "failed", "blocked"}

// SyncTriggers are the values of sync_runs.trigger.
var SyncTriggers = []string{"cron", "manual", "startup"}

func init() {
	m.Register(upSync, downSync)
}

// TakeawaySiteURLs lists the URLs of data/mons_takeaway_sites.txt.
func TakeawaySiteURLs() []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(takeawaySitesTxt))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}

func upSync(app core.App) error {
	// ------------------------------------------- reconciliation fields
	rest, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		return err
	}
	if rest.Fields.GetByName("source_key") == nil {
		rest.Fields.Add(
			&core.TextField{Name: "source_key", Max: 300},
			&core.JSONField{Name: "sources", MaxSize: 16 << 10},
			&core.BoolField{Name: "locked"},
			&core.DateField{Name: "stale_since"},
		)
		rest.AddIndex("idx_restaurants_source_key", false, "source_key", "")
	}
	if err := app.Save(rest); err != nil {
		return err
	}

	items, err := app.FindCollectionByNameOrId("menu_items")
	if err != nil {
		return err
	}
	if items.Fields.GetByName("source_key") == nil {
		items.Fields.Add(
			&core.TextField{Name: "source_key", Max: 300},
			&core.JSONField{Name: "sources", MaxSize: 8 << 10},
			&core.BoolField{Name: "locked"},
		)
	}
	// a 0 € item whose price lives in its options is valid: a required
	// number field would refuse 0 (min 0 still applies)
	if f, ok := items.Fields.GetByName("price").(*core.NumberField); ok {
		f.Required = false
	}
	if err := app.Save(items); err != nil {
		return err
	}

	// ------------------------------------------------------ sync_sources
	if _, err := app.FindCollectionByNameOrId("sync_sources"); err != nil {
		src := core.NewBaseCollection("sync_sources")
		src.ListRule = ptr(isAdminRule)
		src.ViewRule = ptr(isAdminRule)
		src.CreateRule = ptr(isAdminRule)
		src.UpdateRule = ptr(isAdminRule)
		src.DeleteRule = ptr(isAdminRule)
		src.Fields.Add(
			&core.SelectField{Name: "provider", Required: true, MaxSelect: 1, Values: SyncProviders},
			&core.TextField{Name: "label", Max: 120},
			&core.TextField{Name: "url", Max: 1000},
			&core.TextField{Name: "city", Max: 60},
			intField("priority", nil, false),
			&core.BoolField{Name: "enabled"},
			&core.BoolField{Name: "options"},
			&core.DateField{Name: "last_run_at"},
			&core.TextField{Name: "last_status", Max: 1000},
		)
		autodates(src)
		if err := app.Save(src); err != nil {
			return err
		}
		if err := seedSyncSources(app, src); err != nil {
			return err
		}
	}

	// --------------------------------------------------------- sync_runs
	if _, err := app.FindCollectionByNameOrId("sync_runs"); err != nil {
		runs := core.NewBaseCollection("sync_runs")
		runs.ListRule = ptr(isAdminRule)
		runs.ViewRule = ptr(isAdminRule)
		// create / update / delete: nil = server (and superusers) only
		runs.Fields.Add(
			&core.DateField{Name: "started_at"},
			&core.DateField{Name: "finished_at"},
			&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: SyncRunStatuses},
			&core.SelectField{Name: "trigger", Required: true, MaxSelect: 1, Values: SyncTriggers},
			&core.JSONField{Name: "stats", MaxSize: 4 << 10},
			&core.JSONField{Name: "changes", MaxSize: 512 << 10},
			&core.JSONField{Name: "sources", MaxSize: 64 << 10},
			&core.TextField{Name: "log", Max: 200_000},
			&core.TextField{Name: "error", Max: 4000},
		)
		autodates(runs)
		runs.AddIndex("idx_sync_runs_started", false, "started_at", "")
		if err := app.Save(runs); err != nil {
			return err
		}
	}
	return nil
}

func seedSyncSources(app core.App, col *core.Collection) error {
	add := func(provider, label, url string, priority int) error {
		r := core.NewRecord(col)
		r.Load(map[string]any{
			"provider": provider, "label": label, "url": url, "city": "mons",
			"priority": priority, "enabled": true, "options": false,
		})
		return app.Save(r)
	}
	for i, u := range TakeawaySiteURLs() {
		if err := add("takeaway-site", takeawaySiteLabel(u), u, min(takeawaySiteFirstPriority+i, takeawaySiteLastPriority)); err != nil {
			return err
		}
	}
	if err := add("deliveroo", "Deliveroo — Mons", DefaultDeliverooListing, 50); err != nil {
		return err
	}
	return add("weloveat", "weloveat — Mons", DefaultWeloveatAPI, 60)
}

func downSync(app core.App) error {
	for _, name := range []string{"sync_runs", "sync_sources"} {
		if c, err := app.FindCollectionByNameOrId(name); err == nil {
			if err := app.Delete(c); err != nil {
				return err
			}
		}
	}
	if rest, err := app.FindCollectionByNameOrId("restaurants"); err == nil {
		rest.RemoveIndex("idx_restaurants_source_key")
		for _, f := range []string{"source_key", "sources", "locked", "stale_since"} {
			rest.Fields.RemoveByName(f)
		}
		if err := app.Save(rest); err != nil {
			return err
		}
	}
	if items, err := app.FindCollectionByNameOrId("menu_items"); err == nil {
		for _, f := range []string{"source_key", "sources", "locked"} {
			items.Fields.RemoveByName(f)
		}
		if f, ok := items.Fields.GetByName("price").(*core.NumberField); ok {
			f.Required = true
		}
		if err := app.Save(items); err != nil {
			return err
		}
	}
	return nil
}
