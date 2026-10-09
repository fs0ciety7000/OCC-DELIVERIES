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
| `OCC_GOOGLE_CLIENT_ID` / `OCC_GOOGLE_CLIENT_SECRET` | *(facultatif, secrets : « Continuer avec Google », voir § 3 bis)* |
| `OCC_SMTP_HOST` / `OCC_SMTP_PORT` / `OCC_SMTP_USERNAME` / `OCC_SMTP_PASSWORD` | *(facultatif, secrets : e-mails, voir § 3 ter)* |
| `OCC_MAIL_FROM` / `OCC_MAIL_FROM_NAME` | `noreply@fs0ciety.org` / `OCC Deliveries` |

7. *Deploy*. Activer *Auto deploy* (webhook GitHub) pour déployer à chaque push sur `main`.

> Alternative *Docker Compose* : le `docker-compose.yml` fonctionne aussi ; dans
> ce cas supprimer la section `ports` (Coolify route via son proxy) et
> renseigner le domaine sur le service `occ`.

> Le conteneur tourne en utilisateur non-root (uid 10001). Avec un volume
> nommé (défaut Coolify) les permissions sont correctes ; avec un *bind mount*,
> faire `chown -R 10001:10001 <dossier>` sur l'hôte.

## 3. Après le premier déploiement
* Admin : `https://eat.fs0ciety.org/_/` (identifiants `OCC_ADMIN_*`).
* E-mails et Google : de préférence par variables d'environnement (§ 3 bis / § 3 ter). Sans elles,
  `/_/` → *Settings → Mail settings* et *Collections → users → Options → OAuth2* restent utilisables
  (le serveur ne touche à ces réglages que si les variables `OCC_*` correspondantes sont définies) ;
  les boutons OAuth apparaissent automatiquement sur la connexion / l'inscription.
* Restaurants : les restaurants réels de `backend/migrations/data/mons_restaurants.json`
  sont importés automatiquement (et remplacent la démo fictive) ; ensuite tout se
  gère depuis `/admin` (voir § 4).

## 3 bis. Connexion avec Google
Le bouton **« Continuer avec Google »** (connexion **et** inscription) n'apparaît que lorsque le fournisseur
est actif. On peut réutiliser le projet Google Cloud qui sert déjà à `fs0ciety.org` : il suffit d'y ajouter
un client (ou l'URI de redirection ci-dessous à un client « Application Web » existant).

1. <https://console.cloud.google.com/> → sélectionner le projet fs0ciety (ou en créer un).
2. *APIs & Services → OAuth consent screen* (« Écran de consentement » / *Google Auth Platform → Branding*) :
   type **Externe**, nom « OCC Deliveries », e-mail d'assistance, domaine autorisé **`fs0ciety.org`**
   (déjà présent si le projet sert au site), logo facultatif ; portées : `openid`, `email`, `profile`
   (aucune portée sensible → pas de validation Google). Publier l'application (*In production*) ; en mode
   *Testing*, seuls les comptes listés comme testeurs peuvent se connecter.
3. *Credentials → Create credentials → OAuth client ID* → type **Web application** (« Application Web »),
   nom « OCC Deliveries » :
   * *Authorized JavaScript origins* : **`https://eat.fs0ciety.org`**
   * *Authorized redirect URIs* : **`https://eat.fs0ciety.org/api/oauth2-redirect`** (exactement ; en local,
     ajouter `http://localhost:8090/api/oauth2-redirect` et l'origine `http://localhost:8090`)
4. Copier l'*ID client* et le *code secret* → Coolify → *Environment variables* :
   `OCC_GOOGLE_CLIENT_ID=…apps.googleusercontent.com`, `OCC_GOOGLE_CLIENT_SECRET=…` (cocher *Is secret*),
   puis **Redeploy**. Au démarrage le journal affiche « Google OAuth2 enabled from env ».
5. Vérifier : `https://eat.fs0ciety.org/login` → « Continuer avec Google » (fenêtre surgissante ; si le
   navigateur la bloque, l'app l'explique : autoriser les pop-ups pour le site).

Comportement : premier passage = compte créé (nom + photo Google, adresse déjà vérifiée, rôle admin si
l'e-mail est dans `OCC_ADMIN_EMAIL` / `OCC_ADMINS`). Un compte « e-mail + mot de passe » existant avec la
**même adresse** est **lié** automatiquement : s'il était vérifié, son mot de passe reste valable ; s'il ne
l'était pas, PocketBase remplace son mot de passe (sécurité) — l'utilisateur en rechoisit un via « Mot de
passe oublié ». Chacun voit « Google connecté » dans *Profil → Mes infos → Sécurité* et peut le dissocier
s'il a un mot de passe. Désactiver : retirer les deux variables **et** désactiver Google dans `/_/`
(*Collections → users → OAuth2*), sinon la configuration enregistrée reste active.

## 3 ter. E-mails (vérification, mot de passe oublié, changement d'adresse)
Sans SMTP, l'app fonctionne mais sans e-mails : pas de « Mot de passe oublié » (la page l'indique), pas de
lien de réinitialisation envoyé par un admin, pas de changement d'adresse. Avec SMTP :
vérification à l'inscription (le compte reste utilisable sans vérification ; un bandeau discret le
rappelle), « Mot de passe oublié ? » sur la connexion, changement d'adresse et de mot de passe dans le
profil, lien de réinitialisation envoyé depuis *Admin → Utilisateurs*. Les liens des e-mails pointent vers
l'app (`/auth/verifier/…`, `/auth/reinitialiser/…`, `/auth/changer-email/…`), d'où l'importance
d'`OCC_PUBLIC_URL`.

| variable | exemple | notes |
|---|---|---|
| `OCC_SMTP_HOST` | `smtp-relay.brevo.com` | défini = SMTP activé au démarrage |
| `OCC_SMTP_PORT` | `587` | `465` = TLS implicite |
| `OCC_SMTP_USERNAME` | *(identifiant SMTP)* | |
| `OCC_SMTP_PASSWORD` | *(clé SMTP — secret)* | jamais journalisé |
| `OCC_SMTP_TLS` | *(vide)* | `true` force le TLS implicite (défaut : `true` sur 465, STARTTLS sinon) |
| `OCC_MAIL_FROM` | `noreply@fs0ciety.org` | adresse d'un domaine **authentifié** (SPF/DKIM) chez le fournisseur |
| `OCC_MAIL_FROM_NAME` | `OCC Deliveries` | |

Fournisseurs conseillés :
* **Brevo** (ex-Sendinblue, gratuit 300 e-mails/jour) : *SMTP & API* → clé SMTP ; hôte
  `smtp-relay.brevo.com`, port `587`, identifiant = login SMTP affiché. Authentifier `fs0ciety.org`
  (enregistrements DKIM / DMARC fournis) pour envoyer en `noreply@fs0ciety.org`.
* **Resend** (gratuit 3 000 e-mails/mois) : domaine `fs0ciety.org` vérifié ; hôte `smtp.resend.com`,
  port `465` (TLS) ou `587`, identifiant `resend`, mot de passe = clé API.
* **Gmail / Google Workspace** (dépannage, ~500 e-mails/jour) : activer la validation en deux étapes, créer
  un **mot de passe d'application** ; hôte `smtp.gmail.com`, port `587`, identifiant = l'adresse Gmail,
  `OCC_MAIL_FROM` = cette même adresse (ou un alias vérifié).

**Tester** : redéployer, puis *Admin → Utilisateurs* → carte « E-mails » (badge **Actifs**, expéditeur,
serveur) → **« Envoyer un e-mail de test »** : il arrive à l'adresse de l'admin connecté. En cas d'erreur
(identifiants, port, TLS), le message du serveur SMTP s'affiche dans le toast. Ensuite : « Mot de passe
oublié ? » depuis la page de connexion. Les e-mails partent en `text/html` (modèles français
`backend/internal/app/mailtemplates.go`) ; vérifier qu'ils n'arrivent pas en indésirables (SPF/DKIM).

## 4. Gérer les restaurants

**Panneau d'administration : `https://eat.fs0ciety.org/admin`** (en plus de l'admin
PocketBase `/_/`). Le lien « Admin » apparaît dans la navigation des comptes admin.

### Devenir admin
* Crée d'abord un compte normal dans l'app (inscription ou Google).
* Mets son e-mail dans `OCC_ADMIN_EMAIL` (le même e-mail que le superuser) **ou**
  dans `OCC_ADMINS` (plusieurs e-mails séparés par des virgules), puis redéploie :
  au démarrage ces comptes passent `admin`. Un compte créé *après* avec un de ces
  e-mails est admin dès l'inscription. Se reconnecter (ou recharger) pour voir le lien.
* Ensuite, un admin peut promouvoir / rétrograder d'autres comptes dans
  *Admin → Utilisateurs* (on ne peut pas se retirer ses propres droits).
* Un utilisateur ne peut jamais changer son propre rôle (refusé par le serveur).

### Gérer les comptes (*Admin → Utilisateurs*)
* Filtres **Tous / Admins / Suspendus / Non vérifiés**, recherche nom / e-mail ; badges Admin, Suspendu
  (avec motif), Non vérifié, Google ; date d'inscription et de dernière connexion.
* Menu « … » d'un compte : promouvoir / retirer admin, **envoyer un lien de réinitialisation** (e-mail ;
  personne ne voit jamais de mot de passe ; nécessite SMTP), **forcer la déconnexion** (tous appareils),
  **suspendre** (motif facultatif ; sessions coupées, connexion refusée « Compte suspendu ») / réactiver,
  **supprimer le compte** (anonymisation : nom « Compte supprimé », e-mail, photo, coordonnées de
  remboursement et lien Google effacés ; l'historique des commandes et les montants restent).
* Garde-fous serveur : pas d'action sur son propre compte, jamais sur le dernier admin actif.
* Chacun peut aussi supprimer son compte : *Profil → Mes infos → Supprimer mon compte* (taper SUPPRIMER ;
  refusé tant qu'il héberge une commande en cours ou qu'on lui doit un remboursement).

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
| `OCC_ENRICH_ENABLED` | `true` | à la fin de chaque exécution, complète téléphone / adresse / position manquants depuis OpenStreetMap (voir ci-dessous) ; `false` = aucune requête vers Nominatim |

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
* **Coordonnées (téléphone, adresse, position)** : lues dans chaque source quand elle les publie,
  puis normalisées (téléphone `+3265352964`, affiché `+32 65 35 29 64` ; adresse
  `Rue de la Clef 24, 7000 Mons`, localité déduite du code postal). Les restaurants actifs non
  verrouillés à qui il manque encore un téléphone, une adresse ou une vraie position (aperçus Uber
  Eats) sont ensuite cherchés dans **OpenStreetMap** (Nominatim) : au plus 60 requêtes par nuit,
  ≥ 1,1 s entre deux, User-Agent identifié, réponses gardées 30 jours en cache
  (`pb_data/menusync-cache/nominatim`), arrêt immédiat si Nominatim refuse ou limite (429/403).
  Un résultat n'est retenu que s'il s'agit d'un lieu de restauration du même nom, à ≤ 12 km du lieu
  par défaut (≤ 1,5 km de la position connue, dans la même rue que l'adresse connue) ; deux
  homonymes éloignés (deux Pizza Hut) sans position connue = rien n'est écrit. Seuls les champs
  **vides** sont remplis. Changements « Pizza Hut — téléphone ajouté (OpenStreetMap) » et compteur
  « restaurants complétés » dans l'historique. La liste *Admin → Restaurants* signale « sans tél. »
  et « sans adresse ».
* **Attribution OpenStreetMap (ODbL, obligatoire)** : un restaurant complété par OSM garde la
  provenance (`enriched_from`) et sa fiche affiche « Coordonnées : © contributeurs OpenStreetMap »
  (lien vers `openstreetmap.org/copyright`), comme l'envoi « Téléphone ». Ne pas retirer ce crédit.
  Corriger un champ à la main retire son crédit. Respecter la politique d'usage de Nominatim
  (pas de requêtes en masse, pas de relance en boucle) ; en cas d'abus signalé : `OCC_ENRICH_ENABLED=false`.

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
