package menusync

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Contact details (phone, address, coordinates) of a restaurant page. The
// parsers read the structured data first (schema.org Restaurant); these
// helpers complete what is still missing from the rest of the page.

// normalizeContact stores the phone in E.164 (junk dropped) and the address
// as « Rue X 12, 7000 Mons » (domain.NormalizeRestaurantPhone / NormalizeAddress).
func (r *Restaurant) normalizeContact() {
	r.Address = domain.NormalizeAddress(r.Address)
	if p, ok := domain.NormalizeRestaurantPhone(r.Phone); ok {
		r.Phone = p
	} else {
		r.Phone = ""
	}
	if r.Lat < -90 || r.Lat > 90 || r.Lng < -180 || r.Lng > 180 {
		r.Lat, r.Lng = 0, 0
	}
}

// contactTypes are the schema.org types carrying a restaurant's contact.
var contactTypes = []string{"Restaurant", "FoodEstablishment", "LocalBusiness", "CafeOrCoffeeShop", "FastFoodRestaurant", "Bakery", "BarOrPub", "IceCreamShop", "Organization"}

// jsonPhoneRe finds a phone published in an inline JSON / JS object.
var jsonPhoneRe = regexp.MustCompile(`"(?:telephone|phone|phoneNumber|phone_number)"\s*:\s*"([+0-9][0-9 ./()\-]{7,24})"`)

// labelPhoneRe finds « Tél. : 065 … » / « Téléphone 065 … » in a text.
var labelPhoneRe = regexp.MustCompile(`(?i)(?:t[ée]l(?:[ée]phone)?\.?|phone|gsm|appelez(?:-nous)?(?: au)?)\s*:?\s*(\+?[0-9][0-9 ./()\-\x{00a0}]{7,20}[0-9])`)

// bePhoneRunRe finds an unlabelled Belgian number (« 065/35.29.64 », « +32 65 … »).
var bePhoneRunRe = regexp.MustCompile(`(?:\+32|\b0)\s?[1-9][0-9 ./\-]{6,14}[0-9]`)

// fillContactFromDoc completes the phone, address and coordinates of r from
// the rest of the page: JSON-LD / microdata of any business type, tel:
// links, the footer contact block, then JSON embedded in scripts. Values
// already set are kept.
func fillContactFromDoc(r *Restaurant, doc *html.Node) {
	things := append(JSONLD(doc), Microdata(doc)...)
	for _, t := range things {
		if !isType(t, contactTypes...) {
			continue
		}
		var c Restaurant
		applyRestaurantThing(&c, t)
		if r.Phone == "" {
			r.Phone = c.Phone
		}
		if r.Address == "" {
			r.Address = c.Address
		}
		if r.Lat == 0 && r.Lng == 0 {
			r.Lat, r.Lng = c.Lat, c.Lng
		}
	}
	if r.Phone == "" {
		for _, a := range findAll(doc, func(n *html.Node) bool { return n.DataAtom == atom.A }) {
			if h, _ := attr(a, "href"); strings.HasPrefix(strings.ToLower(strings.TrimSpace(h)), "tel:") {
				if p, ok := domain.NormalizeRestaurantPhone(strings.TrimSpace(h)[4:]); ok {
					r.Phone = p
					break
				}
			}
		}
	}
	if r.Phone == "" {
		// footer « Contact » block of the Takeaway template, or any element
		// whose id / class says contact
		for _, n := range findAll(doc, func(n *html.Node) bool {
			id, _ := attr(n, "id")
			cl, _ := attr(n, "class")
			return strings.Contains(strings.ToLower(id+" "+cl), "contact") || strings.Contains(strings.ToLower(id), "address")
		}) {
			if m := labelPhoneRe.FindStringSubmatch(textContent(n, nil)); m != nil {
				if p, ok := domain.NormalizeRestaurantPhone(m[1]); ok {
					r.Phone = p
					break
				}
			}
			if run := bePhoneRunRe.FindString(textContent(n, nil)); run != "" {
				if p, ok := domain.NormalizeRestaurantPhone(run); ok {
					r.Phone = p
					break
				}
			}
		}
	}
	if r.Phone == "" {
		for _, s := range findAll(doc, func(n *html.Node) bool { return n.DataAtom == atom.Script }) {
			if m := jsonPhoneRe.FindStringSubmatch(textOf(s)); m != nil {
				if p, ok := domain.NormalizeRestaurantPhone(m[1]); ok {
					r.Phone = p
					break
				}
			}
		}
	}
	if r.Lat == 0 && r.Lng == 0 {
		lat, _ := strconv.ParseFloat(metaContent(doc, "place:location:latitude"), 64)
		lng, _ := strconv.ParseFloat(metaContent(doc, "place:location:longitude"), 64)
		if lat != 0 && lng != 0 {
			r.Lat, r.Lng = lat, lng
		}
	}
}
