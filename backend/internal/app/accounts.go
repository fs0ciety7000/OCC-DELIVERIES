package app

import (
	"net/http"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/mails"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Hidden users fields (migration 1760000014).
const (
	fieldBanned       = "banned"
	fieldBannedReason = "banned_reason"
	fieldBannedAt     = "banned_at"
	fieldDeletedAt    = "deleted_at"
	fieldPasswordSet  = "password_set"
)

const (
	msgSuspended   = "Compte suspendu. Contacte un·e administrateur·rice d'OCC Deliveries."
	msgMailMissing = "L'envoi d'e-mails n'est pas configuré sur ce serveur (variables OCC_SMTP_*)."
)

func errSuspended() error { return apis.NewForbiddenError(msgSuspended, nil) }

func isBanned(r *core.Record) bool {
	return r != nil && r.Collection().Name == colUsers && r.GetBool(fieldBanned)
}

func isDeleted(r *core.Record) bool {
	return r != nil && !r.GetDateTime(fieldDeletedAt).IsZero()
}

// bannedGuard rejects every request carrying the token of a suspended user
// (defence in depth: suspending rotates the tokenKey, so such tokens are
// normally already invalid). Runs right after PocketBase loads e.Auth.
func bannedGuard() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       "occBannedGuard",
		Priority: apis.DefaultLoadAuthTokenMiddlewarePriority + 6,
		Func: func(e *core.RequestEvent) error {
			if isBanned(e.Auth) {
				return errSuspended()
			}
			return e.Next()
		},
	}
}

func bindAccountHooks(app core.App) {
	// No auth (password, OAuth2, OTP, refresh) for a suspended account.
	app.OnRecordAuthRequest(colUsers).BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if isBanned(e.Record) {
			return errSuspended()
		}
		return e.Next()
	})

	// Suspending (here, via the endpoints or in /_/) logs the user out
	// everywhere: new tokenKey = every issued token is invalid and the
	// realtime connections lose their auth.
	app.OnRecordUpdate(colUsers).BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetBool(fieldBanned) && !e.Record.Original().GetBool(fieldBanned) {
			e.Record.RefreshTokenKey()
			if e.Record.GetDateTime(fieldBannedAt).IsZero() {
				e.Record.Set(fieldBannedAt, types.NowDateTime())
			}
		}
		return e.Next()
	})

	// password_set: a plain password was given (sign-up, /_/, Go code); a
	// Google sign-up gets a random password whose plain value PocketBase
	// clears before saving → false.
	app.OnRecordCreate(colUsers).BindFunc(func(e *core.RecordEvent) error {
		raw, _ := e.Record.GetRaw(core.FieldNamePassword).(*core.PasswordFieldValue)
		e.Record.Set(fieldPasswordSet, raw != nil && raw.Plain != "")
		return e.Next()
	})
	app.OnRecordCreateRequest(colUsers).BindFunc(func(e *core.RecordRequestEvent) error {
		oauth := requestContext(e.RequestEvent) == core.RequestInfoContextOAuth2
		if err := e.Next(); err != nil {
			return err
		}
		// verification e-mail for password sign-ups (Google ones are verified)
		if !oauth && !e.Record.Verified() && mailEnabled(e.App) {
			if err := mails.SendRecordVerification(e.App, e.Record); err != nil {
				e.App.Logger().Warn("verification e-mail not sent", "user", e.Record.Id, "error", err)
			}
		}
		return nil
	})
	app.OnRecordUpdateRequest(colUsers).BindFunc(func(e *core.RecordRequestEvent) error {
		if info, err := e.RequestInfo(); err == nil {
			if pw, _ := info.Body["password"].(string); pw != "" {
				e.Record.Set(fieldPasswordSet, true)
			}
		}
		return e.Next()
	})
	app.OnRecordConfirmPasswordResetRequest(colUsers).BindFunc(func(e *core.RecordConfirmPasswordResetRequestEvent) error {
		e.Record.Set(fieldPasswordSet, true) // saved by the default handler
		return e.Next()
	})

	app.OnRecordAuthWithOAuth2Request(colUsers).BindFunc(onOAuth2)

	// An external auth can be removed only if another way to sign in remains.
	app.OnRecordDeleteRequest(core.CollectionNameExternalAuths).BindFunc(func(e *core.RecordRequestEvent) error {
		if err := checkUnlink(e.App, e.Record.GetString("collectionRef"), e.Record.GetString("recordRef"), e.Record.Id); err != nil {
			return err
		}
		return e.Next()
	})
}

func requestContext(e *core.RequestEvent) string {
	info, err := e.RequestInfo()
	if err != nil {
		return ""
	}
	return info.Context
}

// onOAuth2 (Google): refuse suspended accounts before linking anything,
// default name for a new account, and track the password randomisation that
// PocketBase applies when it links an unverified account by e-mail.
func onOAuth2(e *core.RecordAuthWithOAuth2RequestEvent) error {
	if isBanned(e.Record) {
		return errSuspended()
	}
	if e.IsNewRecord {
		if e.CreateData == nil {
			e.CreateData = map[string]any{}
		}
		if _, ok := e.CreateData["name"]; !ok && strings.TrimSpace(e.OAuth2User.Name) == "" {
			e.CreateData["name"] = nameFromEmail(e.OAuth2User.Email)
		}
	} else if e.Record != nil {
		loggedSelf := e.Auth != nil && e.Auth.Id == e.Record.Id
		if !loggedSelf && !e.Record.Verified() {
			// PocketBase gives it a random password (account takeover guard)
			e.Record.Set(fieldPasswordSet, false)
		}
	}
	if err := e.Next(); err != nil {
		return err
	}
	if e.Record != nil && strings.TrimSpace(e.Record.GetString("name")) == "" {
		name := strings.TrimSpace(e.OAuth2User.Name)
		if name == "" {
			name = nameFromEmail(e.Record.Email())
		}
		e.Record.Set("name", name)
		if err := e.App.Save(e.Record); err != nil {
			e.App.Logger().Warn("oauth2: default name not saved", "user", e.Record.Id, "error", err)
		}
	}
	return nil
}

// nameFromEmail: "alice.dupont@x.be" → "Alice Dupont" (max 60 runes).
func nameFromEmail(email string) string {
	local, _, _ := strings.Cut(email, "@")
	parts := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '-' || r == '+' })
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	name := strings.Join(parts, " ")
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	if name == "" {
		name = "Collègue"
	}
	return name
}

// checkUnlink refuses to remove the last external auth of an account that
// has no password of its own (it could not sign in anymore).
func checkUnlink(app core.App, collectionRef, recordRef, externalAuthID string) error {
	users, err := app.FindCachedCollectionByNameOrId(colUsers)
	if err != nil || collectionRef != users.Id {
		return nil
	}
	u, err := app.FindRecordById(colUsers, recordRef)
	if err != nil || u.GetBool(fieldPasswordSet) {
		return nil
	}
	others, err := app.CountRecords(core.CollectionNameExternalAuths,
		dbx.HashExp{"collectionRef": collectionRef, "recordRef": recordRef}, dbx.Not(dbx.HashExp{"id": externalAuthID}))
	if err != nil {
		return err
	}
	if others == 0 {
		return badRequest("Choisis d'abord un mot de passe (Profil → Sécurité) : sans lui, tu ne pourrais plus te connecter.")
	}
	return nil
}

// ------------------------------------------------------------ me/account

type linkedProvider struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Created  string `json:"created"`
}

type accountInfo struct {
	Email       string           `json:"email"`
	Verified    bool             `json:"verified"`
	PasswordSet bool             `json:"passwordSet"`
	Providers   []linkedProvider `json:"providers"`
	MailEnabled bool             `json:"mailEnabled"`
}

func linkedProviders(app core.App, u *core.Record) ([]linkedProvider, error) {
	list, err := app.FindAllExternalAuthsByRecord(u)
	if err != nil {
		return nil, err
	}
	out := make([]linkedProvider, 0, len(list))
	for _, ea := range list {
		out = append(out, linkedProvider{ID: ea.Id, Provider: ea.Provider(), Created: ea.Created().String()})
	}
	return out, nil
}

func (h *handlers) meAccount(e *core.RequestEvent) error {
	u, err := e.App.FindRecordById(colUsers, e.Auth.Id)
	if err != nil {
		return notFound("Compte introuvable.")
	}
	providers, err := linkedProviders(e.App, u)
	if err != nil {
		return err
	}
	return ok(e, accountInfo{
		Email: u.Email(), Verified: u.Verified(), PasswordSet: u.GetBool(fieldPasswordSet),
		Providers: providers, MailEnabled: mailEnabled(e.App),
	})
}

func (h *handlers) meUnlinkProvider(e *core.RequestEvent) error {
	provider := e.Request.PathValue("provider")
	ea, err := e.App.FindFirstExternalAuthByExpr(dbx.HashExp{
		"collectionRef": e.Auth.Collection().Id, "recordRef": e.Auth.Id, "provider": provider,
	})
	if err != nil {
		return notFound("Ce compte n'est pas lié à " + provider + ".")
	}
	if err := checkUnlink(e.App, ea.CollectionRef(), ea.RecordRef(), ea.Id); err != nil {
		return err
	}
	if err := e.App.Delete(ea); err != nil {
		return err
	}
	return ok(e, map[string]any{"ok": true})
}

// meDelete is the self-service (RGPD) account deletion = anonymisation.
func (h *handlers) meDelete(e *core.RequestEvent) error {
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	if err := domain.CheckDeleteConfirmation(body.Confirm); err != nil {
		return toAPIError(err)
	}
	u, err := e.App.FindRecordById(colUsers, e.Auth.Id)
	if err != nil {
		return notFound("Compte introuvable.")
	}
	admins, err := activeAdmins(e.App)
	if err != nil {
		return err
	}
	hosted, err := e.App.CountRecords(colParties, dbx.HashExp{"host": u.Id},
		dbx.NotIn("status", domain.StatusClosed, domain.StatusCancelled))
	if err != nil {
		return err
	}
	var unsettled int
	err = e.App.DB().NewQuery(`SELECT COUNT(*) FROM payments p JOIN parties pa ON pa.id = p.party
		WHERE p.creditor = {:u} AND p.debtor != {:u} AND p.status != 'confirmed' AND pa.status = {:paying}`).
		Bind(dbx.Params{"u": u.Id, "paying": domain.StatusPaying}).Row(&unsettled)
	if err != nil {
		return err
	}
	if err := domain.CheckSelfDelete(u.GetString("role") == roleAdmin, admins, int(hosted), unsettled); err != nil {
		return toAPIError(err)
	}
	if err := anonymizeUser(e.App, u); err != nil {
		return err
	}
	e.App.Logger().Info("account deleted by its owner", "user", u.Id)
	return e.NoContent(http.StatusNoContent)
}

// activeAdmins counts the users with the admin role, neither banned nor deleted.
func activeAdmins(app core.App) (int, error) {
	n, err := app.CountRecords(colUsers, dbx.HashExp{"role": roleAdmin, fieldBanned: false},
		dbx.Or(dbx.HashExp{fieldDeletedAt: ""}, dbx.NewExp("[["+fieldDeletedAt+"]] IS NULL")))
	return int(n), err
}

// anonymizeUser « deletes » an account while keeping the history of the
// parties (order lines, payments, amounts): personal data is wiped, every
// way to sign in is removed and the account is suspended.
func anonymizeUser(app core.App, u *core.Record) error {
	return app.RunInTransaction(func(tx core.App) error {
		profiles, err := tx.FindAllRecords(colPayoutProfiles, dbx.HashExp{"user": u.Id})
		if err != nil {
			return err
		}
		for _, p := range profiles {
			if err := tx.Delete(p); err != nil {
				return err
			}
		}
		for _, del := range []func(*core.Record) error{
			tx.DeleteAllExternalAuthsByRecord, tx.DeleteAllAuthOriginsByRecord,
			tx.DeleteAllMFAsByRecord, tx.DeleteAllOTPsByRecord,
		} {
			if err := del(u); err != nil {
				return err
			}
		}
		now := types.NowDateTime()
		u.Set("name", domain.DeletedAccountName)
		u.SetEmail(domain.AnonymizedEmail(u.Id))
		u.SetEmailVisibility(false)
		u.SetVerified(false)
		u.Set("avatar", "")
		u.Set("role", roleUser)
		u.SetRandomPassword()
		u.RefreshTokenKey()
		u.Set(fieldBanned, true)
		u.Set(fieldBannedReason, domain.DeletedAccountName)
		if u.GetDateTime(fieldBannedAt).IsZero() {
			u.Set(fieldBannedAt, now)
		}
		u.Set(fieldDeletedAt, now)
		u.Set(fieldPasswordSet, false)
		return tx.Save(u)
	})
}
