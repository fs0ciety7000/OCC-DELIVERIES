package domain

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Payout handles: structured payment identifiers of the payer
// (payout_profiles.revolut_tag / paypal_me / payment_link) and the
// per-payment links built from them with the exact amount.
//
// Only formats confirmed by the provider are prefilled (sources in
// docs/adr/0003-payments.md):
//   - PayPal.me: https://paypal.me/<name>/<amount><CUR> (PayPal help center);
//   - Revolut:   https://revolut.me/<revtag>?amount=<minor units>&currency=EUR&note=…
//     (parameters read by the revolut.me page itself; amount in cents);
//   - Wise Business open link: https://wise.com/pay/business/<name>?amount=…&currency=…&description=…
//     (Wise help center).
// Wero, Bancontact Pay, Lydia/Sumeria and personal Wisetags expose no
// third-party request format: never invented here.

// Link kinds (= payment methods of the matching tiles).
const (
	LinkRevolut = MethodRevolut
	LinkPayPal  = MethodPayPal
	LinkGeneric = MethodLink
)

// PaymentLink is a payment link of the payer, built for one payment.
type PaymentLink struct {
	Kind            string `json:"kind"`
	Label           string `json:"label"`
	URL             string `json:"url"`
	AmountPrefilled bool   `json:"amountPrefilled"`
}

// PayoutHandles are the normalized link handles of a payout profile.
type PayoutHandles struct {
	RevolutTag string
	PayPalMe   string
	Link       string
}

var (
	revtagRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,31}$`)
	paypalMeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)
)

const maxPaymentLink = 500

// parseLoose parses a user-typed URL, adding https:// when the scheme is
// missing ("revolut.me/bob"). ok=false when it does not look like a URL.
func parseLoose(s string) (*url.URL, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t\r\n") {
		return nil, false
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.User != nil || !strings.Contains(u.Host, ".") {
		return nil, false
	}
	return u, true
}

func hostOf(u *url.URL) string {
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func pathSegments(u *url.URL) []string {
	out := []string{}
	for _, s := range strings.Split(u.Path, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// looksLikeURL reports whether the input is a URL rather than a bare handle.
func looksLikeURL(s string) bool {
	return strings.Contains(s, "/") || strings.Contains(s, ".me") || strings.Contains(s, ".com")
}

// revtagFromURL extracts the revtag of a revolut.me link ("" if not one).
func revtagFromURL(u *url.URL) string {
	if hostOf(u) != "revolut.me" {
		return ""
	}
	segs := pathSegments(u)
	if len(segs) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(segs[0], "@"))
}

// paypalFromURL extracts the PayPal.me name of a paypal.me or
// paypal.com/paypalme link ("" if not one).
func paypalFromURL(u *url.URL) string {
	segs := pathSegments(u)
	switch hostOf(u) {
	case "paypal.me":
		if len(segs) > 0 {
			return strings.TrimPrefix(segs[0], "@")
		}
	case "paypal.com":
		if len(segs) > 1 && strings.EqualFold(segs[0], "paypalme") {
			return segs[1]
		}
	}
	return ""
}

// NormalizeRevolutTag accepts "@bob", "bob", "revolut.me/bob" or
// "https://revolut.me/bob/…" and returns the lowercase revtag ("bob").
func NormalizeRevolutTag(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	tag := s
	if looksLikeURL(s) {
		u, ok := parseLoose(s)
		if !ok {
			return "", errRevtag()
		}
		if tag = revtagFromURL(u); tag == "" {
			return "", errRevtag()
		}
	}
	tag = strings.ToLower(strings.TrimPrefix(tag, "@"))
	if !revtagRe.MatchString(tag) {
		return "", errRevtag()
	}
	return tag, nil
}

func errRevtag() error {
	return Errf("Revtag Revolut invalide (ex. @jdoe ou https://revolut.me/jdoe).")
}

// NormalizePayPalMe accepts "bob", "@bob", "paypal.me/bob" or
// "https://www.paypal.com/paypalme/bob" and returns the PayPal.me name.
func NormalizePayPalMe(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	name := s
	if looksLikeURL(s) {
		u, ok := parseLoose(s)
		if !ok {
			return "", errPayPal()
		}
		if name = paypalFromURL(u); name == "" {
			return "", errPayPal()
		}
	}
	name = strings.TrimPrefix(name, "@")
	if !paypalMeRe.MatchString(name) {
		return "", errPayPal()
	}
	return name, nil
}

func errPayPal() error {
	return Errf("Nom PayPal.me invalide (ex. jdoe ou https://paypal.me/jdoe).")
}

// NormalizePaymentLink normalizes a free payment link: https:// added when
// missing, http upgraded, only web links (no javascript:, mailto:…).
func NormalizePaymentLink(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	u, ok := parseLoose(s)
	if !ok || (u.Scheme != "https" && u.Scheme != "http") || len(s) > maxPaymentLink {
		return "", Errf("Lien de paiement invalide (ex. https://paypal.me/jdoe).")
	}
	u.Scheme = "https"
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// ClassifyPaymentLink recognizes a revolut.me or PayPal.me link and returns
// its kind and handle; any other link is LinkGeneric with an empty handle.
func ClassifyPaymentLink(link string) (kind, handle string) {
	u, ok := parseLoose(link)
	if !ok {
		return LinkGeneric, ""
	}
	if tag := revtagFromURL(u); tag != "" && revtagRe.MatchString(tag) {
		return LinkRevolut, tag
	}
	if name := paypalFromURL(u); name != "" && paypalMeRe.MatchString(name) {
		return LinkPayPal, name
	}
	return LinkGeneric, ""
}

// SplitPayoutLink moves a recognizable payment link (revolut.me/…,
// paypal.me/…) into its structured field when that field is empty or
// already holds the same handle. Inputs must be normalized.
func SplitPayoutLink(h PayoutHandles) PayoutHandles {
	kind, handle := ClassifyPaymentLink(h.Link)
	switch kind {
	case LinkRevolut:
		if h.RevolutTag == "" || h.RevolutTag == handle {
			h.RevolutTag, h.Link = handle, ""
		}
	case LinkPayPal:
		if h.PayPalMe == "" || strings.EqualFold(h.PayPalMe, handle) {
			if h.PayPalMe == "" {
				h.PayPalMe = handle
			}
			h.Link = ""
		}
	}
	return h
}

// RevolutPaymentLink builds the revolut.me link with the amount (cents),
// the currency and the note prefilled.
func RevolutPaymentLink(tag string, amount int, note string) string {
	u := url.URL{Scheme: "https", Host: "revolut.me", Path: "/" + tag}
	if amount > 0 {
		q := url.Values{}
		q.Set("amount", strconv.Itoa(amount))
		q.Set("currency", "EUR")
		if note = cleanLine(note); note != "" {
			q.Set("note", note)
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// PayPalMePaymentLink builds https://paypal.me/<name>/<12.40>EUR.
func PayPalMePaymentLink(name string, amount int) string {
	u := url.URL{Scheme: "https", Host: "paypal.me", Path: "/" + name}
	if amount > 0 {
		u.Path += "/" + FormatAmount(amount) + "EUR"
	}
	return u.String()
}

// LinkWithAmount returns the free link with the amount prefilled when its
// format is known (paypal.me, revolut.me, Wise Business open link) and
// whether it was prefilled.
func LinkWithAmount(link string, amount int, ref string) (string, bool) {
	link = strings.TrimSpace(link)
	if link == "" || amount <= 0 {
		return link, false
	}
	switch kind, handle := ClassifyPaymentLink(link); kind {
	case LinkRevolut:
		return RevolutPaymentLink(handle, amount, ref), true
	case LinkPayPal:
		return PayPalMePaymentLink(handle, amount), true
	}
	u, ok := parseLoose(link)
	if !ok {
		return link, false
	}
	segs := pathSegments(u)
	if hostOf(u) == "wise.com" && len(segs) == 3 && segs[0] == "pay" && segs[1] == "business" {
		q := url.Values{}
		q.Set("amount", FormatAmount(amount))
		q.Set("currency", "EUR")
		if ref = cleanLine(ref); ref != "" {
			q.Set("description", ref)
		}
		u.RawQuery = q.Encode()
		return u.String(), true
	}
	return link, false
}

// PaymentLinks returns the payer's links for one payment, in display order
// (Revolut, PayPal, free link), with the amount prefilled when possible.
func PaymentLinks(h PayoutHandles, amount int, ref string) []PaymentLink {
	out := []PaymentLink{}
	if h.RevolutTag != "" {
		out = append(out, PaymentLink{Kind: LinkRevolut, Label: "Revolut", URL: RevolutPaymentLink(h.RevolutTag, amount, ref), AmountPrefilled: amount > 0})
	}
	if h.PayPalMe != "" {
		out = append(out, PaymentLink{Kind: LinkPayPal, Label: "PayPal", URL: PayPalMePaymentLink(h.PayPalMe, amount), AmountPrefilled: amount > 0})
	}
	if h.Link != "" {
		link, prefilled := LinkWithAmount(h.Link, amount, ref)
		label := "Lien de paiement"
		if u, ok := parseLoose(link); ok {
			label = hostOf(u)
		}
		out = append(out, PaymentLink{Kind: LinkGeneric, Label: label, URL: link, AmountPrefilled: prefilled})
	}
	return out
}
