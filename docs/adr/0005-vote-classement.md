# ADR 0005 — Vote par classement (Borda tronqué)

* Statut : accepté — 2026-10-10

## Contexte
Le vote était un vote par approbation : chaque membre « like » autant de restos qu'il veut, le plus liké gagne.
Retour utilisateur : « un membre peut voter pour plusieurs restos mais avoir une préférence (Pitta Shop choix 1,
Akropolis choix 2, Buga Ramen en 3) ». L'approbation ne distingue pas un coup de cœur d'un « pourquoi pas ».

Contraintes : règle **explicable en une phrase** à l'écran, calculée **côté serveur** (l'hôte et le planificateur
des heures limites doivent élire le même gagnant), robuste aux bulletins partiels (on ne classe que ce qui nous va),
et rejouable hors ligne.

## Décision
* **Bulletin** = liste ordonnée des candidats qui me vont (1er choix d'abord), de 0 à K restos (K = nombre de
  candidats, ≤ 20). Stocké dans `votes` (une ligne par resto, `rank` 1..n, index unique `(party, user, rank)`).
* **Points** (Borda tronqué) : un resto classé au rang r rapporte **K − r + 1** points (au moins 1) ; non classé : 0.
  Avec 3 candidats : 1er choix 3 pts, 2e 2 pts, 3e 1 pt. Texte à l'écran : « Ton 1er choix vaut le plus de points ».
  - Classer un resto l'aide toujours, jamais plus que ceux placés au-dessus : pas d'intérêt à classer « contre »
    un resto, ne classer que ce qu'on accepte de manger reste la stratégie naturelle.
  - K fixe pendant le vote (les candidats ne changent qu'au salon) → les points affichés ne bougent pas sous les
    doigts. Si un candidat est désactivé en cours de vote, K diminue et un rang > K garde 1 point.
  - Écarté : barème fixe 3/2/1 (les rangs 4+ ne compteraient plus, alors que l'UI autorise de tout classer) ;
    vote alternatif / Condorcet (plus juste en théorie mais impossible à suivre en direct avec une jauge).
* **Gagnant** : plus de points ; égalité → plus de **1ers choix**, puis plus de **votants** (membres ayant classé le
  resto), puis meilleure note, puis nom (insensible à la casse), puis id. Sans aucun vote : note puis nom
  (comportement inchangé). Code unique : `domain.ComputeTally` (pur, testé en tables), utilisé par
  `/transition`, le planificateur (`applyTransition`) et `GET /tally`.
* **Écriture** : uniquement `PUT /api/occ/parties/{id}/ballot {"ranking": [...], "clientKey"}` qui remplace tout le
  bulletin en une transaction (supprime puis recrée : l'index unique de rang interdit les échanges ligne à ligne).
  Les rules d'écriture de `votes` passent à `nil` (serveur seul) ; la lecture reste ouverte aux membres pour le
  temps réel. Idempotent : même classement ou même `clientKey` que le bulletin stocké → 200 sans écriture.
* **Votes existants** (parties en cours de vote au déploiement) : rangés **par ordre de création** par membre
  (migration `1760000019`) — le premier resto liké devient le 1er choix. Plus simple et plus fidèle qu'un « poids
  égal » qui aurait demandé un second mode de calcul ; les parties terminées ont déjà leur gagnant.
* **Hors ligne** : action `ballot` dans la file (dernier état seulement par party, comme « prêt·e ») ; les anciennes
  entrées `vote` / `unvote` encore en file sont rejouées en relisant le bulletin et en le réécrivant (ajout en fin /
  retrait).

## Conséquences
* Le tableau des scores (points, jauges) vient du serveur (`Tally`), rechargé à chaque évènement temps réel `votes`.
* Un bulletin modifié génère n suppressions + m créations en temps réel : les clients invalident simplement
  `votes` et `tally` (TanStack dédoublonne).
* Un rejeu tardif d'un ancien bulletin (clé différente) peut écraser un bulletin plus récent fait ailleurs : la file
  ne garde que le dernier bulletin par party et rejoue au retour du réseau, avant toute nouvelle saisie.
