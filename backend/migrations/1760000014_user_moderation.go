package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// User moderation and account security:
//   - users.banned / banned_reason / banned_at: suspension by an admin;
//   - users.deleted_at: account anonymised (« Compte supprimé »);
//   - users.password_set: false for accounts created with Google (random
//     password) until the owner chooses one (reset link / profile);
//   - all five are hidden (never in API responses, ignored in client writes);
//   - users.deleteRule = nil: no hard delete through the collection API (it
//     would break the history of the parties); deletion = anonymisation via
//     /api/occ/me/delete and /api/occ/admin/users/{id}.
//
// Existing accounts: password_set = true unless they are linked to an OAuth2
// provider (we cannot know whether that password was ever chosen).

func init() {
	m.Register(upUserModeration, downUserModeration)
}

// UserModerationFields are the hidden fields added to users.
var UserModerationFields = []string{"banned", "banned_reason", "banned_at", "deleted_at", "password_set"}

func upUserModeration(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	add := func(f core.Field) {
		if users.Fields.GetByName(f.GetName()) == nil {
			users.Fields.Add(f)
		}
	}
	add(&core.BoolField{Name: "banned", Hidden: true})
	add(&core.TextField{Name: "banned_reason", Max: 300, Hidden: true})
	add(&core.DateField{Name: "banned_at", Hidden: true})
	add(&core.DateField{Name: "deleted_at", Hidden: true})
	add(&core.BoolField{Name: "password_set", Hidden: true})
	users.DeleteRule = nil
	if err := app.Save(users); err != nil {
		return err
	}
	_, err = app.DB().NewQuery(`UPDATE users SET password_set = (NOT EXISTS (
		SELECT 1 FROM _externalAuths ea WHERE ea.recordRef = users.id AND ea.collectionRef = {:col}))`).
		Bind(map[string]any{"col": users.Id}).Execute()
	return err
}

func downUserModeration(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	for _, f := range UserModerationFields {
		users.Fields.RemoveByName(f)
	}
	users.DeleteRule = ptr(`id = @request.auth.id`)
	return app.Save(users)
}
