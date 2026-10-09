# 🔥 OCC DELIVERIES

**Commandes groupées entre collègues, sans prise de tête.**
On ouvre une commande, tout le monde rejoint, on vote pour le resto, chacun
compose son panier, on envoie vers Uber Eats / Takeaway (ou on exporte), et
chacun rembourse le payeur en scannant un QR code.

`https://eat.fs0ciety.org`

## Démarrer
```bash
# Prérequis : Go 1.27.2+ (auto via GOTOOLCHAIN), Node 24 LTS (`.nvmrc`)
cd frontend && npm ci && cd ..
make dev            # API http://127.0.0.1:8090  ·  front http://localhost:5173
```
ou en conteneur : `cp .env.example .env && docker compose up --build` → http://localhost:8090

Admin PocketBase : `/_/` (identifiants `OCC_ADMIN_EMAIL` / `OCC_ADMIN_PASSWORD`).

## Tests end-to-end (Playwright)
Scénario complet hôte + 2 invités (dont mobile 390×844) contre une instance qui tourne :
`cd e2e && npm ci && E2E_BASE_URL=http://localhost:8090 npx playwright test` (Chromium de `/opt/pw-browsers` ou `PLAYWRIGHT_BROWSERS_PATH` ; `E2E_DEBUG=1` ajoute des captures).

## Stack
PocketBase v0.40 (Go 1.27, SQLite, realtime) · React 19 · Vite · Tailwind v4 ·
TanStack Query · Docker · Coolify.

## Documentation
* [CLAUDE.md](CLAUDE.md) — règles du projet & mémoire de l'équipe
* [Architecture & API](docs/ARCHITECTURE.md)
* [Design system « Ember »](docs/DESIGN_SYSTEM.md)
* [Workflow](docs/WORKFLOW.md) · [Déploiement Coolify](docs/DEPLOYMENT.md) · [Roadmap](docs/ROADMAP.md)
* [Décisions (ADR)](docs/adr/)

### Captures d'écran (revue design)
`cd e2e && node scripts/capture.mjs http://localhost:8090 ../docs/screenshots` — régénère les
captures mobile/desktop, thèmes sombre/clair, de chaque écran (voir `docs/screenshots/`).
