# OCC Deliveries — backend

PocketBase **v0.36.6** used as a Go framework (Go 1.24). Contract: `../docs/ARCHITECTURE.md`.

```bash
go run . serve --http=127.0.0.1:8090   # API /api/occ, admin /_/, SPA from ./pb_public
go test ./...                          # domain table tests + PocketBase integration tests
go vet ./... && gofmt -l .
go build -ldflags "-X main.version=1.2.3" -o occ .
```

| dossier | rôle |
|---|---|
| `main.go`, `spa.go` | bootstrap : migrations, hooks/routes, superuser `OCC_ADMIN_*`, `meta.appURL`, SPA (`OCC_PUBLIC_DIR`, fallback `index.html`, jamais sur `/api/*`) |
| `internal/domain` | logique pure sans PocketBase : prix des options, split des frais (plus grand reste), résumé, state machine, élection, code, IBAN, EPC, Wero/Bancontact, haversine |
| `internal/providers` | Uber Eats / Takeaway / export / téléphone → `Dispatch` |
| `internal/catalog` | import/upsert de restaurants par slug (admin + seed) |
| `internal/app` | hooks (champs forcés, prix serveur, `ready`, IBAN…) et routes `/api/occ/*` + tests d'intégration |
| `migrations` | schéma + règles, seed démo (restaurants **fictifs** autour de Mons ; `OCC_SEED_DEMO=false` pour l'ignorer) |

Les tests d'intégration construisent une fois un `pb_data` migré (temp dir) puis
clonent ce modèle pour chaque test (`tests.NewTestApp`).

Note : PocketBase v0.36.7+ exige Go ≥ 1.25, d'où l'épinglage en v0.36.6.
