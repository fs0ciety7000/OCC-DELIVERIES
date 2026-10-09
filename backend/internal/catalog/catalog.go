// Package catalog imports restaurants and their menus (upsert by slug).
// It is shared by the admin import endpoint and the demo seed migration.
package catalog

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// Collection names.
const (
	Restaurants    = "restaurants"
	MenuCategories = "menu_categories"
	MenuItems      = "menu_items"
)

// RestaurantImport is the JSON accepted by POST /api/occ/admin/import.
type RestaurantImport struct {
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Emoji       string           `json:"emoji"`
	CoverURL    string           `json:"cover_url"`
	Cuisines    []string         `json:"cuisines"`
	Address     string           `json:"address"`
	Lat         float64          `json:"lat"`
	Lng         float64          `json:"lng"`
	Phone       string           `json:"phone"`
	Rating      float64          `json:"rating"`
	RatingCount int              `json:"rating_count"`
	PriceLevel  int              `json:"price_level"`
	EtaMin      int              `json:"eta_min"`
	EtaMax      int              `json:"eta_max"`
	DeliveryFee int              `json:"delivery_fee"`
	MinOrder    int              `json:"min_order"`
	Providers   []providers.Link `json:"providers"`
	Active      *bool            `json:"active"`
	Categories  []CategoryImport `json:"categories"`
}

// CategoryImport is a menu category with its items.
type CategoryImport struct {
	Name  string       `json:"name"`
	Items []ItemImport `json:"items"`
}

// ItemImport is a menu item.
type ItemImport struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Price        int                  `json:"price"`
	Emoji        string               `json:"emoji"`
	Tags         []string             `json:"tags"`
	OptionGroups []domain.OptionGroup `json:"option_groups"`
	Popular      bool                 `json:"popular"`
	Available    *bool                `json:"available"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Validate checks the import payload (French messages) and returns the first problem.
func (in *RestaurantImport) Validate() error {
	if p := in.Problems(); len(p) > 0 {
		return domain.Errf("%s", p[0])
	}
	return nil
}

// Problems normalizes the payload (slug, name) and lists every validation
// problem (French messages). An empty list means the payload is importable.
func (in *RestaurantImport) Problems() []string {
	in.Slug = strings.TrimSpace(strings.ToLower(in.Slug))
	in.Name = strings.TrimSpace(in.Name)
	out := []string{}
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	if !slugRe.MatchString(in.Slug) || len(in.Slug) > 120 {
		add("Slug invalide (lettres minuscules, chiffres et tirets).")
	}
	if in.Name == "" {
		add("Le nom du restaurant est requis.")
	} else if utf8.RuneCountInString(in.Name) > 120 {
		add("Le nom du restaurant est trop long (120 caractères maximum).")
	}
	if in.Rating < 0 || in.Rating > 5 {
		add("La note doit être comprise entre 0 et 5.")
	}
	if in.PriceLevel != 0 && (in.PriceLevel < 1 || in.PriceLevel > 4) {
		add("Le niveau de prix doit être compris entre 1 et 4.")
	}
	if in.DeliveryFee < 0 || in.MinOrder < 0 || in.EtaMin < 0 || in.EtaMax < 0 || in.RatingCount < 0 {
		add("Les montants et durées ne peuvent pas être négatifs.")
	}
	if in.Lat < -90 || in.Lat > 90 || in.Lng < -180 || in.Lng > 180 {
		add("Coordonnées GPS invalides.")
	}
	for _, p := range in.Providers {
		if p.ID != providers.UberEats && p.ID != providers.Takeaway {
			add("Fournisseur inconnu : %q.", p.ID)
		}
	}
	for _, c := range in.Categories {
		if strings.TrimSpace(c.Name) == "" {
			add("Catégorie sans nom.")
		}
		for _, it := range c.Items {
			if strings.TrimSpace(it.Name) == "" {
				add("Article sans nom dans « %s ».", c.Name)
				continue
			}
			if it.Price < 0 {
				add("Prix négatif pour « %s ».", it.Name)
			}
			if err := domain.ValidateOptionGroups(it.OptionGroups); err != nil {
				add("« %s » : %s", it.Name, err.Error())
			}
		}
	}
	return out
}

// ItemCount is the number of menu items of the payload.
func (in *RestaurantImport) ItemCount() int {
	n := 0
	for _, c := range in.Categories {
		n += len(c.Items)
	}
	return n
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Import upserts a restaurant by slug and replaces its whole menu.
// It returns the restaurant id and the number of imported items.
func Import(app core.App, in RestaurantImport) (string, int, error) {
	if err := in.Validate(); err != nil {
		return "", 0, err
	}

	var id string
	count := 0
	err := app.RunInTransaction(func(tx core.App) error {
		rest, err := tx.FindFirstRecordByData(Restaurants, "slug", in.Slug)
		if err != nil {
			col, err := tx.FindCollectionByNameOrId(Restaurants)
			if err != nil {
				return err
			}
			rest = core.NewRecord(col)
		}
		// A partial payload (e.g. the Uber Eats / Takeaway export tool) carries
		// no coordinates / address: keep the ones already known.
		if !rest.IsNew() {
			if in.Lat == 0 && in.Lng == 0 {
				in.Lat, in.Lng = rest.GetFloat("lat"), rest.GetFloat("lng")
			}
			if strings.TrimSpace(in.Address) == "" {
				in.Address = rest.GetString("address")
			}
		}

		active := true
		if in.Active != nil {
			active = *in.Active
		}
		priceLevel := in.PriceLevel
		if priceLevel == 0 {
			priceLevel = 2
		}
		rest.Load(map[string]any{
			"slug":         in.Slug,
			"name":         in.Name,
			"description":  in.Description,
			"emoji":        in.Emoji,
			"cover_url":    in.CoverURL,
			"cuisines":     orEmpty(in.Cuisines),
			"address":      in.Address,
			"lat":          in.Lat,
			"lng":          in.Lng,
			"phone":        in.Phone,
			"rating":       in.Rating,
			"rating_count": in.RatingCount,
			"price_level":  priceLevel,
			"eta_min":      in.EtaMin,
			"eta_max":      in.EtaMax,
			"delivery_fee": in.DeliveryFee,
			"min_order":    in.MinOrder,
			"providers":    orEmpty(in.Providers),
			"active":       active,
		})
		if err := tx.Save(rest); err != nil {
			return err
		}
		id = rest.Id

		// replace the menu
		oldItems, err := tx.FindAllRecords(MenuItems, dbx.HashExp{"restaurant": rest.Id})
		if err != nil {
			return err
		}
		for _, r := range oldItems {
			if err := tx.Delete(r); err != nil {
				return err
			}
		}
		oldCats, err := tx.FindAllRecords(MenuCategories, dbx.HashExp{"restaurant": rest.Id})
		if err != nil {
			return err
		}
		for _, r := range oldCats {
			if err := tx.Delete(r); err != nil {
				return err
			}
		}

		catCol, err := tx.FindCollectionByNameOrId(MenuCategories)
		if err != nil {
			return err
		}
		itemCol, err := tx.FindCollectionByNameOrId(MenuItems)
		if err != nil {
			return err
		}
		for ci, c := range in.Categories {
			cat := core.NewRecord(catCol)
			cat.Load(map[string]any{"restaurant": rest.Id, "name": strings.TrimSpace(c.Name), "position": ci})
			if err := tx.Save(cat); err != nil {
				return err
			}
			for ii, it := range c.Items {
				available := true
				if it.Available != nil {
					available = *it.Available
				}
				item := core.NewRecord(itemCol)
				item.Load(map[string]any{
					"restaurant":    rest.Id,
					"category":      cat.Id,
					"name":          strings.TrimSpace(it.Name),
					"description":   it.Description,
					"price":         it.Price,
					"emoji":         it.Emoji,
					"tags":          orEmpty(it.Tags),
					"option_groups": orEmpty(it.OptionGroups),
					"popular":       it.Popular,
					"available":     available,
					"position":      ii,
				})
				if err := tx.Save(item); err != nil {
					return err
				}
				count++
			}
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	return id, count, nil
}
