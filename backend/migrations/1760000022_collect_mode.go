package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Payer guard (ADR 0003, update 4): parties.collect_mode records how the
// payer wants to be reimbursed, chosen with the payer (POST /payer):
// "transfer" (EPC transfer QR, Revolut, PayPal, link — the payer must have a
// usable payout method) or "cash" (cash or later only, no payout profile
// needed). Empty until the payer is designated. Parties already in
// paying/closed keep their behaviour: "transfer". Idempotent.

func init() {
	m.Register(upCollectMode, downCollectMode)
}

func upCollectMode(app core.App) error {
	col, err := app.FindCollectionByNameOrId("parties")
	if err != nil {
		return err
	}
	if col.Fields.GetByName("collect_mode") == nil {
		col.Fields.Add(&core.SelectField{Name: "collect_mode", MaxSelect: 1, Values: []string{"transfer", "cash"}})
		if err := app.Save(col); err != nil {
			return err
		}
	}
	_, err = app.DB().Update("parties", dbx.Params{"collect_mode": "transfer"}, dbx.And(
		dbx.HashExp{"collect_mode": ""},
		dbx.NewExp("payer != ''"),
	)).Execute()
	return err
}

func downCollectMode(app core.App) error {
	col, err := app.FindCollectionByNameOrId("parties")
	if err != nil {
		return err
	}
	col.Fields.RemoveByName("collect_mode")
	return app.Save(col)
}
