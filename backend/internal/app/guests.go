package app

import (
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/mails"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Guests without account (« lien magique, juste un prénom »): POST
// /api/occ/guest creates a users record flagged is_guest (placeholder e-mail,
// random password, unverified) from a valid party or team invite code and
// returns an auth token. Guests vote, order, mark ready and pay their share;
// they cannot create parties or teams, be admin (app or team) nor save a
// payout profile until they upgrade (POST /api/occ/me/upgrade or Google).

const fieldIsGuest = "is_guest"

const (
	msgGuestNoParty  = "Crée ton compte (gratuit) pour lancer une commande : en invité·e, tu peux rejoindre celles des collègues."
	msgGuestNoTeam   = "Crée ton compte (gratuit) pour créer une équipe."
	msgGuestNoPayout = "Crée ton compte pour enregistrer tes coordonnées de remboursement."
	msgTooMany       = "Trop de tentatives depuis cette connexion. Réessaie dans quelques minutes."
)

// Guest rate limits, per IP.
const (
	guestCreatePerHour   = 10
	invitePreviewPerMin  = 60
	guestCleanupJobID    = "occGuestCleanup"
	guestCleanupSchedule = "40 3 * * *" // UTC, daily
)

type guestState struct {
	create  *domain.RateLimiter
	preview *domain.RateLimiter
}

func newGuestState() *guestState {
	return &guestState{
		create:  domain.NewRateLimiter(guestCreatePerHour, time.Hour),
		preview: domain.NewRateLimiter(invitePreviewPerMin, time.Minute),
	}
}

func isGuestRecord(r *core.Record) bool {
	return r != nil && r.Collection().Name == colUsers && r.GetBool(fieldIsGuest)
}

func errTooManyRequests() error { return apis.NewTooManyRequestsError(msgTooMany, nil) }

type hookHandler = hook.Handler[*core.RequestEvent]

func (h *handlers) guestRoutes(g *router.RouterGroup[*core.RequestEvent], user *hookHandler) {
	g.POST("/guest", h.createGuest)
	g.POST("/me/upgrade", h.upgradeGuest).Bind(user)
}

// ------------------------------------------------------------------ hooks

func bindGuestHooks(app core.App) {
	// is_guest is server managed: a client never sets nor changes it.
	app.OnRecordCreateRequest(colUsers).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() {
			e.Record.Set(fieldIsGuest, false)
		}
		return e.Next()
	})
	app.OnRecordUpdateRequest(colUsers).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() && e.Record.GetBool(fieldIsGuest) != e.Record.Original().GetBool(fieldIsGuest) {
			return forbidden("Ce champ est géré par le serveur.")
		}
		return e.Next()
	})
	// Safety net (whatever the path, /_/ included): a guest is never admin.
	guestNotAdmin := func(e *core.RecordEvent) error {
		if e.Record.GetBool(fieldIsGuest) && e.Record.GetString("role") == roleAdmin {
			e.Record.Set("role", roleUser)
		}
		return e.Next()
	}
	app.OnRecordCreate(colUsers).BindFunc(guestNotAdmin)
	app.OnRecordUpdate(colUsers).BindFunc(guestNotAdmin)

	// Guests have a static (non refreshable) token: refreshing it issues a new
	// one of GuestTokenDays (sliding window while they use the app).
	app.OnRecordAuthRefreshRequest(colUsers).BindFunc(func(e *core.RecordAuthRefreshRequestEvent) error {
		if !isGuestRecord(e.Record) {
			return e.Next()
		}
		token, err := e.Record.NewStaticAuthToken(guestTokenTTL())
		if err != nil {
			return err
		}
		return e.JSON(http.StatusOK, map[string]any{"token": token, "record": e.Record})
	})

	// No payout profile for guests (rule + hook).
	noPayout := func(e *core.RecordRequestEvent) error {
		if isGuestRecord(e.Auth) {
			return forbidden(msgGuestNoPayout)
		}
		return e.Next()
	}
	app.OnRecordCreateRequest(colPayoutProfiles).BindFunc(noPayout)
	app.OnRecordUpdateRequest(colPayoutProfiles).BindFunc(noPayout)

	// Upgrade with Google: a signed-in guest whose Google account is not
	// linked yet becomes a regular account with the Google e-mail (PocketBase
	// links the logged auth record and marks it verified).
	app.OnRecordAuthWithOAuth2Request(colUsers).BindFunc(onGuestOAuth2)

	// Placeholder addresses (guests, deleted accounts) never receive e-mails.
	app.OnMailerSend().BindFunc(func(e *core.MailerEvent) error {
		if e.Message != nil && len(e.Message.To) > 0 {
			real := false
			for _, to := range e.Message.To {
				if !domain.IsPlaceholderEmail(to.Address) {
					real = true
				}
			}
			if !real {
				return nil
			}
		}
		return e.Next()
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if err := se.App.Cron().Add(guestCleanupJobID, guestCleanupSchedule, func() {
			n, err := cleanupGuests(se.App, time.Now())
			if err != nil {
				se.App.Logger().Error("guest cleanup failed", "error", err)
			} else if n > 0 {
				se.App.Logger().Info("inactive guests anonymised", "count", n)
			}
		}); err != nil {
			return err
		}
		return se.Next()
	})
}

func guestTokenTTL() time.Duration { return domain.GuestTokenDays * 24 * time.Hour }

func onGuestOAuth2(e *core.RecordAuthWithOAuth2RequestEvent) error {
	upgrading := e.Auth != nil && e.Record != nil && e.Auth.Id == e.Record.Id && isGuestRecord(e.Record)
	if upgrading {
		email := strings.TrimSpace(e.OAuth2User.Email)
		if email == "" {
			return badRequest("Ton compte Google ne fournit pas d'adresse e-mail.")
		}
		if other, err := e.App.FindAuthRecordByEmail(colUsers, email); err == nil && other.Id != e.Record.Id {
			return badRequest("Un compte existe déjà avec cette adresse Google : déconnecte-toi puis connecte-toi avec Google.")
		}
		e.Record.SetEmail(email)
		e.Record.Set(fieldIsGuest, false)
		e.Record.Set(fieldPasswordSet, false)
	}
	if err := e.Next(); err != nil {
		return err
	}
	if upgrading && e.Record.GetBool(fieldIsGuest) == false {
		// saved by PocketBase only when it had something to update
		if err := e.App.Save(e.Record); err != nil {
			e.App.Logger().Warn("guest upgrade (google) not saved", "user", e.Record.Id, "error", err)
		}
	}
	return nil
}

// --------------------------------------------------------------- handlers

// createGuest: POST /api/occ/guest {name, color?, partyCode | teamCode}.
func (h *handlers) createGuest(e *core.RequestEvent) error {
	if e.Auth != nil {
		return badRequest("Tu es déjà connecté·e : rejoins directement avec le lien.")
	}
	if !h.guests.create.Allow(e.RealIP(), time.Now()) {
		return errTooManyRequests()
	}
	var body struct {
		Name      string `json:"name"`
		Color     string `json:"color"`
		PartyCode string `json:"partyCode"`
		TeamCode  string `json:"teamCode"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	name, err := domain.NormalizeGuestName(body.Name)
	if err != nil {
		return toAPIError(err)
	}
	color, err := domain.NormalizeColor(body.Color)
	if err != nil {
		return toAPIError(err)
	}
	if color == "" {
		color = randomColor()
	}
	partyCode, teamCode := domain.NormalizeCode(body.PartyCode), domain.NormalizeCode(body.TeamCode)
	switch {
	case partyCode != "" && teamCode != "":
		return badRequest("Un seul lien d'invitation à la fois.")
	case partyCode != "" && !domain.IsValidCode(partyCode):
		return badRequest("Code de commande invalide.")
	case teamCode != "" && !domain.IsValidTeamCode(teamCode):
		return badRequest("Lien d'équipe invalide.")
	case partyCode == "" && teamCode == "":
		return badRequest("Un lien d'invitation valide est nécessaire pour continuer en invité·e.")
	}

	var guest, party, team *core.Record
	err = e.App.RunInTransaction(func(tx core.App) error {
		if partyCode != "" {
			p, err := tx.FindFirstRecordByData(colParties, "code", partyCode)
			if err != nil {
				return notFound("Aucune commande ne correspond à ce code.")
			}
			if !containsStatus(joinableStatuses, p.GetString("status")) {
				return badRequest("Cette commande n'accepte plus de nouveaux participants.")
			}
			party = p
		} else {
			t, err := tx.FindFirstRecordByData(colTeams, "code", teamCode)
			if err != nil {
				return notFound("Aucune équipe ne correspond à ce lien.")
			}
			if t.GetBool("archived") {
				return badRequest("Cette équipe est archivée.")
			}
			team = t
		}
		col, err := tx.FindCollectionByNameOrId(colUsers)
		if err != nil {
			return err
		}
		u := core.NewRecord(col)
		id := core.GenerateDefaultRandomId()
		u.Id = id
		u.SetEmail(domain.GuestEmail(id))
		u.SetEmailVisibility(false)
		u.SetVerified(false)
		u.SetRandomPassword()
		u.Set("name", name)
		u.Set("color", color)
		u.Set("role", roleUser)
		u.Set(fieldIsGuest, true)
		if err := tx.Save(u); err != nil {
			return err
		}
		guest = u
		if party != nil {
			return addPartyMember(tx, party, u.Id)
		}
		return addTeamMember(tx, team, u.Id)
	})
	if err != nil {
		return err
	}
	token, err := guest.NewStaticAuthToken(guestTokenTTL())
	if err != nil {
		return err
	}
	out := map[string]any{"token": token, "record": guest, "party": nil, "team": nil}
	if party != nil {
		out["party"] = map[string]any{"id": party.Id, "title": party.GetString("title")}
	}
	if team != nil {
		out["team"] = map[string]any{"id": team.Id, "name": team.GetString("name")}
	}
	e.App.Logger().Info("guest created", "user", guest.Id, "party", party != nil, "team", team != nil)
	return e.JSON(http.StatusOK, out)
}

func containsStatus(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// upgradeGuest: POST /api/occ/me/upgrade {email, password, passwordConfirm,
// name?} — the same record becomes a regular account (history kept).
func (h *handlers) upgradeGuest(e *core.RequestEvent) error {
	var body struct {
		Email           string `json:"email"`
		Password        string `json:"password"`
		PasswordConfirm string `json:"passwordConfirm"`
		Name            string `json:"name"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	u, err := e.App.FindRecordById(colUsers, e.Auth.Id)
	if err != nil {
		return notFound("Compte introuvable.")
	}
	if !isGuestRecord(u) {
		return badRequest("Ton compte est déjà un compte complet.")
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || domain.IsPlaceholderEmail(email) {
		return badRequest("Adresse e-mail invalide.")
	}
	if _, err := e.App.FindAuthRecordByEmail(colUsers, email); err == nil {
		return badRequest("Un compte existe déjà avec cette adresse : connecte-toi avec ce compte.")
	}
	if len([]rune(body.Password)) < 8 {
		return badRequest("Le mot de passe doit faire au moins 8 caractères.")
	}
	if body.PasswordConfirm != "" && body.PasswordConfirm != body.Password {
		return badRequest("Les deux mots de passe ne correspondent pas.")
	}
	if strings.TrimSpace(body.Name) != "" {
		name, err := domain.NormalizeGuestName(body.Name)
		if err != nil {
			return toAPIError(err)
		}
		u.Set("name", name)
	}
	u.SetEmail(email)
	u.SetPassword(body.Password)
	u.SetVerified(false)
	u.Set(fieldIsGuest, false)
	u.Set(fieldPasswordSet, true)
	if err := e.App.Save(u); err != nil {
		return err
	}
	if mailEnabled(e.App) {
		if err := mails.SendRecordVerification(e.App, u); err != nil {
			e.App.Logger().Warn("verification e-mail not sent", "user", u.Id, "error", err)
		}
	}
	e.App.Logger().Info("guest upgraded", "user", u.Id)
	return apis.RecordAuthResponse(e, u, core.MFAMethodPassword, nil)
}

// ----------------------------------------------------------------- cleanup

// cleanupGuests anonymises (same as an account deletion: history and totals
// kept) the guests without activity for GuestInactiveDays, and removes them
// from their teams. Returns the number of guests cleaned up.
func cleanupGuests(app core.App, now time.Time) (int, error) {
	guests, err := app.FindAllRecords(colUsers, dbx.HashExp{fieldIsGuest: true},
		dbx.Or(dbx.HashExp{fieldDeletedAt: ""}, dbx.NewExp("[["+fieldDeletedAt+"]] IS NULL")))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, g := range guests {
		last, err := lastActivity(app, g.Id)
		if err != nil {
			return n, err
		}
		if last.IsZero() {
			last = g.GetDateTime("created").Time()
		}
		if !domain.GuestInactive(last, now) {
			continue
		}
		teams, err := app.FindRecordsByFilter(colTeams, "members.id ?= {:u}", "", 0, 0, dbx.Params{"u": g.Id})
		if err != nil {
			return n, err
		}
		for _, t := range teams {
			t.Set("members-", g.Id)
			t.Set("admins-", g.Id)
			if err := app.Save(t); err != nil {
				return n, err
			}
		}
		if err := anonymizeUser(app, g); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
