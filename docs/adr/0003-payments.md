# ADR 0003 — Remboursements : QR EPC + déclaratif, sans PSP

* Statut : accepté — 2026-10-09

## Contexte
Une personne avance la commande ; les autres la remboursent. Pas de volonté de
manipuler de l'argent (licence, frais, KYC).

## Décision
* **QR EPC069-12** (« QR virement SEPA ») généré côté serveur avec l'IBAN du
  payeur, le montant exact et une communication `OCC <code> <nom>`. Lu par la
  quasi-totalité des apps bancaires belges et européennes.
* Lien de paiement optionnel du payeur (PayPal.me, Revolut, Wero…).
* Choix « espèces » ou « je paierai plus tard ».
* Workflow déclaratif : le débiteur déclare, le payeur confirme. La party se
  clôture quand tout est confirmé.
* L'IBAN est dans `payout_profiles` (privé) et ne sort que dans le payload QR du
  payeur concerné, visible des seuls membres de la party.

## Conséquences
Aucun flux financier ne transite par la plateforme ; pas de rapprochement
bancaire automatique (possible plus tard via la communication structurée).
