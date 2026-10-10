package migrations

import (
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

func TestDropWalletsMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)

	pp, err := app.FindCollectionByNameOrId("payout_profiles")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"wero_id", "bancontact_phone", "wero_qr", "bancontact_qr"} {
		if pp.Fields.GetByName(f) != nil {
			t.Fatalf("field %s must be dropped", f)
		}
	}
	pay, err := app.FindCollectionByNameOrId("payments")
	if err != nil {
		t.Fatal(err)
	}
	values := pay.Fields.GetByName("method").(*core.SelectField).Values
	for _, v := range []string{"wero", "bancontact"} {
		if !slices.Contains(values, v) {
			t.Fatalf("payments.method must keep %q for old records: %v", v, values)
		}
	}

	// back to the pre-migration schema, with a filled profile (production)
	if err := downDropWallets(app); err != nil {
		t.Fatal(err)
	}
	pp, _ = app.FindCollectionByNameOrId("payout_profiles")
	users, _ := app.FindCollectionByNameOrId("users")
	u := core.NewRecord(users)
	u.SetEmail("w@example.com")
	u.SetPassword("password123")
	u.Set("name", "W")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x35\x81\x84\x00\x00\x00\x00IEND\xaeB`\x82")
	f1, _ := filesystem.NewFileFromBytes(png, "wero.png")
	f2, _ := filesystem.NewFileFromBytes(png, "bc.png")
	r := core.NewRecord(pp)
	r.Set("user", u.Id)
	r.Set("iban", "BE71096123456769")
	r.Set("revolut_tag", "wally")
	r.Set("wero_id", "+32470123456")
	r.Set("bancontact_phone", "+32470123456")
	r.Set("wero_qr", f1)
	r.Set("bancontact_qr", f2)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	keys := []string{r.BaseFilesPath() + "/" + r.GetString("wero_qr"), r.BaseFilesPath() + "/" + r.GetString("bancontact_qr")}
	fsys, err := app.NewFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	for _, k := range keys {
		if ok, _ := fsys.Exists(k); !ok {
			t.Fatalf("fixture file %s missing", k)
		}
	}

	for run := 0; run < 2; run++ { // idempotent
		if err := upDropWallets(app); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range keys {
		if ok, _ := fsys.Exists(k); ok {
			t.Fatalf("wallet QR %s must be deleted", k)
		}
	}
	got, err := app.FindRecordById("payout_profiles", r.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetString("iban") != "BE71096123456769" || got.GetString("revolut_tag") != "wally" {
		t.Fatalf("other payout data must be kept: %v", got.PublicExport())
	}
	if _, ok := got.FieldsData()["wero_id"]; ok {
		t.Fatal("wero_id still present")
	}
}
