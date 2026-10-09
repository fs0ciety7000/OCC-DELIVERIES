package app

import (
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/enrich"
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
	app.OnRecordDeleteRequest(colMenuItems).BindFunc(onMenuItemDelete)
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

func onRestaurantUpsert(e *core.RecordRequestEvent) error {
	r := e.Record
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
	if err := normalizeContact(r); err != nil {
		return err
	}
	keepAttribution(r)
	// items_count is computed by the server (never trusted from the client)
	n := 0
	if !r.IsNew() {
		var err error
		if n, err = catalog.CountAvailableItems(e.App, r.Id); err != nil {
			return err
		}
	}
	r.Set(catalog.ItemsCountField, n)
	// unique slug with a readable message (the index would answer a generic error)
	if slug := r.GetString("slug"); slug != "" {
		if other, err := e.App.FindFirstRecordByData(colRestaurants, "slug", slug); err == nil && other.Id != r.Id {
			return badRequest("Ce slug est déjà utilisé par « " + other.GetString("name") + " ».")
		}
	}
	return e.Next()
}

// normalizeContact stores the phone in E.164 and the address as « Rue X 12,
// 7000 Mons ». A new or changed phone that cannot be read is refused; an
// unreadable phone already stored does not block other edits.
func normalizeContact(r *core.Record) error {
	raw := strings.TrimSpace(r.GetString("phone"))
	switch p, ok := domain.NormalizeRestaurantPhone(raw); {
	case raw == "":
		r.Set("phone", "")
	case ok:
		r.Set("phone", p)
	case r.IsNew() || raw != strings.TrimSpace(r.Original().GetString("phone")):
		return badRequest("Numéro de téléphone invalide (ex. 065 12 34 56 ou +32 65 12 34 56).")
	}
	r.Set("address", domain.NormalizeAddress(r.GetString("address")))
	return nil
}

// keepAttribution keeps restaurants.enriched_from server-side: never taken
// from the client, and a field the admin changes is no longer credited to
// OpenStreetMap.
func keepAttribution(r *core.Record) {
	if r.IsNew() {
		r.Set("enriched_from", nil)
		return
	}
	orig := r.Original()
	var a enrich.Attribution
	if raw := strings.TrimSpace(orig.GetString("enriched_from")); raw == "" || raw == "null" || orig.UnmarshalJSONField("enriched_from", &a) != nil || a.Provider == "" {
		r.Set("enriched_from", nil)
		return
	}
	changed := map[string]bool{
		enrich.FieldPhone:   r.GetString("phone") != orig.GetString("phone"),
		enrich.FieldAddress: r.GetString("address") != orig.GetString("address"),
		enrich.FieldGeo:     r.GetFloat("lat") != orig.GetFloat("lat") || r.GetFloat("lng") != orig.GetFloat("lng"),
	}
	kept := []string{}
	for _, f := range a.Fields {
		if !changed[f] {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 {
		r.Set("enriched_from", nil)
		return
	}
	a.Fields = kept
	r.Set("enriched_from", a)
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
	if err := e.Next(); err != nil {
		return err
	}
	return catalog.RefreshItemsCount(e.App, r.GetString("restaurant"))
}

func onMenuItemDelete(e *core.RecordRequestEvent) error {
	if err := e.Next(); err != nil {
		return err
	}
	return catalog.RefreshItemsCount(e.App, e.Record.GetString("restaurant"))
}
