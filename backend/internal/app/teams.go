package app

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Permanent team rooms (« Salon d'équipe », migration 1760000016): a team
// has a fixed invite link /e/:code, members, defaults (office address,
// candidates, split) and launches the « commande du jour » for everyone.

const colTeams = "teams"

// teamTimezone: usual time and daily titles are read at Brussels time.
const teamTimezone = "Europe/Brussels"

// teamImmutableFields are managed by the server (endpoints / hooks).
var teamImmutableFields = []string{"code", "owner", "members", "last_party", "last_launch_at"}

// TeamNotifier is the extension point for push notifications when a team
// party is launched (implemented by the notification package, if any).
// recipients = team members except the host, active accounts only.
// Errors are logged, never returned to the client.
type TeamNotifier interface {
	TeamPartyLaunched(app core.App, team, party *core.Record, recipients []string) error
}

var teamNotifier TeamNotifier

// SetTeamNotifier registers the push notifier of the team launches (nil =
// realtime / in-app toasts only).
func SetTeamNotifier(n TeamNotifier) { teamNotifier = n }

func brussels() *time.Location {
	if loc, err := time.LoadLocation(teamTimezone); err == nil {
		return loc
	}
	return time.UTC
}

func isTeamMember(team *core.Record, userID string) bool {
	return userID != "" && slices.Contains(team.GetStringSlice("members"), userID)
}

func teamRole(team *core.Record, userID string) domain.TeamRole {
	return domain.RoleInTeam(team.GetString("owner"), team.GetStringSlice("admins"), team.GetStringSlice("members"), userID)
}

func errNotTeamMember() error { return forbidden("Tu ne fais pas partie de cette équipe.") }

// teamForMember loads the team of the path and checks the auth user is a member.
func teamForMember(e *core.RequestEvent) (*core.Record, error) {
	team, err := e.App.FindRecordById(colTeams, e.Request.PathValue("id"))
	if err != nil {
		return nil, notFound("Équipe introuvable.")
	}
	if !isTeamMember(team, e.Auth.Id) {
		return nil, errNotTeamMember()
	}
	return team, nil
}

func uniqueTeamCode(app core.App) (string, error) {
	for i := 0; i < 20; i++ {
		code, err := domain.GenerateTeamCode()
		if err != nil {
			return "", err
		}
		n, err := app.CountRecords(colTeams, dbx.HashExp{"code": code})
		if err != nil {
			return "", err
		}
		if n == 0 {
			return code, nil
		}
	}
	return "", badRequest("Impossible de générer un lien d'équipe, réessaie.")
}

// ------------------------------------------------------------------ hooks

func bindTeamHooks(app core.App) {
	app.OnRecordCreateRequest(colTeams).BindFunc(onTeamCreate)
	app.OnRecordUpdateRequest(colTeams).BindFunc(onTeamUpdate)

	// Party linked to a team (creation sheet « Pour l'équipe … »). Runs inside
	// onPartyCreate (bound before): host already forced, transaction open.
	app.OnRecordCreateRequest(colParties).BindFunc(func(e *core.RecordRequestEvent) error {
		if isGuestRecord(e.Auth) {
			return forbidden(msgGuestNoParty)
		}
		teamID := e.Record.GetString("team")
		if teamID == "" {
			return e.Next()
		}
		team, err := e.App.FindRecordById(colTeams, teamID)
		if err != nil {
			return badRequest("Équipe introuvable.")
		}
		if !isTeamMember(team, e.Record.GetString("host")) {
			return errNotTeamMember()
		}
		if team.GetBool("archived") {
			return badRequest("Cette équipe est archivée.")
		}
		if err := applyTeamDefaults(e.App, e.Record, team); err != nil {
			return err
		}
		// onPartyCreate defaulted split_mode to "equal": the team default wins
		// unless the creator chose one
		if info, err := e.RequestInfo(); err == nil {
			if v, _ := info.Body["split_mode"].(string); v == "" && team.GetString("default_split") != "" {
				e.Record.Set("split_mode", team.GetString("default_split"))
			}
		}
		if err := e.Next(); err != nil {
			return err
		}
		if err := markTeamLaunch(e.App, team, e.Record); err != nil {
			return err
		}
		notifyTeamLaunch(e.App, team, e.Record)
		return nil
	})
	app.OnRecordUpdateRequest(colParties).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() && e.Record.GetString("team") != e.Record.Original().GetString("team") {
			return badRequest("Le champ « team » ne peut pas être modifié directement.")
		}
		return e.Next()
	})
}

// validateTeamFields normalises the editable fields of a team.
func validateTeamFields(app core.App, r *core.Record, orig *core.Record) error {
	name, err := domain.NormalizeTeamName(r.GetString("name"))
	if err != nil {
		return toAPIError(err)
	}
	r.Set("name", name)
	t, err := domain.NormalizeUsualTime(r.GetString("usual_time"))
	if err != nil {
		return toAPIError(err)
	}
	r.Set("usual_time", t)
	days, err := domain.NormalizeDays(r.GetStringSlice("usual_days"))
	if err != nil {
		return toAPIError(err)
	}
	r.Set("usual_days", days)
	color, err := domain.NormalizeColor(r.GetString("color"))
	if err != nil {
		return toAPIError(err)
	}
	r.Set("color", color)
	r.Set("emoji", strings.TrimSpace(r.GetString("emoji")))
	r.Set("address", strings.Join(strings.Fields(r.GetString("address")), " "))
	if r.GetString("default_split") == "" {
		r.Set("default_split", domain.SplitEqual)
	}
	cands := r.GetStringSlice("default_candidates")
	if orig == nil || !slices.Equal(cands, orig.GetStringSlice("default_candidates")) {
		if err := validateCandidates(app, cands); err != nil {
			return err
		}
	}
	return nil
}

func onTeamCreate(e *core.RecordRequestEvent) error {
	r := e.Record
	switch {
	case e.Auth != nil && e.Auth.Collection().Name == colUsers:
		if isGuestRecord(e.Auth) {
			return forbidden(msgGuestNoTeam)
		}
		r.Set("owner", e.Auth.Id)
	case e.HasSuperuserAuth():
		// admin UI: explicit owner
	default:
		return forbidden("Connexion requise.")
	}
	owner := r.GetString("owner")
	if owner == "" {
		return badRequest("Le ou la propriétaire de l'équipe est requis·e.")
	}
	code, err := uniqueTeamCode(e.App)
	if err != nil {
		return err
	}
	r.Set("code", code)
	r.Set("members", []string{owner})
	r.Set("admins", []string{})
	r.Set("archived", false)
	r.Set("last_party", "")
	r.Set("last_launch_at", "")
	if err := validateTeamFields(e.App, r, nil); err != nil {
		return err
	}
	return e.Next()
}

func onTeamUpdate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	r, orig := e.Record, e.Record.Original()
	for _, f := range teamImmutableFields {
		if !sameValue(r, orig, f) {
			return badRequest("Le champ « " + f + " » ne peut pas être modifié directement.")
		}
	}
	if !slices.Equal(r.GetStringSlice("admins"), orig.GetStringSlice("admins")) {
		if e.Auth == nil || orig.GetString("owner") != e.Auth.Id {
			return forbidden("Seul·e le ou la propriétaire de l'équipe choisit les admins.")
		}
		admins := slices.DeleteFunc(slices.Clone(r.GetStringSlice("admins")), func(id string) bool { return id == orig.GetString("owner") })
		r.Set("admins", admins)
		guests, err := guestIDs(e.App, admins)
		if err != nil {
			return err
		}
		if err := domain.CheckTeamAdmins(orig.GetStringSlice("members"), admins, guests); err != nil {
			return toAPIError(err)
		}
	}
	if err := validateTeamFields(e.App, r, orig); err != nil {
		return err
	}
	return e.Next()
}

// guestIDs returns the ids (among ids) of guest accounts.
func guestIDs(app core.App, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	recs, err := app.FindRecordsByIds(colUsers, ids)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, u := range recs {
		if isGuestRecord(u) {
			out = append(out, u.Id)
		}
	}
	return out, nil
}

// applyTeamDefaults fills a new team party with the team defaults (only the
// fields the creator left empty).
func applyTeamDefaults(app core.App, party, team *core.Record) error {
	if strings.TrimSpace(party.GetString("delivery_address")) == "" {
		party.Set("delivery_address", team.GetString("address"))
	}
	if len(party.GetStringSlice("candidates")) == 0 {
		party.Set("candidates", activeRestaurants(app, team.GetStringSlice("default_candidates")))
	}
	if party.GetString("split_mode") == "" {
		if s := team.GetString("default_split"); s != "" {
			party.Set("split_mode", s)
		}
	}
	return nil
}

// activeRestaurants keeps the active restaurants of ids (in order): a
// default candidate deactivated since must not block the launch.
func activeRestaurants(app core.App, ids []string) []string {
	out := []string{}
	if len(ids) == 0 {
		return out
	}
	recs, err := app.FindRecordsByIds(colRestaurants, ids)
	if err != nil {
		return out
	}
	active := map[string]bool{}
	for _, r := range recs {
		active[r.Id] = r.GetBool("active")
	}
	for _, id := range ids {
		if active[id] {
			out = append(out, id)
		}
	}
	return out
}

// markTeamLaunch records the launch on the team: the update is broadcast in
// realtime to every team member (teams view rule), whose shell shows a toast.
func markTeamLaunch(app core.App, team, party *core.Record) error {
	t, err := app.FindRecordById(colTeams, team.Id)
	if err != nil {
		return err
	}
	t.Set("last_party", party.Id)
	t.Set("last_launch_at", types.NowDateTime())
	return app.SaveNoValidate(t)
}

// notifyTeamLaunch calls the push notifier (if any), best effort.
func notifyTeamLaunch(app core.App, team, party *core.Record) {
	if teamNotifier == nil {
		return
	}
	recipients := []string{}
	host := party.GetString("host")
	users, err := app.FindRecordsByIds(colUsers, team.GetStringSlice("members"))
	if err != nil {
		app.Logger().Warn("team launch: members not loaded", "team", team.Id, "error", err)
		return
	}
	for _, u := range users {
		if u.Id != host && !isBanned(u) && !isDeleted(u) {
			recipients = append(recipients, u.Id)
		}
	}
	if err := teamNotifier.TeamPartyLaunched(app, team, party, recipients); err != nil {
		app.Logger().Warn("team launch: notification failed", "team", team.Id, "party", party.Id, "error", err)
	}
}

// addPartyMember adds uid to the party (members + party_members), in tx.
func addPartyMember(tx core.App, p *core.Record, uid string) error {
	p.Set("members+", uid)
	if err := tx.Save(p); err != nil {
		return err
	}
	col, err := tx.FindCollectionByNameOrId(colPartyMembers)
	if err != nil {
		return err
	}
	pm := core.NewRecord(col)
	pm.Load(map[string]any{"party": p.Id, "user": uid, "role": "member", "ready": false})
	return tx.Save(pm)
}

// addTeamMember adds uid to the team members (idempotent), in tx.
func addTeamMember(tx core.App, team *core.Record, uid string) error {
	if isTeamMember(team, uid) {
		return nil
	}
	team.Set("members+", uid)
	return tx.Save(team)
}

// createTeamParty creates the « commande du jour » of a team (server side:
// same invariants as onPartyCreate).
func createTeamParty(tx core.App, team *core.Record, host, title string) (*core.Record, error) {
	col, err := tx.FindCollectionByNameOrId(colParties)
	if err != nil {
		return nil, err
	}
	code, err := uniqueCode(tx)
	if err != nil {
		return nil, err
	}
	p := core.NewRecord(col)
	p.Load(map[string]any{
		"code": code, "title": title, "host": host, "members": []string{host},
		"status": domain.StatusLobby, "team": team.Id,
	})
	if err := applyTeamDefaults(tx, p, team); err != nil {
		return nil, err
	}
	if p.GetString("split_mode") == "" {
		p.Set("split_mode", domain.SplitEqual)
	}
	if err := tx.Save(p); err != nil {
		return nil, err
	}
	pmCol, err := tx.FindCollectionByNameOrId(colPartyMembers)
	if err != nil {
		return nil, err
	}
	pm := core.NewRecord(pmCol)
	pm.Load(map[string]any{"party": p.Id, "user": host, "role": "host", "ready": false})
	if err := tx.Save(pm); err != nil {
		return nil, err
	}
	return p, nil
}

// activeTeamParty returns the most recent party of the team that is neither
// closed nor cancelled (nil if none).
func activeTeamParty(app core.App, teamID string) (*core.Record, error) {
	recs, err := app.FindRecordsByFilter(colParties, "team = {:t} && status != 'closed' && status != 'cancelled'",
		"-created", 1, 0, dbx.Params{"t": teamID})
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	return recs[0], nil
}

// ------------------------------------------------------------------ views

type teamMember struct {
	domain.UserInfo
	IsGuest bool   `json:"isGuest"`
	Role    string `json:"role"`
}

type teamParty struct {
	ID          string             `json:"id"`
	Code        string             `json:"code"`
	Title       string             `json:"title"`
	Status      string             `json:"status"`
	Created     string             `json:"created"`
	Host        domain.UserInfo    `json:"host"`
	MemberCount int                `json:"memberCount"`
	IsMember    bool               `json:"isMember"`
	Restaurant  *historyRestaurant `json:"restaurant"`
}

type teamView struct {
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	Code              string              `json:"code"`
	Emoji             string              `json:"emoji"`
	Color             string              `json:"color"`
	Address           string              `json:"address"`
	Lat               float64             `json:"lat"`
	Lng               float64             `json:"lng"`
	UsualTime         string              `json:"usualTime"`
	UsualDays         []string            `json:"usualDays"`
	DefaultCandidates []historyRestaurant `json:"defaultCandidates"`
	DefaultSplit      string              `json:"defaultSplit"`
	Archived          bool                `json:"archived"`
	Created           string              `json:"created"`
	MyRole            string              `json:"myRole"`
	MemberCount       int                 `json:"memberCount"`
	Members           []teamMember        `json:"members"`
	ActiveParty       *teamParty          `json:"activeParty"`
}

func restaurantView(r *core.Record) historyRestaurant {
	return historyRestaurant{
		ID: r.Id, Name: r.GetString("name"), Emoji: r.GetString("emoji"),
		Cover: r.GetString("cover"), CoverURL: r.GetString("cover_url"), Active: r.GetBool("active"),
	}
}

// buildTeamView: full=false for the lists (members limited to 6, no candidates).
func buildTeamView(app core.App, team *core.Record, me string, full bool) (teamView, error) {
	v := teamView{
		ID: team.Id, Name: team.GetString("name"), Code: team.GetString("code"), Emoji: team.GetString("emoji"),
		Color: team.GetString("color"), Address: team.GetString("address"), Lat: team.GetFloat("lat"), Lng: team.GetFloat("lng"),
		UsualTime: team.GetString("usual_time"), UsualDays: team.GetStringSlice("usual_days"),
		DefaultCandidates: []historyRestaurant{}, DefaultSplit: team.GetString("default_split"),
		Archived: team.GetBool("archived"), Created: team.GetDateTime("created").String(),
		MyRole: string(teamRole(team, me)), Members: []teamMember{},
	}
	if v.UsualDays == nil {
		v.UsualDays = []string{}
	}
	if v.DefaultSplit == "" {
		v.DefaultSplit = domain.SplitEqual
	}
	ids := team.GetStringSlice("members")
	users, err := app.FindRecordsByIds(colUsers, ids)
	if err != nil {
		return v, err
	}
	byID := map[string]*core.Record{}
	for _, u := range users {
		if !isDeleted(u) {
			byID[u.Id] = u
		}
	}
	for _, id := range ids { // join order, owner first
		u, found := byID[id]
		if !found {
			continue
		}
		m := teamMember{UserInfo: userInfo(u), IsGuest: isGuestRecord(u), Role: string(teamRole(team, id))}
		if id == team.GetString("owner") {
			v.Members = append([]teamMember{m}, v.Members...)
		} else {
			v.Members = append(v.Members, m)
		}
	}
	v.MemberCount = len(v.Members)
	if !full && len(v.Members) > 6 {
		v.Members = v.Members[:6]
	}
	if full {
		if cands := team.GetStringSlice("default_candidates"); len(cands) > 0 {
			recs, err := app.FindRecordsByIds(colRestaurants, cands)
			if err != nil {
				return v, err
			}
			byRest := map[string]*core.Record{}
			for _, r := range recs {
				byRest[r.Id] = r
			}
			for _, id := range cands {
				if r, found := byRest[id]; found {
					v.DefaultCandidates = append(v.DefaultCandidates, restaurantView(r))
				}
			}
		}
	}
	p, err := activeTeamParty(app, team.Id)
	if err != nil {
		return v, err
	}
	if p != nil {
		tp := buildTeamParty(app, p, me)
		v.ActiveParty = &tp
	}
	return v, nil
}

func buildTeamParty(app core.App, p *core.Record, me string) teamParty {
	tp := teamParty{
		ID: p.Id, Code: p.GetString("code"), Title: p.GetString("title"), Status: p.GetString("status"),
		Created: p.GetDateTime("created").String(), MemberCount: len(p.GetStringSlice("members")),
		IsMember: isMember(p, me), Host: domain.UserInfo{ID: p.GetString("host"), Name: "Membre"},
	}
	if h, err := app.FindRecordById(colUsers, p.GetString("host")); err == nil {
		tp.Host = userInfo(h)
	}
	if rid := p.GetString("restaurant"); rid != "" {
		if r, err := app.FindRecordById(colRestaurants, rid); err == nil {
			rv := restaurantView(r)
			tp.Restaurant = &rv
		}
	}
	return tp
}

// --------------------------------------------------------------- handlers

func (h *handlers) teamRoutes(g *router.RouterGroup[*core.RequestEvent], user *hookHandler) {
	g.GET("/me/teams", h.myTeams).Bind(user)
	g.POST("/teams/join", h.teamJoin).Bind(user)
	g.GET("/teams/{id}", h.teamDetail).Bind(user)
	g.GET("/teams/{id}/parties", h.teamParties).Bind(user)
	g.POST("/teams/{id}/launch", h.teamLaunch).Bind(user)
	g.POST("/teams/{id}/leave", h.teamLeave).Bind(user)
	g.POST("/teams/{id}/code", h.teamNewCode).Bind(user)
	g.DELETE("/teams/{id}/members/{userId}", h.teamRemoveMember).Bind(user)
	g.POST("/parties/{id}/join", h.partyTeamJoin).Bind(user)
	g.GET("/parties/{id}/team", h.partyTeam).Bind(user)
	g.GET("/invites/{code}", h.invitePreview)
}

// myTeams: GET /api/occ/me/teams — my teams (not archived first, then by name).
func (h *handlers) myTeams(e *core.RequestEvent) error {
	recs, err := e.App.FindRecordsByFilter(colTeams, "members.id ?= {:u}", "archived,name", 0, 0, dbx.Params{"u": e.Auth.Id})
	if err != nil {
		return err
	}
	items := make([]teamView, 0, len(recs))
	for _, t := range recs {
		v, err := buildTeamView(e.App, t, e.Auth.Id, false)
		if err != nil {
			return err
		}
		items = append(items, v)
	}
	return ok(e, map[string]any{"items": items})
}

func (h *handlers) teamDetail(e *core.RequestEvent) error {
	team, err := teamForMember(e)
	if err != nil {
		return err
	}
	v, err := buildTeamView(e.App, team, e.Auth.Id, true)
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"team": v})
}

// teamJoin: POST /api/occ/teams/join {code} — idempotent.
func (h *handlers) teamJoin(e *core.RequestEvent) error {
	var body struct {
		Code string `json:"code"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	code := domain.NormalizeCode(body.Code)
	if !domain.IsValidTeamCode(code) {
		return badRequest("Lien d'équipe invalide.")
	}
	var team *core.Record
	already := false
	err := e.App.RunInTransaction(func(tx core.App) error {
		t, err := tx.FindFirstRecordByData(colTeams, "code", code)
		if err != nil {
			return notFound("Aucune équipe ne correspond à ce lien.")
		}
		team = t
		if isTeamMember(t, e.Auth.Id) {
			already = true
			return nil
		}
		if t.GetBool("archived") {
			return badRequest("Cette équipe est archivée.")
		}
		return addTeamMember(tx, t, e.Auth.Id)
	})
	if err != nil {
		return err
	}
	v, err := buildTeamView(e.App, team, e.Auth.Id, true)
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"team": v, "alreadyMember": already})
}

func (h *handlers) teamLeave(e *core.RequestEvent) error {
	team, err := teamForMember(e)
	if err != nil {
		return err
	}
	if team.GetString("owner") == e.Auth.Id {
		return badRequest("Le ou la propriétaire ne peut pas quitter l'équipe : archive-la plutôt.")
	}
	team.Set("members-", e.Auth.Id)
	team.Set("admins-", e.Auth.Id)
	if err := e.App.Save(team); err != nil {
		return err
	}
	return ok(e, map[string]any{"ok": true})
}

func (h *handlers) teamRemoveMember(e *core.RequestEvent) error {
	team, err := teamForMember(e)
	if err != nil {
		return err
	}
	target := e.Request.PathValue("userId")
	if err := domain.CheckRemoveMember(teamRole(team, e.Auth.Id), teamRole(team, target)); err != nil {
		return toAPIError(err)
	}
	team.Set("members-", target)
	team.Set("admins-", target)
	if err := e.App.Save(team); err != nil {
		return err
	}
	v, err := buildTeamView(e.App, team, e.Auth.Id, true)
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"team": v})
}

// teamNewCode: POST /api/occ/teams/{id}/code — new invite link (old one dead).
func (h *handlers) teamNewCode(e *core.RequestEvent) error {
	team, err := teamForMember(e)
	if err != nil {
		return err
	}
	if !teamRole(team, e.Auth.Id).CanManage() {
		return forbidden("Seuls le ou la propriétaire et les admins de l'équipe peuvent faire cela.")
	}
	code, err := uniqueTeamCode(e.App)
	if err != nil {
		return err
	}
	team.Set("code", code)
	if err := e.App.Save(team); err != nil {
		return err
	}
	return ok(e, map[string]any{"code": code})
}

// teamLaunch: POST /api/occ/teams/{id}/launch {title?} — the « commande du
// jour » (idempotent: an active team party is returned as is).
func (h *handlers) teamLaunch(e *core.RequestEvent) error {
	var body struct {
		Title string `json:"title"`
	}
	if e.Request.ContentLength != 0 {
		if err := bindJSON(e, &body); err != nil {
			return err
		}
	}
	if isGuestRecord(e.Auth) {
		return forbidden(msgGuestNoParty)
	}
	team, err := teamForMember(e)
	if err != nil {
		return err
	}
	if team.GetBool("archived") {
		return badRequest("Cette équipe est archivée.")
	}
	title := strings.Join(strings.Fields(body.Title), " ")
	if title == "" {
		title = domain.DailyTitle(time.Now().In(brussels()))
	}
	if len([]rune(title)) > 120 {
		return badRequest("Le titre est trop long (120 caractères maximum).")
	}
	var party *core.Record
	created := false
	err = e.App.RunInTransaction(func(tx core.App) error {
		existing, err := activeTeamParty(tx, team.Id)
		if err != nil {
			return err
		}
		if existing != nil {
			party = existing
			if !isMember(existing, e.Auth.Id) && slices.Contains(joinableStatuses, existing.GetString("status")) {
				return addPartyMember(tx, existing, e.Auth.Id)
			}
			return nil
		}
		p, err := createTeamParty(tx, team, e.Auth.Id, title)
		if err != nil {
			return err
		}
		party, created = p, true
		return markTeamLaunch(tx, team, p)
	})
	if err != nil {
		return err
	}
	if created {
		notifyTeamLaunch(e.App, team, party)
	}
	return ok(e, map[string]any{"party": party, "created": created})
}

// teamParties: GET /api/occ/teams/{id}/parties?page=&perPage= — the team
// history, newest first (my totals, same code as /me/history).
func (h *handlers) teamParties(e *core.RequestEvent) error {
	team, err := teamForMember(e)
	if err != nil {
		return err
	}
	me := e.Auth.Id
	page := queryInt(e, "page", 1, 1, 100000)
	perPage := queryInt(e, "perPage", 10, 1, 50)
	total, err := e.App.CountRecords(colParties, dbx.HashExp{"team": team.Id})
	if err != nil {
		return err
	}
	var rows []struct {
		ID string `db:"id"`
	}
	err = e.App.DB().NewQuery("SELECT id FROM parties WHERE team = {:t} ORDER BY created DESC, id DESC LIMIT {:l} OFFSET {:o}").
		Bind(dbx.Params{"t": team.Id, "l": perPage, "o": (page - 1) * perPage}).All(&rows)
	if err != nil {
		return err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	b, err := loadBundle(e.App, ids, me)
	if err != nil {
		return err
	}
	type item struct {
		Party    historyEntry `json:"party"`
		IsMember bool         `json:"isMember"`
	}
	items := make([]item, 0, len(b.parties))
	for _, p := range b.parties {
		items = append(items, item{Party: b.entry(p, me), IsMember: isMember(p, me)})
	}
	return ok(e, map[string]any{
		"page": page, "perPage": perPage, "totalItems": total,
		"totalPages": (int(total) + perPage - 1) / perPage, "items": items,
	})
}

// partyTeamJoin: POST /api/occ/parties/{id}/join — one tap join for the
// members of the party's team (no code).
func (h *handlers) partyTeamJoin(e *core.RequestEvent) error {
	var party *core.Record
	already := false
	err := e.App.RunInTransaction(func(tx core.App) error {
		p, err := findParty(tx, e.Request.PathValue("id"))
		if err != nil {
			return err
		}
		party = p
		if isMember(p, e.Auth.Id) {
			already = true
			return nil
		}
		teamID := p.GetString("team")
		if teamID == "" {
			return forbidden("Cette commande n'appartient à aucune équipe : demande le code à l'hôte.")
		}
		team, err := tx.FindRecordById(colTeams, teamID)
		if err != nil || !isTeamMember(team, e.Auth.Id) {
			return errNotTeamMember()
		}
		if !slices.Contains(joinableStatuses, p.GetString("status")) {
			return badRequest("Cette commande n'accepte plus de nouveaux participants.")
		}
		return addPartyMember(tx, p, e.Auth.Id)
	})
	if err != nil {
		return err
	}
	return ok(e, map[string]any{"party": party, "alreadyMember": already})
}

// partyTeam: GET /api/occ/parties/{id}/team — the team of the party and its
// members who have not joined yet (lobby « pas encore là »).
func (h *handlers) partyTeam(e *core.RequestEvent) error {
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	out := map[string]any{"team": nil, "missing": []teamMember{}}
	teamID := party.GetString("team")
	if teamID == "" {
		return ok(e, out)
	}
	team, err := e.App.FindRecordById(colTeams, teamID)
	if err != nil {
		return ok(e, out)
	}
	out["team"] = map[string]any{
		"id": team.Id, "name": team.GetString("name"), "emoji": team.GetString("emoji"), "color": team.GetString("color"),
		"isMember": isTeamMember(team, e.Auth.Id),
	}
	missingIDs := []string{}
	for _, id := range team.GetStringSlice("members") {
		if !isMember(party, id) {
			missingIDs = append(missingIDs, id)
		}
	}
	missing := []teamMember{}
	if len(missingIDs) > 0 {
		users, err := e.App.FindRecordsByIds(colUsers, missingIDs)
		if err != nil {
			return err
		}
		byID := map[string]*core.Record{}
		for _, u := range users {
			byID[u.Id] = u
		}
		for _, id := range missingIDs {
			if u, found := byID[id]; found && !isDeleted(u) && !isBanned(u) {
				missing = append(missing, teamMember{UserInfo: userInfo(u), IsGuest: isGuestRecord(u), Role: string(teamRole(team, id))})
			}
		}
	}
	out["missing"] = missing
	return ok(e, out)
}

// invitePreview: GET /api/occ/invites/{code} — public preview of an invite
// link (party: 6 characters, team: 8) for the « Continuer en invité » page.
// Rate limited per IP like the guest creation.
func (h *handlers) invitePreview(e *core.RequestEvent) error {
	if !h.guests.preview.Allow(e.RealIP(), time.Now()) {
		return errTooManyRequests()
	}
	code := domain.NormalizeCode(e.Request.PathValue("code"))
	switch {
	case domain.IsValidCode(code):
		p, err := e.App.FindFirstRecordByData(colParties, "code", code)
		if err != nil {
			return notFound("Aucune commande ne correspond à ce code.")
		}
		host := "Un·e collègue"
		if u, err := e.App.FindRecordById(colUsers, p.GetString("host")); err == nil {
			host = userInfo(u).Name
		}
		return ok(e, map[string]any{
			"kind": "party", "code": code, "title": p.GetString("title"), "host": host,
			"status": p.GetString("status"), "memberCount": len(p.GetStringSlice("members")),
			"joinable": slices.Contains(joinableStatuses, p.GetString("status")),
		})
	case domain.IsValidTeamCode(code):
		t, err := e.App.FindFirstRecordByData(colTeams, "code", code)
		if err != nil {
			return notFound("Aucune équipe ne correspond à ce lien.")
		}
		return ok(e, map[string]any{
			"kind": "team", "code": code, "title": t.GetString("name"), "emoji": t.GetString("emoji"),
			"color": t.GetString("color"), "memberCount": len(t.GetStringSlice("members")),
			"joinable": !t.GetBool("archived"),
		})
	}
	return badRequest("Lien d'invitation invalide.")
}

// lastActivitySQL is the most recent activity of a user (cleanup of guests).
func lastActivity(app core.App, userID string) (time.Time, error) {
	var raw sql.NullString
	err := app.DB().NewQuery(`SELECT MAX(d) FROM (
		SELECT u.updated AS d FROM users u WHERE u.id = {:u}
		UNION ALL SELECT MAX(pm.created) FROM party_members pm WHERE pm.user = {:u}
		UNION ALL SELECT MAX(oi.updated) FROM order_items oi WHERE oi.user = {:u}
		UNION ALL SELECT MAX(v.created) FROM votes v WHERE v.user = {:u}
		UNION ALL SELECT MAX(p.updated) FROM payments p WHERE p.debtor = {:u} OR p.creditor = {:u}
		UNION ALL SELECT MAX(ao.updated) FROM _authOrigins ao WHERE ao.recordRef = {:u}
	)`).Bind(dbx.Params{"u": userID}).Row(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, err
	}
	if !raw.Valid || raw.String == "" {
		return time.Time{}, nil
	}
	d, err := types.ParseDateTime(raw.String)
	if err != nil {
		return time.Time{}, err
	}
	return d.Time(), nil
}
