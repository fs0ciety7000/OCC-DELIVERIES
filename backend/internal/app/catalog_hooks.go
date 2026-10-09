package app

import (
	"slices"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

const colMenuCategories = "menu_categories"

// bindCatalogHooks validates the restaurant / menu writes made through the
// collections API by admins (the rules only check the role).
func bindCatalogHooks(app core.App) {
	app.OnRecordCreateRequest(colRestaurants).BindFunc(onRestaurantUpsert)
	app.OnRecordUpdateRequest(colRestaurants).BindFunc(onRestaurantUpsert)
	app.OnRecordDeleteRequest(colRestaurants).BindFunc(onRestaurantDelete)

	app.OnRecordCreateRequest(colMenuCategories).BindFunc(onCategoryUpsert)
	app.OnRecordUpdateRequest(colMenuCategories).BindFunc(onCategoryUpsert)

	app.OnRecordCreateRequest(colMenuItems).BindFunc(onMenuItemUpsert)
	app.OnRecordUpdateRequest(colMenuItems).BindFunc(onMenuItemUpsert)
}

// cleanStrings normalizes a JSON string list field (trim, lower, dedupe).
func cleanStrings(r *core.Record, field string) error {
	var list []string
	if raw := strings.TrimSpace(r.GetString(field)); raw != "" && raw != "null" {
		if err := r.UnmarshalJSONField(field, &list); err != nil {
			return badRequest("Le champ « " + field + " » doit être une liste de textes.")
		}
	}
	r.Set(field, domain.SplitList(strings.Join(list, "|")))
	return nil
}

// autoLock sets "locked" when an admin edits a record through the API, so
// that the automatic synchronisation never overwrites the edit. A request
// that sets "locked" itself (the lock toggle) wins; a request touching only
// the neutral fields (visibility, order…) does not lock.
func autoLock(e *core.RecordRequestEvent, neutral ...string) {
	info, err := e.RequestInfo()
	if err != nil {
		return
	}
	if _, explicit := info.Body["locked"]; explicit {
		return
	}
	for k := range info.Body {
		if !slices.Contains(neutral, k) {
			e.Record.Set("locked", true)
			return
		}
	}
}

func onRestaurantUpsert(e *core.RecordRequestEvent) error {
	r := e.Record
	if !r.IsNew() {
		autoLock(e, "active", "locked")
	}
	r.Set("slug", strings.ToLower(strings.TrimSpace(r.GetString("slug"))))
	r.Set("name", strings.TrimSpace(r.GetString("name")))
	if err := cleanStrings(r, "cuisines"); err != nil {
		return err
	}
	var links []providers.Link
	if raw := strings.TrimSpace(r.GetString("providers")); raw != "" && raw != "null" {
		if err := r.UnmarshalJSONField("providers", &links); err != nil {
			return badRequest("Liens fournisseurs invalides.")
		}
	}
	clean := []providers.Link{}
	for _, l := range links {
		l.URL = strings.TrimSpace(l.URL)
		if l.URL == "" {
			continue
		}
		if !providers.IsPlatform(l.ID) {
			return badRequest("Fournisseur inconnu : « " + l.ID + " ».")
		}
		if !strings.HasPrefix(l.URL, "https://") && !strings.HasPrefix(l.URL, "http://") {
			return badRequest("Le lien " + l.ID + " doit commencer par https://.")
		}
		clean = append(clean, l)
	}
	r.Set("providers", clean)
	if r.GetInt("eta_min") > 0 && r.GetInt("eta_max") > 0 && r.GetInt("eta_min") > r.GetInt("eta_max") {
		return badRequest("Le délai minimum dépasse le délai maximum.")
	}
	// unique slug with a readable message (the index would answer a generic error)
	if slug := r.GetString("slug"); slug != "" {
		if other, err := e.App.FindFirstRecordByData(colRestaurants, "slug", slug); err == nil && other.Id != r.Id {
			return badRequest("Ce slug est déjà utilisé par « " + other.GetString("name") + " ».")
		}
	}
	return e.Next()
}

func onRestaurantDelete(e *core.RecordRequestEvent) error {
	n, err := e.App.CountRecords(colParties, dbx.Or(dbx.HashExp{"restaurant": e.Record.Id}, dbx.Like("candidates", e.Record.Id)))
	if err != nil {
		return err
	}
	if n > 0 {
		return badRequest("Ce restaurant apparaît dans des commandes : désactive-le plutôt que de le supprimer.")
	}
	return e.Next()
}

func onCategoryUpsert(e *core.RecordRequestEvent) error {
	r := e.Record
	r.Set("name", strings.TrimSpace(r.GetString("name")))
	if !r.IsNew() && r.GetString("restaurant") != r.Original().GetString("restaurant") {
		return badRequest("Une catégorie ne peut pas changer de restaurant.")
	}
	return e.Next()
}

func onMenuItemUpsert(e *core.RecordRequestEvent) error {
	r := e.Record
	// an item added or edited by hand is never touched by the synchronisation
	autoLock(e, "locked", "position")
	r.Set("name", strings.TrimSpace(r.GetString("name")))
	if !r.IsNew() && r.GetString("restaurant") != r.Original().GetString("restaurant") {
		return badRequest("Un article ne peut pas changer de restaurant.")
	}
	if cat := r.GetString("category"); cat != "" {
		c, err := e.App.FindRecordById(colMenuCategories, cat)
		if err != nil || c.GetString("restaurant") != r.GetString("restaurant") {
			return badRequest("La catégorie n'appartient pas à ce restaurant.")
		}
	}
	if err := cleanStrings(r, "tags"); err != nil {
		return err
	}
	var groups []domain.OptionGroup
	if raw := strings.TrimSpace(r.GetString("option_groups")); raw != "" && raw != "null" {
		if err := r.UnmarshalJSONField("option_groups", &groups); err != nil {
			return badRequest("Options invalides.")
		}
	}
	if err := domain.ValidateOptionGroups(groups); err != nil {
		return toAPIError(err)
	}
	if groups == nil {
		groups = []domain.OptionGroup{}
	}
	r.Set("option_groups", groups)
	return e.Next()
}
