package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Persistent rate limits (app/ratelimit.go, domain.Limit): one row per
// limited key (limiter name + SHA-256 of the IP / identity, never the raw
// value) holding its GCRA state, the theoretical arrival time in unix
// milliseconds. A plain SQL table, not a PocketBase collection: no API, no
// rules, only the server reads and writes it. Rows whose tat is in the past
// are equivalent to fresh keys and purged by a cron job. Idempotent.

func init() {
	m.Register(upRateLimits, downRateLimits)
}

func upRateLimits(app core.App) error {
	if _, err := app.DB().NewQuery(`CREATE TABLE IF NOT EXISTS occ_rate_limits (
		key TEXT PRIMARY KEY NOT NULL,
		tat INTEGER NOT NULL,
		ok  INTEGER NOT NULL DEFAULT 0
	) WITHOUT ROWID`).Execute(); err != nil {
		return err
	}
	_, err := app.DB().NewQuery(`CREATE INDEX IF NOT EXISTS idx_occ_rate_limits_tat ON occ_rate_limits (tat)`).Execute()
	return err
}

func downRateLimits(app core.App) error {
	_, err := app.DB().NewQuery(`DROP TABLE IF EXISTS occ_rate_limits`).Execute()
	return err
}
