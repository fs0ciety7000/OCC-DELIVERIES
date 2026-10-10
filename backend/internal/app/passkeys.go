package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Passkeys (WebAuthn, migration 1760000018). PocketBase has no native
// passkeys: the ceremonies are verified by go-webauthn behind custom routes,
// the credentials live in the `passkeys` collection (every rule nil) and a
// successful sign-in answers like PocketBase (apis.RecordAuthResponse →
// {token, record}, OnRecordAuthRequest hooks included: bans, MFA).
//
// Ceremony sessions are kept in memory (single instance), keyed by their
// challenge, single use and short lived: the finish step reads the challenge
// from the signed client data, takes the session and verifies against it.

const colPasskeys = "passkeys"

const (
	passkeyAuthMethod   = "passkey"
	passkeyTimeout      = 5 * time.Minute
	passkeySessionTTL   = 10 * time.Minute // conditional UI waits on the login page
	passkeySessionsMax  = 10000
	passkeyLoginPerMin  = 30 // login/begin per IP
	passkeysPerUserMax  = 20
	passkeyBodyMaxBytes = 64 << 10
	passkeyRPName       = "OCC Deliveries"
)

const (
	msgPasskeysOff     = "La connexion par passkey n'est pas configurée sur ce serveur."
	msgPasskeyGuest    = "Crée ton compte (gratuit) pour ajouter une passkey."
	msgPasskeyExpired  = "La demande a expiré ou a déjà servi. Réessaie."
	msgPasskeyInvalid  = "La passkey n'a pas pu être vérifiée. Réessaie."
	msgPasskeyUnknown  = "Cette passkey n'est plus liée à un compte OCC Deliveries. Connecte-toi autrement, puis supprime-la de ton appareil."
	msgPasskeyCloned   = "Cette passkey a été refusée par mesure de sécurité (compteur incohérent). Connecte-toi autrement et recrée-la."
	msgPasskeyExists   = "Cette passkey est déjà enregistrée."
	msgPasskeyNotFound = "Passkey introuvable."
)

// WebAuthnConfig: OCC_WEBAUTHN_RP_ID / OCC_WEBAUTHN_ORIGINS, both optional
// (derived from OCC_PUBLIC_URL).
type WebAuthnConfig struct {
	RPID    string
	Origins []string
}

func webAuthnConfigFromEnv() WebAuthnConfig {
	c := WebAuthnConfig{RPID: strings.TrimSpace(os.Getenv("OCC_WEBAUTHN_RP_ID"))}
	for _, o := range strings.FieldsFunc(os.Getenv("OCC_WEBAUTHN_ORIGINS"), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			c.Origins = append(c.Origins, o)
		}
	}
	return c
}

const (
	sessionRegister = "register"
	sessionLogin    = "login"
)

type passkeySession struct {
	kind   string
	userID string // register only
	name   string // register only
	data   webauthn.SessionData
}

type passkeyService struct {
	rp       domain.RelyingParty
	err      error // invalid configuration → routes answer 503
	sessions *domain.ChallengeStore[passkeySession]
	login    *domain.RateLimiter
	now      func() time.Time
}

func newPasskeyService(app core.App, cfg Config) *passkeyService {
	rp, err := domain.ResolveRelyingParty(cfg.PublicURL, cfg.WebAuthn.RPID, cfg.WebAuthn.Origins)
	if err != nil {
		app.Logger().Warn("passkeys disabled: invalid WebAuthn configuration", "error", err)
	}
	return &passkeyService{
		rp:       rp,
		err:      err,
		sessions: domain.NewChallengeStore[passkeySession](passkeySessionTTL, passkeySessionsMax),
		login:    domain.NewRateLimiter(passkeyLoginPerMin, time.Minute),
		now:      time.Now,
	}
}

func (s *passkeyService) enabled() bool { return s != nil && s.err == nil }

// webauthnFor builds the verifier for a request (the accepted origins depend
// on its Origin header in dev, see RelyingParty.OriginsFor).
func (s *passkeyService) webauthnFor(e *core.RequestEvent) (*webauthn.WebAuthn, error) {
	if !s.enabled() {
		return nil, apis.NewApiError(http.StatusServiceUnavailable, msgPasskeysOff, nil)
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                  s.rp.ID,
		RPDisplayName:         passkeyRPName,
		RPOrigins:             s.rp.OriginsFor(e.Request.Header.Get("Origin")),
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Timeout: passkeyTimeout, TimeoutUVD: passkeyTimeout},
			Registration: webauthn.TimeoutConfig{Timeout: passkeyTimeout, TimeoutUVD: passkeyTimeout},
		},
	})
	if err != nil {
		e.App.Logger().Error("webauthn config", "error", err)
		return nil, apis.NewApiError(http.StatusServiceUnavailable, msgPasskeysOff, nil)
	}
	return wa, nil
}

func (h *handlers) passkeyRoutes(g *router.RouterGroup[*core.RequestEvent], user *hookHandler) {
	g.POST("/passkeys/login/begin", h.passkeyLoginBegin)
	g.POST("/passkeys/login/finish", h.passkeyLoginFinish)
	g.POST("/passkeys/register/begin", h.passkeyRegisterBegin).Bind(user)
	g.POST("/passkeys/register/finish", h.passkeyRegisterFinish).Bind(user)
	g.GET("/passkeys", h.passkeyList).Bind(user)
	g.PATCH("/passkeys/{id}", h.passkeyRename).Bind(user)
	g.DELETE("/passkeys/{id}", h.passkeyDelete).Bind(user)
}

// bindPasskeyHooks: an anonymised (deleted) account loses its passkeys.
func bindPasskeyHooks(app core.App) {
	app.OnRecordUpdate(colUsers).BindFunc(func(e *core.RecordEvent) error {
		deleting := !e.Record.GetDateTime(fieldDeletedAt).IsZero() && e.Record.Original().GetDateTime(fieldDeletedAt).IsZero()
		if err := e.Next(); err != nil {
			return err
		}
		if deleting {
			return deletePasskeysOf(e.App, e.Record.Id)
		}
		return nil
	})
}

func deletePasskeysOf(app core.App, userID string) error {
	recs, err := app.FindAllRecords(colPasskeys, dbx.HashExp{"user": userID})
	if err != nil {
		return err
	}
	for _, r := range recs {
		if err := app.Delete(r); err != nil {
			return err
		}
	}
	return nil
}

// ------------------------------------------------------------ user adapter

type passkeyUser struct {
	rec   *core.Record
	creds []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte { return []byte(u.rec.Id) }

func (u passkeyUser) WebAuthnName() string { return u.rec.Email() }

func (u passkeyUser) WebAuthnDisplayName() string {
	if n := strings.TrimSpace(u.rec.GetString("name")); n != "" {
		return n
	}
	return u.rec.Email()
}

func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func loadPasskeyUser(app core.App, u *core.Record) (passkeyUser, []*core.Record, error) {
	recs, err := app.FindAllRecords(colPasskeys, dbx.HashExp{"user": u.Id})
	if err != nil {
		return passkeyUser{}, nil, err
	}
	pu := passkeyUser{rec: u}
	for _, r := range recs {
		c, err := credentialFromRecord(r)
		if err != nil {
			app.Logger().Warn("passkey record unreadable", "passkey", r.Id, "error", err)
			continue
		}
		pu.creds = append(pu.creds, c)
	}
	return pu, recs, nil
}

var b64 = base64.RawURLEncoding

func credentialFromRecord(r *core.Record) (webauthn.Credential, error) {
	id, err := b64.DecodeString(r.GetString("credential_id"))
	if err != nil {
		return webauthn.Credential{}, err
	}
	pk, err := b64.DecodeString(r.GetString("public_key"))
	if err != nil {
		return webauthn.Credential{}, err
	}
	var aaguid []byte
	if s := r.GetString("aaguid"); s != "" {
		if u, err := uuid.Parse(s); err == nil {
			aaguid = u[:]
		}
	}
	flags := protocol.FlagUserPresent
	if r.GetBool("user_verified") {
		flags |= protocol.FlagUserVerified
	}
	if r.GetBool("backup_eligible") {
		flags |= protocol.FlagBackupEligible
	}
	if r.GetBool("backup_state") {
		flags |= protocol.FlagBackupState
	}
	var transports []protocol.AuthenticatorTransport
	for _, t := range r.GetStringSlice("transports") {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return webauthn.Credential{
		ID:                id,
		PublicKey:         pk,
		AttestationType:   r.GetString("attestation_type"),
		AttestationFormat: r.GetString("attestation_format"),
		Transport:         transports,
		Flags:             webauthn.NewCredentialFlags(flags),
		Authenticator: webauthn.Authenticator{
			AAGUID:     aaguid,
			SignCount:  uint32(r.GetInt("sign_count")),
			Attachment: protocol.AuthenticatorAttachment(r.GetString("attachment")),
		},
	}, nil
}

func aaguidString(b []byte) string {
	u, err := uuid.FromBytes(b)
	if err != nil {
		return ""
	}
	return u.String()
}

// ---------------------------------------------------------------- views

// PasskeyView is what the owner sees (docs/ARCHITECTURE.md §5).
type passkeyView struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Created    string   `json:"created"`
	LastUsedAt string   `json:"lastUsedAt"`
	Transports []string `json:"transports"`
	Synced     bool     `json:"synced"`
}

func viewPasskey(r *core.Record) passkeyView {
	v := passkeyView{
		ID:         r.Id,
		Name:       r.GetString("name"),
		Created:    r.GetDateTime("created").String(),
		Transports: r.GetStringSlice("transports"),
		Synced:     r.GetBool("backup_state"),
	}
	if t := r.GetDateTime("last_used_at"); !t.IsZero() {
		v.LastUsedAt = t.String()
	}
	if v.Transports == nil {
		v.Transports = []string{}
	}
	return v
}

// writeJSON encodes with encoding/json (v1): the go-webauthn option types are
// written for it (omitempty, custom base64url marshalers).
func writeJSON(e *core.RequestEvent, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return e.Blob(http.StatusOK, "application/json", b)
}

// readCredential reads {credential: <PublicKeyCredential JSON>, name?}.
func readCredential(e *core.RequestEvent) (json.RawMessage, string, error) {
	// BindBody (not a raw decoder): the body must stay re-readable for
	// RecordAuthResponse, which parses it again through RequestInfo().
	if e.Request.ContentLength > passkeyBodyMaxBytes {
		return nil, "", badRequest("Corps de requête invalide.")
	}
	var body struct {
		Credential json.RawMessage `json:"credential"`
		Name       string          `json:"name"`
	}
	if err := e.BindBody(&body); err != nil || len(body.Credential) == 0 {
		return nil, "", badRequest("Corps de requête invalide.")
	}
	return body.Credential, body.Name, nil
}

func logPasskeyError(e *core.RequestEvent, step string, err error) {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		e.App.Logger().Info("passkey "+step+" refused", "type", pe.Type, "details", pe.Details, "info", pe.DevInfo)
		return
	}
	e.App.Logger().Info("passkey "+step+" refused", "error", err)
}

// ------------------------------------------------------------ registration

// passkeyRegisterBegin: POST /api/occ/passkeys/register/begin {name?} →
// CredentialCreation ({publicKey: …}).
func (h *handlers) passkeyRegisterBegin(e *core.RequestEvent) error {
	wa, err := h.passkeys.webauthnFor(e)
	if err != nil {
		return err
	}
	if isGuestRecord(e.Auth) {
		return forbidden(msgPasskeyGuest)
	}
	var body struct {
		Name string `json:"name"`
	}
	if e.Request.ContentLength != 0 {
		if err := bindJSON(e, &body); err != nil {
			return err
		}
	}
	u, err := e.App.FindRecordById(colUsers, e.Auth.Id)
	if err != nil {
		return notFound("Compte introuvable.")
	}
	pu, _, err := loadPasskeyUser(e.App, u)
	if err != nil {
		return err
	}
	if len(pu.creds) >= passkeysPerUserMax {
		return badRequest("Tu as déjà 20 passkeys : supprimes-en une avant d'en ajouter une autre.")
	}
	creation, session, err := wa.BeginRegistration(pu,
		webauthn.WithExclusions(webauthn.Credentials(pu.creds).CredentialDescriptors()),
		webauthn.WithExtensions(webauthn.WithExtensionCredProps()),
	)
	if err != nil {
		e.App.Logger().Error("passkey register begin", "error", err)
		return err
	}
	h.passkeys.sessions.Put(session.Challenge, passkeySession{
		kind: sessionRegister, userID: u.Id, name: body.Name, data: *session,
	}, h.passkeys.now())
	return writeJSON(e, creation)
}

// passkeyRegisterFinish: POST /api/occ/passkeys/register/finish
// {credential, name?} → PasskeyView.
func (h *handlers) passkeyRegisterFinish(e *core.RequestEvent) error {
	wa, err := h.passkeys.webauthnFor(e)
	if err != nil {
		return err
	}
	if isGuestRecord(e.Auth) {
		return forbidden(msgPasskeyGuest)
	}
	raw, name, err := readCredential(e)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(raw)
	if err != nil {
		logPasskeyError(e, "register", err)
		return badRequest(msgPasskeyInvalid)
	}
	sess, found := h.passkeys.sessions.Take(parsed.Response.CollectedClientData.Challenge, h.passkeys.now())
	if !found || sess.kind != sessionRegister || sess.userID != e.Auth.Id {
		return badRequest(msgPasskeyExpired)
	}
	u, err := e.App.FindRecordById(colUsers, e.Auth.Id)
	if err != nil {
		return notFound("Compte introuvable.")
	}
	pu, _, err := loadPasskeyUser(e.App, u)
	if err != nil {
		return err
	}
	cred, err := wa.CreateCredential(pu, sess.data, parsed)
	if err != nil {
		logPasskeyError(e, "register", err)
		return badRequest(msgPasskeyInvalid)
	}
	credID := b64.EncodeToString(cred.ID)
	if n, _ := e.App.CountRecords(colPasskeys, dbx.HashExp{"credential_id": credID}); n > 0 {
		return badRequest(msgPasskeyExists)
	}
	if strings.TrimSpace(name) == "" {
		name = sess.name
	}
	aaguid := aaguidString(cred.Authenticator.AAGUID)
	col, err := e.App.FindCachedCollectionByNameOrId(colPasskeys)
	if err != nil {
		return err
	}
	transports := make([]string, 0, len(cred.Transport))
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	r := core.NewRecord(col)
	r.Set("user", u.Id)
	r.Set("name", domain.PasskeyName(name, aaguid))
	r.Set("credential_id", credID)
	r.Set("public_key", b64.EncodeToString(cred.PublicKey))
	r.Set("sign_count", cred.Authenticator.SignCount)
	r.Set("transports", transports)
	r.Set("aaguid", aaguid)
	r.Set("attestation_type", cred.AttestationType)
	r.Set("attestation_format", cred.AttestationFormat)
	r.Set("user_verified", cred.Flags.UserVerified)
	r.Set("backup_eligible", cred.Flags.BackupEligible)
	r.Set("backup_state", cred.Flags.BackupState)
	r.Set("attachment", string(cred.Authenticator.Attachment))
	if err := e.App.Save(r); err != nil {
		return err
	}
	e.App.Logger().Info("passkey registered", "user", u.Id, "passkey", r.Id, "aaguid", aaguid)
	return ok(e, viewPasskey(r))
}

// ------------------------------------------------------------------ login

// passkeyLoginBegin: POST /api/occ/passkeys/login/begin {conditional?} →
// CredentialAssertion ({publicKey: …, mediation?}), discoverable credentials
// (no allowCredentials: the browser shows the passkeys it has for the site).
func (h *handlers) passkeyLoginBegin(e *core.RequestEvent) error {
	wa, err := h.passkeys.webauthnFor(e)
	if err != nil {
		return err
	}
	if !h.passkeys.login.Allow(e.RealIP(), h.passkeys.now()) {
		return errTooManyRequests()
	}
	var body struct {
		Conditional bool `json:"conditional"`
	}
	if e.Request.ContentLength != 0 {
		if err := bindJSON(e, &body); err != nil {
			return err
		}
	}
	mediation := protocol.MediationDefault
	if body.Conditional {
		mediation = protocol.MediationConditional
	}
	assertion, session, err := wa.BeginDiscoverableMediatedLogin(mediation)
	if err != nil {
		e.App.Logger().Error("passkey login begin", "error", err)
		return err
	}
	h.passkeys.sessions.Put(session.Challenge, passkeySession{kind: sessionLogin, data: *session}, h.passkeys.now())
	return writeJSON(e, assertion)
}

// passkeyLoginFinish: POST /api/occ/passkeys/login/finish {credential} →
// {token, record} (same as auth-with-password).
func (h *handlers) passkeyLoginFinish(e *core.RequestEvent) error {
	wa, err := h.passkeys.webauthnFor(e)
	if err != nil {
		return err
	}
	raw, _, err := readCredential(e)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(raw)
	if err != nil {
		logPasskeyError(e, "login", err)
		return badRequest(msgPasskeyInvalid)
	}
	sess, found := h.passkeys.sessions.Take(parsed.Response.CollectedClientData.Challenge, h.passkeys.now())
	if !found || sess.kind != sessionLogin {
		return badRequest(msgPasskeyExpired)
	}
	pkRec, err := e.App.FindFirstRecordByData(colPasskeys, "credential_id", b64.EncodeToString(parsed.RawID))
	if err != nil || pkRec.GetString("user") != string(parsed.Response.UserHandle) {
		return badRequest(msgPasskeyUnknown)
	}
	var owner *core.Record
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		u, err := e.App.FindRecordById(colUsers, string(userHandle))
		if err != nil {
			return nil, err
		}
		pu, _, err := loadPasskeyUser(e.App, u)
		if err != nil {
			return nil, err
		}
		owner = u
		return pu, nil
	}
	_, cred, err := wa.ValidatePasskeyLogin(handler, sess.data, parsed)
	if err != nil || owner == nil {
		logPasskeyError(e, "login", err)
		return badRequest(msgPasskeyInvalid)
	}
	if cred.Authenticator.CloneWarning {
		e.App.Logger().Warn("passkey sign counter went backwards: refused", "user", owner.Id, "passkey", pkRec.Id,
			"stored", pkRec.GetInt("sign_count"), "received", parsed.Response.AuthenticatorData.Counter)
		return badRequest(msgPasskeyCloned)
	}
	if isBanned(owner) {
		return errSuspended()
	}
	pkRec.Set("sign_count", cred.Authenticator.SignCount)
	pkRec.Set("backup_state", cred.Flags.BackupState)
	pkRec.Set("last_used_at", types.NowDateTime())
	if err := e.App.Save(pkRec); err != nil {
		return err
	}
	return apis.RecordAuthResponse(e, owner, passkeyAuthMethod, nil)
}

// ----------------------------------------------------------- management

// passkeyList: GET /api/occ/passkeys → PasskeyView[] (newest first).
func (h *handlers) passkeyList(e *core.RequestEvent) error {
	recs, err := e.App.FindRecordsByFilter(colPasskeys, "user = {:u}", "-created", 0, 0, dbx.Params{"u": e.Auth.Id})
	if err != nil {
		return err
	}
	out := make([]passkeyView, 0, len(recs))
	for _, r := range recs {
		out = append(out, viewPasskey(r))
	}
	return ok(e, out)
}

func ownPasskey(e *core.RequestEvent) (*core.Record, error) {
	r, err := e.App.FindRecordById(colPasskeys, e.Request.PathValue("id"))
	if err != nil || r.GetString("user") != e.Auth.Id {
		return nil, notFound(msgPasskeyNotFound)
	}
	return r, nil
}

// passkeyRename: PATCH /api/occ/passkeys/{id} {name} → PasskeyView.
func (h *handlers) passkeyRename(e *core.RequestEvent) error {
	r, err := ownPasskey(e)
	if err != nil {
		return err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	name, err := domain.ValidatePasskeyRename(body.Name)
	if err != nil {
		return toAPIError(err)
	}
	r.Set("name", name)
	if err := e.App.Save(r); err != nil {
		return err
	}
	return ok(e, viewPasskey(r))
}

// passkeyDelete: DELETE /api/occ/passkeys/{id} → 204.
func (h *handlers) passkeyDelete(e *core.RequestEvent) error {
	r, err := ownPasskey(e)
	if err != nil {
		return err
	}
	if err := e.App.Delete(r); err != nil {
		return err
	}
	e.App.Logger().Info("passkey deleted", "user", e.Auth.Id, "passkey", r.Id)
	return e.NoContent(http.StatusNoContent)
}
