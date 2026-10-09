# OCC DELIVERIES — Architecture & contrat technique

> Source de vérité du **modèle de données** et de l'**API**. Toute modification du
> schéma ou d'un endpoint met à jour ce fichier dans le même commit.

## 1. Vue d'ensemble

```
┌──────────────────────────── Docker image (1 conteneur) ───────────────────────────┐
│                                                                                    │
│   occ (binaire Go = PocketBase v0.40 étendu)                                       │
│   ├── /api/collections/*   CRUD + realtime (SSE) PocketBase, protégés par rules   │
│   ├── /api/occ/*           endpoints métier (state machine, résumé, paiements…)   │
│   ├── /_/                  admin PocketBase (superuser)                           │
│   └── /*                   SPA React (fichiers de ./pb_public, fallback index)    │
│                                                                                    │
│   SQLite (WAL) dans /pb/pb_data  ← volume persistant Coolify                       │
└────────────────────────────────────────────────────────────────────────────────────┘
```

* **Backend** : PocketBase utilisé comme framework Go (`backend/`). Auth, realtime,
  fichiers, admin UI et migrations fournis ; la logique métier est en Go typé et testé.
* **Frontend** : React 19 + Vite + TypeScript + Tailwind v4 (`frontend/`). SPA
  servie par le même binaire → même origine, pas de CORS, cookies/token simples.
* **Realtime** : le client s'abonne aux collections filtrées par `party` ; les
  règles d'accès PocketBase s'appliquent aussi aux événements SSE.
* **Argent** : toujours en **centimes entiers** (`int`), devise EUR.

Pourquoi PocketBase plutôt que Postgres : voir `docs/adr/0001-stack.md`.

## 2. Arborescence

```
backend/
  main.go                    bootstrap PocketBase, enregistre hooks/routes/migrations
  internal/domain/           logique pure (aucune dépendance PocketBase) + tests
    money.go                 calcul prix options, split des frais (cents exacts)
    party.go                 state machine, élection du restaurant
    code.go                  génération code de party
    epc.go / iban.go         payload QR EPC (SEPA), validation IBAN
    geo.go                   haversine
  internal/providers/        adaptateurs Uber Eats / Takeaway / manuel + tests
  internal/app/              hooks PocketBase + routes /api/occ (glue)
  internal/catalog/           import / export des menus (JSON, CSV, rapport de validation)
  migrations/                migrations Go (schéma, seed démo, rôles admin, données réelles)
    data/                    mons_restaurants.json (données réelles embarquées)
frontend/
  src/lib/                   pb client, types, api, format, hooks realtime
  src/components/ui/         design system (Button, Card, Sheet, Badge, Avatar…)
  src/features/<domaine>/    party, restaurants, auth, profile, payments, admin
  src/routes/                pages
docs/                        architecture, design system, workflow, déploiement, ADR
```

## 3. Modèle de données (collections PocketBase)

Toutes les collections de base ont `created` / `updated` (autodate).
`R(x)` = relation vers x. Les montants sont en centimes.

### `users` (auth, collection existante étendue)
| champ | type | notes |
|---|---|---|
| name | text | affiché partout |
| avatar | file | optionnel |
| color | text | couleur d'avatar générée (`#RRGGBB`) si vide |
| role | select `user` \| `admin` | `user` par défaut (hook) ; `admin` ouvre le panneau `/admin` |

Rules : list/view `@request.auth.id != ""` ; update/delete `id = @request.auth.id`.
Rôle : un hook refuse (403) toute création/modification de `role` par la collection
si le demandeur n'est ni `admin` ni superuser ; les admins changent les rôles via
`PATCH /api/occ/admin/users/{id}/role` (on ne peut pas retirer ses propres droits).
Bootstrap : au démarrage, les comptes dont l'e-mail figure dans `OCC_ADMIN_EMAIL`
ou `OCC_ADMINS` passent `admin` ; un compte créé plus tard avec un de ces e-mails
(mot de passe, OAuth2, admin `/_/`) naît `admin`.

### `payout_profiles` — coordonnées de remboursement (privées)
| champ | type | notes |
|---|---|---|
| user | R(users) unique, requis | |
| holder_name | text | bénéficiaire du virement |
| iban | text | validé mod-97, stocké sans espaces, majuscules |
| bic | text | optionnel |
| payment_link | url | ex. `https://paypal.me/jdoe`, Revolut… |
| wero_id | text | n° de mobile (E.164, ex. `+32470123456`) **ou** e-mail enregistré sur Wero |
| bancontact_phone | text | n° de mobile (E.164) lié à Bancontact Pay |
| wero_qr | file | image (png/jpg/webp, ≤ 1 Mo, `protected`) — QR « recevoir » généré dans l'app bancaire |
| bancontact_qr | file | idem pour Bancontact Pay |

Rules : toutes `user = @request.auth.id` (create : `@request.auth.id != "" && user = @request.auth.id`).
Jamais exposé aux autres membres : QR EPC, identifiants Wero/Bancontact et images
QR ne sortent que via `/api/occ/payments/{id}/qr` et `/wallet-qr/{kind}`, pour les
membres de la party concernée. Hook : `wero_id` / `bancontact_phone` normalisés
(mobile → E.164 avec `+32` par défaut si commence par `0` ; e-mail en minuscules) et validés.

### `restaurants` (lecture publique)
| champ | type | notes |
|---|---|---|
| name | text req | |
| slug | text unique | |
| description | text | |
| emoji | text | visuel par défaut (ex. 🍕) |
| cover | file | optionnel |
| cover_url | url | optionnel (image distante) |
| cuisines | json `string[]` | ex. `["pizza","italien"]` |
| address | text | |
| lat, lng | number | |
| phone | text | |
| rating | number | 0–5 |
| rating_count | number int | |
| price_level | number int | 1–4 |
| eta_min, eta_max | number int | minutes |
| delivery_fee | number int | cents |
| min_order | number int | cents |
| providers | json | `[{ "id": "ubereats"|"takeaway", "url": "https://…" }]` |
| active | bool | |

Rules : list/view `active = true || @request.auth.role = "admin"` ;
create/update/delete `@request.auth.role = "admin"` (superusers : toujours).
Hooks (écritures via la collection) : slug/nom normalisés, slug unique (message clair),
`cuisines` nettoyées (minuscules, sans doublon), `providers` limités à
`ubereats`/`takeaway` en `https://` (liens vides retirés), `eta_min ≤ eta_max` ;
**suppression refusée** si le restaurant apparaît dans une party (le désactiver).

### `menu_categories` (lecture publique)
`restaurant` R(restaurants, cascade) · `name` · `position` int.
Rules : list/view `""` ; create/update/delete `@request.auth.role = "admin"`.
Supprimer une catégorie ne supprime pas ses articles (ils passent « sans catégorie »).

### `menu_items` (lecture publique)
| champ | type | notes |
|---|---|---|
| restaurant | R(restaurants) cascade, req | |
| category | R(menu_categories) | |
| name | text req | |
| description | text | |
| price | number int req | cents (prix de base) |
| emoji | text | |
| image | file | optionnel |
| tags | json `string[]` | `veggie`, `vegan`, `spicy`, `gluten_free`, `new`… |
| option_groups | json | voir ci-dessous |
| popular | bool | |
| available | bool | |
| position | number int | |

Rules : list/view `""` ; create/update/delete `@request.auth.role = "admin"`.
Hook : `option_groups` validés (`domain.ValidateOptionGroups`), `tags` nettoyés,
la catégorie doit appartenir au même restaurant, `restaurant` immuable.

```json
"option_groups": [
  { "id": "size", "name": "Taille", "min": 1, "max": 1,
    "choices": [ { "id": "m", "name": "Moyenne", "price": 0 },
                 { "id": "l", "name": "Large",   "price": 300 } ] },
  { "id": "extras", "name": "Suppléments", "min": 0, "max": 3,
    "choices": [ { "id": "cheese", "name": "Fromage", "price": 150 } ] }
]
```

### `parties` — une commande groupée
| champ | type | notes |
|---|---|---|
| code | text unique | 6 car. `[A-HJ-NP-Z2-9]` (sans 0/O/1/I) — généré serveur |
| title | text | ex. « Midi du vendredi » |
| host | R(users) | = créateur (forcé serveur) |
| members | R(users) multi | maintenu par le serveur (sert aux rules) |
| status | select | `lobby`, `voting`, `ordering`, `review`, `paying`, `closed`, `cancelled` |
| candidates | R(restaurants) multi | restaurants soumis au vote |
| restaurant | R(restaurants) | restaurant retenu |
| provider | select | `ubereats`, `takeaway`, `manual` |
| delivery_address | text | |
| notes | text | |
| voting_ends_at | date | indicatif (affiché en compte à rebours) |
| ordering_ends_at | date | indicatif |
| split_mode | select | `equal` (défaut) ou `proportional` — frais partagés |
| delivery_fee | number int | snapshot du restaurant au passage en `review` (éditable par l'hôte) |
| service_fee | number int | éditable par l'hôte |
| tip | number int | éditable par l'hôte |
| payer | R(users) | celui qui a avancé l'argent |
| dispatch | json | `{ "method": "...", "at": "ISO", "url": "..." }` |
| closed_at | date | |

Rules :
* list/view : `members.id ?= @request.auth.id || @request.auth.role = "admin"`
  (⚠ PocketBase (v0.36 à v0.40) : sur une relation multiple, `members ?= x` compare la
  valeur JSON brute et ne matche jamais → toujours écrire `members.id ?= …`)
* create : `@request.auth.id != ""` → le hook force `host`, `members=[host]`, `code`, `status=lobby`.
* update : `host = @request.auth.id` → le hook **refuse** toute modification de
  `code, host, members, status, restaurant, payer, dispatch, closed_at`
  (passent par `/api/occ/*`). `candidates` modifiable seulement en `lobby`.
  `delivery_fee`, `service_fee`, `tip`, `split_mode` figés à partir de `paying` ;
  plus aucune modification en `closed` / `cancelled`.
* delete : `host = @request.auth.id && status = "lobby"`.

### `party_members`
`party` R(parties, cascade) · `user` R(users) · `role` select(`host`,`member`) ·
`ready` bool. Index unique `(party, user)`.
Rules : list/view `party.members.id ?= @request.auth.id || @request.auth.role = "admin"` ; écriture serveur uniquement.

### `votes` — vote par approbation (on peut liker plusieurs restaurants)
`party` R(parties, cascade) · `user` R(users) · `restaurant` R(restaurants).
Index unique `(party, user, restaurant)`.
Rules :
* list/view `party.members.id ?= @request.auth.id || @request.auth.role = "admin"`
* create `user = @request.auth.id && party.members.id ?= @request.auth.id && party.status = "voting"`
  (+ hook : `restaurant` doit être dans `party.candidates`)
* delete `user = @request.auth.id && party.status = "voting"` ; update interdit.

### `order_items`
| champ | type | notes |
|---|---|---|
| party | R(parties) cascade | |
| user | R(users) | propriétaire de la ligne |
| menu_item | R(menu_items) | doit appartenir à `party.restaurant` et être `available` (requis par le hook ; non requis au niveau schéma pour qu'un ré-import de menu ne bloque pas sur l'historique) |
| quantity | number int | 1–20 |
| selected_options | json | `[{ "group": "size", "choices": ["l"] }]` |
| note | text | ≤ 200 car. |
| name | text | **serveur** : snapshot du nom |
| options_label | text | **serveur** : « Large, Fromage » |
| unit_price | number int | **serveur** : base + options |
| total | number int | **serveur** : unit_price × quantity |

Rules : list/view membres `|| @request.auth.role = "admin"` ; create `user = @request.auth.id && party.members.id ?= @request.auth.id && party.status = "ordering"` ;
update/delete `user = @request.auth.id && party.status = "ordering"`.
Hooks : valide options (min/max, ids), recalcule prix ; toute écriture remet
`party_members.ready = false` pour cet utilisateur.

### `payments` — part de chacun
| champ | type | notes |
|---|---|---|
| party | R(parties) cascade | |
| debtor | R(users) | qui doit |
| creditor | R(users) | le payeur |
| amount | number int | cents |
| method | select | `qr` (virement EPC), `wero`, `bancontact`, `link`, `cash`, `later`, `self` |
| status | select | `pending`, `declared`, `confirmed` |
| reference | text | communication, ex. `OCC K7M2QX Alice` |
| declared_at, confirmed_at | date | |

Rules : list/view `party.members.id ?= @request.auth.id || @request.auth.role = "admin"` ; écriture serveur uniquement.

> Les admins **lisent** toutes les parties et leurs enregistrements (support,
> panneau `/admin/commandes`) mais n'écrivent rien via les collections : l'annulation
> forcée passe par `POST /api/occ/admin/parties/{id}/cancel`. Les endpoints métier
> réservés aux membres (`summary`, `export`…) restent réservés aux membres.

## 4. Cycle de vie d'une party (state machine)

```
            ┌────────────── restaurant imposé par l'hôte ─────────────┐
 lobby ──► voting ──► ordering ──► review ──► paying ──► closed
   │          │          ▲            │                    
   │          │          └── reopen ──┘                    
   └──────────┴──────────┴────────────┴──────► cancelled (hôte, sauf closed)
```

| transition | condition |
|---|---|
| lobby → voting | ≥ 2 `candidates` |
| lobby → ordering | `restaurant` fourni dans la requête |
| voting → ordering | gagnant = plus de votes ; égalité → meilleur `rating`, puis nom ; l'hôte peut imposer `restaurant` (doit être candidat) |
| ordering → review | ≥ 1 `order_item` ; snapshot `delivery_fee` depuis le restaurant |
| review → ordering | réouverture (remet tous les `ready` à false) |
| review → paying | **uniquement** via `POST /payer` |
| paying → closed | automatique quand tous les `payments` sont `confirmed`, ou manuel par l'hôte |
| * → cancelled | hôte, si status ∉ {closed} |

## 5. API métier `/api/occ/*`

Auth : `Authorization: <token>` PocketBase (le SDK le gère). Erreurs au format
PocketBase `{ "status": 400, "message": "…", "data": {} }`, messages en français.

| méthode & route | auth | corps / query | réponse |
|---|---|---|---|
| `GET /api/occ/health` | — | | `{ "status": "ok", "version": "x.y.z" }` |
| `GET /api/occ/config` | — | | `{ "currency":"EUR", "defaultLocation":{lat,lng,label}, "providers":[{id,name,color,enabled}] }` |
| `GET /api/occ/restaurants/nearby` | — | `lat,lng,radiusKm(=5),q,cuisine` | `{ "items": [Restaurant & { "distanceKm": number }] }` triés par distance |
| `POST /api/occ/parties/join` | ✔ | `{ "code": "K7M2QX" }` | `{ "party": Party }` (idempotent ; statuts lobby/voting/ordering/review) |
| `POST /api/occ/parties/{id}/leave` | membre (≠ hôte) | | `{ "ok": true }` — seulement en lobby/voting/ordering ; supprime ses votes/items |
| `POST /api/occ/parties/{id}/transition` | hôte | `{ "to": Status, "restaurant"?: id }` | `{ "party": Party }` |
| `POST /api/occ/parties/{id}/ready` | membre | `{ "ready": bool }` | `{ "member": PartyMember }` (status `ordering` uniquement) |
| `GET /api/occ/parties/{id}/summary` | membre | | `Summary` (ci-dessous) |
| `POST /api/occ/parties/{id}/dispatch` | hôte | `{ "method": "ubereats"\|"takeaway"\|"export"\|"phone" }` | `{ "party": Party, "dispatch": Dispatch }` (status review/paying) |
| `POST /api/occ/parties/{id}/payer` | hôte | `{ "payer": userId }` | `{ "party": Party, "payments": Payment[] }` (review → paying ; en paying : recalcule si aucun paiement tiers confirmé) |
| `GET /api/occ/parties/{id}/export` | membre | `format=csv\|txt\|json` | fichier (`Content-Disposition: attachment`) |
| `POST /api/occ/payments/{id}/action` | voir | `{ "action": "declare"\|"confirm"\|"reset", "method"?: "qr"\|"wero"\|"bancontact"\|"link"\|"cash"\|"later" }` | `{ "payment": Payment }` |
| `GET /api/occ/payments/{id}/qr` | membre | | `PaymentQR` |
| `GET /api/occ/payments/{id}/wallet-qr/{kind}` | membre | `kind=wero\|bancontact` | image QR du payeur (stream, `Cache-Control: private`) ou 404 |
| `GET /api/occ/admin/stats` | admin | | `AdminStats` (ci-dessous) |
| `POST /api/occ/admin/import` | admin | `RestaurantImport` **ou** `RestaurantImport[]` ; `?dryRun=1` | `{ "report": ImportReport, "restaurant": id, "items": n }` — upsert par slug, tout ou rien ; invalide → **400** `{ status, message, data, report }` (rien n'est écrit) ; en dry run → 200 avec `report.valid=false` |
| `POST /api/occ/admin/import/csv` | admin | multipart `file` (CSV, voir ci-dessous) ; `?dryRun=1` | idem |
| `GET /api/occ/admin/export` | admin | | `RestaurantImport[]` (tous les restaurants, actifs ou non, menus complets ; `Content-Disposition: attachment`) — réimportable tel quel |
| `GET /api/occ/admin/users` | admin | `q` (nom/e-mail), `role`, `page`, `perPage` (≤ 200) | `{ page, perPage, totalItems, items: AdminUser[] }` (`AdminUser` = `id,name,email,role,color,avatar,verified,created,parties`) |
| `PATCH /api/occ/admin/users/{id}/role` | admin | `{ "role": "user"\|"admin" }` | `{ "user": AdminUser }` (400 si on se retire ses propres droits) |
| `POST /api/occ/admin/parties/{id}/cancel` | admin | | `{ "party": Party }` — annulation forcée (400 si `closed`/`cancelled`) |

« admin » = utilisateur `role = "admin"` **ou** superuser (401 sans auth, 403 sinon).

Règles `payments/{id}/action` :
* `declare` — débiteur seulement. `method=qr|wero|bancontact|link|cash` → `status=declared` ; `method=later` → `status=pending`.
* `confirm` — créancier (payeur) ou hôte → `status=confirmed`. Si tous confirmés → party `closed`.
* `reset` — créancier ou hôte → `status=pending`.

### `Summary`
```json
{
  "partyId": "…", "currency": "EUR", "status": "review",
  "restaurant": { "id": "…", "name": "…", "minOrder": 1500, "deliveryFee": 299 },
  "participants": [{
    "user": { "id": "…", "name": "Alice", "avatar": "", "color": "#…" },
    "ready": true,
    "items": [{ "id": "…", "name": "Margherita", "optionsLabel": "Large", "note": "",
                "quantity": 1, "unitPrice": 1250, "total": 1250 }],
    "subtotal": 1250, "sharedFees": 150, "total": 1400
  }],
  "consolidated": [{ "menuItem": "…", "name": "Margherita", "optionsLabel": "Large",
                     "quantity": 3, "total": 3750, "notes": ["sans oignon"] }],
  "itemsSubtotal": 4500, "deliveryFee": 299, "serviceFee": 0, "tip": 0,
  "sharedFees": 299, "grandTotal": 4799,
  "minOrderReached": true, "allReady": true, "splitMode": "equal"
}
```
`deliveryFee` : en lobby/voting/ordering, estimation = frais du restaurant ;
à partir de `review`, snapshot de la party. `allReady` = au moins un participant
avec article et tous ceux qui ont un article sont prêts.
Seuls les participants **ayant au moins un article** reçoivent une part des frais
partagés (`deliveryFee + serviceFee + tip`). `equal` : parts égales ;
`proportional` : au prorata du sous-total. Arrondi au centime par la méthode du
plus grand reste (ordre stable par id) → `Σ total = grandTotal` **exactement**.

### `Dispatch`
```json
{ "method": "ubereats", "url": "https://www.ubereats.com/…", "cartText": "…",
  "instructions": ["Ouvrez le restaurant…", "Ajoutez les articles…"] }
```
`cartText` = récap consolidé lisible (copiable / à dicter au téléphone).

### `PaymentQR`
```json
{ "amount": 1400, "reference": "OCC K7M2QX Alice", "beneficiary": "Bob Martin",
  "epc": "BCD\n002\n1\nSCT\n\nBob Martin\nBE71096123456769\nEUR14.00\n\n\nOCC K7M2QX Alice",
  "link": "https://paypal.me/bob/14.00EUR",
  "wero": { "id": "+32470123456", "hasQr": true },
  "bancontact": { "phone": "+32470123456", "hasQr": false },
  "methods": ["qr", "wero", "bancontact", "link", "cash", "later"] }
```
`epc` = payload **EPC069-12** (QR virement SEPA, lu par les apps bancaires
belges/européennes), `null` si le payeur n'a pas d'IBAN. `link` = lien de paiement
du payeur (montant ajouté pour paypal.me), `null` sinon. `wero` / `bancontact` :
`null` si le payeur n'a rien renseigné ; `hasQr` indique qu'une image est
disponible via `/wallet-qr/{kind}`. `methods` = moyens réellement proposés
(selon le profil du payeur), dans l'ordre d'affichage recommandé.

**Wero / Bancontact Pay** n'exposent aucun lien ni QR de demande de paiement
P2P utilisable par un tiers (leurs API sont réservées aux commerçants). L'UI
affiche donc : montant + communication à copier, identifiant du payeur à
copier, QR personnel du payeur s'il l'a téléversé, et un mini-guide
(« Ouvre ton app bancaire → Wero → Envoyer… »). Voir ADR 0003.

### `RestaurantImport`
```json
{ "slug": "…", "name": "…", "...champs restaurant": "…", "active": true,
  "categories": [{ "name": "Pizzas", "items": [{ "name": "…", "price": 1250, "tags": [], "popular": false,
                                                 "available": true, "option_groups": [] }] }] }
```
Clés = champs de `restaurants` (sauf `cover`) ; montants en centimes. Les clés
inconnues (`source_urls`, `menu_checked_at`…) sont ignorées. L'import **remplace**
le menu du restaurant (même slug). Si un restaurant existant est réimporté sans
coordonnées (`lat = lng = 0`) ou sans adresse, celles déjà enregistrées sont
conservées (cas de l'outil `/outils/export-menu.html`).

### `ImportReport`
```json
{ "dryRun": true, "valid": true, "errors": ["Ligne 4 : …"], "items": 14,
  "restaurants": [{ "slug": "chez-mario", "name": "Chez Mario", "exists": false, "active": true,
    "categories": 2, "items": 14, "errors": [], "warnings": ["Coordonnées manquantes : …"],
    "menu": [{ "category": "Pizzas", "name": "Margherita", "price": 1250, "tags": ["veggie"],
               "popular": true, "available": true, "options": 1 }],
    "restaurant": "id après import" }] }
```
Erreurs = bloquantes (slug, nom, prix, options, doublons, lignes CSV) ; avertissements
= non bloquants (coordonnées / adresse manquantes, menu vide, aucun lien fournisseur).

### CSV d'import (`/admin/import/csv`)
Une ligne **par article**, UTF-8 (BOM accepté), séparateur `;`, `,` ou tabulation
(détecté sur l'en-tête). Colonnes (en-tête obligatoire, ordre libre) :

| colonne | requis | notes |
|---|---|---|
| `restaurant_slug` | ✔ | regroupe les lignes d'un même restaurant |
| `restaurant_name` | ✔ pour un nouveau restaurant | une seule ligne suffit |
| `category` | ✔ | catégories dans l'ordre d'apparition |
| `item_name` | ✔ | |
| `description` | | |
| `price_eur` | ✔ | euros, décimales françaises acceptées : `12,50`, `12.5`, `12€50`, `1 234,50 €` |
| `tags` | | séparés par `\|`, `,` ou `;` (`veggie\|spicy`) |
| `popular` | | `oui`/`x`/`1`/`true` ou vide |
| `address`, `lat`, `lng`, `cuisines`, `phone`, `ubereats_url`, `takeaway_url` | | infos restaurant facultatives (première valeur non vide) ; `lat`/`lng` en `50,4542` ou `50.4542` |
| `emoji`, `available` | | facultatifs, par article |

Restaurant existant : seules les colonnes fournies remplacent ses infos ; le menu
est remplacé par celui du fichier ; un article du même nom garde ses `option_groups`
(et son emoji / sa description si la colonne est absente). Les options se gèrent
dans l'éditeur de menu ou en JSON.

### `AdminStats`
```json
{ "users": 42, "admins": 2, "restaurants": { "total": 13, "active": 12 }, "menuItems": 183,
  "parties": { "total": 30, "byStatus": { "lobby": 1, "voting": 0, "ordering": 2, "review": 0,
               "paying": 1, "closed": 24, "cancelled": 2 } },
  "partiesPerDay": [{ "date": "2026-09-10", "count": 0 }],
  "orderedTotal": 123450, "orderedLines": 210,
  "topRestaurants": [{ "id": "…", "name": "…", "slug": "…", "emoji": "🍕", "parties": 7, "amount": 45600 }] }
```
`partiesPerDay` : 30 derniers jours (UTC, jours vides inclus) ; montants hors parties
annulées ; top 5 par nombre de parties.

## 6. Fournisseurs de livraison (`internal/providers`)

Ni Uber Eats ni Takeaway (Just Eat Takeaway) n'offrent d'API publique permettant
à un tiers de **créer un panier client**. Stratégie :

* `Provider` interface : `ID()`, `Name()`, `Color()`, `Dispatch(restaurant, summary) Dispatch`.
* **Uber Eats** : deep link vers la page du restaurant (`providers[].url`) + guide
  pour lancer une *commande groupée Uber Eats* + récap copiable.
* **Takeaway** : deep link `takeaway.com` + récap copiable.
* **Export** : CSV / TXT / JSON. **Téléphone** : script de commande à dicter.
* Extension future : un adaptateur « API partenaire » (Uber Direct / Marketplace,
  JET Connect) se branche derrière la même interface sans toucher au front.

Import des menus : panneau `/admin` (JSON / CSV, aperçu puis confirmation),
`POST /api/occ/admin/import[/csv]`, ou admin PocketBase `/_/`.

### Données réelles (`backend/migrations/data/mons_restaurants.json`)
Embarqué (`go:embed`) au format `RestaurantImport[]`. La migration
`1760000003_replace_demo` l'importe (upsert) puis retire les 9 restaurants **fictifs**
de démo (désactivés plutôt que supprimés s'ils apparaissent dans une party). Fichier
absent, vide ou sans restaurant valide → aucune modification. Les entrées invalides
sont ignorées (journalisées) sans bloquer le démarrage. Sur une installation neuve
avec des données réelles, le seed de démo (`1760000001`) ne fait rien.

## 7. Configuration (variables d'environnement)

| variable | défaut | rôle |
|---|---|---|
| `OCC_ADMIN_EMAIL` / `OCC_ADMIN_PASSWORD` | — | crée/met à jour le superuser au démarrage ; le compte **utilisateur** de même e-mail reçoit le rôle `admin` |
| `OCC_ADMINS` | — | e-mails (séparés par des virgules) promus `admin` au démarrage ou à l'inscription |
| `OCC_PUBLIC_URL` | `http://localhost:8090` | URL publique (liens d'invitation, `meta.appURL`) |
| `OCC_DEFAULT_LAT` / `OCC_DEFAULT_LNG` / `OCC_DEFAULT_LABEL` | `50.4542` / `3.9567` / `Mons` | position par défaut |
| `OCC_SEED_DEMO` | `true` | charge les restaurants de démo au 1er démarrage (ignoré si `migrations/data` contient des restaurants réels) |
| `OCC_PROVIDERS` | `ubereats,takeaway` | fournisseurs activés |
| `OCC_PUBLIC_DIR` | `./pb_public` | dossier de la SPA |
