package menusync

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// --- small DOM helpers ---------------------------------------------------

func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func hasClass(n *html.Node, class string) bool {
	v, _ := attr(n, "class")
	for _, c := range strings.Fields(v) {
		if c == class {
			return true
		}
	}
	return false
}

// textContent returns the text of a node, skipping script/style and the
// nodes for which skip returns true.
func textContent(n *html.Node, skip func(*html.Node) bool) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
			return
		}
		if n.Type == html.ElementNode && (n.DataAtom == atom.Script || n.DataAtom == atom.Style || (skip != nil && skip(n))) {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return CleanText(b.String())
}

// findAll returns the element descendants (incl. n) matching pred, in
// document order. Matches are not searched for further nested matches.
func findAll(n *html.Node, pred func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && pred(n) {
			out = append(out, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func findFirst(n *html.Node, pred func(*html.Node) bool) *html.Node {
	if r := findAll(n, pred); len(r) > 0 {
		return r[0]
	}
	return nil
}

func metaContent(doc *html.Node, name string) string {
	m := findFirst(doc, func(n *html.Node) bool {
		if n.DataAtom != atom.Meta {
			return false
		}
		v, _ := attr(n, "name")
		p, _ := attr(n, "property")
		return strings.EqualFold(v, name) || strings.EqualFold(p, name)
	})
	if m == nil {
		return ""
	}
	v, _ := attr(m, "content")
	return CleanText(v)
}

// --- schema.org microdata ------------------------------------------------

// Thing is a schema.org entity as a JSON-LD-like map: "@type" plus
// properties whose values are string, Thing, or []any of those.
type Thing = map[string]any

// Microdata extracts the top-level schema.org microdata items below n.
func Microdata(n *html.Node) []Thing {
	var out []Thing
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if _, scope := attr(n, "itemscope"); scope {
				if _, prop := attr(n, "itemprop"); !prop {
					// top-level item; keep walking: top-level items can be nested
					// in the DOM (e.g. <html itemscope itemtype=WebPage>)
					out = append(out, microItem(n))
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func microItem(n *html.Node) Thing {
	t := Thing{}
	if typ, ok := attr(n, "itemtype"); ok {
		typ = strings.TrimSpace(typ)
		if i := strings.LastIndexByte(typ, '/'); i >= 0 {
			typ = typ[i+1:]
		}
		t["@type"] = typ
	}
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		for ; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			props, isProp := attr(c, "itemprop")
			_, isScope := attr(c, "itemscope")
			if isProp {
				var v any
				if isScope {
					v = microItem(c)
				} else {
					v = microValue(c)
				}
				for _, p := range strings.Fields(props) {
					addProp(t, p, v)
				}
			}
			if !isScope {
				walk(c.FirstChild)
			}
		}
	}
	walk(n.FirstChild)
	return t
}

func microValue(n *html.Node) string {
	switch n.DataAtom {
	case atom.Meta:
		v, _ := attr(n, "content")
		return CleanText(v)
	case atom.A, atom.Link, atom.Area:
		v, _ := attr(n, "href")
		return v
	case atom.Img, atom.Audio, atom.Video, atom.Source, atom.Iframe, atom.Embed:
		v, _ := attr(n, "src")
		return v
	case atom.Time:
		if v, ok := attr(n, "datetime"); ok {
			return v
		}
	case atom.Data, atom.Meter:
		if v, ok := attr(n, "value"); ok {
			return v
		}
	}
	if v, ok := attr(n, "content"); ok && n.DataAtom != atom.H1 && n.DataAtom != atom.H2 && n.DataAtom != atom.H3 && n.DataAtom != atom.P {
		return CleanText(v)
	}
	return textContent(n, func(c *html.Node) bool { return hasClass(c, "meal-allergens") })
}

func addProp(t Thing, key string, v any) {
	switch old := t[key].(type) {
	case nil:
		t[key] = v
	case []any:
		t[key] = append(old, v)
	default:
		t[key] = []any{old, v}
	}
}

// --- JSON-LD --------------------------------------------------------------

// JSONLD returns every JSON-LD node of a document (flattening @graph/arrays).
func JSONLD(doc *html.Node) []Thing {
	var out []Thing
	var flat func(any)
	flat = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, c := range t {
				flat(c)
			}
		case map[string]any:
			if g, ok := t["@graph"]; ok {
				flat(g)
				return
			}
			out = append(out, t)
		}
	}
	for _, s := range findAll(doc, func(n *html.Node) bool {
		typ, _ := attr(n, "type")
		return n.DataAtom == atom.Script && strings.EqualFold(strings.TrimSpace(typ), "application/ld+json")
	}) {
		var v any
		raw := bytes.TrimSpace([]byte(textOf(s)))
		if json.Unmarshal(raw, &v) == nil {
			flat(v)
		}
	}
	return out
}

func textOf(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}

// --- Thing accessors -----------------------------------------------------

func thingTypes(t Thing) []string {
	switch v := t["@type"].(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func isType(t Thing, types ...string) bool {
	for _, have := range thingTypes(t) {
		have = have[strings.LastIndexByte(have, '/')+1:]
		for _, want := range types {
			if strings.EqualFold(have, want) {
				return true
			}
		}
	}
	return false
}

// values returns a property as a list.
func values(t Thing, key string) []any {
	switch v := t[key].(type) {
	case nil:
		return nil
	case []any:
		return v
	default:
		return []any{v}
	}
}

// str returns the first string value of a property (numbers formatted).
func str(t Thing, key string) string {
	for _, v := range values(t, key) {
		switch x := v.(type) {
		case string:
			return CleanText(x)
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64)
		case map[string]any:
			if s, ok := x["@value"].(string); ok {
				return CleanText(s)
			}
			if s, ok := x["name"].(string); ok {
				return CleanText(s)
			}
		}
	}
	return ""
}

// things returns the Thing values of a property.
func things(t Thing, key string) []Thing {
	var out []Thing
	for _, v := range values(t, key) {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
