package feedsync

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
)

// Providers a sync source can use (sync_sources.provider).
var Providers = []string{menusync.SourceDeliveroo, menusync.SourceWeloveat, menusync.SourceTakeawaySite, menusync.SourceJSONLD}

// Source is an enabled sync_sources row.
type Source struct {
	ID       string
	Provider string
	Label    string
	URL      string // listing page (deliveroo), API root (weloveat), site (takeaway-site / jsonld)
	City     string
	Priority int  // lower = preferred menu
	Options  bool // extra requests for the options (weloveat supplements)
}

// Source statuses (sync_sources.last_status prefix, SourceResult.Status).
const (
	StatusOK      = "ok"
	StatusBlocked = "blocked"
	StatusFailed  = "failed"
)

// SourceResult is the outcome of one source in a run.
type SourceResult struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Provider    string `json:"provider"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Restaurants int    `json:"restaurants"`
	Network     int    `json:"network"`
	Cached      int    `json:"cached"`
	DurationMs  int64  `json:"durationMs"`
}

// FetchOptions tune FetchAll.
type FetchOptions struct {
	RadiusKm float64
	Today    string
	Logf     func(format string, args ...any)
}

func (o FetchOptions) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

// SortSources orders sources by priority (then label) — the fetch order and
// the menu preference.
func SortSources(srcs []Source) {
	slices.SortStableFunc(srcs, func(a, b Source) int {
		if a.Priority != b.Priority {
			return a.Priority - b.Priority
		}
		return strings.Compare(a.Label, b.Label)
	})
}

// Priority returns the menusync source priority implied by the sources
// (each provider ranked by its best row).
func Priority(srcs []Source) []string {
	sorted := slices.Clone(srcs)
	SortSources(sorted)
	var out []string
	for _, s := range sorted {
		if !slices.Contains(out, s.Provider) {
			out = append(out, s.Provider)
		}
	}
	for _, p := range menusync.DefaultPriority {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// FetchAll reads every source sequentially through one polite Fetcher (one
// request at a time, robots.txt, delays, cache). A blocked or failing source
// is reported and the next one is read. It stops early only when ctx ends.
func FetchAll(ctx context.Context, f *menusync.Fetcher, srcs []Source, o FetchOptions) ([]menusync.Restaurant, []SourceResult) {
	sorted := slices.Clone(srcs)
	SortSources(sorted)
	var all []menusync.Restaurant
	var results []SourceResult
	for _, s := range sorted {
		if ctx.Err() != nil {
			results = append(results, SourceResult{ID: s.ID, Label: s.Label, Provider: s.Provider, URL: s.URL, Status: StatusFailed, Message: "interrompu"})
			continue
		}
		start, net0, cache0 := time.Now(), f.Network, f.Cached
		res := SourceResult{ID: s.ID, Label: s.Label, Provider: s.Provider, URL: s.URL}
		o.logf("== %s (%s)", s.Label, s.Provider)
		list, err := fetchOne(ctx, f, s, o)
		res.Restaurants = len(list)
		switch {
		case menusync.IsBlocked(err):
			res.Status, res.Message = StatusBlocked, err.Error()
		case err != nil:
			res.Status, res.Message = StatusFailed, err.Error()
		case len(list) == 0:
			res.Status, res.Message = StatusFailed, "aucun restaurant lu"
		default:
			res.Status, res.Message = StatusOK, fmt.Sprintf("%d restaurant", len(list))
			if len(list) > 1 {
				res.Message += "s"
			}
		}
		res.Network, res.Cached = f.Network-net0, f.Cached-cache0
		res.DurationMs = time.Since(start).Milliseconds()
		o.logf("== %s : %s — %s (%d requêtes, %d depuis le cache)", s.Label, res.Status, res.Message, res.Network, res.Cached)
		all = append(all, list...)
		results = append(results, res)
	}
	return all, results
}

func fetchOne(ctx context.Context, f *menusync.Fetcher, s Source, o FetchOptions) ([]menusync.Restaurant, error) {
	city := strings.ToLower(strings.TrimSpace(s.City))
	if city == "" {
		city = "mons"
	}
	center, ok := menusync.Cities[city]
	if !ok {
		return nil, fmt.Errorf("ville %q inconnue", s.City)
	}
	opts := menusync.Options{
		City: city, Center: center, RadiusKm: o.RadiusKm, Details: s.Options, Today: o.Today, Logf: o.Logf,
	}
	switch s.Provider {
	case menusync.SourceDeliveroo:
		opts.ListingURL = strings.TrimSpace(s.URL)
	case menusync.SourceWeloveat:
		opts.APIBase = strings.TrimSpace(s.URL)
	case menusync.SourceTakeawaySite, menusync.SourceJSONLD:
		if strings.TrimSpace(s.URL) == "" {
			return nil, errors.New("URL du site manquante")
		}
		opts.URLs = []string{strings.TrimSpace(s.URL)}
	default:
		return nil, fmt.Errorf("fournisseur inconnu %q", s.Provider)
	}
	return menusync.Run(ctx, f, s.Provider, opts)
}
