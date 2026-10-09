package app

import (
	"math"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// colAppSettings is the singleton settings collection (migration 1760000010).
const colAppSettings = "app_settings"

// settingsRecord returns the settings row (nil when missing).
func settingsRecord(app core.App) (*core.Record, error) {
	recs, err := app.FindRecordsByFilter(colAppSettings, "", "created", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	return recs[0], nil
}

// minMenuItems is the threshold under which the public listings hide a
// restaurant (0 = disabled, also when the settings are unreadable).
func minMenuItems(app core.App) int {
	rec, err := settingsRecord(app)
	if err != nil || rec == nil {
		return 0
	}
	return rec.GetInt("min_menu_items")
}

func bindSettingsHooks(app core.App) {
	app.OnRecordUpdateRequest(colAppSettings).BindFunc(func(e *core.RecordRequestEvent) error {
		if err := checkMinMenuItems(e.Record.GetFloat("min_menu_items")); err != nil {
			return err
		}
		return e.Next()
	})
}

func checkMinMenuItems(v float64) error {
	if v != math.Trunc(v) {
		return badRequest("Le nombre minimum de plats doit être un nombre entier.")
	}
	return toAPIError(domain.ValidateMinMenuItems(int(v)))
}

// adminSettingsResponse is the body of GET / PATCH /api/occ/admin/settings.
type adminSettingsResponse struct {
	MinMenuItems int `json:"minMenuItems"`
	// HiddenRestaurants: active restaurants hidden from the public listings.
	HiddenRestaurants int `json:"hiddenRestaurants"`
	ActiveRestaurants int `json:"activeRestaurants"`
}

func settingsResponse(app core.App) (adminSettingsResponse, error) {
	out := adminSettingsResponse{MinMenuItems: minMenuItems(app)}
	n, err := app.CountRecords(colRestaurants, dbx.HashExp{"active": true})
	if err != nil {
		return out, err
	}
	out.ActiveRestaurants = int(n)
	if out.MinMenuItems > 0 {
		n, err := app.CountRecords(colRestaurants, dbx.HashExp{"active": true},
			dbx.NewExp("[["+catalog.ItemsCountField+"]] < {:min}", dbx.Params{"min": out.MinMenuItems}))
		if err != nil {
			return out, err
		}
		out.HiddenRestaurants = int(n)
	}
	return out, nil
}

func (h *handlers) adminSettings(e *core.RequestEvent) error {
	out, err := settingsResponse(e.App)
	if err != nil {
		return err
	}
	return ok(e, out)
}

func (h *handlers) adminSetSettings(e *core.RequestEvent) error {
	var body struct {
		MinMenuItems *float64 `json:"minMenuItems"`
	}
	if err := bindJSON(e, &body); err != nil {
		return err
	}
	if body.MinMenuItems == nil {
		return badRequest("Indique le nombre minimum de plats (0 pour tout afficher).")
	}
	if err := checkMinMenuItems(*body.MinMenuItems); err != nil {
		return err
	}
	rec, err := settingsRecord(e.App)
	if err != nil {
		return err
	}
	if rec == nil {
		col, err := e.App.FindCollectionByNameOrId(colAppSettings)
		if err != nil {
			return err
		}
		rec = core.NewRecord(col)
	}
	rec.Set("min_menu_items", int(*body.MinMenuItems))
	if err := e.App.Save(rec); err != nil {
		return err
	}
	out, err := settingsResponse(e.App)
	if err != nil {
		return err
	}
	return ok(e, out)
}
