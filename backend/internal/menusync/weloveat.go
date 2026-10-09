package menusync

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// weloveat.be is an Angular SPA backed by a public JSON API (found in its
// main.*.js bundle: environment.apiUrl). The endpoints below are the ones the
// SPA itself calls anonymously; https://api.weloveat.be/robots.txt allows all.
const (
	WeloveatSite = "https://weloveat.be"
	WeloveatAPI  = "https://api.weloveat.be/api/"
)

// WeloveatCity is the search context of a city.
type WeloveatCity struct {
	Address    string
	PostalCode string
	Locality   string
	Lat, Lng   float64
}

// WeloveatCities maps a -city value to its search context.
var WeloveatCities = map[string]WeloveatCity{
	"mons": {Address: "Grand-Place, 7000 Mons, Belgique", PostalCode: "7000", Locality: "Mons", Lat: 50.4542, Lng: 3.9567},
}

// API paths, relative to WeloveatAPI.
const (
	weloveatSearchPath    = "test/searchEstablishments?page=1"
	weloveatCataloguePath = "products/getFoundEstablishmentProducts"
	weloveatProductPath   = "products/"
)

// WeloveatSearchURL lists establishments (POST, page 1, server returns all).
func WeloveatSearchURL() string { return WeloveatAPI + weloveatSearchPath }

// WeloveatSearchBody is the body the SPA posts to search establishments.
func WeloveatSearchBody(c WeloveatCity) []byte {
	b, _ := json.Marshal(map[string]any{
		"type": "restaurant", "receptionType": "delivery", "perPage": 1000, "currentPage": 1,
		"address": c.Address, "latitude": c.Lat, "longitude": c.Lng,
		"locality": c.Locality, "postal_code": c.PostalCode,
	})
	return b
}

// WeloveatCatalogueURL returns a shop's categories and products (POST).
func WeloveatCatalogueURL() string { return WeloveatAPI + weloveatCataloguePath }

// WeloveatCatalogueBody is the body the SPA posts for a shop catalogue.
func WeloveatCatalogueBody(slug string, c WeloveatCity) []byte {
	b, _ := json.Marshal(map[string]any{
		"slug": slug, "type": "restaurant", "locality": c.Locality, "postal_code": c.PostalCode,
		"receptionType": "delivery", "latitude": c.Lat, "longitude": c.Lng,
	})
	return b
}

// WeloveatProductURL returns a product with its supplements (GET).
func WeloveatProductURL(productSlug string) string {
	return WeloveatAPI + weloveatProductPath + productSlug
}

// WeloveatPageURL is the public restaurant page of the SPA.
func WeloveatPageURL(slug string) string { return WeloveatSite + "/" + slug }

// piiKeys are dropped from weloveat answers before caching: the API leaks
// merchant account data we have no business storing.
var piiKeys = map[string]bool{
	"user": true, "email": true, "fcm_token": true, "first_name": true, "last_name": true,
	"connected_account_id": true, "invitation_redirect_url": true, "iban": true, "vat_number": true,
	"owner": true, "manager": true, "users": true, "password": true, "api_token": true, "remember_token": true,
	"stripe_account_id": true, "birth_date": true, "personal_phone": true,
}

// SanitizeWeloveat removes personal data keys (recursively) from a JSON answer.
func SanitizeWeloveat(b []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("réponse JSON invalide : %w", err)
	}
	var clean func(any) any
	clean = func(v any) any {
		switch t := v.(type) {
		case map[string]any:
			for k, c := range t {
				if piiKeys[k] {
					delete(t, k)
					continue
				}
				t[k] = clean(c)
			}
		case []any:
			for i, c := range t {
				t[i] = clean(c)
			}
		}
		return v
	}
	return json.Marshal(clean(v))
}

// flexNumber decodes 13, 13.5, "13.00" or null.
type flexNumber struct {
	V   float64
	Set bool
}

func (n *flexNumber) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil {
		return nil // not a number: ignore
	}
	n.V, n.Set = v, true
	return nil
}

// i18nText decodes "x" or {"fr":"x", ...}.
type i18nText string

func (t *i18nText) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = i18nText(s)
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err == nil {
		for _, k := range []string{"fr", "en", "nl"} {
			if m[k] != "" {
				*t = i18nText(m[k])
				return nil
			}
		}
		for _, v := range m {
			*t = i18nText(v)
			return nil
		}
	}
	return nil
}

type wlEstablishment struct {
	Name             i18nText   `json:"name"`
	Description      i18nText   `json:"description"`
	Slug             string     `json:"slug"`
	Phone            string     `json:"phone_number"`
	PhoneAlt         string     `json:"phone"`
	Telephone        string     `json:"telephone"`
	Address          string     `json:"address"`
	Street           string     `json:"street"`
	StreetNumber     string     `json:"street_number"`
	PostalCode       string     `json:"postal_code"`
	Locality         string     `json:"locality"`
	City             string     `json:"city"`
	Latitude         flexNumber `json:"latitude"`
	Longitude        flexNumber `json:"longitude"`
	ScoreFloat       flexNumber `json:"score_float"`
	NbrEvaluation    int        `json:"nbr_evaluation"`
	MinimumPerOrder  flexNumber `json:"minimum_per_order"`
	ShippingCost     flexNumber `json:"shipping_cost"`
	FreeDelivery     any        `json:"free_delivery"`
	EstimatedMinutes flexNumber `json:"estimated_delivery_time"`
	Type             string     `json:"type"`
	Categories       []struct {
		Name i18nText `json:"name"`
	} `json:"categories"`
}

// phone returns the business phone of the establishment (the owner's
// personal data is stripped by SanitizeWeloveat before parsing).
func (e wlEstablishment) phone() string {
	return CleanText(firstNonEmpty(e.Phone, e.PhoneAlt, e.Telephone))
}

// address returns the postal address, composed from its parts when the
// API gives no one-line address.
func (e wlEstablishment) address() string {
	if a := trimCountry(e.Address); a != "" {
		return a
	}
	street := CleanText(strings.TrimSpace(e.Street + " " + e.StreetNumber))
	place := CleanText(strings.TrimSpace(e.PostalCode + " " + firstNonEmpty(e.Locality, e.City)))
	return strings.Trim(street+", "+place, ", ")
}

// WeloveatShop is an establishment of a search result.
type WeloveatShop struct {
	Slug     string
	Name     string
	Lat, Lng float64
	Address  string
	Phone    string
}

// ParseWeloveatSearch returns the restaurants of a search answer.
func ParseWeloveatSearch(b []byte) ([]WeloveatShop, error) {
	var res struct {
		Establishments struct {
			Data []wlEstablishment `json:"data"`
		} `json:"establishments"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("recherche weloveat invalide : %w", err)
	}
	var out []WeloveatShop
	for _, e := range res.Establishments.Data {
		if e.Slug == "" || (e.Type != "" && e.Type != "restaurant") {
			continue
		}
		out = append(out, WeloveatShop{Slug: e.Slug, Name: CleanText(string(e.Name)), Lat: e.Latitude.V, Lng: e.Longitude.V, Address: e.address(), Phone: e.phone()})
	}
	return out, nil
}

// WeloveatProductRef is a catalogue product whose supplements can be fetched.
type WeloveatProductRef struct {
	Category, Item int // indexes in Restaurant.Categories / Items
	Slug           string
}

// ParseWeloveatCatalogue parses a shop catalogue answer.
func ParseWeloveatCatalogue(b []byte, pageURL string) (Restaurant, []WeloveatProductRef, error) {
	var res struct {
		Establishment *wlEstablishment `json:"establishment"`
		Results       []struct {
			Category struct {
				Name    i18nText `json:"name"`
				Display *int     `json:"display"`
			} `json:"category"`
			Products []struct {
				Name        i18nText   `json:"name"`
				Description i18nText   `json:"description"`
				Price       flexNumber `json:"price"`
				Slug        string     `json:"slug"`
				Display     *int       `json:"display"`
			} `json:"products"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return Restaurant{}, nil, fmt.Errorf("catalogue weloveat invalide : %w", err)
	}
	e := res.Establishment
	if e == nil || CleanText(string(e.Name)) == "" {
		return Restaurant{}, nil, errors.New("catalogue weloveat sans établissement")
	}
	r := Restaurant{SourceURLs: []string{pageURL}}
	r.Name = CleanText(string(e.Name))
	r.Slug = Slugify(r.Name)
	r.Description = CleanText(string(e.Description))
	r.Address = e.address()
	r.Lat, r.Lng = e.Latitude.V, e.Longitude.V
	r.Phone = e.phone()
	if e.ScoreFloat.V > 0 && e.NbrEvaluation > 0 {
		r.Rating, r.RatingCount = e.ScoreFloat.V, e.NbrEvaluation
	}
	if e.MinimumPerOrder.Set {
		r.MinOrder = EurosToCents(e.MinimumPerOrder.V)
	}
	if e.ShippingCost.Set {
		r.DeliveryFee = EurosToCents(e.ShippingCost.V)
	}
	if e.EstimatedMinutes.Set && e.EstimatedMinutes.V > 0 {
		r.EtaMin = int(e.EstimatedMinutes.V)
		r.EtaMax = r.EtaMin
	}
	for _, c := range e.Categories {
		if n := strings.ToLower(CleanText(string(c.Name))); n != "" {
			r.Cuisines = appendUnique(r.Cuisines, n)
		}
	}
	r.Providers = []providers.Link{{ID: providers.Weloveat, URL: pageURL}}

	var refs []WeloveatProductRef
	for _, c := range res.Results {
		if c.Category.Display != nil && *c.Category.Display == 0 {
			continue
		}
		cat := catalog.CategoryImport{Name: CleanText(string(c.Category.Name))}
		for _, p := range c.Products {
			if p.Display != nil && *p.Display == 0 {
				continue
			}
			it := catalog.ItemImport{Name: CleanText(string(p.Name)), Description: CleanText(string(p.Description)), Price: max(EurosToCents(p.Price.V), 0)}
			if it.Name == "" {
				continue
			}
			cat.Items = append(cat.Items, it)
			if p.Slug != "" {
				refs = append(refs, WeloveatProductRef{Category: len(r.Categories), Item: len(cat.Items) - 1, Slug: p.Slug})
			}
		}
		if cat.Name != "" && len(cat.Items) > 0 {
			r.Categories = append(r.Categories, cat)
		}
	}
	return r, refs, nil
}

// ParseWeloveatProduct returns the supplement groups of a product answer.
// Single-select groups (radio buttons, first choice preselected by the SPA)
// become min 1 / max 1; multi-select ones min 0 / unbounded.
func ParseWeloveatProduct(b []byte) ([]domain.OptionGroup, error) {
	var res struct {
		Data struct {
			Groups []struct {
				ID          int      `json:"id"`
				Name        i18nText `json:"name"`
				Display     *int     `json:"display"`
				Visible     *int     `json:"visible"`
				MultiSelect *int     `json:"multi_select"`
				Pivot       *struct {
					MultiSelect *int `json:"multi_select"`
				} `json:"pivot"`
				Products []struct {
					ID      int        `json:"id"`
					Name    i18nText   `json:"name"`
					Price   flexNumber `json:"price"`
					Display *int       `json:"display"`
					Visible *int       `json:"visible"`
				} `json:"products"`
			} `json:"__"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("produit weloveat invalide : %w", err)
	}
	hidden := func(p *int) bool { return p != nil && *p == 0 }
	var out []domain.OptionGroup
	for _, g := range res.Data.Groups {
		if hidden(g.Display) || hidden(g.Visible) {
			continue
		}
		multi := g.MultiSelect != nil && *g.MultiSelect == 1
		if g.Pivot != nil && g.Pivot.MultiSelect != nil {
			multi = *g.Pivot.MultiSelect == 1
		}
		og := domain.OptionGroup{ID: "g" + strconv.Itoa(g.ID), Name: CleanText(string(g.Name))}
		seen := map[string]bool{}
		for _, p := range g.Products {
			id := "c" + strconv.Itoa(p.ID)
			if hidden(p.Display) || hidden(p.Visible) || seen[id] || CleanText(string(p.Name)) == "" {
				continue
			}
			seen[id] = true
			og.Choices = append(og.Choices, domain.OptionChoice{ID: id, Name: CleanText(string(p.Name)), Price: max(EurosToCents(p.Price.V), 0)})
		}
		if len(og.Choices) == 0 {
			continue
		}
		if !multi {
			og.Min, og.Max = 1, 1
		}
		out = append(out, normalizeGroup(og))
	}
	if err := domain.ValidateOptionGroups(out); err != nil {
		return nil, err
	}
	return out, nil
}

func trimCountry(a string) string {
	a = CleanText(a)
	for _, c := range []string{", Belgique", ", Belgium", ", België"} {
		a = strings.TrimSuffix(a, c)
	}
	return a
}
