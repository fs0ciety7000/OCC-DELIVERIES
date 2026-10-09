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
