# ADR 0001 — Stack : PocketBase (Go) + React, un seul conteneur

* Statut : accepté — 2026-10-09

## Contexte
Plateforme de commandes groupées entre collègues : beaucoup de **temps réel**
(membres, votes, paniers, paiements), petit volume (dizaines d'utilisateurs
simultanés par party, centaines au total), déploiement self-hosted sur Coolify
(`eat.fs0ciety.org`), une petite équipe.

## Options
1. **PocketBase (SQLite) étendu en Go** + SPA React.
2. Postgres + API Node (NestJS/Hono) + WebSocket + auth maison/Lucia + SPA.
3. Supabase self-hosted.

## Décision
Option 1.
* Auth (email, OAuth2), realtime SSE, règles d'accès, fichiers, admin UI et
  migrations **inclus** → on passe le temps sur le produit.
* Utilisé **comme framework Go** (pas en JS hooks) : logique métier typée,
  testable unitairement (`internal/domain` sans dépendance), binaire unique.
* Un seul conteneur, un seul volume (`pb_data`) : déploiement et sauvegarde
  triviaux sur Coolify.
* SQLite WAL tient très largement la charge visée.

## Conséquences
* Scalabilité horizontale limitée (un nœud). Si besoin un jour : migrer les
  données vers Postgres ; `internal/domain` et le contrat `/api/occ` restent.
* Les montants sont stockés en centimes `int` (pas de float).
* PocketBase est épinglé en **v0.36.6** (dernière série compatible Go 1.24) ;
  les montées de version passent par une PR dédiée.
