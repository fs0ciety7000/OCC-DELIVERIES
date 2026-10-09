# ADR 0002 — Intégration Uber Eats / Takeaway par adaptateurs

* Statut : accepté — 2026-10-09

## Contexte
On veut « envoyer » la commande groupée vers Uber Eats ou Takeaway. Aucun des
deux n'expose d'API publique permettant à un tiers de remplir le panier d'un
client : les API Uber (Eats Marketplace, Direct) et JET Connect sont réservées
aux restaurants/partenaires, sous contrat. Le scraping viole leurs CGU.

## Décision
* Interface `providers.Provider` côté Go ; chaque fournisseur produit un
  `Dispatch` : deep link vers le restaurant, instructions pas-à-pas
  (ex. lancer une *commande groupée Uber Eats*), récap consolidé copiable.
* Sorties complémentaires : export CSV/TXT/JSON et script téléphone.
* Menus : saisis dans l'admin, importés en JSON (`/api/occ/admin/import`) ou,
  plus tard, synchronisés par un adaptateur partenaire.

## Conséquences
* Le jour où un accès partenaire est obtenu, on ajoute un adaptateur
  (`Dispatch` qui crée réellement la commande) sans toucher au front ni au schéma.
* Le prix affiché peut différer légèrement de celui de la plateforme ; l'UI le
  signale (« prix indicatifs »).

## Mise à jour — 2026-10-09 : Deliveroo, weloveat, `menusync`
* Deux adaptateurs de même nature (deep link + guide + récap) : `deliveroo` et `weloveat`
  (plateforme belge). Aucune n'offre d'API publique de panier.
* Les **menus** peuvent être lus par l'outil séparé `menusync` (pages publiques, API anonyme de
  la SPA weloveat, sites satellites Takeaway) : usage ponctuel par un admin, robots.txt appliqué,
  requêtes espacées, arrêt sur 403/anti-robot, aucun contournement. Le serveur ne fait jamais
  ces requêtes ; les données passent par l'import admin après relecture.
