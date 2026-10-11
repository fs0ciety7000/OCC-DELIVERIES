package app

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Persistent rate limits: the GCRA state of each key (domain.Limit) lives in
// the plain SQLite table occ_rate_limits (migration 1760000023), so limits
// survive restarts and redeployments. One atomic statement per event.

const (
	rateLimitTable       = "occ_rate_limits"
	rateLimitCleanupJob  = "occRateLimitCleanup"
	rateLimitCleanupCron = "*/15 * * * *"
)

type rateLimiter struct {
	name  string
	limit domain.Limit
}

func newRateLimiter(name string, max int, window time.Duration) rateLimiter {
	return rateLimiter{name: name, limit: domain.Limit{Max: max, Window: window}}
}

// key never stores the raw IP / identity: limiter name + truncated SHA-256.
func (l rateLimiter) key(value string) string {
	sum := sha256.Sum256([]byte(value))
	return l.name + ":" + hex.EncodeToString(sum[:16])
}

// stepSQL mirrors domain.Limit.Step: a fresh key starts at now + T; an
// existing key is allowed when max(tat, now) − now ≤ τ (then tat moves to
// max(tat, now) + T), refused otherwise (tat unchanged). SET expressions read
// the old row values.
const stepSQL = `INSERT INTO ` + rateLimitTable + ` (key, tat, ok) VALUES ({:key}, {:now} + {:t}, 1)
ON CONFLICT(key) DO UPDATE SET
	ok = (max(tat, {:now}) - {:now} <= {:tau}),
	tat = CASE WHEN max(tat, {:now}) - {:now} <= {:tau} THEN max(tat, {:now}) + {:t} ELSE tat END
RETURNING tat, ok`

// allow records an event for value at now; when refused it returns the wait
// before the next allowed event. A database error lets the event through
// (logged): a broken limiter must not lock everyone out.
func (l rateLimiter) allow(app core.App, value string, now time.Time) (bool, time.Duration) {
	var row struct {
		Tat int64 `db:"tat"`
		OK  bool  `db:"ok"`
	}
	ms := now.UnixMilli()
	err := app.DB().NewQuery(stepSQL).Bind(dbx.Params{
		"key": l.key(value),
		"now": ms,
		"t":   l.limit.IntervalMillis(),
		"tau": l.limit.ToleranceMillis(),
	}).One(&row)
	if err != nil {
		app.Logger().Warn("rate limit unavailable", "limiter", l.name, "error", err)
		return true, 0
	}
	if row.OK {
		return true, 0
	}
	_, _, retry := l.limit.Step(row.Tat, ms)
	return false, retry
}

// reset forgets a key (e.g. the identity counter after a successful login).
func (l rateLimiter) reset(app core.App, value string) {
	if _, err := app.DB().Delete(rateLimitTable, dbx.HashExp{"key": l.key(value)}).Execute(); err != nil {
		app.Logger().Warn("rate limit reset failed", "limiter", l.name, "error", err)
	}
}

// refund gives one event back (domain.Limit.Refund).
func (l rateLimiter) refund(app core.App, value string) {
	if _, err := app.DB().NewQuery(`UPDATE ` + rateLimitTable + ` SET tat = tat - {:t} WHERE key = {:key}`).Bind(dbx.Params{
		"t": l.limit.IntervalMillis(), "key": l.key(value),
	}).Execute(); err != nil {
		app.Logger().Warn("rate limit refund failed", "limiter", l.name, "error", err)
	}
}

// cleanupRateLimits deletes the keys back to their fresh state.
func cleanupRateLimits(app core.App, now time.Time) (int64, error) {
	res, err := app.DB().NewQuery(`DELETE FROM ` + rateLimitTable + ` WHERE tat <= {:now}`).Bind(dbx.Params{"now": now.UnixMilli()}).Execute()
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func bindRateLimitCleanup(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if err := se.App.Cron().Add(rateLimitCleanupJob, rateLimitCleanupCron, func() {
			if _, err := cleanupRateLimits(se.App, time.Now()); err != nil {
				se.App.Logger().Warn("rate limit cleanup failed", "error", err)
			}
		}); err != nil {
			return err
		}
		return se.Next()
	})
}

// tooManyRequests answers 429 with a Retry-After header.
func tooManyRequests(e *core.RequestEvent, retry time.Duration, msg string) error {
	e.Response.Header().Set("Retry-After", strconv.Itoa(domain.RetryAfterSeconds(retry)))
	return apis.NewTooManyRequestsError(msg, nil)
}
