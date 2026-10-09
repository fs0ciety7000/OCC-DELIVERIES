package search_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/search"
	"github.com/fs0ciety7000/occ-deliveries/backend/migrations"
)

var (
	dishes = []string{"pizza", "râmen", "burger", "poulet", "saumon", "tikka", "curry", "margherita", "sushi", "maki",
		"poké", "bowl", "frites", "salade", "soupe", "tacos", "kebab", "falafel", "lasagne", "carbonara", "gyoza",
		"pad thaï", "bibimbap", "croque", "boulets", "mitraillette", "tiramisu", "brownie", "crêpe", "gaufre"}
	adjectives = []string{"épicé", "maison", "végétarien", "croustillant", "fumé", "grillé", "à la truffe", "classique",
		"royal", "double", "du chef", "au fromage", "teriyaki", "piquant", "sucré-salé"}
	words = func() []string {
		r := rand.New(rand.NewPCG(1, 2))
		out := make([]string, 400)
		syll := []string{"ba", "ché", "lo", "mi", "ra", "to", "su", "ka", "pè", "ni", "vo", "zu", "gri", "pla", "fro"}
		for i := range out {
			var b strings.Builder
			for range 2 + r.IntN(3) {
				b.WriteString(syll[r.IntN(len(syll))])
			}
			out[i] = b.String()
		}
		return out
	}()
)

// newCatalogApp builds a migrated app holding nRest restaurants and nItems
// menu items (raw SQL inserts, then a full index rebuild).
func newCatalogApp(tb testing.TB, nRest, nItems int) core.App {
	tb.Helper()
	defer migrations.SetRealDataForTesting([]byte("[]"))()
	dir, err := os.MkdirTemp("", "occ-search-bench-*")
	if err != nil {
		tb.Fatal(err)
	}
	app, err := tests.NewTestApp(dir)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		app.Cleanup()
		_ = os.RemoveAll(dir)
	})
	r := rand.New(rand.NewPCG(42, 7))
	err = app.RunInTransaction(func(tx core.App) error {
		for i := range nRest {
			rid := fmt.Sprintf("benchrest%06d", i)
			name := fmt.Sprintf("%s %s %d", strings.ToUpper(words[r.IntN(len(words))][:1])+words[r.IntN(len(words))], dishes[r.IntN(len(dishes))], i)
			if _, err := tx.DB().Insert("restaurants", dbx.Params{
				"id": rid, "name": name, "slug": fmt.Sprintf("bench-%d", i), "active": true,
				"cuisines": fmt.Sprintf(`["%s","%s"]`, dishes[r.IntN(len(dishes))], dishes[r.IntN(len(dishes))]),
				"address":  fmt.Sprintf("Rue %s %d, 7000 Mons", words[r.IntN(len(words))], i), "items_count": nItems / nRest,
				"lat": 50.45 + r.Float64()/100, "lng": 3.95 + r.Float64()/100,
			}).Execute(); err != nil {
				return err
			}
			cid := fmt.Sprintf("benchcat%07d", i)
			if _, err := tx.DB().Insert("menu_categories", dbx.Params{"id": cid, "restaurant": rid, "name": dishes[r.IntN(len(dishes))] + "s"}).Execute(); err != nil {
				return err
			}
		}
		for i := range nItems {
			ri := i % nRest
			desc := make([]string, 10)
			for k := range desc {
				desc[k] = words[r.IntN(len(words))]
			}
			if _, err := tx.DB().Insert("menu_items", dbx.Params{
				"id":          fmt.Sprintf("benchitem%06d", i),
				"restaurant":  fmt.Sprintf("benchrest%06d", ri),
				"category":    fmt.Sprintf("benchcat%07d", ri),
				"name":        dishes[r.IntN(len(dishes))] + " " + adjectives[r.IntN(len(adjectives))],
				"description": strings.Join(desc, " "),
				"price":       500 + r.IntN(1500),
				"available":   r.IntN(10) > 0,
			}).Execute(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	if err := search.Rebuild(app); err != nil {
		tb.Fatal(err)
	}
	return app
}

var benchQueries = []string{"ram", "râmen", "pizza margherita", "piza", "poulet épicé", "RAM", "croustillant", "ba", "tiramissu", "rue"}

func BenchmarkSearch10k(b *testing.B) {
	app := newCatalogApp(b, 200, 10_000)
	for _, q := range benchQueries {
		b.Run(q, func(b *testing.B) {
			p := search.Params{Query: q, Lat: 50.45, Lng: 3.95, HasGeo: true}
			for b.Loop() {
				if _, err := search.Search(app, p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestSearchLatency10k checks the budget (< 50 ms per query for 10k items)
// on the median, which is robust to a loaded CI machine; skipped with -short.
func TestSearchLatency10k(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	start := time.Now()
	app := newCatalogApp(t, 200, 10_000)
	t.Logf("catalogue + index of 10k items built in %s", time.Since(start).Round(time.Millisecond))
	if _, items, _ := search.Counts(app); items < 10_000 { // + the demo seed
		t.Fatalf("indexed %d items", items)
	}
	for _, q := range benchQueries {
		var durs []time.Duration
		var res *search.Result
		for range 15 {
			t0 := time.Now()
			r, err := search.Search(app, search.Params{Query: q})
			if err != nil {
				t.Fatal(err)
			}
			durs = append(durs, time.Since(t0))
			res = r
		}
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		med := durs[len(durs)/2]
		t.Logf("%-18q median %6s  max %6s  (%d restaurants, %d dishes, fuzzy=%v)", q, med.Round(10*time.Microsecond),
			durs[len(durs)-1].Round(10*time.Microsecond), len(res.Restaurants), len(res.Dishes), res.Fuzzy)
		if med > 50*time.Millisecond {
			t.Errorf("%q: median %s over the 50 ms budget", q, med)
		}
	}
}
