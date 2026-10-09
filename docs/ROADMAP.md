# Feuille de route

## v0.1 — MVP (bootstrap)
- [x] Contrat d'architecture, design system, ADR, workflow, agents
- [x] Backend PocketBase/Go : schéma, rules, state machine, API métier, seed démo
- [x] Frontend : accueil, restaurants, menus, party (salon → vote → commande → récap → paiement), profil
- [x] Envoi Uber Eats / Takeaway (deep link + récap), export CSV/TXT/JSON, script téléphone
- [x] Remboursements : QR EPC SEPA, Wero, Bancontact Pay, lien de paiement, espèces, plus tard
- [x] Docker mono-conteneur, CI GitHub Actions, guide Coolify

## v0.2 — Confort
- [ ] Notifications push Web (PWA) : « le vote est ouvert », « tout le monde est prêt »
- [ ] Échéances automatiques (clôture du vote / de la commande à `*_ends_at`)
- [ ] Favoris et « commander comme la dernière fois »
- [ ] Invités sans compte (lien magique, nom seul)
- [ ] Modération de l'hôte : retirer un membre, transférer l'hôte

## v0.3 — Données restaurants
- [ ] Import de menus assisté (CSV, coller un lien → formulaire pré-rempli)
- [ ] Carte (MapLibre) des restaurants autour du bureau
- [ ] Horaires d'ouverture & disponibilité
- [ ] Adaptateur partenaire (Uber Direct / JET Connect) si accès obtenu — ADR 0002

## v0.4 — Équipe & entreprise
- [ ] Espaces d'équipe (bureaux, adresses de livraison enregistrées)
- [ ] Budgets / tickets restaurant, statistiques (qui doit quoi sur le mois)
- [ ] Rapprochement des remboursements (communication structurée belge `+++…+++`)
- [ ] Encaissement Bancontact Pay / Wero via API marchand (PSP) — option écartée en ADR 0003
- [ ] i18n (NL/EN)
