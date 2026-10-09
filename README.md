# 🔥 OCC DELIVERIES

**Commandes groupées entre collègues, sans prise de tête.**
On ouvre une commande, tout le monde rejoint, on vote pour le resto, chacun
compose son panier, on envoie vers Uber Eats / Takeaway (ou on exporte), et
chacun rembourse le payeur en scannant un QR code.

`https://eat.fs0ciety.org`

## Démarrer
```bash
# Prérequis : Go 1.24+, Node 22+
cd frontend && npm ci && cd ..
make dev            # API http://127.0.0.1:8090  ·  front http://localhost:5173
```
ou en conteneur : `cp .env.example .env && docker compose up --build` → http://localhost:8090

Admin PocketBase : `/_/` (identifiants `OCC_ADMIN_EMAIL` / `OCC_ADMIN_PASSWORD`).

## Stack
PocketBase v0.36 (Go, SQLite, realtime) · React 19 · Vite · Tailwind v4 ·
TanStack Query · Docker · Coolify.

## Documentation
* [CLAUDE.md](CLAUDE.md) — règles du projet & mémoire de l'équipe
* [Architecture & API](docs/ARCHITECTURE.md)
* [Design system « Ember »](docs/DESIGN_SYSTEM.md)
* [Workflow](docs/WORKFLOW.md) · [Déploiement Coolify](docs/DEPLOYMENT.md) · [Roadmap](docs/ROADMAP.md)
* [Décisions (ADR)](docs/adr/)
