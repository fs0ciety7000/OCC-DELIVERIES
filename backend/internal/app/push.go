package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/notify"
)

const (
	colPushSubscriptions = "push_subscriptions"
	colServerSecrets     = "server_secrets"

	// vapidSecretName is the server_secrets row holding the generated keys.
	vapidSecretName = "vapid"
	// inAppTopic is the custom realtime topic of the in-app toasts.
	inAppTopic = "occ/notifications"
	// maxSubscriptionsPerUser bounds the devices of one account (oldest dropped).
	maxSubscriptionsPerUser = 10
	// maxPushFailures: a subscription failing that many times in a row is dropped.
	maxPushFailures = 5
	// joinThrottle: at most one « X a rejoint » notification per party and period.
	joinThrottle = 2 * time.Minute
)

// PushConfig is the Web Push configuration (OCC_VAPID_*, OCC_PUSH_ENABLED).
type PushConfig struct {
	Enabled    bool
	PublicKey  string
	PrivateKey string
	Subject    string
	// Pusher replaces the real Web Push transport (tests only).
	Pusher notify.Pusher
	// Now replaces the clock (tests only).
	Now func() time.Time
}

func pushConfigFromEnv(mailFrom, publicURL string) PushConfig {
	c := PushConfig{
		Enabled:    !strings.EqualFold(strings.TrimSpace(os.Getenv("OCC_PUSH_ENABLED")), "false"),
		PublicKey:  strings.TrimSpace(os.Getenv("OCC_VAPID_PUBLIC_KEY")),
		PrivateKey: strings.TrimSpace(os.Getenv("OCC_VAPID_PRIVATE_KEY")),
		Subject:    strings.TrimSpace(os.Getenv("OCC_VAPID_SUBJECT")),
	}
	if c.Subject == "" {
		c.Subject = defaultVAPIDSubject(mailFrom, publicURL)
	}
	return c
}

// defaultVAPIDSubject: mailto:OCC_MAIL_FROM, else mailto:noreply@<host of OCC_PUBLIC_URL>.
func defaultVAPIDSubject(mailFrom, publicURL string) string {
	if strings.Contains(mailFrom, "@") {
		return "mailto:" + strings.TrimSpace(mailFrom)
	}
	host := "localhost"
	if u, err := url.Parse(publicURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	if !strings.Contains(host, ".") {
		host += ".invalid"
	}
	return "mailto:noreply@" + host
}

// ------------------------------------------------------------ actor context

type ctxKey int

const (
	ctxActor ctxKey = iota
	ctxAuto
)

// actorContext carries the authenticated user into Save, so that the
// notification hooks never notify the person who acted.
func actorContext(e *core.RequestEvent) context.Context {
	ctx := context.WithoutCancel(e.Request.Context())
	if e.Auth != nil {
		ctx = context.WithValue(ctx, ctxActor, e.Auth.Id)
	}
	return ctx
}

// autoContext marks saves made by the deadline scheduler.
func autoContext() context.Context {
	return context.WithValue(context.Background(), ctxAuto, true)
}

func actorFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	s, _ := ctx.Value(ctxActor).(string)
	return s
}

func isAuto(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	b, _ := ctx.Value(ctxAuto).(bool)
	return b
}

// ----------------------------------------------------------------- service

type pushService struct {
	app    core.App
	cfg    PushConfig
	keys   notify.Keys
	sender *notify.Sender
	now    func() time.Time

	mu          sync.Mutex
	lastStatus  map[string]string    // party → last status notified (double saves)
	lastJoin    map[string]time.Time // party → last « a rejoint » sent
	joinedSince map[string]int       // party → joins not notified (throttled)
	allReady    map[string]bool      // party → « tout le monde est prêt » sent
}

func newPushService(app core.App, cfg PushConfig) *pushService {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &pushService{
		app: app, cfg: cfg, now: now,
		lastStatus: map[string]string{}, lastJoin: map[string]time.Time{},
		joinedSince: map[string]int{}, allReady: map[string]bool{},
	}
}

func (s *pushService) bind() {
	s.app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		s.app = se.App
		if s.cfg.Enabled {
			keys, err := resolveVAPID(se.App, s.cfg)
			if err != nil {
				se.App.Logger().Error("push: clés VAPID indisponibles, notifications désactivées", "error", err)
			} else {
				s.keys = keys
				pusher := s.cfg.Pusher
				if pusher == nil {
					pusher = notify.WebPusher{Keys: keys}
				}
				s.sender = notify.NewSender(pushStore{app: se.App}, pusher, notify.Options{Logger: se.App.Logger(), Now: s.now})
			}
		}
		return se.Next()
	})
	s.app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		s.sender.Close()
		return e.Next()
	})

	s.app.OnRecordAfterUpdateSuccess(colParties).BindFunc(func(e *core.RecordEvent) error {
		s.onPartyUpdated(e)
		return e.Next()
	})
	s.app.OnRecordAfterCreateSuccess(colPartyMembers).BindFunc(func(e *core.RecordEvent) error {
		s.onMemberJoined(e)
		return e.Next()
	})
	s.app.OnRecordAfterUpdateSuccess(colPartyMembers).BindFunc(func(e *core.RecordEvent) error {
		s.onMemberUpdated(e)
		return e.Next()
	})
	s.app.OnRecordAfterUpdateSuccess(colPayments).BindFunc(func(e *core.RecordEvent) error {
		s.onPaymentUpdated(e)
		return e.Next()
	})
}

func (s *pushService) enabled() bool { return s.sender != nil }

// wait blocks until queued notifications are processed (tests).
func (s *pushService) wait() { s.sender.Wait() }

// send queues m for users (push, filtered by preferences) and, for the
// in-app kinds, sends a realtime toast to their open tabs.
func (s *pushService) send(users []string, m notify.Message) {
	if len(users) == 0 {
		return
	}
	if inAppKinds[m.Kind] {
		s.inApp(users, m)
	}
	if s.sender != nil {
		_ = s.sender.Send(users, m)
	}
}

// inAppKinds are also delivered as realtime toasts (the status changes
// already have their own toasts in the SPA).
var inAppKinds = map[string]bool{
	notify.KindReminderVote: true, notify.KindReminderOrder: true, notify.KindVoteExtended: true,
	notify.KindNeedsHost: true, notify.KindAutoClosed: true, notify.KindAllReady: true,
	notify.KindPaymentDeclared: true, notify.KindPaymentConfirm: true,
}

func (s *pushService) inApp(users []string, m notify.Message) {
	set := map[string]bool{}
	for _, u := range users {
		set[u] = true
	}
	msg := subscriptions.Message{Name: inAppTopic, Data: m.JSON(s.now())}
	for _, chunk := range s.app.SubscriptionsBroker().ChunkedClients(300) {
		for _, c := range chunk {
			if !c.HasSubscription(inAppTopic) {
				continue
			}
			auth, _ := c.Get(apis.RealtimeClientAuthKey).(*core.Record)
			if auth == nil || !set[auth.Id] {
				continue
			}
			go c.Send(msg)
		}
	}
}

// ------------------------------------------------------------------ events

// TeamPartyLaunched implements TeamNotifier (teams.go): « <Équipe> : la
// commande du jour est lancée — rejoins « Midi du lundi » ».
func (s *pushService) TeamPartyLaunched(_ core.App, team, party *core.Record, recipients []string) error {
	s.send(recipients, notify.TeamLaunched(partyInfo(party), team.GetString("name")))
	return nil
}

func partyInfo(p *core.Record) notify.Party {
	return notify.Party{ID: p.Id, Title: p.GetString("title"), Code: p.GetString("code")}
}

func userName(app core.App, id string) string {
	if id == "" {
		return ""
	}
	u, err := app.FindRecordById(colUsers, id)
	if err != nil {
		return ""
	}
	return userInfo(u).Name
}

func (s *pushService) onPartyUpdated(e *core.RecordEvent) {
	p := e.Record
	status := p.GetString("status")
	if status == p.Original().GetString("status") {
		return
	}
	s.mu.Lock()
	if s.lastStatus[p.Id] == status {
		s.mu.Unlock()
		return
	}
	s.lastStatus[p.Id] = status
	delete(s.allReady, p.Id)
	if status == domain.StatusClosed || status == domain.StatusCancelled {
		delete(s.lastJoin, p.Id)
		delete(s.joinedSince, p.Id)
	}
	s.mu.Unlock()

	app := e.App
	actor := actorFrom(e.Context)
	info := partyInfo(p)
	members := notify.Recipients(p.GetStringSlice("members"), actor)

	if status == domain.StatusPaying {
		s.notifyPaying(app, p, members)
		return
	}
	in := notify.StatusInput{Party: info, Status: status, Auto: isAuto(e.Context)}
	switch status {
	case domain.StatusVoting:
		in.Deadline = p.GetDateTime("voting_ends_at").Time()
	case domain.StatusOrdering:
		in.Deadline = p.GetDateTime("ordering_ends_at").Time()
		if r, err := app.FindRecordById(colRestaurants, p.GetString("restaurant")); err == nil {
			in.Restaurant = r.GetString("name")
		}
	}
	if m, ok := notify.StatusChanged(in); ok {
		s.send(members, m)
	}
}

func (s *pushService) notifyPaying(app core.App, p *core.Record, recipients []string) {
	pays, err := app.FindAllRecords(colPayments, dbx.HashExp{"party": p.Id})
	if err != nil {
		return
	}
	payer := p.GetString("payer")
	payerName := userName(app, payer)
	info := partyInfo(p)
	owed := 0
	for _, pay := range pays {
		if pay.GetString("debtor") == payer || pay.GetString("status") == domain.PaymentConfirmed {
			continue
		}
		owed += pay.GetInt("amount")
		if slices.Contains(recipients, pay.GetString("debtor")) {
			s.send([]string{pay.GetString("debtor")}, notify.Paying(info, payerName, pay.GetInt("amount"), false, 0))
		}
	}
	if slices.Contains(recipients, payer) {
		s.send([]string{payer}, notify.Paying(info, payerName, 0, true, owed))
	}
}

func (s *pushService) onMemberJoined(e *core.RecordEvent) {
	pm := e.Record
	if pm.GetString("role") != "member" {
		return
	}
	p, err := e.App.FindRecordById(colParties, pm.GetString("party"))
	if err != nil {
		return
	}
	host := p.GetString("host")
	if host == "" || host == pm.GetString("user") {
		return
	}
	now := s.now()
	s.mu.Lock()
	if last, ok := s.lastJoin[p.Id]; ok && now.Sub(last) < joinThrottle {
		s.joinedSince[p.Id]++
		s.mu.Unlock()
		return
	}
	others := s.joinedSince[p.Id]
	s.lastJoin[p.Id] = now
	s.joinedSince[p.Id] = 0
	s.mu.Unlock()
	s.send([]string{host}, notify.MemberJoined(partyInfo(p), userName(e.App, pm.GetString("user")), others))
}

func (s *pushService) onMemberUpdated(e *core.RecordEvent) {
	pm := e.Record
	ready, was := pm.GetBool("ready"), pm.Original().GetBool("ready")
	if ready == was {
		return
	}
	partyID := pm.GetString("party")
	if !ready {
		s.mu.Lock()
		delete(s.allReady, partyID)
		s.mu.Unlock()
		return
	}
	p, err := e.App.FindRecordById(colParties, partyID)
	if err != nil || p.GetString("status") != domain.StatusOrdering {
		return
	}
	pms, err := partyMembers(e.App, partyID)
	if err != nil || len(pms) < 2 {
		return
	}
	for _, m := range pms {
		if !m.GetBool("ready") {
			return
		}
	}
	host := p.GetString("host")
	if host == pm.GetString("user") { // the host was the last one: they know
		return
	}
	s.mu.Lock()
	sent := s.allReady[partyID]
	s.allReady[partyID] = true
	s.mu.Unlock()
	if !sent {
		s.send([]string{host}, notify.AllReady(partyInfo(p)))
	}
}

func (s *pushService) onPaymentUpdated(e *core.RecordEvent) {
	pay := e.Record
	status := pay.GetString("status")
	if status == pay.Original().GetString("status") || pay.GetString("method") == domain.MethodSelf {
		return
	}
	p, err := e.App.FindRecordById(colParties, pay.GetString("party"))
	if err != nil {
		return
	}
	actor := actorFrom(e.Context)
	debtor, creditor := pay.GetString("debtor"), pay.GetString("creditor")
	switch status {
	case domain.PaymentDeclared:
		if creditor != actor {
			s.send([]string{creditor}, notify.PaymentDeclared(partyInfo(p), userName(e.App, debtor), pay.GetInt("amount"), pay.GetString("method")))
		}
	case domain.PaymentConfirmed:
		if debtor == actor {
			return
		}
		by := actor
		if by == "" {
			by = creditor
		}
		s.send([]string{debtor}, notify.PaymentConfirmed(partyInfo(p), userName(e.App, by), pay.GetInt("amount")))
	}
}

// ------------------------------------------------------------------- store

type pushStore struct{ app core.App }

func (st pushStore) Targets(userIDs []string, c notify.Category) ([]notify.Subscription, error) {
	users, err := st.app.FindRecordsByIds(colUsers, userIDs)
	if err != nil {
		return nil, err
	}
	allowed := make([]any, 0, len(users))
	for _, u := range users {
		if u.GetBool("banned") {
			continue
		}
		if notify.ParsePrefs([]byte(u.GetString("notify_prefs"))).Allows(c) {
			allowed = append(allowed, u.Id)
		}
	}
	if len(allowed) == 0 {
		return nil, nil
	}
	recs, err := st.app.FindAllRecords(colPushSubscriptions, dbx.In("user", allowed...))
	if err != nil {
		return nil, err
	}
	out := make([]notify.Subscription, 0, len(recs))
	for _, r := range recs {
		out = append(out, notify.Subscription{
			ID: r.Id, UserID: r.GetString("user"), Endpoint: r.GetString("endpoint"),
			P256dh: r.GetString("p256dh"), Auth: r.GetString("auth"),
		})
	}
	return out, nil
}

func (st pushStore) Delivered(id string) {
	_, _ = st.app.DB().Update(colPushSubscriptions, dbx.Params{
		"last_ok": types.NowDateTime().String(), "failures": 0,
	}, dbx.HashExp{"id": id}).Execute()
}

func (st pushStore) Failed(id string, gone bool) {
	if !gone {
		_, _ = st.app.DB().NewQuery("UPDATE " + colPushSubscriptions + " SET failures = failures + 1 WHERE id = {:id}").
			Bind(dbx.Params{"id": id}).Execute()
	}
	_, _ = st.app.DB().NewQuery("DELETE FROM " + colPushSubscriptions + " WHERE id = {:id} AND ({:gone} OR failures >= {:max})").
		Bind(dbx.Params{"id": id, "gone": gone, "max": maxPushFailures}).Execute()
}

// ------------------------------------------------------------------- VAPID

// resolveVAPID returns the configured keys, or the generated ones stored in
// server_secrets (created on first start, superusers only).
func resolveVAPID(app core.App, cfg PushConfig) (notify.Keys, error) {
	if cfg.PublicKey != "" && cfg.PrivateKey != "" {
		return notify.Keys{Public: cfg.PublicKey, Private: cfg.PrivateKey, Subject: cfg.Subject}, nil
	}
	if cfg.PublicKey != "" || cfg.PrivateKey != "" {
		app.Logger().Warn("push: OCC_VAPID_PUBLIC_KEY et OCC_VAPID_PRIVATE_KEY vont ensemble ; clés générées utilisées")
	}
	var stored struct {
		Public  string `json:"public"`
		Private string `json:"private"`
	}
	if rec, err := app.FindFirstRecordByData(colServerSecrets, "name", vapidSecretName); err == nil {
		if json.Unmarshal([]byte(rec.GetString("value")), &stored) == nil && stored.Public != "" && stored.Private != "" {
			return notify.Keys{Public: stored.Public, Private: stored.Private, Subject: cfg.Subject}, nil
		}
	}
	keys, err := notify.GenerateKeys()
	if err != nil {
		return notify.Keys{}, err
	}
	col, err := app.FindCollectionByNameOrId(colServerSecrets)
	if err != nil {
		return notify.Keys{}, err
	}
	rec, err := app.FindFirstRecordByData(colServerSecrets, "name", vapidSecretName)
	if err != nil {
		rec = core.NewRecord(col)
		rec.Set("name", vapidSecretName)
	}
	value, _ := json.Marshal(map[string]string{"public": keys.Public, "private": keys.Private})
	rec.Set("value", string(value))
	if err := app.Save(rec); err != nil {
		return notify.Keys{}, err
	}
	app.Logger().Info("push: clés VAPID générées et enregistrées (server_secrets)")
	keys.Subject = cfg.Subject
	return keys, nil
}

// ------------------------------------------------------------------ routes

func (h *handlers) pushRoutes(g *router.RouterGroup[*core.RequestEvent]) {
	user := apis.RequireAuth(colUsers)
	g.GET("/push/public-key", h.pushPublicKey)
	g.POST("/push/subscribe", h.pushSubscribe).Bind(user)
	g.DELETE("/push/subscribe", h.pushUnsubscribe).Bind(user)
	g.POST("/push/test", h.pushTest).Bind(user)
	g.GET("/push/prefs", h.pushPrefs).Bind(user)
	g.PATCH("/push/prefs", h.pushSetPrefs).Bind(user)
}

func (h *handlers) pushPublicKey(e *core.RequestEvent) error {
	return ok(e, map[string]any{"enabled": h.push.enabled(), "publicKey": h.push.keys.Public})
}

type subscriptionBody struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func decodeB64URL(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	return base64.RawURLEncoding.DecodeString(s)
}

// validateSubscription checks a PushSubscription sent by the browser.
func validateSubscription(b subscriptionBody) error {
	u, err := url.Parse(strings.TrimSpace(b.Endpoint))
	if err != nil || u.Scheme != "https" || u.Host == "" || len(b.Endpoint) > 1000 {
		return badRequest("Abonnement invalide : adresse du service de notification incorrecte.")
	}
	if k, err := decodeB64URL(b.Keys.P256dh); err != nil || len(k) != 65 {
		return badRequest("Abonnement invalide : clé de chiffrement incorrecte.")
	}
	if k, err := decodeB64URL(b.Keys.Auth); err != nil || len(k) < 16 || len(k) > 32 {
		return badRequest("Abonnement invalide : secret d'authentification incorrect.")
	}
	return nil
}

func (h *handlers) pushSubscribe(e *core.RequestEvent) error {
	if !h.push.enabled() {
		return badRequest("Les notifications ne sont pas disponibles sur ce serveur.")
	}
	var body subscriptionBody
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	if err := validateSubscription(body); err != nil {
		return err
	}
	endpoint := strings.TrimSpace(body.Endpoint)
	ua := strings.TrimSpace(e.Request.UserAgent())
	if r := []rune(ua); len(r) > 300 {
		ua = string(r[:300])
	}
	var rec *core.Record
	err := e.App.RunInTransaction(func(tx core.App) error {
		col, err := tx.FindCollectionByNameOrId(colPushSubscriptions)
		if err != nil {
			return err
		}
		// same browser, maybe another account: the endpoint follows the last login
		r, err := tx.FindFirstRecordByData(colPushSubscriptions, "endpoint", endpoint)
		if err != nil {
			r = core.NewRecord(col)
			r.Set("endpoint", endpoint)
		}
		r.Set("user", e.Auth.Id)
		r.Set("p256dh", strings.TrimRight(strings.TrimSpace(body.Keys.P256dh), "="))
		r.Set("auth", strings.TrimRight(strings.TrimSpace(body.Keys.Auth), "="))
		r.Set("user_agent", ua)
		r.Set("failures", 0)
		if err := tx.Save(r); err != nil {
			return err
		}
		rec = r
		// keep the most recent devices only
		all, err := tx.FindRecordsByFilter(colPushSubscriptions, "user = {:u}", "-updated", 0, 0, dbx.Params{"u": e.Auth.Id})
		if err != nil {
			return err
		}
		for i, old := range all {
			if i >= maxSubscriptionsPerUser && old.Id != r.Id {
				if err := tx.Delete(old); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"ok": true, "id": rec.Id})
}

func (h *handlers) pushUnsubscribe(e *core.RequestEvent) error {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	res, err := e.App.DB().Delete(colPushSubscriptions, dbx.HashExp{"endpoint": strings.TrimSpace(body.Endpoint), "user": e.Auth.Id}).Execute()
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	return ok(e, map[string]any{"ok": true, "deleted": n})
}

func (h *handlers) pushTest(e *core.RequestEvent) error {
	if !h.push.enabled() {
		return badRequest("Les notifications ne sont pas disponibles sur ce serveur.")
	}
	n, err := e.App.CountRecords(colPushSubscriptions, dbx.HashExp{"user": e.Auth.Id})
	if err != nil {
		return err
	}
	if n == 0 {
		return badRequest("Aucun appareil abonné : active d'abord les notifications sur cet appareil.")
	}
	h.push.send([]string{e.Auth.Id}, notify.Test())
	return ok(e, map[string]any{"queued": true, "devices": n})
}

func (h *handlers) pushPrefs(e *core.RequestEvent) error {
	u, err := e.App.FindRecordById(colUsers, e.Auth.Id)
	if err != nil {
		return notFound("Compte introuvable.")
	}
	n, err := e.App.CountRecords(colPushSubscriptions, dbx.HashExp{"user": e.Auth.Id})
	if err != nil {
		return err
	}
	return ok(e, map[string]any{
		"prefs":   notify.ParsePrefs([]byte(u.GetString("notify_prefs"))),
		"devices": n,
		"enabled": h.push.enabled(),
	})
}

func (h *handlers) pushSetPrefs(e *core.RequestEvent) error {
	var body struct {
		Party     *bool `json:"party"`
		Payments  *bool `json:"payments"`
		Reminders *bool `json:"reminders"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	if body.Party == nil && body.Payments == nil && body.Reminders == nil {
		return badRequest("Aucune préférence à enregistrer.")
	}
	u, err := e.App.FindRecordById(colUsers, e.Auth.Id)
	if err != nil {
		return notFound("Compte introuvable.")
	}
	p := notify.ParsePrefs([]byte(u.GetString("notify_prefs")))
	if body.Party != nil {
		p.Party = *body.Party
	}
	if body.Payments != nil {
		p.Payments = *body.Payments
	}
	if body.Reminders != nil {
		p.Reminders = *body.Reminders
	}
	u.Set("notify_prefs", p)
	if err := e.App.Save(u); err != nil {
		return err
	}
	return ok(e, map[string]any{"prefs": p})
}
