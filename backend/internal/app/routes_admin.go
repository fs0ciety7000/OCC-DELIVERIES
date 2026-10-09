package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// ------------------------------------------------------------------ stats

type dayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type topRestaurant struct {
	ID      string `db:"id" json:"id"`
	Name    string `db:"name" json:"name"`
	Slug    string `db:"slug" json:"slug"`
	Emoji   string `db:"emoji" json:"emoji"`
	Parties int    `db:"parties" json:"parties"`
	Amount  int    `db:"amount" json:"amount"`
}

func (h *handlers) adminStats(e *core.RequestEvent) error {
	db := e.App.DB()
	count := func(col string, where dbx.Expression) (int, error) {
		n, err := e.App.CountRecords(col, where)
		return int(n), err
	}
	users, err := count(colUsers, nil)
	if err != nil {
		return err
	}
	admins, err := count(colUsers, dbx.HashExp{"role": roleAdmin})
	if err != nil {
		return err
	}
	restTotal, err := count(colRestaurants, nil)
	if err != nil {
		return err
	}
	restActive, err := count(colRestaurants, dbx.HashExp{"active": true})
	if err != nil {
		return err
	}
	menuItems, err := count(colMenuItems, nil)
	if err != nil {
		return err
	}

	byStatus := map[string]int{}
	for _, s := range domain.Statuses {
		byStatus[s] = 0
	}
	var rows []struct {
		Status string `db:"status"`
		N      int    `db:"n"`
	}
	if err := db.NewQuery("SELECT status, COUNT(*) AS n FROM parties GROUP BY status").All(&rows); err != nil {
		return err
	}
	totalParties := 0
	for _, r := range rows {
		byStatus[r.Status] = r.N
		totalParties += r.N
	}

	// parties created per day over the last 30 days (UTC), zero filled
	const days = 30
	today := time.Now().UTC().Truncate(24 * time.Hour)
	since := today.AddDate(0, 0, -(days - 1))
	var perDayRows []struct {
		Day string `db:"day"`
		N   int    `db:"n"`
	}
	err = db.NewQuery("SELECT substr(created, 1, 10) AS day, COUNT(*) AS n FROM parties WHERE created >= {:since} GROUP BY day").
		Bind(dbx.Params{"since": since.Format(types.DefaultDateLayout)}).All(&perDayRows)
	if err != nil {
		return err
	}
	perDayMap := map[string]int{}
	for _, r := range perDayRows {
		perDayMap[r.Day] = r.N
	}
	perDay := make([]dayCount, 0, days)
	for d := since; !d.After(today); d = d.AddDate(0, 0, 1) {
		key := d.Format(time.DateOnly)
		perDay = append(perDay, dayCount{Date: key, Count: perDayMap[key]})
	}

	var totals struct {
		Amount int `db:"amount"`
		Lines  int `db:"lines"`
	}
	err = db.NewQuery(`SELECT COALESCE(SUM(oi.total), 0) AS amount, COUNT(oi.id) AS lines
		FROM order_items oi JOIN parties p ON p.id = oi.party WHERE p.status != {:cancelled}`).
		Bind(dbx.Params{"cancelled": domain.StatusCancelled}).One(&totals)
	if err != nil {
		return err
	}

	top := []topRestaurant{}
	err = db.NewQuery(`SELECT r.id AS id, r.name AS name, r.slug AS slug, r.emoji AS emoji,
			COUNT(DISTINCT p.id) AS parties,
			COALESCE((SELECT SUM(oi.total) FROM order_items oi JOIN parties p2 ON p2.id = oi.party
				WHERE p2.restaurant = r.id AND p2.status != {:cancelled}), 0) AS amount
		FROM parties p JOIN restaurants r ON r.id = p.restaurant
		WHERE p.status != {:cancelled}
		GROUP BY r.id ORDER BY parties DESC, amount DESC, r.name ASC LIMIT 5`).
		Bind(dbx.Params{"cancelled": domain.StatusCancelled}).All(&top)
	if err != nil {
		return err
	}

	return ok(e, map[string]any{
		"users":          users,
		"admins":         admins,
		"restaurants":    map[string]int{"total": restTotal, "active": restActive},
		"menuItems":      menuItems,
		"parties":        map[string]any{"total": totalParties, "byStatus": byStatus},
		"partiesPerDay":  perDay,
		"orderedTotal":   totals.Amount,
		"orderedLines":   totals.Lines,
		"topRestaurants": top,
	})
}

// ----------------------------------------------------------------- import

func isDryRun(e *core.RequestEvent) bool {
	v := strings.ToLower(e.Request.URL.Query().Get("dryRun"))
	return v == "1" || v == "true" || v == "yes"
}

// decodeImport accepts a single RestaurantImport object or an array of them.
func decodeImport(body []byte) ([]catalog.RestaurantImport, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, badRequest("Corps de requête vide.")
	}
	if body[0] == '[' {
		var list []catalog.RestaurantImport
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, badRequest("JSON invalide : " + err.Error())
		}
		return list, nil
	}
	var one catalog.RestaurantImport
	if err := json.Unmarshal(body, &one); err != nil {
		return nil, badRequest("JSON invalide : " + err.Error())
	}
	return []catalog.RestaurantImport{one}, nil
}

// respondImport runs the dry run or the import and writes the report.
// An invalid batch answers 400 with the report (nothing is written).
func respondImport(e *core.RequestEvent, list []catalog.RestaurantImport, extraErrors []string) error {
	var (
		rep catalog.Report
		err error
	)
	if isDryRun(e) || len(extraErrors) > 0 {
		rep = catalog.Check(e.App, list)
		rep.DryRun = isDryRun(e)
	} else {
		rep, err = catalog.ImportAll(e.App, list)
		if err != nil {
			return badRequest("Import impossible : " + err.Error())
		}
	}
	rep.Errors = append(extraErrors, rep.Errors...)
	rep.Valid = rep.Valid && len(extraErrors) == 0
	if !rep.Valid && !rep.DryRun {
		return e.JSON(http.StatusBadRequest, map[string]any{
			"status":  http.StatusBadRequest,
			"message": "Import refusé : " + strconv.Itoa(rep.ErrorCount()) + " erreur(s). Rien n'a été modifié.",
			"data":    map[string]any{},
			"report":  rep,
		})
	}
	out := map[string]any{"report": rep}
	// backward compatible fields for a single restaurant
	if len(rep.Restaurants) > 0 {
		out["restaurant"] = rep.Restaurants[0].Restaurant
		out["items"] = rep.Items
	}
	return ok(e, out)
}

func (h *handlers) adminImport(e *core.RequestEvent) error {
	body, err := io.ReadAll(io.LimitReader(e.Request.Body, 20<<20))
	if err != nil {
		return badRequest("Corps de requête illisible.")
	}
	list, err := decodeImport(body)
	if err != nil {
		return err
	}
	return respondImport(e, list, nil)
}

func (h *handlers) adminImportCSV(e *core.RequestEvent) error {
	f, _, err := e.Request.FormFile("file")
	if err != nil {
		return badRequest("Fichier CSV manquant (champ « file »).")
	}
	defer f.Close()
	parsed, lineErrors := catalog.ParseCSV(f)
	list := make([]catalog.RestaurantImport, 0, len(parsed))
	for _, c := range parsed {
		existing, err := catalog.ExportOne(e.App, c.Import.Slug)
		if err != nil {
			return err
		}
		list = append(list, catalog.Merge(existing, c))
	}
	return respondImport(e, list, lineErrors)
}

// ----------------------------------------------------------------- export

func (h *handlers) adminExport(e *core.RequestEvent) error {
	list, err := catalog.ExportAll(e.App)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	name := "occ-restaurants-" + time.Now().UTC().Format("2006-01-02") + ".json"
	e.Response.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	return e.Blob(http.StatusOK, "application/json; charset=utf-8", data)
}

// ------------------------------------------------------------------ users

type adminUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Color    string `json:"color"`
	Avatar   string `json:"avatar"`
	Verified bool   `json:"verified"`
	Created  string `json:"created"`
	Parties  int    `json:"parties"`
}

func toAdminUser(app core.App, r *core.Record) adminUser {
	role := r.GetString("role")
	if role == "" {
		role = roleUser
	}
	n, _ := app.CountRecords(colPartyMembers, dbx.HashExp{"user": r.Id})
	return adminUser{
		ID: r.Id, Name: r.GetString("name"), Email: r.Email(), Role: role,
		Color: r.GetString("color"), Avatar: r.GetString("avatar"), Verified: r.Verified(),
		Created: r.GetDateTime("created").String(), Parties: int(n),
	}
}

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
	filter := "id != ''"
	params := dbx.Params{}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		filter = "(name ~ {:q} || email ~ {:q})"
		params["q"] = s
	}
	if role := q.Get("role"); role == roleAdmin || role == roleUser {
		filter += " && role = {:role}"
		params["role"] = role
	}
	all, err := e.App.FindRecordsByFilter(colUsers, filter, "-created", 0, 0, params)
	if err != nil {
		return err
	}
	start := (page - 1) * perPage
	items := []adminUser{}
	for i := start; i < len(all) && i < start+perPage; i++ {
		items = append(items, toAdminUser(e.App, all[i]))
	}
	return ok(e, map[string]any{"page": page, "perPage": perPage, "totalItems": len(all), "items": items})
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
	u, err := e.App.FindRecordById(colUsers, e.Request.PathValue("id"))
	if err != nil {
		return notFound("Utilisateur introuvable.")
	}
	if u.Id == e.Auth.Id && body.Role != roleAdmin {
		return badRequest("Vous ne pouvez pas retirer vos propres droits d'administration.")
	}
	u.Set("role", body.Role)
	if err := e.App.Save(u); err != nil {
		return err
	}
	return ok(e, map[string]any{"user": toAdminUser(e.App, u)})
}

// ---------------------------------------------------------------- parties

func (h *handlers) adminCancelParty(e *core.RequestEvent) error {
	p, err := findParty(e.App, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	switch p.GetString("status") {
	case domain.StatusClosed:
		return badRequest("Cette commande est déjà clôturée.")
	case domain.StatusCancelled:
		return badRequest("Cette commande est déjà annulée.")
	}
	p.Set("status", domain.StatusCancelled)
	if err := e.App.Save(p); err != nil {
		return err
	}
	e.App.Logger().Info("party force cancelled", "party", p.Id, "by", e.Auth.Id)
	return ok(e, map[string]any{"party": p})
}
