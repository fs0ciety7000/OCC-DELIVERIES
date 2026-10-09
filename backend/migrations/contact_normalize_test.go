package migrations

import (
	"testing"

	"github.com/pocketbase/dbx"
)

func TestContactNormalizeMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)
	rest, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		t.Fatal(err)
	}
	if rest.Fields.GetByName("enriched_from") == nil {
		t.Fatal("enriched_from field missing")
	}
	recs, err := app.FindAllRecords("restaurants")
	if err != nil || len(recs) < 4 {
		t.Fatalf("%d restaurants, %v", len(recs), err)
	}
	cases := []struct{ phone, address, wantPhone, wantAddress string }{
		{"0495466512", "24 Rue de la Clef, 7000", "+32495466512", "Rue de la Clef 24, 7000 Mons"},
		{"+32484158003", "Rue d'Enghien 11, 7000 Mons, Belgique", "+32484158003", "Rue d'Enghien 11, 7000 Mons"},
		{"065/35.29.64", "14 rue de la clef 7000", "+3265352964", "Rue de la Clef 14, 7000 Mons"},
		{"voir site", "", "voir site", ""}, // unreadable phone kept, never deleted
	}
	for i, c := range cases {
		if _, err := app.DB().Update("restaurants", dbx.Params{"phone": c.phone, "address": c.address, "locked": i == 0}, dbx.HashExp{"id": recs[i].Id}).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	for run := 0; run < 2; run++ { // idempotent
		if err := upContactNormalize(app); err != nil {
			t.Fatal(err)
		}
		for i, c := range cases {
			r, err := app.FindRecordById("restaurants", recs[i].Id)
			if err != nil {
				t.Fatal(err)
			}
			if r.GetString("phone") != c.wantPhone || r.GetString("address") != c.wantAddress {
				t.Fatalf("run %d, %q / %q → %q / %q", run, c.phone, c.address, r.GetString("phone"), r.GetString("address"))
			}
		}
	}
}
