package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Backups (app/backup.go): which archive is the latest, which ones the
// retention removes, whether the backups are stale, whether a restored
// database looks like the live one. Pure functions, no PocketBase.

// BackupMaxAge: without a successful backup for that long the admin panel
// shows a danger badge and the admins get (at most) one e-mail per day.
const BackupMaxAge = 36 * time.Hour

// BackupAlertEvery is the minimum delay between two « no backup » e-mails.
const BackupAlertEvery = 24 * time.Hour

// BackupObject is an archive of the backups storage (S3 bucket or pb_data/backups).
type BackupObject struct {
	Key      string    `json:"key"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// IsBackupArchive reports whether a storage key is a PocketBase backup
// archive (a .zip at the root; "directories" and other files are ignored).
func IsBackupArchive(key string) bool {
	return key != "" && !strings.Contains(key, "/") && strings.HasSuffix(strings.ToLower(key), ".zip")
}

// SortBackups returns the archives newest first (ties: key descending, the
// names embed a UTC timestamp). Non-archives are dropped.
func SortBackups(objs []BackupObject) []BackupObject {
	out := make([]BackupObject, 0, len(objs))
	for _, o := range objs {
		if IsBackupArchive(o.Key) {
			out = append(out, o)
		}
	}
	slices.SortStableFunc(out, func(a, b BackupObject) int {
		if c := b.Modified.Compare(a.Modified); c != 0 {
			return c
		}
		return strings.Compare(b.Key, a.Key)
	})
	return out
}

// LatestBackup returns the newest archive.
func LatestBackup(objs []BackupObject) (BackupObject, bool) {
	sorted := SortBackups(objs)
	if len(sorted) == 0 {
		return BackupObject{}, false
	}
	return sorted[0], true
}

// BackupsToPrune returns the keys of the archives starting with prefix that
// exceed the retention (the keep newest are kept). keep <= 0 = no limit.
// Archives with another prefix (made by hand in /_/) are never removed.
func BackupsToPrune(objs []BackupObject, prefix string, keep int) []string {
	if keep <= 0 {
		return nil
	}
	var out []string
	n := 0
	for _, o := range SortBackups(objs) {
		if !strings.HasPrefix(o.Key, prefix) {
			continue
		}
		n++
		if n > keep {
			out = append(out, o.Key)
		}
	}
	return out
}

// BackupHealth is the state shown in the admin panel.
type BackupHealth string

// Backup health values.
const (
	BackupDisabled BackupHealth = "disabled" // no schedule configured
	BackupPending  BackupHealth = "pending"  // no backup yet, but configured recently
	BackupOK       BackupHealth = "ok"
	BackupStale    BackupHealth = "stale" // no successful backup for maxAge
)

// CheckBackupHealth: latest is the time of the newest backup (nil = none);
// since is when the server started watching (process start): with no
// backup at all the state stays « pending » during the first maxAge.
func CheckBackupHealth(enabled bool, latest *time.Time, since, now time.Time, maxAge time.Duration) BackupHealth {
	if !enabled {
		return BackupDisabled
	}
	if latest == nil || latest.IsZero() {
		if now.Sub(since) < maxAge {
			return BackupPending
		}
		return BackupStale
	}
	if now.Sub(*latest) > maxAge {
		return BackupStale
	}
	return BackupOK
}

// ShouldSendBackupAlert: one « no backup » e-mail per day at most.
func ShouldSendBackupAlert(health BackupHealth, lastAlert, now time.Time) bool {
	if health != BackupStale {
		return false
	}
	return lastAlert.IsZero() || now.Sub(lastAlert) >= BackupAlertEvery
}

// TableCount compares the rows of a table in the restored backup and in the
// live database.
type TableCount struct {
	Name   string `json:"name"`
	Label  string `json:"label"` // French plural, e.g. « utilisateurs »
	Backup int    `json:"backup"`
	Live   int    `json:"live"`
}

// CheckBackupCounts tells whether the restored counts are plausible.
// A backup is older than the live database, so it normally has as many rows
// or a few less. Errors (the verification fails):
//   - the backup is empty while the live table has rows;
//   - the live table has ≥ 20 rows and the backup less than half of them.
//
// Warnings (verification still ok): the backup has far more rows than the
// live database (more than twice + 10) — data deleted since, to look at.
func CheckBackupCounts(counts []TableCount) (errs, warns []string) {
	for _, c := range counts {
		label := c.Label
		if label == "" {
			label = c.Name
		}
		switch {
		case c.Backup < 0 || c.Live < 0:
			errs = append(errs, fmt.Sprintf("Comptage impossible pour les %s.", label))
		case c.Live > 0 && c.Backup == 0:
			errs = append(errs, fmt.Sprintf("La sauvegarde ne contient aucun enregistrement « %s » alors que la base en compte %d.", label, c.Live))
		case c.Live >= 20 && c.Backup*2 < c.Live:
			errs = append(errs, fmt.Sprintf("La sauvegarde contient %d %s, moins de la moitié de la base actuelle (%d).", c.Backup, label, c.Live))
		case c.Backup > c.Live*2+10:
			warns = append(warns, fmt.Sprintf("La sauvegarde contient %d %s contre %d dans la base actuelle (suppressions depuis ?).", c.Backup, label, c.Live))
		}
	}
	return errs, warns
}

// MissingTables returns the expected tables absent from a restored database.
func MissingTables(expected, present []string) []string {
	var out []string
	for _, t := range expected {
		if !slices.Contains(present, t) {
			out = append(out, t)
		}
	}
	return out
}

// PragmaOK reports whether the rows of PRAGMA integrity_check / quick_check
// mean « no problem » (a single row "ok").
func PragmaOK(rows []string) bool {
	return len(rows) == 1 && strings.EqualFold(strings.TrimSpace(rows[0]), "ok")
}
