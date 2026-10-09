package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Web Push notifications, automatic deadlines and offline idempotency:
//   - push_subscriptions: one row per browser (PushSubscription), owner only;
//     written by /api/occ/push/subscribe (create/update rules nil);
//   - server_secrets: superuser-only key/value store (auto-generated VAPID
//     key pair when OCC_VAPID_* are not set — must survive restarts);
//   - users.notify_prefs (hidden json): { party, payments, reminders } on/off,
//     read and written through /api/occ/push/prefs;
//   - parties.auto_close_disabled: the host turns automatic closing off;
//   - parties.auto_events (json, server): what the scheduler did
//     (« Vote clôturé automatiquement à 11:45 ») — shown in the party;
//   - parties.auto_state (hidden json, server): reminder / extension flags so
//     that a restart never sends a reminder twice;
//   - order_items.client_key / votes.client_key: client idempotency key (the
//     offline outbox replays an action → the server returns the existing row).
//
// Open parties that already had deadlines keep the old « indicative »
// behaviour (auto_close_disabled = true): only new parties close by themselves.

func init() {
	m.Register(upPushDeadlines, downPushDeadlines)
}

// Collections created by this migration.
const (
	PushSubscriptionsCollection = "push_subscriptions"
	ServerSecretsCollection     = "server_secrets"
)

func upPushDeadlines(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	if users.Fields.GetByName("notify_prefs") == nil {
		users.Fields.Add(&core.JSONField{Name: "notify_prefs", MaxSize: 1 << 10, Hidden: true})
		if err := app.Save(users); err != nil {
			return err
		}
	}

	// ------------------------------------------------- push_subscriptions
	if _, err := app.FindCollectionByNameOrId(PushSubscriptionsCollection); err != nil {
		owner := `user = @request.auth.id`
		ps := core.NewBaseCollection(PushSubscriptionsCollection)
		ps.ListRule = ptr(owner)
		ps.ViewRule = ptr(owner)
		ps.DeleteRule = ptr(owner)
		ps.Fields.Add(
			&core.RelationField{Name: "user", CollectionId: users.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
			&core.TextField{Name: "endpoint", Required: true, Max: 1000},
			&core.TextField{Name: "p256dh", Required: true, Max: 200},
			&core.TextField{Name: "auth", Required: true, Max: 100},
			&core.TextField{Name: "user_agent", Max: 300},
			&core.DateField{Name: "last_ok"},
			intField("failures", fptr(0), false),
		)
		autodates(ps)
		ps.AddIndex("idx_push_subscriptions_endpoint", true, "endpoint", "")
		ps.AddIndex("idx_push_subscriptions_user", false, "user", "")
		if err := app.Save(ps); err != nil {
			return err
		}
	}

	// ----------------------------------------------------- server_secrets
	if _, err := app.FindCollectionByNameOrId(ServerSecretsCollection); err != nil {
		ss := core.NewBaseCollection(ServerSecretsCollection) // all rules nil: superusers only
		ss.Fields.Add(
			&core.TextField{Name: "name", Required: true, Max: 100},
			&core.TextField{Name: "value", Max: 5000, Hidden: true},
		)
		autodates(ss)
		ss.AddIndex("idx_server_secrets_name", true, "name", "")
		if err := app.Save(ss); err != nil {
			return err
		}
	}

	// ------------------------------------------------------------ parties
	parties, err := app.FindCollectionByNameOrId("parties")
	if err != nil {
		return err
	}
	if parties.Fields.GetByName("auto_close_disabled") == nil {
		parties.Fields.Add(
			&core.BoolField{Name: "auto_close_disabled"},
			&core.JSONField{Name: "auto_events", MaxSize: 16 << 10},
			&core.JSONField{Name: "auto_state", MaxSize: 2 << 10, Hidden: true},
		)
		if err := app.Save(parties); err != nil {
			return err
		}
		if _, err := app.DB().NewQuery(`UPDATE parties SET auto_close_disabled = TRUE
			WHERE status IN ('lobby', 'voting', 'ordering', 'review')`).Execute(); err != nil {
			return err
		}
	}

	// ------------------------------------------- order_items / votes keys
	for _, name := range []string{"order_items", "votes"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}
		if col.Fields.GetByName("client_key") != nil {
			continue
		}
		col.Fields.Add(&core.TextField{Name: "client_key", Max: 64, Pattern: `^[A-Za-z0-9_-]*$`})
		col.AddIndex("idx_"+name+"_client_key", true, "user, client_key", "client_key != ''")
		if err := app.Save(col); err != nil {
			return err
		}
	}
	return nil
}

func downPushDeadlines(app core.App) error {
	for _, name := range []string{PushSubscriptionsCollection, ServerSecretsCollection} {
		if col, err := app.FindCollectionByNameOrId(name); err == nil {
			if err := app.Delete(col); err != nil {
				return err
			}
		}
	}
	remove := func(colName string, fields ...string) error {
		col, err := app.FindCollectionByNameOrId(colName)
		if err != nil {
			return err
		}
		for _, f := range fields {
			col.Fields.RemoveByName(f)
		}
		col.RemoveIndex("idx_" + colName + "_client_key")
		return app.Save(col)
	}
	if err := remove("users", "notify_prefs"); err != nil {
		return err
	}
	if err := remove("parties", "auto_close_disabled", "auto_events", "auto_state"); err != nil {
		return err
	}
	if err := remove("order_items", "client_key"); err != nil {
		return err
	}
	return remove("votes", "client_key")
}
