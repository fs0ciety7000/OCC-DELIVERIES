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

## Mise à jour 1 — 2026-10-09 : liens avec montant, QR statiques

### Problèmes remontés
* « Un QR Wero / Bancontact Pay varie avec le montant » : le QR « recevoir »
  téléversé par le payeur est **statique** (sans montant, ou pire : capture d'un
  QR généré pour un autre montant). Faux pour des parts individuelles.
* « Les liens Revolut ne s'affichent pas » : cause racine côté front/profil,
  pas côté API (`/payments/{id}/qr` renvoyait bien `link`) :
  1. le formulaire du profil refusait tout lien sans `https://` (bouton
     « Enregistrer » désactivé) — or on colle typiquement `revolut.me/jdoe` ou
     `@jdoe` : le lien n'était **jamais enregistré** ;
  2. un lien Revolut enregistré n'avait pas de montant (seul paypal.me en
     recevait) et vivait derrière une tuile générique « Lien de paiement »
     en avant-dernière position, jamais présélectionnée (Wero l'était) ;
  3. `PaymentQR` était mis en cache 5 min sans relecture : un moyen ajouté par
     le payeur après coup restait invisible (profil privé, pas de realtime).

### Recherche (formats de demande **à montant** déclenchables par un tiers)
| Moyen | Verdict | Format retenu | Sources |
|---|---|---|---|
| **QR EPC069-12** (virement SEPA) | ✅ universel, montant + communication | inchangé | EPC069-12 « Quick Response Code: guidelines to enable data capture for the initiation of a SCT » (European Payments Council) ; lu par les apps KBC, BNP Paribas Fortis, ING, Belfius, Argenta… qui proposent le virement instantané. Wero vit dans ces mêmes apps. |
| **PayPal.me** | ✅ officiel | `https://paypal.me/<nom>/<montant><DEVISE>` (ex. `/12.40EUR`) | PayPal, « What is PayPal.Me? » — *« Just add the amount you want to request to the end of your link… PayPal.Me/DiaRusso/25AUD »* — https://www.paypal.com/us/cshelp/article/what-is-paypalme-help432 |
| **Revolut** (revolut.me) | ✅ confirmé dans le code de la page revolut.me (pas de doc publique) | `https://revolut.me/<revtag>?amount=<centimes>&currency=EUR&note=<texte>` | Bundle JS de https://revolut.me (consulté le 2026-10-09) : lit `amount` (`parseInt`), `currency`, `note` dans l'URL ; champ montant de type `money-fractional` (unités mineures) et construit lui-même ses liens de partage avec ces paramètres. Corroboré : https://stackoverflow.com/questions/67294510 (réponse 2026 `?amount=100&currency=EUR&note=…`). Aide Revolut « Requesting money » : lien revolut.me générique ou avec montant fixé dans l'app. Réserve : si l'app Revolut intercepte le lien, le pré-remplissage n'est pas garanti → l'UI demande de vérifier le montant et garde la copie du montant. La forme `/<revtag>/eur5/<note>` est signalée comme cassée dans l'app : non utilisée. |
| **Wise Business** (open link) | ✅ officiel | `https://wise.com/pay/business/<nom>?amount=…&currency=EUR&description=…` (via le lien libre) | Wise, « How to use your open link to get paid through your website » — https://wise.com/help/articles/5qGvWQuTiX0RSSvxWvKcBC |
| **Wisetag** (Wise perso) | ❌ aucun paramètre documenté | lien libre tel quel | Wise, « What's a Wisetag and how do I use it? » — https://wise.com/help/articles/6DtiR7Ugdp7hfoKJHfRfvJ |
| **Wero** | ❌ demande créée dans l'app du bénéficiaire uniquement (« send or request money… with just a phone number ») ; aucun lien/QR tiers | identifiant + montant + communication à copier | https://wero-wallet.eu/send-money , https://wero-wallet.eu/ |
| **Bancontact Pay** (ex-Payconiq) | ❌ les demandes de paiement P2P (« leur app s'ouvre avec tous les détails préremplis ») et les QR avec montant sont **générés dans l'app du bénéficiaire**, chiffrés, valables 60 jours ; aucun format public | idem Wero | https://www.bancontact.com/fr/consommateur/payments/entre-amis |
| **Lydia / Sumeria** | ❌ aucun format public confirmé | lien libre tel quel (montant à saisir) | — |

### Décision
* Profil : champs structurés `revolut_tag` et `paypal_me` (migration
  `1760000013`, qui y déplace les anciens `payment_link` reconnaissables) ;
  `payment_link` reste pour les autres services ; normalisation tolérante
  (`@jdoe`, `revolut.me/jdoe`, sans `https://`) dans `domain/payout.go`.
* `PaymentQR.links[]` : liens construits **par paiement** côté serveur avec le
  montant exact (et la communication quand le format a un champ note),
  `amountPrefilled` explicite ; jamais de format inventé.
* `payments.method` gagne `revolut` et `paypal` (une tuile par wallet ; le
  payeur voit le moyen déclaré).
* Ordre des tuiles par utilité : QR EPC → liens pré-remplis → lien libre →
  Wero / Bancontact Pay → espèces → plus tard ; sur mobile, les liens
  pré-remplis passent devant le QR (on ne scanne pas son propre écran ; le QR
  reste accessible via « Afficher le QR pour un collègue », IBAN / montant /
  communication toujours copiables).
* **QR personnel statique : conservé mais rétrogradé**. On ne le masque pas
  (seul moyen Wero pour un payeur qui n'a pas donné son numéro, et utile
  pour scanner depuis un autre téléphone), mais il est **toujours** précédé de
  « QR sans montant : saisis 12,40 € dans l'app » et replié derrière
  « Afficher son QR personnel (sans montant) » dès qu'un identifiant existe.
  Les aides du profil recommandent l'IBAN / le numéro plutôt que ce QR.
* `PaymentQR` relu toutes les 60 s tant que la part est en attente (et au
  retour sur l'onglet) ; le payeur voit une carte « Tes moyens de
  remboursement » (ou l'alerte « Aucun moyen renseigné ») avec un lien vers
  son profil.
