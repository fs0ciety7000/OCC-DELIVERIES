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
| `OCC_PROVIDERS` | `ubereats,takeaway` |

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
  **Géocoder** via OpenStreetMap, liens Uber Eats / Takeaway, frais en euros).
* **Menu** (clic sur un resto) : catégories (ajout, renommage, ordre ↑↓, suppression),
  articles (prix en euros `12,50`, étiquettes, disponible / populaire, options :
  groupes min/max et choix avec supplément).
* **Commandes** : liste filtrable par statut, détail (membres, articles, paiements),
  **annulation forcée**.
* **Import** : voir ci-dessous.

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
