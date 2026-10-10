package domain

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Passkeys (WebAuthn): pure helpers — relying party resolution, passkey
// names and the single-use challenge store. The ceremonies themselves are
// verified by github.com/go-webauthn/webauthn in internal/app/passkeys.go.

// PasskeyNameMax is the maximum length (runes) of a passkey name.
const PasskeyNameMax = 60

// DefaultPasskeyName is used when neither the user nor the AAGUID gives one.
const DefaultPasskeyName = "Passkey"

// RelyingParty is the WebAuthn relying party: its ID (a registrable domain,
// "eat.fs0ciety.org") and the exact origins allowed in the client data.
type RelyingParty struct {
	ID      string
	Origins []string
}

// Dev servers accepted when the relying party is localhost (Vite, vite
// preview, the Go server).
var localhostDevOrigins = []string{"http://localhost:5173", "http://localhost:4173", "http://localhost:8090"}

// ResolveRelyingParty derives the relying party from the public URL
// (OCC_PUBLIC_URL), overridden by rpID (OCC_WEBAUTHN_RP_ID) and origins
// (OCC_WEBAUTHN_ORIGINS). Every origin must be http(s), https unless on
// localhost, and its host must be the RP ID or one of its subdomains.
func ResolveRelyingParty(publicURL, rpID string, origins []string) (RelyingParty, error) {
	pub, err := parseOrigin(publicURL)
	if err != nil && (rpID == "" || len(origins) == 0) {
		return RelyingParty{}, fmt.Errorf("OCC_PUBLIC_URL: %w", err)
	}
	rp := RelyingParty{ID: strings.ToLower(strings.TrimSpace(rpID))}
	if rp.ID == "" {
		rp.ID = strings.ToLower(pub.Hostname())
	}
	if rp.ID == "" || strings.ContainsAny(rp.ID, ":/ ") || net.ParseIP(rp.ID) != nil {
		return RelyingParty{}, fmt.Errorf("RP ID invalide %q (un nom de domaine, sans schéma ni port)", rp.ID)
	}
	add := func(o string) {
		for _, x := range rp.Origins {
			if x == o {
				return
			}
		}
		rp.Origins = append(rp.Origins, o)
	}
	if len(origins) == 0 {
		add(pub.Scheme + "://" + strings.ToLower(pub.Host))
		if rp.ID == "localhost" {
			for _, o := range localhostDevOrigins {
				add(o)
			}
		}
	}
	for _, raw := range origins {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		u, err := parseOrigin(raw)
		if err != nil {
			return RelyingParty{}, fmt.Errorf("OCC_WEBAUTHN_ORIGINS: %w", err)
		}
		add(u.Scheme + "://" + strings.ToLower(u.Host))
	}
	for _, o := range rp.Origins {
		u, _ := url.Parse(o)
		host := u.Hostname()
		if host != rp.ID && !strings.HasSuffix(host, "."+rp.ID) {
			return RelyingParty{}, fmt.Errorf("origine %s hors du domaine %s", o, rp.ID)
		}
		if u.Scheme != "https" && host != "localhost" {
			return RelyingParty{}, fmt.Errorf("origine %s : https obligatoire (sauf localhost)", o)
		}
	}
	return rp, nil
}

func parseOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, fmt.Errorf("URL invalide %q", raw)
	}
	return u, nil
}

// OriginsFor returns the origins accepted for a ceremony started from
// requestOrigin (the Origin header): the configured ones, plus any
// http://localhost:<port> when the RP is localhost (dev servers, e2e).
func (rp RelyingParty) OriginsFor(requestOrigin string) []string {
	if rp.ID != "localhost" || requestOrigin == "" {
		return rp.Origins
	}
	u, err := url.Parse(requestOrigin)
	if err != nil || u.Hostname() != "localhost" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" {
		return rp.Origins
	}
	o := u.Scheme + "://" + strings.ToLower(u.Host)
	for _, x := range rp.Origins {
		if x == o {
			return rp.Origins
		}
	}
	return append(append([]string{}, rp.Origins...), o)
}

// knownAuthenticators maps the AAGUIDs of common passkey providers to a
// default name (https://github.com/passkeydeveloper/passkey-authenticator-aaguids).
var knownAuthenticators = map[string]string{
	"fbfc3007-154e-4ecc-8c0b-6e020557d7bd": "Trousseau iCloud",
	"dd4ec289-e01d-41c9-bb89-70fa845d4bf2": "Trousseau iCloud",
	"ea9b8d66-4d01-1d21-3ce4-b6b48cb575d4": "Gestionnaire de mots de passe Google",
	"adce0002-35bc-c60a-648b-0b25f1f05503": "Chrome sur Mac",
	"08987058-cadc-4b81-b6e1-30de50dcbe96": "Windows Hello",
	"9ddd1817-af5a-4672-a2b9-3e3dd95000a9": "Windows Hello",
	"6028b017-b1d4-4c02-b4b3-afcdafc96bb2": "Windows Hello",
	"bada5566-a7aa-401f-bd96-45619a55120d": "1Password",
	"d548826e-79b4-db40-a3d8-11116f7e8349": "Bitwarden",
	"53414d53-554e-4700-0000-000000000000": "Samsung Pass",
	"531126d6-e717-415c-9320-3d9aa6981239": "Dashlane",
	"b84e4048-15dc-4dd0-8640-f4f60813c8af": "NordPass",
	"0ea242b4-43c4-4a1b-8b17-dd6d0b6baec6": "Keeper",
	"f3809540-7f14-49c1-a8b3-8f813b225541": "Enpass",
	"771b48fd-d3d4-4f74-9232-fc157ab0507a": "Edge sur Mac",
}

// PasskeyName normalises a user-given name (trimmed, inner spaces collapsed,
// at most PasskeyNameMax runes). Empty → the provider name of the AAGUID,
// else DefaultPasskeyName.
func PasskeyName(name, aaguid string) string {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		if n, ok := knownAuthenticators[strings.ToLower(aaguid)]; ok {
			return n
		}
		return DefaultPasskeyName
	}
	if utf8.RuneCountInString(name) > PasskeyNameMax {
		name = strings.TrimSpace(string([]rune(name)[:PasskeyNameMax]))
	}
	return name
}

// ValidatePasskeyRename checks a rename: the name is required.
func ValidatePasskeyRename(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", Errf("Donne un nom à ta passkey.")
	}
	return PasskeyName(name, ""), nil
}

// ChallengeStore keeps WebAuthn ceremony sessions server side, keyed by
// their challenge: short lived (TTL) and single use (Take removes the entry,
// even an expired one). Bounded: when full, expired entries are dropped,
// then the oldest. Safe for concurrent use; the clock is passed in.
type ChallengeStore[T any] struct {
	TTL time.Duration
	Max int

	mu    sync.Mutex
	items map[string]challengeEntry[T]
}

type challengeEntry[T any] struct {
	value   T
	created time.Time
}

// NewChallengeStore returns a store keeping at most max entries for ttl.
func NewChallengeStore[T any](ttl time.Duration, max int) *ChallengeStore[T] {
	return &ChallengeStore[T]{TTL: ttl, Max: max, items: map[string]challengeEntry[T]{}}
}

// Put stores value under key (an existing entry is replaced).
func (s *ChallengeStore[T]) Put(key string, value T, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]challengeEntry[T]{}
	}
	if _, exists := s.items[key]; !exists && s.Max > 0 && len(s.items) >= s.Max {
		s.gc(now)
		for len(s.items) >= s.Max {
			oldest, first := "", true
			var at time.Time
			for k, v := range s.items {
				if first || v.created.Before(at) {
					oldest, at, first = k, v.created, false
				}
			}
			delete(s.items, oldest)
		}
	}
	s.items[key] = challengeEntry[T]{value: value, created: now}
}

// Take returns and removes the value stored under key; false when missing,
// already taken or expired.
func (s *ChallengeStore[T]) Take(key string, now time.Time) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var zero T
	it, ok := s.items[key]
	if !ok {
		return zero, false
	}
	delete(s.items, key)
	if now.Sub(it.created) >= s.TTL {
		return zero, false
	}
	return it.value, true
}

// Len returns the number of stored entries (expired ones included).
func (s *ChallengeStore[T]) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func (s *ChallengeStore[T]) gc(now time.Time) {
	for k, v := range s.items {
		if now.Sub(v.created) >= s.TTL {
			delete(s.items, k)
		}
	}
}
