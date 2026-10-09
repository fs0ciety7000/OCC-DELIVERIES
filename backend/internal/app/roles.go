package app

import (
	"slices"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// User roles (users.role).
const (
	roleUser  = "user"
	roleAdmin = "admin"
)

// isAdmin reports whether the request comes from a superuser or a user with
// the "admin" role.
func isAdmin(e *core.RequestEvent) bool {
	if e.Auth == nil {
		return false
	}
	if e.Auth.IsSuperuser() {
		return true
	}
	return e.Auth.Collection().Name == colUsers && e.Auth.GetString("role") == roleAdmin
}

// requireAdmin guards /api/occ/admin/*: 401 without auth, 403 without the role.
func requireAdmin(e *core.RequestEvent) error {
	if e.Auth == nil {
		return apis.NewUnauthorizedError("Connexion requise.", nil)
	}
	if !isAdmin(e) {
		return forbidden("Réservé aux administrateurs.")
	}
	return e.Next()
}

func bindRoleHooks(app core.App, cfg Config) {
	// Default role, and bootstrap admins (OCC_ADMIN_EMAIL / OCC_ADMINS) when
	// they register — whatever the way (password, OAuth2, admin UI).
	app.OnRecordCreate(colUsers).BindFunc(func(e *core.RecordEvent) error {
		if slices.Contains(cfg.AdminEmails, strings.ToLower(e.Record.Email())) {
			e.Record.Set("role", roleAdmin)
		} else if e.Record.GetString("role") == "" {
			e.Record.Set("role", roleUser)
		}
		return e.Next()
	})

	// Nobody grants themselves a role through the collections API.
	app.OnRecordCreateRequest(colUsers).BindFunc(func(e *core.RecordRequestEvent) error {
		if role := e.Record.GetString("role"); role != "" && role != roleUser && !isAdmin(e.RequestEvent) {
			return forbidden("Vous ne pouvez pas choisir votre rôle.")
		}
		return e.Next()
	})
	app.OnRecordUpdateRequest(colUsers).BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Record.GetString("role") != e.Record.Original().GetString("role") && !isAdmin(e.RequestEvent) {
			return forbidden("Vous ne pouvez pas modifier votre rôle.")
		}
		return e.Next()
	})
}

// promoteAdmins gives the "admin" role to the existing users whose e-mail is
// in the bootstrap list.
func promoteAdmins(app core.App, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	in := make([]any, len(emails))
	for i, e := range emails {
		in[i] = e
	}
	recs, err := app.FindAllRecords(colUsers, dbx.In("LOWER(email)", in...), dbx.Not(dbx.HashExp{"role": roleAdmin}))
	if err != nil {
		return err
	}
	for _, r := range recs {
		r.Set("role", roleAdmin)
		if err := app.Save(r); err != nil {
			return err
		}
		app.Logger().Info("admin role granted", "email", r.Email())
	}
	return nil
}
