# CLAUDE.md — OCC DELIVERIES

Plateforme de **commandes groupées** entre collègues : on ouvre une commande
(« party »), les collègues rejoignent, votent pour un restaurant, chacun compose
son panier, on envoie vers Uber Eats / Takeaway (ou on exporte), on désigne le
payeur et chacun rembourse sa part (Wero, Bancontact Pay, QR virement SEPA, espèces, plus tard).

Production : `https://eat.fs0ciety.org` (Coolify, Docker).

## Documents de référence — à lire avant de coder
| sujet | fichier |
|---|---|
| Modèle de données, state machine, API `/api/occ` | `docs/ARCHITECTURE.md` (**contrat**) |
| Design system « Ember » (tokens, composants, ton) | `docs/DESIGN_SYSTEM.md` |
| Workflow, branches, Definition of Done, agents | `docs/WORKFLOW.md` |
| Déploiement Coolify | `docs/DEPLOYMENT.md` |
| Décisions (ADR) | `docs/adr/` |
| Feuille de route | `docs/ROADMAP.md` |

## Stack
* **backend/** — Go 1.27.2 (`toolchain` dans `go.mod`), PocketBase **v0.40.5** utilisé comme framework
  (SQLite WAL, auth, realtime SSE, admin `/_/`). Migrations Go dans `backend/migrations`.
* **frontend/** — React 19, Vite, TypeScript strict, Tailwind CSS v4, TanStack
  Query, React Router, `pocketbase` JS SDK, `motion`, `lucide-react`, `sonner`,
  `qrcode.react`. Tests : Vitest + Testing Library. Node **24 LTS** (`.nvmrc`, `engines`).
* **Déploiement** — un seul conteneur (Dockerfile multi-étapes) : le binaire Go
  sert l'API **et** la SPA (`pb_public`). Volume persistant `/pb/pb_data`.
  Images : `node:24-alpine` → `golang:1.27-alpine` → `alpine:3.24`.

## Commandes
```bash
# Backend
cd backend && go run . serve --http=127.0.0.1:8090   # API + admin sur :8090
cd backend && go test ./...                           # tests
cd backend && go vet ./... && gofmt -l .              # lint

# Frontend
cd frontend && npm ci
cd frontend && npm run dev        # Vite :5173, proxy /api et /_ → :8090
cd frontend && npm run typecheck && npm run lint && npm test && npm run build

# Tout-en-un
make dev | make test | make build | make docker
docker compose up --build         # http://localhost:8090
```

## Règles du projet (non négociables)
1. **Argent en centimes `int`** partout (Go, JSON, TS). Formatage uniquement à l'affichage.
2. **Le serveur fait foi** : prix, totaux, statuts, codes et parts sont calculés
   côté Go. Le front n'envoie que des intentions (`menu_item`, options, quantité…).
3. Toute écriture sensible passe par une **rule PocketBase** *et* une validation
   dans un hook/route. Jamais de rule `""` en écriture.
4. La logique métier pure vit dans `backend/internal/domain` (zéro import
   PocketBase) et est **testée** (tables de tests).
5. Le contrat `docs/ARCHITECTURE.md` est mis à jour **dans le même commit** que
   tout changement de schéma ou d'endpoint ; les types TS (`frontend/src/lib/types.ts`)
   aussi.
6. Front : uniquement les tokens du design system (pas de hex en dur), textes en
   français, mobile d'abord, accessibilité AA, `prefers-reduced-motion`.
7. Pas de secrets dans le repo. Config via variables `OCC_*` (voir ARCHITECTURE §7).
8. Commits conventionnels (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`).
9. Dépendances : toujours les dernières versions stables (LTS pour Node), épinglées
   exactement ; exceptions documentées dans le journal.

## Agents du projet (`.claude/agents/`)
* `backend-engineer` — Go/PocketBase, migrations, hooks, routes, tests.
* `frontend-engineer` — React/Tailwind, écrans, realtime, design system.
* `product-designer` — revue UX/UI contre le design system, accessibilité.
* `qa-engineer` — scénarios end-to-end, revue de régressions, tests.
* `devops-engineer` — Docker, CI, Coolify, sauvegardes.

## Journal de bord
Les apprentissages importants (pièges PocketBase, décisions d'UX) sont ajoutés
ci-dessous, du plus récent au plus ancien.

* 2026-10-09 — **Mise en production** sur `https://eat.fs0ciety.org` (Coolify, app
  `ichbnb7y7rucoiqfcs0xlhfd`, branche `main`, auto-deploy). CI GitHub verte (build Docker
  complet inclus). API Coolify 4.4 : `POST /api/v1/deploy` (plus de GET),
  `PATCH /applications/{uuid}/envs/bulk`, `POST /applications/{uuid}/storages`.
  `/api/occ/health` renvoie `version: dev` tant que « Include Source Commit in Build »
  n'est pas activé dans Coolify.
* 2026-10-09 — E2E Playwright (`e2e/`) : parcours complet hôte + 2 invités + non-membre,
  vert sur PocketBase v0.36 puis v0.40.5. Exception de version : `@playwright/test`
  épinglé en **1.56.1** pour correspondre au Chromium préinstallé de l'environnement
  cloud (chromium-1194) ; à monter avec l'image navigateur.
* 2026-10-09 — Docker Hub limite les pulls (429) dans l'environnement cloud : utiliser
  `mirror.gcr.io/library/<image>` + `docker tag`. Les builds Docker n'y ont pas de
  réseau : valider l'étape runtime avec les artefacts construits localement ;
  la CI GitHub fait le build complet.
* 2026-10-09 — Mise à niveau complète vers les dernières versions stables :
  Go 1.24.7 → **1.27.2** · PocketBase v0.36.6 → **v0.40.5** (+ `go get -u ./...`) ·
  Node 22 → **24 LTS** · `golang:1.27-alpine`, `node:24-alpine`, `alpine:3.24` ·
  actions checkout/setup-go/setup-node **v7**, setup-buildx **v4**, build-push **v7** ·
  jsdom 29 → 30 (les autres paquets npm étaient déjà au dernier stable).
  - PocketBase v0.40 passe à `encoding/json/v2` : réponses JSON avec slices nil → `[]`
    (plus `null`) et clés des records non triées ; `BindBody` reste insensible à la casse ;
  - `app.ResetBootstrapState()` déprécié → `ClearBootstrap()` (harness de tests) ;
  - v0.40 : les erreurs des commandes CLI sortent en code ≠ 0 ; en-tête
    `Cross-Origin-Opener-Policy: same-origin` ajouté par défaut ;
  - ozzo-validation remplacé par le fork `github.com/pocketbase/ozzo-validation` (indirect) ;
  - rules inchangées : `members.id ?= @request.auth.id` reste la bonne forme (tests authz verts).
  - **Exceptions sous la dernière version** : `typescript` **6.0.3** (7.0.2 existe, mais
    `typescript-eslint` 8.71.1, dernier stable, exige `typescript <6.1.0`) ;
    `@types/node` **24.19.1** (26.x existe mais suit Node 26 « Current » ; on aligne les
    types sur le runtime LTS 24). `pocketbase` JS SDK 0.28.1 est déjà le dernier.
* 2026-10-09 — Pièges PocketBase v0.36 découverts au bootstrap :
  - relations multiples dans les rules : `members.id ?= @request.auth.id`, **jamais** `members ?= …` (compare le JSON brut → personne ne matche) ;
  - une update rule ne voit que l'enregistrement stocké : bloquer les champs immuables dans le hook en comparant à `e.Record.Original()` ;
  - les create rules s'exécutent avant `OnRecordCreateRequest` → le hook peut écraser ce que le client envoie ;
  - effets de bord atomiques : `e.App.RunInTransaction(func(tx){ e.App = tx; e.Next() … })` puis restaurer `e.App` ;
  - `BoolField{Required:true}` exige `true`, `NumberField{Required:true}` refuse 0 ;
  - `apis.Static(…, true)` sur `/{path...}` répond aussi aux `/api/*` inconnus → garde dans `backend/spa.go` ;
  - tests d'intégration : un `pb_data` migré dans `TestMain`, cloné par test avec `tests.NewTestApp(dir)`.
* 2026-10-09 — Bootstrap : PocketBase **v0.36.6** (v0.36.7+ et v0.37+ exigent Go ≥ 1.25 ; monter Go avant de monter PocketBase — fait le jour même, voir ci-dessus).
  Le connecteur MCP Uber Eats n'était pas joignable pendant le bootstrap :
  l'intégration passe par l'adaptateur deep link (ADR 0002).
