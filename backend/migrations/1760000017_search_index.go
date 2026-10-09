package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/search"
)

// Global search (GET /api/occ/search): SQLite FTS5 tables of the restaurants
// (name, cuisines, address) and menu items (name, description, category,
// restaurant name), filled with text folded in Go (case, accents), plus
// their fts5vocab views (typo corrections). Not PocketBase collections: plain
// tables maintained by internal/search (record hooks + Reindex after
// import / synchronisation). The whole catalogue is indexed here.

func init() {
	m.Register(upSearchIndex, downSearchIndex)
}

func upSearchIndex(app core.App) error {
	return search.CreateSchema(app)
}

func downSearchIndex(app core.App) error {
	return search.DropSchema(app)
}
