package catalog

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// Report is the result of a (dry run) import: what would be / was written and
// every validation problem.
type Report struct {
	DryRun bool `json:"dryRun"`
	// Valid is true when nothing blocks the import.
	Valid bool `json:"valid"`
	// Errors are global problems (unreadable file, CSV lines…).
	Errors      []string           `json:"errors"`
	Restaurants []RestaurantReport `json:"restaurants"`
	// Items is the total number of menu items.
	Items int `json:"items"`
}

// RestaurantReport describes one restaurant of an import.
type RestaurantReport struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Exists tells whether the slug is already in the catalogue (update).
	Exists     bool          `json:"exists"`
	Active     bool          `json:"active"`
	Categories int           `json:"categories"`
	Items      int           `json:"items"`
	Errors     []string      `json:"errors"`
	Warnings   []string      `json:"warnings"`
	Menu       []PreviewItem `json:"menu"`
	// Restaurant is the record id once imported.
	Restaurant string `json:"restaurant,omitempty"`
}

// PreviewItem is one menu line of the preview table.
type PreviewItem struct {
	Category  string   `json:"category"`
	Name      string   `json:"name"`
	Price     int      `json:"price"`
	Tags      []string `json:"tags"`
	Popular   bool     `json:"popular"`
	Available bool     `json:"available"`
	Options   int      `json:"options"`
}

// Check validates a batch without writing and builds its report.
func Check(app core.App, list []RestaurantImport) Report {
	rep := Report{DryRun: true, Errors: []string{}, Restaurants: []RestaurantReport{}}
	if len(list) == 0 {
		rep.Errors = append(rep.Errors, "Aucun restaurant à importer.")
	}
	seen := map[string]int{}
	for i := range list {
		in := &list[i]
		rr := RestaurantReport{
			Errors: in.Problems(), Warnings: []string{}, Menu: []PreviewItem{},
			Slug: in.Slug, Name: in.Name, Categories: len(in.Categories), Items: in.ItemCount(),
			Active: in.Active == nil || *in.Active,
		}
		if prev, dup := seen[in.Slug]; dup && in.Slug != "" {
			rr.Errors = append(rr.Errors, fmt.Sprintf("Slug « %s » en double (déjà utilisé par le restaurant n° %d).", in.Slug, prev+1))
		}
		seen[in.Slug] = i
		if in.Slug != "" {
			if _, err := app.FindFirstRecordByData(Restaurants, "slug", in.Slug); err == nil {
				rr.Exists = true
			}
		}
		switch {
		case in.Lat == 0 && in.Lng == 0 && rr.Exists:
			rr.Warnings = append(rr.Warnings, "Coordonnées manquantes : celles déjà enregistrées sont conservées.")
		case in.Lat == 0 && in.Lng == 0:
			rr.Warnings = append(rr.Warnings, "Coordonnées manquantes : le restaurant n'apparaîtra pas dans « À proximité » (géocode l'adresse dans /admin).")
		}
		if strings.TrimSpace(in.Address) == "" && !rr.Exists {
			rr.Warnings = append(rr.Warnings, "Adresse manquante.")
		}
		if rr.Items == 0 {
			rr.Warnings = append(rr.Warnings, "Menu vide.")
		}
		if len(in.Providers) == 0 {
			rr.Warnings = append(rr.Warnings, "Aucun lien Uber Eats / Takeaway.")
		}
		for _, c := range in.Categories {
			for _, it := range c.Items {
				rr.Menu = append(rr.Menu, PreviewItem{
					Category: c.Name, Name: it.Name, Price: it.Price, Tags: orEmpty(it.Tags),
					Popular: it.Popular, Available: it.Available == nil || *it.Available, Options: len(it.OptionGroups),
				})
			}
		}
		rep.Items += rr.Items
		rep.Restaurants = append(rep.Restaurants, rr)
	}
	rep.Valid = rep.isValid()
	return rep
}

func (r *Report) isValid() bool {
	if len(r.Errors) > 0 {
		return false
	}
	for _, rr := range r.Restaurants {
		if len(rr.Errors) > 0 {
			return false
		}
	}
	return true
}

// ErrorCount is the total number of blocking problems.
func (r *Report) ErrorCount() int {
	n := len(r.Errors)
	for _, rr := range r.Restaurants {
		n += len(rr.Errors)
	}
	return n
}

// ImportAll validates the batch and, when everything is valid, imports it in
// a single transaction (all or nothing). The report carries the record ids.
func ImportAll(app core.App, list []RestaurantImport) (Report, error) {
	rep := Check(app, list)
	rep.DryRun = false
	if !rep.Valid {
		return rep, nil
	}
	err := app.RunInTransaction(func(tx core.App) error {
		for i, in := range list {
			id, _, err := Import(tx, in)
			if err != nil {
				return fmt.Errorf("%s : %w", in.Slug, err)
			}
			rep.Restaurants[i].Restaurant = id
		}
		return nil
	})
	return rep, err
}
