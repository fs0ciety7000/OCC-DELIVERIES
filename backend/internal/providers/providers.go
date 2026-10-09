// Package providers turns a party summary into a "dispatch": a deep link,
// step by step instructions (French) and a copyable cart recap.
//
// Neither Uber Eats, Takeaway, Deliveroo nor weloveat expose a public API to fill a customer cart,
// see docs/adr/0002-providers.md. A future partner adapter only needs to
// implement Provider.
package providers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Provider ids.
const (
	UberEats  = "ubereats"
	Takeaway  = "takeaway"
	Deliveroo = "deliveroo"
	Weloveat  = "weloveat"
	Export    = "export"
	Phone     = "phone"
)

// Link is a restaurant page on a delivery platform.
type Link struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Restaurant is the restaurant data needed to dispatch an order.
type Restaurant struct {
	ID        string
	Name      string
	Address   string
	Phone     string
	Providers []Link
}

// LinkFor returns the restaurant URL for a provider id ("" if none).
func (r Restaurant) LinkFor(id string) string {
	for _, l := range r.Providers {
		if l.ID == id && l.URL != "" {
			return l.URL
		}
	}
	return ""
}

// Dispatch is what the host gets to actually place the order.
type Dispatch struct {
	Method       string   `json:"method"`
	URL          string   `json:"url"`
	CartText     string   `json:"cartText"`
	Instructions []string `json:"instructions"`
}

// Provider produces a Dispatch for a restaurant and a party summary.
type Provider interface {
	ID() string
	Name() string
	Color() string
	Dispatch(r Restaurant, s domain.Summary) Dispatch
}

var registry = map[string]Provider{
	UberEats:  uberEats{},
	Takeaway:  takeaway{},
	Deliveroo: deliveroo{},
	Weloveat:  weloveat{},
	Export:    export{},
	Phone:     phone{},
}

// Get returns a registered provider.
func Get(id string) (Provider, bool) {
	p, ok := registry[id]
	return p, ok
}

// IDs returns every registered provider id, sorted.
func IDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Platforms are the delivery platforms that can be enabled through OCC_PROVIDERS.
var Platforms = []string{UberEats, Takeaway, Deliveroo, Weloveat}

// IsPlatform reports whether id is a delivery platform (vs. export/phone).
func IsPlatform(id string) bool {
	for _, p := range Platforms {
		if p == id {
			return true
		}
	}
	return false
}

// CartText renders the consolidated cart as readable French text.
func CartText(r Restaurant, s domain.Summary) string {
	var b strings.Builder
	name := r.Name
	if name == "" && s.Restaurant != nil {
		name = s.Restaurant.Name
	}
	fmt.Fprintf(&b, "Commande groupée — %s\n", name)
	b.WriteString(strings.Repeat("-", 32) + "\n")
	articles := 0
	for _, l := range s.Consolidated {
		articles += l.Quantity
		fmt.Fprintf(&b, "%d × %s", l.Quantity, l.Name)
		if l.OptionsLabel != "" {
			fmt.Fprintf(&b, " (%s)", l.OptionsLabel)
		}
		fmt.Fprintf(&b, " — %s\n", domain.FormatEUR(l.Total))
		for _, n := range l.Notes {
			fmt.Fprintf(&b, "   • %s\n", n)
		}
	}
	b.WriteString(strings.Repeat("-", 32) + "\n")
	fmt.Fprintf(&b, "%d article(s) — sous-total : %s\n", articles, domain.FormatEUR(s.ItemsSubtotal))
	if s.DeliveryFee > 0 {
		fmt.Fprintf(&b, "Livraison : %s\n", domain.FormatEUR(s.DeliveryFee))
	}
	if s.ServiceFee > 0 {
		fmt.Fprintf(&b, "Frais de service : %s\n", domain.FormatEUR(s.ServiceFee))
	}
	if s.Tip > 0 {
		fmt.Fprintf(&b, "Pourboire : %s\n", domain.FormatEUR(s.Tip))
	}
	fmt.Fprintf(&b, "Total estimé : %s", domain.FormatEUR(s.GrandTotal))
	return b.String()
}

type uberEats struct{}

func (uberEats) ID() string    { return UberEats }
func (uberEats) Name() string  { return "Uber Eats" }
func (uberEats) Color() string { return "#06C167" }

func (p uberEats) Dispatch(r Restaurant, s domain.Summary) Dispatch {
	u := r.LinkFor(UberEats)
	if u == "" {
		u = "https://www.ubereats.com/be"
	}
	return Dispatch{
		Method:   UberEats,
		URL:      u,
		CartText: CartText(r, s),
		Instructions: []string{
			fmt.Sprintf("Ouvrez la page de « %s » dans l'app ou le site Uber Eats (bouton ci-dessus).", r.Name),
			"Lancez une « commande groupée » Uber Eats depuis la page du restaurant (icône groupe / « Commande groupée »).",
			"Ajoutez les articles du récapitulatif ci-dessous (ou partagez le lien de la commande groupée Uber Eats aux participants).",
			"Vérifiez l'adresse de livraison puis validez et payez la commande dans Uber Eats.",
			"Revenez ici pour désigner le payeur : chacun verra sa part à rembourser.",
		},
	}
}

type takeaway struct{}

func (takeaway) ID() string    { return Takeaway }
func (takeaway) Name() string  { return "Takeaway.com" }
func (takeaway) Color() string { return "#FF8000" }

func (p takeaway) Dispatch(r Restaurant, s domain.Summary) Dispatch {
	u := r.LinkFor(Takeaway)
	if u == "" {
		u = "https://www.takeaway.com/be-fr"
	}
	return Dispatch{
		Method:   Takeaway,
		URL:      u,
		CartText: CartText(r, s),
		Instructions: []string{
			fmt.Sprintf("Ouvrez la page de « %s » sur Takeaway.com (bouton ci-dessus).", r.Name),
			"Ajoutez au panier les articles du récapitulatif ci-dessous, avec leurs options.",
			"Indiquez les remarques (sans oignon…) dans le champ commentaire de la commande.",
			"Vérifiez l'adresse de livraison puis validez et payez la commande.",
			"Revenez ici pour désigner le payeur : chacun verra sa part à rembourser.",
		},
	}
}

type deliveroo struct{}

func (deliveroo) ID() string    { return Deliveroo }
func (deliveroo) Name() string  { return "Deliveroo" }
func (deliveroo) Color() string { return "#00CCBC" }

func (p deliveroo) Dispatch(r Restaurant, s domain.Summary) Dispatch {
	u := r.LinkFor(Deliveroo)
	if u == "" {
		u = "https://deliveroo.be/fr/"
	}
	return Dispatch{
		Method:   Deliveroo,
		URL:      u,
		CartText: CartText(r, s),
		Instructions: []string{
			fmt.Sprintf("Ouvrez la page de « %s » dans l'app ou le site Deliveroo (bouton ci-dessus).", r.Name),
			"Vérifiez que l'adresse de livraison est la bonne : Deliveroo adapte la carte et les frais à l'adresse.",
			"Ajoutez au panier les articles du récapitulatif ci-dessous, avec leurs options (ou lancez une « commande de groupe » Deliveroo et partagez son lien).",
			"Indiquez les remarques (sans oignon…) dans les instructions pour le restaurant, puis validez et payez la commande.",
			"Revenez ici pour désigner le payeur : chacun verra sa part à rembourser.",
		},
	}
}

type weloveat struct{}

func (weloveat) ID() string    { return Weloveat }
func (weloveat) Name() string  { return "weloveat" }
func (weloveat) Color() string { return "#113B3A" }

func (p weloveat) Dispatch(r Restaurant, s domain.Summary) Dispatch {
	u := r.LinkFor(Weloveat)
	if u == "" {
		u = "https://weloveat.be/restaurants"
	}
	return Dispatch{
		Method:   Weloveat,
		URL:      u,
		CartText: CartText(r, s),
		Instructions: []string{
			fmt.Sprintf("Ouvrez la page de « %s » sur weloveat.be (bouton ci-dessus).", r.Name),
			"Choisissez « Livraison » et saisissez l'adresse de livraison pour vérifier que le restaurant livre chez vous.",
			"Ajoutez au panier les articles du récapitulatif ci-dessous, avec leurs suppléments.",
			"Indiquez les remarques (sans oignon…) dans le commentaire de la commande, puis validez et payez.",
			"Revenez ici pour désigner le payeur : chacun verra sa part à rembourser.",
		},
	}
}

type export struct{}

func (export) ID() string    { return Export }
func (export) Name() string  { return "Export" }
func (export) Color() string { return "#6B7280" }

func (p export) Dispatch(r Restaurant, s domain.Summary) Dispatch {
	return Dispatch{
		Method:   Export,
		URL:      "/api/occ/parties/" + s.PartyID + "/export?format=txt",
		CartText: CartText(r, s),
		Instructions: []string{
			"Téléchargez le récapitulatif (CSV, TXT ou JSON) ou copiez le texte ci-dessous.",
			"Transmettez-le au restaurant ou passez la commande par le canal de votre choix.",
			"Revenez ici pour désigner le payeur.",
		},
	}
}

type phone struct{}

func (phone) ID() string    { return Phone }
func (phone) Name() string  { return "Téléphone" }
func (phone) Color() string { return "#2563EB" }

func (p phone) Dispatch(r Restaurant, s domain.Summary) Dispatch {
	u := ""
	if tel := strings.Map(func(c rune) rune {
		if c == ' ' || c == '.' || c == '/' || c == '-' {
			return -1
		}
		return c
	}, r.Phone); tel != "" {
		u = "tel:" + tel
	}
	steps := []string{}
	if r.Phone != "" {
		steps = append(steps, fmt.Sprintf("Appelez « %s » au %s.", r.Name, domain.FormatPhone(r.Phone)))
	} else {
		steps = append(steps, fmt.Sprintf("Appelez « %s ».", r.Name))
	}
	steps = append(steps,
		"« Bonjour, je souhaite passer une commande en livraison, s'il vous plaît. »",
		"Dictez les articles du récapitulatif ci-dessous, ligne par ligne, avec les remarques.",
		"Donnez l'adresse de livraison et un numéro de téléphone, puis demandez le délai et le montant total.",
		"Revenez ici pour désigner le payeur.",
	)
	return Dispatch{Method: Phone, URL: u, CartText: CartText(r, s), Instructions: steps}
}
