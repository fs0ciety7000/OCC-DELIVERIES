# Workflow d'équipe

## Branches & PR
* `main` = production (déployée automatiquement par Coolify au push).
* Travail sur branches `feat/…`, `fix/…`, `chore/…` → PR vers `main`.
* La CI (`.github/workflows/ci.yml`) doit être verte : Go (vet, gofmt, tests),
  front (typecheck, lint, tests, build), build Docker.
* Squash merge, message conventionnel.

## Cycle d'une fonctionnalité
1. **Cadrer** — user story + critères d'acceptation dans l'issue (ou `docs/ROADMAP.md`).
2. **Contrat** — si le schéma ou l'API change : PR qui modifie d'abord
   `docs/ARCHITECTURE.md` (+ ADR si décision structurante).
3. **Backend** — migration Go (jamais modifier une migration déjà déployée :
   en créer une nouvelle), logique dans `internal/domain` + tests, glue dans `internal/app`.
4. **Frontend** — types `src/lib/types.ts`, API `src/lib/api.ts`, écran dans
   `src/features/…`, composants du design system.
5. **QA** — scénario manuel ou e2e : 2 navigateurs (hôte + invité) en parallèle
   pour vérifier le realtime.
6. **Docs** — mettre à jour CLAUDE.md (journal), ROADMAP, DESIGN_SYSTEM si nouveau composant.

## Definition of Done
- [ ] Critères d'acceptation vérifiés sur mobile (375 px) et desktop.
- [ ] Tests ajoutés (domain Go obligatoire ; composants critiques côté front).
- [ ] `make test` et `make build` verts.
- [ ] Règles PocketBase revues (qui peut lire/écrire ?).
- [ ] Textes en français, états vides / chargement / erreur gérés.
- [ ] Contrat et docs à jour.

## Orchestration des agents Claude
Pour une grosse fonctionnalité, l'agent principal :
1. met à jour le contrat (`docs/ARCHITECTURE.md`) ;
2. lance en parallèle `backend-engineer` et `frontend-engineer` sur ce contrat ;
3. intègre, puis lance `qa-engineer` (scénario e2e) et `product-designer` (revue UI) ;
4. corrige, met à jour le journal de `CLAUDE.md`, commit.

## Migrations
* Fichiers `backend/migrations/<timestamp>_<nom>.go`, appliqués automatiquement au démarrage.
* Toujours fournir le `down` quand c'est raisonnable.
* Données de démo : migration `seed_demo` conditionnée par `OCC_SEED_DEMO`.
