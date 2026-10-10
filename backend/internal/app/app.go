// Package app wires the OCC Deliveries business logic into PocketBase:
// record hooks (validation, server computed fields) and the /api/occ routes.
package app

import (
	"errors"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/search"
)

// Collection names.
const (
	colUsers          = "users"
	colRestaurants    = "restaurants"
	colMenuItems      = "menu_items"
	colParties        = "parties"
	colPartyMembers   = "party_members"
	colVotes          = "votes"
	colOrderItems     = "order_items"
	colPayments       = "payments"
	colPayoutProfiles = "payout_profiles"
)

// Config is the runtime configuration (see docs/ARCHITECTURE.md §7).
type Config struct {
	Version      string
	PublicURL    string
	DefaultLat   float64
	DefaultLng   float64
	DefaultLabel string
	// Providers are the enabled delivery platforms (OCC_PROVIDERS).
	Providers []string
	// AdminEmails get the "admin" role (OCC_ADMIN_EMAIL + OCC_ADMINS), lower-cased.
	AdminEmails []string
	// Sync drives the automatic menu synchronisation (OCC_SYNC_*).
	Sync SyncConfig
	// Mail is the SMTP configuration (OCC_SMTP_*, OCC_MAIL_*).
	Mail MailConfig
	// Google is the Google OAuth2 client (OCC_GOOGLE_CLIENT_*).
	Google GoogleConfig
	// Push is the Web Push configuration (OCC_VAPID_*, OCC_PUSH_ENABLED).
	Push PushConfig
	// WebAuthn is the passkeys relying party (OCC_WEBAUTHN_*, passkeys.go).
	WebAuthn WebAuthnConfig
}

// ConfigFromEnv reads the OCC_* environment variables.
func ConfigFromEnv(version string) Config {
	cfg := Config{
		Version:      version,
		PublicURL:    strings.TrimRight(envOr("OCC_PUBLIC_URL", "http://localhost:8090"), "/"),
		DefaultLat:   envFloat("OCC_DEFAULT_LAT", 50.4542),
		DefaultLng:   envFloat("OCC_DEFAULT_LNG", 3.9567),
		DefaultLabel: envOr("OCC_DEFAULT_LABEL", "Mons"),
	}
	cfg.Sync = syncConfigFromEnv()
	authConfigFromEnv(&cfg)
	cfg.Push = pushConfigFromEnv(cfg.Mail.From, cfg.PublicURL)
	cfg.WebAuthn = webAuthnConfigFromEnv()
	cfg.AdminEmails = parseEmails(os.Getenv("OCC_ADMIN_EMAIL") + "," + os.Getenv("OCC_ADMINS"))
	for _, p := range strings.Split(envOr("OCC_PROVIDERS", "ubereats,takeaway,deliveroo,weloveat"), ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if providers.IsPlatform(p) && !slices.Contains(cfg.Providers, p) {
			cfg.Providers = append(cfg.Providers, p)
		}
	}
	return cfg
}

// parseEmails splits a comma/semicolon/space separated list of e-mails.
func parseEmails(s string) []string {
	out := []string{}
	for _, e := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' }) {
		e = strings.ToLower(strings.TrimSpace(e))
		if strings.Contains(e, "@") && !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	return out
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(key)), 64); err == nil {
		return v
	}
	return def
}

func (c Config) providerEnabled(id string) bool {
	return slices.Contains(c.Providers, id)
}

// Register binds every hook and the /api/occ routes on the app.
func Register(app core.App, cfg Config) {
	register(app, cfg)
}

func register(app core.App, cfg Config) *handlers {
	if cfg.Sync.DefaultLat == 0 && cfg.Sync.DefaultLng == 0 {
		cfg.Sync.DefaultLat, cfg.Sync.DefaultLng = cfg.DefaultLat, cfg.DefaultLng
	}
	if cfg.Sync.DefaultLabel == "" {
		cfg.Sync.DefaultLabel = cfg.DefaultLabel
	}
	h := &handlers{cfg: cfg, sync: newSyncer(app, cfg.Sync), push: newPushService(app, cfg.Push), guests: newGuestState(),
		payoutAsk: domain.NewRateLimiter(1, payoutAskWindow)}
	h.sync.bind()
	h.push.bind()
	SetTeamNotifier(h.push)
	h.deadlines = newDeadlineScheduler(app, h.push, cfg.Push.Now)
	h.deadlines.bind()
	h.orderMail = newOrderMailer(app, cfg.PublicURL)
	h.orderMail.bind()
	bindHooks(app)
	bindRoleHooks(app, cfg)
	bindCatalogHooks(app)
	bindSyncHooks(app)
	bindSettingsHooks(app)
	bindAccountHooks(app)
	bindTeamHooks(app) // after bindHooks: runs inside onPartyCreate
	bindGuestHooks(app)
	h.passkeys = newPasskeyService(app, cfg)
	bindPasskeyHooks(app)
	search.Bind(app)
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if err := applyAuthSettings(se.App, cfg); err != nil {
			return err
		}
		if err := promoteAdmins(se.App, cfg.AdminEmails); err != nil {
			return err
		}
		se.Router.Bind(bannedGuard())
		h.routes(se.Router)
		return se.Next()
	})
	return h
}

type handlers struct {
	cfg       Config
	sync      *syncer
	push      *pushService
	deadlines *deadlineScheduler
	guests    *guestState         // rate limits of the guest endpoints
	orderMail *orderMailer        // « bon de commande » e-mail (ordermail.go)
	passkeys  *passkeyService     // WebAuthn sign-in (passkeys.go)
	payoutAsk *domain.RateLimiter // « ajoute ton IBAN » requests (payerguard.go)
}

func (h *handlers) routes(r *router.Router[*core.RequestEvent]) {
	g := r.Group("/api/occ")
	g.GET("/health", h.health)
	g.GET("/config", h.config)
	g.GET("/restaurants/nearby", h.nearby)
	g.GET("/search", h.search)

	user := apis.RequireAuth(colUsers)
	g.POST("/parties/join", h.join).Bind(user)
	g.POST("/parties/{id}/leave", h.leave).Bind(user)
	g.POST("/parties/{id}/transition", h.transition).Bind(user)
	g.POST("/parties/{id}/ready", h.ready).Bind(user)
	g.GET("/parties/{id}/summary", h.summary).Bind(user)
	g.POST("/parties/{id}/dispatch", h.dispatch).Bind(user)
	g.POST("/parties/{id}/payer", h.payer).Bind(user)
	g.GET("/parties/{id}/payout-readiness", h.payoutReadiness).Bind(user)
	g.POST("/parties/{id}/payout-request", h.payoutRequest).Bind(user)
	g.GET("/parties/{id}/export", h.export).Bind(user)
	g.GET("/parties/{id}/reorder", h.reorderPreview).Bind(user)
	g.POST("/parties/{id}/reorder", h.reorderApply).Bind(user)
	g.GET("/me/history", h.meHistory).Bind(user)
	g.GET("/me/stats", h.meStats).Bind(user)
	g.GET("/me/account", h.meAccount).Bind(user)
	g.DELETE("/me/providers/{provider}", h.meUnlinkProvider).Bind(user)
	g.POST("/me/delete", h.meDelete).Bind(user)
	h.pushRoutes(g)
	g.POST("/payments/{id}/action", h.paymentAction).Bind(user)
	g.GET("/payments/{id}/qr", h.paymentQR).Bind(user)
	g.GET("/parties/{id}/payments/qr", h.paymentsCollectQR).Bind(user)
	h.teamRoutes(g, user)
	h.guestRoutes(g, user)
	h.passkeyRoutes(g, user)
	h.voteRoutes(g, user)

	admin := g.Group("/admin")
	admin.BindFunc(requireAdmin)
	admin.GET("/stats", h.adminStats)
	admin.POST("/import", h.adminImport)
	admin.POST("/import/csv", h.adminImportCSV)
	admin.GET("/export", h.adminExport)
	admin.GET("/users", h.adminUsers)
	admin.PATCH("/users/{id}/role", h.adminSetRole)
	admin.POST("/users/{id}/ban", h.adminBan)
	admin.POST("/users/{id}/unban", h.adminUnban)
	admin.POST("/users/{id}/logout", h.adminLogout)
	admin.POST("/users/{id}/password-reset", h.adminPasswordReset)
	admin.DELETE("/users/{id}", h.adminDeleteUser)
	admin.GET("/mail", h.adminMail)
	admin.POST("/mail/test", h.adminMailTest)
	admin.POST("/parties/{id}/cancel", h.adminCancelParty)
	admin.GET("/sync/status", h.adminSyncStatus)
	admin.GET("/sync/runs", h.adminSyncRuns)
	admin.GET("/sync/runs/{id}", h.adminSyncRun)
	admin.POST("/sync/run", h.adminSyncStart)
	admin.POST("/sync/discover", h.adminSyncDiscover)
	admin.POST("/sync/sources", h.adminSyncAddSource)
	admin.GET("/settings", h.adminSettings)
	admin.PATCH("/settings", h.adminSetSettings)
}

// ---------------------------------------------------------------- helpers

// toAPIError converts business errors into 400 responses with their French message.
func toAPIError(err error) error {
	if err == nil {
		return nil
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return apis.NewBadRequestError(de.Msg, nil)
	}
	var ae *router.ApiError
	if errors.As(err, &ae) {
		return ae
	}
	return err
}

func badRequest(msg string) error { return apis.NewBadRequestError(msg, nil) }

func forbidden(msg string) error { return apis.NewForbiddenError(msg, nil) }

func notFound(msg string) error { return apis.NewNotFoundError(msg, nil) }

func errNotMember() error { return forbidden("Vous ne faites pas partie de cette commande.") }

func errNotHost() error { return forbidden("Seul l'hôte de la commande peut faire cela.") }

func findParty(app core.App, id string) (*core.Record, error) {
	p, err := app.FindRecordById(colParties, id)
	if err != nil {
		return nil, notFound("Commande introuvable.")
	}
	return p, nil
}

func isMember(party *core.Record, userID string) bool {
	return userID != "" && slices.Contains(party.GetStringSlice("members"), userID)
}

func isHost(party *core.Record, userID string) bool {
	return userID != "" && party.GetString("host") == userID
}

// partyForMember loads the party and checks the auth user is a member.
func partyForMember(e *core.RequestEvent) (*core.Record, error) {
	party, err := findParty(e.App, e.Request.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if !isMember(party, e.Auth.Id) {
		return nil, errNotMember()
	}
	return party, nil
}

// partyForHost loads the party and checks the auth user is the host.
func partyForHost(e *core.RequestEvent) (*core.Record, error) {
	party, err := partyForMember(e)
	if err != nil {
		return nil, err
	}
	if !isHost(party, e.Auth.Id) {
		return nil, errNotHost()
	}
	return party, nil
}

func bindJSON(e *core.RequestEvent, dst any) error {
	if err := e.BindBody(dst); err != nil {
		return badRequest("Corps de requête invalide.")
	}
	return nil
}

func ok(e *core.RequestEvent, data any) error {
	return e.JSON(http.StatusOK, data)
}
