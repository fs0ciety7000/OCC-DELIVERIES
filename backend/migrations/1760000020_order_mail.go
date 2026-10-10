package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Order e-mail (« bon de commande ») sent once to every member who ordered
// when the party enters "paying": parties.order_mail_sent_at (hidden date,
// written by the server only, claimed atomically so the mail is never sent
// twice). Parties already past "review" keep it empty: the mail is only sent
// on the review → paying transition. Idempotent.

func init() {
	m.Register(upOrderMail, downOrderMail)
}

func upOrderMail(app core.App) error {
	col, err := app.FindCollectionByNameOrId("parties")
	if err != nil {
		return err
	}
	if col.Fields.GetByName("order_mail_sent_at") != nil {
		return nil
	}
	col.Fields.Add(&core.DateField{Name: "order_mail_sent_at", Hidden: true})
	return app.Save(col)
}

func downOrderMail(app core.App) error {
	col, err := app.FindCollectionByNameOrId("parties")
	if err != nil {
		return err
	}
	col.Fields.RemoveByName("order_mail_sent_at")
	return app.Save(col)
}
