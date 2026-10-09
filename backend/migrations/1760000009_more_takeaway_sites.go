package migrations

import (
	"slices"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// More Takeaway.com mini-sites (data/mons_takeaway_sites.txt, research of
// 2026-10-09 described in data/SOURCES_menusync.md): production was seeded by
// 1760000005 with the first ten sites only. This migration adds a
// takeaway-site sync_sources row for every URL of the file that has none yet
// (idempotent: a fresh database already got them all from 1760000005).

// originalTakeawaySites are the sites seeded in production by 1760000005.
var originalTakeawaySites = []string{
	"https://www.ladriaticamons.be/",
	"https://www.o-sando-mons.be/",
	"https://www.tomomons.be/",
	"https://www.papoumons.be/",
	"https://www.cup-pasta-mons.be/",
	"https://www.sushi-lover.be/",
	"https://www.snack-alibaba.be/",
	"https://www.lebosphore-mons.be/",
	"https://www.baalbeckmons.be/",
	"https://www.lafritemayomons-mons.be/",
}

// Takeaway mini-sites use the priorities 10 to 39 (in file order, the last
// ones sharing 39); Deliveroo, weloveat and the Uber Eats snapshot follow.
const (
	takeawaySiteFirstPriority = 10
	takeawaySiteLastPriority  = 39
)

func init() {
	m.Register(upMoreTakeawaySites, downMoreTakeawaySites)
}

// takeawaySiteLabel is the sync_sources label of a mini-site URL.
func takeawaySiteLabel(u string) string {
	l := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	return "Site " + strings.TrimSuffix(strings.TrimPrefix(l, "www."), "/")
}

// NewTakeawaySiteURLs lists the mini-sites added after 1760000005.
func NewTakeawaySiteURLs() []string {
	var out []string
	for _, u := range TakeawaySiteURLs() {
		if !slices.Contains(originalTakeawaySites, u) {
			out = append(out, u)
		}
	}
	return out
}

func upMoreTakeawaySites(app core.App) error {
	col, err := app.FindCollectionByNameOrId("sync_sources")
	if err != nil {
		return err
	}
	for i, u := range TakeawaySiteURLs() {
		n, err := app.CountRecords(col, dbx.HashExp{"url": u})
		if err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		r := core.NewRecord(col)
		r.Load(map[string]any{
			"provider": "takeaway-site", "label": takeawaySiteLabel(u), "url": u, "city": "mons",
			"priority": min(takeawaySiteFirstPriority+i, takeawaySiteLastPriority), "enabled": true, "options": false,
		})
		if err := app.Save(r); err != nil {
			return err
		}
	}
	return nil
}

func downMoreTakeawaySites(app core.App) error {
	for _, u := range NewTakeawaySiteURLs() {
		recs, err := app.FindRecordsByFilter("sync_sources", "provider = 'takeaway-site' && url = {:url}", "", 0, 0, dbx.Params{"url": u})
		if err != nil {
			return err
		}
		for _, r := range recs {
			if err := app.Delete(r); err != nil {
				return err
			}
		}
	}
	return nil
}
