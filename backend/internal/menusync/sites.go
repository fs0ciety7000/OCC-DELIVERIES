package menusync

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

var takeawayMenuRe = regexp.MustCompile(`^https?://(www\.)?(takeaway\.com|just-eat\.[a-z.]+)/[a-z-]+/menu/[a-z0-9-]+`)

// ParseTakeawaySite parses a restaurant mini-site built by Takeaway.com
// (shared template: .menucat categories, schema.org Product microdata items,
// schema.org Restaurant in the footer, <meta name="orderUrl">). Side dishes
// are not part of the page (ordering is disabled there): no option groups.
func ParseTakeawaySite(page []byte, pageURL string) (Restaurant, error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return Restaurant{}, err
	}
	r := Restaurant{SourceURLs: []string{pageURL}}

	for _, t := range Microdata(doc) {
		if isType(t, "Restaurant", "FoodEstablishment", "LocalBusiness") {
			applyRestaurantThing(&r, t)
			break
		}
	}
	if r.Name == "" {
		r.Name = siteTitleName(doc)
	}
	if r.Name == "" {
		return Restaurant{}, errors.New("nom du restaurant introuvable")
	}
	fillContactFromDoc(&r, doc)
	r.Slug = Slugify(r.Name)

	if u := takeawayLink(doc); u != "" {
		r.Providers = []providers.Link{{ID: providers.Takeaway, URL: u}}
	}
	if bg := findFirst(doc, func(n *html.Node) bool { return hasClass(n, "background") }); bg != nil {
		if st, _ := attr(bg, "style"); st != "" {
			if m := regexp.MustCompile(`url\('?([^')]+)'?\)`).FindStringSubmatch(st); m != nil {
				r.CoverURL = absURL(pageURL, m[1])
			}
		}
	}

	for _, cat := range findAll(doc, func(n *html.Node) bool { return hasClass(n, "menucat") }) {
		c := catalog.CategoryImport{}
		if cn := findFirst(cat, func(n *html.Node) bool { return hasClass(n, "category-name") }); cn != nil {
			c.Name = textContent(cn, nil)
		}
		for _, t := range Microdata(cat) {
			if !isType(t, "Product", "MenuItem") {
				continue
			}
			if it, ok := thingItem(t); ok {
				c.Items = append(c.Items, it)
			}
		}
		if c.Name != "" && len(c.Items) > 0 {
			r.Categories = append(r.Categories, c)
		}
	}
	if len(r.Categories) == 0 {
		return Restaurant{}, errors.New("aucun plat trouvé (carte vide ou modèle de page différent)")
	}
	return r, nil
}

// ParseJSONLD reads a restaurant from schema.org JSON-LD (Restaurant with
// hasMenu → hasMenuSection → hasMenuItem), with a microdata fallback.
func ParseJSONLD(page []byte, pageURL string) (Restaurant, error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return Restaurant{}, err
	}
	nodes := JSONLD(doc)
	nodes = append(nodes, Microdata(doc)...)
	r := Restaurant{SourceURLs: []string{pageURL}}
	var menus []Thing
	for _, t := range nodes {
		if isType(t, "Restaurant", "FoodEstablishment", "CafeOrCoffeeShop", "FastFoodRestaurant", "Bakery", "BarOrPub") && r.Name == "" {
			applyRestaurantThing(&r, t)
			menus = append(menus, things(t, "hasMenu")...)
		}
		if isType(t, "Menu") {
			menus = append(menus, t)
		}
	}
	if r.Name == "" {
		return Restaurant{}, errors.New("aucune entité schema.org Restaurant trouvée")
	}
	fillContactFromDoc(&r, doc)
	r.Slug = Slugify(r.Name)
	if u := takeawayLink(doc); u != "" {
		r.Providers = []providers.Link{{ID: providers.Takeaway, URL: u}}
	}
	for _, m := range menus {
		addMenuSections(&r, m, "Carte")
	}
	r.Categories = dropEmptyCategories(r.Categories)
	if len(r.Categories) == 0 {
		return Restaurant{}, errors.New("restaurant trouvé mais aucun menu schema.org (hasMenu / MenuSection / MenuItem)")
	}
	return r, nil
}

func addMenuSections(r *Restaurant, t Thing, fallback string) {
	if items := things(t, "hasMenuItem"); len(items) > 0 {
		name := str(t, "name")
		if name == "" {
			name = fallback
		}
		c := catalog.CategoryImport{Name: name}
		for _, it := range items {
			if item, ok := thingItem(it); ok {
				c.Items = append(c.Items, item)
			}
		}
		r.Categories = append(r.Categories, c)
	}
	for _, s := range things(t, "hasMenuSection") {
		addMenuSections(r, s, fallback)
	}
}

// thingItem converts a schema.org Product/MenuItem into a menu item.
func thingItem(t Thing) (catalog.ItemImport, bool) {
	it := catalog.ItemImport{Name: str(t, "name"), Description: str(t, "description")}
	if it.Name == "" {
		return it, false
	}
	price, ok := 0, false
	for _, o := range things(t, "offers") {
		if price, ok = ParseEuroCents(str(o, "price")); ok {
			break
		}
	}
	if !ok {
		price, ok = ParseEuroCents(str(t, "price"))
	}
	if !ok {
		return it, false // never guess a price
	}
	it.Price = price
	return it, true
}

func applyRestaurantThing(r *Restaurant, t Thing) {
	r.Name = str(t, "name")
	if d := str(t, "description"); d != "" {
		r.Description = d
	}
	if p := str(t, "telephone"); p != "" {
		r.Phone = p
	}
	for _, a := range values(t, "address") {
		switch v := a.(type) {
		case string:
			r.Address = CleanText(v)
		case map[string]any:
			street, pc, loc := str(v, "streetAddress"), str(v, "postalCode"), str(v, "addressLocality")
			r.Address = strings.TrimSpace(strings.Trim(street+", "+strings.TrimSpace(pc+" "+loc), ", "))
		}
		if r.Address != "" {
			break
		}
	}
	for _, g := range things(t, "geo") {
		lat, err1 := strconv.ParseFloat(str(g, "latitude"), 64)
		lng, err2 := strconv.ParseFloat(str(g, "longitude"), 64)
		if err1 == nil && err2 == nil {
			r.Lat, r.Lng = lat, lng
		}
	}
	for _, c := range values(t, "servesCuisine") {
		if s, ok := c.(string); ok {
			r.Cuisines = appendUnique(r.Cuisines, strings.ToLower(CleanText(s)))
		}
	}
	for _, ar := range things(t, "aggregateRating") {
		v, err := strconv.ParseFloat(strings.ReplaceAll(str(ar, "ratingValue"), ",", "."), 64)
		best, _ := strconv.ParseFloat(str(ar, "bestRating"), 64)
		if err == nil && v > 0 {
			if best > 0 && best != 5 {
				v = v * 5 / best
			}
			if v <= 5 {
				r.Rating = v
				r.RatingCount, _ = strconv.Atoi(firstNonEmpty(str(ar, "ratingCount"), str(ar, "reviewCount")))
			}
		}
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// takeawayLink returns the Takeaway.com store page of a site, if any.
func takeawayLink(doc *html.Node) string {
	if u := metaContent(doc, "orderUrl"); takeawayMenuRe.MatchString(u) {
		return u
	}
	for _, a := range findAll(doc, func(n *html.Node) bool { return n.DataAtom == atom.A }) {
		if h, _ := attr(a, "href"); takeawayMenuRe.MatchString(h) {
			return h
		}
	}
	return ""
}

// siteTitleName reads "Tomo Mons - Commander un repas…" style titles.
func siteTitleName(doc *html.Node) string {
	t := findFirst(doc, func(n *html.Node) bool { return n.DataAtom == atom.Title })
	if t == nil {
		return ""
	}
	s := textContent(t, nil)
	if i := strings.Index(s, " - "); i > 0 {
		s = s[:i]
	}
	return CleanText(s)
}

func absURL(base, ref string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := b.Parse(ref)
	if err != nil {
		return ref
	}
	return r.String()
}
