package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tests"
)

// softAuthenticator is a software passkey provider: an ECDSA P-256 key
// (ES256), "none" attestation, synced (BE + BS) like iCloud / Google.
type softAuthenticator struct {
	t       *testing.T
	key     *ecdsa.PrivateKey
	credID  []byte
	aaguid  []byte
	rpID    string
	origin  string
	counter uint32
	userID  []byte // user handle, learnt at registration
}

func newSoftAuthenticator(t *testing.T) *softAuthenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	// AAGUID of iCloud Keychain → default name « Trousseau iCloud »
	aaguid := []byte{0xfb, 0xfc, 0x30, 0x07, 0x15, 0x4e, 0x4e, 0xcc, 0x8c, 0x0b, 0x6e, 0x02, 0x05, 0x57, 0xd7, 0xbd}
	return &softAuthenticator{t: t, key: key, credID: id, aaguid: aaguid, rpID: "localhost", origin: "http://localhost:8090"}
}

var b64t = base64.RawURLEncoding

const flagsSynced = 0x01 | 0x04 | 0x08 | 0x10 // UP | UV | BE | BS

func (a *softAuthenticator) clientData(typ, challenge string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": a.origin, "crossOrigin": false})
	return b
}

func (a *softAuthenticator) authData(flags byte, withCred bool) []byte {
	h := sha256.Sum256([]byte(a.rpID))
	out := append([]byte{}, h[:]...)
	if withCred {
		flags |= 0x40 // AT
	}
	out = append(out, flags)
	out = binary.BigEndian.AppendUint32(out, a.counter)
	if withCred {
		out = append(out, a.aaguid...)
		out = binary.BigEndian.AppendUint16(out, uint16(len(a.credID)))
		out = append(out, a.credID...)
		x := a.key.X.FillBytes(make([]byte, 32))
		y := a.key.Y.FillBytes(make([]byte, 32))
		cose, err := webauthncbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
		if err != nil {
			a.t.Fatal(err)
		}
		out = append(out, cose...)
	}
	return out
}

// create answers navigator.credentials.create() options (begin response).
func (a *softAuthenticator) create(options map[string]any) map[string]any {
	a.t.Helper()
	pk := options["publicKey"].(map[string]any)
	user := pk["user"].(map[string]any)
	uid, err := b64t.DecodeString(user["id"].(string))
	if err != nil {
		a.t.Fatal(err)
	}
	a.userID = uid
	att, err := webauthncbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(flagsSynced, true)})
	if err != nil {
		a.t.Fatal(err)
	}
	return map[string]any{
		"id": b64t.EncodeToString(a.credID), "rawId": b64t.EncodeToString(a.credID), "type": "public-key",
		"authenticatorAttachment": "platform",
		"response": map[string]any{
			"clientDataJSON":    b64t.EncodeToString(a.clientData("webauthn.create", pk["challenge"].(string))),
			"attestationObject": b64t.EncodeToString(att),
			"transports":        []string{"internal", "hybrid"},
		},
		"clientExtensionResults": map[string]any{"credProps": map[string]any{"rk": true}},
	}
}

// get answers navigator.credentials.get() options (discoverable login).
func (a *softAuthenticator) get(options map[string]any) map[string]any {
	a.t.Helper()
	pk := options["publicKey"].(map[string]any)
	cd := a.clientData("webauthn.get", pk["challenge"].(string))
	ad := a.authData(flagsSynced, false)
	h := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), h[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		a.t.Fatal(err)
	}
	return map[string]any{
		"id": b64t.EncodeToString(a.credID), "rawId": b64t.EncodeToString(a.credID), "type": "public-key",
		"authenticatorAttachment": "platform",
		"response": map[string]any{
			"clientDataJSON":    b64t.EncodeToString(cd),
			"authenticatorData": b64t.EncodeToString(ad),
			"signature":         b64t.EncodeToString(sig),
			"userHandle":        b64t.EncodeToString(a.userID),
		},
		"clientExtensionResults": map[string]any{},
	}
}

func (e *env) registerPasskey(u user, a *softAuthenticator, name string) map[string]any {
	e.t.Helper()
	opts := e.expect(http.StatusOK, "POST", "/api/occ/passkeys/register/begin", u.token, map[string]any{"name": name}).m(e.t)
	return e.expect(http.StatusOK, "POST", "/api/occ/passkeys/register/finish", u.token,
		map[string]any{"credential": a.create(opts)}).m(e.t)
}

func (e *env) passkeyLogin(a *softAuthenticator) resp {
	e.t.Helper()
	opts := e.expect(http.StatusOK, "POST", "/api/occ/passkeys/login/begin", "", map[string]any{}).m(e.t)
	return e.do("POST", "/api/occ/passkeys/login/finish", "", map[string]any{"credential": a.get(opts)})
}

func TestPasskeyRegisterAndLogin(t *testing.T) {
	e := newEnv(t)
	alice := e.user("Alice")
	a := newSoftAuthenticator(t)

	// begin: discoverable credential, user verification, no attestation
	opts := e.expect(http.StatusOK, "POST", "/api/occ/passkeys/register/begin", alice.token, nil).m(t)
	pk := opts["publicKey"].(map[string]any)
	if pk["rp"].(map[string]any)["id"] != "localhost" || pk["attestation"] != "none" {
		t.Fatalf("unexpected options: %v", pk)
	}
	sel := pk["authenticatorSelection"].(map[string]any)
	if sel["residentKey"] != "required" || sel["userVerification"] != "required" {
		t.Fatalf("authenticatorSelection = %v", sel)
	}
	if u := pk["user"].(map[string]any); u["name"] != "alice@example.com" || u["displayName"] != "Alice" {
		t.Fatalf("user entity = %v", u)
	}
	view := e.expect(http.StatusOK, "POST", "/api/occ/passkeys/register/finish", alice.token,
		map[string]any{"credential": a.create(opts)}).m(t)
	if view["name"] != "Trousseau iCloud" || view["synced"] != true || view["lastUsedAt"] != "" {
		t.Fatalf("view = %v", view)
	}

	// named at begin
	b := newSoftAuthenticator(t)
	if v := e.registerPasskey(alice, b, "  Clé   USB "); v["name"] != "Clé USB" {
		t.Fatalf("name = %v", v["name"])
	}
	// the same credential cannot be registered twice
	opts = e.expect(http.StatusOK, "POST", "/api/occ/passkeys/register/begin", alice.token, nil).m(t)
	if excl, _ := opts["publicKey"].(map[string]any)["excludeCredentials"].([]any); len(excl) != 2 {
		t.Fatalf("excludeCredentials = %v", excl)
	}
	r := e.expect(http.StatusBadRequest, "POST", "/api/occ/passkeys/register/finish", alice.token, map[string]any{"credential": a.create(opts)})
	if !strings.Contains(string(r.body), "déjà enregistrée") {
		t.Fatalf("duplicate: %s", r.body)
	}

	var list []map[string]any
	e.expect(http.StatusOK, "GET", "/api/occ/passkeys", alice.token, nil).json(t, &list)
	if len(list) != 2 {
		t.Fatalf("list = %v", list)
	}

	// usernameless login → PocketBase auth response
	a.counter = 1
	res := e.passkeyLogin(a)
	if res.status != http.StatusOK {
		t.Fatalf("login: %d %s", res.status, res.body)
	}
	var auth struct {
		Token  string         `json:"token"`
		Record map[string]any `json:"record"`
	}
	res.json(t, &auth)
	if auth.Token == "" || auth.Record["id"] != alice.id() {
		t.Fatalf("auth = %s", res.body)
	}
	e.expect(http.StatusOK, "GET", "/api/occ/me/account", auth.Token, nil)

	rec, err := e.app.FindFirstRecordByData(colPasskeys, "credential_id", b64t.EncodeToString(a.credID))
	if err != nil {
		t.Fatal(err)
	}
	if rec.GetInt("sign_count") != 1 || rec.GetDateTime("last_used_at").IsZero() {
		t.Fatalf("sign_count=%d last_used_at=%v", rec.GetInt("sign_count"), rec.GetDateTime("last_used_at"))
	}

	// sign counter: must increase; a regression (cloned key) is refused
	a.counter = 5
	if res := e.passkeyLogin(a); res.status != http.StatusOK {
		t.Fatalf("login 2: %d %s", res.status, res.body)
	}
	a.counter = 3
	res = e.passkeyLogin(a)
	if res.status != http.StatusBadRequest || !strings.Contains(string(res.body), "compteur") {
		t.Fatalf("cloned: %d %s", res.status, res.body)
	}
	rec, _ = e.app.FindRecordById(colPasskeys, rec.Id)
	if rec.GetInt("sign_count") != 5 {
		t.Fatalf("sign_count after refusal = %d", rec.GetInt("sign_count"))
	}
	// authenticators without a counter (always 0) keep working
	if res := e.passkeyLogin(b); res.status != http.StatusOK {
		t.Fatalf("login counter 0: %d %s", res.status, res.body)
	}
	if res := e.passkeyLogin(b); res.status != http.StatusOK {
		t.Fatalf("login counter 0 again: %d %s", res.status, res.body)
	}
}

func TestPasskeyReplayAndOrigin(t *testing.T) {
	e := newEnv(t)
	bob := e.user("Bob")
	a := newSoftAuthenticator(t)

	// replayed registration
	opts := e.expect(http.StatusOK, "POST", "/api/occ/passkeys/register/begin", bob.token, nil).m(t)
	cred := a.create(opts)
	e.expect(http.StatusOK, "POST", "/api/occ/passkeys/register/finish", bob.token, map[string]any{"credential": cred})
	r := e.expect(http.StatusBadRequest, "POST", "/api/occ/passkeys/register/finish", bob.token, map[string]any{"credential": cred})
	if !strings.Contains(string(r.body), "expiré") {
		t.Fatalf("replay: %s", r.body)
	}

	// replayed assertion
	a.counter = 1
	lopts := e.expect(http.StatusOK, "POST", "/api/occ/passkeys/login/begin", "", nil).m(t)
	assertion := a.get(lopts)
	e.expect(http.StatusOK, "POST", "/api/occ/passkeys/login/finish", "", map[string]any{"credential": assertion})
	r = e.expect(http.StatusBadRequest, "POST", "/api/occ/passkeys/login/finish", "", map[string]any{"credential": assertion})
	if !strings.Contains(string(r.body), "expiré") {
		t.Fatalf("login replay: %s", r.body)
	}

	// a login challenge cannot finish a registration (and vice versa)
	lopts = e.expect(http.StatusOK, "POST", "/api/occ/passkeys/login/begin", "", nil).m(t)
	other := newSoftAuthenticator(t)
	ropts := map[string]any{"publicKey": map[string]any{"challenge": lopts["publicKey"].(map[string]any)["challenge"],
		"user": map[string]any{"id": b64t.EncodeToString([]byte(bob.id()))}}}
	e.expect(http.StatusBadRequest, "POST", "/api/occ/passkeys/register/finish", bob.token, map[string]any{"credential": other.create(ropts)})

	// wrong origin (phishing site) — registration and login
	evil := newSoftAuthenticator(t)
	evil.origin = "https://evil.example"
	opts = e.expect(http.StatusOK, "POST", "/api/occ/passkeys/register/begin", bob.token, nil).m(t)
	e.expect(http.StatusBadRequest, "POST", "/api/occ/passkeys/register/finish", bob.token, map[string]any{"credential": evil.create(opts)})
	a.origin = "https://evil.example"
	a.counter = 9
	if res := e.passkeyLogin(a); res.status != http.StatusBadRequest {
		t.Fatalf("wrong origin login: %d %s", res.status, res.body)
	}
	// wrong RP ID
	a.origin, a.rpID = "http://localhost:8090", "example.com"
	if res := e.passkeyLogin(a); res.status != http.StatusBadRequest {
		t.Fatalf("wrong rp id login: %d %s", res.status, res.body)
	}
	// another localhost port (dev server, e2e) is accepted when it is the request origin
	a.rpID, a.origin, a.counter = "localhost", "http://localhost:8102", 10
	lopts = e.expect(http.StatusOK, "POST", "/api/occ/passkeys/login/begin", "", nil).m(t)
	body, _ := json.Marshal(map[string]any{"credential": a.get(lopts)})
	req := httptest.NewRequest("POST", "/api/occ/passkeys/login/finish", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:8102")
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dev origin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPasskeyBannedGuestAndDelete(t *testing.T) {
	e := newEnv(t)
	carol := e.user("Carol")
	dave := e.user("Dave")
	a := newSoftAuthenticator(t)
	view := e.registerPasskey(carol, a, "Pixel")
	id := view["id"].(string)

	// unauthenticated management
	e.expect(http.StatusUnauthorized, "POST", "/api/occ/passkeys/register/begin", "", nil)
	e.expect(http.StatusUnauthorized, "GET", "/api/occ/passkeys", "", nil)
	// the collection itself is closed
	e.expect(http.StatusForbidden, "GET", "/api/collections/passkeys/records", carol.token, nil)

	// rename / delete: owner only
	e.expect(http.StatusNotFound, "PATCH", "/api/occ/passkeys/"+id, dave.token, map[string]any{"name": "x"})
	e.expect(http.StatusNotFound, "DELETE", "/api/occ/passkeys/"+id, dave.token, nil)
	e.expect(http.StatusBadRequest, "PATCH", "/api/occ/passkeys/"+id, carol.token, map[string]any{"name": "  "})
	if v := e.expect(http.StatusOK, "PATCH", "/api/occ/passkeys/"+id, carol.token, map[string]any{"name": "Pixel 8"}).m(t); v["name"] != "Pixel 8" {
		t.Fatalf("rename = %v", v)
	}

	// banned → 403 « Compte suspendu »
	u, _ := e.app.FindRecordById(colUsers, carol.id())
	u.Set(fieldBanned, true)
	if err := e.app.Save(u); err != nil {
		t.Fatal(err)
	}
	a.counter = 1
	res := e.passkeyLogin(a)
	if res.status != http.StatusForbidden || !strings.Contains(string(res.body), "suspendu") {
		t.Fatalf("banned: %d %s", res.status, res.body)
	}
	u.Set(fieldBanned, false)
	if err := e.app.Save(u); err != nil {
		t.Fatal(err)
	}
	tok, _ := u.NewAuthToken()

	// delete → the passkey cannot sign in anymore
	e.expect(http.StatusNoContent, "DELETE", "/api/occ/passkeys/"+id, tok, nil)
	a.counter = 2
	res = e.passkeyLogin(a)
	if res.status != http.StatusBadRequest || !strings.Contains(string(res.body), "plus liée") {
		t.Fatalf("deleted: %d %s", res.status, res.body)
	}

	// guests cannot register a passkey
	guest := e.user("Guest")
	guest.rec.Set(fieldIsGuest, true)
	if err := e.app.Save(guest.rec); err != nil {
		t.Fatal(err)
	}
	r := e.expect(http.StatusForbidden, "POST", "/api/occ/passkeys/register/begin", guest.token, nil)
	if !strings.Contains(string(r.body), "Crée ton compte") {
		t.Fatalf("guest: %s", r.body)
	}

	// account deletion (anonymisation) removes the passkeys
	e.registerPasskey(dave, newSoftAuthenticator(t), "")
	d, _ := e.app.FindRecordById(colUsers, dave.id())
	if err := anonymizeUser(e.app, d); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.app.CountRecords(colPasskeys, dbx.HashExp{"user": dave.id()}); n != 0 {
		t.Fatalf("passkeys left after deletion: %d", n)
	}
}

func TestPasskeyLoginRateLimitAndConfig(t *testing.T) {
	e := newEnv(t)
	if cfg := e.expect(http.StatusOK, "GET", "/api/occ/config", "", nil).m(t); cfg["passkeys"] != true {
		t.Fatalf("config passkeys = %v", cfg["passkeys"])
	}
	opts := e.expect(http.StatusOK, "POST", "/api/occ/passkeys/login/begin", "", map[string]any{"conditional": true}).m(t)
	if opts["mediation"] != "conditional" {
		t.Fatalf("mediation = %v", opts["mediation"])
	}
	if _, has := opts["publicKey"].(map[string]any)["allowCredentials"]; has {
		t.Fatalf("discoverable login must not list credentials: %v", opts)
	}
	for i := 1; i < passkeyLoginPerMin; i++ {
		e.expect(http.StatusOK, "POST", "/api/occ/passkeys/login/begin", "", nil)
	}
	e.expect(http.StatusTooManyRequests, "POST", "/api/occ/passkeys/login/begin", "", nil)

	// invalid relying party (http outside localhost) → passkeys disabled
	cfg := testConfig
	cfg.PublicURL = "http://eat.example.org"
	off := newEnvWith(t, cfg, func(*tests.TestApp) {})
	if c := off.expect(http.StatusOK, "GET", "/api/occ/config", "", nil).m(t); c["passkeys"] != false {
		t.Fatalf("config passkeys = %v", c["passkeys"])
	}
	off.expect(http.StatusServiceUnavailable, "POST", "/api/occ/passkeys/login/begin", "", nil)

	// explicit relying party
	cfg.WebAuthn = WebAuthnConfig{RPID: "example.org", Origins: []string{"https://eat.example.org"}}
	on := newEnvWith(t, cfg, nil)
	lopts := on.expect(http.StatusOK, "POST", "/api/occ/passkeys/login/begin", "", nil).m(t)
	if lopts["publicKey"].(map[string]any)["rpId"] != "example.org" {
		t.Fatalf("rpId = %v", lopts["publicKey"])
	}
}
