# ADR 0003 — Remboursements : QR EPC + déclaratif, sans PSP

* Statut : accepté — 2026-10-09

## Contexte
Une personne avance la commande ; les autres la remboursent. Pas de volonté de
manipuler de l'argent (licence, frais, KYC).

## Décision
* **QR EPC069-12** (« QR virement SEPA ») généré côté serveur avec l'IBAN du
  payeur, le montant exact et une communication `OCC <code> <nom>`. Lu par la
  quasi-totalité des apps bancaires belges et européennes.
* **Wero** et **Bancontact Pay** (ex-Payconiq, renommé le 16/03/2026) : les
  deux wallets majoritaires en Belgique. Aucun n'offre de demande de paiement
  P2P déclenchable par un tiers ; on stocke donc l'identifiant du payeur
  (mobile / e-mail Wero, mobile Bancontact Pay) et, optionnellement, l'image
  de son QR « recevoir », affichés avec le montant et la communication à copier.
* Lien de paiement optionnel du payeur (PayPal.me, Revolut…).
* Choix « espèces » ou « je paierai plus tard ».
* Workflow déclaratif : le débiteur déclare, le payeur confirme. La party se
  clôture quand tout est confirmé.
* L'IBAN est dans `payout_profiles` (privé) et ne sort que dans le payload QR du
  payeur concerné, visible des seuls membres de la party.

## Option écartée (pour l'instant)
Encaisser via l'API marchand Bancontact Pay ou Wero e-commerce (via un PSP type
Mollie/Stripe) puis reverser au payeur : demande un contrat marchand, des frais
et fait transiter l'argent par la plateforme. À reconsidérer pour une offre
« entreprise ».

## Conséquences
Aucun flux financier ne transite par la plateforme ; pas de rapprochement
bancaire automatique (possible plus tard via la communication structurée).
