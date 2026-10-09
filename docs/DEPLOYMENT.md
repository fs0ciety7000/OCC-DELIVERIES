# Déploiement — Coolify (`eat.fs0ciety.org`)

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
