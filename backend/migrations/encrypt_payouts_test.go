package migrations

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/payoutvault"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/secretbox"
)

type payoutRow struct {
	ID         string `db:"id"`
	User       string `db:"user"`
	HolderName string `db:"holder_name"`
	IBAN       string `db:"iban"`
	BIC        string `db:"bic"`
	RevolutTag string `db:"revolut_tag"`
}

func readPayoutRows(t *testing.T, app core.App) map[string]payoutRow {
	t.Helper()
	var rows []payoutRow
	if err := app.DB().Select("id", "user", "holder_name", "iban", "bic", "revolut_tag").From("payout_profiles").All(&rows); err != nil {
		t.Fatal(err)
	}
	out := map[string]payoutRow{}
	for _, r := range rows {
		out[r.User] = r
	}
	return out
}

func TestEncryptPayoutsMigration(t *testing.T) {
	defer SetRealDataForTesting([]byte("[]"))()
	app := newApp(t)
	oldEnv := dataKeysEnv
	defer func() { dataKeysEnv = oldEnv }()

	users, _ := app.FindCollectionByNameOrId("users")
	pp, _ := app.FindCollectionByNameOrId("payout_profiles")
	ids := map[string]string{}
	for i, name := range []string{"Bob", "Zoé"} {
		u := core.NewRecord(users)
		u.SetEmail(strings.ToLower(name[:1]) + "@example.com")
		u.SetPassword("password123")
		u.Set("name", name)
		if err := app.Save(u); err != nil {
			t.Fatal(err)
		}
		ids[name] = u.Id
		r := core.NewRecord(pp) // no app hooks here: stored in plaintext (production before the migration)
		r.Set("user", u.Id)
		r.Set("holder_name", name+" Martin")
		if i == 0 {
			r.Set("iban", "BE71096123456769")
			r.Set("bic", "GKCCBEBB")
		}
		r.Set("revolut_tag", "bobm")
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
	}

	key := bytes.Repeat([]byte{9}, 32)
	keys, _ := secretbox.New(key)
	envWith := func(v map[string]string) func(string) string { return func(k string) string { return v[k] } }

	// no key: nothing changes, no error
	dataKeysEnv = envWith(nil)
	if err := upEncryptPayouts(app); err != nil {
		t.Fatal(err)
	}
	if r := readPayoutRows(t, app)[ids["Bob"]]; r.IBAN != "BE71096123456769" {
		t.Fatalf("without key the row must stay in plaintext: %+v", r)
	}

	// invalid key: refused, nothing written
	dataKeysEnv = envWith(map[string]string{"OCC_DATA_KEY": "trop-courte"})
	if err := upEncryptPayouts(app); err == nil {
		t.Fatal("an invalid key must fail the migration")
	}

	// with the key: sealed, idempotent
	dataKeysEnv = envWith(map[string]string{"OCC_DATA_KEY": base64.StdEncoding.EncodeToString(key)})
	var first map[string]payoutRow
	for run := 0; run < 2; run++ {
		if err := upEncryptPayouts(app); err != nil {
			t.Fatal(err)
		}
		rows := readPayoutRows(t, app)
		if run == 0 {
			first = rows
		} else if rows[ids["Bob"]] != first[ids["Bob"]] {
			t.Fatal("second run must not re-seal (idempotent)")
		}
	}
	bob := first[ids["Bob"]]
	for _, v := range []string{bob.HolderName, bob.IBAN, bob.BIC} {
		if !strings.HasPrefix(v, "enc:v1:"+keys.CurrentID()+":") {
			t.Fatalf("expected a sealed value, got %q", v)
		}
	}
	if bob.RevolutTag != "bobm" {
		t.Fatalf("handles stay in plaintext: %q", bob.RevolutTag)
	}
	if iban, _, err := keys.Open(bob.IBAN, payoutvault.AAD("iban", ids["Bob"])); err != nil || iban != "BE71096123456769" {
		t.Fatalf("open: %q %v", iban, err)
	}
	if z := first[ids["Zoé"]]; z.IBAN != "" || z.BIC != "" || !secretbox.IsSealed(z.HolderName) {
		t.Fatalf("empty fields stay empty: %+v", z)
	}

	// down: back to plaintext
	if err := downEncryptPayouts(app); err != nil {
		t.Fatal(err)
	}
	if r := readPayoutRows(t, app)[ids["Bob"]]; r.IBAN != "BE71096123456769" || r.HolderName != "Bob Martin" || r.BIC != "GKCCBEBB" {
		t.Fatalf("down must restore plaintext: %+v", r)
	}
	n, _ := app.CountRecords("payout_profiles", dbx.NewExp("iban LIKE 'enc:%' OR holder_name LIKE 'enc:%'"))
	if n != 0 {
		t.Fatalf("%d sealed rows left after down", n)
	}
}
