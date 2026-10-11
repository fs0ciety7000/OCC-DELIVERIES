package app

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/cron"
	"github.com/pocketbase/pocketbase/tools/inflector"
	"github.com/pocketbase/pocketbase/tools/mailer"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/migrations"
)

// Automatic backups of pb_data (PocketBase built-in backups, S3 storage —
// Cloudflare R2 in production) and their restore verification.
//
//   - OCC_BACKUP_S3_* set → at startup the PocketBase backup settings are
//     written (schedule, retention, S3); absent → /_/ settings left untouched.
//   - after every backup (and hourly when the latest one was never checked)
//     the latest archive is downloaded into pb_data/.pb_temp_to_delete,
//     data.db is extracted and opened read-only: PRAGMA integrity_check and
//     quick_check, expected tables, counts compared with the live database
//     (domain.CheckBackupCounts). Result stored in backup_checks.
//   - no successful backup for 36 h → danger badge in /admin and one e-mail
//     per day to the admins (when SMTP is configured).

const (
	colBackupChecks = migrations.BackupChecksCollection

	// DefaultBackupCron is OCC_BACKUP_CRON's default. PocketBase evaluates the
	// backup schedule in UTC: 00:15 UTC = 02:15 in Brussels in summer (CEST),
	// 01:15 in winter (CET) — always before the menu synchronisation (03:30).
	DefaultBackupCron = "15 0 * * *"
	defaultBackupKeep = 14

	// backupAutoPrefix is the prefix PocketBase gives to scheduled backups
	// (its retention only prunes those). Manual runs reuse it so that the
	// retention also bounds them.
	backupAutoPrefix = "@auto_pb_backup_"

	backupWatchJobID    = "occ_backup_watch"
	backupWatchSchedule = "7 * * * *" // hourly: unchecked backup, staleness alert

	backupKindBackup = "backup"
	backupKindVerify = "verify"
	backupKindAlert  = "alert"

	backupTriggerCron   = "cron"
	backupTriggerManual = "manual"
	backupTriggerBackup = "backup" // verification right after a backup

	backupRunBackup = "backup"
	backupRunVerify = "verify"

	maxBackupDownload = int64(8 << 30) // archive and extracted data.db
	maxBackupChecks   = 300            // rows kept in backup_checks
	backupOpTimeout   = 30 * time.Minute
)

// backupExpectedTables must exist in a restored data.db.
var backupExpectedTables = []string{
	"_collections", "_params", "_migrations", "_superusers",
	colUsers, colParties, colRestaurants, colMenuItems, colOrderItems, colPayments, colAppSettings,
}

// backupCountedTables are compared with the live database (French labels).
var backupCountedTables = []struct{ name, label string }{
	{colUsers, "utilisateurs"},
	{colParties, "commandes"},
	{colRestaurants, "restaurants"},
	{colMenuItems, "plats"},
	{colOrderItems, "lignes de commande"},
	{colPayments, "paiements"},
}

var errBackupBusy = errors.New("backup busy")

// BackupConfig is the backups configuration (OCC_BACKUP_*).
type BackupConfig struct {
	Endpoint       string // OCC_BACKUP_S3_ENDPOINT (https://<account>.r2.cloudflarestorage.com)
	Bucket         string // OCC_BACKUP_S3_BUCKET
	Region         string // OCC_BACKUP_S3_REGION (default "auto")
	AccessKey      string // OCC_BACKUP_S3_ACCESS_KEY
	Secret         string // OCC_BACKUP_S3_SECRET (never logged)
	ForcePathStyle bool   // OCC_BACKUP_S3_FORCE_PATH_STYLE (default true)
	Cron           string // OCC_BACKUP_CRON, UTC (default DefaultBackupCron)
	Keep           int    // OCC_BACKUP_KEEP (default 14)
	// Now is the clock (tests); default time.Now.
	Now func() time.Time
}

// Enabled reports whether every S3 value needed is set.
func (c BackupConfig) Enabled() bool {
	return c.Endpoint != "" && c.Bucket != "" && c.AccessKey != "" && c.Secret != ""
}

// partial: some S3 values set, not all (ignored with a warning).
func (c BackupConfig) partial() bool {
	return !c.Enabled() && (c.Endpoint != "" || c.Bucket != "" || c.AccessKey != "" || c.Secret != "")
}

func backupConfigFromEnv(lookup func(string) string) BackupConfig {
	get := func(k string) string { return strings.TrimSpace(lookup(k)) }
	bc := BackupConfig{
		Endpoint:       strings.TrimRight(get("OCC_BACKUP_S3_ENDPOINT"), "/"),
		Bucket:         get("OCC_BACKUP_S3_BUCKET"),
		Region:         get("OCC_BACKUP_S3_REGION"),
		AccessKey:      get("OCC_BACKUP_S3_ACCESS_KEY"),
		Secret:         get("OCC_BACKUP_S3_SECRET"),
		ForcePathStyle: true,
		Cron:           get("OCC_BACKUP_CRON"),
		Keep:           defaultBackupKeep,
	}
	if bc.Endpoint != "" && !strings.Contains(bc.Endpoint, "://") {
		bc.Endpoint = "https://" + bc.Endpoint
	}
	if bc.Region == "" {
		bc.Region = "auto"
	}
	if v, err := strconv.ParseBool(get("OCC_BACKUP_S3_FORCE_PATH_STYLE")); err == nil {
		bc.ForcePathStyle = v
	}
	if bc.Cron == "" {
		bc.Cron = DefaultBackupCron
	} else if _, err := cron.NewSchedule(bc.Cron); err != nil {
		bc.Cron = DefaultBackupCron // invalid: settings validation would reject it
	}
	if n, err := strconv.Atoi(get("OCC_BACKUP_KEEP")); err == nil && n >= 1 && n <= 1000 {
		bc.Keep = n
	}
	return bc
}

// applyBackupSettings copies the env configuration into the PocketBase
// backup settings. Returns whether something changed; nothing is touched
// unless every S3 value is set.
func applyBackupSettings(s *core.Settings, bc BackupConfig) bool {
	if !bc.Enabled() {
		return false
	}
	want := core.BackupsConfig{
		Cron:        bc.Cron,
		CronMaxKeep: bc.Keep,
		S3: core.S3Config{
			Enabled: true, Bucket: bc.Bucket, Region: bc.Region, Endpoint: bc.Endpoint,
			AccessKey: bc.AccessKey, Secret: bc.Secret, ForcePathStyle: bc.ForcePathStyle,
		},
	}
	if s.Backups == want {
		return false
	}
	s.Backups = want
	return true
}

// backupService runs the manual backups, the verifications and the watch job.
type backupService struct {
	app     core.App
	cfg     BackupConfig
	started time.Time

	mu         sync.Mutex
	running    string // "", backupRunBackup, backupRunVerify
	manualName string // name of the manual backup in progress
	manualSeen bool   // the OnBackupCreate hook recorded it
	wg         sync.WaitGroup
}

func newBackupService(app core.App, cfg BackupConfig) *backupService {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &backupService{app: app, cfg: cfg, started: cfg.Now()}
}

func (b *backupService) now() time.Time { return b.cfg.Now() }

func (b *backupService) bind() {
	b.app.OnBackupCreate().BindFunc(func(e *core.BackupEvent) error {
		start := b.now()
		err := e.Next()
		manual := b.isManual(e.Name)
		trigger := backupTriggerCron
		if manual || !strings.HasPrefix(e.Name, backupAutoPrefix) {
			trigger = backupTriggerManual // our button, or a backup made in /_/
		}
		b.recordBackup(e.App, e.Name, trigger, b.now().Sub(start), err)
		if err == nil && !manual {
			// scheduled (or /_/) backup: verify it in the background
			b.wg.Add(1)
			go func() {
				defer b.wg.Done()
				if _, verr := b.verify(backupTriggerBackup); verr != nil && !errors.Is(verr, errBackupBusy) {
					b.app.Logger().Warn("backup verification failed", "error", verr)
				}
			}()
		}
		return err
	})
	b.app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if err := se.App.Cron().Add(backupWatchJobID, backupWatchSchedule, func() { b.watch() }); err != nil {
			return err
		}
		return se.Next()
	})
}

// applySettings runs at startup (OnServe).
func (b *backupService) applySettings(app core.App) error {
	if b.cfg.partial() {
		app.Logger().Warn("automatic backups ignored: set OCC_BACKUP_S3_ENDPOINT, _BUCKET, _ACCESS_KEY and _SECRET")
	}
	s := app.Settings()
	if !applyBackupSettings(s, b.cfg) {
		return nil
	}
	if err := app.Save(s); err != nil {
		return err
	}
	app.Logger().Info("automatic backups configured from env", "bucket", b.cfg.Bucket, "cron", b.cfg.Cron, "timezone", "UTC", "keep", b.cfg.Keep)
	return nil
}

func (b *backupService) isManual(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if name == "" || name != b.manualName {
		return false
	}
	b.manualSeen = true
	return true
}

func (b *backupService) runningOp() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

func (b *backupService) acquire(op string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running != "" {
		return false
	}
	b.running = op
	return true
}

func (b *backupService) release() {
	b.mu.Lock()
	b.running, b.manualName, b.manualSeen = "", "", false
	b.mu.Unlock()
}

// wait blocks until the background operations end (tests).
func (b *backupService) wait() { b.wg.Wait() }

// backupName mirrors PocketBase's scheduled backup names.
func backupName(app core.App, t time.Time) string {
	name := inflector.Snakecase(app.Settings().Meta.AppName)
	if len(name) > 50 {
		name = name[:50]
	}
	return backupAutoPrefix + name + "_" + t.UTC().Format("20060102150405") + ".zip"
}

// startBackup launches « Sauvegarder maintenant »: backup, retention, then
// verification of the new archive. errBackupBusy when something runs.
func (b *backupService) startBackup() error {
	if b.app.Store().Has(core.StoreKeyActiveBackup) || !b.acquire(backupRunBackup) {
		return errBackupBusy
	}
	name := backupName(b.app, b.now())
	b.mu.Lock()
	b.manualName = name
	b.mu.Unlock()
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer b.release()
		ctx, cancel := context.WithTimeout(context.Background(), backupOpTimeout)
		defer cancel()
		start := b.now()
		if err := b.app.CreateBackup(ctx, name); err != nil {
			b.app.Logger().Error("manual backup failed", "name", name, "error", err)
			b.mu.Lock()
			seen := b.manualSeen
			b.mu.Unlock()
			if !seen { // refused before the hook (another backup / restore)
				b.recordBackup(b.app, name, backupTriggerManual, b.now().Sub(start), err)
			}
			return
		}
		b.prune(ctx)
		b.mu.Lock()
		b.running = backupRunVerify
		b.mu.Unlock()
		if _, err := b.verifyLocked(ctx, backupTriggerManual); err != nil {
			b.app.Logger().Warn("backup verification failed", "error", err)
		}
	}()
	return nil
}

// startVerify launches « Vérifier la restauration » in the background.
func (b *backupService) startVerify() error {
	if !b.acquire(backupRunVerify) {
		return errBackupBusy
	}
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer b.release()
		ctx, cancel := context.WithTimeout(context.Background(), backupOpTimeout)
		defer cancel()
		if _, err := b.verifyLocked(ctx, backupTriggerManual); err != nil {
			b.app.Logger().Warn("backup verification failed", "error", err)
		}
	}()
	return nil
}

// verify runs a verification now (synchronously), errBackupBusy when busy.
func (b *backupService) verify(trigger string) (*core.Record, error) {
	if !b.acquire(backupRunVerify) {
		return nil, errBackupBusy
	}
	defer b.release()
	ctx, cancel := context.WithTimeout(context.Background(), backupOpTimeout)
	defer cancel()
	return b.verifyLocked(ctx, trigger)
}

// keep is the retention of the scheduled backups.
func (b *backupService) keep() int {
	if b.cfg.Enabled() {
		return b.cfg.Keep
	}
	return b.app.Settings().Backups.CronMaxKeep
}

// prune applies the retention after a manual backup (the scheduled ones are
// pruned by PocketBase itself).
func (b *backupService) prune(ctx context.Context) {
	objs, err := listBackups(ctx, b.app)
	if err != nil {
		b.app.Logger().Warn("backup retention: list failed", "error", err)
		return
	}
	keys := domain.BackupsToPrune(objs, backupAutoPrefix, b.keep())
	if len(keys) == 0 {
		return
	}
	fsys, err := b.app.NewBackupsFilesystem()
	if err != nil {
		b.app.Logger().Warn("backup retention: storage unavailable", "error", err)
		return
	}
	defer fsys.Close()
	fsys.SetContext(ctx)
	for _, k := range keys {
		if err := fsys.Delete(k); err != nil {
			b.app.Logger().Warn("backup retention: delete failed", "key", k, "error", err)
		}
	}
}

// listBackups lists the archives of the backups storage, newest first.
func listBackups(ctx context.Context, app core.App) ([]domain.BackupObject, error) {
	fsys, err := app.NewBackupsFilesystem()
	if err != nil {
		return nil, err
	}
	defer fsys.Close()
	fsys.SetContext(ctx)
	files, err := fsys.List("")
	if err != nil {
		return nil, err
	}
	objs := make([]domain.BackupObject, 0, len(files))
	for _, f := range files {
		if f.IsDir {
			continue
		}
		objs = append(objs, domain.BackupObject{Key: f.Key, Size: f.Size, Modified: f.ModTime.UTC()})
	}
	return domain.SortBackups(objs), nil
}

// ------------------------------------------------------------ verification

// verifyResult is one restore verification.
type verifyResult struct {
	ok        bool
	key       string
	size      int64
	modified  time.Time
	integrity string
	errs      []string
	warnings  []string
	counts    []domain.TableCount
}

func (r *verifyResult) fail(msg string) { r.errs = append(r.errs, msg) }

// verifyLocked verifies the latest backup and stores the result (the
// caller holds the running slot).
func (b *backupService) verifyLocked(ctx context.Context, trigger string) (*core.Record, error) {
	start := b.now()
	res := verifyLatestBackup(ctx, b.app)
	rec, err := b.saveCheck(b.app, backupKindVerify, trigger, func(r *core.Record) {
		r.Set("ok", res.ok)
		r.Set("backup_key", res.key)
		r.Set("backup_size", res.size)
		if !res.modified.IsZero() {
			r.Set("backup_modified", res.modified)
		}
		r.Set("duration_ms", b.now().Sub(start).Milliseconds())
		r.Set("integrity", truncateRunes(res.integrity, 2000))
		r.Set("error", truncateRunes(strings.Join(res.errs, "\n"), 4000))
		r.Set("warnings", nonNil(res.warnings))
		r.Set("counts", nonNilCounts(res.counts))
	})
	if err != nil {
		return nil, err
	}
	if res.ok {
		b.app.Logger().Info("backup verified", "key", res.key, "size", res.size)
	} else {
		b.app.Logger().Error("backup verification failed", "key", res.key, "errors", strings.Join(res.errs, " | "))
	}
	return rec, nil
}

// verifyLatestBackup downloads the newest archive, extracts data.db into a
// temporary directory (removed at the end) and checks it.
func verifyLatestBackup(ctx context.Context, app core.App) (res verifyResult) {
	objs, err := listBackups(ctx, app)
	if err != nil {
		res.fail("Stockage des sauvegardes injoignable : " + safeErr(err))
		return res
	}
	latest, found := domain.LatestBackup(objs)
	if !found {
		res.fail("Aucune sauvegarde trouvée.")
		return res
	}
	res.key, res.size, res.modified = latest.Key, latest.Size, latest.Modified

	tmpRoot := filepath.Join(app.DataDir(), core.LocalTempDirName)
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		res.fail("Dossier temporaire impossible à créer : " + safeErr(err))
		return res
	}
	dir, err := os.MkdirTemp(tmpRoot, "occ_backup_verify_")
	if err != nil {
		res.fail("Dossier temporaire impossible à créer : " + safeErr(err))
		return res
	}
	defer os.RemoveAll(dir)

	zipPath := filepath.Join(dir, "backup.zip")
	n, err := downloadBackup(ctx, app, latest.Key, zipPath)
	if err != nil {
		res.fail("Téléchargement impossible : " + safeErr(err))
		return res
	}
	if latest.Size > 0 && n != latest.Size {
		res.fail(fmt.Sprintf("Archive incomplète : %d octets reçus sur %d.", n, latest.Size))
		return res
	}
	res.size = n

	dbPath := filepath.Join(dir, "data.db")
	hasAux, err := extractDataDB(zipPath, dbPath)
	if err != nil {
		res.fail(safeErr(err))
		return res
	}
	if !hasAux {
		res.warnings = append(res.warnings, "L'archive ne contient pas auxiliary.db (journaux PocketBase).")
	}
	checkRestoredDB(ctx, app, dbPath, &res)
	res.ok = len(res.errs) == 0
	return res
}

func downloadBackup(ctx context.Context, app core.App, key, dst string) (int64, error) {
	fsys, err := app.NewBackupsFilesystem()
	if err != nil {
		return 0, err
	}
	defer fsys.Close()
	fsys.SetContext(ctx)
	r, err := fsys.GetReader(key)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	f, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxBackupDownload+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxBackupDownload {
		err = errors.New("archive trop volumineuse")
	}
	return n, err
}

// extractDataDB extracts the root data.db of a PocketBase backup archive to
// dst (no other path is ever written: no zip slip). Reports whether the
// archive also holds auxiliary.db.
func extractDataDB(zipPath, dst string) (hasAux bool, err error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return false, fmt.Errorf("Archive illisible (zip invalide) : %s", safeErr(err))
	}
	defer zr.Close()
	var data *zip.File
	for _, f := range zr.File {
		switch f.Name {
		case "data.db":
			data = f
		case "auxiliary.db":
			hasAux = true
		}
	}
	if data == nil {
		return hasAux, errors.New("L'archive ne contient pas data.db.")
	}
	if data.UncompressedSize64 > uint64(maxBackupDownload) {
		return hasAux, errors.New("data.db trop volumineux.")
	}
	rc, err := data.Open()
	if err != nil {
		return hasAux, fmt.Errorf("data.db illisible dans l'archive : %s", safeErr(err))
	}
	defer rc.Close()
	out, err := os.Create(dst)
	if err != nil {
		return hasAux, err
	}
	_, err = io.Copy(out, io.LimitReader(rc, maxBackupDownload))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return hasAux, fmt.Errorf("Extraction de data.db impossible (archive corrompue ?) : %s", safeErr(err))
	}
	return hasAux, nil
}

// checkRestoredDB opens the extracted database read-only and fills res.
func checkRestoredDB(ctx context.Context, app core.App, path string, res *verifyResult) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		res.fail("Base restaurée impossible à ouvrir : " + safeErr(err))
		return
	}
	defer db.Close()

	var integrity []string
	for _, pragma := range []string{"integrity_check", "quick_check"} {
		rows, err := pragmaRows(ctx, db, pragma)
		if err != nil {
			res.fail(fmt.Sprintf("PRAGMA %s impossible : %s", pragma, safeErr(err)))
			return
		}
		if !domain.PragmaOK(rows) {
			res.fail(fmt.Sprintf("PRAGMA %s : %s", pragma, truncateRunes(strings.Join(rows, " ; "), 500)))
		}
		integrity = append(integrity, pragma+": "+truncateRunes(strings.Join(rows, " ; "), 300))
	}
	res.integrity = strings.Join(integrity, "\n")

	tables, err := stringRows(ctx, db, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		res.fail("Tables illisibles : " + safeErr(err))
		return
	}
	if missing := domain.MissingTables(backupExpectedTables, tables); len(missing) > 0 {
		res.fail("Tables absentes de la sauvegarde : " + strings.Join(missing, ", ") + ".")
	}

	for _, t := range backupCountedTables {
		c := domain.TableCount{Name: t.name, Label: t.label, Backup: -1, Live: -1}
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdent(t.name)).Scan(&n); err == nil {
			c.Backup = n
		}
		if live, err := app.CountRecords(t.name); err == nil {
			c.Live = int(live)
		}
		res.counts = append(res.counts, c)
	}
	errs, warns := domain.CheckBackupCounts(res.counts)
	res.errs = append(res.errs, errs...)
	res.warnings = append(res.warnings, warns...)
}

func pragmaRows(ctx context.Context, db *sql.DB, pragma string) ([]string, error) {
	return stringRows(ctx, db, "PRAGMA "+pragma)
}

func stringRows(ctx context.Context, db *sql.DB, query string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// safeErr: error text for the admin panel (never contains credentials: the
// S3 client reports the code / message of the response, not the request).
func safeErr(err error) string { return truncateRunes(err.Error(), 400) }

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilCounts(c []domain.TableCount) []domain.TableCount {
	if c == nil {
		return []domain.TableCount{}
	}
	return c
}

// ------------------------------------------------------------ journal

// recordBackup stores a backup attempt (scheduled, manual or from /_/).
func (b *backupService) recordBackup(app core.App, name, trigger string, d time.Duration, err error) {
	_, serr := b.saveCheck(app, backupKindBackup, trigger, func(r *core.Record) {
		r.Set("ok", err == nil)
		r.Set("backup_key", name)
		r.Set("duration_ms", d.Milliseconds())
		if err != nil {
			r.Set("error", truncateRunes(err.Error(), 4000))
		}
	})
	if serr != nil {
		app.Logger().Warn("backup journal write failed", "error", serr)
	}
}

func (b *backupService) saveCheck(app core.App, kind, trigger string, fill func(*core.Record)) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId(colBackupChecks)
	if err != nil {
		return nil, err
	}
	r := core.NewRecord(col)
	r.Set("kind", kind)
	r.Set("trigger", trigger)
	fill(r)
	if err := app.Save(r); err != nil {
		return nil, err
	}
	// keep the journal small
	if _, err := app.DB().NewQuery(`DELETE FROM ` + colBackupChecks + ` WHERE id NOT IN
		(SELECT id FROM ` + colBackupChecks + ` ORDER BY created DESC, id DESC LIMIT {:n})`).
		Bind(dbx.Params{"n": maxBackupChecks}).Execute(); err != nil {
		app.Logger().Warn("backup journal prune failed", "error", err)
	}
	return r, nil
}

// lastCheck returns the newest row of a kind (ok: only successful ones).
func lastCheck(app core.App, kind string, onlyOK bool) *core.Record {
	filter := "kind = {:k}"
	if onlyOK {
		filter += " && ok = true"
	}
	recs, err := app.FindRecordsByFilter(colBackupChecks, filter, "-created", 1, 0, dbx.Params{"k": kind})
	if err != nil || len(recs) == 0 {
		return nil
	}
	return recs[0]
}

// ------------------------------------------------------------ watch & alert

// backupsConfigured: a schedule is set (env or /_/).
func backupsConfigured(app core.App) bool { return app.Settings().Backups.Cron != "" }

// health computes the state from the newest archive.
func (b *backupService) health(latest *domain.BackupObject) domain.BackupHealth {
	var t *time.Time
	if latest != nil {
		t = &latest.Modified
	}
	return domain.CheckBackupHealth(backupsConfigured(b.app), t, b.started, b.now(), domain.BackupMaxAge)
}

// watch runs hourly: verifies the latest backup if nobody did, and sends
// the « no backup for 36 h » alert (once per day).
func (b *backupService) watch() {
	if !backupsConfigured(b.app) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	objs, err := listBackups(ctx, b.app)
	cancel()
	var latest *domain.BackupObject
	if err != nil {
		b.app.Logger().Warn("backup watch: list failed", "error", err)
	} else if l, ok := domain.LatestBackup(objs); ok {
		latest = &l
		if last := lastCheck(b.app, backupKindVerify, false); last == nil || last.GetString("backup_key") != l.Key {
			if _, err := b.verify(backupTriggerCron); err != nil && !errors.Is(err, errBackupBusy) {
				b.app.Logger().Warn("backup verification failed", "error", err)
			}
		}
	}
	b.maybeAlert(b.health(latest))
}

// maybeAlert e-mails the active admins when the backups are stale (at most
// once per BackupAlertEvery, only with SMTP).
func (b *backupService) maybeAlert(h domain.BackupHealth) {
	var lastAt time.Time
	if last := lastCheck(b.app, backupKindAlert, false); last != nil {
		lastAt = last.GetDateTime("created").Time()
	}
	if !domain.ShouldSendBackupAlert(h, lastAt, b.now()) || !mailEnabled(b.app) {
		return
	}
	sent, err := b.sendAlert()
	_, serr := b.saveCheck(b.app, backupKindAlert, backupTriggerCron, func(r *core.Record) {
		r.Set("ok", err == nil && sent > 0)
		if err != nil {
			r.Set("error", truncateRunes(err.Error(), 4000))
		} else if sent == 0 {
			r.Set("error", "Aucun administrateur à prévenir.")
		}
	})
	if serr != nil {
		b.app.Logger().Warn("backup journal write failed", "error", serr)
	}
}

func (b *backupService) sendAlert() (int, error) {
	admins, err := b.app.FindAllRecords(colUsers, dbx.HashExp{"role": roleAdmin, fieldBanned: false},
		dbx.Or(dbx.HashExp{fieldDeletedAt: ""}, dbx.NewExp("[["+fieldDeletedAt+"]] IS NULL")))
	if err != nil {
		return 0, err
	}
	s := b.app.Settings()
	body := backupAlertMailBody(s.Meta.AppURL)
	sent := 0
	var lastErr error
	for _, a := range admins {
		to := a.Email()
		if _, err := mail.ParseAddress(to); err != nil || strings.HasSuffix(to, ".invalid") {
			continue
		}
		msg := &mailer.Message{
			From:    mail.Address{Name: s.Meta.SenderName, Address: s.Meta.SenderAddress},
			To:      []mail.Address{{Address: to}},
			Subject: "⚠️ Aucune sauvegarde depuis 36 h — " + s.Meta.AppName,
			HTML:    body,
		}
		if err := b.app.NewMailClient().Send(msg); err != nil {
			lastErr = err
			b.app.Logger().Warn("backup alert e-mail failed", slog.String("error", err.Error()))
			continue
		}
		sent++
	}
	if sent == 0 && lastErr != nil {
		return 0, lastErr
	}
	return sent, nil
}

// nextCronRun is the next minute matching a 5-field expression in UTC.
func nextCronRun(expr string, from time.Time) *time.Time {
	if expr == "" {
		return nil
	}
	sched, err := cron.NewSchedule(expr)
	if err != nil {
		return nil
	}
	t := from.UTC().Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 60*24*32; i++ {
		if sched.IsDue(cron.NewMoment(t)) {
			return &t
		}
		t = t.Add(time.Minute)
	}
	return nil
}

func isoTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func isoDate(d types.DateTime) string { return isoTime(d.Time()) }

// backupAlertMailBody is the « no backup for 36 h » e-mail.
func backupAlertMailBody(appURL string) string {
	body := renderMail(mailContent{
		Preheader: "Les sauvegardes automatiques de pb_data semblent arrêtées.",
		Title:     "Aucune sauvegarde depuis 36 h ⚠️",
		Paragraphs: []string{
			"Aucune sauvegarde réussie de la base d'OCC Deliveries n'a été trouvée dans le stockage des sauvegardes depuis plus de 36 heures.",
			"Vérifie l'état dans l'administration (tableau de bord → Sauvegardes) puis lance « Sauvegarder maintenant ». Si l'erreur persiste, contrôle les variables OCC_BACKUP_S3_* et le bucket Cloudflare R2.",
		},
		CTALabel: "Ouvrir l'administration",
		CTAURL:   "{APP_URL}/admin",
		Footnote: "Cet e-mail est envoyé au plus une fois par jour tant que la situation dure.",
	})
	return strings.NewReplacer("{APP_URL}", html.EscapeString(appURL), "{APP_NAME}", "OCC Deliveries").Replace(body)
}
