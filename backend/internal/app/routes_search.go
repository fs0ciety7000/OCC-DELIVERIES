package app

import (
	"strconv"

	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/search"
)

// search is GET /api/occ/search?q=&limit=&lat=&lng= — public for the
// restaurants and dishes (same visibility as /restaurants/nearby: active,
// incomplete menus hidden, available dishes only); people only for an
// authenticated user, and only the colleagues sharing a party (or a team)
// with them.
func (h *handlers) search(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	if len([]rune(q.Get("q"))) > 200 {
		return badRequest("Recherche trop longue (200 caractères au plus).")
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	p := search.Params{
		Query:    q.Get("q"),
		Limit:    limit,
		MinItems: minMenuItems(e.App),
	}
	if q.Get("lat") != "" && q.Get("lng") != "" {
		p.Lat = queryFloat(e, "lat", h.cfg.DefaultLat)
		p.Lng = queryFloat(e, "lng", h.cfg.DefaultLng)
		p.HasGeo = true
	}
	if e.Auth != nil && e.Auth.Collection().Name == colUsers {
		p.UserID = e.Auth.Id
	}
	p.Admin = isAdmin(e)
	res, err := search.Search(e.App, p)
	if err != nil {
		return err
	}
	return ok(e, res)
}
