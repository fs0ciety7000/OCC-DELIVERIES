package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Europe/Brussels in the alpine image

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/cron"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/feedsync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
	"github.com/fs0ciety7000/occ-deliveries/backend/migrations"
)

// Collections of the synchronisation.
const (
	colSyncSources = "sync_sources"
	colSyncRuns    = "sync_runs"
)

// sync_runs.status / trigger values.
const (
	runRunning = "running"
	runSuccess = "success"
	runPartial = "partial"
	runFailed  = "failed"
	runBlocked = "blocked"

	triggerCron    = "cron"
	triggerManual  = "manual"
	triggerStartup = "startup"
)

const (
	// DefaultSyncCron is OCC_SYNC_CRON's default: every night at 03:30 (Brussels).
	DefaultSyncCron = "30 3 * * *"
	syncTimezone    = "Europe/Brussels"
	syncRadiusKm    = 8
	maxRunChanges   = 500
	maxRunLog       = 150_000
	syncJobID       = "occ_menu_sync"
)

// SyncConfig drives the automatic synchronisation (OCC_SYNC_*).
type SyncConfig struct {
	Enabled    bool          // OCC_SYNC_ENABLED (default true)
	Cron       string        // OCC_SYNC_CRON, read in Europe/Brussels time
	OnStart    bool          // OCC_SYNC_ON_START: first sync after start when none succeeded yet
	StartDelay time.Duration // OCC_SYNC_START_DELAY (default 60s)
	CacheDir   string        // default <pb_data>/menusync-cache
	CacheTTL   time.Duration // default 20h
	// NewFetcher builds the HTTP fetcher of a run (tests replace the pauses).
	NewFetcher func(cacheDir string) *menusync.Fetcher
	// Snapshot returns the Uber Eats snapshot read by the "ubereats-snapshot"
	// sources (default: the embedded migrations/data/mons_ubereats.json).
	Snapshot func() ([]byte, error)
	// DefaultLat / DefaultLng place the snapshot restaurants without
	// coordinates (OCC_DEFAULT_LAT / LNG).
	DefaultLat, DefaultLng float64
}

func syncConfigFromEnv() SyncConfig {
	c := SyncConfig{
		Enabled:    envBool("OCC_SYNC_ENABLED", true),
		Cron:       envOr("OCC_SYNC_CRON", DefaultSyncCron),
		OnStart:    envBool("OCC_SYNC_ON_START", true),
		StartDelay: 60 * time.Second,
		CacheTTL:   20 * time.Hour,
	}
	if d, err := time.ParseDuration(strings.TrimSpace(envOr("OCC_SYNC_START_DELAY", ""))); err == nil && d >= 0 {
		c.StartDelay = d
	}
	return c
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(envOr(key, ""))) {
	case "1", "true", "yes", "oui", "on":
		return true
	case "0", "false", "no", "non", "off":
		return false
	}
	return def
}

var (
	errSyncRunning  = errors.New("une synchronisation est déjà en cours")
	errSyncDisabled = errors.New("synchronisation désactivée (OCC_SYNC_ENABLED=false)")
)

// syncer runs the synchronisation: one run at a time, in the background.
type syncer struct {
	app   core.App
	cfg   SyncConfig
	loc   *time.Location
	sched *cron.Schedule

	mu        sync.Mutex
	runningID string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newSyncer(app core.App, cfg SyncConfig) *syncer {
	loc, err := time.LoadLocation(syncTimezone)
	if err != nil {
		loc = time.UTC
	}
	if strings.TrimSpace(cfg.Cron) == "" {
		cfg.Cron = DefaultSyncCron
	}
	sched, err := cron.NewSchedule(cfg.Cron)
	if err != nil {
		app.Logger().Error("OCC_SYNC_CRON invalide, valeur par défaut utilisée", "cron", cfg.Cron, "error", err)
		cfg.Cron = DefaultSyncCron
		sched, _ = cron.NewSchedule(cfg.Cron)
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 20 * time.Hour
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &syncer{app: app, cfg: cfg, loc: loc, sched: sched, ctx: ctx, cancel: cancel}
}

// bind registers the cron job, the startup run and the shutdown hook.
func (s *syncer) bind() {
	s.app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		s.app = se.App
		// a run cannot survive a restart
		if _, err := se.App.DB().Update(colSyncRuns, dbx.Params{
			"status": runFailed, "error": "interrompue (redémarrage du serveur)",
			"finished_at": types.NowDateTime().String(),
		}, dbx.HashExp{"status": runRunning}).Execute(); err != nil {
			se.App.Logger().Warn("sync: nettoyage des exécutions interrompues", "error", err)
		}
		if s.cfg.Enabled {
			// PocketBase's cron runs in UTC: the job ticks every minute and
			// checks our own schedule in Brussels time.
			if err := se.App.Cron().Add(syncJobID, "* * * * *", func() { s.tick(time.Now()) }); err != nil {
				return err
			}
			if s.cfg.OnStart {
				s.wg.Add(1)
				go func() {
					defer s.wg.Done()
					s.startupRun()
				}()
			}
		}
		return se.Next()
	})
	s.app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		s.stop()
		return e.Next()
	})
}

func (s *syncer) stop() {
	s.cancel()
	s.wg.Wait()
}

// running returns the id of the run in progress ("" if none).
func (s *syncer) running() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runningID
}

// tick starts a cron run when the schedule is due at t (Brussels time).
func (s *syncer) tick(t time.Time) bool {
	if !s.cfg.Enabled || !s.sched.IsDue(cron.NewMoment(t.In(s.loc))) {
		return false
	}
	if _, err := s.start(triggerCron); err != nil {
		s.app.Logger().Info("sync: exécution planifiée ignorée", "reason", err.Error())
		return false
	}
	return true
}

func (s *syncer) startupRun() {
	select {
	case <-s.ctx.Done():
		return
	case <-time.After(s.cfg.StartDelay):
	}
	n, err := s.app.CountRecords(colSyncRuns, dbx.In("status", runSuccess, runPartial))
	if err != nil || n > 0 {
		return
	}
	if _, err := s.start(triggerStartup); err != nil {
		s.app.Logger().Info("sync: première synchronisation ignorée", "reason", err.Error())
	}
}

// nextRun is the next scheduled start (nil when disabled).
func (s *syncer) nextRun(from time.Time) *time.Time {
	if !s.cfg.Enabled {
		return nil
	}
	t := from.In(s.loc).Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 60*24*8; i++ {
		if s.sched.IsDue(cron.NewMoment(t)) {
			return &t
		}
		t = t.Add(time.Minute)
	}
	return nil
}

// begin takes the lock and records a "running" run.
func (s *syncer) begin(trigger string) (*core.Record, error) {
	if !s.cfg.Enabled {
		return nil, errSyncDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runningID != "" {
		return nil, errSyncRunning
	}
	if s.ctx.Err() != nil {
		return nil, errors.New("serveur en cours d'arrêt")
	}
	col, err := s.app.FindCollectionByNameOrId(colSyncRuns)
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(col)
	rec.Load(map[string]any{
		"status": runRunning, "trigger": trigger, "started_at": types.NowDateTime(),
		"stats": feedsync.Stats{}, "changes": []string{}, "sources": []feedsync.SourceResult{},
	})
	if err := s.app.Save(rec); err != nil {
		return nil, err
	}
	s.runningID = rec.Id
	return rec, nil
}

// start launches a run in the background.
func (s *syncer) start(trigger string) (*core.Record, error) {
	rec, err := s.begin(trigger)
	if err != nil {
		return nil, err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.execute(s.ctx, rec)
	}()
	return rec, nil
}

// runLog is the capped journal of a run.
type runLog struct {
	mu        sync.Mutex
	b         strings.Builder
	truncated bool
}

func (l *runLog) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	line := fmt.Sprintf(format, args...)
	if l.b.Len()+len(line)+1 > maxRunLog {
		if !l.truncated {
			l.b.WriteString("… (journal tronqué)\n")
			l.truncated = true
		}
		return
	}
	l.b.WriteString(line)
	l.b.WriteByte('\n')
}

func (l *runLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func (s *syncer) newFetcher() *menusync.Fetcher {
	dir := s.cfg.CacheDir
	if dir == "" {
		dir = filepath.Join(s.app.DataDir(), "menusync-cache")
	}
	var f *menusync.Fetcher
	if s.cfg.NewFetcher != nil {
		f = s.cfg.NewFetcher(dir)
	} else {
		f = menusync.NewFetcher(dir)
	}
	if f.CacheTTL == 0 {
		f.CacheTTL = s.cfg.CacheTTL
	}
	return f
}

// execute performs a run and records its outcome. It always releases the lock.
func (s *syncer) execute(ctx context.Context, rec *core.Record) {
	app := s.app
	start := time.Now()
	lg := &runLog{}
	var stats feedsync.Stats
	var changes []string
	var results []feedsync.SourceResult
	status, errMsg := runFailed, ""

	// live journal: flushed every few seconds while the run goes on
	flushDone := make(chan struct{})
	flushStopped := make(chan struct{})
	go func() {
		defer close(flushStopped)
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-flushDone:
				return
			case <-t.C:
				_, _ = app.DB().Update(colSyncRuns, dbx.Params{"log": lg.String()}, dbx.HashExp{"id": rec.Id}).Execute()
			}
		}
	}()

	defer func() {
		close(flushDone)
		<-flushStopped
		if r := recover(); r != nil {
			status, errMsg = runFailed, fmt.Sprintf("erreur interne : %v", r)
		}
		if len(changes) > maxRunChanges {
			extra := len(changes) - maxRunChanges
			changes = append(changes[:maxRunChanges], fmt.Sprintf("… et %d autres changements", extra))
		}
		if changes == nil {
			changes = []string{}
		}
		if results == nil {
			results = []feedsync.SourceResult{}
		}
		rec.Load(map[string]any{
			"status": status, "finished_at": types.NowDateTime(), "stats": stats, "changes": changes,
			"sources": results, "log": lg.String(), "error": truncate(errMsg, 4000),
		})
		if err := app.Save(rec); err != nil {
			app.Logger().Error("sync: enregistrement de l'exécution", "error", err)
		}
		app.Logger().Info("sync: terminé", slog.String("status", status), slog.Duration("duration", time.Since(start).Round(time.Second)),
			slog.Int("restaurants_created", stats.RestaurantsCreated), slog.Int("restaurants_updated", stats.RestaurantsUpdated),
			slog.Int("items_created", stats.ItemsCreated), slog.Int("items_updated", stats.ItemsUpdated))
		s.mu.Lock()
		s.runningID = ""
		s.mu.Unlock()
	}()

	srcRecs, err := app.FindRecordsByFilter(colSyncSources, "enabled = true", "priority,label", 0, 0)
	if err != nil {
		errMsg = err.Error()
		return
	}
	if len(srcRecs) == 0 {
		errMsg = "Aucune source activée."
		return
	}
	var sources []feedsync.Source
	city := ""
	for _, r := range srcRecs {
		sources = append(sources, feedsync.Source{
			ID: r.Id, Provider: r.GetString("provider"), Label: cmpOr(r.GetString("label"), r.GetString("provider")),
			URL: r.GetString("url"), City: r.GetString("city"), Priority: r.GetInt("priority"), Options: r.GetBool("options"),
		})
		if city == "" {
			city = strings.ToLower(strings.TrimSpace(r.GetString("city")))
		}
	}
	city = cmpOr(city, "mons")
	lg.logf("Synchronisation (%s) : %d sources", rec.GetString("trigger"), len(sources))

	netSources, snapSources := feedsync.SplitSources(sources)
	f := s.newFetcher()
	f.Logf = lg.logf
	feed, res := feedsync.FetchAll(ctx, f, netSources, feedsync.FetchOptions{RadiusKm: syncRadiusKm, Today: menusync.Today(), Logf: lg.logf})
	results = res
	// Uber Eats snapshot: embedded file, no network
	var snapshot []feedsync.SnapshotEntry
	if len(snapSources) > 0 {
		data, err := s.snapshotData()
		for _, src := range snapSources {
			var entries []feedsync.SnapshotEntry
			var r feedsync.SourceResult
			if err != nil {
				r = feedsync.SourceResult{ID: src.ID, Label: src.Label, Provider: src.Provider, Status: feedsync.StatusFailed, Message: err.Error()}
			} else {
				entries, r = feedsync.ReadSnapshot(src, data, lg.logf)
			}
			results = append(results, r)
			if snapshot == nil {
				snapshot = entries // several rows read the same file: once is enough
			}
		}
	}
	s.saveSourceStatus(results)
	lg.logf("Requêtes réseau : %d, depuis le cache : %d", f.Network, f.Cached)
	if ctx.Err() != nil {
		errMsg = "interrompue (arrêt du serveur)"
		return
	}

	merged, mstats := menusync.MergeIn(feed, feedsync.Priority(netSources), city)
	for _, g := range mstats.Merged {
		lg.logf("fusionné : %s", strings.Join(g.Sources, " | "))
	}
	lg.logf("Fusion : %d fiches → %d restaurants", mstats.In, mstats.Out)

	incomplete := map[string]bool{}
	ok, blocked := 0, 0
	for _, r := range results {
		switch r.Status {
		case feedsync.StatusOK:
			ok++
		case feedsync.StatusBlocked:
			blocked++
			incomplete[r.Provider] = true
		default:
			incomplete[r.Provider] = true
		}
	}

	stored, err := loadSnapshot(app)
	if err != nil {
		errMsg = err.Error()
		return
	}
	plan := feedsync.Reconcile(stored, merged, feedsync.Options{
		Now: time.Now(), City: city, Incomplete: incomplete,
		Snapshot: snapshot, DefaultLat: s.cfg.DefaultLat, DefaultLng: s.cfg.DefaultLng,
	})
	for _, l := range plan.Log {
		lg.logf("%s", l)
	}
	failures := 0
	for _, rp := range plan.Restaurants {
		if ctx.Err() != nil {
			errMsg = "interrompue (arrêt du serveur)"
			break
		}
		if err := applyRestaurantPlan(app, rp); err != nil {
			failures++
			lg.logf("! %s : %v", rp.Restaurant.Name, err)
			continue
		}
		stats.Add(rp.Stats)
		changes = append(changes, rp.Changes...)
	}
	if len(snapshot) > 0 {
		lg.logf("Uber Eats (instantané) : %d restaurants, %d reconnus", len(snapshot), plan.SnapshotMatched)
	}
	lg.logf("Restaurants : %d reconnus (%d verrouillés), %d créés, %d mis à jour, %d obsolètes ; plats : %d créés, %d mis à jour (%d prix), %d indisponibles",
		plan.Matched, plan.Locked, stats.RestaurantsCreated, stats.RestaurantsUpdated, stats.RestaurantsStale,
		stats.ItemsCreated, stats.ItemsUpdated, stats.ItemsPriceChanged, stats.ItemsUnavailable)

	switch {
	case ok == 0 && blocked > 0:
		status = runBlocked
		errMsg = cmpOr(errMsg, "Toutes les sources ont refusé l'accès.")
	case ok == 0:
		status = runFailed
		errMsg = cmpOr(errMsg, "Aucune source n'a pu être lue.")
	case ok < len(results) || failures > 0 || errMsg != "":
		status = runPartial
		if failures > 0 {
			errMsg = cmpOr(errMsg, fmt.Sprintf("%d restaurant(s) non enregistré(s), voir le journal.", failures))
		}
	default:
		status = runSuccess
	}
}

func cmpOr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

// snapshotData returns the Uber Eats snapshot file.
func (s *syncer) snapshotData() ([]byte, error) {
	if s.cfg.Snapshot != nil {
		return s.cfg.Snapshot()
	}
	return migrations.UberEatsSnapshot()
}

func (s *syncer) saveSourceStatus(results []feedsync.SourceResult) {
	for _, r := range results {
		rec, err := s.app.FindRecordById(colSyncSources, r.ID)
		if err != nil {
			continue
		}
		rec.Set("last_run_at", types.NowDateTime())
		rec.Set("last_status", truncate(r.Status+": "+r.Message, 1000))
		if err := s.app.Save(rec); err != nil {
			s.app.Logger().Warn("sync: statut de la source", "source", r.Label, "error", err)
		}
	}
}

// ---------------------------------------------------------------- snapshot

func jsonList[T any](r *core.Record, field string) []T {
	var out []T
	if raw := strings.TrimSpace(r.GetString(field)); raw != "" && raw != "null" {
		_ = r.UnmarshalJSONField(field, &out)
	}
	return out
}

func loadSnapshot(app core.App) ([]*feedsync.Restaurant, error) {
	rests, err := app.FindAllRecords(colRestaurants)
	if err != nil {
		return nil, err
	}
	byID := map[string]*feedsync.Restaurant{}
	out := make([]*feedsync.Restaurant, 0, len(rests))
	for _, r := range rests {
		fr := &feedsync.Restaurant{
			ID: r.Id, SourceKey: r.GetString("source_key"), Slug: r.GetString("slug"), Name: r.GetString("name"),
			Description: r.GetString("description"), Emoji: r.GetString("emoji"), CoverURL: r.GetString("cover_url"),
			Address: r.GetString("address"), Phone: r.GetString("phone"), Cuisines: jsonList[string](r, "cuisines"),
			Lat: r.GetFloat("lat"), Lng: r.GetFloat("lng"), Rating: r.GetFloat("rating"), RatingCount: r.GetInt("rating_count"),
			PriceLevel: r.GetInt("price_level"), EtaMin: r.GetInt("eta_min"), EtaMax: r.GetInt("eta_max"),
			DeliveryFee: r.GetInt("delivery_fee"), MinOrder: r.GetInt("min_order"),
			Providers: jsonList[providers.Link](r, "providers"), Sources: jsonList[feedsync.SourceRef](r, "sources"),
			Locked: r.GetBool("locked"), Active: r.GetBool("active"), StaleSince: r.GetString("stale_since"),
			PartialMenu: r.GetBool("partial_menu"), GeoApprox: r.GetBool("geo_approx"),
		}
		byID[r.Id] = fr
		out = append(out, fr)
	}
	cats, err := app.FindAllRecords(colMenuCategories)
	if err != nil {
		return nil, err
	}
	for _, c := range cats {
		if fr := byID[c.GetString("restaurant")]; fr != nil {
			fr.Categories = append(fr.Categories, &feedsync.Category{ID: c.Id, Name: c.GetString("name"), Position: c.GetInt("position")})
		}
	}
	items, err := app.FindAllRecords(colMenuItems)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		fr := byID[it.GetString("restaurant")]
		if fr == nil {
			continue
		}
		fr.Items = append(fr.Items, &feedsync.Item{
			ID: it.Id, SourceKey: it.GetString("source_key"), CategoryID: it.GetString("category"), Name: it.GetString("name"),
			Description: it.GetString("description"), Price: it.GetInt("price"),
			OptionGroups: jsonList[domain.OptionGroup](it, "option_groups"), Popular: it.GetBool("popular"),
			Available: it.GetBool("available"), Locked: it.GetBool("locked"), Position: it.GetInt("position"),
			Sources: jsonList[feedsync.SourceRef](it, "sources"),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------- apply

func orEmptyList[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func httpURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		return u
	}
	return ""
}

// applyRestaurantPlan writes one restaurant and its menu changes in a
// transaction. Records locked meanwhile by an admin are left alone.
func applyRestaurantPlan(app core.App, rp *feedsync.RestaurantPlan) error {
	r := rp.Restaurant
	return app.RunInTransaction(func(tx core.App) error {
		var rec *core.Record
		if r.ID == "" {
			col, err := tx.FindCollectionByNameOrId(colRestaurants)
			if err != nil {
				return err
			}
			rec = core.NewRecord(col)
			slug := r.Slug
			for n := 2; ; n++ {
				if _, err := tx.FindFirstRecordByData(colRestaurants, "slug", slug); err != nil {
					break
				}
				slug = fmt.Sprintf("%s-%d", r.Slug, n)
			}
			rec.Set("slug", slug)
			rec.Set("active", true)
		} else {
			var err error
			if rec, err = tx.FindRecordById(colRestaurants, r.ID); err != nil {
				return err
			}
			if rec.GetBool("locked") {
				return nil // locked since the snapshot
			}
		}
		if rp.Save || rec.IsNew() {
			stale := any("")
			if r.StaleSince != "" {
				stale = r.StaleSince
			}
			rec.Load(map[string]any{
				"name":         truncate(r.Name, 120),
				"description":  truncate(r.Description, 2000),
				"emoji":        truncate(r.Emoji, 16),
				"cover_url":    httpURL(r.CoverURL),
				"cuisines":     orEmptyList(r.Cuisines),
				"address":      truncate(r.Address, 300),
				"lat":          r.Lat,
				"lng":          r.Lng,
				"phone":        truncate(r.Phone, 40),
				"rating":       min(max(r.Rating, 0), 5),
				"rating_count": max(r.RatingCount, 0),
				"price_level":  min(max(r.PriceLevel, 1), 4),
				"eta_min":      max(r.EtaMin, 0),
				"eta_max":      max(r.EtaMax, 0),
				"delivery_fee": max(r.DeliveryFee, 0),
				"min_order":    max(r.MinOrder, 0),
				"providers":    orEmptyList(r.Providers),
				"source_key":   truncate(r.SourceKey, 300),
				"sources":      orEmptyList(r.Sources),
				"stale_since":  stale,
				"partial_menu": r.PartialMenu,
				"geo_approx":   r.GeoApprox,
			})
			if err := tx.Save(rec); err != nil {
				return err
			}
		}

		catIDs := map[string]string{}
		if len(rp.NewCategories) > 0 {
			catCol, err := tx.FindCollectionByNameOrId(colMenuCategories)
			if err != nil {
				return err
			}
			for _, c := range rp.NewCategories {
				cr := core.NewRecord(catCol)
				cr.Load(map[string]any{"restaurant": rec.Id, "name": truncate(c.Name, 120), "position": c.Position})
				if err := tx.Save(cr); err != nil {
					return err
				}
				catIDs[c.Name] = cr.Id
			}
		}
		if len(rp.Items) == 0 {
			return nil
		}
		itemCol, err := tx.FindCollectionByNameOrId(colMenuItems)
		if err != nil {
			return err
		}
		for _, it := range rp.Items {
			var ir *core.Record
			if it.ID == "" {
				ir = core.NewRecord(itemCol)
				ir.Set("restaurant", rec.Id)
				ir.Set("position", it.Position)
				ir.Set("tags", []string{})
			} else {
				if ir, err = tx.FindRecordById(colMenuItems, it.ID); err != nil {
					return err
				}
				if ir.GetBool("locked") || ir.GetString("restaurant") != rec.Id {
					continue
				}
			}
			cat := it.CategoryID
			if cat == "" {
				cat = catIDs[it.CategoryName]
			}
			ir.Load(map[string]any{
				"category":      cat,
				"name":          truncate(it.Name, 160),
				"description":   truncate(it.Description, 2000),
				"price":         max(it.Price, 0),
				"option_groups": orEmptyList(it.OptionGroups),
				"popular":       it.Popular,
				"available":     it.Available,
				"source_key":    truncate(it.SourceKey, 300),
				"sources":       orEmptyList(it.Sources),
			})
			if err := tx.Save(ir); err != nil {
				return fmt.Errorf("%s : %w", it.Name, err)
			}
		}
		return nil
	})
}
