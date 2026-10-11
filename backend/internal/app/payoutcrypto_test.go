package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/payoutvault"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/secretbox"
)

func testKeyring(t *testing.T, current byte, old ...byte) *secretbox.Keyring {
	t.Helper()
	var olds [][]byte
	for _, o := range old {
		olds = append(olds, bytes.Repeat([]byte{o}, 32))
	}
	var cur []byte
	if current != 0 {
		cur = bytes.Repeat([]byte{current}, 32)
	}
	k, err := secretbox.New(cur, olds...)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func newEnvWithKeys(t *testing.T, keys *secretbox.Keyring) *env {
	t.Helper()
	cfg := testConfig
	cfg.DataKeys = keys
	return newEnvWith(t, cfg, nil)
}

type rawPayout struct {
	ID         string `db:"id"`
	HolderName string `db:"holder_name"`
	IBAN       string `db:"iban"`
	BIC        string `db:"bic"`
	RevolutTag string `db:"revolut_tag"`
	PaypalMe   string `db:"paypal_me"`
	Updated    string `db:"updated"`
}

// rawPayoutRow reads the stored row with raw SQL (what a stolen backup holds).
func (e *env) rawPayoutRow(userID string) rawPayout {
	e.t.Helper()
	var r rawPayout
	err := e.app.DB().NewQuery("SELECT id, holder_name, iban, bic, revolut_tag, paypal_me, updated FROM payout_profiles WHERE user = {:u}").
		Bind(map[string]any{"u": userID}).One(&r)
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

func assertSealed(t *testing.T, r rawPayout, kid string) {
	t.Helper()
	for _, v := range []string{r.HolderName, r.IBAN, r.BIC} {
		if !strings.HasPrefix(v, "enc:v1:"+kid+":") {
			t.Fatalf("expected a value sealed with key %s, got %q", kid, v)
		}
	}
	all := r.HolderName + r.IBAN + r.BIC
	for _, leak := range []string{"BE71", "Martin", "GKCC"} {
		if strings.Contains(all, leak) {
			t.Fatalf("plaintext %q leaked in the stored row", leak)
		}
	}
}

func payingWithBob(t *testing.T, e *env) (pid string, alice, bob, carol user) {
	t.Helper()
	pid, alice, bob, carol = reviewParty(t, e)
	r := e.expect(200, "POST", "/api/collections/payout_profiles/records", bob.token, map[string]any{
		"user": bob.id(), "holder_name": "  Bob Martin ", "iban": "be71 0961 2345 6769", "bic": "gkccbebb",
		"revolut_tag": "@bobm",
	}).m(t)
	// the create response is the owner's: plaintext, normalized
	if r["iban"] != "BE71096123456769" || r["holder_name"] != "Bob Martin" || r["bic"] != "GKCCBEBB" {
		t.Fatalf("create response: %v", r)
	}
	return pid, alice, bob, carol
}

func TestPayoutEncryptionAtRest(t *testing.T) {
	keys := testKeyring(t, 1)
	e := newEnvWithKeys(t, keys)
	pid, alice, bob, carol := payingWithBob(t, e)
	su := e.superuser()

	row := e.rawPayoutRow(bob.id())
	assertSealed(t, row, keys.CurrentID())
	if row.RevolutTag != "bobm" {
		t.Fatalf("handles are not encrypted (semi-public): %q", row.RevolutTag)
	}

	// owner: plaintext through the collection (form of the profile page)
	view := "/api/collections/payout_profiles/records/" + row.ID
	if r := e.expect(200, "GET", view, bob.token, nil).m(t); r["iban"] != "BE71096123456769" || r["holder_name"] != "Bob Martin" || r["bic"] != "GKCCBEBB" {
		t.Fatalf("owner view: %v", r)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	e.expect(200, "GET", "/api/collections/payout_profiles/records?filter="+url.QueryEscape("user = '"+bob.id()+"'"), bob.token, nil).json(t, &list)
	if len(list.Items) != 1 || list.Items[0]["iban"] != "BE71096123456769" {
		t.Fatalf("owner list: %v", list.Items)
	}
	// other members: nothing (rules); superuser: the envelope only
	e.expect(404, "GET", view, alice.token, nil)
	e.expect(200, "GET", "/api/collections/payout_profiles/records", alice.token, nil).json(t, &list)
	if len(list.Items) != 0 {
		t.Fatalf("alice sees %v", list.Items)
	}
	if r := e.expect(200, "GET", view, su, nil).m(t); !secretbox.IsSealed(r["iban"].(string)) {
		t.Fatalf("superuser must not get the plaintext: %v", r)
	}

	// partial update: the untouched sealed fields are opened, validated and sealed again
	if r := e.expect(200, "PATCH", view, bob.token, map[string]any{"paypal_me": "bob"}).m(t); r["iban"] != "BE71096123456769" || r["paypal_me"] != "bob" {
		t.Fatalf("patch response: %v", r)
	}
	row2 := e.rawPayoutRow(bob.id())
	assertSealed(t, row2, keys.CurrentID())
	if iban, _, err := keys.Open(row2.IBAN, payoutvault.AAD("iban", bob.id())); err != nil || iban != "BE71096123456769" {
		t.Fatalf("stored iban: %q %v", iban, err)
	}
	// validation still runs on the plaintext
	e.expect(400, "PATCH", view, bob.token, map[string]any{"iban": "BE00 0000 0000 0000"})
	// a forged envelope is refused (unreadable), never stored as is
	e.expect(422, "PATCH", view, bob.token, map[string]any{"iban": "enc:v1:" + keys.CurrentID() + ":AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"})
	// a ciphertext moved to another owner's row does not open
	e.expect(422, "POST", "/api/collections/payout_profiles/records", carol.token, map[string]any{"user": carol.id(), "iban": row2.IBAN})

	// payer guard: readiness reads the IBAN in plaintext (mod-97)
	var rd readinessResp
	e.expect(200, "GET", path("/api/occ/parties/%s/payout-readiness", pid), alice.token, nil).json(t, &rd)
	for _, m := range rd.Members {
		if m.User == bob.id() && (!m.Payout.Ready || !m.Payout.IBAN) {
			t.Fatalf("bob readiness: %+v", m)
		}
	}
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": bob.id()})

	// EPC payload for alice's share: plaintext IBAN, BIC and holder
	pay, err := e.app.FindFirstRecordByFilter(colPayments, "party = {:p} && debtor = {:d}", map[string]any{"p": pid, "d": alice.id()})
	if err != nil {
		t.Fatal(err)
	}
	var qr struct {
		Beneficiary string  `json:"beneficiary"`
		EPC         *string `json:"epc"`
		IBAN        *string `json:"iban"`
	}
	e.expect(200, "GET", path("/api/occ/payments/%s/qr", pay.Id), alice.token, nil).json(t, &qr)
	if qr.EPC == nil || !strings.HasPrefix(*qr.EPC, "BCD\n002\n1\nSCT\nGKCCBEBB\nBob Martin\nBE71096123456769\nEUR") || qr.IBAN == nil || *qr.IBAN != "BE71096123456769" || qr.Beneficiary != "Bob Martin" {
		t.Fatalf("qr: %+v %v", qr, qr.EPC)
	}
	// « Encaisser » (payer only)
	var collect struct {
		IBAN  *string `json:"iban"`
		Items []struct {
			EPC *string `json:"epc"`
		} `json:"items"`
	}
	e.expect(200, "GET", path("/api/occ/parties/%s/payments/qr", pid), bob.token, nil).json(t, &collect)
	if collect.IBAN == nil || *collect.IBAN != "BE71096123456769" || len(collect.Items) == 0 || !strings.Contains(*collect.Items[0].EPC, "BE71096123456769") {
		t.Fatalf("collect: %+v", collect)
	}
	// carol (another member) gets the QR of her own share only; never the profile
	e.expect(404, "GET", view, carol.token, nil)
	// reads never rewrite a fresh row
	if e.rawPayoutRow(bob.id()) != row2 {
		t.Fatal("a row sealed with the current key must not be rewritten on read")
	}

	// realtime / enrich: a record loaded server-side keeps the envelope in memory
	rec, _ := e.app.FindRecordById(colPayoutProfiles, row.ID)
	if !secretbox.IsSealed(rec.GetString("iban")) {
		t.Fatal("records loaded from the database hold the envelope")
	}
	// a server-side save of such a record keeps working (opened, validated, sealed)
	rec.Set("paypal_me", "bob2")
	if err := e.app.Save(rec); err != nil {
		t.Fatal(err)
	}
	if rec.GetString("iban") != "BE71096123456769" {
		t.Fatalf("in-memory value after save: %q", rec.GetString("iban"))
	}
	assertSealed(t, e.rawPayoutRow(bob.id()), keys.CurrentID())
}

func TestPayoutKeyRotation(t *testing.T) {
	k1 := testKeyring(t, 1)
	e := newEnvWithKeys(t, k1)
	pid, alice, bob, carol := payingWithBob(t, e)
	e.expect(200, "POST", "/api/collections/payout_profiles/records", carol.token, map[string]any{"user": carol.id(), "holder_name": "Carol Martin", "iban": "BE68539007547034", "bic": "BBRUBEBB"})
	assertSealed(t, e.rawPayoutRow(bob.id()), k1.CurrentID())

	// rotation: new current key 2, key 1 kept for decryption only
	k21 := testKeyring(t, 2, 1)
	payoutvault.SetKeys(e.app, k21)
	view := "/api/collections/payout_profiles/records/" + e.rawPayoutRow(bob.id()).ID
	if r := e.expect(200, "GET", view, bob.token, nil).m(t); r["iban"] != "BE71096123456769" {
		t.Fatalf("old key must still open: %v", r)
	}
	// a server read (payer guard of bob only) re-seals the stale row lazily with the new key
	e.expect(200, "POST", path("/api/occ/parties/%s/payer", pid), alice.token, map[string]any{"payer": bob.id()})
	if !strings.Contains(e.rawPayoutRow(carol.id()).IBAN, k1.CurrentID()) {
		t.Fatal("carol's row was not read: still sealed with key 1")
	}
	bobRow := e.rawPayoutRow(bob.id())
	assertSealed(t, bobRow, k21.CurrentID())

	// the one-shot command re-seals the rest (carol), then old key can go
	key := func(b byte) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32)) }
	env := map[string]string{"OCC_DATA_KEY": key(2), "OCC_DATA_KEY_OLD": key(1)}
	cmd := NewPayoutsReencryptCommand(e.app, func(k string) string { return env[k] })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1 updated") || strings.Contains(out.String(), key(2)) {
		t.Fatalf("command output: %q", out.String())
	}
	assertSealed(t, e.rawPayoutRow(carol.id()), k21.CurrentID())
	if e.rawPayoutRow(bob.id()) != bobRow {
		t.Fatal("already rotated rows are untouched")
	}
	payoutvault.SetKeys(e.app, testKeyring(t, 2))
	if r := e.expect(200, "GET", "/api/collections/payout_profiles/records/"+e.rawPayoutRow(carol.id()).ID, carol.token, nil).m(t); r["iban"] != "BE68539007547034" {
		t.Fatalf("after rotation: %v", r)
	}

	// command errors: no key / unknown key left
	cmd = NewPayoutsReencryptCommand(e.app, func(string) string { return "" })
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("the command needs OCC_DATA_KEY")
	}
	cmd = NewPayoutsReencryptCommand(e.app, func(k string) string { return map[string]string{"OCC_DATA_KEY": key(3)}[k] })
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("rows sealed with a missing key must be reported: %v", err)
	}

	// --decrypt: back to plaintext (turning encryption off)
	cmd = NewPayoutsReencryptCommand(e.app, func(k string) string { return map[string]string{"OCC_DATA_KEY_OLD": key(2)}[k] })
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--decrypt"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if r := e.rawPayoutRow(bob.id()); r.IBAN != "BE71096123456769" || r.HolderName != "Bob Martin" {
		t.Fatalf("decrypt: %+v", r)
	}
}

func TestPayoutWithoutKey(t *testing.T) {
	e := newEnvWithKeys(t, nil)
	pid, alice, bob, _ := payingWithBob(t, e)

	// no key: legacy plaintext storage, everything works
	row := e.rawPayoutRow(bob.id())
	if row.IBAN != "BE71096123456769" || row.HolderName != "Bob Martin" {
		t.Fatalf("plaintext expected without key: %+v", row)
	}
	e.expect(200, "GET", path("/api/occ/parties/%s/payout-readiness", pid), alice.token, nil)

	// the key appears (next start): the startup sweep seals the rows
	k1 := testKeyring(t, 1)
	payoutvault.SetKeys(e.app, k1)
	cfg := testConfig
	cfg.PublicURL = "https://eat.example.org"
	if err := checkPayoutEncryption(e.app, cfg); err != nil {
		t.Fatal(err)
	}
	assertSealed(t, e.rawPayoutRow(bob.id()), k1.CurrentID())
	before := e.rawPayoutRow(bob.id())
	if err := checkPayoutEncryption(e.app, cfg); err != nil { // idempotent
		t.Fatal(err)
	}
	if e.rawPayoutRow(bob.id()) != before {
		t.Fatal("second sweep must not rewrite")
	}

	// the key is lost (misconfiguration): no crash, nobody locked out
	payoutvault.SetKeys(e.app, nil)
	if err := checkPayoutEncryption(e.app, cfg); err != nil {
		t.Fatal(err)
	}
	view := "/api/collections/payout_profiles/records/" + row.ID
	if r := e.expect(200, "GET", view, bob.token, nil).m(t); r["iban"] != "" || r["revolut_tag"] != "bobm" {
		t.Fatalf("unreadable fields are blanked for the owner, never sent as envelopes: %v", r)
	}
	var rd readinessResp
	e.expect(200, "GET", path("/api/occ/parties/%s/payout-readiness", pid), alice.token, nil).json(t, &rd)
	for _, m := range rd.Members {
		if m.User == bob.id() && (m.Payout.IBAN || !m.Payout.Ready) { // Revolut still usable
			t.Fatalf("bob readiness without key: %+v", m)
		}
	}
	// a partial update cannot keep unreadable data: French 422
	r := e.expect(422, "PATCH", view, bob.token, map[string]any{"paypal_me": "bob"})
	if !strings.Contains(string(r.body), "Ressaisis ton IBAN") {
		t.Fatalf("message: %s", r.body)
	}
	// re-entering everything works (stored in plaintext, no key)
	e.expect(200, "PATCH", view, bob.token, map[string]any{"holder_name": "Bob Martin", "iban": "BE71096123456769", "bic": ""})
	if r := e.rawPayoutRow(bob.id()); r.IBAN != "BE71096123456769" {
		t.Fatalf("re-entered: %+v", r)
	}

	// an invalid key refuses to start (named, never echoed)
	cfg.DataKeyErr = errors.New("OCC_DATA_KEY: secretbox: key is not valid base64")
	if err := checkPayoutEncryption(e.app, cfg); err == nil || !strings.Contains(err.Error(), "OCC_DATA_KEY") {
		t.Fatalf("invalid key: %v", err)
	}
}

func TestDataKeysFromEnvConfig(t *testing.T) {
	k := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32))
	t.Setenv("OCC_DATA_KEY", k)
	t.Setenv("OCC_DATA_KEY_OLD", "")
	cfg := ConfigFromEnv("test")
	if cfg.DataKeyErr != nil || !cfg.DataKeys.Enabled() {
		t.Fatalf("config: %v", cfg.DataKeyErr)
	}
	t.Setenv("OCC_DATA_KEY", "abc")
	if cfg := ConfigFromEnv("test"); cfg.DataKeyErr == nil || strings.Contains(cfg.DataKeyErr.Error(), "abc") {
		t.Fatalf("invalid key: %v", cfg.DataKeyErr)
	}
}

func TestSuperuserMFA(t *testing.T) {
	e := newEnv(t) // SuperuserMFA false in testConfig, no SMTP
	col, _ := e.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if col.MFA.Enabled {
		t.Fatal("MFA must not be enabled without SMTP")
	}
	su := core.NewRecord(col)
	su.SetEmail("root@example.com")
	su.SetPassword("password123456")
	if err := e.app.Save(su); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig
	cfg.SuperuserMFA = true
	// without SMTP: left off (no lock-out)
	if err := applySuperuserMFA(e.app, cfg); err != nil {
		t.Fatal(err)
	}
	col, _ = e.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if col.MFA.Enabled || col.OTP.Enabled {
		t.Fatal("no SMTP: unchanged")
	}

	enableTestSMTP(e)
	for range 2 { // idempotent
		if err := applySuperuserMFA(e.app, cfg); err != nil {
			t.Fatal(err)
		}
	}
	col, _ = e.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if !col.MFA.Enabled || col.MFA.Duration != superuserMFADuration || !col.OTP.Enabled || col.OTP.Length != 6 ||
		col.OTP.Duration != superuserOTPDuration || !strings.Contains(col.OTP.EmailTemplate.Subject, "administration") {
		t.Fatalf("collection: %+v %+v", col.MFA, col.OTP)
	}

	// password alone is not enough
	login := map[string]any{"identity": "root@example.com", "password": "password123456"}
	r := e.do("POST", "/api/collections/_superusers/auth-with-password", "", login)
	mfaID, _ := r.m(t)["mfaId"].(string)
	if r.status != 401 || mfaID == "" {
		t.Fatalf("expected 401 + mfaId, got %d %s", r.status, r.body)
	}
	// code by e-mail (French template), then password + code
	mailer := e.app.TestMailer
	n := mailer.TotalSend()
	otp := e.expect(200, "POST", "/api/collections/_superusers/request-otp", "", map[string]any{"email": "root@example.com"}).m(t)
	for i := 0; i < 200 && mailer.TotalSend() == n; i++ { // sent in background by PocketBase
		time.Sleep(10 * time.Millisecond)
	}
	msg := mailer.LastMessage()
	if !strings.Contains(msg.Subject, "Code de connexion à l'administration") || strings.Contains(msg.HTML, "{OTP}") {
		t.Fatalf("otp mail: %s", msg.Subject)
	}
	code := ""
	if i := strings.Index(msg.HTML, "letter-spacing:0.18em\">"); i >= 0 {
		code = msg.HTML[i+len("letter-spacing:0.18em\">"):]
		code = code[:strings.Index(code, "<")]
	}
	if len(code) != 6 {
		t.Fatalf("6-digit code expected, got %q", code)
	}
	auth := e.expect(200, "POST", "/api/collections/_superusers/auth-with-otp?mfaId="+mfaID, "", map[string]any{"otpId": otp["otpId"], "password": code}).m(t)
	if auth["token"] == "" {
		t.Fatalf("auth: %v", auth)
	}

	// the startup bootstrap (upsert) keeps working with MFA on
	su.SetPassword("another-password-123")
	if err := e.app.Save(su); err != nil {
		t.Fatal(err)
	}

	// OCC_SUPERUSER_MFA=false turns it off
	cfg.SuperuserMFA = false
	if err := applySuperuserMFA(e.app, cfg); err != nil {
		t.Fatal(err)
	}
	col, _ = e.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if col.MFA.Enabled {
		t.Fatal("OCC_SUPERUSER_MFA=false must disable MFA")
	}
	e.expect(200, "POST", "/api/collections/_superusers/auth-with-password", "", map[string]any{"identity": "root@example.com", "password": "another-password-123"})

	if !superuserMFAFromEnv(func(string) string { return "" }) || superuserMFAFromEnv(func(string) string { return "false" }) {
		t.Fatal("OCC_SUPERUSER_MFA default true")
	}
}
