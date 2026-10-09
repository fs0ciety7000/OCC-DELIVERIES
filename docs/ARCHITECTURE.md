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
  Le shell (`AppShell`) s'abonne en plus à `parties` `*` : les rules ne livrent que
  les parties dont on est membre → bandeau « Commande en cours » et toasts de statut
  partout dans l'app (`useMyPartiesRealtime`). Il s'abonne aussi à `teams` `*` (rules : membres) : quand une
  commande d'équipe est lancée (`last_party` change), toast « Rejoindre » pour les membres pas encore dedans
  (`TeamLaunchListener`).
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
    payout.go                revtag Revolut / PayPal.me / lien libre : normalisation, liens avec montant
    geo.go                   haversine
    contact.go               téléphones (E.164, affichage) et adresses (« Rue X 12, 7000 Mons ») des restaurants
    team.go                  équipes (codes 8 car., noms, heure/jours habituels, rôles) et invités (prénom, e-mail réservé, inactivité)
    ratelimit.go             limiteur en mémoire à fenêtre glissante (création d'invités par IP)
    deadline.go              heures limites : rappels / clôtures automatiques (planification pure, idempotente)
  internal/notify/           notifications Web Push : messages français + liens, préférences, envoi (pool de workers, VAPID) + tests
  internal/providers/        adaptateurs Uber Eats / Takeaway / Deliveroo / weloveat / manuel + tests
  internal/menusync/         lecture des flux restaurants + menus (Deliveroo, weloveat, sites), normalisation, fusion + tests
  internal/feedsync/         synchronisation : lecture des sources + réconciliation pure avec la base + tests
  internal/enrich/           enrichissement OpenStreetMap (Nominatim) : téléphone / adresse / position manquants + tests
  internal/search/           recherche globale : index SQLite FTS5 (restos, plats), pliage des accents, fautes de frappe, collègues + tests / benchmark
  cmd/menusync/              outil CLI `menusync` (binaire séparé, /pb/menusync dans l'image)
  cmd/vapid/                 `go run ./cmd/vapid` : génère une paire de clés VAPID (Web Push)
  internal/app/              hooks PocketBase + routes /api/occ (glue) ; sync.go = planification / exécution de la synchronisation ;
                             push.go = notifications (hooks d'évènements, abonnements, VAPID) ; deadlines.go = planificateur des heures limites
  internal/catalog/           import / export des menus (JSON, CSV, rapport de validation)
  migrations/                migrations Go (schéma, seed démo, rôles admin, données réelles)
    data/                    mons_restaurants.json (données réelles embarquées), mons_ubereats.json (instantané Uber Eats)
frontend/
  src/lib/                   pb client, types, api, format, hooks realtime
  src/components/ui/         design system (Button, Card, Sheet, Badge, Avatar…)
  src/features/<domaine>/    party, restaurants, auth, profile, payments, admin
  src/routes/                pages
  src/pwa/                   enregistrement du service worker, invite d'installation, PwaRuntime (bandeau hors ligne, rejeu, toasts temps réel)
  pwa/                       service worker : sw.js (gabarit), sw-routes.js (règles de cache pures), plugin.ts (plugin Vite → dist/sw.js)
  scripts/generate-icons.mjs icônes PWA (PNG 192/512, maskable, badge, apple-touch) depuis public/favicon.svg
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
| banned | bool, **hidden** | compte suspendu (migration `1760000014`) : toute authentification et toute requête refusées (403 « Compte suspendu… ») |
| banned_reason | text ≤ 300, **hidden** | motif (admins uniquement) |
| banned_at | date, **hidden** | posé par le serveur à la suspension |
| deleted_at | date, **hidden** | compte supprimé = **anonymisé** (voir §3 *Comptes*) |
| is_guest | bool (visible, **écrit par le serveur seulement**) | compte **invité·e** créé par `POST /api/occ/guest` (prénom seul, migration `1760000016`) ; un client ne peut ni le poser (forcé à `false` à l'inscription) ni le changer (403) ; lisible pour l'UI (bandeau invité, badges « Invité·e ») — voir §3 *Invités* |
| notify_prefs | json, **hidden** | préférences de notification `{ "party": bool, "payments": bool, "reminders": bool }` (migration `1760000015`) ; vide / clé absente = activé ; lu et écrit par `GET/PATCH /api/occ/push/prefs` |
| password_set | bool, **hidden** | `false` pour un compte créé avec Google (mot de passe aléatoire) tant que son titulaire n'en a pas choisi un (lien « mot de passe oublié », changement depuis le profil) ; sert à interdire de dissocier Google d'un compte qui ne pourrait plus se connecter |

Champs **hidden** : jamais renvoyés par l'API des collections (même au titulaire), ignorés en écriture
pour tout non-superuser ; lus par les endpoints `/api/occ/me/account` et `/api/occ/admin/users`.

Rules : list/view `@request.auth.id != ""` ; update `id = @request.auth.id` ; **delete `nil`**
(superuser seulement, migration `1760000014`) : la suppression passe par l'anonymisation
(`POST /api/occ/me/delete`, `DELETE /api/occ/admin/users/{id}`), une suppression réelle casserait
l'historique des commandes (hôte, lignes, paiements).
Rôle : un hook refuse (403) toute création/modification de `role` par la collection
si le demandeur n'est ni `admin` ni superuser ; les admins changent les rôles via
`PATCH /api/occ/admin/users/{id}/role` (on ne peut pas retirer ses propres droits).
Bootstrap : au démarrage, les comptes dont l'e-mail figure dans `OCC_ADMIN_EMAIL`
ou `OCC_ADMINS` passent `admin` ; un compte créé plus tard avec un de ces e-mails
(mot de passe, OAuth2, admin `/_/`) naît `admin`.

### Comptes : Google, e-mails, suspension, suppression (`app/accounts.go`, `app/authsettings.go`)
* **Google (OAuth2 PocketBase)** — au démarrage, si `OCC_GOOGLE_CLIENT_ID` **et** `OCC_GOOGLE_CLIENT_SECRET`
  sont définis, le fournisseur `google` est activé sur `users` avec ces identifiants et les champs
  `name` / `avatar` sont mappés (nom et photo Google à l'inscription). Sans ces variables, rien n'est
  touché (une configuration faite dans `/_/` est conservée ; pour désactiver : `/_/` → *users* → *OAuth2*).
  Flux « popup » du SDK : redirection vers `{OCC_PUBLIC_URL}/api/oauth2-redirect`.
  - Premier passage : compte créé (`verified = true`, `password_set = false`, couleur, rôle `admin` si
    l'e-mail est dans `OCC_ADMIN_EMAIL` / `OCC_ADMINS`) ; sans nom Google, nom déduit de l'e-mail
    (`alice.dupont@…` → « Alice Dupont »). Pas d'e-mail de vérification.
  - **Liaison par e-mail** (comportement PocketBase v0.40, `apis/record_auth_with_oauth2.go`, testé) : lien
    Google déjà connu → ce compte ; sinon, connecté → lien ajouté au compte courant ; sinon compte de **même
    e-mail** : s'il est **vérifié**, Google y est simplement lié (mot de passe conservé) ; s'il **n'est pas
    vérifié**, PocketBase le lie mais **remplace son mot de passe par un mot de passe aléatoire** (protection
    contre la prise de compte) et le marque vérifié → `password_set = false` (l'utilisateur peut en choisir
    un via « Mot de passe oublié »).
  - Compte suspendu : refusé avant toute liaison (403).
  - Dissocier Google (`DELETE /api/occ/me/providers/google`, ou la collection `_externalAuths`) : refusé
    (400) si c'est le dernier moyen de connexion d'un compte sans mot de passe choisi.
* **E-mails** — SMTP lu au démarrage depuis `OCC_SMTP_*` / `OCC_MAIL_*` (appliqué seulement si
  `OCC_SMTP_HOST` est défini ; aucun secret journalisé) ; `meta.appURL = OCC_PUBLIC_URL`. Modèles
  **français** (`app/mailtemplates.go`, réinstallés à chaque démarrage) dont les liens visent la SPA :
  vérification `{APP_URL}/auth/verifier/{TOKEN}`, mot de passe `{APP_URL}/auth/reinitialiser/{TOKEN}`,
  changement d'adresse `{APP_URL}/auth/changer-email/{TOKEN}` ; alerte « nouvelle connexion » et code OTP
  traduits. Inscription par mot de passe → e-mail de vérification si SMTP actif. **Politique** : un compte
  non vérifié reste pleinement utilisable (bandeau discret « Confirme ton adresse e-mail » avec « Renvoyer »).
  `GET /api/occ/config` expose `mailEnabled`.
* **Suspension** — `banned = true` (endpoint admin, ou `/_/`) : le hook d'update fait tourner `tokenKey`
  (tous les jetons émis deviennent invalides → 401, et les connexions realtime perdent leur auth) ;
  `OnRecordAuthRequest` refuse toute authentification (mot de passe, Google, OTP, refresh) avec
  **403 « Compte suspendu. Contacte un·e administrateur·rice d'OCC Deliveries. »** ; un middleware global
  (juste après le chargement du jeton) refuse en 403 toute requête portant le jeton d'un compte suspendu
  (défense en profondeur si `banned` est écrit sans passer par les hooks).
* **Suppression = anonymisation** (`anonymizeUser`, en transaction) : `name` → « Compte supprimé »,
  `email` → `deleted-<id>@occ.invalid` (unique, TLD réservé), avatar retiré, rôle `user`, mot de passe
  aléatoire, `tokenKey` renouvelé, `banned = true`, `deleted_at`, `payout_profiles` supprimé, liens OAuth2,
  origines de connexion, MFA et OTP supprimés. Parties, lignes de commande, votes et paiements sont
  **conservés** (montants et totaux inchangés, le nom affiché devient « Compte supprimé »). Irréversible
  (pas de réactivation). Une nouvelle connexion Google avec l'ancienne adresse crée un compte neuf.

### Invités sans compte (« lien magique, juste un prénom » — `app/guests.go`, migration `1760000016`)
* **Création** — `POST /api/occ/guest { name, color?, partyCode | teamCode }` (sans session ; avec une session → 400) :
  un code **valide** est obligatoire (commande en lobby/voting/ordering/review, ou équipe non archivée), prénom
  1–40 caractères avec au moins une lettre (`domain.NormalizeGuestName`), couleur `#RRGGBB` facultative (sinon tirée
  au hasard). En une transaction : enregistrement `users` avec `is_guest = true`, e-mail
  `guest-<id>@guest.occ.invalid` (TLD réservé, jamais d'envoi), mot de passe aléatoire, `verified = false`,
  `password_set = false`, rôle `user` ; puis ajout à la commande (`members` + `party_members`) ou à l'équipe.
  Réponse `{ token, record, party: { id, title } | null, team: { id, name } | null }`.
* **Jeton** — `record.NewStaticAuthToken(30 j)` (`domain.GuestTokenDays`). `auth-refresh` d'un·e invité·e renvoie un
  **nouveau** jeton statique de 30 jours (fenêtre glissante tant qu'il ou elle utilise l'app) ; sans activité, le
  jeton expire.
* **Limite de débit** — en mémoire par IP (`domain.RateLimiter`, fenêtre glissante) : **10 créations / heure**
  (les tentatives à code invalide comptent : pas de force brute), aperçu `GET /api/occ/invites/{code}` 60 / minute.
  Dépassement → **429** « Trop de tentatives… ». (Mémoire du processus : remise à zéro au redémarrage.)
* **Ce qu'un·e invité·e peut faire** : voir la commande, voter, commander, se déclarer prêt·e, payer sa part (QR EPC,
  liens), rejoindre une équipe et en un geste ses commandes, changer son prénom / sa couleur, supprimer son compte.
* **Interdits (rule + hook / garde)** : créer une commande (`parties.createRule … && @request.auth.is_guest = false`
  + hook 403 « Crée ton compte… ») et donc lancer la commande du jour d'une équipe (403) ; créer une équipe (rule + hook) ;
  être admin d'équipe (hook `teams` : 400) ; être **admin** de l'app (`isAdmin` exclut les invités,
  `PATCH /admin/users/{id}/role` → 400, et filet de sécurité : un enregistrement invité repasse toujours `role = user`) ;
  enregistrer un `payout_profiles` (rules create/update `… && @request.auth.is_guest = false` + hook 403).
  Aucun e-mail n'est envoyé aux adresses `*.invalid` (hook `OnMailerSend` : invités et comptes supprimés).
* **Créer mon compte (même enregistrement)** — `POST /api/occ/me/upgrade { email, password, passwordConfirm?, name? }`
  (invité·e seulement, sinon 400) : e-mail valide, non `.invalid`, libre (sinon 400 « Un compte existe déjà… ») ;
  mot de passe ≥ 8 ; → `is_guest = false`, `password_set = true`, `verified = false` (e-mail de vérification si SMTP),
  réponse d'authentification PocketBase `{ token, record }`. **Google** : un·e invité·e connecté·e qui passe par
  `auth-with-oauth2` (compte Google encore lié à personne) est converti·e : e-mail Google, vérifié,
  `is_guest = false` (400 si l'adresse Google appartient déjà à un autre compte). Commandes, votes, paiements et
  équipes sont conservés (même id).
* **Nettoyage** — tâche cron `occGuestCleanup` (`40 3 * * *`, UTC) : invités non supprimés sans activité depuis
  **60 jours** (`domain.GuestInactive` ; activité = max de `users.updated`, arrivées dans une commande, lignes,
  votes, paiements, origines de connexion) → retirés de leurs équipes puis **anonymisés** (`anonymizeUser`, comme une
  suppression de compte : historique et totaux conservés).
* **Admin** — `AdminUser.isGuest` (badge « Invité ») et filtre `GET /api/occ/admin/users?status=guest`.

### `payout_profiles` — coordonnées de remboursement (privées)
| champ | type | notes |
|---|---|---|
| user | R(users) unique, requis | |
| holder_name | text | bénéficiaire du virement |
| iban | text | validé mod-97, stocké sans espaces, majuscules |
| bic | text | optionnel |
| revolut_tag | text (≤ 32) | revtag Revolut, minuscules sans `@` (saisie `@jdoe`, `revolut.me/jdoe`… acceptée) |
| paypal_me | text (≤ 40) | nom PayPal.me (saisie `jdoe`, `paypal.me/jdoe`, `paypal.com/paypalme/jdoe` acceptée) |
| payment_link | url | autre lien de paiement (Lydia/Sumeria, Wise Business…), `https://` ajouté si absent |
| wero_id | text | n° de mobile (E.164, ex. `+32470123456`) **ou** e-mail enregistré sur Wero |
| bancontact_phone | text | n° de mobile (E.164) lié à Bancontact Pay |
| wero_qr | file | image (png/jpg/webp, ≤ 1 Mo, `protected`) — QR « recevoir » généré dans l'app bancaire |
| bancontact_qr | file | idem pour Bancontact Pay |

Rules : toutes `user = @request.auth.id` (create : `@request.auth.id != "" && user = @request.auth.id`) ;
create et update exigent en plus `@request.auth.is_guest = false` (migration `1760000016` : pas de profil pour un·e invité·e).
Jamais exposé aux autres membres : QR EPC, identifiants Wero/Bancontact et images
QR ne sortent que via `/api/occ/payments/{id}/qr` et `/wallet-qr/{kind}`, pour les
membres de la party concernée. Hook : `wero_id` / `bancontact_phone` normalisés
(mobile → E.164 avec `+32` par défaut si commence par `0` ; e-mail en minuscules) et validés ;
`revolut_tag` / `paypal_me` / `payment_link` normalisés (`domain/payout.go`) et un lien
`revolut.me/…` ou `paypal.me/…` collé dans `payment_link` est déplacé dans le champ structuré
(migration `1760000013` : même traitement pour les profils existants).

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
| address | text | normalisée `Rue X 12, 7000 Mons` (`domain.NormalizeAddress` : pays retiré, numéro en tête déplacé après la rue, casse corrigée, localité déduite du code postal autour de Mons et inversement) |
| lat, lng | number | |
| phone | text | **E.164** (`+3265352964`, `domain.NormalizeRestaurantPhone` : `0…` → `+32…`, `00` → `+`, `+32 (0)` corrigé, premier numéro valide d'une liste, déchets refusés) ; affiché groupé `+32 65 35 29 64` (`formatPhone` côté front, `domain.FormatPhone` côté Go) |
| rating | number | 0–5 |
| rating_count | number int | |
| price_level | number int | 1–4 |
| eta_min, eta_max | number int | minutes |
| delivery_fee | number int | cents |
| min_order | number int | cents |
| providers | json | `[{ "id": "ubereats"|"takeaway"|"deliveroo"|"weloveat", "url": "https://…" }]` |
| active | bool | |
| source_key | text (index) | synchronisation : clé stable `<source>:<url normalisée>` (ex. `takeaway-site:tomomons.be`) |
| sources | json | synchronisation : `[{ "provider": "deliveroo", "url": "https://…", "checked_at": "2026-10-09" }]` |
| locked | bool | verrouillé : la synchronisation ne le modifie plus (choix **explicite** de l'admin, jamais automatique) |
| stale_since | date | obsolète : plus proposé par aucune source activée depuis cette date (vide sinon) ; reste actif |
| partial_menu | bool | **carte partielle** : seuls quelques plats sont connus (catégorie « Aperçu », instantané Uber Eats) ; badge « Aperçu du menu » + bandeau vers Uber Eats — migration `1760000006` |
| geo_approx | bool | **position approximative** (lieu par défaut `OCC_DEFAULT_LAT/LNG`) : distance masquée dans l'UI, classé après les autres dans `/nearby` |
| enriched_from | json \| null | **serveur** (migration `1760000012`) : provenance des champs complétés depuis OpenStreetMap `{ "provider": "osm", "url": "https://www.openstreetmap.org/node/…", "fields": ["phone","address","geo"], "checked_at": "2026-10-09" }` — affiche le crédit ODbL « Coordonnées : © contributeurs OpenStreetMap » ; jamais pris du client, un champ modifié par un admin n'est plus crédité |
| items_count | number int ≥ 0 | **serveur** : nombre de plats **disponibles** (migration `1760000010`, rétro-calculé). Recalculé après chaque création / modification / suppression d'article via la collection, à la fin de `catalog.Import` et après chaque restaurant écrit par la synchronisation (`catalog.RefreshItemsCount`, simple `UPDATE … COUNT(*)`, sans hook ni `updated`) ; toute valeur envoyée par un client est remplacée par le compte réel |

Rules : list/view `active = true || @request.auth.role = "admin"` ;
create/update/delete `@request.auth.role = "admin"` (superusers : toujours).
Hooks (écritures via la collection) : slug/nom normalisés, slug unique (message clair),
`cuisines` nettoyées (minuscules, sans doublon), `providers` limités aux plateformes
(`ubereats`, `takeaway`, `deliveroo`, `weloveat`) en `https://` (liens vides retirés), `eta_min ≤ eta_max` ;
`phone` normalisé en E.164 (400 « Numéro de téléphone invalide… » si un numéro **nouveau ou modifié** est illisible ;
un numéro illisible déjà stocké ne bloque pas les autres modifications), `address` normalisée, `enriched_from` géré
par le serveur (voir ci-dessus) ;
**suppression refusée** si le restaurant apparaît dans une party (le désactiver).
**Verrouillage** : uniquement explicite (interrupteur « Verrouillé »). Une modification admin ne
verrouille pas : la synchronisation peut ensuite remettre à jour la fiche (migration `1760000007`
a levé les verrous posés par l'ancien verrouillage automatique).
**Cartes incomplètes** : un restaurant actif dont `items_count` < `app_settings.min_menu_items`
(si > 0) est **absent des listes publiques** (`/api/occ/restaurants/nearby` : accueil, page Restos,
choix des candidats) mais reste lisible par lien direct (`/api/collections/restaurants/records/{id}`),
utilisable dans les parties existantes (candidats / restaurant retenu : les transitions ne vérifient que
`active`) et visible des admins. Rien n'est écrit sur le restaurant : la règle est évaluée à la lecture
(`domain.HiddenIncomplete`), donc un menu complété réapparaît tout seul et baisser le seuil (0) ré-affiche tout.
**Import** (`catalog.Import`) : le restaurant visé est retrouvé par slug, sinon par un lien de
plateforme identique (hôte + chemin, liens génériques type page ville ignorés), sinon par le nom
normalisé (casse, accents, « (Mons) », « - Mons ») — dans les deux derniers cas seulement si un seul
restaurant correspond ; le slug existant est conservé, les liens fusionnés et les champs absents du
fichier (note, délai, frais, cuisines, description…) gardent leur valeur. Téléphone (E.164 si lisible,
sinon tel quel) et adresse normalisés. En fin d'import : `items_count` puis index de recherche
(`search.Reindex`, voir *Recherche globale*).
**Normalisation existante** : la migration `1760000012` réécrit une fois tous les téléphones / adresses
stockés (idempotente, verrouillés compris : seule la forme change ; un téléphone illisible est conservé).

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
| price | number int ≥ 0 | cents (prix de base ; 0 permis quand le prix est dans les options — migration `1760000005`) |
| emoji | text | |
| image | file | optionnel |
| tags | json `string[]` | `veggie`, `vegan`, `spicy`, `gluten_free`, `new`… |
| option_groups | json | voir ci-dessous |
| popular | bool | |
| available | bool | |
| position | number int | |
| source_key | text | synchronisation : clé du plat dans son restaurant (slug du nom, `--<catégorie>` si le nom se répète) |
| sources | json | synchronisation : source du menu (`[{ provider, url, checked_at }]`) |
| locked | bool | verrouillé : jamais modifié par la synchronisation |

Rules : list/view `""` ; create/update/delete `@request.auth.role = "admin"`.
Hook : `option_groups` validés (`domain.ValidateOptionGroups`), `tags` nettoyés,
la catégorie doit appartenir au même restaurant, `restaurant` immuable.
**Verrouillage** : uniquement explicite (interrupteur / cadenas de l'éditeur de menu).

```json
"option_groups": [
  { "id": "size", "name": "Taille", "min": 1, "max": 1,
    "choices": [ { "id": "m", "name": "Moyenne", "price": 0 },
                 { "id": "l", "name": "Large",   "price": 300 } ] },
  { "id": "extras", "name": "Suppléments", "min": 0, "max": 3,
    "choices": [ { "id": "cheese", "name": "Fromage", "price": 150 } ] }
]
```

### `app_settings` — réglages globaux (une seule ligne)
| champ | type | notes |
|---|---|---|
| min_menu_items | number int 0–100 | seuil « cartes incomplètes » : restaurants de moins de N plats disponibles masqués des listes publiques ; `0` = désactivé |

Rules : list/view/update `@request.auth.role = "admin"` ; create/delete `nil` (la ligne est créée par la
migration `1760000010`, avec `10` si des données réelles sont embarquées — production — sinon `0` — démo).
Hook (update via la collection) : entier 0–100, message en français. Le front passe par
`GET/PATCH /api/occ/admin/settings` ; le public lit le seuil dans `/api/occ/config` (`minMenuItems`).

### `sync_sources` — sources de la synchronisation (admin)
| champ | type | notes |
|---|---|---|
| provider | select req | `deliveroo`, `weloveat`, `takeaway-site`, `jsonld`, `ubereats-snapshot` (migration `1760000006`) |
| label | text | nom affiché (défaut : le fournisseur) |
| url | text | page liste Deliveroo de la ville ; racine de l'API weloveat (vide = `https://api.weloveat.be/api/`) ; site du restaurant (`takeaway-site`, `jsonld`) — `https://` requis sauf weloveat ; toujours vide pour `ubereats-snapshot` (le hook l'efface) |
| city | text | ville de recherche (`mons`, seule connue ; défaut) — rayon 8 km |
| priority | number int | plus petit = lu en premier et menu préféré quand un restaurant est sur plusieurs sources |
| enabled | bool | |
| options | bool | requêtes supplémentaires pour les options (suppléments weloveat : 1 requête par plat) ; défaut `false` |
| last_run_at | date | **serveur** |
| last_status | text | **serveur** : `ok: 40 restaurants`, `blocked: …`, `failed: …` |

Rules : toutes `@request.auth.role = "admin"`. Hook : fournisseur, URL et ville validés,
`last_*` non modifiables via l'API. Seed (`1760000005`) : un `takeaway-site` par URL de
`migrations/data/mons_takeaway_sites.txt` (priorités 10…), Deliveroo Mons (50), weloveat Mons (60) ;
seed (`1760000006`) : « Uber Eats (instantané connecteur) » (`ubereats-snapshot`, 70, activée).

### `sync_runs` — exécutions de la synchronisation
| champ | type | notes |
|---|---|---|
| started_at, finished_at | date | |
| status | select | `running`, `success`, `partial`, `failed`, `blocked` |
| trigger | select | `cron`, `manual`, `startup` |
| stats | json | `SyncStats` : `{ restaurants_created, restaurants_updated, restaurants_stale, items_created, items_updated, items_price_changed, items_unavailable, restaurants_enriched }` (`restaurants_enriched` : complétés depuis OpenStreetMap ; absent des exécutions antérieures) |
| changes | json | ≤ 500 diffs lisibles (`"Tomo — Miso ramen : 14,50 € → 15,00 €"`, `"X : nouveau restaurant (52 plats)"`, `"… : retiré de la carte (indisponible)"`, `"Pizza Hut — téléphone ajouté, adresse ajoutée, position précisée (OpenStreetMap)"`) |
| sources | json | `SyncSourceResult[]` : `{ id, label, provider, url, status: "ok"\|"blocked"\|"failed", message, restaurants, network, cached, durationMs }` |
| log | text | journal (≤ 150 000 car., mis à jour toutes les 5 s pendant l'exécution) |
| error | text | |

Rules : list/view `@request.auth.role = "admin"` ; écriture **serveur uniquement** (rules `nil`).
Une exécution `running` trouvée au démarrage passe `failed` (« interrompue »).

### Synchronisation automatique (`internal/feedsync` + `internal/app/sync.go`)
1. Sources activées triées par priorité, lues **une par une** par un seul `menusync.Fetcher`
   (politesse §6) ; 403 / page anti-robot → source `blocked`, erreur ou 0 restaurant → `failed`,
   les suivantes sont lues quand même.
2. Fusion (`menusync.MergeIn`, priorité des sources) puis **réconciliation pure**
   (`feedsync.Reconcile`, testée sur fixtures) contre un instantané de la base :
   * rapprochement : `source_key`, puis lien plateforme / page source identique, puis
     nom + position (`menusync.SameRestaurant`) — deux passes pour qu'un rapprochement flou ne
     prenne pas la fiche liée à une autre clé ;
   * restaurant verrouillé : ignoré (ni modifié, ni marqué obsolète) ;
   * champs « éditoriaux » (nom, description, emoji, adresse, coordonnées, téléphone, cuisines,
     image) remplis seulement s'ils sont vides ; note, nombre d'avis, frais, minimum, délai
     suivent la source quand elle a une valeur ; liens plateformes ajoutés ; jamais de valeur vide ;
   * plats (non verrouillés) rapprochés par `source_key` puis nom : prix, description, options
     (si la source en a), disponibilité, populaire, catégorie suivent la source ; nouveaux plats
     et catégories créés ; plats absents → `available = false` (jamais supprimés) ; menu vide
     côté source → aucun changement ;
   * restaurant connu d'une source (`sources` non vide) qu'aucune source ne renvoie →
     `stale_since` (sauf si un de ses fournisseurs a échoué / été bloqué pendant l'exécution) ;
     réapparition → effacé.
   * **instantané Uber Eats** (source `ubereats-snapshot`, aucune requête réseau : lit le fichier
     embarqué `migrations/data/mons_ubereats.json`, voir *Instantané Uber Eats* ci-dessous),
     appliqué **après** les flux (`feedsync.Reconcile`, `Options.Snapshot`) contre l'état
     planifié (base + restaurants créés par les flux dans la même exécution) :
     - rapprochement : lien Uber Eats (URL normalisée) ou `source_key` `ubereats-snapshot:<url>`,
       puis nom normalisé souple (`menusync.NameMatch` : « (Mons) », « - Mons », « (Independant) »,
       accents / casse / ponctuation ignorés ; même clé, ou une clé contenue dans l'autre (≥ 6 car.),
       ou similarité « token-set » ≥ 0,8 sur des mots significatifs) ; contrôle de distance
       (≤ 1,5 km) **seulement** si les deux ont de vraies coordonnées (`geo_approx` = pas de coordonnées) ;
     - restaurant reconnu : verrouillé → ignoré ; sinon lien Uber Eats ajouté s'il manque, note /
       nombre d'avis / délai remplis **seulement s'ils valent 0**, **menu jamais touché** ;
     - restaurant reconnu à carte partielle (créé par l'instantané) : l'aperçu suit le fichier
       (plats ajoutés / prix / absents → indisponibles), note et délai aussi — sauf si un flux le
       liste également (le flux l'emporte, l'instantané ne fait que compléter) ;
     - non reconnu : création **active** avec slug, nom normalisé, cuisines (catégories normalisées),
       emoji deviné, note, avis, `eta_min` = délai, `eta_max` = délai + 15, lien Uber Eats,
       adresse / coordonnées du fichier (0 → lieu par défaut et `geo_approx = true`),
       `partial_menu = true`, catégorie « Aperçu » avec les plats d'exemple (aucune si pas de plat) ;
     - un flux (Deliveroo, weloveat, site…) reconnu sur un restaurant `partial_menu` (même règles de
       nom souple) **remplace l'aperçu** dès qu'il apporte ≥ 5 plats (`partial_menu = false`, plats de
       l'aperçu rapprochés par nom ou rendus indisponibles, vraie position si `geo_approx`) ; avec
       moins de 5 plats, le menu reste celui de l'aperçu ;
     - obsolescence : un restaurant présent dans le fichier est « vu » à chaque exécution (jamais
       marqué obsolète parce que les flux ne le listent pas) ; retiré du fichier → obsolète comme
       pour toute source ; fichier illisible → source `failed`, rien n'est marqué obsolète.
3. Écriture **par restaurant dans une transaction** (`RunInTransaction`) ; un enregistrement
   verrouillé entre-temps est laissé tel quel. En fin de transaction : `items_count` puis index de
   recherche du restaurant (`refreshAfterSync` → `catalog.RefreshItemsCount` + `search.Reindex`).
4. **Enrichissement OpenStreetMap** (`internal/enrich` + `app/enrich.go`, `OCC_ENRICH_ENABLED`, défaut `true`) à la
   fin de l'exécution : restaurants **actifs, non verrouillés** sans téléphone, sans adresse ou sans vraie position
   (`geo_approx` ou 0) — en exécution ciblée, seulement ceux qu'elle a écrits. Requête Nominatim
   `search?format=jsonv2&addressdetails=1&extratags=1&namedetails=1&countrycodes=be&q=<nom nettoyé>, <localité de
   l'adresse ou OCC_DEFAULT_LABEL>`. **Politique Nominatim stricte** : User-Agent identifié
   `OCC-Deliveries-enrich/1.0 (+https://eat.fs0ciety.org)`, une requête à la fois, ≥ 1,1 s entre deux, cache disque
   `pb_data/menusync-cache/nominatim` de **30 jours** (réponses vides comprises), au plus **60 requêtes réseau par
   exécution** (les suivantes à la prochaine ; le cache ne compte pas), aucun nouvel essai, 429 / 403 / 5xx = arrêt de
   l'enrichissement pour l'exécution. Résultat accepté (`enrich.Pick`, pur et testé) seulement si : `amenity` =
   restaurant, fast_food, cafe, ice_cream, food_court (bar, pub, biergarten : même nom ou nom contenu) ou `shop` =
   bakery, pastry, deli, confectionery, ice_cream ; nom reconnu (`menusync.NameMatch` sur tous les noms OSM : name,
   brand, official_name, alt_name…) ; ≤ 12 km du lieu par défaut ; ≤ 1,5 km des coordonnées réelles s'il en a ; même
   rue que l'adresse connue ; deux homonymes à plus de 300 m sans position connue = ambigu, rien n'est écrit.
   Écriture (transaction, re-vérifiée) des **seuls champs vides** : téléphone (`phone` / `contact:phone`, E.164),
   adresse (`addressdetails`, normalisée), position (et `geo_approx = false`) ; `enriched_from` mis à jour ; jamais de
   valeur remplacée. Données © contributeurs OpenStreetMap (ODbL) : crédit affiché sur la fiche restaurant et dans
   l'envoi « Téléphone ».
5. Planification : job `app.Cron()` chaque minute qui vérifie `OCC_SYNC_CRON` à l'heure de
   **Europe/Brussels** (le cron PocketBase est en UTC ; `time/tzdata` embarqué). Une seule
   exécution à la fois (verrou en mémoire ; manuel → 409, tick cron ignoré). Exécution en
   goroutine (jamais dans une requête), annulée proprement à l'arrêt. Cache disque
   `pb_data/menusync-cache`, TTL 20 h (30 jours pour Nominatim).

**Coordonnées lues par les flux** (`menusync`, normalisées par `Restaurant.Clean`) : sites Takeaway et JSON-LD —
entité schema.org (Restaurant, LocalBusiness…) en microdonnées ou JSON-LD (`telephone`, `address`, `geo`), puis liens
`tel:`, bloc « Contact » du pied de page, JSON embarqué dans les scripts ; Deliveroo — `__NEXT_DATA__` (adresse +
`postCode`, épingle de carte, bloc « Coordonnées », téléphone de l'objet restaurant) ; weloveat — `phone_number` /
`phone` / `telephone`, `address` ou ses parties (rue, numéro, code postal, localité), coordonnées de la recherche ;
seules les coordonnées **professionnelles** de l'établissement sont gardées (`user`, `owner`, `manager`, e-mails,
jetons… retirés avant le cache).

### Recherche globale (`internal/search`, migration `1760000017`)
Index **SQLite FTS5** (le SQLite de PocketBase — modernc, 3.53 — inclut FTS5 et `fts5vocab`), en tables
SQL simples, **hors collections PocketBase** (invisibles dans `/_/`, incluses dans les sauvegardes) :

| table | contenu |
|---|---|
| `occ_search_restaurants` | FTS5 `name`, `cuisines`, `address` |
| `occ_search_items` | FTS5 `name`, `description`, `category` (nom de la catégorie), `restaurant` (nom du resto) |
| `occ_search_restaurant_docs` / `occ_search_item_docs` | `docid` (= `rowid` FTS) ↔ id PocketBase (`restaurant` ; `item`, `restaurant` indexé) — une colonne id non indexée dans la table FTS serait parcourue à chaque suppression |
| `occ_search_vocab_restaurants` / `occ_search_vocab_items` | `fts5vocab(…, 'row')` : termes indexés (corrections de fautes) |

* **Texte plié en Go** à l'indexation **et** à la requête (`search.Fold` : minuscules, accents retirés,
  ligatures `œ` → `oe`, `æ` → `ae`, `ß` → `ss`) ; tokenizer `unicode61 remove_diacritics 2` (défense en
  profondeur), index de préfixes `2 3`. « Râmen », « RAMEN », « ram » trouvent « ramen ».
* **Visibilité jamais stockée** : jointure à la requête sur `restaurants` / `menu_items` → restaurant
  `active`, cartes incomplètes masquées (`items_count < min_menu_items`, même règle que `/nearby`), plats
  `available` seulement. Prix, disponibilité et `items_count` ne demandent donc aucune réindexation.
* **Mise à jour** : hooks `OnRecordCreate/Update/Delete` (autour de l'écriture, **dans sa transaction**
  s'il y en a une) sur `restaurants` (ligne seule ; ligne + plats si le **nom** change), `menu_items`
  (si nom, description, catégorie ou restaurant changent), `menu_categories` (renommage / suppression →
  plats du resto) ; appels explicites `search.Reindex(tx, restaurantID)` en fin de `catalog.Import` et de
  chaque restaurant écrit par la synchronisation. Une erreur d'index est journalisée (une fois) et ne
  bloque jamais l'écriture. Filet de sécurité au démarrage (`RebuildIfDrifted`) : reconstruction complète
  si les comptes de l'index diffèrent du catalogue (écritures SQL brutes d'une migration, sauvegarde
  restaurée…). La migration indexe tout le catalogue ; avant elle, toutes les fonctions sont sans effet.
* **Requête** : termes = lettres / chiffres pliés (≤ 6, 32 caractères ; la syntaxe FTS saisie est
  neutralisée), chacun en **préfixe** (`"ram"*`), tous requis (ET). Un terme de ≥ 4 lettres qui n'est le
  préfixe d'aucun terme indexé est remplacé par ses **corrections** (`Vocab.Corrections`, Damerau-Levenshtein
  1 jusqu'à 6 lettres, 2 au-delà ; plus proche, puis plus long préfixe commun, puis plus fréquent ; ≤ 3) —
  « piza » → `("pizza"* OR "pita"*)`, `fuzzy: true`. Un plat doit correspondre **par lui-même** (nom,
  description ou catégorie) sur au moins un terme : « tomo » ne liste pas toute la carte de Tomo, « tomo
  shoyu » cible le shoyu de Tomo. Une seule lettre : raccourcis seulement.
* **Classement** : `bm25` pondéré (restos : nom 10, cuisines 4, adresse 1 ; plats : nom 8, catégorie 3,
  restaurant 2, description 1) sur 4 × `limit` candidats, puis bonus en Go : nom exact (+100 resto, +50 plat)
  > nom qui commence par la saisie (+40 / +20) > tous les termes en début de mots du nom (+20 / +10) >
  cuisine exacte (+8) > meilleure correction (+6) > plat populaire (+2).
* **Collègues** (connecté uniquement, collection `users`) : utilisateurs qui partagent **au moins une party**
  (`party_members`) **ou une équipe** (`teams` : `owner`, `admins`, `members`, si la collection existe) ;
  jamais soi-même, ni les comptes supprimés / suspendus ; correspondance sur les mots du **nom** (l'e-mail
  n'est jamais cherché). Personne d'autre n'est jamais lu.
* **Performance** (`BenchmarkSearch10k`, `TestSearchLatency10k` : 200 restos, 10 000 plats, Xeon 2,1 GHz) :
  6–10 ms par requête, 20 ms pour un préfixe de 2 lettres très fréquent ; index complet reconstruit en ~1 s.

### `teams` — salon d'équipe permanent (migration `1760000016`, `app/teams.go`)
| champ | type | notes |
|---|---|---|
| name | text req ≤ 80 | ex. « OCC Mons — midi » (espaces normalisés, 2 car. min.) |
| code | text unique | 8 car. `[A-HJ-NP-Z2-9]` — **serveur** ; lien fixe `/e/:code` |
| owner | R(users) req | **serveur** (= créateur) |
| admins | R(users) multi | choisis par le propriétaire seul ; membres non invités uniquement ; le propriétaire n'y figure jamais |
| members | R(users) multi | **serveur** (rejoindre / quitter / retirer passent par `/api/occ/teams/*`) |
| address | text ≤ 300 | adresse du bureau (adresse de livraison par défaut) |
| lat, lng | number | facultatif (bouton « Localiser » : Nominatim depuis le navigateur, comme l'admin restos) |
| usual_time | text `HH:MM` | heure habituelle, **Europe/Brussels** (`12h15`, `1215`… normalisés) |
| usual_days | select multi `mon`…`sun` | jours habituels (ordre lundi → dimanche) |
| default_candidates | R(restaurants) multi ≤ 20 | candidats proposés par défaut (actifs à l'enregistrement) |
| default_split | select `equal` \| `proportional` | partage des frais par défaut (`equal` si vide) |
| emoji | text | |
| color | text `#RRGGBB` | majuscules |
| archived | bool | archivée : plus de membres ni de commande, historique conservé (propriétaire) |
| last_party | R(parties) | **serveur** : dernière commande lancée pour l'équipe |
| last_launch_at | date | **serveur** : sa date (l'update est diffusé en realtime aux membres → toast « Rejoindre ») |

Rules : list/view `members.id ?= @request.auth.id || @request.auth.role = "admin"` ;
create `@request.auth.id != "" && @request.auth.is_guest = false` ; update `owner = @request.auth.id || admins.id ?= @request.auth.id` ;
**delete `nil`** (archiver). Hooks : à la création `owner` = demandeur, `members = [owner]`, `admins = []`, `code` généré ;
en mise à jour `code`, `owner`, `members`, `last_party`, `last_launch_at` refusés (400), `admins` modifiable par le
propriétaire seul (403 sinon, `domain.CheckTeamAdmins`) ; champs normalisés (`domain.NormalizeTeamName`,
`NormalizeUsualTime`, `NormalizeDays`, `NormalizeColor`), candidats actifs.

**Commande d'équipe** (`parties.team`) : créée par `POST /api/occ/teams/{id}/launch` (titre `domain.DailyTitle` à l'heure de
Bruxelles : « Midi du lundi », « Soirée du lundi » dès 16 h ; idempotent : une commande d'équipe ni close ni annulée est
renvoyée telle quelle, le demandeur y est ajouté) ou par la collection avec `team` (feuille « Pour l'équipe … » : le
créateur doit être membre, équipe non archivée). Valeurs par défaut si laissées vides : `delivery_address` = adresse de
l'équipe, `candidates` = candidats par défaut **encore actifs**, `split_mode` = partage de l'équipe. Puis `last_party` /
`last_launch_at` mis à jour (diffusion realtime) et **point d'extension notifications** : `app.SetTeamNotifier(n)` avec
`TeamNotifier.TeamPartyLaunched(app, team, party, recipients)` (destinataires = membres hors hôte, comptes actifs ;
erreurs journalisées) — à brancher sur le service Web Push. Un membre de l'équipe rejoint en un geste, sans code
(`POST /api/occ/parties/{id}/join`) ; `team` est immuable sur la party.

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
| provider | select | `ubereats`, `takeaway`, `deliveroo`, `weloveat`, `manual` |
| delivery_address | text | |
| notes | text | |
| voting_ends_at | date | « Fin du vote » : rappel 2 min avant, clôture automatique à l'heure (voir *Heures limites*) ; éditable par l'hôte |
| ordering_ends_at | date | « Fin de la commande » : idem |
| auto_close_disabled | bool | l'hôte désactive la clôture automatique (les rappels restent) ; migration `1760000015` : `true` pour les parties ouvertes existantes (leurs heures restent indicatives) |
| auto_events | json | **serveur** : journal des actions automatiques `[{ "kind", "at": RFC 3339, "text": "Vote clôturé automatiquement à 11:45 — Pizza Nonna" }]` (20 derniers) ; `kind` : `reminder_vote`, `reminder_order`, `vote_closed`, `vote_extended`, `vote_needs_host`, `ordering_closed`, `ordering_needs_host`, `auto_failed` |
| auto_state | json, **hidden** | **serveur** : mémoire du planificateur (rappels envoyés / prolongation / abandon, indexés par l'heure limite concernée) |
| split_mode | select | `equal` (défaut) ou `proportional` — frais partagés |
| delivery_fee | number int | snapshot du restaurant au passage en `review` (éditable par l'hôte) |
| service_fee | number int | éditable par l'hôte |
| tip | number int | éditable par l'hôte |
| payer | R(users) | celui qui a avancé l'argent |
| dispatch | json | `{ "method": "...", "at": "ISO", "url": "..." }` |
| closed_at | date | |
| team | R(teams) | équipe pour laquelle la commande a été lancée (migration `1760000016`) ; immuable |

Rules :
* list/view : `members.id ?= @request.auth.id || @request.auth.role = "admin"`
  (⚠ PocketBase (v0.36 à v0.40) : sur une relation multiple — dans les rules **comme dans les
  filtres `?filter=` du client** — `members ?= x` compare la
  valeur JSON brute et ne matche jamais → toujours écrire `members.id ?= …`)
* create : `@request.auth.id != "" && @request.auth.is_guest = false` (migration `1760000016`) → le hook force `host`, `members=[host]`, `code`, `status=lobby`.
* update : `host = @request.auth.id` → le hook **refuse** toute modification de
  `code, host, members, status, restaurant, payer, dispatch, closed_at, auto_events`
  (passent par `/api/occ/*` ou le planificateur ; `auto_state` est caché donc ignoré). `candidates` modifiable seulement en `lobby`.
  `delivery_fee`, `service_fee`, `tip`, `split_mode` figés à partir de `paying` ;
  plus aucune modification en `closed` / `cancelled`.
* delete : `host = @request.auth.id && status = "lobby"`.

### `party_members`
`party` R(parties, cascade) · `user` R(users) · `role` select(`host`,`member`) ·
`ready` bool. Index unique `(party, user)`.
Rules : list/view `party.members.id ?= @request.auth.id || @request.auth.role = "admin"` ; écriture serveur uniquement.

### `votes` — vote par approbation (on peut liker plusieurs restaurants)
`party` R(parties, cascade) · `user` R(users) · `restaurant` R(restaurants) · `client_key` text ≤ 64 `[A-Za-z0-9_-]`
(migration `1760000015`). Index uniques `(party, user, restaurant)` et `(user, client_key)` si `client_key` non vide.
**Idempotent** : un vote déjà existant (même `client_key`, ou même restaurant pour ce membre) renvoie **200** avec le
vote existant au lieu d'une erreur d'unicité (rejeu de la file hors ligne).
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
| client_key | text ≤ 64 `[A-Za-z0-9_-]` | clé d'idempotence du client (migration `1760000015`), index unique `(user, client_key)` si non vide ; **création** avec une clé déjà utilisée par ce membre → **200** avec la ligne existante, rien n'est créé ni modifié (même party exigée, sinon 400) ; non modifiable ensuite |

Rules : list/view membres `|| @request.auth.role = "admin"` ; create `user = @request.auth.id && party.members.id ?= @request.auth.id && party.status = "ordering"` ;
update/delete `user = @request.auth.id && party.status = "ordering"`.
Hooks : valide options (min/max, ids), recalcule prix ; toute écriture remet
`party_members.ready = false` pour cet utilisateur.

### `push_subscriptions` — appareils abonnés aux notifications (migration `1760000015`)
| champ | type | notes |
|---|---|---|
| user | R(users) cascade, req | |
| endpoint | text ≤ 1000, **unique** | URL du service push du navigateur (`https://`) |
| p256dh, auth | text | clés du `PushSubscription` (base64 URL ; p256dh = 65 octets, auth = 16–32) |
| user_agent | text ≤ 300 | navigateur (affichage, diagnostic) |
| last_ok | date | dernière remise acceptée (2xx) |
| failures | number int | échecs consécutifs ; 5 → supprimé ; 404 / 410 du service push → supprimé tout de suite |

Rules : list/view/delete `user = @request.auth.id` ; create/update `nil` (uniquement `POST /api/occ/push/subscribe`).
Un même `endpoint` qui s'abonne avec un autre compte (même navigateur, autre connexion) **change de propriétaire**.
Au plus 10 appareils par compte (les plus anciens sont retirés). Les invités peuvent s'abonner (vrais appareils).

### `server_secrets` — secrets générés par le serveur (migration `1760000015`)
`name` text unique · `value` text **hidden**. Toutes les rules `nil` (superusers seulement, jamais exposé).
Ligne `vapid` : `{ "public", "private" }` générés au premier démarrage si `OCC_VAPID_PUBLIC_KEY` /
`OCC_VAPID_PRIVATE_KEY` sont absents (les abonnements survivent ainsi aux redémarrages et aux déploiements :
le volume `pb_data` est persistant). La clé privée ne sort jamais du serveur.

### `payments` — part de chacun
| champ | type | notes |
|---|---|---|
| party | R(parties) cascade | |
| debtor | R(users) | qui doit |
| creditor | R(users) | le payeur |
| amount | number int | cents |
| method | select | `qr` (virement EPC), `revolut`, `paypal`, `link`, `wero`, `bancontact`, `cash`, `later`, `self` |
| status | select | `pending`, `declared`, `confirmed` |
| reference | text | communication, ex. `OCC K7M2QX Alice` |
| declared_at, confirmed_at | date | |

Rules : list/view `party.members.id ?= @request.auth.id || @request.auth.role = "admin"` ; écriture serveur uniquement.

> Les admins **lisent** toutes les parties et leurs enregistrements (support,
> panneau `/admin/commandes`) mais n'écrivent rien via les collections : l'annulation
> forcée passe par `POST /api/occ/admin/parties/{id}/cancel`. Les endpoints métier
> réservés aux membres (`summary`, `export`…) restent réservés aux membres.

### Notifications push (`internal/notify`, `app/push.go`)
* **Transport** : Web Push (RFC 8291, `aes128gcm`) signé VAPID (`github.com/SherClockHolmes/webpush-go` v1.4.0).
  Clés : `OCC_VAPID_PUBLIC_KEY` + `OCC_VAPID_PRIVATE_KEY` (les deux), sinon paire générée une fois et stockée dans
  `server_secrets`. `sub` VAPID = `OCC_VAPID_SUBJECT`, sinon `mailto:OCC_MAIL_FROM`, sinon `mailto:noreply@<hôte de OCC_PUBLIC_URL>`.
* **Envoi asynchrone** : file en mémoire (256) + 4 workers ; une requête n'attend jamais un service push (file pleine →
  notification ignorée et journalisée). Par message : `TTL` (2 h ; rappels 3 min ; arrivées 30 min), `Urgency`
  (`high` pour rappels / « tu dois » / « tout le monde est prêt », `low` pour la clôture), `Topic` = `party-<id>`
  (un message non remis est remplacé par le suivant de la même party). 2xx → `last_ok` ; 404 / 410 → abonnement supprimé ;
  autre échec → `failures + 1` (supprimé à 5). Comptes suspendus ignorés.
* **Charge utile** (lue par le service worker) : `{ kind, title, body, url: "/party/<id>", tag: "party-<id>", partyId,
  renotify, icon: "/icons/icon-192.png", badge: "/icons/badge-72.png", ts }`.
* **Évènements** (hooks *après commit* ; l'auteur de l'action n'est jamais notifié — il est transmis par le contexte
  du `Save`) :

  | évènement | destinataires | préférence | exemple |
  |---|---|---|---|
  | lobby → voting | membres | `party` | « Le vote est ouvert 🗳️ — … avant 11:45 » |
  | → ordering | membres | `party` | « On commande chez Pizza Nonna 🍽️ » (« Vote clôturé automatiquement. » si planificateur) |
  | → review | membres | `party` | « Le récap est prêt 🧾 » |
  | → paying | chaque débiteur / le payeur | `payments` | « Paiement : tu dois 12,40 € » / « C'est toi qui paies 💳 » |
  | → closed / cancelled | membres | `party` | « Commande clôturée ✅ » / « Commande annulée » |
  | tous les membres prêts | hôte (sauf s'il est le dernier) | `party` | « Tout le monde est prêt ✅ » (une fois par phase) |
  | un membre rejoint | hôte | `party` | « Bob a rejoint la commande 👋 » (≤ 1 / 2 min / party, puis « Bob et 2 autres personnes ont rejoint ») |
  | remboursement déclaré | payeur | `payments` | « Bob a déclaré t'avoir remboursé 12,40 € (Wero). Pense à confirmer. » |
  | remboursement confirmé | débiteur | `payments` | « Bob a confirmé ton remboursement de 12,40 €. » |
  | équipe : commande lancée (`TeamNotifier`) | membres de l'équipe sauf l'hôte | `party` | « Compta : la commande du jour est lancée — Rejoins « Midi du lundi ». » |
  | rappels / clôtures automatiques | voir *Heures limites* | `reminders` / `party` | « Plus que 2 minutes pour voter ⏳ » |
  | test | soi | toujours | « Notifications activées 🔔 » |
* **Toasts in-app** : rappels, prolongation, « à toi de décider », clôture sans être prêt, « tout le monde est prêt »,
  remboursements déclarés / confirmés sont aussi envoyés en temps réel sur le sujet personnalisé
  **`occ/notifications`** aux onglets ouverts du destinataire (même charge utile ; le SPA s'y abonne dans le shell).
  Les changements de statut ont déjà leurs toasts (abonnement `parties`).

### Heures limites automatiques (`domain/deadline.go`, `app/deadlines.go`)
Job cron PocketBase `occDeadlines` **chaque minute** : parties `voting` avec `voting_ends_at` et `ordering` avec
`ordering_ends_at`. Pour chacune, en transaction (relecture), `domain.PlanDeadline` (pur, testé) renvoie **une** action :
* **2 min avant** l'heure : rappel push (+ toast) aux membres qui **n'ont pas voté** / **ne sont pas prêts** (ceux qui
  ont un panier : « valide ton panier »), une seule fois par heure limite (même si l'auto-clôture est désactivée).
* **À l'heure** (si `auto_close_disabled = false`) :
  - vote : ≥ 1 vote → `voting → ordering` avec le gagnant (même règle que l'hôte : votes, puis note, puis nom) ;
    0 vote → **prolongé une fois** de 5 min (nouvelle heure = maintenant + 5 min, push à l'hôte, nouveau rappel) ;
    toujours 0 vote → l'hôte est prévenu une fois (« à toi de décider »), la party reste en vote ;
  - commande : ≥ 1 article → `ordering → review` (snapshot des frais) ; les membres **non prêts gardent leur panier tel
    quel** (`ready` reste `false`) et reçoivent « Heure limite atteinte » ; panier vide → l'hôte est prévenu une fois,
    la party reste ouverte ;
  - transition impossible (ex. candidats désactivés) → `auto_close_disabled = true`, évènement `auto_failed`, push à l'hôte.
* Chaque action est notée dans `auto_events` (affiché dans la party) et dans `auto_state` (caché), indexée par l'heure
  limite (`2026-10-09T09:45:00Z`) : un nouveau réglage de l'hôte repart de zéro, un redémarrage ne répète rien, une
  heure dépassée pendant un arrêt est traitée au premier passage (pas de rappel rétroactif). Granularité : la minute
  (clôture au plus ~60 s après l'heure).
* L'hôte règle l'heure par la collection (`voting_ends_at` / `ordering_ends_at` / `auto_close_disabled`, rule update
  hôte) ; l'UI propose +5/+10/+15/+20 min (arrondi à la minute) ou « à HH:MM » (heure de **Bruxelles**).

### Mode hors ligne (service worker `dist/sw.js`, file `src/lib/outbox.ts`)
Service worker écrit à la main (`frontend/pwa/`), assemblé au build par `pwa/plugin.ts` (aucune dépendance PWA) ;
enregistré seulement en production (`/sw.js`, portée `/`). Règles (`pwa/sw-routes.js`, testées) :
* **Coquille** précachée dans `occ-shell-<empreinte>` (`/`, JS, CSS, polices latines, icônes, manifeste) ; à
  l'activation les anciennes coquilles sont supprimées ; `skipWaiting` + `clients.claim`.
* **Navigation** : réseau d'abord (préchargement), repli sur `/` en cache.
* **Catalogue public** (`/api/occ/config`, `/api/occ/restaurants/nearby`, restaurants / catégories / plats) :
  *stale-while-revalidate* (`occ-api-v1`, 120 entrées).
* **Données d'une commande** (`/parties/{id}/summary`, collections `parties`, `party_members`, `votes`,
  `order_items`, `payments`) : **réseau d'abord**, cache seulement hors ligne — un *stale-while-revalidate* renverrait
  une liste périmée juste après une écriture ou un évènement temps réel.
* **Images** (`/api/files/…` sans jeton, images distantes) : cache d'abord, 150 entrées.
* **Jamais en cache** : toute requête non GET, `/api/realtime`, authentification (`/api/collections/users/…`,
  `_…`, OAuth2), `/api/occ/me/*`, `/api/occ/push/*`, `/api/occ/admin/*`, `/api/occ/payments/*`, coordonnées de
  paiement, fichiers protégés, `/_/`. Déconnexion → message `CLEAR_USER_DATA` (cache `occ-api-v1` vidé).
* **Push** : `push` → `showNotification` (icône, badge, `tag` par party, `renotify`) ; `notificationclick` → onglet
  existant focalisé (navigation interne par message `OPEN_URL`) ou nouvelle fenêtre.

**File d'actions hors ligne** (IndexedDB `occ-outbox`, 50 au plus) — uniquement des actions sûres :
« prêt·e » (dernier état seulement), vote / retrait de vote (s'annulent mutuellement), ajout au panier ; chaque action
porte une clé (`client_key`) et le serveur dédoublonne (voir `votes`, `order_items`). Hors ligne ou serveur
injoignable (erreur réseau) → mise en file ; au retour du réseau rejeu **dans l'ordre**, arrêt à la première erreur
réseau, action refusée par le serveur (4xx) abandonnée avec un toast. Les actions non rejouables (transitions,
paiements, heures limites…) sont désactivées hors ligne (« Indisponible hors ligne »). TanStack Query :
`networkMode: 'offlineFirst'` (requêtes), `'always'` (mutations, la file décide).

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
| voting → ordering | gagnant = plus de votes ; égalité → meilleur `rating`, puis nom ; l'hôte peut imposer `restaurant` (doit être candidat) ; **ou automatique** à `voting_ends_at` (≥ 1 vote, voir *Heures limites*) |
| ordering → review | ≥ 1 `order_item` ; snapshot `delivery_fee` depuis le restaurant ; **ou automatique** à `ordering_ends_at` |
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
| `GET /api/occ/config` | — | | `{ "currency":"EUR", "defaultLocation":{lat,lng,label}, "providers":[{id,name,color,enabled}], "minMenuItems": 10, "mailEnabled": true }` (`minMenuItems` = seuil des cartes incomplètes, 0 = aucun ; `mailEnabled` = SMTP configuré : vérification, mot de passe oublié, changement d'adresse disponibles) |
| `GET /api/occ/restaurants/nearby` | — | `lat,lng,radiusKm(=5),q,cuisine` | `{ "items": [Restaurant & { "distanceKm": number }] }` triés par distance ; les positions approximatives (`geo_approx`) après les autres ; restaurants actifs uniquement, **sans les cartes incomplètes** (`items_count < minMenuItems` quand le seuil > 0) |
| `GET /api/occ/search` | — (collègues : ✔) | `q` (≤ 200 car.), `limit` (par groupe, 1–20, défaut 6), `lat`, `lng` (facultatifs, distances) | `SearchResult` (ci-dessous) — restos et plats publics (mêmes règles de visibilité que `/nearby`) ; `people` toujours `[]` sans connexion ; **400** requête trop longue |
| `POST /api/occ/parties/join` | ✔ | `{ "code": "K7M2QX" }` | `{ "party": Party, "alreadyMember": bool }` — idempotent : un **membre** retrouve sa party quel que soit son statut (`alreadyMember: true`, lien `/j/:code` réouvert) ; un nouveau venu seulement en lobby/voting/ordering/review |
| `POST /api/occ/parties/{id}/leave` | membre (≠ hôte) | | `{ "ok": true }` — seulement en lobby/voting/ordering ; supprime ses votes/items |
| `POST /api/occ/parties/{id}/transition` | hôte | `{ "to": Status, "restaurant"?: id }` | `{ "party": Party }` |
| `POST /api/occ/parties/{id}/ready` | membre | `{ "ready": bool }` | `{ "member": PartyMember }` (status `ordering` uniquement) |
| `GET /api/occ/parties/{id}/summary` | membre | | `Summary` (ci-dessous) |
| `POST /api/occ/parties/{id}/dispatch` | hôte | `{ "method": "ubereats"\|"takeaway"\|"deliveroo"\|"weloveat"\|"export"\|"phone" }` (plateforme désactivée par `OCC_PROVIDERS` → 400) | `{ "party": Party, "dispatch": Dispatch }` (status review/paying) |
| `POST /api/occ/parties/{id}/payer` | hôte | `{ "payer": userId }` | `{ "party": Party, "payments": Payment[] }` (review → paying ; en paying : recalcule si aucun paiement tiers confirmé) |
| `GET /api/occ/parties/{id}/export` | membre | `format=csv\|txt\|json` | fichier (`Content-Disposition: attachment`) |
| `GET /api/occ/parties/{id}/reorder` | membre | | `ReorderPreview` (ci-dessous) : ma dernière commande dans le restaurant de la party (`source: null` si aucune) |
| `POST /api/occ/parties/{id}/reorder` | membre | | `{ "added": [{ name, quantity }], "skipped": [{ name, reason }] }` — status `ordering` (sinon 400) ; ajoute à **mon** panier les lignes encore valides de ma dernière commande ici (prix recalculés serveur, `ready` remis à false) ; **404** sans commande précédente |
| `GET /api/occ/me/history` | ✔ | `page` (≥ 1), `perPage` (1–50, défaut 10) | `{ page, perPage, totalItems, totalPages, items: HistoryEntry[] }` — mes parties (membre, tous statuts), plus récentes d'abord |
| `GET /api/occ/me/stats` | ✔ | | `MyStats` (ci-dessous) |
| `GET /api/occ/me/account` | ✔ | | `{ email, verified, passwordSet, providers: [{ id, provider, created }], mailEnabled }` — sécurité du compte (Profil → Sécurité) |
| `DELETE /api/occ/me/providers/{provider}` | ✔ | | `{ "ok": true }` — dissocie Google ; **400** si le compte n'a pas de mot de passe choisi et aucun autre lien ; **404** si non lié |
| `POST /api/occ/me/delete` | ✔ | `{ "confirm": "SUPPRIMER" }` (casse et espaces ignorés) | **204** — anonymise son compte (RGPD, voir §3 *Comptes*) ; **400** sans le mot, si j'héberge une commande ni clôturée ni annulée, si des collègues me doivent encore un remboursement (commande `paying`), ou si je suis le dernier admin actif |
| `GET /api/occ/push/public-key` | — | | `{ "enabled": bool, "publicKey": "<VAPID base64 URL>" }` (`""` si désactivé) |
| `POST /api/occ/push/subscribe` | ✔ | `PushSubscription.toJSON()` : `{ "endpoint": "https://…", "keys": { "p256dh", "auth" } }` | `{ "ok": true, "id" }` — upsert par `endpoint` (rattaché au compte courant) ; **400** endpoint non `https`, clés invalides ou notifications désactivées (`OCC_PUSH_ENABLED=false`) |
| `DELETE /api/occ/push/subscribe` | ✔ | `{ "endpoint" }` | `{ "ok": true, "deleted": 0\|1 }` — seulement ses propres abonnements |
| `POST /api/occ/push/test` | ✔ | | `{ "queued": true, "devices": n }` — notification « Notifications activées 🔔 » à **soi-même** ; **400** « Aucun appareil abonné… » |
| `GET /api/occ/push/prefs` | ✔ | | `{ "prefs": NotifyPrefs, "devices": n, "enabled": bool }` |
| `PATCH /api/occ/push/prefs` | ✔ | `{ "party"?: bool, "payments"?: bool, "reminders"?: bool }` | `{ "prefs": NotifyPrefs }` ; **400** si aucune clé |
| `POST /api/occ/payments/{id}/action` | voir | `{ "action": "declare"\|"confirm"\|"reset", "method"?: "qr"\|"revolut"\|"paypal"\|"link"\|"wero"\|"bancontact"\|"cash"\|"later" }` | `{ "payment": Payment }` |
| `GET /api/occ/payments/{id}/qr` | membre | | `PaymentQR` |
| `GET /api/occ/payments/{id}/wallet-qr/{kind}` | membre | `kind=wero\|bancontact` | image QR du payeur (stream, `Cache-Control: private`) ou 404 |
| `POST /api/occ/guest` | — (sans session) | `{ "name": "Léa", "color"?: "#RRGGBB", "partyCode"?: "K7M2QX", "teamCode"?: "K7M2QXAB" }` (un seul code) | `{ token, record, party: {id,title}\|null, team: {id,name}\|null }` — crée un·e invité·e et le fait rejoindre (§3 *Invités*) ; **400** code absent / mal formé / commande fermée / équipe archivée / prénom vide, **404** code inconnu, **429** limite par IP |
| `GET /api/occ/invites/{code}` | — | | aperçu public d'un lien : commande (6 car.) `{ kind: "party", code, title, host, status, memberCount, joinable }` ou équipe (8 car.) `{ kind: "team", code, title, emoji, color, memberCount, joinable }` ; 400 / 404 ; 429 (60/min/IP) |
| `POST /api/occ/me/upgrade` | invité·e | `{ email, password, passwordConfirm?, name? }` | `{ token, record }` — « Créer mon compte » sur le même enregistrement (historique conservé) ; 400 si déjà un compte complet, e-mail invalide / pris, mot de passe < 8 |
| `GET /api/occ/me/teams` | ✔ | | `{ items: TeamView[] }` — mes équipes (non archivées d'abord, puis nom) ; 6 membres max par équipe, sans candidats |
| `POST /api/occ/teams/join` | ✔ | `{ "code": "K7M2QXAB" }` | `{ team: TeamView, alreadyMember }` — idempotent ; 400 lien mal formé / équipe archivée, 404 inconnu |
| `GET /api/occ/teams/{id}` | membre de l'équipe | | `{ team: TeamView }` (ci-dessous) ; 403 non-membre, 404 |
| `GET /api/occ/teams/{id}/parties` | membre de l'équipe | `page`, `perPage` (1–50) | `{ page, perPage, totalItems, totalPages, items: [{ party: HistoryEntry, isMember }] }` — historique de l'équipe, plus récent d'abord (mes totaux, même code que `/me/history`) |
| `POST /api/occ/teams/{id}/launch` | membre (pas invité·e) | `{ "title"?: string }` (≤ 120) | `{ party: Party, created: bool }` — « Lancer la commande du jour » (voir `teams`) ; 400 équipe archivée ; 403 invité·e / non-membre |
| `POST /api/occ/teams/{id}/leave` | membre (≠ propriétaire) | | `{ ok: true }` (perd aussi ses droits d'admin) |
| `DELETE /api/occ/teams/{id}/members/{userId}` | propriétaire / admin | | `{ team: TeamView }` — propriétaire : tout le monde sauf lui ; admin : les simples membres (`domain.CheckRemoveMember`, 400 sinon) |
| `POST /api/occ/teams/{id}/code` | propriétaire / admin | | `{ code }` — nouveau lien `/e/:code` (l'ancien ne fonctionne plus) |
| `POST /api/occ/parties/{id}/join` | membre de l'équipe de la party | | `{ party, alreadyMember }` — rejoindre **en un geste, sans code** ; 403 party sans équipe ou non-membre de l'équipe ; 400 statut fermé |
| `GET /api/occ/parties/{id}/team` | membre de la party | | `{ team: { id, name, emoji, color, isMember } \| null, missing: TeamMember[] }` — « Membres de l'équipe pas encore là » (comptes actifs) |
| `GET /api/occ/admin/stats` | admin | | `AdminStats` (ci-dessous) |
| `POST /api/occ/admin/import` | admin | `RestaurantImport` **ou** `RestaurantImport[]` ; `?dryRun=1` | `{ "report": ImportReport, "restaurant": id, "items": n }` — upsert par slug, tout ou rien ; invalide → **400** `{ status, message, data, report }` (rien n'est écrit) ; en dry run → 200 avec `report.valid=false` |
| `POST /api/occ/admin/import/csv` | admin | multipart `file` (CSV, voir ci-dessous) ; `?dryRun=1` | idem |
| `GET /api/occ/admin/export` | admin | | `RestaurantImport[]` (tous les restaurants, actifs ou non, menus complets ; `Content-Disposition: attachment`) — réimportable tel quel |
| `GET /api/occ/admin/users` | admin | `q` (nom/e-mail, `%` `_` littéraux), `role` (`user`\|`admin`), `status` (`banned`\|`unverified`\|`deleted`\|`guest`), `page`, `perPage` (≤ 200) | `{ page, perPage, totalItems, items: AdminUser[] }`, plus récents d'abord (`AdminUser` ci-dessous) |
| `PATCH /api/occ/admin/users/{id}/role` | admin | `{ "role": "user"\|"admin" }` | `{ "user": AdminUser }` (400 si on se retire ses propres droits, si c'est le dernier admin actif, ou si le compte est supprimé) |
| `POST /api/occ/admin/users/{id}/ban` | admin | `{ "reason"?: string }` (≤ 300 car.) | `{ "user": AdminUser }` — suspend et déconnecte partout ; **400** soi-même, dernier admin actif, déjà suspendu, compte supprimé, motif trop long |
| `POST /api/occ/admin/users/{id}/unban` | admin | | `{ "user": AdminUser }` — **400** si non suspendu ou supprimé |
| `POST /api/occ/admin/users/{id}/logout` | admin | | `{ "user": AdminUser }` — « Forcer la déconnexion » : `tokenKey` renouvelé (tous les appareils) |
| `POST /api/occ/admin/users/{id}/password-reset` | admin | | `{ "sent": true, "email": "…" }` — e-mail de réinitialisation envoyé à l'utilisateur (aucun mot de passe visible) ; **400** si SMTP non configuré (« L'envoi d'e-mails n'est pas configuré… ») ou envoi en échec |
| `DELETE /api/occ/admin/users/{id}` | admin | | `{ "user": AdminUser }` — anonymisation (historique conservé) ; **400** soi-même (« depuis ton profil »), dernier admin actif, déjà supprimé |
| `GET /api/occ/admin/mail` | admin | | `{ enabled, host, port, tls, senderAddress, senderName, fromEnv }` (jamais d'identifiant ni de mot de passe) |
| `POST /api/occ/admin/mail/test` | admin | | `{ "sent": true, "to": "<e-mail du demandeur>" }` — e-mail de test à l'admin (ou superuser) connecté ; **400** sans SMTP ou en cas d'échec (message de l'erreur SMTP) |
| `POST /api/occ/admin/parties/{id}/cancel` | admin | | `{ "party": Party }` — annulation forcée (400 si `closed`/`cancelled`) |
| `GET /api/occ/admin/sync/status` | admin | | `{ enabled, cron, timezone: "Europe/Brussels", running: SyncRun\|null (avec logTail), lastRun: SyncRun\|null, nextRunAt: ISO\|null }` |
| `GET /api/occ/admin/sync/runs` | admin | `page`, `perPage` (≤ 100, défaut 20) | `{ page, perPage, totalItems, items: SyncRun[] }` (sans `changes` ni `log`, avec `changesCount`), plus récent d'abord |
| `GET /api/occ/admin/sync/runs/{id}` | admin | | `{ "run": SyncRun }` complet (`changes`, `log`) ; 404 sinon |
| `GET /api/occ/admin/settings` | admin | | `AdminSettings` |
| `PATCH /api/occ/admin/settings` | admin | `{ "minMenuItems": 0–100 }` (entier ; 0 = tout afficher) | `AdminSettings` à jour ; 400 si absent, décimal ou hors bornes |
| `POST /api/occ/admin/sync/run` | admin | facultatif : `{ "sourceId": id }` | **202** `{ "run": SyncRun }` (status `running`, exécution en arrière-plan) ; sans corps : toutes les sources activées ; avec `sourceId` : **cette source seule** (même désactivée, exécution « ciblée », voir §3) ; **404** source inconnue ; **409** si une exécution tourne (ciblée ou non) ; 400 si `OCC_SYNC_ENABLED=false` |
| `POST /api/occ/admin/sync/discover` | admin | `{ "query": string }` (≤ 300 car.) : lien takeaway.com / just-eat (`…/menu/<slug>`), nom libre, ou adresse directe d'un site | `SyncDiscoverResult` (ci-dessous) — 10 à 20 s ; **400** requête vide / illisible / lien Takeaway sans `/menu/` ou `OCC_SYNC_ENABLED=false` ; **409** si une découverte tourne déjà (une à la fois) |
| `POST /api/occ/admin/sync/sources` | admin | `{ "url": "https://www.site.be/", "label"?: string }` | **201** `{ "source": SyncSource, "created": true }` ; **200** `{ source, created: false }` si une source `takeaway-site` / `jsonld` lit déjà ce site (même hôte sans `www.`, même chemin : idempotent) ; 400 URL invalide ou takeaway.com. Crée `takeaway-site`, activée, ville `mons`, priorité = première libre entre 10 et 39 (39 si tout est pris), libellé = `label` ou l'hôte |

« admin » = utilisateur `role = "admin"` **ou** superuser (401 sans auth, 403 sinon).
« admin actif » = `role = "admin"`, ni suspendu ni supprimé : on ne peut jamais suspendre, rétrograder ni
supprimer le **dernier** (même un superuser), ni agir sur son propre compte depuis l'administration
(`domain.CheckModeration`, pur et testé).

Endpoints PocketBase utilisés tels quels par le front (modèles d'e-mails ci-dessus) :
`request-verification` / `confirm-verification`, `request-password-reset` / `confirm-password-reset`,
`request-email-change` / `confirm-email-change` (mot de passe actuel requis ; toutes les sessions sont
fermées), `auth-with-oauth2`, `auth-methods`, update `users` avec `oldPassword` (changement de mot de passe).

### `AdminUser`
```json
{ "id": "…", "name": "Bob", "email": "bob@occ.be", "role": "user", "color": "#…", "avatar": "",
  "verified": true, "created": "2026-10-01 10:00:00.000Z", "parties": 3,
  "banned": true, "bannedReason": "spam", "bannedAt": "2026-10-09 08:00:00.000Z",
  "deleted": false, "deletedAt": "", "passwordSet": true, "providers": ["google"],
  "lastLoginAt": "2026-10-08 10:00:00.000Z", "isGuest": false }
```
`lastLoginAt` = dernière origine de connexion connue (`_authOrigins`, mise à jour à chaque connexion par mot
de passe / Google ; `""` si aucune).

Règles `payments/{id}/action` :
* `declare` — débiteur seulement. `method=qr|revolut|paypal|link|wero|bancontact|cash` → `status=declared` ; `method=later` → `status=pending`.
* `confirm` — créancier (payeur) ou hôte → `status=confirmed`. Si tous confirmés → party `closed`.
* `reset` — créancier ou hôte → `status=pending`.

### `TeamView` (`/teams/{id}`, `/me/teams`)
```json
{ "id": "…", "name": "OCC Mons — midi", "code": "K7M2QXAB", "emoji": "🍕", "color": "#FF6A3D",
  "address": "Rue de Nimy 7, 7000 Mons", "lat": 50.45, "lng": 3.95, "usualTime": "12:15",
  "usualDays": ["mon","tue","wed","thu","fri"], "defaultSplit": "equal", "archived": false,
  "defaultCandidates": [{ "id": "…", "name": "Pizza Nonna", "emoji": "🍕", "cover": "", "cover_url": "", "active": true }],
  "created": "…", "myRole": "owner", "memberCount": 3,
  "members": [{ "id": "…", "name": "Bob", "avatar": "", "color": "#…", "isGuest": false, "role": "owner" }],
  "activeParty": { "id": "…", "code": "K7M2QX", "title": "Midi du lundi", "status": "voting", "created": "…",
                   "host": { "id": "…", "name": "Bob", "avatar": "", "color": "#…" }, "memberCount": 2, "isMember": false,
                   "restaurant": null } }
```
`members` : propriétaire d'abord puis ordre d'arrivée, comptes supprimés exclus (`TeamMember` = `UserInfo` + `isGuest`,
`role` `owner`\|`admin`\|`member`) ; `myRole` du demandeur ; `activeParty` = commande d'équipe la plus récente ni close ni
annulée (`null` sinon) ; `defaultCandidates` vide dans `/me/teams`.

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

### `HistoryEntry` (`/me/history`)
```json
{ "id": "…", "code": "K7M2QX", "title": "Midi du vendredi", "status": "closed",
  "created": "2026-10-09 10:02:11.000Z", "closedAt": "2026-10-09 13:40:00.000Z",
  "restaurant": { "id": "…", "name": "Pizza Nonna", "emoji": "🍕", "cover": "", "cover_url": "", "active": true },
  "provider": "ubereats", "host": { "id": "…", "name": "Bob", "avatar": "", "color": "#…" }, "isHost": false,
  "memberCount": 3,
  "items": [{ "menuItem": "…", "name": "Margherita", "optionsLabel": "Large", "note": "", "quantity": 2, "unitPrice": 1450, "total": 2900 }],
  "subtotal": 2900, "sharedFees": 133, "total": 3033, "grandTotal": 6099,
  "payer": { "id": "…", "name": "Bob", "avatar": "", "color": "#…" },
  "payment": { "id": "…", "method": "wero", "status": "confirmed", "amount": 3033 } }
```
`items` = **mes** lignes seulement ; `subtotal` / `sharedFees` / `total` / `grandTotal` = exactement ceux du
`Summary` de la party (même code, `summaryFromRecords`) ; `restaurant` `null` tant qu'aucun n'est retenu ;
`payer` / `payment` (mon remboursement, débiteur) `null` avant `paying` ; `payment.method = "self"` si j'ai
avancé l'argent. Chargement par lots (party_members → parties, membres, articles, utilisateurs, restaurants,
mes paiements : 8 requêtes par page, comptage compris, quelle que soit sa taille — aucun N+1).

### `MyStats` (`/me/stats`)
```json
{ "orders": 12, "totalSpent": 18450,
  "favoriteRestaurant": { "id": "…", "name": "Pizza Nonna", "emoji": "🍕", "orders": 5 },
  "favoriteDish": { "name": "Margherita", "quantity": 7, "orders": 5 } }
```
Commande comptée (`domain.ComputeHistoryStats`, pur et testé) = party `review` / `paying` / `closed` où j'ai
au moins un article ; `totalSpent` = Σ de mes parts (`total`). Favoris : plus de commandes (resto) / plus
grande quantité cumulée (plat, nom insensible à la casse), puis le plus récent, puis le nom ; `null` si aucun.

### `SearchResult` (`/search`)
```json
{ "query": "piza", "terms": ["pizza", "pita"], "fuzzy": true,
  "restaurants": [{ "id": "…", "name": "La Pizza", "emoji": "🍕", "cuisines": ["pizza", "italien"],
                    "itemsCount": 12, "distanceKm": 0.4 }],
  "dishes": [{ "id": "…", "name": "Pizza truffe", "price": 1600, "emoji": "🍕",
               "snippet": "…crème de truffe, mozzarella…", "restaurant": { "id": "…", "name": "La Pizza", "emoji": "🍕" } }],
  "people": [{ "id": "…", "name": "Bob Martin", "avatar": "", "color": "#…", "sharedParties": 3,
               "recentParties": [{ "id": "…", "code": "K7M2QX", "title": "Midi du vendredi", "status": "closed",
                                   "created": "2026-10-09 10:02:11.000Z" }] }],
  "actions": [{ "id": "new-party", "label": "Lancer une commande", "href": "/?lancer=1" }] }
```
`terms` = termes pliés réellement cherchés (corrections comprises), pour surligner côté client ; `snippet` =
extrait **texte brut** de la description (accents d'origine, ≤ 90 car., « … ») autour du premier terme trouvé
(début de la description sinon) ; `distanceKm` absent sans `lat`/`lng` ou si `geo_approx` ;
`itemsCount` = plats disponibles ; `sharedParties` = parties en commun (0 = collègue d'équipe seulement),
`recentParties` = 3 dernières. `actions` (raccourcis statiques filtrés par les termes ; tous si `q` est vide) :
`new-party`, `restaurants`, `my-orders` (connecté), `admin` (admin).

### `ReorderPreview` (`/parties/{id}/reorder`)
```json
{ "source": { "partyId": "…", "title": "Midi du lundi", "created": "2026-10-05 10:00:00.000Z" },
  "items": [{ "menuItem": "…", "name": "Margherita", "optionsLabel": "Large", "note": "bien cuite", "quantity": 2,
              "unitPrice": 1400, "available": true },
            { "menuItem": "…", "name": "Tiramisu", "optionsLabel": "", "note": "", "quantity": 1,
              "unitPrice": 0, "available": false, "reason": "n'est plus disponible" }] }
```
Source = ma party la plus récente (≠ celle-ci, non annulée) au **même restaurant** où j'ai des articles.
Chaque ligne est revalidée contre la carte actuelle : plat absent / d'un autre restaurant (« n'est plus à la
carte »), indisponible (« n'est plus disponible »), options refusées par `domain.PriceOptions` (« ses options
ont changé ») ; `unitPrice` = prix **actuel** recalculé (le client n'envoie jamais de prix). `POST` crée les
lignes valides en une transaction (snapshots `name` / `options_label` / `unit_price` / `total` serveur).

### `Dispatch`
```json
{ "method": "ubereats", "url": "https://www.ubereats.com/…", "cartText": "…",
  "instructions": ["Ouvrez le restaurant…", "Ajoutez les articles…"] }
```
`cartText` = récap consolidé lisible (copiable / à dicter au téléphone).

### `PaymentQR`
```json
{ "amount": 1240, "reference": "OCC K7M2QX Alice", "beneficiary": "Bob Martin",
  "epc": "BCD\n002\n1\nSCT\n\nBob Martin\nBE71096123456769\nEUR12.40\n\n\nOCC K7M2QX Alice",
  "iban": "BE71096123456769",
  "links": [
    { "kind": "revolut", "label": "Revolut", "amountPrefilled": true,
      "url": "https://revolut.me/bobm?amount=1240&currency=EUR&note=OCC+K7M2QX+Alice" },
    { "kind": "paypal", "label": "PayPal", "amountPrefilled": true,
      "url": "https://paypal.me/bobm/12.40EUR" },
    { "kind": "link", "label": "lydia-app.com", "amountPrefilled": false,
      "url": "https://lydia-app.com/collect/…" } ],
  "wero": { "id": "+32470123456", "hasQr": true },
  "bancontact": { "phone": "+32470123456", "hasQr": false },
  "methods": ["qr", "revolut", "paypal", "link", "wero", "bancontact", "cash", "later"] }
```
`epc` = payload **EPC069-12** (QR virement SEPA, lu par les apps bancaires
belges/européennes : montant **et** communication pré-remplis — le seul QR « à
montant » universel), `null` si le payeur n'a pas d'IBAN ; `iban` l'accompagne
(copie manuelle). `links` = liens du payeur construits **pour ce paiement**
(`[]` si aucun), ordre Revolut, PayPal, lien libre ; `amountPrefilled` vaut
`true` seulement si le format du fournisseur est confirmé (ADR 0003) :
Revolut `?amount=<centimes>&currency=EUR&note=<communication>`, PayPal.me
`/<montant>EUR`, lien libre Wise Business `?amount=&currency=&description=` ;
tout autre lien est renvoyé tel quel (`false`). `wero` / `bancontact` :
`null` si le payeur n'a rien renseigné ; `hasQr` indique qu'une image est
disponible via `/wallet-qr/{kind}`. `methods` = moyens réellement proposés
(selon le profil du payeur), par ordre d'utilité : `qr`, `revolut`, `paypal`,
`link`, `wero`, `bancontact`, `cash`, `later` (le front avance les liens
pré-remplis devant `qr` sur mobile).

**Wero / Bancontact Pay** n'exposent aucun lien ni QR de demande de paiement
P2P utilisable par un tiers (demandes créées dans l'app du bénéficiaire ;
API réservées aux commerçants). L'UI affiche donc : identifiant du payeur,
montant et communication à copier, mini-guide, et le QR personnel du payeur
s'il l'a téléversé, **toujours** avec l'avertissement « QR sans montant :
saisis 12,40 € dans l'app ». Voir ADR 0003.

### `RestaurantImport`
```json
{ "slug": "…", "name": "…", "...champs restaurant": "…", "active": true,
  "categories": [{ "name": "Pizzas", "items": [{ "name": "…", "price": 1250, "tags": [], "popular": false,
                                                 "available": true, "option_groups": [] }] }] }
```
Clés = champs de `restaurants` (sauf `cover`) ; montants en centimes ; `partial_menu` et
`geo_approx` facultatifs (`false` par défaut : importer une carte complète met fin à l'aperçu ;
sans coordonnées, `geo_approx` existant conservé avec la position). Les clés
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

### `SyncRun`
```json
{ "id": "…", "started_at": "2026-10-09 01:30:00.000Z", "finished_at": "2026-10-09 01:38:12.000Z",
  "status": "partial", "trigger": "cron",
  "stats": { "restaurants_created": 0, "restaurants_updated": 12, "restaurants_stale": 1, "items_created": 4,
             "items_updated": 31, "items_price_changed": 9, "items_unavailable": 6 },
  "sources": [{ "id": "…", "label": "Deliveroo — Mons", "provider": "deliveroo", "url": "https://…",
                "status": "blocked", "message": "accès refusé par … (HTTP 403)", "restaurants": 0,
                "network": 2, "cached": 0, "durationMs": 3100 }],
  "changesCount": 50, "changes": ["Tomo — Miso ramen : 14,50 € → 15,00 €"], "log": "…", "error": "" }
```
Les sources (`sync_sources`) se gèrent par la collection (rules admin) ; `POST /admin/sync/sources`
ajoute un site Takeaway trouvé par « Découvrir » (mêmes validations que le hook de la collection).

### `SyncDiscoverResult`
```json
{ "query": "https://www.takeaway.com/be-fr/menu/snack-a-la-gare", "kind": "takeaway", "slug": "snack-a-la-gare",
  "tried": [{ "host": "www.snack-a-la-gare.be", "status": "found", "message": "Snack à la Gare : 196 plats" },
            { "host": "snack-a-la-gare.be", "status": "skipped", "message": "même site que la variante déjà trouvée" },
            { "host": "www.snackalagare.be", "status": "absent", "message": "nom de domaine inexistant" }],
  "found": [{ "url": "https://www.snack-a-la-gare.be/", "host": "www.snack-a-la-gare.be", "name": "Snack à la Gare",
              "address": "7 Rue Léopold II, 7000 Mons", "items": 196, "categories": 13,
              "takeawayUrl": "https://www.takeaway.com/be/menu/snack-a-la-gare", "lat": 50.45, "lng": 3.94,
              "distanceKm": 0.9, "alreadySource": false }],
  "network": 4, "durationMs": 6667, "interrupted": false }
```
`kind` : `takeaway` (slug lu dans `/menu/<slug>` ; `/be/`, `/be-fr/`, `/be-nl/`, just-eat, thuisbezorgd…), `name`
(nom libre, slugifié) ou `site` (adresse directe : seule cette page est vérifiée). `tried[].status` :
`found`, `absent` (DNS : aucune requête), `unreachable`, `http`, `robots` (interdit : page non demandée),
`blocked` (403 / page anti-robot : hôte abandonné), `other` (site sans le modèle Takeaway), `invalid`
(modèle reconnu, carte illisible), `skipped` (variante `www.` / nue d'un domaine déjà vérifié).
`distanceKm` (vs `OCC_DEFAULT_LAT/LNG`) seulement si la page publie ses coordonnées (pas de géocodage).
`alreadySource` / `sourceId` : une source lit déjà ce site.

**Découverte (`menusync.PlanDiscovery` + `menusync.Prober`).** Candidats purs et testés
(`DiscoverHosts`) : le slug tel quel, sans tirets, puis avec la ville (`-mons`, `mons`, `…-mons` doublé
pour les slugs qui portent déjà la ville), en `.be` puis `.com`, et en dernier les formes sans articles
ni mots génériques (`le/la/les/l'`, `snack`, `restaurant`, `pizzeria`…) ; ≤ 16 domaines, chacun essayé
en `www.` puis nu. **takeaway.com n'est jamais demandé** (défi Cloudflare) et une redirection vers la
plateforme n'est jamais suivie. Politesse : User-Agent identifié, robots.txt avant toute page, une
requête à la fois et ≥ 1 s entre deux, délai 6 s, aucun nouvel essai, 403 / anti-robot = hôte abandonné.
Modèle reconnu (`LooksLikeTakeawaySite`) : classes `menucat` + `menucard__meals-group` / `meal-wrapper`,
microdonnées `itemprop` name/price, référence Takeaway ; la carte est lue par `ParseTakeawaySite`.

**Exécution ciblée** (`sourceId`). Seule cette source est lue ; la réconciliation est « ciblée »
(`feedsync.Options.Scoped`) : aucun restaurant n'est marqué obsolète, la provenance (`sources`) d'un
restaurant reconnu est complétée (jamais remplacée), et le menu n'est appliqué que si aucune source
déjà liée au restaurant n'est prioritaire (priorité calculée sur toutes les sources activées).

### `AdminSettings`
```json
{ "minMenuItems": 10, "hiddenRestaurants": 24, "activeRestaurants": 61 }
```
`hiddenRestaurants` = restaurants **actifs** masqués des listes publiques par le seuil (0 si désactivé).

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

Ni Uber Eats, ni Takeaway (Just Eat Takeaway), ni Deliveroo, ni weloveat n'offrent
d'API publique permettant à un tiers de **créer un panier client**. Stratégie :

* `Provider` interface : `ID()`, `Name()`, `Color()`, `Dispatch(restaurant, summary) Dispatch`.
* **Uber Eats** : deep link vers la page du restaurant (`providers[].url`) + guide
  pour lancer une *commande groupée Uber Eats* + récap copiable.
* **Takeaway** : deep link `takeaway.com` + récap copiable.
* **Deliveroo** (`#00CCBC`) : deep link vers la page du restaurant (`deliveroo.be/fr/menu/…`,
  repli `deliveroo.be/fr/`) + guide (adresse, panier ou « commande de groupe ») + récap.
* **weloveat** (`#113B3A`, plateforme belge, SPA Angular) : deep link `weloveat.be/{slug}`
  (repli `weloveat.be/restaurants`) + guide + récap.
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
Les plateformes de `restaurants.providers` sont celles de `providers.Platforms`.

### Instantané Uber Eats (`backend/migrations/data/mons_ubereats.json`)
Le connecteur officiel Uber Eats n'est joignable que depuis une session Claude (fortement limité,
au plus 5 plats d'exemple par restaurant, jamais la carte complète) : le serveur ne peut pas
l'appeler. Le lead relève les restaurants qui livrent le bureau et les commite dans ce fichier
(embarqué, `go:embed`, `[]` = vide) ; la source `ubereats-snapshot` le relit à chaque
synchronisation (aucune requête). Format (commentaires pour la doc, le fichier est du JSON strict) :
```jsonc
[{ "name": "CTR Chicken Mons (Independant)",
   "url": "https://www.ubereats.com/be/store/ctr-chicken-mons/…",  // sans query string (retirée sinon)
   "rating": 4.6, "rating_count": 320, "eta_min": 25,              // 0 = inconnu
   "categories": ["Poulet", "Burgers"], "promo": "-20 %",          // promo : ignorée (éphémère)
   "lat": 50.4551, "lng": 3.9512, "address": "…", "geo_approx": false, // 0 = inconnu → lieu par défaut
   "items": [{ "name": "Tenders x6", "price": 890 }],              // centimes, 0–5 plats d'exemple
   "checked_at": "2026-10-09" }]
```
Entrées sans nom, sans lien `https://…ubereats.com/…` ou en double (même URL) ignorées (journal) ;
plats sans nom, à 0 € ou en double retirés ; note bornée à 0–5. Un test vérifie que le fichier commité
est lisible. Mise à jour : `docs/DEPLOYMENT.md`.

**Flux de menus (`menusync`)** — bibliothèque utilisée par la synchronisation automatique
du serveur (§3 *Synchronisation automatique*) et par l'outil en ligne de commande : il lit
les pages publiques (Deliveroo : `__NEXT_DATA__` des pages liste et menu ; weloveat :
l'API JSON anonyme qu'appelle la SPA ; sites satellites Takeaway : microdonnées
schema.org ; sites quelconques : JSON-LD/microdonnées), produit des `RestaurantImport`
(+ `source`, `source_urls`, `menu_checked_at`, ignorés par l'import), fusionne les
sources (`menusync merge`) et peut importer via l'endpoint admin. Politesse obligatoire :
User-Agent identifié, requêtes séquentielles ≥ 2,5 s, robots.txt appliqué, cache disque,
arrêt net sur 403 / page anti-robot (aucun contournement). Normalisation : noms (ALL-CAPS →
casse française, sigles courts conservés, suffixes de ville / zone retirés), cuisines (minuscules,
synonymes et pluriels unifiés, tags génériques retirés s'il y en a d'autres, 4 au plus), plats
(espaces, 0 € sans options retirés, doublons d'une catégorie retirés). Doublons entre sources :
même lien plateforme ou page source, même nom normalisé ≤ 1,5 km, nom proche ≤ 300 m, ou nom
proche à la même adresse. Voir `docs/DEPLOYMENT.md`.

## 7. Configuration (variables d'environnement)

| variable | défaut | rôle |
|---|---|---|
| `OCC_ADMIN_EMAIL` / `OCC_ADMIN_PASSWORD` | — | crée/met à jour le superuser au démarrage ; le compte **utilisateur** de même e-mail reçoit le rôle `admin` |
| `OCC_ADMINS` | — | e-mails (séparés par des virgules) promus `admin` au démarrage ou à l'inscription |
| `OCC_PUBLIC_URL` | `http://localhost:8090` | URL publique (liens d'invitation, `meta.appURL`) |
| `OCC_DEFAULT_LAT` / `OCC_DEFAULT_LNG` / `OCC_DEFAULT_LABEL` | `50.4542` / `3.9567` / `Mons` | position par défaut (aussi celle des restaurants Uber Eats sans coordonnées, `geo_approx`) |
| `OCC_SEED_DEMO` | `true` | charge les restaurants de démo au 1er démarrage (ignoré si `migrations/data` contient des restaurants réels) |
| `OCC_PROVIDERS` | `ubereats,takeaway,deliveroo,weloveat` | plateformes de livraison activées (config + dispatch) |
| `OCC_PUBLIC_DIR` | `./pb_public` | dossier de la SPA |
| `OCC_SYNC_ENABLED` | `true` | synchronisation automatique des restaurants / menus ; `false` = aucune requête sortante, déclenchement manuel refusé |
| `OCC_SYNC_CRON` | `30 3 * * *` | planification (cron 5 champs) lue à l'heure de **Europe/Brussels** |
| `OCC_SYNC_ON_START` | `true` | lance une synchronisation après le démarrage si aucune n'a encore réussi |
| `OCC_SYNC_START_DELAY` | `60s` | délai de cette première synchronisation (durée Go) |
| `OCC_ENRICH_ENABLED` | `true` | complète téléphone / adresse / position manquants depuis OpenStreetMap (Nominatim) à la fin de chaque synchronisation ; `false` = aucune requête vers Nominatim (sans effet si `OCC_SYNC_ENABLED=false`) |
| `OCC_GOOGLE_CLIENT_ID` / `OCC_GOOGLE_CLIENT_SECRET` | — | client OAuth « Application Web » Google : active « Continuer avec Google » (les deux requis ; absents = configuration `/_/` inchangée) |
| `OCC_SMTP_HOST` | — | serveur SMTP ; défini = e-mails activés (sinon réglages `/_/` inchangés) |
| `OCC_SMTP_PORT` | `587` | `465` = TLS implicite par défaut |
| `OCC_SMTP_USERNAME` / `OCC_SMTP_PASSWORD` | — | identifiants SMTP (jamais journalisés ; stockés dans les réglages PocketBase de `pb_data`) |
| `OCC_SMTP_TLS` | `true` si port 465, sinon `false` | `true` = TLS implicite ; `false` = STARTTLS si le serveur le propose |
| `OCC_MAIL_FROM` | `OCC_SMTP_USERNAME` s'il contient `@` | expéditeur (ex. `noreply@fs0ciety.org`) |
| `OCC_MAIL_FROM_NAME` | `OCC Deliveries` | nom d'expéditeur |
| `OCC_PUSH_ENABLED` | `true` | notifications Web Push (`false` = aucune notification, abonnement refusé) |
| `OCC_VAPID_PUBLIC_KEY` / `OCC_VAPID_PRIVATE_KEY` | — | paire VAPID (`go run ./cmd/vapid`) ; absentes = paire générée une fois et gardée dans `server_secrets` (`pb_data`) ; **changer de clés invalide tous les abonnements** |
| `OCC_VAPID_SUBJECT` | `mailto:OCC_MAIL_FROM` | contact de l'expéditeur des notifications (`mailto:` ou `https://`) |
