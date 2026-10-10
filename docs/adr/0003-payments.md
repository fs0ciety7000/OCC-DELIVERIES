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

## Mise à jour 2 — 2026-10-10 : « Encaisser » côté payeur, recherche d'un autre système

### Demande
« Créer les codes QR avec montant (sur la vue de celui qui a payé) destinés à
chaque membre. Paiement par PayPal ? Trouver un autre système ? »

### Décision — QR à montant présentés par le payeur
* `GET /api/occ/parties/{id}/payments/qr` (**payeur seulement** : ni l'hôte non
  payeur, ni un débiteur, ni un non-membre) renvoie, pour chaque part à
  encaisser, le payload **EPC069-12** et les liens à montant, construits par
  les **mêmes** fonctions que `/payments/{id}/qr` (`loadPayout().qrFor()`).
  Aucune donnée nouvelle ne sort : le payeur ne lit que ses propres
  coordonnées et des montants que tous les membres voient déjà.
* PayingStep (payeur) : carte « Encaisser » (une carte par collègue, statut
  temps réel, QR, « Confirmer la réception ») + mode « Présenter » plein écran
  (QR géant, montant, balayage / flèches, Wake Lock détecté, conseil
  « luminosité au maximum » — le Web ne peut pas régler la luminosité).
* Deux natures de QR, au choix :
  * **Virement** (EPC) : à scanner **dans l'app bancaire** (KBC, BNP Paribas
    Fortis, ING, Belfius, Argenta…), montant et communication `OCC <code>
    <nom>` pré-remplis. Reste le moyen recommandé : universel en Belgique,
    gratuit, instantané si la banque le propose ;
  * **lien Revolut / PayPal.me à montant** encodé tel quel dans un QR : scanné
    avec l'appareil photo, il ouvre la page/l'app avec le montant. Ce n'est pas
    un nouveau format : c'est le lien documenté en mise à jour 1. Les liens
    sans montant (lien libre, Wisetag, Lydia) ne sont **pas** proposés en QR.

### Recherche « autre système » (Belgique, octobre 2026)
Question : quels moyens P2P permettent à une **application tierce** de
fabriquer une demande **à montant pré-rempli** que le débiteur ouvre ou scanne ?

| Moyen | Verdict | Pourquoi | Sources |
|---|---|---|---|
| **QR EPC069-12** (virement SEPA) | ✅ en place | Format public de l'EPC ; lu par les apps bancaires belges. Seul QR à montant universel et gratuit. | EPC069-12 (European Payments Council) ; mise à jour 1 |
| **PayPal.me** | ✅ en place (lien + QR du lien) | `paypal.me/<nom>/<montant>EUR` documenté. Le « QR PayPal » de l'app est un QR personnel où le payeur **saisit** le montant : pas de format tiers avec montant ; on encode donc le lien PayPal.me. Compte PayPal requis des deux côtés, frais possibles hors « amis et famille ». | https://www.paypal.com/us/cshelp/article/what-is-paypalme-help432 ; https://qwac.paypal.com/us/brc/article/how-customers-pay-via-qr-codes (« the customer scans the code, enters the amount ») |
| **Revolut** (revolut.me) | ✅ en place (lien + QR du lien) | `?amount=<centimes>&currency=EUR&note=` (mise à jour 1). L'aide Revolut décrit en plus des liens à montant fixe **créés dans l'app** (non générables par un tiers). | https://help.revolut.com/help/adding-money/with-money-from-friends-or-relatives/requesting-money/ |
| **Wise** | ⚠️ inchangé | Compte perso : « Request → Anyone » crée un lien (QR Wisetag) **dans l'app**, valable 30 jours, sans format public ; seul le lien Business ouvert a `?amount=` (déjà géré). | https://wise.com/help/articles/2WvlZST6DiDMUBhyl1N4zM/how-do-i-request-money ; https://wise.com/help/articles/4qr3kkvIQlHNiD8BegEB4u/getting-paid-to-your-wise-business-by-payment-link |
| **Wero** (EPI) | ❌ pour du P2P | Demandes et QR « Recevoir » créés dans l'app du bénéficiaire (KBC : « Receive payment »). Les QR/liens **à montant générables hors app** n'existent que côté **commerçant** : p. ex. Buckaroo « Wero Invoice QR » (`wero-qr.buckaroo.io/invoice?storeId=…&amount=…`, contrat marchand) et l'API « Wero Merchant Payment » d'ABN AMRO (accès anticipé, Pays-Bas). Revient à l'option écartée (PSP, frais, argent via un compte marchand). | https://www.kbcbrussels.be/retail/en/products/payments/self-banking/wero.html ; https://wero-wallet.eu/be-fr ; https://docs.buckaroo.io/v2/docs/invoice-qr ; https://developer.abnamro.com/api-products/wero-merchant-payment/overview ; https://banking.vision/wero-entschluesselt |
| **Bancontact Pay** (ex-Payconiq) | ❌ | Toujours : QR et demandes de paiement P2P générés **dans l'app** du bénéficiaire, chiffrés, valables 60 jours ; nouvelle « Cagnotte » (lien partagé, montant fixe ou libre) également créée dans l'app. Les API documentées (Checkout.com, Adyen, Worldline…) sont marchandes. | https://www.bancontact.com/fr/consommateur/payments/entre-amis ; https://www.checkout.com/docs/payments/add-payment-methods/bancontact/payment-setup-api |
| **Tikkie** (ABN AMRO) | ❌ en Belgique | Exige un compte de paiement **néerlandais** (numéro belge accepté, pas l'IBAN belge) ; paiement via iDEAL. | https://www.tikkie.me/nl/hulp/veelgestelde-vragen ; https://webwoordenboek.nl/kenniscentrum/is-tikkie-europees |
| **Lydia / Sumeria** | ❌ | Orientés France ; aucun format public de demande à montant trouvé. Lien libre conservé tel quel (montant à saisir). | https://sumeria.eu/documents/tcs/fr/12032026/2-sumeria/2-3-sumeria-annexe-tarifs-et-limites-fr-240426.pdf |
| **Liens « demander de l'argent » des banques** (KBC, BNP Paribas Fortis, ING, Belfius) | ❌ | Ce sont les demandes **Wero** intégrées aux apps (BNP : demande à un contact via son numéro de mobile). Aucune URL publique. | https://www.bnpparibasfortis.be/en/public/individuals/daily-banking/payments/mobile-payments/wero ; page KBC ci-dessus |
| **Mollie / Stripe payment links** | ❌ (option écartée maintenue) | Liens à montant parfaits techniquement, mais compte marchand, KYC, frais par transaction et l'argent transite par la plateforme. | ADR 0003 « Option écartée » |
| **SEPA Request-to-Pay (SRTP)** | ⏳ pas utilisable | Schéma EPC (rulebook v4.0 depuis le 5/10/2025, API inter-prestataires v1.0, vagues d'homologation 2026) en **adoption précoce** ; aucune banque belge n'expose de demande SRTP à des particuliers ni à une app tierce non agréée. À surveiller (ce serait le « Wero à montant » standard). | https://ecovis.lt/?p=8805 (rulebook v4.0) ; présentation EPC « SRTP Scheme » d'octobre 2025 (EPC145-25, « early adoption ») ; https://www.ecb.europa.eu/paym/groups/erpb/shared/pdf/25th-ERPB%20meeting%20on%2018%20June%202026/Statement.pdf ; https://www.vixio.com/insights/pc-epc-finalises-api-specifications-sepa-request-pay-interoperability |

### Conclusion
Aucun format **nouveau** et légitime n'a été trouvé pour du P2P belge : on
n'invente rien. Recommandation affichée : QR **virement** (EPC) d'abord — il
remplace dans les faits la « demande Wero à montant » puisque les apps qui
portent Wero le lisent —, puis Revolut / PayPal.me à montant pour ceux qui les
utilisent ; Wero / Bancontact Pay restent « identifiant + montant à saisir ».
À réévaluer si SRTP arrive dans les apps belges ou si EPI publie un format de
demande P2P ouvert aux tiers.

## Mise à jour 3 — 2026-10-10 : retrait de Wero et Bancontact Pay

### Décision
Demande utilisateur : « On peut retirer les options Bancontact Pay et Wero
directement pour éviter confusion. » Les deux wallets sont **retirés** de
l'application (profil, tuiles, `PaymentQR`, guides, QR personnels).

### Pourquoi
* Ni Wero ni Bancontact Pay ne permettent à un tiers de pré-remplir un
  **montant** (mises à jour 1 et 2) : la tuile n'offrait qu'un identifiant à
  copier et un montant à saisir à la main, ou un QR personnel **statique**
  source d'erreurs (« un QR Wero varie avec le montant »).
* Le **QR virement EPC** fait le même travail en mieux : il est lu par les apps
  bancaires belges (KBC, BNP Paribas Fortis, ING, Belfius, Argenta…) — celles
  où vit justement Wero — avec montant **et** communication pré-remplis, en
  virement instantané si la banque le propose. Les liens Revolut / PayPal.me à
  montant couvrent les autres usages.
* Deux tuiles « montant à saisir » à côté de tuiles « montant pré-rempli »
  brouillaient le message ; la tuile virement s'intitule désormais
  « Virement (QR) — Scanne le QR avec ton app bancaire (montant déjà rempli) ».

### Ce qui change
* `payout_profiles` : champs `wero_id`, `bancontact_phone`, `wero_qr`,
  `bancontact_qr` **supprimés** (migration `1760000021`, idempotente) et les
  images QR téléversées **effacées du stockage** (vignettes comprises) : ce
  sont des données personnelles (numéros de mobile) qui ne servent plus.
  Le `down` recrée les champs vides (les données ne sont pas restaurables).
  Un ancien client qui envoie encore ces champs les voit ignorés.
* `PaymentQR` perd `wero` / `bancontact` ; `methods` ne les propose plus ;
  `GET /api/occ/payments/{id}/wallet-qr/{kind}` est supprimé.
* `POST /payments/{id}/action` refuse `declare` avec `wero` / `bancontact`
  (400 « Wero et Bancontact Pay ne sont plus proposés… »).
* **Paiements existants** : `payments.method` garde les valeurs `wero` et
  `bancontact` (rien n'est réécrit) ; les listes de paiements, l'historique,
  les notifications et e-mails affichent toujours « Wero » / « Bancontact
  Pay » pour ces anciens paiements, sans tuile. Le payeur peut toujours les
  confirmer ou les réinitialiser.

### À réévaluer
Si EPI publie un format de demande Wero **à montant** ouvert aux tiers, ou si
SEPA Request-to-Pay arrive dans les apps belges (mise à jour 2).
