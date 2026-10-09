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

## Mise à jour — 2026-10-09 (2) : synchronisation automatique dans le serveur
Ceci **remplace** la phrase « Le serveur ne fait jamais ces requêtes » de la mise à jour précédente.

**Contexte.** Les restaurants et leurs cartes doivent être fidèles et à jour sans travail
manuel : relancer `menusync` à la main puis importer n'était pas tenu dans la durée, et
l'import remplace tout le menu (il écrase les retouches admin et recrée les articles).

**Décision.** Le serveur relit lui-même les sources publiques (`internal/feedsync`, au-dessus
de `menusync`) et **réconcilie** au lieu d'importer :
* sources configurées en base (`sync_sources`, admin) — par défaut : les 10 sites satellites
  Takeaway de Mons, la page liste Deliveroo Mons, l'API anonyme de weloveat (Mons) ;
* exécution **planifiée** (`OCC_SYNC_CRON`, défaut 03:30 heure de Bruxelles, heure creuse),
  **au démarrage** si aucune n'a encore réussi, ou **manuelle** depuis `/admin/synchronisation` ;
* rapprochement par clé de source, lien plateforme, puis nom + position (les fiches curées
  sont reconnues, jamais dupliquées) ; mise à jour en place, transaction par restaurant ;
* aucune suppression : un plat absent devient indisponible, un restaurant que plus aucune
  source ne propose est marqué **obsolète** (badge admin), toujours visible ;
* les fiches **verrouillées** (toute modification admin les verrouille) ne sont jamais touchées.

**Garde-fous** (identiques à l'outil, non désactivables) : User-Agent identifié
`OCC-Deliveries-menusync/1.0 (+https://eat.fs0ciety.org)` ; **une requête à la fois** (une
seule exécution possible, verrou en mémoire + 409), ≥ 2,5 s (+ aléa) entre deux requêtes,
robots.txt appliqué (jamais d'appel aux `/api/` ni `graphql` de Deliveroo), 1 seul nouvel essai
sur 429/5xx avec `Retry-After`, **arrêt net** d'une source sur 403 ou page anti-robot
(statut « bloquée », les autres sources continuent, aucun contournement), cache disque
`pb_data/menusync-cache` de 20 h (une relance le même jour ne refait pas les requêtes),
données personnelles retirées des réponses weloveat avant le cache. Volume : ~150 requêtes par
nuit pour Mons (suppléments weloveat désactivés par défaut : 1 requête par plat). Coupure
immédiate : `OCC_SYNC_ENABLED=false` (plus aucune requête sortante, déclenchement manuel refusé).

**Conséquences.** Le serveur a désormais des requêtes sortantes (à autoriser si un pare-feu
sortant est ajouté). Les prix restent « indicatifs » (les plateformes appliquent souvent une
marge). Les CGU restent applicables : usage interne, volumes modestes, sources désactivables
une à une ; si une plateforme le demande ou se met à bloquer, on désactive sa source.

## Mise à jour — 2026-10-09 (3) : instantané Uber Eats (cartes partielles)
**Contexte.** Le connecteur officiel Uber Eats (MCP) répond depuis une session Claude, pas depuis le
serveur ; il est fortement limité et ne renvoie que 0 à 5 plats d'exemple par restaurant. Les
utilisateurs veulent pourtant voir tout de suite les restaurants Uber Eats qui livrent le bureau.

**Décision.** Le lead fige les résultats du connecteur dans `migrations/data/mons_ubereats.json`
(embarqué) ; une source `ubereats-snapshot` le relit à chaque synchronisation, **sans réseau**.
Les restaurants inconnus sont créés **actifs** avec `partial_menu = true` (catégorie « Aperçu »,
badge « Aperçu du menu », bandeau renvoyant à Uber Eats) et, sans coordonnées, au lieu par défaut
(`geo_approx = true`, distance masquée). Un restaurant déjà connu ne reçoit que son lien Uber Eats et
les note / avis / délai manquants — jamais de changement de menu. Une source à carte complète
(≥ 5 plats) remplace ensuite l'aperçu. Rapprochement par lien puis nom souple (`menusync.NameMatch`).

**Conséquences.** Pas de scraping d'Uber Eats ni de requête du serveur vers Uber Eats ; la fraîcheur
dépend des mises à jour du fichier (manuelles, `docs/DEPLOYMENT.md`). La commande sur ces restaurants
se fait via l'envoi « Uber Eats » (deep link + commande groupée Uber Eats), la carte de l'app n'étant
qu'un aperçu aux prix indicatifs.
