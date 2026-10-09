package app

import (
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/mails"
	"github.com/pocketbase/pocketbase/tools/mailer"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// ------------------------------------------------------------------ users

type adminUser struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Email        string   `json:"email"`
	Role         string   `json:"role"`
	Color        string   `json:"color"`
	Avatar       string   `json:"avatar"`
	Verified     bool     `json:"verified"`
	Created      string   `json:"created"`
	Parties      int      `json:"parties"`
	Banned       bool     `json:"banned"`
	BannedReason string   `json:"bannedReason"`
	BannedAt     string   `json:"bannedAt"`
	Deleted      bool     `json:"deleted"`
	DeletedAt    string   `json:"deletedAt"`
	PasswordSet  bool     `json:"passwordSet"`
	Providers    []string `json:"providers"`
	LastLoginAt  string   `json:"lastLoginAt"`
}

func dateString(r *core.Record, field string) string {
	d := r.GetDateTime(field)
	if d.IsZero() {
		return ""
	}
	return d.String()
}

func toAdminUser(app core.App, r *core.Record) adminUser {
	role := r.GetString("role")
	if role == "" {
		role = roleUser
	}
	n, _ := app.CountRecords(colPartyMembers, dbx.HashExp{"user": r.Id})
	out := adminUser{
		ID: r.Id, Name: r.GetString("name"), Email: r.Email(), Role: role,
		Color: r.GetString("color"), Avatar: r.GetString("avatar"), Verified: r.Verified(),
		Created: r.GetDateTime("created").String(), Parties: int(n),
		Banned: r.GetBool(fieldBanned), BannedReason: r.GetString(fieldBannedReason), BannedAt: dateString(r, fieldBannedAt),
		Deleted: isDeleted(r), DeletedAt: dateString(r, fieldDeletedAt), PasswordSet: r.GetBool(fieldPasswordSet),
		Providers: []string{},
	}
	if list, err := app.FindAllExternalAuthsByRecord(r); err == nil {
		for _, ea := range list {
			out.Providers = append(out.Providers, ea.Provider())
		}
	}
	// last sign-in = most recent auth origin (password / OAuth2 logins)
	var last string
	_ = app.DB().NewQuery("SELECT COALESCE(MAX(updated), '') FROM _authOrigins WHERE recordRef = {:id} AND collectionRef = {:col}").
		Bind(dbx.Params{"id": r.Id, "col": r.Collection().Id}).Row(&last)
	out.LastLoginAt = last
	return out
}

// adminUsers lists the accounts. Filters: q (name / e-mail), role
// (user|admin), status (banned|unverified|deleted).
func (h *handlers) adminUsers(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("perPage"))
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}
	conds := []dbx.Expression{}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s) + "%"
		conds = append(conds, dbx.NewExp(`([[name]] LIKE {:q} ESCAPE '\' OR [[email]] LIKE {:q} ESCAPE '\')`, dbx.Params{"q": like}))
	}
	if role := q.Get("role"); role == roleAdmin || role == roleUser {
		conds = append(conds, dbx.HashExp{"role": role})
	}
	notDeleted := dbx.Or(dbx.HashExp{fieldDeletedAt: ""}, dbx.NewExp("[["+fieldDeletedAt+"]] IS NULL"))
	switch q.Get("status") {
	case "banned":
		conds = append(conds, dbx.HashExp{fieldBanned: true}, notDeleted)
	case "unverified":
		conds = append(conds, dbx.HashExp{"verified": false}, notDeleted)
	case "deleted":
		conds = append(conds, dbx.Not(notDeleted))
	}
	users, err := e.App.FindCachedCollectionByNameOrId(colUsers)
	if err != nil {
		return err
	}
	query := func() *dbx.SelectQuery {
		q := e.App.RecordQuery(users)
		if len(conds) > 0 {
			q = q.AndWhere(dbx.And(conds...))
		}
		return q
	}
	var total int
	if err := query().Select("COUNT(*)").Row(&total); err != nil {
		return err
	}
	recs := []*core.Record{}
	err = query().OrderBy("created DESC", "id DESC").
		Offset(int64((page - 1) * perPage)).Limit(int64(perPage)).All(&recs)
	if err != nil {
		return err
	}
	items := make([]adminUser, 0, len(recs))
	for _, r := range recs {
		items = append(items, toAdminUser(e.App, r))
	}
	return ok(e, map[string]any{"page": page, "perPage": perPage, "totalItems": total, "items": items})
}

// targetUser loads the user of the path and the moderation context.
func targetUser(e *core.RequestEvent) (*core.Record, domain.ModerationTarget, error) {
	u, err := e.App.FindRecordById(colUsers, e.Request.PathValue("id"))
	if err != nil {
		return nil, domain.ModerationTarget{}, notFound("Utilisateur introuvable.")
	}
	return u, domain.ModerationTarget{
		ID: u.Id, IsAdmin: u.GetString("role") == roleAdmin, Banned: u.GetBool(fieldBanned), Deleted: isDeleted(u),
	}, nil
}

func actorID(e *core.RequestEvent) string {
	if e.Auth == nil || e.Auth.IsSuperuser() {
		return ""
	}
	return e.Auth.Id
}

func (h *handlers) moderate(e *core.RequestEvent, action domain.ModerationAction) (*core.Record, error) {
	u, t, err := targetUser(e)
	if err != nil {
		return nil, err
	}
	admins, err := activeAdmins(e.App)
	if err != nil {
		return nil, err
	}
	if err := domain.CheckModeration(action, actorID(e), t, admins); err != nil {
		return nil, toAPIError(err)
	}
	return u, nil
}

func (h *handlers) respondUser(e *core.RequestEvent, u *core.Record) error {
	fresh, err := e.App.FindRecordById(colUsers, u.Id)
	if err == nil {
		u = fresh
	}
	return ok(e, map[string]any{"user": toAdminUser(e.App, u)})
}

func (h *handlers) adminSetRole(e *core.RequestEvent) error {
	var body struct {
		Role string `json:"role"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	if body.Role != roleAdmin && body.Role != roleUser {
		return badRequest("Rôle inconnu (user ou admin).")
	}
	u, t, err := targetUser(e)
	if err != nil {
		return err
	}
	if t.Deleted {
		return badRequest("Ce compte a été supprimé.")
	}
	if body.Role == roleUser && t.IsAdmin {
		if u, err = h.moderate(e, domain.ActionDemote); err != nil {
			return err
		}
	} else if u.Id == actorID(e) && body.Role != roleAdmin {
		return badRequest("Vous ne pouvez pas retirer vos propres droits d'administration.")
	}
	u.Set("role", body.Role)
	if err := e.App.Save(u); err != nil {
		return err
	}
	return h.respondUser(e, u)
}

func (h *handlers) adminBan(e *core.RequestEvent) error {
	var body struct {
		Reason string `json:"reason"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	reason, err := domain.NormalizeBanReason(body.Reason)
	if err != nil {
		return toAPIError(err)
	}
	u, err := h.moderate(e, domain.ActionBan)
	if err != nil {
		return err
	}
	if u.GetBool(fieldBanned) {
		return badRequest("Ce compte est déjà suspendu.")
	}
	u.Set(fieldBanned, true)
	u.Set(fieldBannedReason, reason)
	u.Set(fieldBannedAt, types.NowDateTime())
	u.RefreshTokenKey() // logs out every session (also done by the update hook)
	if err := e.App.Save(u); err != nil {
		return err
	}
	e.App.Logger().Info("user suspended", "user", u.Id, "by", e.Auth.Id)
	return h.respondUser(e, u)
}

func (h *handlers) adminUnban(e *core.RequestEvent) error {
	u, t, err := targetUser(e)
	if err != nil {
		return err
	}
	if t.Deleted {
		return badRequest("Ce compte a été supprimé : il ne peut plus être réactivé.")
	}
	if !t.Banned {
		return badRequest("Ce compte n'est pas suspendu.")
	}
	u.Set(fieldBanned, false)
	u.Set(fieldBannedReason, "")
	u.Set(fieldBannedAt, "")
	if err := e.App.Save(u); err != nil {
		return err
	}
	e.App.Logger().Info("user reactivated", "user", u.Id, "by", e.Auth.Id)
	return h.respondUser(e, u)
}

// adminLogout rotates the tokenKey: every session of the user ends.
func (h *handlers) adminLogout(e *core.RequestEvent) error {
	u, t, err := targetUser(e)
	if err != nil {
		return err
	}
	if t.Deleted {
		return badRequest("Ce compte a été supprimé.")
	}
	u.RefreshTokenKey()
	if err := e.App.Save(u); err != nil {
		return err
	}
	return h.respondUser(e, u)
}

// adminPasswordReset e-mails a reset link to the user (nobody ever sees a
// password). Needs SMTP.
func (h *handlers) adminPasswordReset(e *core.RequestEvent) error {
	u, t, err := targetUser(e)
	if err != nil {
		return err
	}
	if t.Deleted {
		return badRequest("Ce compte a été supprimé.")
	}
	if !mailEnabled(e.App) {
		return badRequest(msgMailMissing)
	}
	if err := mails.SendRecordPasswordReset(e.App, u); err != nil {
		e.App.Logger().Warn("password reset e-mail failed", "user", u.Id, "error", err)
		return badRequest("Envoi impossible : " + err.Error())
	}
	return ok(e, map[string]any{"sent": true, "email": u.Email()})
}

// adminDeleteUser anonymises the account (history kept, see anonymizeUser).
func (h *handlers) adminDeleteUser(e *core.RequestEvent) error {
	u, err := h.moderate(e, domain.ActionDelete)
	if err != nil {
		return err
	}
	if err := anonymizeUser(e.App, u); err != nil {
		return err
	}
	e.App.Logger().Info("account deleted by an admin", "user", u.Id, "by", e.Auth.Id)
	return h.respondUser(e, u)
}

// ------------------------------------------------------------------- mail

type mailStatus struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	TLS           bool   `json:"tls"`
	SenderAddress string `json:"senderAddress"`
	SenderName    string `json:"senderName"`
	FromEnv       bool   `json:"fromEnv"`
}

func (h *handlers) adminMail(e *core.RequestEvent) error {
	s := e.App.Settings()
	st := mailStatus{
		Enabled: s.SMTP.Enabled, SenderAddress: s.Meta.SenderAddress, SenderName: s.Meta.SenderName,
		FromEnv: h.cfg.Mail.Host != "",
	}
	if s.SMTP.Enabled {
		st.Host, st.Port, st.TLS = s.SMTP.Host, s.SMTP.Port, s.SMTP.TLS
	}
	return ok(e, st)
}

// adminMailTest sends a test e-mail to the requesting admin.
func (h *handlers) adminMailTest(e *core.RequestEvent) error {
	if !mailEnabled(e.App) {
		return badRequest(msgMailMissing)
	}
	to := e.Auth.Email()
	if to == "" {
		return badRequest("Ton compte n'a pas d'adresse e-mail.")
	}
	s := e.App.Settings()
	msg := &mailer.Message{
		From:    mail.Address{Name: s.Meta.SenderName, Address: s.Meta.SenderAddress},
		To:      []mail.Address{{Address: to}},
		Subject: "E-mail de test — " + s.Meta.AppName,
		HTML:    testMailBody(s.Meta.AppURL),
	}
	if err := e.App.NewMailClient().Send(msg); err != nil {
		e.App.Logger().Warn("test e-mail failed", "error", err)
		return badRequest("Envoi impossible : " + err.Error())
	}
	return e.JSON(http.StatusOK, map[string]any{"sent": true, "to": to})
}
