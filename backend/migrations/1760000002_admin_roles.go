package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Roles of the in-app admin panel (/admin). Superusers (/_/) keep full access.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// isAdminRule is appended to the read rules so that admins see everything.
const isAdminRule = `@request.auth.role = "admin"`

func init() {
	m.Register(upAdminRoles, downAdminRoles)
}

func upAdminRoles(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	if users.Fields.GetByName("role") == nil {
		users.Fields.Add(&core.SelectField{Name: "role", MaxSelect: 1, Values: []string{RoleUser, RoleAdmin}})
	}
	if err := app.Save(users); err != nil {
		return err
	}
	if _, err := app.DB().Update("users", dbx.Params{"role": RoleUser}, dbx.Or(dbx.HashExp{"role": ""}, dbx.NewExp("role IS NULL"))).Execute(); err != nil {
		return err
	}

	rules := map[string]struct{ list, write string }{
		"restaurants":     {list: `active = true || ` + isAdminRule, write: isAdminRule},
		"menu_categories": {write: isAdminRule},
		"menu_items":      {write: isAdminRule},
	}
	for name, r := range rules {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}
		if r.list != "" {
			c.ListRule = ptr(r.list)
			c.ViewRule = ptr(r.list)
		}
		c.CreateRule = ptr(r.write)
		c.UpdateRule = ptr(r.write)
		c.DeleteRule = ptr(r.write)
		if err := app.Save(c); err != nil {
			return err
		}
	}

	read := map[string]string{
		"parties":       `members.id ?= @request.auth.id || ` + isAdminRule,
		"party_members": `party.members.id ?= @request.auth.id || ` + isAdminRule,
		"votes":         `party.members.id ?= @request.auth.id || ` + isAdminRule,
		"order_items":   `party.members.id ?= @request.auth.id || ` + isAdminRule,
		"payments":      `party.members.id ?= @request.auth.id || ` + isAdminRule,
	}
	for name, rule := range read {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}
		c.ListRule = ptr(rule)
		c.ViewRule = ptr(rule)
		if err := app.Save(c); err != nil {
			return err
		}
	}
	return nil
}

func downAdminRoles(app core.App) error {
	restore := map[string]struct{ list, view *string }{
		"restaurants":     {ptr(`active = true`), ptr(`active = true`)},
		"menu_categories": {ptr(""), ptr("")},
		"menu_items":      {ptr(""), ptr("")},
		"parties":         {ptr(`members.id ?= @request.auth.id`), ptr(`members.id ?= @request.auth.id`)},
		"party_members":   {ptr(`party.members.id ?= @request.auth.id`), ptr(`party.members.id ?= @request.auth.id`)},
		"votes":           {ptr(`party.members.id ?= @request.auth.id`), ptr(`party.members.id ?= @request.auth.id`)},
		"order_items":     {ptr(`party.members.id ?= @request.auth.id`), ptr(`party.members.id ?= @request.auth.id`)},
		"payments":        {ptr(`party.members.id ?= @request.auth.id`), ptr(`party.members.id ?= @request.auth.id`)},
	}
	for name, r := range restore {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		c.ListRule, c.ViewRule = r.list, r.view
		if name == "restaurants" || name == "menu_categories" || name == "menu_items" {
			c.CreateRule, c.UpdateRule, c.DeleteRule = nil, nil, nil
		}
		if err := app.Save(c); err != nil {
			return err
		}
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	users.Fields.RemoveByName("role")
	return app.Save(users)
}
