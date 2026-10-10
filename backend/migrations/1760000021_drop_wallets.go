package migrations

import (
	"path"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Wero and Bancontact Pay are no longer offered (ADR 0003, update 3): no
// third party can prefill an amount for them, the EPC transfer QR (read by
// the Belgian bank apps that also carry Wero) and the Revolut / PayPal.me
// links with the amount cover the need. The payout profile fields
// (wero_id, bancontact_phone — personal phone numbers — and the uploaded
// « receive » QR images) are dropped and the stored files deleted.
// payments.method keeps the "wero" / "bancontact" values so that old
// declarations stay valid and keep their label. Idempotent.

func init() {
	m.Register(upDropWallets, downDropWallets)
}

var (
	walletTextFields = []string{"wero_id", "bancontact_phone"}
	walletFileFields = []string{"wero_qr", "bancontact_qr"}
)

func upDropWallets(app core.App) error {
	pp, err := app.FindCollectionByNameOrId("payout_profiles")
	if err != nil {
		return err
	}

	// Files to delete once the schema change succeeded (collected first: the
	// columns are gone after the save).
	var keys []string
	for _, field := range walletFileFields {
		if pp.Fields.GetByName(field) == nil {
			continue
		}
		var rows []struct {
			ID   string `db:"id"`
			File string `db:"file"`
		}
		if err := app.DB().Select("id", field+" AS file").From(pp.Name).
			Where(dbx.NewExp("COALESCE([[" + field + "]], '') != ''")).All(&rows); err != nil {
			return err
		}
		for _, r := range rows {
			keys = append(keys, pp.BaseFilesPath()+"/"+r.ID+"/"+r.File)
		}
	}

	changed := false
	for _, name := range append(append([]string{}, walletTextFields...), walletFileFields...) {
		if pp.Fields.GetByName(name) != nil {
			pp.Fields.RemoveByName(name)
			changed = true
		}
	}
	if changed {
		if err := app.Save(pp); err != nil {
			return err
		}
	}
	return deleteFiles(app, keys)
}

// deleteFiles removes stored files and their thumbnails (best effort for
// already missing files).
func deleteFiles(app core.App, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	fsys, err := app.NewFilesystem()
	if err != nil {
		return err
	}
	defer fsys.Close()
	for _, k := range keys {
		if exists, _ := fsys.Exists(k); exists {
			if err := fsys.Delete(k); err != nil {
				return err
			}
		}
		fsys.DeletePrefix(path.Dir(k) + "/thumbs_" + path.Base(k) + "/")
	}
	return nil
}

func downDropWallets(app core.App) error {
	pp, err := app.FindCollectionByNameOrId("payout_profiles")
	if err != nil {
		return err
	}
	if pp.Fields.GetByName("wero_id") == nil {
		pp.Fields.Add(&core.TextField{Name: "wero_id", Max: 254})
	}
	if pp.Fields.GetByName("bancontact_phone") == nil {
		pp.Fields.Add(&core.TextField{Name: "bancontact_phone", Max: 16})
	}
	for _, name := range walletFileFields {
		if pp.Fields.GetByName(name) == nil {
			pp.Fields.Add(&core.FileField{Name: name, MaxSelect: 1, MaxSize: 1 << 20, MimeTypes: walletQRMimeTypes, Protected: true})
		}
	}
	return app.Save(pp)
}
