package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Passkeys (WebAuthn): one row per credential registered from the profile.
// Every rule is nil (superusers only): the owner lists, renames and deletes
// them through /api/occ/passkeys, sign-in goes through /passkeys/login/*.
// Binary values are stored base64url (no padding); credential_id is unique
// (a credential belongs to a single account).

func init() {
	m.Register(upPasskeys, downPasskeys)
}

// PasskeysCollection is the collection created by this migration.
const PasskeysCollection = "passkeys"

func upPasskeys(app core.App) error {
	if _, err := app.FindCollectionByNameOrId(PasskeysCollection); err == nil {
		return nil
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	pk := core.NewBaseCollection(PasskeysCollection) // all rules nil
	pk.Fields.Add(
		&core.RelationField{Name: "user", CollectionId: users.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
		&core.TextField{Name: "name", Required: true, Max: 60},
		&core.TextField{Name: "credential_id", Required: true, Max: 1400},
		&core.TextField{Name: "public_key", Required: true, Max: 4000},
		intField("sign_count", fptr(0), false),
		&core.JSONField{Name: "transports", MaxSize: 1 << 10},
		&core.TextField{Name: "aaguid", Max: 36},
		&core.TextField{Name: "attestation_type", Max: 40},
		&core.TextField{Name: "attestation_format", Max: 40},
		&core.BoolField{Name: "user_verified"},
		&core.BoolField{Name: "backup_eligible"},
		&core.BoolField{Name: "backup_state"},
		&core.TextField{Name: "attachment", Max: 40},
		&core.DateField{Name: "last_used_at"},
	)
	autodates(pk)
	pk.AddIndex("idx_passkeys_credential_id", true, "credential_id", "")
	pk.AddIndex("idx_passkeys_user", false, "user", "")
	return app.Save(pk)
}

func downPasskeys(app core.App) error {
	col, err := app.FindCollectionByNameOrId(PasskeysCollection)
	if err != nil {
		return nil
	}
	return app.Delete(col)
}
