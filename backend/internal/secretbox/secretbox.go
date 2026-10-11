// Package secretbox encrypts short secrets at rest (payout data: IBAN,
// account holder, BIC) with AES-256-GCM. Pure Go, no PocketBase import.
//
// Envelope (a printable string, stored in a text column):
//
//	enc:v1:<kid>:<base64url(nonce ‖ ciphertext ‖ tag)>
//
// v1 = AES-256-GCM, random 96-bit nonce, 128-bit tag. kid = first 8 hex
// digits of SHA-256("occ-data-key-id\x00" ‖ key): the key that sealed the
// value, so that values sealed with an older key are still readable
// (rotation) and recognised as stale. The additional data (AAD) binds the
// value to its column and owner: a ciphertext copied into another row or
// column does not open.
//
// A value without the "enc:" prefix is legacy plaintext and is returned as
// is by Open (and reported stale when a current key is configured).
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// Prefix marks a sealed value.
	Prefix = "enc:"
	// version1 is the only envelope format so far.
	version1 = "v1"
	// KeySize is the key length in bytes (AES-256).
	KeySize = 32
	// EnvKey / EnvOldKeys are the environment variables read by FromEnv.
	EnvKey     = "OCC_DATA_KEY"
	EnvOldKeys = "OCC_DATA_KEY_OLD"
)

var (
	// ErrUnknownKey: the value was sealed with a key that is not configured
	// (missing OCC_DATA_KEY, or the old key removed too early).
	ErrUnknownKey = errors.New("secretbox: value sealed with an unknown key")
	// ErrTampered: authentication failed (modified value, wrong AAD, wrong key).
	ErrTampered = errors.New("secretbox: authentication failed")
	// ErrMalformed: "enc:" prefix but not a valid envelope.
	ErrMalformed = errors.New("secretbox: malformed envelope")
	// ErrAlreadySealed: Seal called on an already sealed value.
	ErrAlreadySealed = errors.New("secretbox: value already sealed")
)

type key struct {
	id   string
	aead cipher.AEAD
}

// Keyring holds the current key (seals and opens) and older keys (open
// only, during a rotation). The zero value and nil are valid: no key,
// values are kept in plaintext.
type Keyring struct {
	current *key
	byID    map[string]*key
}

// ParseKey decodes a base64 key (standard or URL alphabet, padding
// optional) of exactly 32 bytes (`openssl rand -base64 32`).
func ParseKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			if len(b) != KeySize {
				return nil, fmt.Errorf("secretbox: key must be %d bytes, got %d", KeySize, len(b))
			}
			return b, nil
		}
	}
	return nil, errors.New("secretbox: key is not valid base64")
}

func newKey(raw []byte) (*key, error) {
	if len(raw) != KeySize {
		return nil, fmt.Errorf("secretbox: key must be %d bytes, got %d", KeySize, len(raw))
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &key{id: KeyID(raw), aead: aead}, nil
}

// KeyID is the public identifier of a key (8 hex digits, not secret).
func KeyID(raw []byte) string {
	h := sha256.Sum256(append([]byte("occ-data-key-id\x00"), raw...))
	return hex.EncodeToString(h[:4])
}

// New builds a keyring. current may be nil (no encryption: Seal keeps
// plaintext, Open still reads values sealed with one of the old keys).
func New(current []byte, old ...[]byte) (*Keyring, error) {
	k := &Keyring{byID: map[string]*key{}}
	if current != nil {
		c, err := newKey(current)
		if err != nil {
			return nil, err
		}
		k.current = c
		k.byID[c.id] = c
	}
	for _, raw := range old {
		o, err := newKey(raw)
		if err != nil {
			return nil, err
		}
		if _, dup := k.byID[o.id]; !dup {
			k.byID[o.id] = o
		}
	}
	return k, nil
}

// FromEnv reads OCC_DATA_KEY (current key) and OCC_DATA_KEY_OLD (older
// keys, comma separated, decrypt only). Unset = empty keyring, no error; a
// set but invalid value is an error (never silently ignored).
func FromEnv(getenv func(string) string) (*Keyring, error) {
	var current []byte
	if v := strings.TrimSpace(getenv(EnvKey)); v != "" {
		b, err := ParseKey(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", EnvKey, err)
		}
		current = b
	}
	var old [][]byte
	for _, v := range strings.Split(getenv(EnvOldKeys), ",") {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		b, err := ParseKey(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", EnvOldKeys, err)
		}
		old = append(old, b)
	}
	return New(current, old...)
}

// Enabled reports whether a current key is configured (values get sealed).
func (k *Keyring) Enabled() bool { return k != nil && k.current != nil }

// CurrentID is the id of the current key ("" without key).
func (k *Keyring) CurrentID() string {
	if !k.Enabled() {
		return ""
	}
	return k.current.id
}

// HasKeys reports whether at least one key (current or old) is configured.
func (k *Keyring) HasKeys() bool { return k != nil && len(k.byID) > 0 }

// IsSealed reports whether s is an envelope (prefix check only).
func IsSealed(s string) bool { return strings.HasPrefix(s, Prefix) }

// KeyIDOf returns the key id of an envelope ("" if s is not sealed or malformed).
func KeyIDOf(s string) string {
	_, kid, _, err := split(s)
	if err != nil {
		return ""
	}
	return kid
}

// Seal encrypts plain with the current key. "" stays "". Without a current
// key the plaintext is returned unchanged.
func (k *Keyring) Seal(plain, aad string) (string, error) {
	if plain == "" || !k.Enabled() {
		return plain, nil
	}
	if IsSealed(plain) {
		return "", ErrAlreadySealed
	}
	nonce := make([]byte, k.current.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := k.current.aead.Seal(nonce, nonce, []byte(plain), []byte(aad))
	return Prefix + version1 + ":" + k.current.id + ":" + base64.RawURLEncoding.EncodeToString(out), nil
}

// Open returns the plaintext of a stored value. Plaintext (legacy) values
// are returned unchanged. stale reports that the value should be sealed
// again with the current key (plaintext, or sealed with an older key);
// it is always false without a current key.
func (k *Keyring) Open(stored, aad string) (plain string, stale bool, err error) {
	if !IsSealed(stored) {
		return stored, stored != "" && k.Enabled(), nil
	}
	_, kid, payload, err := split(stored)
	if err != nil {
		return "", false, err
	}
	if k == nil {
		return "", false, ErrUnknownKey
	}
	key, found := k.byID[kid]
	if !found {
		return "", false, ErrUnknownKey
	}
	ns := key.aead.NonceSize()
	if len(payload) < ns+key.aead.Overhead() {
		return "", false, ErrMalformed
	}
	out, err := key.aead.Open(nil, payload[:ns], payload[ns:], []byte(aad))
	if err != nil {
		return "", false, ErrTampered
	}
	return string(out), k.Enabled() && kid != k.current.id, nil
}

func split(s string) (version, kid string, payload []byte, err error) {
	if !IsSealed(s) {
		return "", "", nil, ErrMalformed
	}
	parts := strings.SplitN(strings.TrimPrefix(s, Prefix), ":", 3)
	if len(parts) != 3 || parts[0] != version1 || len(parts[1]) != 8 {
		return "", "", nil, ErrMalformed
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", nil, ErrMalformed
	}
	return parts[0], parts[1], b, nil
}
