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
