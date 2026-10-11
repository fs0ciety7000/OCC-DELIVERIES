package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// /api/occ/admin/backups* (admin): state of the automatic backups, manual
// backup, restore verification. Never returns a credential.

const maxBackupsListed = 30

type backupFileView struct {
	Key      string `json:"key"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

type backupCheckView struct {
	ID             string              `json:"id"`
	Kind           string              `json:"kind"`
	Trigger        string              `json:"trigger"`
	OK             bool                `json:"ok"`
	BackupKey      string              `json:"backupKey"`
	BackupSize     int64               `json:"backupSize"`
	BackupModified string              `json:"backupModified"`
	DurationMs     int64               `json:"durationMs"`
	Integrity      string              `json:"integrity"`
	Error          string              `json:"error"`
	Warnings       []string            `json:"warnings"`
	Counts         []domain.TableCount `json:"counts"`
	Created        string              `json:"created"`
}

type backupStatusView struct {
	Configured       bool             `json:"configured"`
	FromEnv          bool             `json:"fromEnv"`
	Storage          string           `json:"storage"` // "s3" | "local"
	Bucket           string           `json:"bucket"`
	Cron             string           `json:"cron"`
	Timezone         string           `json:"timezone"`
	Keep             int              `json:"keep"`
	NextBackupAt     *string          `json:"nextBackupAt"`
	Running          *string          `json:"running"`
	Health           string           `json:"health"`
	MaxAgeHours      int              `json:"maxAgeHours"`
	LastBackup       *backupFileView  `json:"lastBackup"`
	Backups          []backupFileView `json:"backups"`
	TotalBackups     int              `json:"totalBackups"`
	ListError        string           `json:"listError"`
	LastVerification *backupCheckView `json:"lastVerification"`
	LastBackupRun    *backupCheckView `json:"lastBackupRun"`
	LastAlertAt      string           `json:"lastAlertAt"`
}

func (h *handlers) backupRoutes(admin *router.RouterGroup[*core.RequestEvent]) {
	admin.GET("/backups", h.adminBackups)
	admin.POST("/backups/run", h.adminBackupRun)
	admin.POST("/backups/verify", h.adminBackupVerify)
}

func checkView(r *core.Record) *backupCheckView {
	if r == nil {
		return nil
	}
	v := &backupCheckView{
		ID: r.Id, Kind: r.GetString("kind"), Trigger: r.GetString("trigger"), OK: r.GetBool("ok"),
		BackupKey: r.GetString("backup_key"), BackupSize: int64(r.GetInt("backup_size")),
		BackupModified: isoDate(r.GetDateTime("backup_modified")), DurationMs: int64(r.GetInt("duration_ms")),
		Integrity: r.GetString("integrity"), Error: r.GetString("error"),
		Warnings: []string{}, Counts: []domain.TableCount{}, Created: isoDate(r.GetDateTime("created")),
	}
	_ = r.UnmarshalJSONField("warnings", &v.Warnings)
	_ = r.UnmarshalJSONField("counts", &v.Counts)
	if v.Warnings == nil {
		v.Warnings = []string{}
	}
	if v.Counts == nil {
		v.Counts = []domain.TableCount{}
	}
	return v
}

func fileView(o domain.BackupObject) backupFileView {
	return backupFileView{Key: o.Key, Size: o.Size, Modified: isoTime(o.Modified)}
}

func (h *handlers) backupStatus(app core.App) backupStatusView {
	s := app.Settings()
	b := h.backups
	out := backupStatusView{
		Configured: s.Backups.Cron != "", FromEnv: b.cfg.Enabled(), Storage: "local",
		Cron: s.Backups.Cron, Timezone: "UTC", Keep: s.Backups.CronMaxKeep,
		Health: string(domain.BackupDisabled), MaxAgeHours: int(domain.BackupMaxAge / time.Hour),
		Backups: []backupFileView{},
	}
	if s.Backups.S3.Enabled {
		out.Storage, out.Bucket = "s3", s.Backups.S3.Bucket
	}
	if t := nextCronRun(s.Backups.Cron, b.now()); t != nil {
		iso := isoTime(*t)
		out.NextBackupAt = &iso
	}
	if op := b.runningOp(); op != "" {
		out.Running = &op
	} else if app.Store().Has(core.StoreKeyActiveBackup) {
		op := backupRunBackup // scheduled backup or /_/ operation in progress
		out.Running = &op
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	objs, err := listBackups(ctx, app)
	cancel()
	var latest *domain.BackupObject
	if err != nil {
		out.ListError = "Stockage des sauvegardes injoignable : " + safeErr(err)
	} else {
		out.TotalBackups = len(objs)
		for i, o := range objs {
			if i == 0 {
				l := o
				latest = &l
				v := fileView(o)
				out.LastBackup = &v
			}
			if i < maxBackupsListed {
				out.Backups = append(out.Backups, fileView(o))
			}
		}
	}
	out.Health = string(b.health(latest))
	out.LastVerification = checkView(lastCheck(app, backupKindVerify, false))
	out.LastBackupRun = checkView(lastCheck(app, backupKindBackup, false))
	if a := lastCheck(app, backupKindAlert, false); a != nil {
		out.LastAlertAt = isoDate(a.GetDateTime("created"))
	}
	return out
}

func (h *handlers) adminBackups(e *core.RequestEvent) error {
	return ok(e, h.backupStatus(e.App))
}

const msgBackupBusy = "Une sauvegarde ou une vérification est déjà en cours. Réessaie dans quelques minutes."

func (h *handlers) adminBackupRun(e *core.RequestEvent) error {
	if err := h.backups.startBackup(); err != nil {
		if errors.Is(err, errBackupBusy) {
			return apis.NewApiError(http.StatusConflict, msgBackupBusy, nil)
		}
		return err
	}
	e.App.Logger().Info("manual backup requested", "by", e.Auth.Id)
	return e.JSON(http.StatusAccepted, map[string]any{"started": true, "running": backupRunBackup})
}

func (h *handlers) adminBackupVerify(e *core.RequestEvent) error {
	if err := h.backups.startVerify(); err != nil {
		if errors.Is(err, errBackupBusy) {
			return apis.NewApiError(http.StatusConflict, msgBackupBusy, nil)
		}
		return err
	}
	return e.JSON(http.StatusAccepted, map[string]any{"started": true, "running": backupRunVerify})
}
