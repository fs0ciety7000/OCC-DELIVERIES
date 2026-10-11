package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// testClock is an adjustable clock; every reading advances it by a second
// so that two backups never get the same (second-resolution) name.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(time.Second)
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// newBackupEnv registers the app with cfg and returns the env and handlers.
// before runs before the OnServe hooks.
func newBackupEnv(t *testing.T, cfg Config, before func(*tests.TestApp)) (*env, *handlers) {
	t.Helper()
	ta, err := tests.NewTestApp(templateDir)
	if err != nil {
		t.Fatal(err)
	}
	h := register(ta, cfg)
	t.Cleanup(func() {
		h.backups.wait()
		ta.Cleanup()
	})
	if before != nil {
		before(ta)
	}
	return serveEnv(t, ta), h
}

func TestBackupConfigFromEnv(t *testing.T) {
	full := map[string]string{
		"OCC_BACKUP_S3_ENDPOINT":   "https://acc.r2.cloudflarestorage.com/",
		"OCC_BACKUP_S3_BUCKET":     "occ-deliveries-backups",
		"OCC_BACKUP_S3_ACCESS_KEY": "AK",
		"OCC_BACKUP_S3_SECRET":     "SK",
	}
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range full {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name      string
		env       map[string]string
		enabled   bool
		partial   bool
		endpoint  string
		region    string
		pathStyle bool
		cron      string
		keep      int
	}{
		{"rien", map[string]string{}, false, false, "", "auto", true, DefaultBackupCron, 14},
		{"complet", full, true, false, "https://acc.r2.cloudflarestorage.com", "auto", true, DefaultBackupCron, 14},
		{"partiel", map[string]string{"OCC_BACKUP_S3_BUCKET": "b"}, false, true, "", "auto", true, DefaultBackupCron, 14},
		{"sans schéma", with(map[string]string{"OCC_BACKUP_S3_ENDPOINT": "acc.r2.cloudflarestorage.com"}), true, false, "https://acc.r2.cloudflarestorage.com", "auto", true, DefaultBackupCron, 14},
		{"réglages", with(map[string]string{"OCC_BACKUP_S3_REGION": "weur", "OCC_BACKUP_S3_FORCE_PATH_STYLE": "false", "OCC_BACKUP_CRON": "0 3 * * *", "OCC_BACKUP_KEEP": "30"}), true, false, "https://acc.r2.cloudflarestorage.com", "weur", false, "0 3 * * *", 30},
		{"cron invalide, rétention hors bornes", with(map[string]string{"OCC_BACKUP_CRON": "chaque nuit", "OCC_BACKUP_KEEP": "0"}), true, false, "https://acc.r2.cloudflarestorage.com", "auto", true, DefaultBackupCron, 14},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bc := backupConfigFromEnv(func(k string) string { return tc.env[k] })
			if bc.Enabled() != tc.enabled || bc.partial() != tc.partial || bc.Endpoint != tc.endpoint || bc.Region != tc.region ||
				bc.ForcePathStyle != tc.pathStyle || bc.Cron != tc.cron || bc.Keep != tc.keep {
				t.Fatalf("%+v", bc)
			}
		})
	}
}

func TestApplyBackupSettings(t *testing.T) {
	s := &core.Settings{}
	s.Backups.Cron = "0 1 * * *" // set in /_/
	if applyBackupSettings(s, BackupConfig{Bucket: "b"}) || s.Backups.Cron != "0 1 * * *" {
		t.Fatal("an incomplete env configuration must not touch the settings")
	}
	bc := BackupConfig{Endpoint: "https://x", Bucket: "b", Region: "auto", AccessKey: "a", Secret: "s", ForcePathStyle: true, Cron: DefaultBackupCron, Keep: 14}
	if !applyBackupSettings(s, bc) {
		t.Fatal("expected a change")
	}
	if s.Backups.Cron != DefaultBackupCron || s.Backups.CronMaxKeep != 14 || !s.Backups.S3.Enabled || !s.Backups.S3.ForcePathStyle || s.Backups.S3.Secret != "s" {
		t.Fatalf("%+v", s.Backups)
	}
	if err := s.Backups.Validate(); err != nil {
		t.Fatal(err)
	}
	if applyBackupSettings(s, bc) {
		t.Fatal("second call must be a no-op")
	}
}

func TestNextCronRun(t *testing.T) {
	from := time.Date(2026, 10, 10, 0, 15, 30, 0, time.UTC)
	if got := nextCronRun(DefaultBackupCron, from); got == nil || !got.Equal(time.Date(2026, 10, 11, 0, 15, 0, 0, time.UTC)) {
		t.Fatal(got)
	}
	if nextCronRun("", from) != nil || nextCronRun("nope", from) != nil {
		t.Fatal("expected nil")
	}
}

// waitBackupIdle waits for the background operations of the service.
func waitBackupIdle(t *testing.T, h *handlers) {
	t.Helper()
	h.backups.wait()
	if op := h.backups.runningOp(); op != "" {
		t.Fatalf("still running: %s", op)
	}
}

func TestBackupLocalRunAndVerify(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)}
	cfg := testConfig
	cfg.Backup = BackupConfig{Now: clock.now}
	e, h := newBackupEnv(t, cfg, nil)
	boss, alice := e.admin("Boss"), e.user("Alice")

	e.expect(401, "GET", "/api/occ/admin/backups", "", nil)
	e.expect(403, "GET", "/api/occ/admin/backups", alice.token, nil)
	e.expect(403, "POST", "/api/occ/admin/backups/run", alice.token, nil)
	e.expect(403, "POST", "/api/occ/admin/backups/verify", alice.token, nil)

	// nothing configured: disabled, local storage, nothing listed
	st := e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t)
	if st["configured"] != false || st["health"] != "disabled" || st["storage"] != "local" || st["lastBackup"] != nil || st["fromEnv"] != false {
		t.Fatalf("status: %v", st)
	}

	// verification without any backup: stored as failed
	e.expect(202, "POST", "/api/occ/admin/backups/verify", boss.token, nil)
	waitBackupIdle(t, h)
	st = e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t)
	lv := st["lastVerification"].(map[string]any)
	if lv["ok"] != false || !strings.Contains(lv["error"].(string), "Aucune sauvegarde") {
		t.Fatalf("verification: %v", lv)
	}

	// schedule set in /_/ (no S3): local backups in pb_data/backups
	s := e.app.Settings()
	s.Backups.Cron, s.Backups.CronMaxKeep = DefaultBackupCron, 2
	if err := e.app.Save(s); err != nil {
		t.Fatal(err)
	}
	st = e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t)
	if st["configured"] != true || st["health"] != "pending" || st["nextBackupAt"] == nil || st["timezone"] != "UTC" {
		t.Fatalf("status: %v", st)
	}

	for i := 0; i < 3; i++ {
		r := e.expect(202, "POST", "/api/occ/admin/backups/run", boss.token, nil).m(t)
		if r["started"] != true {
			t.Fatal(r)
		}
		waitBackupIdle(t, h)
	}
	st = e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t)
	if st["health"] != "ok" || st["totalBackups"] != float64(2) { // retention 2
		t.Fatalf("status: %v", st)
	}
	last := st["lastBackup"].(map[string]any)
	if !strings.HasPrefix(last["key"].(string), backupAutoPrefix) || last["size"].(float64) <= 0 {
		t.Fatalf("last backup: %v", last)
	}
	run := st["lastBackupRun"].(map[string]any)
	if run["ok"] != true || run["trigger"] != "manual" || run["backupKey"] != last["key"] {
		t.Fatalf("backup run: %v", run)
	}
	lv = st["lastVerification"].(map[string]any)
	if lv["ok"] != true || lv["backupKey"] != last["key"] || lv["trigger"] != "manual" || lv["error"] != "" {
		t.Fatalf("verification: %v", lv)
	}
	if !strings.Contains(lv["integrity"].(string), "integrity_check: ok") || !strings.Contains(lv["integrity"].(string), "quick_check: ok") {
		t.Fatalf("integrity: %v", lv["integrity"])
	}
	counts := map[string]map[string]any{}
	for _, c := range lv["counts"].([]any) {
		m := c.(map[string]any)
		counts[m["name"].(string)] = m
	}
	if u := counts["users"]; u == nil || u["backup"].(float64) < 2 || u["backup"] != u["live"] {
		t.Fatalf("counts: %v", lv["counts"])
	}
	if r := counts["restaurants"]; r == nil || r["backup"].(float64) == 0 {
		t.Fatalf("counts: %v", lv["counts"])
	}
	// temporary files removed
	entries, _ := os.ReadDir(filepath.Join(e.app.DataDir(), core.LocalTempDirName))
	for _, en := range entries {
		if strings.HasPrefix(en.Name(), "occ_backup_verify_") {
			t.Fatalf("temp dir left: %s", en.Name())
		}
	}

	// busy → 409
	if !h.backups.acquire(backupRunVerify) {
		t.Fatal("acquire")
	}
	r := e.expect(409, "POST", "/api/occ/admin/backups/run", boss.token, nil)
	if !strings.Contains(string(r.body), "déjà en cours") {
		t.Fatal(string(r.body))
	}
	e.expect(409, "POST", "/api/occ/admin/backups/verify", boss.token, nil)
	if st := e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t); st["running"] != "verify" {
		t.Fatalf("running: %v", st["running"])
	}
	h.backups.release()

	// a backup made in /_/ (another prefix) is recorded and verified too
	if err := e.app.CreateBackup(t.Context(), "pb_backup_manuel.zip"); err != nil {
		t.Fatal(err)
	}
	waitBackupIdle(t, h)
	st = e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t)
	if lv := st["lastVerification"].(map[string]any); lv["ok"] != true || lv["trigger"] != "backup" || lv["backupKey"] != "pb_backup_manuel.zip" {
		t.Fatalf("verification after /_/ backup: %v", lv)
	}
}

// zipWith builds a zip archive holding the given files.
func zipWith(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// sqliteFile creates a valid SQLite database with the given statements.
func sqliteFile(t *testing.T, stmts ...string) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBackupS3RunVerifyAndBrokenArchives(t *testing.T) {
	fake := newFakeS3(t, "occ-deliveries-backups")
	clock := &testClock{t: time.Now().UTC()}
	cfg := testConfig
	cfg.Backup = backupConfigFromEnv(func(k string) string {
		return map[string]string{
			"OCC_BACKUP_S3_ENDPOINT":   fake.srv.URL,
			"OCC_BACKUP_S3_BUCKET":     "occ-deliveries-backups",
			"OCC_BACKUP_S3_ACCESS_KEY": "test-access",
			"OCC_BACKUP_S3_SECRET":     "test-secret",
			"OCC_BACKUP_KEEP":          "2",
		}[k]
	})
	cfg.Backup.Now = clock.now
	e, h := newBackupEnv(t, cfg, nil)
	boss := e.admin("Boss")

	// startup applied the env configuration
	if b := e.app.Settings().Backups; !b.S3.Enabled || b.S3.Bucket != "occ-deliveries-backups" || b.Cron != DefaultBackupCron || b.CronMaxKeep != 2 || !b.S3.ForcePathStyle || b.S3.Region != "auto" {
		t.Fatalf("settings: %+v", b)
	}
	st := e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil)
	if strings.Contains(string(st.body), "test-secret") || strings.Contains(string(st.body), "test-access") {
		t.Fatal("credentials leaked")
	}
	if m := st.m(t); m["storage"] != "s3" || m["bucket"] != "occ-deliveries-backups" || m["fromEnv"] != true || m["health"] != "pending" || m["keep"] != float64(2) {
		t.Fatalf("status: %v", m)
	}

	for i := 0; i < 3; i++ {
		e.expect(202, "POST", "/api/occ/admin/backups/run", boss.token, nil)
		waitBackupIdle(t, h)
		time.Sleep(1100 * time.Millisecond) // S3 modification times have a second resolution
	}
	keys := fake.keys()
	if len(keys) != 2 {
		t.Fatalf("retention: %v", keys)
	}
	for _, k := range keys {
		if !strings.HasPrefix(k, backupAutoPrefix) {
			t.Fatalf("key %s", k)
		}
	}
	m := e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t)
	lv := m["lastVerification"].(map[string]any)
	if m["health"] != "ok" || lv["ok"] != true || lv["backupKey"] != keys[1] {
		t.Fatalf("status: %v", m)
	}

	// broken archives, each one becoming the newest
	future := time.Now().Add(time.Hour)
	broken := []struct {
		name, want string
		data       []byte
	}{
		{"pas un zip", "zip invalide", []byte("not a zip at all")},
		{"sans data.db", "ne contient pas data.db", zipWith(t, map[string][]byte{"auxiliary.db": []byte("x")})},
		{"base illisible", "", zipWith(t, map[string][]byte{"data.db": bytes.Repeat([]byte("garbage!"), 1024)})},
		{"base vide", "Tables absentes", zipWith(t, map[string][]byte{"data.db": sqliteFile(t, "CREATE TABLE x (id TEXT)")})},
	}
	for i, b := range broken {
		key := "zz_broken_" + string(rune('a'+i)) + ".zip"
		fake.put(key, b.data, future.Add(time.Duration(i)*time.Minute))
		rec, err := h.backups.verify(backupTriggerManual)
		if err != nil {
			t.Fatalf("%s: %v", b.name, err)
		}
		if rec.GetBool("ok") || rec.GetString("backup_key") != key || rec.GetString("error") == "" || !strings.Contains(rec.GetString("error"), b.want) {
			t.Fatalf("%s: ok=%v key=%s error=%q", b.name, rec.GetBool("ok"), rec.GetString("backup_key"), rec.GetString("error"))
		}
	}

	// S3 unreachable: listed as an error, verification fails cleanly
	fake.srv.Close()
	m = e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t)
	if !strings.Contains(m["listError"].(string), "injoignable") || m["lastBackup"] != nil {
		t.Fatalf("status: %v", m)
	}
}

func TestBackupStaleAlert(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)}
	cfg := testConfig
	cfg.Backup = BackupConfig{Now: clock.now}
	e, h := newBackupEnv(t, cfg, nil)
	boss := e.admin("Boss")
	e.admin("Chef")
	e.user("Alice")
	s := e.app.Settings()
	s.Backups.Cron, s.Backups.CronMaxKeep = DefaultBackupCron, 14
	if err := e.app.Save(s); err != nil {
		t.Fatal(err)
	}

	// no SMTP: nothing sent, nothing recorded
	clock.advance(40 * time.Hour)
	h.backups.watch()
	if e.app.TestMailer.TotalSend() != 0 || lastCheck(e.app, backupKindAlert, false) != nil {
		t.Fatal("no alert without SMTP")
	}
	if st := e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t); st["health"] != "stale" {
		t.Fatalf("health: %v", st["health"])
	}

	enableTestSMTP(e)
	h.backups.watch()
	if n := e.app.TestMailer.TotalSend(); n != 2 { // Boss and Chef, not Alice
		t.Fatalf("sent %d", n)
	}
	msg := e.app.TestMailer.LastMessage()
	if !strings.Contains(msg.Subject, "Aucune sauvegarde depuis 36 h") || !strings.Contains(msg.HTML, "/admin") || strings.Contains(msg.HTML, "{APP_") {
		t.Fatalf("mail: %s / %s", msg.Subject, msg.HTML)
	}
	// once per day
	clock.advance(3 * time.Hour)
	h.backups.watch()
	if n := e.app.TestMailer.TotalSend(); n != 2 {
		t.Fatalf("sent %d", n)
	}
	clock.advance(22 * time.Hour)
	h.backups.watch()
	if n := e.app.TestMailer.TotalSend(); n != 4 {
		t.Fatalf("sent %d", n)
	}
	if st := e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t); st["lastAlertAt"] == "" {
		t.Fatalf("lastAlertAt: %v", st)
	}

	// a fresh backup: healthy again, no more alert, the watch verified it
	if err := e.app.CreateBackup(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	waitBackupIdle(t, h)
	h.backups.cfg.Now = func() time.Time { return time.Now().Add(time.Hour) }
	clock.advance(48 * time.Hour)
	h.backups.watch()
	if n := e.app.TestMailer.TotalSend(); n != 4 {
		t.Fatalf("sent %d", n)
	}
	if st := e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t); st["health"] != "ok" {
		t.Fatalf("health: %v", st["health"])
	}
}

func TestBackupChecksRulesAreServerOnly(t *testing.T) {
	e := newEnv(t)
	boss := e.admin("Boss")
	col, err := e.app.FindCollectionByNameOrId(colBackupChecks)
	if err != nil {
		t.Fatal(err)
	}
	if col.ListRule != nil || col.ViewRule != nil || col.CreateRule != nil || col.UpdateRule != nil || col.DeleteRule != nil {
		t.Fatal("backup_checks rules must all be nil")
	}
	e.expect(403, "GET", "/api/collections/backup_checks/records", boss.token, nil)
	e.expect(403, "POST", "/api/collections/backup_checks/records", boss.token, map[string]any{"kind": "verify", "ok": true})
}

func TestBackupDisabledFromEnv(t *testing.T) {
	for _, tc := range []struct {
		v    string
		want bool
	}{{"", false}, {"0", false}, {"false", false}, {"1", true}, {"true", true}, {"oui", false}} {
		bc := backupConfigFromEnv(func(k string) string {
			return map[string]string{"OCC_BACKUP_DISABLED": tc.v, "OCC_BACKUP_S3_ENDPOINT": "https://x", "OCC_BACKUP_S3_BUCKET": "b",
				"OCC_BACKUP_S3_ACCESS_KEY": "a", "OCC_BACKUP_S3_SECRET": "s"}[k]
		})
		if bc.Disabled != tc.want {
			t.Fatalf("%q: %v", tc.v, bc.Disabled)
		}
	}
	// the switch wins over the S3 variables and clears the restored settings
	s := &core.Settings{}
	s.Backups = core.BackupsConfig{Cron: DefaultBackupCron, CronMaxKeep: 14, S3: core.S3Config{Enabled: true, Bucket: "prod", Endpoint: "https://x", Region: "auto", AccessKey: "a", Secret: "s"}}
	bc := BackupConfig{Disabled: true, Endpoint: "https://x", Bucket: "b", AccessKey: "a", Secret: "s", Cron: DefaultBackupCron, Keep: 14}
	if !applyBackupSettings(s, bc) || s.Backups != (core.BackupsConfig{}) {
		t.Fatalf("%+v", s.Backups)
	}
	if applyBackupSettings(s, bc) {
		t.Fatal("second call must be a no-op")
	}
}

// A restored copy started with OCC_BACKUP_DISABLED=1 never writes to (nor
// prunes) the production bucket configured in the restored settings.
func TestBackupDisabledRestoredCopy(t *testing.T) {
	prod := newFakeS3(t, "occ-deliveries-backups")
	prod.put("@auto_pb_backup_occ_deliveries_20261010001500.zip", []byte("archive de prod"), time.Now().Add(-24*time.Hour))
	cfg := testConfig
	cfg.Backup = BackupConfig{Disabled: true}
	e, h := newBackupEnv(t, cfg, func(ta *tests.TestApp) {
		// settings as restored from the production pb_data
		s := ta.Settings()
		s.Backups = core.BackupsConfig{Cron: DefaultBackupCron, CronMaxKeep: 1, S3: core.S3Config{
			Enabled: true, Bucket: "occ-deliveries-backups", Region: "auto", Endpoint: prod.srv.URL,
			AccessKey: "prod-access", Secret: "prod-secret", ForcePathStyle: true,
		}}
		if err := ta.Save(s); err != nil {
			t.Fatal(err)
		}
	})
	boss := e.admin("Boss")

	if b := e.app.Settings().Backups; b != (core.BackupsConfig{}) {
		t.Fatalf("restored backup settings must be cleared: %+v", b)
	}
	for _, j := range e.app.Cron().Jobs() {
		if j.Id() == "__pbAutoBackup__" {
			t.Fatal("PocketBase backup job must be removed")
		}
	}
	st := e.expect(200, "GET", "/api/occ/admin/backups", boss.token, nil).m(t)
	if st["disabledByEnv"] != true || st["health"] != "disabled" || st["configured"] != false || st["fromEnv"] != false || st["storage"] != "local" {
		t.Fatalf("status: %v", st)
	}
	for _, u := range []string{"/api/occ/admin/backups/run", "/api/occ/admin/backups/verify"} {
		r := e.expect(400, "POST", u, boss.token, nil)
		if !strings.Contains(string(r.body), "désactivées sur cette instance") {
			t.Fatalf("%s: %s", u, r.body)
		}
	}
	h.backups.watch()

	// /_/ cannot re-enable them
	su := e.superuser()
	r := e.expect(400, "PATCH", "/api/settings", su, map[string]any{"backups": map[string]any{"cron": "0 1 * * *", "cronMaxKeep": 3}})
	if !strings.Contains(string(r.body), "OCC_BACKUP_DISABLED") {
		t.Fatal(string(r.body))
	}
	e.expect(200, "PATCH", "/api/settings", su, map[string]any{"meta": map[string]any{"appName": "OCC copie"}})

	// even if S3 comes back by another path, no backup is sent to it
	s := e.app.Settings()
	s.Backups.S3 = core.S3Config{Enabled: true, Bucket: "occ-deliveries-backups", Region: "auto", Endpoint: prod.srv.URL, AccessKey: "a", Secret: "s", ForcePathStyle: true}
	if err := e.app.CreateBackup(t.Context(), "pb_backup_copie.zip"); err == nil {
		t.Fatal("backup to S3 must be refused")
	}
	h.backups.wait()
	if keys := prod.keys(); len(keys) != 1 || keys[0] != "@auto_pb_backup_occ_deliveries_20261010001500.zip" {
		t.Fatalf("production bucket touched: %v", keys)
	}
	prod.mu.Lock()
	defer prod.mu.Unlock()
	for _, req := range prod.requests {
		if !strings.HasPrefix(req, "GET ") && !strings.HasPrefix(req, "HEAD ") {
			t.Fatalf("write request to the production bucket: %s", req)
		}
	}
}

func TestBackupRestoreRoute(t *testing.T) {
	cfg := testConfig
	cfg.Backup = BackupConfig{}
	e, h := newBackupEnv(t, cfg, nil)
	boss, alice := e.admin("Boss"), e.user("Alice")
	restored := make(chan string, 2)
	h.backups.restoreFn = func(_ context.Context, name string) error {
		restored <- name
		return nil
	}

	if err := e.app.CreateBackup(t.Context(), "pb_backup_manual.zip"); err != nil {
		t.Fatal(err)
	}
	h.backups.wait()
	ok := map[string]any{"key": "pb_backup_manual.zip", "confirm": "RESTAURER"}

	e.expect(401, "POST", "/api/occ/admin/backups/restore", "", ok)
	e.expect(403, "POST", "/api/occ/admin/backups/restore", alice.token, ok)
	e.expect(400, "POST", "/api/occ/admin/backups/restore", boss.token, map[string]any{"confirm": "RESTAURER"})
	e.expect(400, "POST", "/api/occ/admin/backups/restore", boss.token, map[string]any{"key": "../data.db", "confirm": "RESTAURER"})
	e.expect(400, "POST", "/api/occ/admin/backups/restore", boss.token, map[string]any{"key": "pb_backup_manual.zip"})
	e.expect(404, "POST", "/api/occ/admin/backups/restore", boss.token, map[string]any{"key": "absent.zip", "confirm": "RESTAURER"})

	// busy → 409, nothing restored
	if !h.backups.acquire(backupRunVerify) {
		t.Fatal("acquire")
	}
	e.expect(409, "POST", "/api/occ/admin/backups/restore", boss.token, ok)
	h.backups.release()

	r := e.expect(202, "POST", "/api/occ/admin/backups/restore", boss.token, ok).m(t)
	if r["running"] != "restore" || r["key"] != "pb_backup_manual.zip" {
		t.Fatal(r)
	}
	h.backups.wait()
	if got := <-restored; got != "pb_backup_manual.zip" {
		t.Fatal(got)
	}
	// success: PocketBase restarts the process, the lock stays taken meanwhile
	if op := h.backups.runningOp(); op != "restore" {
		t.Fatalf("running: %q", op)
	}
	h.backups.release()

	// a failed restore releases the lock
	h.backups.restoreFn = func(context.Context, string) error { return errors.New("boom") }
	e.expect(202, "POST", "/api/occ/admin/backups/restore", boss.token, ok)
	waitBackupIdle(t, h)
}
