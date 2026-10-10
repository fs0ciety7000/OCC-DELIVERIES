# OCC Deliveries — backend

PocketBase **v0.40.5** used as a Go framework (Go 1.27.2). Contract: `../docs/ARCHITECTURE.md`.

```bash
go run . serve --http=127.0.0.1:8090   # API /api/occ, admin /_/, SPA from ./pb_public
go test ./...                          # domain table tests + PocketBase integration tests
go vet ./... && gofmt -l .
go build -ldflags "-X main.version=1.2.3" -o occ .
```

| dossier | rôle |
|---|---|
| `main.go`, `spa.go` | bootstrap : migrations, hooks/routes, superuser `OCC_ADMIN_*`, `meta.appURL`, SPA (`OCC_PUBLIC_DIR`, fallback `index.html`, jamais sur `/api/*`) |
| `internal/domain` | logique pure sans PocketBase : prix des options, split des frais (plus grand reste), résumé, state machine, élection, code, IBAN, EPC, liens de paiement, haversine |
| `internal/providers` | Uber Eats / Takeaway / Deliveroo / weloveat / export / téléphone → `Dispatch` |
| `internal/menusync`, `cmd/menusync` | outil `menusync` : flux restaurants + menus (Deliveroo, weloveat, sites Takeaway, JSON-LD), fusion, import ; parseurs testés sur `internal/menusync/testdata` (aucun réseau en test) |
| `internal/catalog` | import/upsert de restaurants par slug (admin + seed) |
| `internal/app` | hooks (champs forcés, prix serveur, `ready`, IBAN…) et routes `/api/occ/*` + tests d'intégration |
| `migrations` | schéma + règles, seed démo (restaurants **fictifs** autour de Mons ; `OCC_SEED_DEMO=false` pour l'ignorer) |

Les tests d'intégration construisent une fois un `pb_data` migré (temp dir) puis
clonent ce modèle pour chaque test (`tests.NewTestApp`).

Note : PocketBase v0.40+ exige Go ≥ 1.27 (`encoding/json/v2`) ; le `toolchain go1.27.2` de `go.mod` est téléchargé automatiquement (`GOTOOLCHAIN=auto`).

## menusync — flux de menus

Binaire séparé (`/pb/menusync` dans l'image Docker). Détails, politesse et usage sur
Coolify : `../docs/DEPLOYMENT.md` § « Gérer les restaurants ».

```bash
go run ./cmd/menusync -provider deliveroo -city mons -limit 5 -out mons-deliveroo.json
go run ./cmd/menusync -provider weloveat -city mons -out mons-weloveat.json      # -options : suppléments (long)
go run ./cmd/menusync -provider takeaway-site -urls migrations/data/mons_takeaway_sites.txt -out mons-sites.json
go run ./cmd/menusync -provider jsonld -url https://exemple.be/menu -out x.json
go run ./cmd/menusync merge -in mons-sites.json,mons-deliveroo.json,existing=migrations/data/mons_restaurants.json -out merged.json
go run ./cmd/menusync -in merged.json -import https://eat.fs0ciety.org -token "$TOKEN" -dry-run
```

Codes de sortie : 0 ok, 1 erreur, 2 usage, 3 arrêt sur 403 / page anti-robot (fichier partiel écrit).
Les fichiers produits le 2026-10-09 sont dans `migrations/data/` (non branchés aux migrations,
voir `migrations/data/SOURCES_menusync.md`).
