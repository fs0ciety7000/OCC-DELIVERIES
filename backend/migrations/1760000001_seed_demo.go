package migrations

import (
	"os"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// Demo restaurants are FICTIONAL. Provider URLs point to the generic Mons
// listing pages of each platform.
const (
	uberEatsMons = "https://www.ubereats.com/be/city/mons-wal"
	takeawayMons = "https://www.takeaway.com/be-fr/livraison/repas/mons-7000"
)

func init() {
	m.Register(upSeed, downSeed)
}

// SeedEnabled reports whether the demo seed should run (OCC_SEED_DEMO != "false").
func SeedEnabled() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("OCC_SEED_DEMO")), "false")
}

func upSeed(app core.App) error {
	// Fresh installs with real data (migrations/data) skip the fictional demo:
	// 1760000003_replace_demo imports the real restaurants instead.
	if !SeedEnabled() || HasRealData() {
		return nil
	}
	for _, r := range DemoRestaurants() {
		if _, _, err := catalog.Import(app, r); err != nil {
			return err
		}
	}
	return nil
}

func downSeed(app core.App) error {
	for _, r := range DemoRestaurants() {
		rec, err := app.FindFirstRecordByData(catalog.Restaurants, "slug", r.Slug)
		if err != nil {
			continue
		}
		// keep restaurants already used by a party
		if n, _ := app.CountRecords("parties", dbx.Or(dbx.HashExp{"restaurant": rec.Id}, dbx.Like("candidates", rec.Id))); n > 0 {
			continue
		}
		if err := app.Delete(rec); err != nil {
			return err
		}
	}
	return nil
}

// --- small builders -------------------------------------------------------

func links(both bool) []providers.Link {
	l := []providers.Link{{ID: providers.UberEats, URL: uberEatsMons}}
	if both {
		l = append(l, providers.Link{ID: providers.Takeaway, URL: takeawayMons})
	}
	return l
}

func ch(id, name string, price int) domain.OptionChoice {
	return domain.OptionChoice{ID: id, Name: name, Price: price}
}

func grp(id, name string, min, max int, choices ...domain.OptionChoice) domain.OptionGroup {
	return domain.OptionGroup{ID: id, Name: name, Min: min, Max: max, Choices: choices}
}

func it(name, desc string, price int, emoji string, tags []string, popular bool, groups ...domain.OptionGroup) catalog.ItemImport {
	if tags == nil {
		tags = []string{}
	}
	return catalog.ItemImport{Name: name, Description: desc, Price: price, Emoji: emoji, Tags: tags, Popular: popular, OptionGroups: groups}
}

func tags(t ...string) []string { return t }

// DemoRestaurants returns the fictional demo catalogue around Mons (BE).
func DemoRestaurants() []catalog.RestaurantImport {
	pizzaSize := grp("size", "Taille", 1, 1, ch("m", "Moyenne (29 cm)", 0), ch("l", "Large (34 cm)", 300))
	pizzaExtras := grp("extras", "Suppléments", 0, 4,
		ch("mozza", "Mozzarella", 150), ch("burrata", "Burrata", 350), ch("jambon", "Jambon cuit", 200),
		ch("champignons", "Champignons", 100), ch("piment", "Huile pimentée", 50))
	burgerCuisson := grp("cuisson", "Cuisson", 1, 1, ch("rose", "Rosé", 0), ch("apoint", "À point", 0), ch("bien", "Bien cuit", 0))
	burgerExtras := grp("extras", "Suppléments", 0, 3,
		ch("bacon", "Bacon", 150), ch("cheddar", "Cheddar", 100), ch("oeuf", "Œuf", 100), ch("jalapenos", "Jalapeños", 80))
	burgerMenu := grp("formule", "Formule", 1, 1, ch("seul", "Burger seul", 0), ch("menu", "Menu frites + boisson", 450))
	friesSauce := grp("sauce", "Sauce", 0, 2,
		ch("mayo", "Mayonnaise", 80), ch("andalouse", "Andalouse", 80), ch("samourai", "Samouraï", 80),
		ch("americaine", "Américaine", 80), ch("tartare", "Tartare", 80), ch("ketchup", "Ketchup", 60))
	friesSize := grp("size", "Portion", 1, 1, ch("petite", "Petite", 0), ch("moyenne", "Moyenne", 100), ch("grande", "Grande", 200))
	pokeBase := grp("base", "Base", 1, 1, ch("riz", "Riz vinaigré", 0), ch("quinoa", "Quinoa", 0), ch("salade", "Mesclun", 0), ch("mix", "Moitié riz / moitié salade", 0))
	pokeToppings := grp("toppings", "Toppings", 0, 3,
		ch("avocat", "Avocat", 150), ch("edamame", "Edamame", 100), ch("mangue", "Mangue", 100), ch("wakame", "Wakame", 100), ch("oignons", "Oignons frits", 50))
	pokeSauce := grp("sauce", "Sauce", 1, 1, ch("soja", "Soja sucrée", 0), ch("sriracha", "Mayo sriracha", 0), ch("ponzu", "Ponzu", 0), ch("sesame", "Sésame", 0))
	spice := grp("piquant", "Niveau de piquant", 1, 1, ch("doux", "Doux", 0), ch("moyen", "Moyen", 0), ch("fort", "Fort 🌶️", 0))
	naan := grp("accomp", "Accompagnement", 0, 2, ch("riz", "Riz basmati", 250), ch("naan", "Naan nature", 250), ch("garlic", "Naan à l'ail", 300), ch("cheese", "Cheese naan", 350))
	thaiProt := grp("proteine", "Protéine", 1, 1, ch("poulet", "Poulet", 0), ch("tofu", "Tofu", 0), ch("boeuf", "Bœuf", 150), ch("crevettes", "Crevettes", 250))
	saladProt := grp("proteine", "Protéine", 0, 1, ch("poulet", "Poulet grillé", 250), ch("saumon", "Saumon fumé", 350), ch("falafel", "Falafels", 200), ch("halloumi", "Halloumi", 250))
	dressing := grp("dressing", "Vinaigrette", 1, 1, ch("balsamique", "Balsamique", 0), ch("cesar", "César", 0), ch("miel", "Miel-moutarde", 0), ch("citron", "Citron-huile d'olive", 0))
	sushiSauce := grp("extras", "Extras", 0, 3, ch("gingembre", "Gingembre en plus", 50), ch("wasabi", "Wasabi en plus", 50), ch("soja", "Sauce soja sucrée", 50))
	mezzeSauce := grp("sauce", "Sauce", 0, 2, ch("tahini", "Tahiné", 0), ch("toum", "Toum (ail)", 0), ch("harissa", "Harissa", 0))
	drinkSize := grp("size", "Format", 1, 1, ch("33", "33 cl", 0), ch("50", "50 cl", 100))

	return []catalog.RestaurantImport{
		{
			Slug: "la-bella-nonna", Name: "La Bella Nonna", Emoji: "🍕",
			Description: "Pizzas napolitaines au feu de bois et pâtes fraîches, comme chez la nonna.",
			Cuisines:    []string{"pizza", "italien"}, Address: "Rue de la Chaussée 41, 7000 Mons",
			Lat: 50.4538, Lng: 3.9531, Phone: "065 00 11 01", Rating: 4.6, RatingCount: 842, PriceLevel: 2,
			EtaMin: 25, EtaMax: 40, DeliveryFee: 299, MinOrder: 1500, Providers: links(true),
			Categories: []catalog.CategoryImport{
				{Name: "Pizzas", Items: []catalog.ItemImport{
					it("Margherita", "Sauce tomate San Marzano, fior di latte, basilic frais.", 1050, "🍕", tags("veggie"), true, pizzaSize, pizzaExtras),
					it("Regina", "Tomate, mozzarella, jambon cuit, champignons de Paris.", 1350, "🍕", nil, true, pizzaSize, pizzaExtras),
					it("Diavola", "Tomate, mozzarella, salami piquant, huile pimentée.", 1400, "🌶️", tags("spicy"), false, pizzaSize, pizzaExtras),
					it("Quattro formaggi", "Mozzarella, gorgonzola, taleggio, parmesan.", 1450, "🧀", tags("veggie"), false, pizzaSize, pizzaExtras),
					it("Burrata e pesto", "Base pesto, tomates cerises, burrata entière, roquette.", 1650, "🍕", tags("veggie", "new"), false, pizzaSize),
				}},
				{Name: "Pâtes", Items: []catalog.ItemImport{
					it("Spaghetti carbonara", "Guanciale, jaune d'œuf, pecorino, poivre noir.", 1450, "🍝", nil, true),
					it("Penne all'arrabbiata", "Sauce tomate relevée à l'ail et au piment.", 1250, "🍝", tags("vegan", "spicy"), false),
				}},
				{Name: "Desserts & boissons", Items: []catalog.ItemImport{
					it("Tiramisu maison", "Mascarpone, café, biscuits savoiardi.", 650, "🍰", tags("veggie"), true),
					it("San Pellegrino", "Eau pétillante.", 300, "💧", tags("vegan"), false, drinkSize),
				}},
			},
		},
		{
			Slug: "le-comptoir-du-burger", Name: "Le Comptoir du Burger", Emoji: "🍔",
			Description: "Smash burgers de bœuf belge, pains briochés du boulanger d'à côté.",
			Cuisines:    []string{"burger", "américain"}, Address: "Rue d'Havré 88, 7000 Mons",
			Lat: 50.4561, Lng: 3.9614, Phone: "065 00 11 02", Rating: 4.4, RatingCount: 1290, PriceLevel: 2,
			EtaMin: 20, EtaMax: 35, DeliveryFee: 249, MinOrder: 1200, Providers: links(true),
			Categories: []catalog.CategoryImport{
				{Name: "Burgers", Items: []catalog.ItemImport{
					it("Classic smash", "Double steak smashé, cheddar, oignons, pickles, sauce maison.", 1290, "🍔", nil, true, burgerCuisson, burgerMenu, burgerExtras),
					it("Bacon lover", "Steak, bacon croustillant, cheddar affiné, sauce barbecue.", 1490, "🥓", nil, true, burgerCuisson, burgerMenu, burgerExtras),
					it("Chicken crispy", "Filet de poulet pané, coleslaw, mayo épicée.", 1350, "🍗", tags("spicy"), false, burgerMenu, burgerExtras),
					it("Veggie forestier", "Galette champignons-lentilles, comté, roquette.", 1350, "🥬", tags("veggie"), false, burgerMenu, burgerExtras),
				}},
				{Name: "Accompagnements", Items: []catalog.ItemImport{
					it("Frites maison", "Pommes de terre belges, double cuisson.", 400, "🍟", tags("vegan"), true, friesSauce),
					it("Onion rings", "Rondelles d'oignon panées (8 pièces).", 550, "🧅", tags("veggie"), false),
				}},
				{Name: "Boissons", Items: []catalog.ItemImport{
					it("Cola", "Canette 33 cl.", 290, "🥤", tags("vegan"), false),
					it("Limonade maison", "Citron, menthe, sucre de canne.", 390, "🍋", tags("vegan", "new"), false),
				}},
			},
		},
		{
			Slug: "maison-hanami", Name: "Maison Hanami", Emoji: "🍣",
			Description: "Sushis, makis et plateaux préparés à la minute.",
			Cuisines:    []string{"sushi", "japonais"}, Address: "Rue de Nimy 17, 7000 Mons",
			Lat: 50.4579, Lng: 3.9522, Phone: "065 00 11 03", Rating: 4.7, RatingCount: 615, PriceLevel: 3,
			EtaMin: 30, EtaMax: 45, DeliveryFee: 349, MinOrder: 2000, Providers: links(true),
			Categories: []catalog.CategoryImport{
				{Name: "Plateaux", Items: []catalog.ItemImport{
					it("Plateau Hanami (18 pièces)", "6 California saumon, 6 makis thon, 6 sushis assortis.", 2390, "🍱", nil, true, sushiSauce),
					it("Plateau veggie (16 pièces)", "Makis concombre, avocat, inari et California veggie.", 1790, "🥒", tags("veggie"), false, sushiSauce),
				}},
				{Name: "Makis & California", Items: []catalog.ItemImport{
					it("California saumon avocat (8)", "Saumon, avocat, sésame grillé.", 990, "🍣", nil, true, sushiSauce),
					it("Spicy tuna roll (8)", "Thon, mayo sriracha, ciboulette.", 1090, "🌶️", tags("spicy"), false, sushiSauce),
					it("Dragon roll crevette (8)", "Crevette tempura, avocat, sauce unagi.", 1290, "🍤", tags("new"), false, sushiSauce),
				}},
				{Name: "Entrées", Items: []catalog.ItemImport{
					it("Soupe miso", "Tofu, wakame, ciboule.", 400, "🥣", tags("veggie"), false),
					it("Gyoza poulet (5)", "Raviolis grillés, sauce ponzu.", 650, "🥟", nil, true),
					it("Edamame", "Fleur de sel.", 450, "🫛", tags("vegan", "gluten_free"), false),
				}},
			},
		},
		{
			Slug: "poke-lagon", Name: "Poké Lagon", Emoji: "🥗",
			Description: "Poké bowls frais à composer, poissons marinés et légumes croquants.",
			Cuisines:    []string{"poke", "hawaïen", "healthy"}, Address: "Boulevard Dolez 12, 7000 Mons",
			Lat: 50.4497, Lng: 3.9489, Phone: "065 00 11 04", Rating: 4.5, RatingCount: 388, PriceLevel: 2,
			EtaMin: 20, EtaMax: 35, DeliveryFee: 199, MinOrder: 1200, Providers: links(false),
			Categories: []catalog.CategoryImport{
				{Name: "Bowls signature", Items: []catalog.ItemImport{
					it("Bowl saumon", "Saumon mariné, concombre, radis, sésame.", 1390, "🐟", nil, true, pokeBase, pokeToppings, pokeSauce),
					it("Bowl thon spicy", "Thon rouge, mayo sriracha, oignons frits.", 1490, "🌶️", tags("spicy"), true, pokeBase, pokeToppings, pokeSauce),
					it("Bowl poulet teriyaki", "Poulet grillé, carottes, chou rouge.", 1290, "🍗", nil, false, pokeBase, pokeToppings, pokeSauce),
					it("Bowl tofu croustillant", "Tofu pané, mangue, edamame.", 1250, "🥢", tags("vegan"), false, pokeBase, pokeToppings, pokeSauce),
				}},
				{Name: "Extras", Items: []catalog.ItemImport{
					it("Mochi glacé (2)", "Mangue et matcha.", 500, "🍡", tags("veggie"), false),
					it("Thé glacé maison", "Hibiscus et citron vert.", 350, "🧋", tags("vegan"), false),
				}},
			},
		},
		{
			Slug: "les-jardins-du-cedre", Name: "Les Jardins du Cèdre", Emoji: "🧆",
			Description: "Cuisine libanaise familiale : mezzés, grillades et pâtisseries orientales.",
			Cuisines:    []string{"libanais", "méditerranéen"}, Address: "Rue des Fripiers 5, 7000 Mons",
			Lat: 50.4525, Lng: 3.9580, Phone: "065 00 11 05", Rating: 4.8, RatingCount: 507, PriceLevel: 2,
			EtaMin: 25, EtaMax: 40, DeliveryFee: 250, MinOrder: 1500, Providers: links(true),
			Categories: []catalog.CategoryImport{
				{Name: "Mezzés", Items: []catalog.ItemImport{
					it("Houmous", "Purée de pois chiches, tahiné, citron, pain pita.", 650, "🫓", tags("vegan"), true),
					it("Falafels (6)", "Pois chiches et fèves, herbes fraîches.", 750, "🧆", tags("vegan"), true, mezzeSauce),
					it("Taboulé libanais", "Persil, menthe, boulgour, tomate, citron.", 650, "🌿", tags("vegan"), false),
				}},
				{Name: "Sandwichs & assiettes", Items: []catalog.ItemImport{
					it("Shawarma poulet (wrap)", "Poulet mariné, toum, pickles, frites.", 950, "🌯", nil, true, mezzeSauce),
					it("Assiette mixte grill", "Kafta, chich taouk, riz, salade, houmous.", 1790, "🍢", nil, false),
					it("Assiette végétarienne", "Falafels, houmous, moutabal, taboulé, pita.", 1490, "🥙", tags("veggie"), false),
				}},
				{Name: "Douceurs", Items: []catalog.ItemImport{
					it("Baklava (4)", "Pistache et fleur d'oranger.", 550, "🍯", tags("veggie"), false),
				}},
			},
		},
		{
			Slug: "saveurs-du-gange", Name: "Saveurs du Gange", Emoji: "🍛",
			Description: "Currys mijotés, tandoori et naans cuits au four traditionnel.",
			Cuisines:    []string{"indien", "curry"}, Address: "Avenue de Jemappes 140, 7000 Mons",
			Lat: 50.4486, Lng: 3.9376, Phone: "065 00 11 06", Rating: 4.3, RatingCount: 451, PriceLevel: 2,
			EtaMin: 35, EtaMax: 50, DeliveryFee: 299, MinOrder: 1800, Providers: links(true),
			Categories: []catalog.CategoryImport{
				{Name: "Currys", Items: []catalog.ItemImport{
					it("Butter chicken", "Poulet tandoori, sauce tomate beurrée et crème.", 1550, "🍛", tags("gluten_free"), true, spice, naan),
					it("Lamb rogan josh", "Agneau mijoté aux épices du Cachemire.", 1750, "🍖", tags("spicy"), false, spice, naan),
					it("Chana masala", "Pois chiches, tomate, gingembre, coriandre.", 1250, "🫘", tags("vegan"), false, spice, naan),
					it("Palak paneer", "Épinards et fromage frais indien.", 1350, "🥬", tags("veggie"), false, spice, naan),
				}},
				{Name: "Entrées & pains", Items: []catalog.ItemImport{
					it("Samosas légumes (3)", "Chaussons croustillants, chutney tamarin.", 600, "🥟", tags("vegan"), true),
					it("Cheese naan", "Naan fourré au fromage fondu.", 400, "🫓", tags("veggie"), true),
				}},
				{Name: "Boissons", Items: []catalog.ItemImport{
					it("Lassi mangue", "Yaourt, mangue, cardamome.", 450, "🥭", tags("veggie"), false),
				}},
			},
		},
		{
			Slug: "petit-bangkok", Name: "Petit Bangkok", Emoji: "🍜",
			Description: "Street food thaïlandaise au wok : pad thaï, currys verts, soupes.",
			Cuisines:    []string{"thaï", "asiatique"}, Address: "Rue de Bertaimont 60, 7000 Mons",
			Lat: 50.4479, Lng: 3.9591, Phone: "065 00 11 07", Rating: 4.5, RatingCount: 702, PriceLevel: 2,
			EtaMin: 25, EtaMax: 40, DeliveryFee: 279, MinOrder: 1500, Providers: links(true),
			Categories: []catalog.CategoryImport{
				{Name: "Wok", Items: []catalog.ItemImport{
					it("Pad thaï", "Nouilles de riz sautées, cacahuètes, citron vert, pousses de soja.", 1390, "🍜", nil, true, thaiProt, spice),
					it("Riz sauté à l'ananas", "Riz jasmin, ananas, noix de cajou, curry doux.", 1290, "🍍", nil, false, thaiProt),
					it("Nouilles drunken", "Larges nouilles, basilic thaï, piment frais.", 1390, "🌶️", tags("spicy"), false, thaiProt, spice),
				}},
				{Name: "Currys & soupes", Items: []catalog.ItemImport{
					it("Curry vert", "Lait de coco, aubergines thaï, basilic, riz jasmin.", 1490, "🥥", tags("gluten_free"), true, thaiProt, spice),
					it("Tom yum crevettes", "Soupe acidulée à la citronnelle et galanga.", 990, "🍲", tags("spicy"), false),
				}},
				{Name: "Entrées", Items: []catalog.ItemImport{
					it("Rouleaux de printemps (4)", "Légumes croquants, sauce cacahuète.", 650, "🥬", tags("vegan"), true),
					it("Satay de poulet (4)", "Brochettes marinées, sauce satay.", 750, "🍢", nil, false),
				}},
			},
		},
		{
			Slug: "friterie-du-beffroi", Name: "Friterie du Beffroi", Emoji: "🍟",
			Description: "La friterie belge du quartier : frites à la graisse de bœuf, fricadelles et mitraillettes.",
			Cuisines:    []string{"friterie", "belge", "snack"}, Address: "Rue de la Grande Triperie 9, 7000 Mons",
			Lat: 50.4550, Lng: 3.9545, Phone: "065 00 11 08", Rating: 4.2, RatingCount: 1534, PriceLevel: 1,
			EtaMin: 15, EtaMax: 30, DeliveryFee: 199, MinOrder: 1000, Providers: links(true),
			Categories: []catalog.CategoryImport{
				{Name: "Frites", Items: []catalog.ItemImport{
					it("Cornet de frites", "Frites fraîches cuites deux fois, comme il se doit.", 350, "🍟", nil, true, friesSize, friesSauce),
				}},
				{Name: "Snacks", Items: []catalog.ItemImport{
					it("Fricadelle", "La classique, bien grillée.", 300, "🌭", nil, true, friesSauce),
					it("Boulettes sauce lapin (2)", "Boulets à la liégeoise, sauce sirop de Liège.", 750, "🧆", nil, false),
					it("Cervelas", "Grillé, servi avec moutarde.", 350, "🌭", nil, false),
					it("Croquette fromage (2)", "Fromage belge fondant.", 500, "🧀", tags("veggie"), false),
				}},
				{Name: "Mitraillettes", Items: []catalog.ItemImport{
					it("Mitraillette poulet", "Demi-baguette, poulet pané, frites, crudités, sauce au choix.", 850, "🥖", nil, true, friesSauce),
					it("Mitraillette fricadelle", "Demi-baguette, fricadelles, frites, sauce au choix.", 750, "🥖", nil, false, friesSauce),
				}},
			},
		},
		{
			Slug: "verdure-et-co", Name: "Verdure & Co", Emoji: "🥗",
			Description: "Salades gourmandes, soupes de saison et jus pressés.",
			Cuisines:    []string{"salades", "healthy", "végétarien"}, Address: "Place du Parc 3, 7000 Mons",
			Lat: 50.4602, Lng: 3.9668, Phone: "065 00 11 09", Rating: 4.4, RatingCount: 233, PriceLevel: 2,
			EtaMin: 20, EtaMax: 30, DeliveryFee: 199, MinOrder: 1200, Providers: links(false),
			Categories: []catalog.CategoryImport{
				{Name: "Salades", Items: []catalog.ItemImport{
					it("César", "Romaine, croûtons, parmesan, œuf mollet.", 1150, "🥗", tags("veggie"), true, saladProt, dressing),
					it("Chèvre chaud", "Mesclun, toasts de chèvre, miel, noix.", 1250, "🧀", tags("veggie"), true, dressing),
					it("Buddha bowl", "Quinoa, patate douce rôtie, houmous, pickles.", 1290, "🥙", tags("vegan", "gluten_free"), false, saladProt, dressing),
					it("Niçoise", "Thon, haricots verts, olives, pommes de terre, œuf.", 1290, "🐟", tags("gluten_free"), false, dressing),
				}},
				{Name: "Soupes", Items: []catalog.ItemImport{
					it("Soupe du jour", "Selon le marché, servie avec pain au levain.", 550, "🍲", tags("vegan"), false),
				}},
				{Name: "Jus & douceurs", Items: []catalog.ItemImport{
					it("Jus vert pressé", "Pomme, concombre, céleri, gingembre.", 490, "🥤", tags("vegan", "new"), false, drinkSize),
					it("Cookie avoine-chocolat", "Fait maison.", 290, "🍪", tags("veggie"), false),
				}},
			},
		},
	}
}
