# Sources : flux `menusync` (Mons, 2026-10-09)

Fichiers produits par `backend/cmd/menusync` le **2026-10-09** (`menu_checked_at`), au
format `RestaurantImport` (+ `source`, `source_urls`, `menu_checked_at`, ignorés à l'import).
Ils **ne sont pas branchés aux migrations** : en production, la synchronisation automatique du
serveur relit les mêmes sources (table `sync_sources`) et met la base à jour elle-même. Aucun prix n'est estimé, aucun champ inventé :
un champ absent de la source reste vide (`0`, `""`, `[]`).

| fichier | source | restaurants | plats | plats avec options |
|---|---|---|---|---|
| `mons_deliveroo.json` | Deliveroo, liste « Mons Center » + pages menu, rayon 8 km | 40 | 2 996 | 1 274 (33 restaurants) |
| `mons_weloveat.json` | weloveat.be, API publique de la SPA, rayon 8 km | 70 | 4 806 | 0 (suppléments non lus, voir plus bas) |
| `mons_takeaway_sites.json` | 10 sites satellites Takeaway (`mons_takeaway_sites.txt`) | 10 | 1 204 | 0 |
| `mons_merged.json` | fusion des trois + `mons_restaurants.json` (`existing`) | 102 | 7 533 | 1 263 (31 restaurants) |

Commande de fusion (133 fiches → 102 restaurants, 31 doublons fusionnés, 21 groupes ; régénéré le
2026-10-09 avec la normalisation : noms, cuisines, plats à 0 € sans options et doublons retirés,
« Bagel City » / « BAGEL CITY » et « Pili Pili Mons » / « PILI PILI MONS » fusionnés) :

```bash
menusync merge -in mons_takeaway_sites.json,mons_deliveroo.json,mons_weloveat.json,existing=mons_restaurants.json \
  -out mons_merged.json        # priorité du menu : takeaway-site > deliveroo > weloveat > jsonld > existing
```

## Deliveroo
* Page liste `https://deliveroo.be/fr/restaurants/brussels/mons-center?fulfillment_method=DELIVERY&geohash=u0fz40u6z12k`
  (53 cartes), puis la page menu publique de chaque restaurant (`/fr/menu/Brussels/<zone>/<resto>?geohash=…`).
  Données lues dans le JSON `__NEXT_DATA__` de ces pages. Aucune requête vers `/api/` ou `graphql`
  (interdits par `robots.txt`, vérifié par l'outil avant chaque requête).
* Écartés : 6 à plus de 8 km (Binche, La Louvière, cartes cadeaux…), 5 pages sans plat lisible
  (fermés ou épiceries dont la carte est sur des sous-pages), 1 page 404, 1 doublon
  (Khéops & Bryan's, deux fiches pour la même adresse).
* Prix : `price.fractional` (centimes) tels qu'affichés sur Deliveroo, **souvent majorés par rapport au
  restaurant**. Options : groupes de *modifiers* (min/max, suppléments en centimes) ; les sous-options
  imbriquées ne sont pas reprises. Le minimum de sélection est ramené au nombre de choix disponibles.
* Frais de livraison et minimum : lus dans l'en-tête de la page pour une livraison au centre de Mons
  (géohash de la liste). Note et nombre d'avis : seulement s'ils sont affichés. Délai : jamais renseigné
  (la page n'affiche qu'un « environ 32 minutes » identique partout, non repris).
* Disponibilité : la plupart des restaurants étaient fermés à l'heure du passage ; l'indicateur
  « indisponible » n'est repris que si le restaurant était ouvert.
* Adresse : telle que publiée, sans le « Brussels » que Deliveroo accole aux zones belges.

## weloveat
* weloveat.be est une SPA Angular. Son bundle `main.*.js` contient `apiUrl: "https://api.weloveat.be/api/"`
  et les appels utilisés ici, tous **anonymes** :
  `POST test/searchEstablishments?page=1` (liste, 73 établissements autour de Mons),
  `POST products/getFoundEstablishmentProducts` (catégories + produits d'un établissement),
  `GET products/{slug}` (suppléments d'un produit). `https://api.weloveat.be/robots.txt` :
  `User-agent: * / Disallow:` (tout autorisé) ; `https://weloveat.be/robots.txt` renvoie la page de la SPA.
* L'API renvoie aussi des **données personnelles** du commerçant (e-mail, jetons…) : elles sont
  supprimées avant toute écriture dans le cache disque et ne sont jamais conservées.
* Écartés : 3 établissements à plus de 8 km.
* Prix en euros dans l'API → centimes. Minimum de commande (`minimum_per_order`), note (`score_float`,
  `nbr_evaluation`), délai (`estimated_delivery_time`, 45 min pour la plupart), téléphone public de
  l'établissement. Frais de livraison absents de l'API (zones par adresse) → `0`.
* Suppléments : disponibles (`-options`), mais 1 requête par plat ; pour 4 806 plats cela
  représente environ 4 h 20 de requêtes espacées de 2,5 s minimum. Le fichier livré ne les contient pas.
  Un essai sur 1 restaurant (SEASON'S) a donné 44 plats sur 51 avec options (groupes à choix unique :
  min 1 / max 1 ; à choix multiple : min 0 / sans maximum).
* Lien plateforme : `https://weloveat.be/{slug}` (route de la SPA).

## Sites satellites Takeaway
* Gabarit commun : catégories `.menucat`, plats en microdonnées schema.org `Product`
  (`name`, `description`, `offers.price`), restaurant en microdonnées `Restaurant` (adresse,
  coordonnées), lien Takeaway dans `<meta name="orderUrl">`. `robots.txt` : `Allow: /`.
* Options : la commande est désactivée sur ces sites (`menucard_ShowSideDishes` est une fonction vide)
  et aucune URL de suppléments n'est exposée → pas d'options.
* Les cartes sont **complètes** (contrairement aux extraits de `mons_restaurants.json`).

## Non retenus
* Source `jsonld` essayée sur `lapizza-mons.com/carte`, `pacific-traiteur.be` et `thai.cafe/fr/menu` :
  aucun menu schema.org exploitable (0 restaurant).

## Sites satellites Takeaway : recherche élargie (2026-10-09, migration `1760000009`)
Demande : « il faut scraper un maximum de restaurants ». `takeaway.com` reste derrière un défi anti-robot
Cloudflare, qui n'est **jamais contourné**. Les sites satellites construits par Takeaway sont publics,
ont le même gabarit et un `robots.txt` « `User-agent: * / Allow: /` ».

**Résultat : 65 nouveaux sites confirmés, soit 75 au total dans `mons_takeaway_sites.txt`.**
L'analyse par `menusync -provider takeaway-site -radius 10` donne **8 177 plats** pour les 65 nouveaux sites
(sans options : voir plus haut). Avec les 10 sites d'origine (1 204 plats), on arrive à environ 9 400 plats.
La migration `1760000009_more_takeaway_sites.go` crée une ligne `sync_sources` (`takeaway-site`, ville `mons`,
active, priorité 10 → 39, les derniers sites partageant 39) pour chaque URL du fichier qui n'en a pas encore. Elle
est idempotente : une base neuve reçoit déjà toute la liste de `1760000005`. Le retour arrière ne supprime que
les sites ajoutés après les 10 d'origine. `mons_takeaway_sites.json` et `mons_merged.json` n'ont **pas** été
régénérés : la synchronisation du serveur relit directement ces sites.

Méthode :
1. **Recherche web** sur le titre du gabarit, « *Nom* – Commander un repas en ligne à *Commune* » (Mons, Jemappes,
   Cuesmes, Nimy, Ghlin, Hyon, Havré, Obourg, Spiennes, Saint-Symphorien, Quaregnon, Frameries, « Bergen »,
   variante anglaise « Order food online in Mons »), combiné avec des types de cuisine (pizza, kebab, snack,
   friterie, sushi, chinois, thaï, indien, burger, pitta, tacos, libanais, italien, poké, sandwich, couscous,
   donuts, etc.).
2. **Domaines devinés** à partir des 205 noms de `mons_deliveroo.json`, `mons_weloveat.json`, `mons_ubereats.json`,
   `mons_restaurants.json` et des noms vus dans les extraits Takeaway : `nom.be`, `nom-mons.be`, `nommons.be`,
   `nom-mons-mons.be`. Environ 650 domaines ont été testés, avec une seule requête GET sur la page d'accueil,
   2,6 s entre deux requêtes et l'agent `OCC-Deliveries-menusync/1.0`. Cette méthode a donné 5 sites que la
   recherche n'avait pas trouvés (Chez Dona Julia, Pizza Régal, Five Senses, La Cédraie, Victory's).
3. **Vérification** de chaque candidat : `robots.txt` doit autoriser `/`. La page d'accueil doit présenter le gabarit
   Takeaway (`menucat`, `meal-wrapper`, microdonnées `itemprop`, `<meta name="orderUrl">` vers takeaway.com).
   L'analyse réelle doit trouver **au moins 5 plats**, et l'adresse doit être **à 10 km au plus** de 50.4542, 3.9567.
   Aucun candidat retenu n'est à plus de 8,3 km.

**Snack à la Gare** (rue Léopold II 7, 7000 Mons) a bien un site satellite : `https://www.snack-a-la-gare.be/`
(196 plats, lien `takeaway.com/be/menu/snack-a-la-gare`).

| # | restaurant | adresse publiée | site (`https://www.…/`) | plats | trouvé par |
|---|---|---|---|---|---|
| 1 | Alaa Snack | 16 Rue Rogier, 7000 Mons | alaasnack-mons.be | 34 | recherche |
| 2 | Asia Box Go | 2 Place Louise, 7000 Mons | asiaboxgo.be | 317 | recherche |
| 3 | Bagel City | Rue de la Chaussée 19, 7000 Mons | bagelcity-mons.be | 61 | recherche |
| 4 | Chez Dona Julia | Rue de Nimy 13, 7000 Mons | chezdonajuliamons.be | 66 | deviné |
| 5 | CTR Chicken | 1 Place Léopold, 7000 Mons | chitir-chicken-mons.be | 218 | recherche |
| 6 | Donroll's | 110 Grand'Rue, 7000 Mons | donrolls-mons.be | 162 | recherche |
| 7 | Dreams Donuts | Grand'Rue 16, 7000 Mons | dreamsdonuts-mons.be | 13 | recherche |
| 8 | Délice d'Asie | 71 Avenue d'Hyon, 7000 Mons | delicedasie.be | 178 | recherche |
| 9 | En Faim | 132 Rue de nimy, 7000 Mons | en-faim.be | 97 | recherche |
| 10 | Five Senses | Grand Place 33, 7000 Mons | five-senses.be | 56 | deviné |
| 11 | Friterie Grand'Place | 34 Grand'Place., 7000 Mons | friterie-grandplace.be | 124 | recherche |
| 12 | Khao soi | Rue de Bertaimont 8, 7000 Mons | khaosoi-mons.be | 44 | recherche |
| 13 | L'Atelier Libanais | Rue Samson 1, 7000 Bergen | latelierlibanais.be | 72 | recherche |
| 14 | La Cédraie | Rue des Capucins 39, 7000 Mons | lacedraie.be | 228 | deviné |
| 15 | La Petite Couscoussière | Rue Des Clercs 7, 7000 Mons | la-petite-couscoussiere-mons.be | 41 | recherche |
| 16 | La Sandwicherie de La Guinguette Montoise | 69 Avenue d'Hyon, 7000 Mons | lasandwicheriedelaguinguettemontoise-mons.be | 69 | recherche |
| 17 | Le Copenhagen | 11 Grand Place, 7000 Mons | le-copenhagen.be | 104 | recherche |
| 18 | Melanza Mons | 2 Rue des Arquebusiers, 7000 Mons | melanza-mons.be | 129 | recherche |
| 19 | New China Restaurant | 64 Grand-Rue, 7000 Mons | newchinarestaurantmons.be | 77 | recherche |
| 20 | O'Tacos Centre ville de Mons | 1 Rue de la Poterie, 7000 Mons | otacosmons.be | 85 | recherche |
| 21 | Pacific Traiteur Asiatique | Rue de nimy 45, 7000 Mons | pacific-traiteur-asiatique.be | 84 | recherche |
| 22 | Pastella Mons | 4 Av. Frère Orban, 7000 Mons | pastella-mons.be | 50 | recherche |
| 23 | Pitta Mons | 81 Rue de Nimy, 7000 Mons | pittamons.be | 240 | recherche |
| 24 | Pitta Shop | 5 rue du Miroir, 7000 Mons | pittashop.be | 214 | recherche |
| 25 | Sandwicherie Boulangerie Montoise | 38 Rue de Nimy, 7000 Mons | sandwicherie-boulangerie-montoise.be | 105 | recherche |
| 26 | Season's | 2 Rue Notre Dame, 7000 Mons | seasons-mons.be | 33 | recherche |
| 27 | Snack du Lido | 136 Rue de Nimy, 7000 Mons | snackdulido.be | 188 | recherche |
| 28 | Snack Pitta Grec Akropolis | 2 rue du Miroir, 7000 Mons | snack-pitta-grill-akropolis.be | 306 | recherche |
| 29 | Snack à la Gare | 7 Rue Léopold II, 7000 Mons | snack-a-la-gare.be | 196 | recherche (demandé) |
| 30 | Sushi Flower | 34 rue des Capucins, 7000 Mons | sushiflower.be | 151 | recherche |
| 31 | Sushi Home | 3 Rue de la Petite Guirlande, 7000 Mons | sushi-home-mons-mons.be | 101 | recherche |
| 32 | Sushi House | 26 Grand Place, 7000 Mons | sushi-house-mons.be | 136 | recherche |
| 33 | Victory's | — (non publiée) | victorysmons.be | 65 | deviné |
| 34 | Wang de Fu | 58 Chemin des Mourdreux, 7000 Mons | wang-de-fu.be | 123 | recherche |
| 35 | Zenzou | Rue de la Clef 24, 7000 Mons | zenzou.be | 100 | recherche |
| 36 | L'Alighieri | 145 avenue Joseph Wauters, 7033 Cuesmes | lalighieri.be | 105 | recherche |
| 37 | Arrivo pizza | 5 Rue Hankar, 7080 Frameries | arrivopizzaframeries.be | 86 | recherche |
| 38 | Couscous Dellys Frameries | 2 Route de Pâturages, 7080 Frameries | couscousdellysframeries-frameries.be | 27 | recherche |
| 39 | Garibaldi | Rue De La France 1, 7080 Frameries | garibaldiframeries.be | 74 | recherche |
| 40 | Pékin Express | Rue des alliés 41A, 7080 Frameries | pekin-express.be | 145 | recherche |
| 41 | Sushi Flower | 63 Rue des Alliés, 7080 Frameries | sushiflower-frameries.be | 119 | recherche |
| 42 | Ap Pizza Street Food Napoletano | Rue de Mons 218, 7011 Mons | ap-pizza-street-food.be | 103 | recherche |
| 43 | Pizza Régal | Rue de Mons 139B, 7011 Ghlin | pizza-regal.be | 157 | deviné |
| 44 | Pizzeria Reggina | 99 Rue de Mons, 7011 Ghlin | pizzeriareggina.be | 135 | recherche |
| 45 | Pizza House | 333 Chaussée de Maubeuge, 7022 Mons | pizzahousemons.be | 209 | recherche |
| 46 | Bella Napoli.it | 746 Av. Maréchal Foch, 7012 Jemappes | bella-napoliit.be | 101 | recherche |
| 47 | Eliz Shop | Rue Albert Defrise 3, 7012 Mons | eliz-shop-mons.be | 213 | recherche |
| 48 | Friterie de Jemappes chez Jean | 736 Avenue maréchal foch, 7012 Mons | friterie-de-jemappes-chez-jean.be | 300 | recherche |
| 49 | La Chine | 704-706 Avenue du Roi Albert, 7012 Jemappes | lachine-jemappes.be | 154 | recherche |
| 50 | Super Asia | 784 Avenue Maréchal Foch, 7012 Jemappes | super-asia.be | 95 | recherche |
| 51 | Touchdown Grill | 25 Rue Clémenceau, 7012 Mons | touchdown-grill-mons.be | 15 | recherche |
| 52 | China Express | Rue des Viaducs 61, 7020 Bergen | chinaexpressbergen.be | 134 | recherche |
| 53 | Ciao Pizza | Route d'Ath 2, 7020 Nimy | ciaopizza-nimy.be | 65 | recherche |
| 54 | Imako - Poke Bowls | 88 Rue des Viaducs, 7020 Nimy | imako-nimy.be | 72 | recherche |
| 55 | Bubble Mood | Rue Jules Destrée 169, 7390 Quaregnon | bubblemood-quaregnon.be | 87 | recherche |
| 56 | Burger City | Rue Grand-Place 14, 7390 Quaregnon | burgercity-quaregnon.be | 180 | recherche |
| 57 | Dal Fratello | Rue des Vaches 199, 7390 Quaregnon | dal-fratello.be | 90 | recherche |
| 58 | Friterie Meknes | Rue de la grosse cotte 87, 7390 Quaregnon | friteriemeknes.be | 183 | recherche |
| 59 | Le Snack Thessaloniki | Rue Carnot 1/2, 7390 Quaregnon | le-snack-thessaloniki-quaregnon.be | 77 | recherche |
| 60 | New Asia Quaregnon | 9 Route de Mons, 7390 Quaregnon | lepalaisdubonheurwasmuel.be | 137 | recherche |
| 61 | Pita star | 16 Rue du Village, 7390 Quaregnon | pitastar.be | 176 | recherche |
| 62 | Snack chez Tevfik | 3 Rue Modeste Derbaix, 7390 Quaregnon | cheztevfik.be | 124 | recherche |
| 63 | Snack Junior | 168 Rue Jules Destrée, 7390 Quaregnon | snackjuniorquaregnon.be | 287 | recherche |
| 64 | Nota Bene Sympho | Chaussée Du Roi Baudouin 161, 7030 Saint-Symphorien | nota-bene-sympho.be | 163 | recherche |
| 65 | Little Italy | 192 Chaussée de Beaumont, 7032 Spiennes | littleitaly-spiennes.be | 97 | recherche |

Remarques :
* **Sushi Flower** a deux établissements avec chacun son site (Mons et Frameries). La synchronisation les distingue
  par clé de source et donne au second un slug unique.
* **Victory's** a un site en anglais, sans adresse publiée. Les coordonnées du site le placent à environ 0,5 km
  du centre.
* Le normaliseur de `menusync` retire « Mons » en fin de nom. On obtient donc « Pitta », « Melanza », « Pastella »
  et « O'Tacos Centre ville de » (noms corrigés dans le tableau ci-dessus). Le dernier nom est maladroit et peut
  être corrigé dans l'admin.
* Les cartes de **Dreams Donuts** (13 plats) et de **Touchdown Grill** (15) sont courtes, mais complètes.

**Candidats écartés** :
* *Site satellite désactivé* : la page ne montre que le gabarit vide « www.takeaway.com/be », sans `.menucat`
  et sans `orderUrl`. Sont concernés : `la-recre.be` (Quaregnon), `latrattoriamons.be`, `crumbsmons.be`,
  `lapizza-mons.be`, `vaianapokemons.be`, `gurkhacuisinemons.be`, `vitalgaufre-mons.be`, `lepaysan-mons.be`,
  `san-lorenzo-jemappes.be`, `frites-a-gogo.be` (Best Kebab 2), `friteriesnackchezpetruz.be`,
  `pizzaallinchickenquaregnon.be` et `efekebabgrill-quaregnon.be` (Quaregnon).
* *Domaine disparu* (DNS introuvable, encore indexé par les moteurs) : `pizzahouse-mons.be` (ancien domaine de
  Pizza House, remplacé par `pizzahousemons.be`), `saporiditalia-jemappes.be` (La Palmeraie Jemappes),
  `african-gardens.be` (Jemappes), `pizzattitudeframeries.be`, `restaurantasiaframeries.be` et
  `pizzeriamozzabellamons.be`.
* *Hors zone* (même gabarit, autre ville) : `la-palmeraie.be` (Châtelet), `thai-cafe.be` (Ixelles).
* *Pas un site Takeaway* (site propre, menu en JavaScript ou sans prix) : La Rondinella, `lapizza-mons.com`,
  So Greek, Pitta Assos, Snack AliBaba Cuesmes (`snackalibaba.top`), Mr Gaston, Les Saveurs du Maghreb, La Chine
  Jemappes (`lachinejemappes.be`, ancien système ; son site satellite `lachine-jemappes.be` est retenu).
* Pas de site satellite trouvé pour les restaurants vus seulement sur takeaway.com : Kheops & Bryan's, Bowl Street,
  Gurkha (désactivé), Incheon Korean Fried Chicken, Les Saveurs d'Afrique, Pasta & Love, Upgame, Öz Pita Grill et
  Pitta Palace Grill.
