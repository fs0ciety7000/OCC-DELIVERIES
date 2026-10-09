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
* **backend/** — Go 1.24, PocketBase **v0.36.x** utilisé comme framework
  (SQLite WAL, auth, realtime SSE, admin `/_/`). Migrations Go dans `backend/migrations`.
* **frontend/** — React 19, Vite, TypeScript strict, Tailwind CSS v4, TanStack
  Query, React Router, `pocketbase` JS SDK, `motion`, `lucide-react`, `sonner`,
  `qrcode.react`. Tests : Vitest + Testing Library.
* **Déploiement** — un seul conteneur (Dockerfile multi-étapes) : le binaire Go
  sert l'API **et** la SPA (`pb_public`). Volume persistant `/pb/pb_data`.

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

## Agents du projet (`.claude/agents/`)
* `backend-engineer` — Go/PocketBase, migrations, hooks, routes, tests.
* `frontend-engineer` — React/Tailwind, écrans, realtime, design system.
* `product-designer` — revue UX/UI contre le design system, accessibilité.
* `qa-engineer` — scénarios end-to-end, revue de régressions, tests.
* `devops-engineer` — Docker, CI, Coolify, sauvegardes.

## Journal de bord
Les apprentissages importants (pièges PocketBase, décisions d'UX) sont ajoutés
ci-dessous, du plus récent au plus ancien.

* 2026-10-09 — Bootstrap : PocketBase v0.36.9 (v0.37+ exige Go ≥ 1.25).
  Le connecteur MCP Uber Eats n'était pas joignable pendant le bootstrap :
  l'intégration passe par l'adaptateur deep link (ADR 0002).
