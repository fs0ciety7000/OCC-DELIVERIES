package app

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// ------------------------------------------------------------------ types

// historyRestaurant mirrors the restaurant record fields the cards need
// (same JSON names as the collection so the front can reuse its components).
type historyRestaurant struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Emoji    string `json:"emoji"`
	Cover    string `json:"cover"`
	CoverURL string `json:"cover_url"`
	Active   bool   `json:"active"`
}

type historyItem struct {
	MenuItem     string `json:"menuItem"`
	Name         string `json:"name"`
	OptionsLabel string `json:"optionsLabel"`
	Note         string `json:"note"`
	Quantity     int    `json:"quantity"`
	UnitPrice    int    `json:"unitPrice"`
	Total        int    `json:"total"`
}

type historyPayment struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Status string `json:"status"`
	Amount int    `json:"amount"`
}

// historyEntry is one party of GET /api/occ/me/history (see ARCHITECTURE §5).
type historyEntry struct {
	ID          string             `json:"id"`
	Code        string             `json:"code"`
	Title       string             `json:"title"`
	Status      string             `json:"status"`
	Created     string             `json:"created"`
	ClosedAt    string             `json:"closedAt"`
	Restaurant  *historyRestaurant `json:"restaurant"`
	Provider    string             `json:"provider"`
	Host        domain.UserInfo    `json:"host"`
	IsHost      bool               `json:"isHost"`
	MemberCount int                `json:"memberCount"`
	Items       []historyItem      `json:"items"`
	Subtotal    int                `json:"subtotal"`
	SharedFees  int                `json:"sharedFees"`
	Total       int                `json:"total"`
	GrandTotal  int                `json:"grandTotal"`
	Payer       *domain.UserInfo   `json:"payer"`
	Payment     *historyPayment    `json:"payment"`
}

// ------------------------------------------------------------ batch load

// partyBundle holds every record needed to summarise a set of parties,
// loaded with a constant number of queries (no N+1).
type partyBundle struct {
	parties  []*core.Record // in the requested order
	rests    map[string]*core.Record
	members  map[string][]*core.Record // by party, host first
	items    map[string][]*core.Record // by party, creation order
	users    map[string]*core.Record
	payments map[string]*core.Record // my payment (debtor) by party
}

const bundleChunk = 200

func toAny(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

func chunks(ids []string, n int) [][]string {
	var out [][]string
	for len(ids) > n {
		out = append(out, ids[:n])
		ids = ids[n:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}

func loadBundle(app core.App, ids []string, me string) (*partyBundle, error) {
	b := &partyBundle{
		rests: map[string]*core.Record{}, members: map[string][]*core.Record{},
		items: map[string][]*core.Record{}, users: map[string]*core.Record{}, payments: map[string]*core.Record{},
	}
	byID := map[string]*core.Record{}
	restIDs, userIDs := []string{}, []string{}
	seenRest, seenUser := map[string]bool{}, map[string]bool{}
	addUser := func(id string) {
		if id != "" && !seenUser[id] {
			seenUser[id] = true
			userIDs = append(userIDs, id)
		}
	}

	for _, part := range chunks(ids, bundleChunk) {
		recs, err := app.FindRecordsByIds(colParties, part)
		if err != nil {
			return nil, err
		}
		for _, p := range recs {
			byID[p.Id] = p
			if rid := p.GetString("restaurant"); rid != "" && !seenRest[rid] {
				seenRest[rid] = true
				restIDs = append(restIDs, rid)
			}
			addUser(p.GetString("host"))
			addUser(p.GetString("payer"))
		}
		pms, err := app.FindAllRecords(colPartyMembers, dbx.In("party", toAny(part)...))
		if err != nil {
			return nil, err
		}
		for _, pm := range pms {
			pid := pm.GetString("party")
			b.members[pid] = append(b.members[pid], pm)
			addUser(pm.GetString("user"))
		}
		its, err := app.FindAllRecords(colOrderItems, dbx.In("party", toAny(part)...))
		if err != nil {
			return nil, err
		}
		for _, it := range its {
			pid := it.GetString("party")
			b.items[pid] = append(b.items[pid], it)
		}
		pays, err := app.FindAllRecords(colPayments, dbx.In("party", toAny(part)...), dbx.HashExp{"debtor": me})
		if err != nil {
			return nil, err
		}
		for _, pay := range pays {
			b.payments[pay.GetString("party")] = pay
		}
	}
	for _, part := range chunks(restIDs, bundleChunk) {
		recs, err := app.FindRecordsByIds(colRestaurants, part)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			b.rests[r.Id] = r
		}
	}
	for _, part := range chunks(userIDs, bundleChunk) {
		recs, err := app.FindRecordsByIds(colUsers, part)
		if err != nil {
			return nil, err
		}
		for _, u := range recs {
			b.users[u.Id] = u
		}
	}
	for pid := range b.members {
		sortMembers(b.members[pid])
	}
	for pid := range b.items {
		sortByCreated(b.items[pid])
	}
	for _, id := range ids {
		if p, found := byID[id]; found {
			b.parties = append(b.parties, p)
		}
	}
	return b, nil
}

func (b *partyBundle) userInfo(id string) domain.UserInfo {
	if u, found := b.users[id]; found {
		return userInfo(u)
	}
	return domain.UserInfo{ID: id, Name: "Membre"}
}

// entry builds my view of one party (totals from the shared summary logic).
func (b *partyBundle) entry(p *core.Record, me string) historyEntry {
	rest := b.rests[p.GetString("restaurant")]
	s := summaryFromRecords(p, rest, b.members[p.Id], b.items[p.Id], b.users)
	mine, _ := s.ParticipantByID(me)

	e := historyEntry{
		ID: p.Id, Code: p.GetString("code"), Title: p.GetString("title"), Status: p.GetString("status"),
		Created: p.GetDateTime("created").String(), Provider: p.GetString("provider"),
		Host: b.userInfo(p.GetString("host")), IsHost: p.GetString("host") == me,
		MemberCount: len(b.members[p.Id]), Items: []historyItem{},
		Subtotal: mine.Subtotal, SharedFees: mine.SharedFees, Total: mine.Total, GrandTotal: s.GrandTotal,
	}
	if ca := p.GetDateTime("closed_at"); !ca.IsZero() {
		e.ClosedAt = ca.String()
	}
	if rest != nil {
		e.Restaurant = &historyRestaurant{
			ID: rest.Id, Name: rest.GetString("name"), Emoji: rest.GetString("emoji"),
			Cover: rest.GetString("cover"), CoverURL: rest.GetString("cover_url"), Active: rest.GetBool("active"),
		}
	}
	for _, it := range b.items[p.Id] {
		if it.GetString("user") != me {
			continue
		}
		e.Items = append(e.Items, historyItem{
			MenuItem: it.GetString("menu_item"), Name: it.GetString("name"), OptionsLabel: it.GetString("options_label"),
			Note: it.GetString("note"), Quantity: it.GetInt("quantity"), UnitPrice: it.GetInt("unit_price"), Total: it.GetInt("total"),
		})
	}
	if payer := p.GetString("payer"); payer != "" {
		ui := b.userInfo(payer)
		e.Payer = &ui
	}
	if pay, found := b.payments[p.Id]; found {
		e.Payment = &historyPayment{ID: pay.Id, Method: pay.GetString("method"), Status: pay.GetString("status"), Amount: pay.GetInt("amount")}
	}
	return e
}

// myPartyIDs lists the parties I belong to, newest first (party_members is
// maintained by the server together with parties.members).
func myPartyIDs(app core.App, me string, limit, offset int) ([]string, error) {
	var rows []struct {
		ID string `db:"id"`
	}
	q := "SELECT p.id AS id FROM parties p INNER JOIN party_members pm ON pm.party = p.id WHERE pm.user = {:u} ORDER BY p.created DESC, p.id DESC"
	params := dbx.Params{"u": me}
	if limit > 0 {
		q += " LIMIT {:l} OFFSET {:o}"
		params["l"], params["o"] = limit, offset
	}
	if err := app.DB().NewQuery(q).Bind(params).All(&rows); err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids, nil
}

func queryInt(e *core.RequestEvent, key string, def, lo, hi int) int {
	n, err := strconv.Atoi(strings.TrimSpace(e.Request.URL.Query().Get(key)))
	if err != nil {
		return def
	}
	return min(max(n, lo), hi)
}

// --------------------------------------------------------------- handlers

// meHistory: GET /api/occ/me/history?page=&perPage= — my parties, newest first.
func (h *handlers) meHistory(e *core.RequestEvent) error {
	me := e.Auth.Id
	page := queryInt(e, "page", 1, 1, 100000)
	perPage := queryInt(e, "perPage", 10, 1, 50)

	var total int
	err := e.App.DB().NewQuery("SELECT COUNT(*) FROM parties p INNER JOIN party_members pm ON pm.party = p.id WHERE pm.user = {:u}").
		Bind(dbx.Params{"u": me}).Row(&total)
	if err != nil {
		return err
	}
	ids, err := myPartyIDs(e.App, me, perPage, (page-1)*perPage)
	if err != nil {
		return err
	}
	b, err := loadBundle(e.App, ids, me)
	if err != nil {
		return err
	}
	items := make([]historyEntry, 0, len(b.parties))
	for _, p := range b.parties {
		items = append(items, b.entry(p, me))
	}
	return ok(e, map[string]any{
		"page": page, "perPage": perPage, "totalItems": total,
		"totalPages": (total + perPage - 1) / perPage, "items": items,
	})
}

// meStats: GET /api/occ/me/stats — orders placed, total spent, favourites.
func (h *handlers) meStats(e *core.RequestEvent) error {
	me := e.Auth.Id
	ids, err := myPartyIDs(e.App, me, 0, 0)
	if err != nil {
		return err
	}
	b, err := loadBundle(e.App, ids, me)
	if err != nil {
		return err
	}
	orders := make([]domain.HistoryOrder, 0, len(b.parties))
	for _, p := range b.parties {
		en := b.entry(p, me)
		o := domain.HistoryOrder{Status: en.Status, Total: en.Total}
		if en.Restaurant != nil {
			o.RestaurantID, o.RestaurantName, o.RestaurantEmoji = en.Restaurant.ID, en.Restaurant.Name, en.Restaurant.Emoji
		}
		for _, it := range en.Items {
			o.Lines = append(o.Lines, domain.HistoryLine{Name: it.Name, Quantity: it.Quantity})
		}
		orders = append(orders, o)
	}
	return ok(e, domain.ComputeHistoryStats(orders))
}

// ------------------------------------------------------------ reorder

type reorderLine struct {
	MenuItem     string `json:"menuItem"`
	Name         string `json:"name"`
	OptionsLabel string `json:"optionsLabel"`
	Note         string `json:"note"`
	Quantity     int    `json:"quantity"`
	UnitPrice    int    `json:"unitPrice"`
	Available    bool   `json:"available"`
	Reason       string `json:"reason,omitempty"`

	selected []domain.SelectedOption
	priced   domain.PricedOptions
}

type reorderSource struct {
	PartyID string `json:"partyId"`
	Title   string `json:"title"`
	Created string `json:"created"`
}

// lastOrderAt finds my most recent order lines at the party restaurant, in
// another (non cancelled) party, re-validated against the current menu.
func lastOrderAt(app core.App, party *core.Record, me string) (*reorderSource, []reorderLine, error) {
	restID := party.GetString("restaurant")
	if restID == "" {
		return nil, nil, nil
	}
	var row struct {
		ID string `db:"id"`
	}
	err := app.DB().NewQuery(`SELECT p.id AS id FROM parties p
		WHERE p.restaurant = {:r} AND p.id != {:cur} AND p.status != 'cancelled'
		AND EXISTS (SELECT 1 FROM order_items oi WHERE oi.party = p.id AND oi.user = {:u})
		ORDER BY p.created DESC, p.id DESC LIMIT 1`).
		Bind(dbx.Params{"r": restID, "cur": party.Id, "u": me}).One(&row)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && row.ID == "") {
		return nil, nil, nil // no previous order here
	}
	if err != nil {
		return nil, nil, err
	}
	src, err := app.FindRecordById(colParties, row.ID)
	if err != nil {
		return nil, nil, err
	}
	recs, err := app.FindAllRecords(colOrderItems, dbx.HashExp{"party": src.Id, "user": me})
	if err != nil {
		return nil, nil, err
	}
	sortByCreated(recs)

	menuIDs := make([]string, 0, len(recs))
	for _, r := range recs {
		menuIDs = append(menuIDs, r.GetString("menu_item"))
	}
	menu := map[string]*core.Record{}
	if mis, err := app.FindRecordsByIds(colMenuItems, menuIDs); err == nil {
		for _, m := range mis {
			menu[m.Id] = m
		}
	}

	lines := make([]reorderLine, 0, len(recs))
	for _, r := range recs {
		var sel []domain.SelectedOption
		_ = r.UnmarshalJSONField("selected_options", &sel)
		l := reorderLine{
			MenuItem: r.GetString("menu_item"), Name: r.GetString("name"), OptionsLabel: r.GetString("options_label"),
			Note: r.GetString("note"), Quantity: min(max(r.GetInt("quantity"), 1), 20), selected: sel,
		}
		m, found := menu[l.MenuItem]
		switch {
		case !found || m.GetString("restaurant") != restID:
			l.Reason = "n'est plus à la carte"
		case !m.GetBool("available"):
			l.Reason = "n'est plus disponible"
		default:
			var groups []domain.OptionGroup
			_ = m.UnmarshalJSONField("option_groups", &groups)
			priced, err := domain.PriceOptions(m.GetInt("price"), groups, sel)
			if err != nil {
				l.Reason = "ses options ont changé"
				break
			}
			l.Available, l.priced = true, priced
			l.Name, l.OptionsLabel, l.UnitPrice = m.GetString("name"), priced.Label, priced.UnitPrice
		}
		lines = append(lines, l)
	}
	return &reorderSource{PartyID: src.Id, Title: src.GetString("title"), Created: src.GetDateTime("created").String()}, lines, nil
}

// reorderPreview: GET /api/occ/parties/{id}/reorder — my last order at this
// restaurant (source null when there is none).
func (h *handlers) reorderPreview(e *core.RequestEvent) error {
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	src, lines, err := lastOrderAt(e.App, party, e.Auth.Id)
	if err != nil {
		return err
	}
	if lines == nil {
		lines = []reorderLine{}
	}
	return ok(e, map[string]any{"source": src, "items": lines})
}

// reorderApply: POST /api/occ/parties/{id}/reorder — adds my last order at
// this restaurant to my cart (prices recomputed, unavailable lines skipped).
func (h *handlers) reorderApply(e *core.RequestEvent) error {
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	me := e.Auth.Id
	type added struct {
		Name     string `json:"name"`
		Quantity int    `json:"quantity"`
	}
	type skipped struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	out := struct {
		Added   []added   `json:"added"`
		Skipped []skipped `json:"skipped"`
	}{Added: []added{}, Skipped: []skipped{}}

	err = e.App.RunInTransaction(func(tx core.App) error {
		p, err := tx.FindRecordById(colParties, party.Id)
		if err != nil {
			return err
		}
		if p.GetString("status") != domain.StatusOrdering {
			return badRequest("La commande n'accepte plus de modifications du panier.")
		}
		src, lines, err := lastOrderAt(tx, p, me)
		if err != nil {
			return err
		}
		if src == nil {
			return notFound("Aucune commande précédente dans ce restaurant.")
		}
		col, err := tx.FindCollectionByNameOrId(colOrderItems)
		if err != nil {
			return err
		}
		for _, l := range lines {
			if !l.Available {
				out.Skipped = append(out.Skipped, skipped{Name: l.Name, Reason: l.Reason})
				continue
			}
			sel := l.priced.Selected
			if sel == nil {
				sel = []domain.SelectedOption{}
			}
			rec := core.NewRecord(col)
			rec.Load(map[string]any{
				"party": p.Id, "user": me, "menu_item": l.MenuItem, "quantity": l.Quantity,
				"selected_options": sel, "note": l.Note, "name": l.Name, "options_label": l.priced.Label,
				"unit_price": l.priced.UnitPrice, "total": l.priced.UnitPrice * l.Quantity,
			})
			if err := tx.Save(rec); err != nil {
				return err
			}
			out.Added = append(out.Added, added{Name: l.Name, Quantity: l.Quantity})
		}
		if len(out.Added) > 0 {
			return resetReady(tx, p.Id, me)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return ok(e, out)
}
