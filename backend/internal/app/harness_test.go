package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
	_ "github.com/fs0ciety7000/occ-deliveries/backend/migrations"
)

// templateDir is a pb_data directory with every migration applied (schema +
// demo seed). Each test app works on its own clone of it.
var templateDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "occ-test-template-*")
	if err != nil {
		panic(err)
	}
	if err := buildTemplate(dir); err != nil {
		_ = os.RemoveAll(dir)
		panic(err)
	}
	templateDir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func buildTemplate(dir string) error {
	ta, err := tests.NewTestApp(dir) // clones dir, bootstraps and runs all migrations
	if err != nil {
		return err
	}
	defer ta.Cleanup()
	// close the db connections (WAL checkpoint) then copy the migrated clone
	// back into the template dir
	if err := ta.ClearBootstrap(); err != nil {
		return err
	}
	return os.CopyFS(dir, os.DirFS(ta.DataDir()))
}

var testConfig = Config{
	Version:      "test",
	PublicURL:    "http://localhost:8090",
	DefaultLat:   50.4542,
	DefaultLng:   3.9567,
	DefaultLabel: "Mons",
	Providers:    []string{providers.UberEats, providers.Takeaway},
}

func newTestApp(t testing.TB) *tests.TestApp {
	t.Helper()
	ta, err := tests.NewTestApp(templateDir)
	if err != nil {
		t.Fatal(err)
	}
	Register(ta, testConfig)
	return ta
}

type env struct {
	t   *testing.T
	app *tests.TestApp
	mux http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ta := newTestApp(t)
	t.Cleanup(ta.Cleanup)

	r, err := apis.NewRouter(ta)
	if err != nil {
		t.Fatal(err)
	}
	se := &core.ServeEvent{App: ta, Router: r}
	var mux http.Handler
	err = ta.OnServe().Trigger(se, func(e *core.ServeEvent) error {
		m, err := e.Router.BuildMux()
		mux = m
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return &env{t: t, app: ta, mux: mux}
}

type resp struct {
	status int
	body   []byte
	header http.Header
}

func (r resp) json(t *testing.T, dst any) {
	t.Helper()
	if err := json.Unmarshal(r.body, dst); err != nil {
		t.Fatalf("invalid json %q: %v", r.body, err)
	}
}

func (r resp) m(t *testing.T) map[string]any {
	t.Helper()
	var out map[string]any
	r.json(t, &out)
	return out
}

func (e *env) do(method, url, token string, body any) resp {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return resp{status: rec.Code, body: rec.Body.Bytes(), header: rec.Header()}
}

// expect performs a request and asserts the status code.
func (e *env) expect(status int, method, url, token string, body any) resp {
	e.t.Helper()
	r := e.do(method, url, token, body)
	if r.status != status {
		e.t.Fatalf("%s %s: expected %d, got %d: %s", method, url, status, r.status, r.body)
	}
	return r
}

type user struct {
	rec   *core.Record
	token string
}

func (u user) id() string { return u.rec.Id }

func (e *env) user(name string) user {
	e.t.Helper()
	col, err := e.app.FindCollectionByNameOrId(colUsers)
	if err != nil {
		e.t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.SetEmail(strings.ToLower(name) + "@example.com")
	r.SetPassword("password123")
	r.Set("name", name)
	if err := e.app.Save(r); err != nil {
		e.t.Fatal(err)
	}
	tok, err := r.NewAuthToken()
	if err != nil {
		e.t.Fatal(err)
	}
	return user{rec: r, token: tok}
}

func boolPtr(b bool) *bool { return &b }

// testRestaurants imports two deterministic restaurants and returns their ids.
func (e *env) testRestaurants() (pizza, burger string) {
	e.t.Helper()
	size := domain.OptionGroup{ID: "size", Name: "Taille", Min: 1, Max: 1, Choices: []domain.OptionChoice{
		{ID: "m", Name: "Moyenne", Price: 0}, {ID: "l", Name: "Large", Price: 300},
	}}
	extras := domain.OptionGroup{ID: "extras", Name: "Suppléments", Min: 0, Max: 2, Choices: []domain.OptionChoice{
		{ID: "cheese", Name: "Fromage", Price: 150}, {ID: "ham", Name: "Jambon", Price: 200},
	}}
	pizza, _, err := catalog.Import(e.app, catalog.RestaurantImport{
		Slug: "test-pizza", Name: "Test Pizza", Rating: 4.0, Lat: 50.4542, Lng: 3.9567,
		DeliveryFee: 299, MinOrder: 1500, Phone: "065 00 00 00",
		Providers: []providers.Link{{ID: providers.UberEats, URL: "https://www.ubereats.com/be/store/test-pizza"}},
		Categories: []catalog.CategoryImport{{Name: "Pizzas", Items: []catalog.ItemImport{
			{Name: "Margherita", Price: 1000, OptionGroups: []domain.OptionGroup{size, extras}},
			{Name: "Tiramisu", Price: 600},
			{Name: "Calzone", Price: 1200, Available: boolPtr(false)},
		}}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	burger, _, err = catalog.Import(e.app, catalog.RestaurantImport{
		Slug: "test-burger", Name: "Test Burger", Rating: 4.9, Lat: 50.455, Lng: 3.95,
		DeliveryFee: 199, MinOrder: 1000,
		Categories: []catalog.CategoryImport{{Name: "Burgers", Items: []catalog.ItemImport{
			{Name: "Cheeseburger", Price: 900},
		}}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return pizza, burger
}

func (e *env) menuItem(restaurantID, name string) string {
	e.t.Helper()
	r, err := e.app.FindFirstRecordByFilter(colMenuItems, "restaurant = {:r} && name = {:n}",
		map[string]any{"r": restaurantID, "n": name})
	if err != nil {
		e.t.Fatalf("menu item %s: %v", name, err)
	}
	return r.Id
}

func pngBytes(t testing.TB) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (e *env) attachFile(rec *core.Record, field, name string, data []byte) {
	e.t.Helper()
	f, err := filesystem.NewFileFromBytes(data, name)
	if err != nil {
		e.t.Fatal(err)
	}
	rec.Set(field, f)
	if err := e.app.Save(rec); err != nil {
		e.t.Fatal(err)
	}
}

func path(format string, args ...any) string { return fmt.Sprintf(format, args...) }

func jsonDecode(res *http.Response, dst any) error {
	return json.NewDecoder(res.Body).Decode(dst)
}

var newRecord = core.NewRecord
