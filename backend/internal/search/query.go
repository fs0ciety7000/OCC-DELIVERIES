package search

import (
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Params of a search (GET /api/occ/search).
type Params struct {
	Query string
	// Limit per group (restaurants, dishes, people).
	Limit int
	// Lat / Lng: optional position for distanceKm (HasGeo).
	Lat, Lng float64
	HasGeo   bool
	// MinItems: incomplete-menu threshold (app_settings.min_menu_items).
	MinItems int
	// UserID: authenticated user ("" = anonymous → no people).
	UserID string
	// Admin: adds the « Admin » action.
	Admin bool
}

// DefaultLimit / MaxLimit bound Params.Limit.
const (
	DefaultLimit = 6
	MaxLimit     = 20
)

// RestaurantHit is a restaurant result.
type RestaurantHit struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Emoji      string   `json:"emoji"`
	Cuisines   []string `json:"cuisines"`
	ItemsCount int      `json:"itemsCount"`
	DistanceKm *float64 `json:"distanceKm,omitempty"`
	score      float64
}

// RestaurantRef is the restaurant of a dish.
type RestaurantRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
}

// DishHit is a menu item result (available items of visible restaurants only).
type DishHit struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Price      int           `json:"price"`
	Emoji      string        `json:"emoji"`
	Snippet    string        `json:"snippet"`
	Restaurant RestaurantRef `json:"restaurant"`
	score      float64
}

// SharedParty is a party shared with a colleague.
type SharedParty struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Created string `json:"created"`
}

// PersonHit is a colleague (shares at least one party — or team — with the user).
type PersonHit struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Avatar        string        `json:"avatar"`
	Color         string        `json:"color"`
	SharedParties int           `json:"sharedParties"`
	RecentParties []SharedParty `json:"recentParties"`
}

// Action is a static shortcut of the command palette.
type Action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Href  string `json:"href"`
}

// Result is the body of GET /api/occ/search.
type Result struct {
	Query string `json:"query"`
	// Terms: folded terms actually searched (typo corrections included), for highlighting.
	Terms []string `json:"terms"`
	// Fuzzy: at least one term was corrected (« Résultats approchants »).
	Fuzzy       bool            `json:"fuzzy"`
	Restaurants []RestaurantHit `json:"restaurants"`
	Dishes      []DishHit       `json:"dishes"`
	People      []PersonHit     `json:"people"`
	Actions     []Action        `json:"actions"`
}

// Search runs the global search.
func Search(app core.App, p Params) (*Result, error) {
	if p.Limit <= 0 {
		p.Limit = DefaultLimit
	}
	p.Limit = min(p.Limit, MaxLimit)
	res := &Result{
		Query:       strings.TrimSpace(p.Query),
		Terms:       []string{},
		Restaurants: []RestaurantHit{},
		Dishes:      []DishHit{},
		People:      []PersonHit{},
	}
	terms := Terms(p.Query)
	res.Actions = actions(terms, p)
	// a single letter matches too much to be useful
	if len(terms) == 0 || (len(terms) == 1 && len([]rune(terms[0])) < 2) {
		return res, nil
	}

	groups := make([][]string, len(terms))
	if Ready(app) {
		vocab := cachedVocab(app)
		for i, t := range terms {
			if alts := vocab.Corrections(t); len(alts) > 0 {
				groups[i] = alts
				res.Fuzzy = true
			}
		}
	}
	for i, t := range terms {
		if groups[i] == nil {
			res.Terms = append(res.Terms, t)
		} else {
			res.Terms = append(res.Terms, groups[i]...)
		}
	}

	if Ready(app) {
		var err error
		if res.Restaurants, err = searchRestaurants(app, p, terms, groups); err != nil {
			return nil, err
		}
		if res.Dishes, err = searchDishes(app, p, terms, groups); err != nil {
			return nil, err
		}
	}
	if p.UserID != "" {
		people, err := searchPeople(app, p, terms)
		if err != nil {
			return nil, err
		}
		res.People = people
	}
	return res, nil
}

// termExpr is the FTS5 expression of one term: a prefix query, or its
// corrections when the term is unknown.
func termExpr(term string, alts []string) string {
	if len(alts) == 0 {
		return `"` + term + `"*`
	}
	parts := make([]string, 0, len(alts))
	for _, a := range alts {
		parts = append(parts, `"`+a+`"*`)
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// matchExpr: every term must match (implicit AND). Terms only contain
// letters and digits (Terms), quoting is safe.
func matchExpr(terms []string, groups [][]string) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = termExpr(t, groups[i])
	}
	return strings.Join(parts, " AND ")
}

// candidateFactor: FTS candidates fetched per result before re-ranking.
const candidateFactor = 4

type restaurantCand struct {
	ID         string  `db:"id"`
	Name       string  `db:"name"`
	Emoji      string  `db:"emoji"`
	Cuisines   string  `db:"cuisines"`
	ItemsCount int     `db:"items_count"`
	Lat        float64 `db:"lat"`
	Lng        float64 `db:"lng"`
	Approx     bool    `db:"geo_approx"`
	Rank       float64 `db:"rank"`
}

func searchRestaurants(app core.App, p Params, terms []string, groups [][]string) ([]RestaurantHit, error) {
	var cands []restaurantCand
	// column weights: name 10, cuisines 4, address 1 (bm25 is negative: lower = better)
	err := app.DB().NewQuery(`SELECT r.id AS id, r.name AS name, r.emoji AS emoji, r.cuisines AS cuisines,
			r.items_count AS items_count, r.lat AS lat, r.lng AS lng, r.geo_approx AS geo_approx,
			bm25(` + TableRestaurants + `, 10.0, 4.0, 1.0) AS rank
		FROM ` + TableRestaurants + ` f
		JOIN ` + TableRestaurantDocs + ` d ON d.docid = f.rowid
		JOIN restaurants r ON r.id = d.restaurant
		WHERE ` + TableRestaurants + ` MATCH {:q} AND r.active = TRUE AND ({:min} = 0 OR r.items_count >= {:min})
		ORDER BY rank LIMIT {:lim}`).
		Bind(dbx.Params{"q": matchExpr(terms, groups), "min": p.MinItems, "lim": p.Limit * candidateFactor}).
		All(&cands)
	if err != nil {
		return nil, err
	}
	phrase := strings.Join(terms, " ")
	primary := primaryTerms(terms, groups)
	fuzzy := slicesAnyNonNil(groups)
	hits := make([]RestaurantHit, 0, len(cands))
	for _, c := range cands {
		cuisines := jsonList(c.Cuisines)
		if cuisines == nil {
			cuisines = []string{}
		}
		name := strings.Join(Words(c.Name), " ")
		score := -c.Rank
		switch {
		case name == phrase: // exact name
			score += 100
		case strings.HasPrefix(name, phrase):
			score += 40
		case allWordPrefixes(Words(c.Name), terms):
			score += 20
		}
		for _, cu := range cuisines {
			if cf := Fold(cu); cf == phrase || containsWord(primary, cf) {
				score += 8
				break
			}
		}
		if fuzzy && allWordPrefixes(Words(c.Name+" "+strings.Join(cuisines, " ")), primary) {
			score += fuzzyBonus
		}
		h := RestaurantHit{ID: c.ID, Name: c.Name, Emoji: c.Emoji, Cuisines: cuisines, ItemsCount: c.ItemsCount, score: score}
		if p.HasGeo && !c.Approx && (c.Lat != 0 || c.Lng != 0) {
			d := math.Round(domain.HaversineKm(p.Lat, p.Lng, c.Lat, c.Lng)*100) / 100
			h.DistanceKm = &d
		}
		hits = append(hits, h)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].Name < hits[j].Name
	})
	return hits[:min(len(hits), p.Limit)], nil
}

type dishCand struct {
	ID          string  `db:"id"`
	Name        string  `db:"name"`
	Description string  `db:"description"`
	Price       int     `db:"price"`
	Emoji       string  `db:"emoji"`
	Popular     bool    `db:"popular"`
	RID         string  `db:"rid"`
	RName       string  `db:"rname"`
	REmoji      string  `db:"remoji"`
	Rank        float64 `db:"rank"`
}

func searchDishes(app core.App, p Params, terms []string, groups [][]string) ([]DishHit, error) {
	// At least one term must hit the dish itself (name, description,
	// category): « tomo » must not list the whole menu of « Tomo ».
	any := make([]string, len(terms))
	for i, t := range terms {
		any[i] = termExpr(t, groups[i])
	}
	q := "{name description category} : (" + strings.Join(any, " OR ") + ") AND " + matchExpr(terms, groups)
	var cands []dishCand
	// column weights: name 8, description 1, category 3, restaurant 2
	err := app.DB().NewQuery(`SELECT m.id AS id, m.name AS name, m.description AS description, m.price AS price,
			m.emoji AS emoji, m.popular AS popular, r.id AS rid, r.name AS rname, r.emoji AS remoji,
			bm25(` + TableItems + `, 8.0, 1.0, 3.0, 2.0) AS rank
		FROM ` + TableItems + ` f
		JOIN ` + TableItemDocs + ` d ON d.docid = f.rowid
		JOIN menu_items m ON m.id = d.item
		JOIN restaurants r ON r.id = m.restaurant
		WHERE ` + TableItems + ` MATCH {:q} AND m.available = TRUE AND r.active = TRUE
			AND ({:min} = 0 OR r.items_count >= {:min})
		ORDER BY rank LIMIT {:lim}`).
		Bind(dbx.Params{"q": q, "min": p.MinItems, "lim": p.Limit * candidateFactor}).
		All(&cands)
	if err != nil {
		return nil, err
	}
	phrase := strings.Join(terms, " ")
	primary := primaryTerms(terms, groups)
	fuzzy := slicesAnyNonNil(groups)
	hits := make([]DishHit, 0, len(cands))
	for _, c := range cands {
		words := Words(c.Name)
		name := strings.Join(words, " ")
		score := -c.Rank
		switch {
		case name == phrase:
			score += 50
		case strings.HasPrefix(name, phrase):
			score += 20
		case allWordPrefixes(words, terms):
			score += 10
		}
		if c.Popular {
			score += 2
		}
		if fuzzy && allWordPrefixes(words, primary) {
			score += fuzzyBonus
		}
		hits = append(hits, DishHit{
			ID: c.ID, Name: c.Name, Price: c.Price, Emoji: c.Emoji,
			Snippet:    Snippet(c.Description, snippetTerms(terms, groups), 90),
			Restaurant: RestaurantRef{ID: c.RID, Name: c.RName, Emoji: c.REmoji},
			score:      score,
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		if hits[i].Name != hits[j].Name {
			return hits[i].Name < hits[j].Name
		}
		return hits[i].ID < hits[j].ID
	})
	return hits[:min(len(hits), p.Limit)], nil
}

// primaryTerms: each term, or its best correction (closest, then most frequent).
func primaryTerms(terms []string, groups [][]string) []string {
	out := make([]string, len(terms))
	for i, t := range terms {
		out[i] = t
		if len(groups[i]) > 0 {
			out[i] = groups[i][0]
		}
	}
	return out
}

// fuzzyBonus favours the results matching the best corrections (« piza »:
// « pizza » before « pita », which bm25 would rank first for being rarer).
const fuzzyBonus = 6

func snippetTerms(terms []string, groups [][]string) []string {
	out := []string{}
	for i, t := range terms {
		if groups[i] == nil {
			out = append(out, t)
		} else {
			out = append(out, groups[i]...)
		}
	}
	return out
}

// allWordPrefixes: every term is the prefix of a word.
func allWordPrefixes(words, terms []string) bool {
	for _, t := range terms {
		found := false
		for _, w := range words {
			if strings.HasPrefix(w, t) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func slicesAnyNonNil(groups [][]string) bool {
	for _, g := range groups {
		if g != nil {
			return true
		}
	}
	return false
}

func containsWord(list []string, w string) bool {
	for _, x := range list {
		if x == w {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- actions

type actionDef struct {
	Action
	keywords string
	auth     bool
	admin    bool
}

var actionDefs = []actionDef{
	{Action: Action{ID: "new-party", Label: "Lancer une commande", Href: "/?lancer=1"}, keywords: "nouvelle commande groupee creer salon party"},
	{Action: Action{ID: "restaurants", Label: "Restos à proximité", Href: "/restaurants"}, keywords: "restaurants carte autour proche"},
	{Action: Action{ID: "my-orders", Label: "Mes commandes", Href: "/profile"}, keywords: "historique profil commandes passees", auth: true},
	{Action: Action{ID: "admin", Label: "Admin", Href: "/admin"}, keywords: "administration panneau gestion", admin: true},
}

// actions returns the static shortcuts matching the terms (all of them
// without terms), filtered by role.
func actions(terms []string, p Params) []Action {
	out := []Action{}
	for _, a := range actionDefs {
		if (a.auth && p.UserID == "") || (a.admin && !p.Admin) {
			continue
		}
		if len(terms) > 0 && !allWordPrefixes(Words(a.Label+" "+a.keywords), terms) {
			continue
		}
		out = append(out, a.Action)
	}
	return out
}

// ---------------------------------------------------------------- vocabulary cache

type vocabEntry struct {
	gen   int64
	vocab Vocab
}

var vocabCache sync.Map // data dir → vocabEntry

// cachedVocab returns the indexed terms (both tables), cached until the next index write.
func cachedVocab(app core.App) Vocab {
	key := app.DataDir()
	gen := generation.Load()
	if v, ok := vocabCache.Load(key); ok && v.(vocabEntry).gen == gen {
		return v.(vocabEntry).vocab
	}
	var rows []struct {
		Term string `db:"term"`
		Docs int    `db:"doc"`
	}
	all := []VocabTerm{}
	for _, t := range []string{TableVocabRestaurant, TableVocabItems} {
		rows = rows[:0]
		if err := app.DB().NewQuery("SELECT term, doc FROM " + t).All(&rows); err != nil {
			continue
		}
		for _, r := range rows {
			all = append(all, VocabTerm{Term: r.Term, Docs: r.Docs})
		}
	}
	v := NewVocab(all)
	vocabCache.Store(key, vocabEntry{gen: gen, vocab: v})
	return v
}

// ---------------------------------------------------------------- people

type personRow struct {
	ID     string `db:"id"`
	Name   string `db:"name"`
	Avatar string `db:"avatar"`
	Color  string `db:"color"`
}

// colleagueIDs returns the users who share a party (or a team, when the
// teams collection exists) with userID. Nobody else is ever searched.
func colleagueIDs(app core.App, userID string) ([]string, error) {
	var ids []string
	err := app.DB().NewQuery(`SELECT DISTINCT b.user FROM party_members a
		JOIN party_members b ON b.party = a.party
		WHERE a.user = {:me} AND b.user != {:me}`).Bind(dbx.Params{"me": userID}).Column(&ids)
	if err != nil {
		return nil, err
	}
	// teams (migration 1760000016): owner / admins / members are relations
	// stored as JSON arrays; skipped while the collection does not exist
	if col, err := app.FindCachedCollectionByNameOrId("teams"); err == nil &&
		col.Fields.GetByName("members") != nil && col.Fields.GetByName("owner") != nil && col.Fields.GetByName("admins") != nil {
		var team []string
		if err := app.DB().NewQuery(`WITH mine AS (
				SELECT t.owner AS owner, CASE WHEN json_valid(t.members) THEN t.members ELSE '[]' END AS members,
					CASE WHEN json_valid(t.admins) THEN t.admins ELSE '[]' END AS admins
				FROM teams t
				WHERE t.owner = {:me}
					OR EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(t.members) THEN t.members ELSE '[]' END) WHERE value = {:me})
					OR EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(t.admins) THEN t.admins ELSE '[]' END) WHERE value = {:me}))
			SELECT owner AS u FROM mine
			UNION SELECT j.value FROM mine, json_each(mine.members) j
			UNION SELECT j.value FROM mine, json_each(mine.admins) j`).Bind(dbx.Params{"me": userID}).Column(&team); err == nil {
			for _, id := range team {
				if id != "" && id != userID {
					ids = append(ids, id)
				}
			}
		}
	}
	return ids, nil
}

func searchPeople(app core.App, p Params, terms []string) ([]PersonHit, error) {
	ids, err := colleagueIDs(app, p.UserID)
	if err != nil || len(ids) == 0 {
		return []PersonHit{}, err
	}
	anyIDs := make([]any, len(ids))
	for i, id := range ids {
		anyIDs[i] = id
	}
	var rows []personRow
	// deleted (anonymised) and suspended accounts are never suggested
	err = app.DB().Select("id", "name", "avatar", "color").
		From("users").
		Where(dbx.In("id", anyIDs...)).
		AndWhere(dbx.NewExp("COALESCE([[deleted_at]], '') = '' AND COALESCE([[banned]], FALSE) = FALSE")).
		All(&rows)
	if err != nil {
		return nil, err
	}
	type scored struct {
		row   personRow
		score int
	}
	phrase := strings.Join(terms, " ")
	var matched []scored
	for _, r := range rows {
		words := Words(r.Name)
		if !allWordPrefixes(words, terms) {
			continue
		}
		s := 1
		if name := strings.Join(words, " "); name == phrase {
			s = 3
		} else if strings.HasPrefix(name, phrase) {
			s = 2
		}
		matched = append(matched, scored{r, s})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score != matched[j].score {
			return matched[i].score > matched[j].score
		}
		return matched[i].row.Name < matched[j].row.Name
	})
	limit := min(p.Limit, 5)
	out := []PersonHit{}
	for _, m := range matched[:min(len(matched), limit)] {
		h := PersonHit{ID: m.row.ID, Name: m.row.Name, Avatar: m.row.Avatar, Color: m.row.Color, RecentParties: []SharedParty{}}
		if err := app.DB().NewQuery(`SELECT COUNT(DISTINCT a.party) FROM party_members a
			JOIN party_members b ON b.party = a.party AND b.user = {:other}
			WHERE a.user = {:me}`).Bind(dbx.Params{"me": p.UserID, "other": m.row.ID}).Row(&h.SharedParties); err != nil {
			return nil, err
		}
		var parties []struct {
			ID      string `db:"id"`
			Code    string `db:"code"`
			Title   string `db:"title"`
			Status  string `db:"status"`
			Created string `db:"created"`
		}
		if err := app.DB().NewQuery(`SELECT p.id AS id, p.code AS code, p.title AS title, p.status AS status, p.created AS created
			FROM parties p
			JOIN party_members a ON a.party = p.id AND a.user = {:me}
			JOIN party_members b ON b.party = p.id AND b.user = {:other}
			ORDER BY p.created DESC LIMIT 3`).Bind(dbx.Params{"me": p.UserID, "other": m.row.ID}).All(&parties); err != nil {
			return nil, err
		}
		for _, x := range parties {
			h.RecentParties = append(h.RecentParties, SharedParty(x))
		}
		out = append(out, h)
	}
	return out, nil
}
