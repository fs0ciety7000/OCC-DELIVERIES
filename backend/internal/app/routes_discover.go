package app

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
)

// discoverTimeout bounds a whole "Découvrir" request (≈ 16 domains, most of
// them absent: only DNS lookups; ~2 polite requests per live host).
const discoverTimeout = 90 * time.Second

// Takeaway satellite sources created by "Découvrir" take the first free
// priority in this range (the seeded sites start at 10; Deliveroo is 50).
const (
	discoverPriorityMin = 10
	discoverPriorityMax = 39
)

type discoverFound struct {
	menusync.DiscoverFound
	AlreadySource bool   `json:"alreadySource"`
	SourceID      string `json:"sourceId,omitempty"`
}

// sourceSiteKey is the identity of a site source: bare host + path.
func sourceSiteKey(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	p := strings.TrimSuffix(u.EscapedPath(), "/")
	return menusync.BareHost(u.Host) + strings.ToLower(p)
}

// findSiteSource returns the takeaway-site / jsonld source reading this site.
func findSiteSource(app core.App, siteURL string) (*core.Record, error) {
	key := sourceSiteKey(siteURL)
	if key == "" {
		return nil, nil
	}
	recs, err := app.FindRecordsByFilter(colSyncSources, "provider = 'takeaway-site' || provider = 'jsonld'", "priority", 0, 0)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		if sourceSiteKey(r.GetString("url")) == key {
			return r, nil
		}
	}
	return nil, nil
}

// POST /api/occ/admin/sync/discover {"query": "…"}
func (h *handlers) adminSyncDiscover(e *core.RequestEvent) error {
	var body struct {
		Query string `json:"query"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	q := strings.TrimSpace(body.Query)
	if q == "" {
		return badRequest("Indique un lien takeaway.com, un nom de restaurant ou l'adresse du site.")
	}
	if utf8.RuneCountInString(q) > 300 {
		return badRequest("Recherche trop longue (300 caractères au plus).")
	}
	if !h.sync.cfg.Enabled {
		return badRequest("La synchronisation est désactivée (OCC_SYNC_ENABLED=false) : aucune requête sortante.")
	}
	city := menusync.DefaultCity
	plan, err := menusync.PlanDiscovery(q, city)
	if err != nil {
		msg := err.Error()
		return badRequest(strings.ToUpper(msg[:1]) + msg[1:] + ".")
	}
	if !h.sync.discovering.TryLock() {
		return apis.NewApiError(http.StatusConflict, "Une découverte est déjà en cours, réessaie dans un instant.", nil)
	}
	defer h.sync.discovering.Unlock()

	p := h.sync.newProber()
	p.Center = menusync.City{Lat: h.cfg.DefaultLat, Lng: h.cfg.DefaultLng}
	ctx, cancel := context.WithTimeout(e.Request.Context(), discoverTimeout)
	defer cancel()
	start := time.Now()
	res := p.Discover(ctx, plan)
	e.App.Logger().Info("sync: découverte", "query", q, "hosts", len(plan.Hosts), "requests", p.Network,
		"found", len(res.Found), "duration", time.Since(start).Round(time.Millisecond))

	found := make([]discoverFound, 0, len(res.Found))
	for _, f := range res.Found {
		df := discoverFound{DiscoverFound: f}
		if src, err := findSiteSource(e.App, f.URL); err != nil {
			return err
		} else if src != nil {
			df.AlreadySource, df.SourceID = true, src.Id
		}
		found = append(found, df)
	}
	return ok(e, map[string]any{
		"query": q, "kind": plan.Kind, "slug": plan.Slug,
		"tried": orEmptyList(res.Tried), "found": found,
		"network": p.Network, "durationMs": time.Since(start).Milliseconds(),
		"interrupted": ctx.Err() != nil,
	})
}

// POST /api/occ/admin/sync/sources {"url": "https://www.site.be/", "label"?: "…"}
// adds a Takeaway satellite site as a source (idempotent on the site).
func (h *handlers) adminSyncAddSource(e *core.RequestEvent) error {
	var body struct {
		URL   string `json:"url"`
		Label string `json:"label"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	raw := strings.TrimSpace(body.URL)
	u, err := url.Parse(raw)
	if raw == "" || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return badRequest("L'adresse du site doit commencer par https://.")
	}
	if menusync.IsTakeawayPlatformHost(u.Host) {
		return badRequest("Ce lien pointe vers takeaway.com : ajoute le site du restaurant (bouton « Découvrir »).")
	}
	u.RawQuery, u.Fragment, u.User = "", "", nil
	u.Host = strings.ToLower(u.Host)
	if u.Path == "" {
		u.Path = "/"
	}
	siteURL := u.String()

	var rec *core.Record
	created := false
	err = e.App.RunInTransaction(func(tx core.App) error {
		existing, err := findSiteSource(tx, siteURL)
		if err != nil {
			return err
		}
		if existing != nil {
			rec = existing
			return nil
		}
		col, err := tx.FindCollectionByNameOrId(colSyncSources)
		if err != nil {
			return err
		}
		all, err := tx.FindAllRecords(colSyncSources)
		if err != nil {
			return err
		}
		used := map[int]bool{}
		for _, r := range all {
			used[r.GetInt("priority")] = true
		}
		priority := discoverPriorityMax
		for p := discoverPriorityMin; p <= discoverPriorityMax; p++ {
			if !used[p] {
				priority = p
				break
			}
		}
		label := truncate(body.Label, 120)
		if label == "" {
			label = menusync.BareHost(u.Host)
		}
		rec = core.NewRecord(col)
		rec.Load(map[string]any{
			"provider": menusync.SourceTakeawaySite, "label": label, "url": siteURL, "city": menusync.DefaultCity,
			"priority": priority, "enabled": true, "options": false,
		})
		if err := normalizeSyncSource(rec); err != nil {
			return err
		}
		created = true
		return tx.Save(rec)
	})
	if err != nil {
		return err
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return e.JSON(status, map[string]any{"source": rec, "created": created})
}
