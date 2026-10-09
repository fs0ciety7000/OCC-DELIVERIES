package app

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/feedsync"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
)

// syncRun is the JSON shape of a sync_runs record (docs/ARCHITECTURE.md).
type syncRun struct {
	ID           string                  `json:"id"`
	StartedAt    string                  `json:"started_at"`
	FinishedAt   string                  `json:"finished_at"`
	Status       string                  `json:"status"`
	Trigger      string                  `json:"trigger"`
	Stats        feedsync.Stats          `json:"stats"`
	Sources      []feedsync.SourceResult `json:"sources"`
	ChangesCount int                     `json:"changesCount"`
	Changes      []string                `json:"changes,omitempty"`
	Log          string                  `json:"log,omitempty"`
	LogTail      string                  `json:"logTail,omitempty"`
	Error        string                  `json:"error"`
}

func toSyncRun(r *core.Record, full bool) syncRun {
	out := syncRun{
		ID: r.Id, StartedAt: r.GetString("started_at"), FinishedAt: r.GetString("finished_at"),
		Status: r.GetString("status"), Trigger: r.GetString("trigger"), Error: r.GetString("error"),
		Sources: orEmptyList(jsonList[feedsync.SourceResult](r, "sources")),
	}
	_ = r.UnmarshalJSONField("stats", &out.Stats)
	changes := jsonList[string](r, "changes")
	out.ChangesCount = len(changes)
	if full {
		out.Changes = orEmptyList(changes)
		out.Log = r.GetString("log")
	}
	return out
}

func syncError(err error) error {
	switch {
	case errors.Is(err, errSyncRunning):
		return apis.NewApiError(http.StatusConflict, "Une synchronisation est déjà en cours.", nil)
	case errors.Is(err, errSyncDisabled):
		return badRequest("La synchronisation automatique est désactivée (OCC_SYNC_ENABLED=false).")
	}
	return err
}

// GET /api/occ/admin/sync/status
func (h *handlers) adminSyncStatus(e *core.RequestEvent) error {
	s := h.sync
	out := map[string]any{
		"enabled": s.cfg.Enabled, "cron": s.cfg.Cron, "timezone": s.loc.String(),
		"running": nil, "lastRun": nil, "nextRunAt": nil,
	}
	if next := s.nextRun(time.Now()); next != nil {
		out["nextRunAt"] = next.UTC().Format(time.RFC3339)
	}
	if id := s.running(); id != "" {
		if r, err := e.App.FindRecordById(colSyncRuns, id); err == nil {
			run := toSyncRun(r, false)
			run.LogTail = logTail(r.GetString("log"), 12)
			out["running"] = run
		}
	}
	if last, err := e.App.FindRecordsByFilter(colSyncRuns, "status != 'running'", "-started_at", 1, 0); err == nil && len(last) > 0 {
		out["lastRun"] = toSyncRun(last[0], false)
	}
	return ok(e, out)
}

// logTail returns the last n lines of a journal.
func logTail(log string, n int) string {
	lines := strings.Split(strings.TrimRight(log, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// GET /api/occ/admin/sync/runs
func (h *handlers) adminSyncRuns(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	page = max(page, 1)
	perPage, _ := strconv.Atoi(q.Get("perPage"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	total, err := e.App.CountRecords(colSyncRuns)
	if err != nil {
		return err
	}
	recs, err := e.App.FindRecordsByFilter(colSyncRuns, "id != ''", "-started_at", perPage, (page-1)*perPage)
	if err != nil {
		return err
	}
	items := make([]syncRun, 0, len(recs))
	for _, r := range recs {
		items = append(items, toSyncRun(r, false))
	}
	return ok(e, map[string]any{"page": page, "perPage": perPage, "totalItems": total, "items": items})
}

// GET /api/occ/admin/sync/runs/{id}
func (h *handlers) adminSyncRun(e *core.RequestEvent) error {
	r, err := e.App.FindRecordById(colSyncRuns, e.Request.PathValue("id"))
	if err != nil {
		return notFound("Synchronisation introuvable.")
	}
	return ok(e, map[string]any{"run": toSyncRun(r, true)})
}

// POST /api/occ/admin/sync/run
func (h *handlers) adminSyncStart(e *core.RequestEvent) error {
	rec, err := h.sync.start(triggerManual)
	if err != nil {
		return syncError(err)
	}
	return e.JSON(http.StatusAccepted, map[string]any{"run": toSyncRun(rec, false)})
}

// ---------------------------------------------------------------- sources

func bindSyncHooks(app core.App) {
	app.OnRecordCreateRequest(colSyncSources).BindFunc(onSyncSourceUpsert)
	app.OnRecordUpdateRequest(colSyncSources).BindFunc(onSyncSourceUpsert)
}

// onSyncSourceUpsert validates a source written by an admin. last_run_at /
// last_status are written by the server only.
func onSyncSourceUpsert(e *core.RecordRequestEvent) error {
	r := e.Record
	provider := r.GetString("provider")
	if !slices.Contains(feedsync.Providers, provider) {
		return badRequest("Fournisseur inconnu (deliveroo, weloveat, takeaway-site, jsonld).")
	}
	r.Set("label", strings.TrimSpace(r.GetString("label")))
	if r.GetString("label") == "" {
		r.Set("label", provider)
	}
	u := strings.TrimSpace(r.GetString("url"))
	r.Set("url", u)
	if u == "" && provider != menusync.SourceWeloveat {
		return badRequest("L'adresse (URL) de la source est requise.")
	}
	if u != "" {
		pu, err := url.Parse(u)
		if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" {
			return badRequest("L'adresse de la source doit commencer par https://.")
		}
	}
	city := strings.ToLower(strings.TrimSpace(r.GetString("city")))
	if city == "" {
		city = "mons"
	}
	if _, ok := menusync.Cities[city]; !ok {
		return badRequest("Ville inconnue : seules ces villes sont gérées : mons.")
	}
	r.Set("city", city)
	if r.IsNew() {
		r.Set("last_run_at", "")
		r.Set("last_status", "")
	} else {
		r.Set("last_run_at", r.Original().Get("last_run_at"))
		r.Set("last_status", r.Original().Get("last_status"))
	}
	return e.Next()
}
