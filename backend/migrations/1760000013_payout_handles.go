package migrations

import (
	"slices"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Structured payout handles: payout_profiles.revolut_tag / paypal_me (the
// server builds per-payment links with the exact amount), payments.method
// gains "revolut" / "paypal", and the legacy free payment_link is moved into
// the matching field when it is a revolut.me / PayPal.me link. Idempotent.

func init() {
	m.Register(upPayoutHandles, downPayoutHandles)
}

var newPaymentMethods = []string{domain.MethodRevolut, domain.MethodPayPal}

func upPayoutHandles(app core.App) error {
	pp, err := app.FindCollectionByNameOrId("payout_profiles")
	if err != nil {
		return err
	}
	if pp.Fields.GetByName("revolut_tag") == nil {
		pp.Fields.Add(&core.TextField{Name: "revolut_tag", Max: 32})
	}
	if pp.Fields.GetByName("paypal_me") == nil {
		pp.Fields.Add(&core.TextField{Name: "paypal_me", Max: 40})
	}
	if err := app.Save(pp); err != nil {
		return err
	}

	pay, err := app.FindCollectionByNameOrId("payments")
	if err != nil {
		return err
	}
	if f, ok := pay.Fields.GetByName("method").(*core.SelectField); ok {
		changed := false
		for _, v := range newPaymentMethods {
			if !slices.Contains(f.Values, v) {
				f.Values = append(f.Values, v)
				changed = true
			}
		}
		if changed {
			if err := app.Save(pay); err != nil {
				return err
			}
		}
	}
	return SplitLegacyPaymentLinks(app)
}

// SplitLegacyPaymentLinks moves recognizable payment links into the
// structured fields (direct update: no hook, `updated` untouched).
func SplitLegacyPaymentLinks(app core.App) error {
	var rows []struct {
		ID      string `db:"id"`
		Revolut string `db:"revolut_tag"`
		PayPal  string `db:"paypal_me"`
		Link    string `db:"payment_link"`
	}
	if err := app.DB().Select("id", "revolut_tag", "paypal_me", "payment_link").From("payout_profiles").All(&rows); err != nil {
		return err
	}
	for _, r := range rows {
		in := domain.PayoutHandles{RevolutTag: r.Revolut, PayPalMe: r.PayPal, Link: r.Link}
		out := domain.SplitPayoutLink(in)
		if out == in {
			continue
		}
		if _, err := app.DB().Update("payout_profiles", dbx.Params{
			"revolut_tag": out.RevolutTag, "paypal_me": out.PayPalMe, "payment_link": out.Link,
		}, dbx.HashExp{"id": r.ID}).Execute(); err != nil {
			return err
		}
	}
	return nil
}

func downPayoutHandles(app core.App) error {
	pp, err := app.FindCollectionByNameOrId("payout_profiles")
	if err != nil {
		return err
	}
	// Give the handles back to payment_link when it is free.
	var rows []struct {
		ID      string `db:"id"`
		Revolut string `db:"revolut_tag"`
		PayPal  string `db:"paypal_me"`
		Link    string `db:"payment_link"`
	}
	if err := app.DB().Select("id", "revolut_tag", "paypal_me", "payment_link").From("payout_profiles").All(&rows); err != nil {
		return err
	}
	for _, r := range rows {
		link := r.Link
		switch {
		case link != "":
		case r.Revolut != "":
			link = "https://revolut.me/" + r.Revolut
		case r.PayPal != "":
			link = "https://paypal.me/" + r.PayPal
		default:
			continue
		}
		if _, err := app.DB().Update("payout_profiles", dbx.Params{"payment_link": link}, dbx.HashExp{"id": r.ID}).Execute(); err != nil {
			return err
		}
	}
	for _, name := range []string{"revolut_tag", "paypal_me"} {
		if f := pp.Fields.GetByName(name); f != nil {
			pp.Fields.RemoveById(f.GetId())
		}
	}
	if err := app.Save(pp); err != nil {
		return err
	}

	pay, err := app.FindCollectionByNameOrId("payments")
	if err != nil {
		return err
	}
	if _, err := app.DB().Update("payments", dbx.Params{"method": domain.MethodLink},
		dbx.In("method", domain.MethodRevolut, domain.MethodPayPal)).Execute(); err != nil {
		return err
	}
	if f, ok := pay.Fields.GetByName("method").(*core.SelectField); ok {
		f.Values = slices.DeleteFunc(f.Values, func(v string) bool { return slices.Contains(newPaymentMethods, v) })
		return app.Save(pay)
	}
	return nil
}
