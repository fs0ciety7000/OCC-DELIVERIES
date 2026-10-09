// Package migrations holds the Go migrations of OCC Deliveries (schema + demo seed).
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

var imageMimeTypes = []string{"image/png", "image/jpeg", "image/webp", "image/gif", "image/svg+xml"}

var walletQRMimeTypes = []string{"image/png", "image/jpeg", "image/webp"}

func ptr(s string) *string { return types.Pointer(s) }

func fptr(f float64) *float64 { return types.Pointer(f) }

func autodates(c *core.Collection) {
	c.Fields.Add(
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
}

func intField(name string, min *float64, required bool) *core.NumberField {
	return &core.NumberField{Name: name, OnlyInt: true, Min: min, Required: required}
}

func init() {
	m.Register(upSchema, downSchema)
}

// collection names, in creation order
var occCollections = []string{
	"restaurants", "menu_categories", "menu_items",
	"parties", "party_members", "votes", "order_items", "payments", "payout_profiles",
}

func upSchema(app core.App) error {
	// ---------------------------------------------------------------- users
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	if users.Fields.GetByName("color") == nil {
		users.Fields.Add(&core.TextField{Name: "color", Max: 7, Pattern: `^(#[0-9A-Fa-f]{6})?$`})
	}
	users.ListRule = ptr(`@request.auth.id != ""`)
	users.ViewRule = ptr(`@request.auth.id != ""`)
	users.UpdateRule = ptr(`id = @request.auth.id`)
	users.DeleteRule = ptr(`id = @request.auth.id`)
	if err := app.Save(users); err != nil {
		return err
	}

	// ---------------------------------------------------------- restaurants
	restaurants := core.NewBaseCollection("restaurants")
	restaurants.ListRule = ptr(`active = true`)
	restaurants.ViewRule = ptr(`active = true`)
	restaurants.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 120},
		&core.TextField{Name: "slug", Required: true, Max: 120, Pattern: `^[a-z0-9]+(?:-[a-z0-9]+)*$`},
		&core.TextField{Name: "description", Max: 2000},
		&core.TextField{Name: "emoji", Max: 16},
		&core.FileField{Name: "cover", MaxSelect: 1, MaxSize: 5 << 20, MimeTypes: imageMimeTypes},
		&core.URLField{Name: "cover_url"},
		&core.JSONField{Name: "cuisines", MaxSize: 4 << 10},
		&core.TextField{Name: "address", Max: 300},
		&core.NumberField{Name: "lat", Min: fptr(-90), Max: fptr(90)},
		&core.NumberField{Name: "lng", Min: fptr(-180), Max: fptr(180)},
		&core.TextField{Name: "phone", Max: 40},
		&core.NumberField{Name: "rating", Min: fptr(0), Max: fptr(5)},
		intField("rating_count", fptr(0), false),
		&core.NumberField{Name: "price_level", OnlyInt: true, Min: fptr(1), Max: fptr(4)},
		intField("eta_min", fptr(0), false),
		intField("eta_max", fptr(0), false),
		intField("delivery_fee", fptr(0), false),
		intField("min_order", fptr(0), false),
		&core.JSONField{Name: "providers", MaxSize: 8 << 10},
		&core.BoolField{Name: "active"},
	)
	autodates(restaurants)
	restaurants.AddIndex("idx_restaurants_slug", true, "slug", "")
	if err := app.Save(restaurants); err != nil {
		return err
	}

	// ------------------------------------------------------ menu_categories
	categories := core.NewBaseCollection("menu_categories")
	categories.ListRule = ptr("")
	categories.ViewRule = ptr("")
	categories.Fields.Add(
		&core.RelationField{Name: "restaurant", CollectionId: restaurants.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.TextField{Name: "name", Required: true, Max: 120},
		intField("position", nil, false),
	)
	autodates(categories)
	categories.AddIndex("idx_menu_categories_restaurant", false, "restaurant", "")
	if err := app.Save(categories); err != nil {
		return err
	}

	// ----------------------------------------------------------- menu_items
	items := core.NewBaseCollection("menu_items")
	items.ListRule = ptr("")
	items.ViewRule = ptr("")
	items.Fields.Add(
		&core.RelationField{Name: "restaurant", CollectionId: restaurants.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.RelationField{Name: "category", CollectionId: categories.Id, MaxSelect: 1},
		&core.TextField{Name: "name", Required: true, Max: 160},
		&core.TextField{Name: "description", Max: 2000},
		intField("price", fptr(0), true),
		&core.TextField{Name: "emoji", Max: 16},
		&core.FileField{Name: "image", MaxSelect: 1, MaxSize: 5 << 20, MimeTypes: imageMimeTypes, Thumbs: []string{"400x300"}},
		&core.JSONField{Name: "tags", MaxSize: 4 << 10},
		&core.JSONField{Name: "option_groups", MaxSize: 64 << 10},
		&core.BoolField{Name: "popular"},
		&core.BoolField{Name: "available"},
		intField("position", nil, false),
	)
	autodates(items)
	items.AddIndex("idx_menu_items_restaurant", false, "restaurant", "")
	if err := app.Save(items); err != nil {
		return err
	}

	// -------------------------------------------------------------- parties
	parties := core.NewBaseCollection("parties")
	parties.ListRule = ptr(`members.id ?= @request.auth.id`)
	parties.ViewRule = ptr(`members.id ?= @request.auth.id`)
	parties.CreateRule = ptr(`@request.auth.id != ""`)
	parties.UpdateRule = ptr(`host = @request.auth.id`)
	parties.DeleteRule = ptr(`host = @request.auth.id && status = "lobby"`)
	parties.Fields.Add(
		&core.TextField{Name: "code", Required: true, Min: 6, Max: 6, Pattern: `^[A-HJ-NP-Z2-9]{6}$`},
		&core.TextField{Name: "title", Max: 120},
		&core.RelationField{Name: "host", CollectionId: users.Id, MaxSelect: 1, Required: true},
		&core.RelationField{Name: "members", CollectionId: users.Id, MaxSelect: 999},
		&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"lobby", "voting", "ordering", "review", "paying", "closed", "cancelled"}},
		&core.RelationField{Name: "candidates", CollectionId: restaurants.Id, MaxSelect: 20},
		&core.RelationField{Name: "restaurant", CollectionId: restaurants.Id, MaxSelect: 1},
		&core.SelectField{Name: "provider", MaxSelect: 1, Values: []string{"ubereats", "takeaway", "manual"}},
		&core.TextField{Name: "delivery_address", Max: 300},
		&core.TextField{Name: "notes", Max: 2000},
		&core.DateField{Name: "voting_ends_at"},
		&core.DateField{Name: "ordering_ends_at"},
		&core.SelectField{Name: "split_mode", MaxSelect: 1, Values: []string{"equal", "proportional"}},
		intField("delivery_fee", fptr(0), false),
		intField("service_fee", fptr(0), false),
		intField("tip", fptr(0), false),
		&core.RelationField{Name: "payer", CollectionId: users.Id, MaxSelect: 1},
		&core.JSONField{Name: "dispatch", MaxSize: 4 << 10},
		&core.DateField{Name: "closed_at"},
	)
	autodates(parties)
	parties.AddIndex("idx_parties_code", true, "code", "")
	if err := app.Save(parties); err != nil {
		return err
	}

	membersOnly := `party.members.id ?= @request.auth.id`

	// -------------------------------------------------------- party_members
	pm := core.NewBaseCollection("party_members")
	pm.ListRule = ptr(membersOnly)
	pm.ViewRule = ptr(membersOnly)
	pm.Fields.Add(
		&core.RelationField{Name: "party", CollectionId: parties.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.RelationField{Name: "user", CollectionId: users.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.SelectField{Name: "role", Required: true, MaxSelect: 1, Values: []string{"host", "member"}},
		&core.BoolField{Name: "ready"},
	)
	autodates(pm)
	pm.AddIndex("idx_party_members_party_user", true, "party, user", "")
	if err := app.Save(pm); err != nil {
		return err
	}

	// ---------------------------------------------------------------- votes
	votes := core.NewBaseCollection("votes")
	votes.ListRule = ptr(membersOnly)
	votes.ViewRule = ptr(membersOnly)
	votes.CreateRule = ptr(`user = @request.auth.id && party.members.id ?= @request.auth.id && party.status = "voting"`)
	votes.DeleteRule = ptr(`user = @request.auth.id && party.status = "voting"`)
	votes.Fields.Add(
		&core.RelationField{Name: "party", CollectionId: parties.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.RelationField{Name: "user", CollectionId: users.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.RelationField{Name: "restaurant", CollectionId: restaurants.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
	)
	autodates(votes)
	votes.AddIndex("idx_votes_party_user_restaurant", true, "party, user, restaurant", "")
	if err := app.Save(votes); err != nil {
		return err
	}

	// ---------------------------------------------------------- order_items
	oi := core.NewBaseCollection("order_items")
	oi.ListRule = ptr(membersOnly)
	oi.ViewRule = ptr(membersOnly)
	oi.CreateRule = ptr(`user = @request.auth.id && party.members.id ?= @request.auth.id && party.status = "ordering"`)
	oi.UpdateRule = ptr(`user = @request.auth.id && party.status = "ordering"`)
	oi.DeleteRule = ptr(`user = @request.auth.id && party.status = "ordering"`)
	oi.Fields.Add(
		&core.RelationField{Name: "party", CollectionId: parties.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.RelationField{Name: "user", CollectionId: users.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		// not required at schema level so that re-importing a menu never blocks on
		// historical lines (the snapshot name/prices stay); the hook enforces it.
		&core.RelationField{Name: "menu_item", CollectionId: items.Id, MaxSelect: 1},
		&core.NumberField{Name: "quantity", OnlyInt: true, Min: fptr(1), Max: fptr(20), Required: true},
		&core.JSONField{Name: "selected_options", MaxSize: 8 << 10},
		&core.TextField{Name: "note", Max: 200},
		&core.TextField{Name: "name", Max: 160},
		&core.TextField{Name: "options_label", Max: 1000},
		intField("unit_price", fptr(0), false),
		intField("total", fptr(0), false),
	)
	autodates(oi)
	oi.AddIndex("idx_order_items_party", false, "party", "")
	if err := app.Save(oi); err != nil {
		return err
	}

	// ------------------------------------------------------------- payments
	payments := core.NewBaseCollection("payments")
	payments.ListRule = ptr(membersOnly)
	payments.ViewRule = ptr(membersOnly)
	payments.Fields.Add(
		&core.RelationField{Name: "party", CollectionId: parties.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.RelationField{Name: "debtor", CollectionId: users.Id, MaxSelect: 1, Required: true},
		&core.RelationField{Name: "creditor", CollectionId: users.Id, MaxSelect: 1, Required: true},
		intField("amount", fptr(0), false),
		&core.SelectField{Name: "method", MaxSelect: 1, Values: []string{"qr", "wero", "bancontact", "link", "cash", "later", "self"}},
		&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"pending", "declared", "confirmed"}},
		&core.TextField{Name: "reference", Max: 140},
		&core.DateField{Name: "declared_at"},
		&core.DateField{Name: "confirmed_at"},
	)
	autodates(payments)
	payments.AddIndex("idx_payments_party", false, "party", "")
	if err := app.Save(payments); err != nil {
		return err
	}

	// ------------------------------------------------------ payout_profiles
	owner := `user = @request.auth.id`
	pp := core.NewBaseCollection("payout_profiles")
	pp.ListRule = ptr(owner)
	pp.ViewRule = ptr(owner)
	pp.CreateRule = ptr(`@request.auth.id != "" && user = @request.auth.id`)
	pp.UpdateRule = ptr(owner)
	pp.DeleteRule = ptr(owner)
	pp.Fields.Add(
		&core.RelationField{Name: "user", CollectionId: users.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.TextField{Name: "holder_name", Max: 70},
		&core.TextField{Name: "iban", Max: 34},
		&core.TextField{Name: "bic", Max: 11},
		&core.URLField{Name: "payment_link"},
		&core.TextField{Name: "wero_id", Max: 254},
		&core.TextField{Name: "bancontact_phone", Max: 16},
		&core.FileField{Name: "wero_qr", MaxSelect: 1, MaxSize: 1 << 20, MimeTypes: walletQRMimeTypes, Protected: true},
		&core.FileField{Name: "bancontact_qr", MaxSelect: 1, MaxSize: 1 << 20, MimeTypes: walletQRMimeTypes, Protected: true},
	)
	autodates(pp)
	pp.AddIndex("idx_payout_profiles_user", true, "user", "")
	return app.Save(pp)
}

func downSchema(app core.App) error {
	for i := len(occCollections) - 1; i >= 0; i-- {
		c, err := app.FindCollectionByNameOrId(occCollections[i])
		if err != nil {
			continue
		}
		if err := app.Delete(c); err != nil {
			return err
		}
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	users.Fields.RemoveByName("color")
	users.ListRule = ptr(`id = @request.auth.id`)
	users.ViewRule = ptr(`id = @request.auth.id`)
	return app.Save(users)
}
