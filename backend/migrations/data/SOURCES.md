# Sources : restaurants réels de Mons (`mons_restaurants.json`)

Vérification du **2026-10-09** (`menu_checked_at`). Tous les plats et tous les prix viennent
d'une page lue ce jour-là. Pour en extraire les noms, les descriptions et les prix, un script
a analysé le HTML. Aucun prix n'a été estimé et aucun plat n'a été ajouté.

## Méthode et limites générales

* **Accès bloqué** : les pages `ubereats.com` et `takeaway.com` refusent les requêtes depuis
  l'environnement de recherche (erreur DNS côté WebFetch, HTTP 403 côté shell). Les menus viennent donc :
  * des **sites satellites officiels Takeaway.com** du restaurant (« *Nom* – Commander un repas en
    ligne à Mons »). Takeaway.com héberge ces sites, qui reprennent la carte et les prix publiés sur
    Takeaway. La balise `<meta name="orderUrl">` de chaque site donne l'URL de la fiche
    Takeaway, reprise dans `providers` ;
  * ou du **site propre** du restaurant (La Pizza, Pacific, Thai Café).
* **Uber Eats** : aucune fiche Uber Eats n'a pu être ouverte. Les résultats de recherche montrent
  pourtant que plusieurs enseignes de Mons y sont présentes (Domino's, Pizza Hut, Pita Mons, Buga Ramen,
  Bowl Street, Quick Imagix, etc.). Aucun lien `ubereats` n'a été ajouté, faute d'avoir pu vérifier
  qu'il s'agit bien des mêmes établissements.
* **Note et nombre d'avis** : non vérifiés, donc mis à `0`. Ils ne figurent pas sur les sites
  satellites, et les fiches Takeaway et Uber Eats n'étaient pas accessibles.
* **Frais de livraison et minimum de commande** : inconnus (`0`) sauf mention contraire ci-dessous.
  `eta_min` et `eta_max` sont des estimations par défaut, sauf pour Pacific (environ 45 min, chiffre
  publié par le restaurant).
* **Options et suppléments** : les sites satellites chargent les choix (sauces, suppléments, tailles)
  dynamiquement et ils n'étaient pas visibles. `option_groups` est donc vide partout. Les tailles
  différentes figurent comme articles distincts quand la carte les présente ainsi (ex. Adriatica M/XL).
* **Coordonnées** : géocodées le 2026-10-09 avec OpenStreetMap Nominatim à partir de l'adresse publiée.
* **Champs choisis par nous** : les emojis, les tags (`veggie`, `vegan`, `spicy`) et les drapeaux
  `popular`. Les tags découlent du nom et de la composition publiés ; aucun drapeau `popular` ne
  repose sur des données de ventes. `price_level` est notre estimation d'après les prix relevés.
* **Coupes** : les menus sont partiels (12 à 16 articles représentatifs par restaurant). Les noms et
  descriptions sont repris tels quels. Seules exceptions : une majuscule initiale, la suppression des
  caractères chinois sur Sushi Lover, et un préfixe « California » ajouté à deux rolls de Sushi Lover
  pour plus de clarté. Les catégories sont parfois regroupées.

## Restaurants

| # | Restaurant | Cuisine | Adresse | Sources | Vérifié | Non vérifié / remarques |
|---|---|---|---|---|---|---|
| 1 | **La Pizza** (`la-pizza-mons`) | pizza, italien | Chaussée de Binche 50/06, 7000 Mons | https://www.lapizza-mons.com/ · https://www.lapizza-mons.com/carte · Takeaway : https://www.takeaway.com/be-fr/menu/la-pizza-3 | 6 pizzas « incontournables » et 6 antipasti avec prix (site officiel), téléphone, adresse, « livraison offerte dès 45 € » | La fiche Takeaway n'apparaît que dans les résultats de recherche (non ouverte) ; un avis client sur le site mentionne une commande via Takeaway. Les autres catégories de la carte se chargent en JavaScript et n'ont pas pu être lues. Frais sous 45 € inconnus. |
| 2 | **Adriatica** (`adriatica-mons`) | pizza, italien, pâtes | Chaussée du Roeulx 385, 7000 Mons | https://www.ladriaticamons.be/ → https://www.takeaway.com/be/menu/adriatica-mons | Carte complète avec prix (pizzas M et XL, pâtes, desserts) | Frais et minimum inconnus. À environ 2,6 km du centre. |
| 3 | **Ô Sando** (`o-sando-mons`) | japonais, sandwichs, poké | Rue de la Coupe 7, 7000 Mons | https://www.o-sando-mons.be/ → https://www.takeaway.com/be/menu/o-sando | Carte et prix | Frais et minimum inconnus. |
| 4 | **Tomo** (`tomo-mons`) | ramen, japonais | Rue d'Enghien 11, 7000 Mons | https://www.tomomons.be/ → https://www.takeaway.com/be/menu/tomo-mons | Carte et prix | Frais et minimum inconnus. |
| 5 | **Papou** (`papou-mons`) | grec | Grand Rue 70, 7000 Mons | https://www.papoumons.be/ → https://www.takeaway.com/be/menu/papou-mons | Carte et prix | Frais et minimum inconnus. |
| 6 | **Cup Pasta** (`cup-pasta-mons`) | pâtes, italien | Rue de la Chaussée 17, 7000 Mons | https://www.cup-pasta-mons.be/ → https://www.takeaway.com/be/menu/cup-1 | Carte et prix | Frais et minimum inconnus. Le choix des pâtes et des sauces pour « Cup à composer » n'est pas modélisé. |
| 7 | **Sushi Lover** (`sushi-lover-mons`) | sushi, japonais, poké | Rue de la Chaussée 4, 7000 Mons | https://www.sushi-lover.be/ → https://www.takeaway.com/be/menu/sushi-lover | Carte et prix | Frais et minimum inconnus. |
| 8 | **Snack Alibaba** (`snack-alibaba-mons`) | kebab, snack, turc | Rue de Nimy 17, 7000 Mons | https://www.snack-alibaba.be/ → https://www.takeaway.com/be/menu/snack-alibaba | Carte et prix | Frais et minimum inconnus. |
| 9 | **Le Bosphore** (`le-bosphore-mons`) | kebab, turc, burger | Rue de la Clef 32, 7000 Mons | https://www.lebosphore-mons.be/ → https://www.takeaway.com/be/menu/le-bosphore-mons | Carte et prix | Frais et minimum inconnus. |
| 10 | **Baalbeck** (`baalbeck-mons`) | libanais | Rue de la Clef 16, 7000 Mons | https://www.baalbeckmons.be/ → https://www.takeaway.com/be/menu/baalbeck | Carte et prix | Un extrait de recherche donnait « Rue de la Clef 14 » ; nous retenons le n° 16, publié sur le site satellite. Frais et minimum inconnus. |
| 11 | **Pacific Traiteur Asiatique** (`pacific-traiteur-mons`) | chinois, asiatique | Rue de Nimy 45, 7000 Mons | https://www.pacific-traiteur.be/service-livraison-a-domicile.html · https://pacificmons.wikeo.net/ · Takeaway : https://www.takeaway.com/be-fr/menu/pacific-traiteur-asiatique-mons | Tarifs de livraison du site officiel, téléphone 065/84 14 23, délai d'environ 45 min, **minimum 25 € pour Mons** (30 € pour Nimy, Hyon, Cuesmes, Ghlin, etc.) | La fiche Takeaway n'apparaît que dans les résultats de recherche (non ouverte). Les frais de livraison ne sont pas indiqués. Un annuaire tiers affichait l'établissement comme « momenteel gesloten » au moment de son passage, ce qui reflète sans doute seulement l'heure du passage. Il faut le confirmer par téléphone. |
| 12 | **Thai Café Grands Prés** (`thai-cafe-grands-pres`) | thaï, asiatique | Place des Grands Prés 1, 7000 Mons | https://thai.cafe/fr/menu · https://thai.cafe/fr/restaurants · https://thai.cafe/en | Adresse de l'établissement de Mons (page « restaurants »), **menu et prix de la chaîne** (site national), téléphone central 02 888 80 80, livraison gratuite dès 40 € | ⚠️ Les prix proviennent du menu national de la chaîne et n'ont pas été confirmés pour l'établissement de Mons. Le barème des frais n'est pas clair (« 5 € si < 40 € », mais l'exemple indique 38 € → 2 €), d'où `delivery_fee = 0`. Une offre d'emploi plus ancienne citait la « rue Marguerite Bervoets 59 » ; nous retenons l'adresse du site officiel. Aucun lien de plateforme n'a été vérifié : la commande se fait via thai.cafe. |
| 13 | **La Frite Mayo** (`la-frite-mayo-havre`) | friterie, snack belge, burger | Chaussée du Roeulx 1210, 7021 Havré (Mons) | https://www.lafritemayomons-mons.be/ → https://www.takeaway.com/be/menu/la-frite-mayo-mons | Carte et prix | ⚠️ Situé à Havré, à environ 6,4 km du bureau : la zone de livraison ne couvre peut-être pas le centre. Frais et minimum inconnus. |

## Écartés (pas de prix vérifiables ou hors sujet)

* **Indien** : nous n'avons trouvé aucun menu avec prix vérifiable. *Gurkha cuisine* (Place du
  Marché aux Herbes 10, népalais/indien) figure sur Takeaway, mais son site satellite
  `gurkhacuisinemons.be` est désactivé. *Pourquoi pas l'Inde* (rue de la Clef 50) n'a pas de carte en
  ligne.
* **Burger dédié** : la carte de *Mr Gaston* (Chaussée de Binche 141) se charge via un widget
  GloriaFood en JavaScript, illisible. Nous n'avons trouvé aucune page lisible pour *Kheops & Bryan's
  Burger House* (rue des Capucins 19, sur Takeaway). Les burgers restent couverts par Le Bosphore et
  La Frite Mayo.
* **La Rondinella** (italien, place Léopold) : le site affiche les frais (Mons : minimum 20 €, frais
  2,50 €), mais le menu se charge en JavaScript (GloriaFood).
* **La Trattoria Mons** : son site satellite Takeaway est désactivé et redirige vers takeaway.com.
* **Zenzou** : bar à bubble tea, ce qui ne convient pas à un repas de midi.
* **Wang de Fu** (chinois, Chemin des Mourdreux 58) et **Sushi House / Mizu** (Grand-Place 26) :
  menus vérifiés sur leurs sites satellites Takeaway, mais non retenus pour garder de la variété. Ils
  peuvent être ajoutés facilement.
* **Délices d'Asie** (chinois, avenue d'Hyon 71) : le site publie ses zones de livraison
  (2 km : minimum 25 €, frais 2,50 €) ; non retenu pour la même raison de variété.
