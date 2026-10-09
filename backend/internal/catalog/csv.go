package catalog

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// CSV format (one row per menu item, see docs/ARCHITECTURE.md):
//
//	restaurant_slug,restaurant_name,category,item_name,description,price_eur,tags,popular
//
// plus the optional restaurant columns address, lat, lng, cuisines, phone,
// ubereats_url, takeaway_url (filled once per restaurant) and the optional
// item columns emoji, available.
//
// csvMandatory are the columns that must be present in the header.
var csvMandatory = []string{"restaurant_slug", "category", "item_name", "price_eur"}

// CSVRestaurant is a restaurant parsed from a CSV file. Set tells which
// optional restaurant / item columns were provided, so that an import only
// overrides what the file actually contains (see Merge).
type CSVRestaurant struct {
	Import RestaurantImport
	Set    map[string]bool
}

// ParseCSV parses a menu CSV (comma, semicolon or tab separated, UTF-8 with or
// without BOM). It returns the restaurants in file order and the line errors.
func ParseCSV(r io.Reader) ([]CSVRestaurant, []string) {
	data, err := io.ReadAll(io.LimitReader(r, 10<<20))
	if err != nil {
		return nil, []string{"Fichier illisible."}
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, []string{"Le fichier CSV est vide."}
	}

	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = detectDelimiter(data)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		return nil, []string{"En-tête CSV illisible."}
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	var errs []string
	for _, c := range csvMandatory {
		if _, ok := col[c]; !ok {
			errs = append(errs, fmt.Sprintf("Colonne obligatoire manquante : %s.", c))
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}

	var (
		out    []CSVRestaurant
		bySlug = map[string]int{}
		// category index per restaurant
		catIdx = map[string]map[string]int{}
	)
	line := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err == nil {
			line, _ = cr.FieldPos(0)
		} else {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				line = pe.Line
			}
			errs = append(errs, fmt.Sprintf("Ligne %d : CSV invalide.", line))
			continue
		}
		get := func(name string) (string, bool) {
			i, ok := col[name]
			if !ok || i >= len(rec) {
				return "", false
			}
			return strings.TrimSpace(rec[i]), true
		}
		val := func(name string) string { v, _ := get(name); return v }
		if strings.Join(rec, "") == "" {
			continue
		}

		slug := strings.ToLower(val("restaurant_slug"))
		if slug == "" {
			errs = append(errs, fmt.Sprintf("Ligne %d : restaurant_slug manquant.", line))
			continue
		}
		idx, ok := bySlug[slug]
		if !ok {
			idx = len(out)
			bySlug[slug] = idx
			catIdx[slug] = map[string]int{}
			out = append(out, CSVRestaurant{Import: RestaurantImport{Slug: slug, Categories: []CategoryImport{}}, Set: map[string]bool{}})
		}
		cur := &out[idx]
		ri := &cur.Import

		// restaurant metadata: first non-empty value wins
		setText := func(column string, dst *string) {
			if v := val(column); v != "" && !cur.Set[column] {
				*dst = v
				cur.Set[column] = true
			}
		}
		setText("restaurant_name", &ri.Name)
		setText("address", &ri.Address)
		setText("phone", &ri.Phone)
		for _, c := range []string{"lat", "lng"} {
			if v := val(c); v != "" && !cur.Set[c] {
				f, err := domain.ParseDecimal(v)
				if err != nil {
					errs = append(errs, fmt.Sprintf("Ligne %d : %s invalide (« %s »).", line, c, v))
					continue
				}
				if c == "lat" {
					ri.Lat = f
				} else {
					ri.Lng = f
				}
				cur.Set[c] = true
			}
		}
		if v := val("cuisines"); v != "" && !cur.Set["cuisines"] {
			ri.Cuisines = domain.SplitList(v)
			cur.Set["cuisines"] = true
		}
		for _, p := range []struct{ column, id string }{{"ubereats_url", providers.UberEats}, {"takeaway_url", providers.Takeaway}} {
			if v := val(p.column); v != "" && !cur.Set[p.column] {
				if !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
					errs = append(errs, fmt.Sprintf("Ligne %d : %s doit commencer par https://.", line, p.column))
					continue
				}
				ri.Providers = append(ri.Providers, providers.Link{ID: p.id, URL: v})
				cur.Set[p.column] = true
			}
		}

		// menu item (a row with only restaurant metadata is allowed)
		itemName := val("item_name")
		category := val("category")
		priceRaw := val("price_eur")
		if itemName == "" && priceRaw == "" && category == "" {
			continue
		}
		if itemName == "" {
			errs = append(errs, fmt.Sprintf("Ligne %d : item_name manquant.", line))
			continue
		}
		if category == "" {
			errs = append(errs, fmt.Sprintf("Ligne %d : category manquante pour « %s ».", line, itemName))
			continue
		}
		price, err := domain.ParseEuros(priceRaw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("Ligne %d (« %s ») : %s", line, itemName, err.Error()))
			continue
		}
		popular, err := domain.ParseYesNo(val("popular"))
		if err != nil {
			errs = append(errs, fmt.Sprintf("Ligne %d : popular — %s", line, err.Error()))
			continue
		}
		item := ItemImport{
			Name: itemName, Description: val("description"), Price: price,
			Tags: domain.SplitList(val("tags")), Popular: popular,
		}
		if v, ok := get("emoji"); ok {
			item.Emoji = v
		}
		if v, ok := get("available"); ok && v != "" {
			b, err := domain.ParseYesNo(v)
			if err != nil {
				errs = append(errs, fmt.Sprintf("Ligne %d : available — %s", line, err.Error()))
				continue
			}
			item.Available = &b
		}
		ci, ok := catIdx[slug][strings.ToLower(category)]
		if !ok {
			ci = len(ri.Categories)
			catIdx[slug][strings.ToLower(category)] = ci
			ri.Categories = append(ri.Categories, CategoryImport{Name: category})
		}
		ri.Categories[ci].Items = append(ri.Categories[ci].Items, item)
	}
	_, hasDesc := col["description"]
	_, hasEmoji := col["emoji"]
	for i := range out {
		out[i].Set["description"] = hasDesc
		out[i].Set["emoji"] = hasEmoji
	}
	if len(out) == 0 && len(errs) == 0 {
		errs = append(errs, "Aucune ligne de menu dans le fichier.")
	}
	return out, errs
}

// detectDelimiter picks ';', '\t' or ',' from the header line (Excel FR
// exports use ';' because ',' is the decimal separator).
func detectDelimiter(data []byte) rune {
	first, _ := bufio.NewReader(bytes.NewReader(data)).ReadString('\n')
	counts := map[rune]int{';': strings.Count(first, ";"), '\t': strings.Count(first, "\t"), ',': strings.Count(first, ",")}
	best := ','
	for _, d := range []rune{';', '\t'} {
		if counts[d] > counts[best] {
			best = d
		}
	}
	return best
}

// Merge builds the payload to import from a CSV restaurant and, when the slug
// already exists, its current state: metadata columns absent from the file
// are kept, the menu is replaced by the file's, and items with the same name
// keep their option groups (and emoji / description when the file has no
// such column).
func Merge(existing *RestaurantImport, c CSVRestaurant) RestaurantImport {
	if existing == nil {
		return c.Import
	}
	out := *existing
	in := c.Import
	if c.Set["restaurant_name"] {
		out.Name = in.Name
	}
	if c.Set["address"] {
		out.Address = in.Address
	}
	if c.Set["phone"] {
		out.Phone = in.Phone
	}
	if c.Set["lat"] {
		out.Lat = in.Lat
	}
	if c.Set["lng"] {
		out.Lng = in.Lng
	}
	if c.Set["cuisines"] {
		out.Cuisines = in.Cuisines
	}
	if len(in.Providers) > 0 {
		merged := []providers.Link{}
		override := map[string]providers.Link{}
		for _, p := range in.Providers {
			override[p.ID] = p
		}
		for _, p := range existing.Providers {
			if _, ok := override[p.ID]; !ok {
				merged = append(merged, p)
			}
		}
		out.Providers = append(merged, in.Providers...)
	}

	old := map[string]ItemImport{}
	for _, cat := range existing.Categories {
		for _, it := range cat.Items {
			old[strings.ToLower(strings.TrimSpace(it.Name))] = it
		}
	}
	out.Categories = make([]CategoryImport, 0, len(in.Categories))
	for _, cat := range in.Categories {
		nc := CategoryImport{Name: cat.Name, Items: make([]ItemImport, 0, len(cat.Items))}
		for _, it := range cat.Items {
			if prev, ok := old[strings.ToLower(it.Name)]; ok {
				it.OptionGroups = prev.OptionGroups
				if !c.Set["emoji"] {
					it.Emoji = prev.Emoji
				}
				if !c.Set["description"] {
					it.Description = prev.Description
				}
				if it.Available == nil {
					it.Available = prev.Available
				}
			}
			nc.Items = append(nc.Items, it)
		}
		out.Categories = append(out.Categories, nc)
	}
	return out
}
