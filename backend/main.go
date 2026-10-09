// Command occ is the OCC Deliveries server: PocketBase extended in Go,
// serving the /api/occ business API and the React SPA.
package main

import (
	"log"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/app"
	_ "github.com/fs0ciety7000/occ-deliveries/backend/migrations"
)

// version is overridden at build time: -ldflags "-X main.version=1.2.3".
var version = "0.1.0-dev"

func main() {
	pb := pocketbase.New()

	migratecmd.MustRegister(pb, pb.RootCmd, migratecmd.Config{
		TemplateLang: migratecmd.TemplateLangGo,
		Automigrate:  false,
		Dir:          "migrations",
	})

	cfg := app.ConfigFromEnv(version)
	app.Register(pb, cfg)

	pb.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if err := applySettings(se.App, cfg); err != nil {
			return err
		}
		if err := upsertSuperuser(se.App); err != nil {
			return err
		}
		return se.Next()
	})

	// SPA (pb_public) with index.html fallback, registered last so that
	// /api and /_ keep priority.
	pb.OnServe().Bind(&hookHandlerSPA)

	if err := pb.Start(); err != nil {
		log.Fatal(err)
	}
}

func publicDir() string {
	if d := strings.TrimSpace(os.Getenv("OCC_PUBLIC_DIR")); d != "" {
		return d
	}
	return "./pb_public"
}

func applySettings(a core.App, cfg app.Config) error {
	s := a.Settings()
	changed := false
	if s.Meta.AppName != "OCC Deliveries" {
		s.Meta.AppName = "OCC Deliveries"
		changed = true
	}
	if cfg.PublicURL != "" && s.Meta.AppURL != cfg.PublicURL {
		s.Meta.AppURL = cfg.PublicURL
		changed = true
	}
	if !changed {
		return nil
	}
	return a.Save(s)
}

func upsertSuperuser(a core.App) error {
	email := strings.TrimSpace(os.Getenv("OCC_ADMIN_EMAIL"))
	password := os.Getenv("OCC_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return nil
	}
	su, err := a.FindAuthRecordByEmail(core.CollectionNameSuperusers, email)
	if err != nil {
		col, err := a.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		if err != nil {
			return err
		}
		su = core.NewRecord(col)
		su.SetEmail(email)
	} else if su.ValidatePassword(password) {
		return nil
	}
	su.SetPassword(password)
	return a.Save(su)
}

var hookHandlerSPA = spaHandler()
