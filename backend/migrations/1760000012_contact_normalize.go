package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Restaurant contact details: restaurants.enriched_from (attribution of the
// fields filled from OpenStreetMap, ODbL) and a one-shot normalization of
// the stored phones (E.164) and addresses (« Rue X 12, 7000 Mons »).
// Idempotent; a phone the normalizer cannot read is left as is (never
// deleted). Locked restaurants are normalized too: only the shape changes.

func init() {
	m.Register(upContactNormalize, downContactNormalize)
}

func upContactNormalize(app core.App) error {
	rest, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		return err
	}
	if rest.Fields.GetByName("enriched_from") == nil {
		rest.Fields.Add(&core.JSONField{Name: "enriched_from", MaxSize: 4 << 10})
		if err := app.Save(rest); err != nil {
			return err
		}
	}
	return NormalizeRestaurantContacts(app)
}

// NormalizeRestaurantContacts rewrites every stored phone / address in its
// canonical form (direct update: no hook, `updated` untouched).
func NormalizeRestaurantContacts(app core.App) error {
	var rows []struct {
		ID      string `db:"id"`
		Phone   string `db:"phone"`
		Address string `db:"address"`
	}
	if err := app.DB().Select("id", "phone", "address").From("restaurants").All(&rows); err != nil {
		return err
	}
	for _, r := range rows {
		phone := r.Phone
		if p, ok := domain.NormalizeRestaurantPhone(r.Phone); ok {
			phone = p
		}
		addr := domain.NormalizeAddress(r.Address)
		if phone == r.Phone && addr == r.Address {
			continue
		}
		if _, err := app.DB().Update("restaurants", dbx.Params{"phone": phone, "address": addr}, dbx.HashExp{"id": r.ID}).Execute(); err != nil {
			return err
		}
	}
	return nil
}

func downContactNormalize(app core.App) error {
	rest, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		return err
	}
	if f := rest.Fields.GetByName("enriched_from"); f != nil {
		rest.Fields.RemoveById(f.GetId())
		return app.Save(rest)
	}
	return nil
}
