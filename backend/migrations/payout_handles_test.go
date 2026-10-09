package migrations

import (
	"slices"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func TestPayoutHandlesMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)

	pp, err := app.FindCollectionByNameOrId("payout_profiles")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"revolut_tag", "paypal_me", "payment_link"} {
		if pp.Fields.GetByName(f) == nil {
			t.Fatalf("field %s missing", f)
		}
	}
	pay, err := app.FindCollectionByNameOrId("payments")
	if err != nil {
		t.Fatal(err)
	}
	values := pay.Fields.GetByName("method").(*core.SelectField).Values
	if !slices.Contains(values, "revolut") || !slices.Contains(values, "paypal") || !slices.Contains(values, "link") {
		t.Fatalf("method values %v", values)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ link, revolut, wantRevolut, wantPayPal, wantLink string }{
		{"https://revolut.me/Bob", "", "bob", "", ""},
		{"https://www.paypal.me/jdoe/5EUR", "", "", "jdoe", ""},
		{"https://wise.com/pay/business/acme", "", "", "", "https://wise.com/pay/business/acme"},
		{"https://revolut.me/other", "mine", "mine", "", "https://revolut.me/other"},
		{"", "", "", "", ""},
	}
	ids := make([]string, len(cases))
	for i, c := range cases {
		u := core.NewRecord(users)
		u.SetEmail("u" + string(rune('a'+i)) + "@example.com")
		u.SetPassword("password123")
		u.Set("name", "U")
		if err := app.Save(u); err != nil {
			t.Fatal(err)
		}
		r := core.NewRecord(pp)
		r.Set("user", u.Id)
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
		if _, err := app.DB().Update("payout_profiles", dbx.Params{"payment_link": c.link, "revolut_tag": c.revolut}, dbx.HashExp{"id": r.Id}).Execute(); err != nil {
			t.Fatal(err)
		}
		ids[i] = r.Id
	}
	for run := 0; run < 2; run++ { // idempotent
		if err := upPayoutHandles(app); err != nil {
			t.Fatal(err)
		}
		for i, c := range cases {
			r, err := app.FindRecordById("payout_profiles", ids[i])
			if err != nil {
				t.Fatal(err)
			}
			if r.GetString("revolut_tag") != c.wantRevolut || r.GetString("paypal_me") != c.wantPayPal || r.GetString("payment_link") != c.wantLink {
				t.Fatalf("run %d, %q → %q / %q / %q", run, c.link, r.GetString("revolut_tag"), r.GetString("paypal_me"), r.GetString("payment_link"))
			}
		}
	}
}
