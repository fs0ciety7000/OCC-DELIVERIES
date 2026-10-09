# Déploiement — Coolify (`eat.fs0ciety.org`)

> **État actuel (2026-10-09)** — en production sur `https://eat.fs0ciety.org`.
> Coolify `coolify.fs0ciety.org` · projet *Main Stack* / `production` · app
> « OCC Deliveries » (uuid `ichbnb7y7rucoiqfcs0xlhfd`) · source GitHub App
> privée · branche **`main`** · auto-deploy activé (chaque push sur `main`
> redéploie) · volume `/pb/pb_data` · healthcheck `/api/occ/health`.
> Pilotable via l'API Coolify (`COOLIFY_API_URL` / `COOLIFY_API_TOKEN` dans
> l'environnement cloud ; `POST /api/v1/deploy {"uuid": …}` pour redéployer).

Un seul conteneur : le binaire Go sert l'API, le realtime, l'admin et la SPA.
Données : SQLite dans le volume `/pb/pb_data` (**à sauvegarder**).

## 1. DNS
Enregistrement `A` (ou `CNAME`) `eat.fs0ciety.org` → IP du serveur Coolify.

## 2. Créer l'application
1. Coolify → *Projects* → *New resource* → *Public/Private repository* →
   `fs0ciety7000/occ-deliveries`, branche `main`.
2. **Build pack : `Dockerfile`** (recommandé) — Dockerfile à la racine.
   *Ports Exposes* : `8090`.
3. *Domains* : `https://eat.fs0ciety.org` (Coolify génère le certificat Let's Encrypt).
4. *Persistent storage* → ajouter un volume : destination **`/pb/pb_data`**.
5. *Health check* : chemin `/api/occ/health`, port `8090`.
6. *Environment variables* (cocher *Build variable* uniquement pour `VERSION` si souhaité) :

| variable | exemple |
|---|---|
| `OCC_PUBLIC_URL` | `https://eat.fs0ciety.org` |
| `OCC_ADMIN_EMAIL` | `admin@fs0ciety.org` |
| `OCC_ADMIN_PASSWORD` | *(secret fort, ≥ 10 car.)* |
| `OCC_ADMINS` | `alice@fs0ciety.org,bob@fs0ciety.org` *(facultatif : admins du panneau `/admin`)* |
| `OCC_DEFAULT_LAT` / `OCC_DEFAULT_LNG` / `OCC_DEFAULT_LABEL` | `50.4542` / `3.9567` / `Mons` |
| `OCC_SEED_DEMO` | `true` au premier déploiement, puis indifférent |
| `OCC_PROVIDERS` | `ubereats,takeaway,deliveroo,weloveat` |

7. *Deploy*. Activer *Auto deploy* (webhook GitHub) pour déployer à chaque push sur `main`.

> Alternative *Docker Compose* : le `docker-compose.yml` fonctionne aussi ; dans
> ce cas supprimer la section `ports` (Coolify route via son proxy) et
> renseigner le domaine sur le service `occ`.

> Le conteneur tourne en utilisateur non-root (uid 10001). Avec un volume
> nommé (défaut Coolify) les permissions sont correctes ; avec un *bind mount*,
> faire `chown -R 10001:10001 <dossier>` sur l'hôte.

## 3. Après le premier déploiement
* Admin : `https://eat.fs0ciety.org/_/` (identifiants `OCC_ADMIN_*`).
* *Settings → Mail settings* : configurer un SMTP (vérification d'e-mail, reset mot de passe).
* *Collections → users → Options → OAuth2* : activer Google / Microsoft si voulu
  (les boutons apparaissent automatiquement sur la page de connexion).
* Restaurants : les restaurants réels de `backend/migrations/data/mons_restaurants.json`
  sont importés automatiquement (et remplacent la démo fictive) ; ensuite tout se
  gère depuis `/admin` (voir § 4).

## 4. Gérer les restaurants

**Panneau d'administration : `https://eat.fs0ciety.org/admin`** (en plus de l'admin
PocketBase `/_/`). Le lien « Admin » apparaît dans la navigation des comptes admin.

### Devenir admin
* Crée d'abord un compte normal dans l'app (inscription ou Google/Microsoft).
* Mets son e-mail dans `OCC_ADMIN_EMAIL` (le même e-mail que le superuser) **ou**
  dans `OCC_ADMINS` (plusieurs e-mails séparés par des virgules), puis redéploie :
  au démarrage ces comptes passent `admin`. Un compte créé *après* avec un de ces
  e-mails est admin dès l'inscription. Se reconnecter (ou recharger) pour voir le lien.
* Ensuite, un admin peut promouvoir / rétrograder d'autres comptes dans
  *Admin → Utilisateurs* (on ne peut pas se retirer ses propres droits).
* Un utilisateur ne peut jamais changer son propre rôle (refusé par le serveur).

### Ce qu'on peut faire
* **Tableau de bord** : utilisateurs, restaurants, commandes par statut et par jour,
  montant commandé, restaurants les plus commandés.
* **Restaurants** : rechercher, masquer / réafficher (un resto masqué disparaît de
  l'app mais reste dans l'historique), créer / modifier (adresse + bouton
  **Géocoder** via OpenStreetMap, liens Uber Eats / Takeaway / Deliveroo / weloveat,
  frais en euros). Badges **Verrouillé** et **Obsolète** : voir *Synchronisation*.
* **Cartes incomplètes** (carte en tête de *Restaurants*) : voir ci-dessous.
* **Menu** (clic sur un resto) : catégories (ajout, renommage, ordre ↑↓, suppression),
  articles (prix en euros `12,50`, étiquettes, disponible / populaire, options :
  groupes min/max et choix avec supplément).
* **Commandes** : liste filtrable par statut, détail (membres, articles, paiements),
  **annulation forcée**.
* **Synchronisation** : restaurants et menus relus automatiquement, voir ci-dessous.
* **Import** : voir ci-dessous.

### Cartes incomplètes : masquer / ré-afficher les restos de moins de N plats
* *Admin → Restaurants*, carte **Cartes incomplètes** : interrupteur « Masquer les restaurants
  incomplets » + champ « moins de [10] plats » (1–100, bouton **Appliquer** si on change le
  nombre pendant que le filtre est actif). Activé par défaut à **10** en production (migration
  `1760000010`) ; désactivé (0) sur une installation de démo.
* Un restaurant masqué disparaît de l'accueil, de *Restos* et du choix des candidats, mais reste
  ouvrable par lien, utilisable dans les commandes déjà lancées, et visible dans l'admin avec le
  badge « Masqué : carte incomplète (N plats) ». Seuls les plats **disponibles** comptent.
* **Ré-afficher** : couper l'interrupteur (tout redevient visible), baisser le seuil, ou compléter
  le menu — le restaurant réapparaît tout seul dès qu'il atteint le seuil (import, favori
  « Exporter vers OCC », synchronisation ou éditeur de menu). « Voir les restaurants masqués »
  (ou le filtre *Incomplets*, lien `/admin/restaurants?filtre=incompletes`) liste ceux à compléter.
* API (jeton admin ou superuser) : `GET /api/occ/admin/settings`,
  `PATCH /api/occ/admin/settings` avec `{"minMenuItems": 10}` (`0` = tout afficher). Le réglage
  vit dans la collection `app_settings` (une ligne, aussi modifiable dans `/_/`).

### Importer des menus (JSON ou CSV)
1. *Admin → Import* : télécharge le **modèle CSV** (une ligne par article, prix en
   euros avec virgule, séparateur `;` compatible Excel FR) ou l'**exemple JSON**.
2. Remplis-le puis **glisse-dépose** le fichier : l'app affiche un **aperçu**
   (restaurants nouveaux / mis à jour, menu, erreurs ligne par ligne, avertissements)
   **sans rien modifier**.
3. Si tout est valide, clique **Importer** : tout est écrit d'un coup (ou rien en cas
   d'erreur). Un restaurant est reconnu par son `slug` ; son menu est **remplacé**.
* CSV : colonnes `restaurant_slug, restaurant_name, category, item_name, description,
  price_eur, tags, popular` (+ facultatives `address, lat, lng, cuisines, phone,
  ubereats_url, takeaway_url`) — détail dans `docs/ARCHITECTURE.md`. Les options
  (tailles, suppléments) se règlent ensuite dans l'éditeur de menu ou en JSON.
* **Depuis Uber Eats / Takeaway** : l'outil `https://eat.fs0ciety.org/outils/export-menu.html`
  (favori à glisser dans la barre du navigateur) récupère le menu de la page du resto
  ouverte et produit un JSON d'un restaurant à déposer dans *Import*. Ce JSON n'a
  souvent ni adresse ni coordonnées : l'aperçu l'indique (« Coordonnées manquantes »),
  complète-les ensuite avec **Géocoder** — un réimport ultérieur les conserve.

### Sauvegarder / éditer en masse
* *Admin → Import → Exporter tout (JSON)* télécharge **tous** les restaurants
  (y compris masqués) avec leurs menus, dans le format d'import : c'est la
  sauvegarde des menus. Pour éditer en masse : exporter → modifier le JSON →
  réimporter (aperçu d'abord).
* API équivalente (jeton d'un admin) : `GET /api/occ/admin/export`,
  `POST /api/occ/admin/import[?dryRun=1]`, `POST /api/occ/admin/import/csv[?dryRun=1]`.

### Synchronisation automatique (restaurants et menus)
Le serveur relit chaque nuit les sources publiques et met le catalogue à jour tout seul
(ADR 0002, mise à jour 2). Page **Admin → Synchronisation** : état (en cours / dernier
résultat / prochaine exécution), bouton **Synchroniser maintenant**, historique des
exécutions (compteurs, liste des changements « Tomo — Miso ramen : 14,50 € → 15,00 € »,
journal) et liste des **sources**.

| variable | défaut | rôle |
|---|---|---|
| `OCC_SYNC_ENABLED` | `true` | `false` coupe tout : aucune requête sortante, bouton manuel refusé |
| `OCC_SYNC_CRON` | `30 3 * * *` | planification (cron 5 champs) lue à l'**heure de Bruxelles** (le cron interne de PocketBase est en UTC : le serveur convertit) |
| `OCC_SYNC_ON_START` | `true` | ~60 s après le démarrage, lance une synchronisation si aucune n'a encore réussi (premier déploiement) |
| `OCC_SYNC_START_DELAY` | `60s` | délai de ce premier lancement (durée Go : `30s`, `5m`) |

* **Durée** : quelques minutes (≈ 150 requêtes espacées d'au moins 2,5 s). Une seule
  exécution à la fois ; une relance le même jour lit le cache (`pb_data/menusync-cache`, 20 h)
  et ne refait presque aucune requête.
* **Sources** (*Admin → Synchronisation → Sources*) : activer / désactiver, priorité (la plus
  petite fournit le menu quand un restaurant est sur plusieurs sources : sites Takeaway 10–39,
  Deliveroo 50, weloveat 60, instantané Uber Eats 70 par défaut), *options* (weloveat : suppléments, 1 requête par plat —
  plusieurs heures, désactivé par défaut ; Deliveroo fournit les siennes sans coût).
  **Ajouter une source** : *Site Takeaway* = URL du site satellite d'un restaurant
  (`https://www.tomomons.be/`), *Site (schema.org)* = page carte d'un restaurant publiant un
  menu JSON-LD, *Deliveroo* = URL d'une page liste de ville (avec `?geohash=`), *weloveat* =
  racine de l'API (vide = défaut). Villes gérées : `mons` (rayon 8 km).
* **Découvrir un site Takeaway** (carte en tête des *Sources*) : takeaway.com est protégé par un
  défi Cloudflare (jamais contourné), mais beaucoup de restos ont un **site satellite** officiel au
  même modèle (`https://www.snack-a-la-gare.be/`, `https://www.tomomons.be/`…), lisible poliment.
  1. Colle le lien de la page du resto sur takeaway.com (ex.
     `https://www.takeaway.com/be-fr/menu/snack-a-la-gare`, `/be/`, `/be-nl/` ou just-eat), **ou**
     tape son nom (« Snack à la Gare »), **ou** colle directement l'adresse du site pour la vérifier.
  2. **Découvrir** : le serveur essaie une à une ≤ 16 adresses probables (`<slug>.be`, sans tirets,
     avec `-mons`/`mons`, `.com`, en `www.` puis sans) — 10 à 20 s ; les noms de domaine
     inexistants ne coûtent aucune requête, robots.txt est respecté, ≥ 1 s entre deux requêtes,
     takeaway.com n'est jamais appelé. Chaque site trouvé montre nom, adresse, nombre de plats et
     de catégories, distance (si le site publie ses coordonnées) et un lien pour le vérifier.
  3. **Ajouter et synchroniser** : crée la source *Site Takeaway* (activée, priorité libre entre 10
     et 39, nom du resto) puis lit **cette source seule** tout de suite (exécution « ciblée » :
     rien ne devient obsolète, le menu n'écrase pas celui d'une source prioritaire). Le résultat
     s'affiche sous le site et dans l'historique. « Déjà suivi » = une source lit déjà ce site.
     Si une synchronisation tourne déjà, la source est ajoutée et sera lue à la suivante.
  4. Rien trouvé : la liste des adresses vérifiées s'affiche ; si tu connais le site, ajoute-le à la
     main (**Ajouter une source** → *Site Takeaway*). Refusé si `OCC_SYNC_ENABLED=false`.
* **Ce que fait une exécution** : les restaurants sont reconnus (même source, même lien
  plateforme, ou même nom au même endroit) — les fiches existantes ne sont jamais dupliquées ;
  prix, descriptions, options et disponibilité des plats suivent la source ; nouveaux plats et
  catégories ajoutés ; un plat qui disparaît passe **indisponible** (il revient tout seul s'il
  réapparaît) ; les champs déjà remplis (adresse, coordonnées, téléphone, nom, emoji…) ne sont
  jamais écrasés, et jamais par une valeur vide. Rien n'est supprimé.
* **« Verrouillé »** : désactivé par défaut. Active l'interrupteur « Verrouillé » (formulaire du
  restaurant / de l'article, ou cadenas dans l'éditeur de menu) pour que la synchronisation ne
  touche plus jamais à cette fiche. Une simple modification ne verrouille pas.
* **Cartes partielles Uber Eats** : le connecteur ne fournit que quelques plats. Pour la carte
  complète : ouvre la page du resto sur Uber Eats, favori « Exporter vers OCC », puis
  `/admin → Import` — le fichier complète la fiche existante (retrouvée par lien ou nom, sans
  doublon) et retire le badge « Aperçu du menu ».
* **« Obsolète »** : plus aucune source activée ne propose ce restaurant (fermé, retiré d'une
  plateforme, source supprimée). Il reste **visible** ; le badge disparaît s'il revient. À toi
  de le masquer s'il a vraiment fermé. Une source en échec ou bloquée ce jour-là ne rend rien
  obsolète.
* **Statuts** : *Réussie*, *Partielle* (une source en échec / bloquée, les autres appliquées),
  *Bloquée* (toutes les sources ont refusé : 403 ou page anti-robot — rien n'est tenté pour
  contourner ; si cela dure, désactiver la source), *Échec*.
* **Désactiver** : `OCC_SYNC_ENABLED=false` puis redéployer (ou désactiver les sources une à
  une, effet immédiat). Les données déjà synchronisées restent.

### Instantané Uber Eats (restaurants à carte partielle)
Uber Eats n'est lisible que par le **connecteur Uber Eats d'une session Claude** (très limité :
nom, lien, note, délai, catégories et au plus 5 plats d'exemple — jamais la carte complète). Le
serveur ne l'appelle pas : le lead fige le résultat dans
`backend/migrations/data/mons_ubereats.json` (format : `docs/ARCHITECTURE.md`, *Instantané Uber
Eats*), lu par la source **« Uber Eats (instantané connecteur) »** (priorité 70, activée, aucune
requête réseau).

**Rafraîchir l'instantané :**
1. Dans une session Claude, interroger le connecteur Uber Eats pour l'adresse du bureau et
   réécrire le fichier (une entrée par restaurant ; prix en centimes ; `url` sans `?…` ;
   `lat`/`lng` à 0 et `geo_approx: true` si la position est inconnue ; `checked_at` du jour).
2. `cd backend && go test ./internal/app -run EmbeddedUberEats` (le fichier doit être lisible),
   puis commit + push sur `main` → redéploiement Coolify.
3. Appliqué à la **prochaine synchronisation** (nuit), ou tout de suite : *Admin →
   Synchronisation → **Synchroniser maintenant*** (les flux réseau relisent le cache du jour).

**Ce qui se passe** : un restaurant déjà connu (lien Uber Eats ou nom proche, « (Mons) » /
« (Independant) » ignorés) reçoit le lien Uber Eats et, s'ils manquent, note / avis / délai — sa
carte n'est jamais touchée. Un restaurant inconnu est créé **actif** avec le badge **« Aperçu du
menu »**, une catégorie « Aperçu » (les plats d'exemple) et un bandeau « Carte partielle… » avec un
bouton vers Uber Eats ; la commande groupée passe alors par l'envoi « Uber Eats ». Dès qu'une autre
source (site, Deliveroo, weloveat) fournit au moins 5 plats pour ce restaurant, sa carte complète
remplace l'aperçu (badge retiré). Les restaurants du fichier ne deviennent jamais « obsolètes »
tant qu'ils y figurent ; retirés du fichier, ils le deviennent. Les fiches **verrouillées** sont
ignorées. Admin : interrupteurs « Carte partielle » et « Position approximative » dans le
formulaire du restaurant (ils ne verrouillent pas la fiche à eux seuls).

## 5. Sauvegardes
* PocketBase : *Settings → Backups* → sauvegardes automatiques planifiées
  (option S3 compatible). Recommandé : quotidien, rétention 14.
* Ou sauvegarde du volume Coolify (*Scheduled backups* du serveur).

## 6. Exploitation
* Logs : Coolify → *Logs*, et *Logs* dans l'admin PocketBase (requêtes API).
* Mise à jour : push sur `main` → build → redéploiement (migrations appliquées
  automatiquement au démarrage).
* Rollback : *Deployments* → redéployer un commit précédent (les migrations
  sont additives ; restaurer une sauvegarde si une migration destructive a eu lieu).

## 7. Local
```bash
cp .env.example .env   # adapter
docker compose up --build
open http://localhost:8090
```

## 8. Outil `menusync` (ligne de commande)

Les restaurants se gèrent dans l'admin (`/_/`) ou par import JSON
(`POST /api/occ/admin/import`, un `RestaurantImport` par appel, jeton superuser).
Le serveur **synchronise lui-même** les cartes chaque nuit (voir §4 *Synchronisation
automatique*). L'outil en ligne de commande `menusync` (`/pb/menusync`) reste disponible
pour des essais ponctuels ou produire des fichiers JSON.

| source (`-provider`) | ce qui est lu | options (suppléments) |
|---|---|---|
| `deliveroo` | page liste de la ville puis page menu de chaque restaurant (`__NEXT_DATA__`) | oui (modifiers Deliveroo) |
| `weloveat` | API JSON anonyme de la SPA (`api.weloveat.be`, robots.txt « Allow all ») | avec `-options` (1 requête de plus par plat, long) |
| `takeaway-site` | sites satellites Takeaway d'un restaurant (`-urls fichier.txt` ou `-url`) | non (chargées en JS, commande désactivée sur ces sites) |
| `jsonld` | tout site exposant schema.org `Restaurant` + `hasMenu` (JSON-LD ou microdonnées) | non |

```bash
# dans le conteneur Coolify (Terminal de l'app, ou depuis l'hôte) :
docker exec -it <conteneur> /pb/menusync -provider deliveroo -city mons -limit 5 \
  -cache /pb/pb_data/menusync-cache -out /pb/pb_data/mons-deliveroo.json
docker exec -it <conteneur> /pb/menusync -provider weloveat -city mons \
  -cache /pb/pb_data/menusync-cache -out /pb/pb_data/mons-weloveat.json
docker exec -it <conteneur> /pb/menusync -provider takeaway-site -urls /pb/pb_data/sites.txt \
  -cache /pb/pb_data/menusync-cache -out /pb/pb_data/mons-sites.json

# fusionner (doublons : même lien plateforme / page source, même nom ≤ 1,5 km,
# nom proche ≤ 300 m, ou nom proche + même rue et numéro)
docker exec -it <conteneur> /pb/menusync merge \
  -in /pb/pb_data/mons-sites.json,/pb/pb_data/mons-deliveroo.json,/pb/pb_data/mons-weloveat.json \
  -out /pb/pb_data/mons-merged.json   # -priority takeaway-site,deliveroo,weloveat,jsonld,existing

# importer (vérifier d'abord avec -dry-run) ; jeton superuser :
#   curl -s -X POST https://eat.fs0ciety.org/api/collections/_superusers/auth-with-password \
#     -H 'Content-Type: application/json' -d '{"identity":"<OCC_ADMIN_EMAIL>","password":"…"}'  → "token"
docker exec -it -e OCC_IMPORT_TOKEN=<jeton> <conteneur> /pb/menusync \
  -in /pb/pb_data/mons-merged.json -import http://127.0.0.1:8090 -dry-run
```

* Sortie : tableau JSON au format `RestaurantImport` (prix en **centimes**, slug = nom
  « slugifié »), plus `source`, `source_urls`, `menu_checked_at` (ignorés à l'import).
  Un restaurant par ligne (`-pretty` pour du JSON indenté).
* Données nettoyées : noms (« BAGEL CITY » → « Bagel City », suffixes « - Mons - Mons Center »,
  « (MON) » retirés), cuisines unifiées (pluriels, synonymes, « boissons » / « desserts » retirés
  s'il y a mieux, 4 au plus), plats à 0 € sans options et doublons retirés.
* `merge` garde les liens de toutes les plateformes, prend le menu de la source préférée
  (`-priority`), les champs « éditoriaux » (slug, nom, emoji, gamme de prix, adresse) de
  la fiche `existing` (préfixe `existing=fichier.json`) et n'invente aucun champ absent.
* **Politesse (non désactivable)** : User-Agent `OCC-Deliveries-menusync/1.0 (+https://eat.fs0ciety.org)`,
  requêtes séquentielles espacées d'au moins 2,5 s (+ aléa), 1 seul nouvel essai sur
  429/5xx (`Retry-After` respecté), robots.txt appliqué (URL interdite = non demandée,
  jamais d'appel aux API `/api/` ou `graphql` de Deliveroo), cache disque (`-cache`) pour
  ne pas refaire les requêtes. Sur **403 ou page anti-robot** (Cloudflare, PerimeterX…)
  l'outil **s'arrête** (code 3) en écrivant ce qu'il a déjà lu : aucun contournement n'est
  tenté (ni navigateur furtif, ni captcha, ni proxy). Relancer plus tard.
* Rayon : `-radius 8` km autour du centre de `-city` (défaut). `-limit N` pour un essai.
* Les CGU des plateformes restent applicables : usage interne, volumes modestes, données
  vérifiées avant import (les prix des plateformes incluent souvent une marge).
