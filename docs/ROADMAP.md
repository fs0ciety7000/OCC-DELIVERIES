# Feuille de route

## v0.1 — MVP (bootstrap)
- [x] Contrat d'architecture, design system, ADR, workflow, agents
- [x] Backend PocketBase/Go : schéma, rules, state machine, API métier, seed démo
- [x] Frontend : accueil, restaurants, menus, party (salon → vote → commande → récap → paiement), profil
- [x] Envoi Uber Eats / Takeaway (deep link + récap), export CSV/TXT/JSON, script téléphone
- [x] Remboursements : QR EPC SEPA, liens Revolut / PayPal.me avec montant, lien de paiement, espèces, plus tard (Wero / Bancontact Pay retirés : ADR 0003 mise à jour 3)
- [x] Docker mono-conteneur, CI GitHub Actions, guide Coolify

## v0.1.1 — Administration & données réelles
- [x] Rôle `admin` (bootstrap `OCC_ADMIN_EMAIL` / `OCC_ADMINS`, promotion depuis l'app, anti auto-promotion)
- [x] Panneau `/admin` : tableau de bord, restaurants (géocodage Nominatim), éditeur de menu (catégories, articles, options), commandes (annulation forcée), utilisateurs
- [x] Import JSON (objet ou tableau) et CSV (décimales françaises) avec aperçu / dry run, export JSON de sauvegarde
- [x] Restaurants réels de Mons embarqués (`migrations/data`), démo fictive retirée ; outil d'export Uber Eats / Takeaway (`/outils/export-menu.html`)
- [x] E2E indépendant des données (restaurants et articles choisis via l'API)

## v0.2 — Confort
- [ ] Notifications push Web (PWA) : « le vote est ouvert », « tout le monde est prêt »
- [ ] Échéances automatiques (clôture du vote / de la commande à `*_ends_at`)
- [ ] Favoris et « commander comme la dernière fois »
- [ ] Invités sans compte (lien magique, nom seul)
- [ ] Modération de l'hôte : retirer un membre, transférer l'hôte

## v0.3 — Données restaurants
- [x] Import de menus CSV / JSON avec aperçu (panneau `/admin`)
- [x] Flux `menusync` (Deliveroo, weloveat, sites Takeaway, schema.org) : normalisation des noms / cuisines / plats, dédoublonnage
- [x] **Synchronisation automatique** dans le serveur (cron nocturne, au démarrage, manuelle) : rapprochement avec les fiches existantes, verrouillage des modifications admin, plats retirés → indisponibles, restaurants obsolètes signalés, historique des exécutions — page `/admin/synchronisation`
- [ ] Synchronisation : suppléments weloveat par lots (étalés sur plusieurs nuits), alertes (e-mail) sur échec / blocage
- [ ] Import assisté : coller un lien Uber Eats / Takeaway → formulaire pré-rempli (côté serveur)
- [ ] Journal d'audit des actions admin ; upload d'images (cover / plats) depuis `/admin`
- [ ] Carte (MapLibre) des restaurants autour du bureau
- [ ] Horaires d'ouverture & disponibilité
- [ ] Adaptateur partenaire (Uber Direct / JET Connect) si accès obtenu — ADR 0002

## v0.4 — Équipe & entreprise
- [ ] Espaces d'équipe (bureaux, adresses de livraison enregistrées)
- [ ] Budgets / tickets restaurant, statistiques (qui doit quoi sur le mois)
- [ ] Rapprochement des remboursements (communication structurée belge `+++…+++`)
- [ ] Encaissement Bancontact Pay / Wero via API marchand (PSP) — option écartée en ADR 0003
- [ ] i18n (NL/EN)
