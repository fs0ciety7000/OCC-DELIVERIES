package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// 1760000008 folds an Uber Eats preview into the full restaurant of the same
// store (« Altaj (Quaregnon) » → « Altaj ») and leaves unrelated previews alone.
func TestMergePartialDuplicates(t *testing.T) {
	app := newApp(t)
	col, err := app.FindCollectionByNameOrId("restaurants")
	if err != nil {
		t.Fatal(err)
	}
	mk := func(slug, name string, partial bool, rating float64, links string) *core.Record {
		r := core.NewRecord(col)
		r.Load(map[string]any{"slug": slug, "name": name, "active": true, "partial_menu": partial,
			"rating": rating, "price_level": 2, "providers": links})
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	full := mk("altaj", "Altaj", false, 0, `[{"id":"weloveat","url":"https://weloveat.be/altaj"}]`)
	dup := mk("altaj-quaregnon", "Altaj (Quaregnon)", true, 4.4, `[{"id":"ubereats","url":"https://www.ubereats.com/store/altaj-quaregnon/x"}]`)
	alone := mk("pizza-hut", "Pizza Hut", true, 4.5, `[{"id":"ubereats","url":"https://www.ubereats.com/store/pizza-hut-mons/y"}]`)

	if err := mergePartialDuplicates(app); err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindRecordById("restaurants", dup.Id); err == nil {
		t.Fatal("duplicate preview must be removed")
	}
	got, _ := app.FindRecordById("restaurants", full.Id)
	if got.GetFloat("rating") != 4.4 {
		t.Fatalf("full restaurant not completed: %v", got.FieldsData())
	}
	var links []map[string]string
	_ = got.UnmarshalJSONField("providers", &links)
	if len(links) != 2 {
		t.Fatalf("links %v", links)
	}
	if _, err := app.FindRecordById("restaurants", alone.Id); err != nil {
		t.Fatal("unrelated preview must stay")
	}
}
