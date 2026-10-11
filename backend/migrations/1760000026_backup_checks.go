package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// backup_checks: journal of the automatic backups (app/backup.go). One row per
// backup attempt (`backup`), per restore verification (`verify`: the latest
// backup downloaded, unzipped, PRAGMA integrity_check / quick_check, expected
// tables and plausible counts) and per « no backup for 36 h » e-mail
// (`alert`, at most one per day). Written by the server only: every rule is
// nil (superusers in /_/), admins read it through /api/occ/admin/backups.
// Idempotent.

func init() {
	m.Register(upBackupChecks, downBackupChecks)
}

// BackupChecksCollection is the collection created by this migration.
const BackupChecksCollection = "backup_checks"

func upBackupChecks(app core.App) error {
	if _, err := app.FindCollectionByNameOrId(BackupChecksCollection); err == nil {
		return nil
	}
	c := core.NewBaseCollection(BackupChecksCollection) // all rules nil: server only
	c.Fields.Add(
		&core.SelectField{Name: "kind", Required: true, MaxSelect: 1, Values: []string{"backup", "verify", "alert"}},
		&core.SelectField{Name: "trigger", MaxSelect: 1, Values: []string{"cron", "manual", "backup"}},
		&core.BoolField{Name: "ok"},
		&core.TextField{Name: "backup_key", Max: 500},
		&core.NumberField{Name: "backup_size", OnlyInt: true},
		&core.DateField{Name: "backup_modified"},
		&core.NumberField{Name: "duration_ms", OnlyInt: true},
		&core.TextField{Name: "integrity", Max: 2000},
		&core.TextField{Name: "error", Max: 4000},
		&core.JSONField{Name: "warnings", MaxSize: 16 << 10},
		&core.JSONField{Name: "counts", MaxSize: 16 << 10},
	)
	autodates(c)
	c.AddIndex("idx_backup_checks_kind_created", false, "kind, created", "")
	return app.Save(c)
}

func downBackupChecks(app core.App) error {
	c, err := app.FindCollectionByNameOrId(BackupChecksCollection)
	if err != nil {
		return nil
	}
	return app.Delete(c)
}
