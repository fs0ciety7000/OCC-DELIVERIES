package migrations

import "testing"

func TestBackupChecksMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)

	col, err := app.FindCollectionByNameOrId(BackupChecksCollection)
	if err != nil {
		t.Fatal(err)
	}
	if col.ListRule != nil || col.ViewRule != nil || col.CreateRule != nil || col.UpdateRule != nil || col.DeleteRule != nil {
		t.Fatal("backup_checks must be server only (nil rules)")
	}
	for _, f := range []string{"kind", "trigger", "ok", "backup_key", "backup_size", "backup_modified", "duration_ms", "integrity", "error", "warnings", "counts", "created"} {
		if col.Fields.GetByName(f) == nil {
			t.Fatalf("missing field %s", f)
		}
	}
	// idempotent, reversible
	if err := upBackupChecks(app); err != nil {
		t.Fatal(err)
	}
	if err := downBackupChecks(app); err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindCollectionByNameOrId(BackupChecksCollection); err == nil {
		t.Fatal("down must drop the collection")
	}
	if err := downBackupChecks(app); err != nil {
		t.Fatal(err)
	}
	if err := upBackupChecks(app); err != nil {
		t.Fatal(err)
	}
}
