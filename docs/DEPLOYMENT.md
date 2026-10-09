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
* Remplacer/compléter les restaurants de démo (admin ou `POST /api/occ/admin/import`).

## 4. Sauvegardes
* PocketBase : *Settings → Backups* → sauvegardes automatiques planifiées
  (option S3 compatible). Recommandé : quotidien, rétention 14.
* Ou sauvegarde du volume Coolify (*Scheduled backups* du serveur).

## 5. Exploitation
* Logs : Coolify → *Logs*, et *Logs* dans l'admin PocketBase (requêtes API).
* Mise à jour : push sur `main` → build → redéploiement (migrations appliquées
  automatiquement au démarrage).
* Rollback : *Deployments* → redéployer un commit précédent (les migrations
  sont additives ; restaurer une sauvegarde si une migration destructive a eu lieu).

## 6. Local
```bash
cp .env.example .env   # adapter
docker compose up --build
open http://localhost:8090
```

## 7. Gérer les restaurants

Les restaurants se gèrent dans l'admin (`/_/`) ou par import JSON
(`POST /api/occ/admin/import`, un `RestaurantImport` par appel, jeton superuser).
Pour **récupérer les cartes réelles** des plateformes, l'image contient l'outil
`menusync` (`/pb/menusync`, binaire séparé : le serveur ne l'appelle jamais).

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

# fusionner (doublons : même lien plateforme, ou nom proche + < 150 m / même rue et numéro)
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
