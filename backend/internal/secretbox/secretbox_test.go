package secretbox

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, KeySize) }

func mustRing(t *testing.T, current []byte, old ...[]byte) *Keyring {
	t.Helper()
	k, err := New(current, old...)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestParseKey(t *testing.T) {
	raw := testKey(7)
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"std padded", base64.StdEncoding.EncodeToString(raw), true},
		{"std raw", base64.RawStdEncoding.EncodeToString(raw), true},
		{"url", base64.URLEncoding.EncodeToString(raw), true},
		{"spaces", "  " + base64.StdEncoding.EncodeToString(raw) + "\n", true},
		{"too short", base64.StdEncoding.EncodeToString(raw[:16]), false},
		{"too long", base64.StdEncoding.EncodeToString(append(raw, 1)), false},
		{"not base64", "pas une clé !", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseKey(c.in)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v, want ok=%v", err, c.ok)
			}
			if c.ok && !bytes.Equal(got, raw) {
				t.Fatal("decoded key differs")
			}
		})
	}
}

func TestSealOpen(t *testing.T) {
	cur := mustRing(t, testKey(1))
	other := mustRing(t, testKey(2))
	rotated := mustRing(t, testKey(2), testKey(1)) // new current 2, old 1
	none := mustRing(t, nil)
	var nilRing *Keyring

	sealed, err := cur.Seal("BE71096123456769", "payout_profiles.iban:u1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "enc:v1:"+cur.CurrentID()+":") || strings.Contains(sealed, "BE71") {
		t.Fatalf("unexpected envelope %q", sealed)
	}
	again, _ := cur.Seal("BE71096123456769", "payout_profiles.iban:u1")
	if again == sealed {
		t.Fatal("nonce must be random")
	}
	// flip one byte of the payload
	parts := strings.SplitN(sealed, ":", 4)
	payload, _ := base64.RawURLEncoding.DecodeString(parts[3])
	payload[len(payload)-1] ^= 1
	tampered := strings.Join(parts[:3], ":") + ":" + base64.RawURLEncoding.EncodeToString(payload)

	cases := []struct {
		name      string
		ring      *Keyring
		stored    string
		aad       string
		want      string
		wantStale bool
		wantErr   error
	}{
		{"round trip", cur, sealed, "payout_profiles.iban:u1", "BE71096123456769", false, nil},
		{"wrong aad (other owner)", cur, sealed, "payout_profiles.iban:u2", "", false, ErrTampered},
		{"wrong aad (other column)", cur, sealed, "payout_profiles.bic:u1", "", false, ErrTampered},
		{"tampered", cur, tampered, "payout_profiles.iban:u1", "", false, ErrTampered},
		{"wrong key", other, sealed, "payout_profiles.iban:u1", "", false, ErrUnknownKey},
		{"no key", none, sealed, "payout_profiles.iban:u1", "", false, ErrUnknownKey},
		{"nil keyring", nilRing, sealed, "payout_profiles.iban:u1", "", false, ErrUnknownKey},
		{"rotation: old key opens, stale", rotated, sealed, "payout_profiles.iban:u1", "BE71096123456769", true, nil},
		{"legacy plaintext with key: stale", cur, "BE71096123456769", "x", "BE71096123456769", true, nil},
		{"legacy plaintext without key", none, "BE71096123456769", "x", "BE71096123456769", false, nil},
		{"legacy plaintext nil keyring", nilRing, "Bob", "x", "Bob", false, nil},
		{"empty", cur, "", "x", "", false, nil},
		{"malformed version", cur, "enc:v9:abcdef12:AAAA", "x", "", false, ErrMalformed},
		{"malformed kid", cur, "enc:v1:abc:AAAA", "x", "", false, ErrMalformed},
		{"malformed base64", cur, "enc:v1:" + cur.CurrentID() + ":***", "x", "", false, ErrMalformed},
		{"truncated", cur, "enc:v1:" + cur.CurrentID() + ":AAAA", "x", "", false, ErrMalformed},
		{"prefix only", cur, "enc:", "x", "", false, ErrMalformed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, stale, err := c.ring.Open(c.stored, c.aad)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if got != c.want || stale != c.wantStale {
				t.Fatalf("got (%q, %v), want (%q, %v)", got, stale, c.want, c.wantStale)
			}
		})
	}
}

func TestSealEdgeCases(t *testing.T) {
	cur := mustRing(t, testKey(1))
	cases := []struct {
		name    string
		ring    *Keyring
		in      string
		want    string // "" + sealed=true means "an envelope"
		sealed  bool
		wantErr error
	}{
		{"empty stays empty", cur, "", "", false, nil},
		{"no key keeps plaintext", mustRing(t, nil, testKey(1)), "Bob", "Bob", false, nil},
		{"nil keyring keeps plaintext", nil, "Bob", "Bob", false, nil},
		{"already sealed", cur, "enc:v1:abcdef12:xyz", "", false, ErrAlreadySealed},
		{"unicode", cur, "Zoé Ünïcødé 🍕", "", true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.ring.Seal(c.in, "aad")
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if c.sealed {
				if !IsSealed(got) {
					t.Fatalf("expected an envelope, got %q", got)
				}
				back, stale, err := c.ring.Open(got, "aad")
				if err != nil || stale || back != c.in {
					t.Fatalf("round trip: %q %v %v", back, stale, err)
				}
				return
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestFromEnv(t *testing.T) {
	k1 := base64.StdEncoding.EncodeToString(testKey(1))
	k2 := base64.StdEncoding.EncodeToString(testKey(2))
	cases := []struct {
		name    string
		env     map[string]string
		enabled bool
		hasKeys bool
		ok      bool
	}{
		{"unset", nil, false, false, true},
		{"current", map[string]string{EnvKey: k1}, true, true, true},
		{"current + old list", map[string]string{EnvKey: k2, EnvOldKeys: k1 + " , " + k2 + ","}, true, true, true},
		{"old only", map[string]string{EnvOldKeys: k1}, false, true, true},
		{"invalid current", map[string]string{EnvKey: "court"}, false, false, false},
		{"invalid old", map[string]string{EnvKey: k1, EnvOldKeys: "court"}, false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			k, err := FromEnv(func(s string) string { return c.env[s] })
			if (err == nil) != c.ok {
				t.Fatalf("err = %v", err)
			}
			if err != nil {
				if strings.Contains(err.Error(), "court") {
					t.Fatal("error must not echo the key value")
				}
				return
			}
			if k.Enabled() != c.enabled || k.HasKeys() != c.hasKeys {
				t.Fatalf("enabled=%v hasKeys=%v", k.Enabled(), k.HasKeys())
			}
		})
	}
}

func TestKeyIDOf(t *testing.T) {
	cur := mustRing(t, testKey(1))
	s, _ := cur.Seal("x", "")
	if KeyIDOf(s) != cur.CurrentID() || KeyID(testKey(1)) != cur.CurrentID() {
		t.Fatal("key id mismatch")
	}
	if KeyIDOf("plain") != "" || KeyIDOf("enc:v1:zz") != "" {
		t.Fatal("non envelopes have no key id")
	}
}
