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
}

// ConfigFromEnv reads the OCC_* environment variables.
func ConfigFromEnv(version string) Config {
	cfg := Config{
		Version:      version,
		PublicURL:    envOr("OCC_PUBLIC_URL", "http://localhost:8090"),
		DefaultLat:   envFloat("OCC_DEFAULT_LAT", 50.4542),
		DefaultLng:   envFloat("OCC_DEFAULT_LNG", 3.9567),
		DefaultLabel: envOr("OCC_DEFAULT_LABEL", "Mons"),
	}
	for _, p := range strings.Split(envOr("OCC_PROVIDERS", "ubereats,takeaway"), ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if providers.IsPlatform(p) && !slices.Contains(cfg.Providers, p) {
			cfg.Providers = append(cfg.Providers, p)
		}
	}
	return cfg
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
	h := &handlers{cfg: cfg}
	bindHooks(app)
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		h.routes(se.Router)
		return se.Next()
	})
}

type handlers struct {
	cfg Config
}

func (h *handlers) routes(r *router.Router[*core.RequestEvent]) {
	g := r.Group("/api/occ")
	g.GET("/health", h.health)
	g.GET("/config", h.config)
	g.GET("/restaurants/nearby", h.nearby)

	user := apis.RequireAuth(colUsers)
	g.POST("/parties/join", h.join).Bind(user)
	g.POST("/parties/{id}/leave", h.leave).Bind(user)
	g.POST("/parties/{id}/transition", h.transition).Bind(user)
	g.POST("/parties/{id}/ready", h.ready).Bind(user)
	g.GET("/parties/{id}/summary", h.summary).Bind(user)
	g.POST("/parties/{id}/dispatch", h.dispatch).Bind(user)
	g.POST("/parties/{id}/payer", h.payer).Bind(user)
	g.GET("/parties/{id}/export", h.export).Bind(user)
	g.POST("/payments/{id}/action", h.paymentAction).Bind(user)
	g.GET("/payments/{id}/qr", h.paymentQR).Bind(user)
	g.GET("/payments/{id}/wallet-qr/{kind}", h.walletQR).Bind(user)

	g.POST("/admin/import", h.adminImport).Bind(apis.RequireSuperuserAuth())
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
