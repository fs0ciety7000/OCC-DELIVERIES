# OCC DELIVERIES — Design system « Ember »

Premium, chaleureux, nocturne. Pensé **mobile d'abord** (on commande depuis son
téléphone à 11h45) mais confortable sur desktop. Sombre par défaut, thème clair
complet.

## 1. Principes

1. **Le groupe avant tout** — on voit toujours qui est là, qui a voté, qui est prêt.
   Avatars colorés, compteurs live, micro-animations quand un collègue agit.
2. **Une action principale par écran** — un seul bouton `primary` (dégradé braise)
   visible ; le reste en `secondary`/`ghost`.
3. **Chiffres lisibles** — montants en `tabular-nums`, toujours formatés
   `Intl.NumberFormat('fr-BE', { style: 'currency', currency: 'EUR' })`.
4. **Calme, pas criard** — surfaces sombres en couches, une seule couleur d'accent,
   la couleur d'état n'apparaît que lorsqu'elle porte une information.
5. **Accessible** — contraste AA minimum, focus visible (`ring` braise),
   cibles ≥ 44 px, `prefers-reduced-motion` respecté, libellés ARIA en français.

## 2. Tokens (CSS custom properties → `@theme` Tailwind v4)

Définis dans `frontend/src/styles/tokens.css`. Les composants n'utilisent **que**
les tokens (jamais de hex en dur).

### Couleurs — thème sombre (défaut)
| token | valeur | usage |
|---|---|---|
| `--color-bg` | `#0A0A0D` | fond de page |
| `--color-surface` | `#121217` | cartes |
| `--color-elevated` | `#1A1A21` | popovers, sheets, inputs |
| `--color-border` | `rgb(255 255 255 / 0.08)` | séparateurs |
| `--color-border-strong` | `rgb(255 255 255 / 0.16)` | inputs focus/hover |
| `--color-fg` | `#F6F4EF` | texte principal |
| `--color-muted` | `#A3A1AB` | texte secondaire |
| `--color-subtle` | `#86838F` | placeholders, méta (≥ 4,5:1 sur surface) |
| `--color-brand` | `#FF6A3D` | braise — accent unique |
| `--color-brand-2` | `#FFB547` | ambre — fin du dégradé |
| `--color-brand-fg` | `#1A0B05` | texte sur braise |
| `--color-brand-ink` / `--color-brand-ink-2` | = brand / brand-2 | braise posée **en texte** (`.text-brand`, `.text-ember`) ; assombrie en clair pour l'AA |
| `--color-success` | `#3DD68C` | prêt, payé |
| `--color-warning` | `#F5B83D` | en attente |
| `--color-danger` | `#F26D6D` | erreurs, annulation |
| `--color-info` | `#6AA8FF` | infos |
| `--color-ubereats` | `#06C167` | marque fournisseur |
| `--color-takeaway` | `#FF8000` | marque fournisseur |
| `--color-deliveroo` | `#00CCBC` | marque fournisseur (teal Deliveroo) |
| `--color-weloveat` | `#113B3A` | marque fournisseur (teal profond, couleur *primary* du thème Material de weloveat.be ; leur `manifest.json` n'a pas de `theme_color`) |
| `--color-ubereats-ink` / `--color-takeaway-ink` / `--color-deliveroo-ink` | = marque | texte des badges fournisseurs (AA sur leur teinte à 14 %) |
| `--color-weloveat-ink` | `#6FD3C9` | teinte claire dérivée (le teal profond est illisible sur fond sombre) ; sert aussi de fond/bord du badge `weloveat` (8,0:1) |
| `--color-qr-bg` / `--color-qr-fg` | `#FFFFFF` / `#000000` | QR codes, identiques dans les deux thèmes |
| `--color-ink` / `--color-paper` | `#17151A` / `#FFFFFF` | texte posé sur une couleur d'avatar (choisi selon la luminance) |
| `--color-food-*` | crust, cheese, tomato, basil, chili, bun, patty, lettuce, rice, nori, salmon, broth, bowl, steel, gold, ink | illustrations culinaires SVG uniquement |

### Thème clair (`[data-theme="light"]`)
`bg #FAF8F4`, `surface #FFFFFF`, `elevated #F3F0EA`, `border rgb(20 16 12 / 0.08)`,
`fg #17151A`, `muted #5F5B66`, `subtle #6F6B77`, `brand #F2542D`, `brand-2 #F59E0B`,
`brand-ink #B93A17`, `brand-ink-2 #9A5B00`, `success #0B7A4B`, `warning #94600F`,
`danger #C03030`, `info #2560C0`, `ubereats-ink #036B4D`, `takeaway-ink #A84A00`, `deliveroo-ink #006B62` (5,1:1 sur elevated),
`weloveat-ink #113B3A` (= marque, 8,4:1).
En clair, `.text-brand` est redirigé vers `--color-brand-ink` (`styles/index.css`) :
la braise vive (3,4:1 sur blanc) reste réservée aux aplats, dégradés et icônes.

### Dégradés & effets
* `--gradient-ember: linear-gradient(135deg, var(--color-brand), var(--color-brand-2))`
* `--shadow-card: 0 1px 0 rgb(255 255 255 / .04) inset, 0 8px 24px -12px rgb(0 0 0 / .6)`
* `--shadow-glow: 0 0 0 1px rgb(255 106 61 / .35), 0 12px 32px -8px rgb(255 106 61 / .45)`
* Fond de page : léger halo radial braise en haut (`.app-aurora`) + grain SVG à 3 %.

### Typographie
| rôle | police | taille / interlignage |
|---|---|---|
| Display (titres, montants héros) | **Bricolage Grotesque Variable** | 40/44, 32/36, 24/30 — `font-weight 650–750`, `letter-spacing -0.02em` |
| Texte | **Inter Variable** | 16/24 corps, 14/20 secondaire, 12/16 méta |
| Chiffres | Inter `tabular-nums` | |

Polices auto-hébergées via `@fontsource-variable/*` (aucun appel externe).

### Espacements, rayons, élévation
* Échelle 4 px (Tailwind par défaut). Gouttière mobile 16 px, desktop 32 px.
* Rayons : `--radius-sm 10px` (badges, inputs), `--radius-md 14px` (boutons),
  `--radius-lg 20px` (cartes), `--radius-xl 28px` (sheets, héros), `full` (avatars, pills).
* Largeur max contenu `1200px` ; colonne de lecture `680px`.

### Mouvement (`motion`)
* Durées : 120 ms (hover), 200 ms (entrées), 320 ms (sheets).
* Courbe : `cubic-bezier(.2,.8,.2,1)` ; ressorts `stiffness 380, damping 30` pour les listes.
* Entrées de liste : fade + translateY 8 px, stagger 30 ms.
* Évènements live (vote, prêt, paiement) : *pulse* 600 ms sur l'avatar concerné.
* `prefers-reduced-motion` ou réglage « Animations réduites » → opacité uniquement (voir « Mouvement réduit »).

### Deux moteurs, deux rôles (ADR 0004)
| moteur | rôle | exemples |
|---|---|---|
| **`motion`** | UI fonctionnelle, déclarative, liée à l'état React | entrées de listes, sheets, `layout`, `AnimatePresence`, hover/tap |
| **GSAP 3.15** + `@gsap/react` (`useGSAP`) | moments « signature », timelines, SVG, trajectoires, physique | animations culinaires ci-dessous |

Règles GSAP : toujours `useGSAP({ scope })` (nettoyage auto), plugins
enregistrés une seule fois dans `src/lib/gsap.ts` (`Flip`, `MotionPathPlugin`,
`MorphSVGPlugin`, `Physics2DPlugin`, `SplitText`), import dynamique des scènes
lourdes (`React.lazy`), `gsap.matchMedia()` avec
`(prefers-reduced-motion: reduce)` → version statique. Animer uniquement
`transform` / `opacity` (et attributs SVG) ; 60 fps sur mobile milieu de gamme.

### Animations culinaires (« Food motion »)
Illustrations **SVG maison** (style flat, tokens de couleur, trait 2 px) dans
`src/components/food/` — pas d'images lourdes, pas de Lottie.

| moment | animation | technique |
|---|---|---|
| Accueil — héros | ingrédients flottants (tomate, basilic, piment, sushi, frite, feuille) en parallaxe lente autour du titre « Qu'est-ce qu'on mange ? » ; titre révélé lettre par lettre | timeline GSAP en boucle `yoyo`, `SplitText`, parallaxe au pointeur (desktop) |
| Chargement | `FoodLoader` : une pizza dont les parts se découpent puis se recomposent ; variante bol de ramen fumant | timeline SVG, `MorphSVG` pour la vapeur |
| Vote | cœur qui « explose » en mini-ingrédients de la cuisine du resto votée ; barre de score qui se remplit comme une jauge de sauce | `Physics2D` (particules), `motion` pour la barre |
| Restaurant gagnant | carte qui se retourne + emoji qui rebondit, couverts qui s'entrechoquent | timeline GSAP |
| Ajout au panier | la vignette du plat « vole » en arc jusqu'au sac de commande, qui se gonfle | `Flip` + `MotionPath` |
| « Je suis prêt » | cloche de service *ding* (rotation + onde) sur l'avatar | timeline courte |
| Tout le monde prêt | petite vapeur qui monte au-dessus de la AvatarStack | `MorphSVG` |
| Envoi (dispatch) | scooter de livraison qui parcourt une route pointillée jusqu'au bureau | `MotionPath` + `drawSVG`-like `strokeDashoffset` |
| Paiement confirmé | pièce/billet qui tombe dans une tirelire-burger | timeline GSAP |
| Party clôturée | pluie d'emojis 🍕🍣🍔🥟🌮🍜 avec gravité, ~1,5 s, une seule fois | `Physics2D` sur un calque `pointer-events:none` |
| États vides | assiette vide avec fourchette qui tapote, oscillation douce ; **panier vide** : sac de livraison qui se balance + miette qui tombe (`EmptyBag`) | boucle GSAP lente |
| Panier vivant (Commande) | un plat ajouté par un·e collègue (realtime) vole du menu — ou du bord droit — jusqu'à **son avatar** dans l'en-tête (`[data-party-avatars]`), qui fait un *bump* ; avatar hors écran → vers le total du groupe ; mes ajouts gardent le vol vers le sac. Barre collante : ligne « Total du groupe · n paniers » dont le montant **compte** jusqu'à sa valeur (`AnimatedMoney`, centimes entiers, `tabular-nums`, valeur finale en `sr-only`) | `MotionPath` (`ColleagueFlight.ts`, départs espacés ≥ 220 ms, 3 vols max, au-delà bump seul), tween GSAP arrondi au centime |
| Changement d'étape | remplissage du `Stepper` qui grandit (scaleX, 500 ms) + point ambre de l'étape courante qui pulse une fois ; contenu en fondu + glissement 28 px **selon le sens** (avancer → vers la gauche, « Rouvrir » → vers la droite) ; annonce `aria-live` « Étape 3 sur 5 : Commande » ; focus posé sur le contenu s'il a été perdu | `motion` (`StepTransition`, `AnimatePresence custom`) |
| Attente (vote) | j'ai voté, d'autres non : carte « On attend les autres… » avec un **livreur** qui regarde sa montre, tapote du pied, bulle « … » (`WaitingRider`, boucle ≈ 3 s) ; compteurs « 2/4 ont voté », « 3/5 prêts », « n prêts » qui pulsent à chaque changement (`PulseOnChange`) | timeline SVG (`svgOrigin`), scale 1 → 1,14 |
| Attente (salon) | « En attente des collègues… » (seul·e) / « En attente du lancement du vote… » (invité·e) : tomate, basilic, piment qui sautillent (`WaitingDots`) | boucle GSAP `stagger` |
| Commande envoyée | carte `success/6` pleine largeur en tête du récap et des remboursements : scooter qui traverse une route (marquage qui défile, lignes de vitesse), repasse et se gare, puis « Commande envoyée via Uber Eats à 12:03 · arrivée estimée ~35 min » (milieu de `eta_min`/`eta_max` arrondi à 5 min, masqué sans délai ; téléphone / export : « Commande passée par téléphone » / « Commande exportée »). **Une fois par party** et par appareil (`localStorage occ-dispatched-<id>`), ≈ 2 s, bouton « Passer l'animation » ; côté hôte, attend la fermeture de la sheet d'envoi ; ensuite carte statique | timeline GSAP lazy (`DispatchedScene`) |
| Révélation du gagnant | vote → commande (seulement après un vrai vote, ≥ 2 candidats) : la carte « Le resto gagnant » se retourne et grandit, l'emoji rebondit, gerbe d'ingrédients, effacement à ~2 s ; calque `pointer-events: none`, décoratif (le toast annonce déjà) ; une fois par party (`occ-winner-<id>`) | timeline + `Physics2D` lazy (`WinnerReveal`) |

Budget : chaque scène ≤ 6 Ko gzip, aucune animation bloquante (> 1,5 s) sur
un parcours critique, toutes interruptibles. Mesuré (2026-10-09) : `DispatchedScene` 1,4 Ko,
`WinnerReveal` 1,0 Ko, `WaitingRider` 1,0 Ko gzip (chunks lazy) ; petits composants de party
≈ + 3,9 Ko dans le chunk `PartyPage` (importés par leur fichier, **pas** via le barrel
`components/food/index.ts`, que le shell importe : ils finiraient dans le bundle initial).

### Micro-interactions
| interaction | rendu | notes |
|---|---|---|
| Appui sur un `Button` | enfoncement `scale(var(--press-scale))` = 0,97, `--press-duration` 120 ms | utilitaire `press` (`styles/index.css`) dans `buttonClass` ; aucun effet en mouvement réduit |
| Vote | cœur qui se **remplit comme un liquide** (vague qui monte, 550 ms) + pop 1,22 (`LiquidHeart`), puis la gerbe `VoteBurst` | vide → retombe en 250 ms |
| « Je suis prêt·e » | la coche se **dessine** (`stroke-dashoffset`) dans un cercle qui pop (`DrawCheck`) | |
| Toasts | entrée 320 ms sur la courbe Ember (`[data-sonner-toast]`) | coupée en mouvement réduit |
| Haptique | `navigator.vibrate` : vote 10 ms, prêt `[15, 40, 15]`, paiement confirmé `[20, 60, 20]` (payeur qui confirme, ou ma part confirmée en direct) | `lib/haptics.ts` : garde sur l'API, `try/catch`, jamais seule porteuse d'information, coupée par « Animations réduites » |

### Mouvement réduit
`prefers-reduced-motion` **ou** le réglage **« Animations réduites »** (Profil → Mes infos →
Apparence, interrupteur désactivé et coché si le système le demande déjà) coupent toutes les
animations non essentielles. Réglage stocké comme le thème (`localStorage occ-motion`,
`data-motion="reduced"` posé sur `<html>` dès `index.html`) et exposé par `useMotionPref()`
(`lib/motionPref.ts`) ; branché partout : `withMotion` / `prefersReducedMotion` (GSAP),
`MotionConfig reducedMotion="always"` (`motion`), variantes Tailwind `motion-reduce:` /
`motion-safe:` redéfinies, mêmes règles CSS globales que le média, haptique. Versions
statiques : carte « Commande envoyée » posée, pas de révélation ni de vol, compteurs qui
sautent à la valeur finale, cœur / coche à l'état final.

## 3. Composants (`frontend/src/components/ui`)

| composant | variantes | notes |
|---|---|---|
| `Button` | `primary` (dégradé braise + glow), `secondary` (surface + bordure), `ghost`, `danger`; tailles `sm` `md` `lg`, `icon` | état `loading` (spinner), `asChild` non requis |
| `Card` | `default`, `interactive` (hover lift), `selected` (bordure braise) | |
| `Badge` | `neutral`, `brand`, `success`, `warning`, `danger`, `info`, `ubereats`, `takeaway`, `deliveroo`, `weloveat` | pill 12 px ; un ancien paiement Wero / Bancontact Pay s'affiche en `neutral` (variantes et tokens de marque retirés, ADR 0003 maj 3) |
| `Avatar` / `AvatarStack` | tailles 24/32/40/56 ; anneau `ready` vert | initiales sur `user.color` si pas d'image |
| `Input`, `Textarea`, `Field` | label + aide + erreur | |
| `Sheet` | bottom sheet mobile / dialog centré desktop | focus trap, Échap |
| `Stepper` | étapes de la party (Salon → Vote → Commande → Récap → Paiement) | remplissage animé (scaleX) + point ambre de l'étape courante ; voir « Changement d'étape » |
| `Money` | `<Money cents={1250} />` | tabular-nums |
| `EmptyState` | illustration emoji + titre + action | |
| `Skeleton` | shimmer | |
| `QuantityStepper` | − 1 + | |
| `Countdown` | mm:ss | |
| `Toast` | via `sonner`, thème sombre | |
| `QRCodeCard` | QR généré (`value`) + montant + bouton copier | `qrcode.react`, fond blanc obligatoire (lisibilité scanners), `fgColor="currentColor"` sur `--color-qr-fg` |
| Tuiles de paiement (`MethodTiles`) | une tuile ≥ 80 px par moyen, dans l'ordre de `PaymentQR.methods` (utilité) : Virement (QR), Revolut, PayPal, Lien, Espèces, Plus tard (Wero / Bancontact Pay retirés : pas de montant pré-rempli possible). **Mobile** (< 1024 px) : les liens à montant pré-rempli passent devant le Virement (QR) (on ne scanne pas son propre écran) | `role="radiogroup"` ; sous-titre `muted` 12 px dans le nom accessible : « Scanne le QR avec ton app bancaire (montant déjà rempli) » (virement), « Montant pré-rempli » / « Montant à saisir » (liens). Revolut/PayPal : wordmark texte sur `fg/8` (pas de logo ni de couleur de marque). **Virement (QR)** : texte « Ton app bancaire (KBC, BNP Paribas Fortis…) lit ce QR… C'est la même app que celle où vit Wero » ; desktop = QR EPC 240 px « Ouvre ton app bancaire et scanne » ; mobile = encart `info` + bouton « Afficher le QR pour un collègue » (`aria-expanded`) ; IBAN, montant, communication toujours copiables. **Liens** : bouton `primary` `lg` pleine largeur « Payer 12,40 € avec Revolut » (lien externe) si `amountPrefilled`, sinon `secondary` « Ouvrir … » + encart `warning` « saisis 12,40 € ». **Aucun faux deep link** (pas d'API P2P tierce) |
| Carte « Tes moyens de remboursement » (payeur) | | pastilles des moyens visibles par les collègues (`MethodMark` + libellé), ou encart `warning` « Aucun moyen renseigné… » ; lien `secondary` `sm` vers *Mes infos* |
| `Segmented`, `Chip`, `CopyButton` | | radiogroup en pills ; filtre `aria-pressed` ; copie + toast |
| Bandeau « Commande en cours » (`ResumeBanner`, `features/party/ActiveParties.tsx`) | `header` (pastille desktop ≥ 768 px dans l'en-tête : point live braise + titre tronqué 176 px + statut ≥ 1024 px + « Reprendre → » en `text-brand`, fond `brand/10`, bord `brand/30`, h 36 px) ; `dock` (mobile : `aside` « Commande en cours » fixé au-dessus de la tab bar, carte `elevated/95` floutée bord `brand/30`, vignette resto 40 px, sur-titre braise en capitales 11 px + titre + « Étape 3/5 · Commande », bouton `secondary sm` « Reprendre ») | toute la carte est un lien vers la party (nom accessible « Reprendre la commande « X » — statut ») ; ≥ 2 commandes → bouton qui ouvre une `Sheet` « Mes commandes en cours » (lignes 56 px : vignette, titre, badge statut, resto). Masqué dans la party, sur `/j/…`, l'auth, et sur l'accueil quand `ResumeHero` s'y affiche. Quand le dock est là, `.has-resume-dock` porte `--tabbar-h` à 128 px en mobile : barres collantes et bas de page remontent d'autant. Point live : `animate-ping`, coupé par `motion-reduce` |
| `ResumeHero` | | accueil, **exactement une** commande en cours (ex. retour après connexion) : carte `rounded-xl` bord `brand/40`, fond `brand/10`, `shadow-glow`, sur-titre « Ta commande t'attend », titre display 20 px, étape + resto ; remplace la section « Mes commandes en cours » |
| Carte d'historique (`HistoryCard`, profil) | | vignette resto 56 px, nom du resto en titre (`h3`), date longue (« ven. 9 octobre 2026 ») + titre de la commande en `muted` 12 px, badge de **mon** remboursement (`success` Remboursé, `info` Déclaré · Wero, `warning` À rembourser, `brand` Tu as avancé l'argent, `danger` Annulée, statut en cours avec point live), **ma part** à droite (display 18 px, `tabular-nums`, « ma part » en `subtle`). Pied bordé : « Mes plats (n) » (`aria-expanded`, chevron qui pivote, `motion-reduce`) → panneau `elevated/40` (quantité ×, nom, options, note en italique, total de ligne ; puis Mes plats / Ma part des frais / Ma part / Total de la commande / payeur et moyen) ; lien « Voir » ou « Reprendre » (en cours) ; « Relancer ici » (`ghost`, `text-brand`, icône `RotateCcw`) seulement si le resto est actif et la commande finie → `CreatePartySheet` avec ce resto (commande directe, sans vote) |
| Onglets du profil (`ProfileTabs`) | | motif ARIA *tabs* (flèches, Début / Fin, `tabIndex` itinérant) au look `Segmented` (pills pleine largeur, 44 px) : « Mes commandes » (défaut) / « Mes infos » (`?onglet=infos`) |
| Carte « Comme la dernière fois ? » (`ReorderCard`, étape Commande) | | affichée quand **mon** panier est vide et que j'ai déjà commandé dans ce resto : pastille icône `History` braise 40 px, date de la commande source, 4 plats max (+ n autres), plats retirés en `warning`, « Environ X € » (prix actuels serveur), bouton `secondary` « Reprendre ma dernière commande » (le `primary` reste « Je suis prêt·e ») ; toasts : succès « n plats remis dans ton panier », avertissement listant les plats sautés et la raison |
| Palette de recherche (`CommandPalette`, `features/search`) | mobile (< 768 px) : **plein écran** `elevated`, flèche retour 44 px, champ 56 px en 18 px ; desktop : dialogue 640 px à 12 vh du haut, `rounded-xl`, bord `border`, `shadow-card`, voile `bg/70` flouté, pied d'aide clavier (`Kbd` : ↑ ↓ naviguer, ↵ ouvrir, Échap fermer, « / » ou Ctrl K / ⌘K partout) | ouverture : bouton d'en-tête (`SearchButton` : icône 44 px, pastille « Rechercher… Ctrl K » dès 1024 px), **Ctrl K / ⌘K** (bascule) et **« / »** hors champ de saisie, jamais par-dessus une autre modale. Motif ARIA *combobox* + *listbox* : le focus reste dans le champ (`aria-activedescendant`), ↑ ↓ bouclent, Entrée ouvre, survol = option active, Tab piégé dans le dialogue, Échap (fiche → résultats → fermeture), focus rendu au déclencheur. Saisie différée de **150 ms** ; `Spinner` dans le champ pendant le chargement, squelettes au premier chargement, erreur `role="alert"` + « Réessayer », annonce `aria-live` (« 6 résultats »). **Vide** : chips « Envie de… » (Pizza, Sushi, Burger, Poké, Ramen, Thaï, Indien, Libanais, Frites), recherches récentes (6, sur l'appareil, « Effacer l'historique »), raccourcis. **Résultats** groupés avec en-têtes 12 px capitales `subtle` + icône : Restaurants (`Store`), Plats (`UtensilsCrossed`), Collègues (`Users`), Raccourcis (`Sparkles`, pastille `brand/12`) ; lignes 56 px, vignette emoji 40 px `surface`, titre gras, méta `muted` 12 px (cuisines · distance · n plats ; « Chez X » · extrait `subtle` ; « n commandes en commun »), prix `tabular-nums` à droite, option active `fg/7` + ↵. Termes trouvés surlignés `<mark>` `brand/20` (pliage accents identique au serveur). « Résultats approchants pour « x » » (`Sparkles` braise) si faute corrigée ; aucun résultat : loupe + « Aucun résultat pour « x » » + chips. **Collègue** : fiche (avatar 56, nom display 20 px, « n commandes en commun » / « Vous êtes dans la même équipe », 3 dernières commandes en liens 48 px avec badge de statut). **Plat** → page du resto `?plat=<id>` : défilement centré sur le plat de sa catégorie (pas la rangée « Populaires »), focus sur la carte, **pulse braise 2 × 600 ms** (anneau `brand` 55 % → 0) ; `prefers-reduced-motion` : défilement instantané, anneau fixe 2 s. Entrées : fondu + 8 px (opacité seule en mouvement réduit) |

## 4. Écrans clés

1. **Accueil** — héros « Qu'est-ce qu'on mange ? », CTA *Lancer une commande*,
   champ *Rejoindre avec un code*, mes commandes en cours, restaurants à proximité.
   Partout ailleurs : bandeau « Commande en cours » (voir composants) et toasts de statut
   (« Le vote est ouvert — « Midi du vendredi » », action « Voir ») quand une de mes commandes
   avance pendant que je suis sur une autre page.
2. **Restaurants** — recherche, filtres cuisine en chips, cartes (cover/emoji,
   note, ETA, frais, distance, badges fournisseurs). Dès 2 caractères, section **« Plats correspondants »**
   (même endpoint que la palette) au-dessus des restos : cartes 64 px (vignette emoji 40 px, nom surligné,
   « Chez X · prix », chevron) vers le resto ancré sur le plat ; squelettes pendant le chargement, rien si
   aucun plat ne correspond, erreur discrète + « Réessayer ».
3. **Restaurant** — héros, catégories en onglets collants, items avec options.
4. **Party** — en-tête (titre, code, avatars, stepper) + contenu par étape :
   * En-tête : code + copier le lien + **partager** (`navigator.share` avec le lien `/j/:code`, si dispo) ;
     un membre qui rouvre `/j/:code` revient directement dans la salle (« Te revoilà dans … »).
   * *Salon* : lien + QR d'invitation, membres live, sélection des candidats.
   * *Vote* (par classement, ADR 0005) : cartes restaurants, cœur → pastille de rang « 1er / 2e… » ; carte « Mon classement »
     (pastille 36 px, 1er choix en `bg-ember`, « +n pts », Monter / Descendre / Retirer en boutons icônes 44 px, focus conservé
     sur le bouton déplacé, annonce `aria-live` « X est maintenant ton 2e choix ») ; carte « Classement en direct » (points
     serveur, jauges `meter`, réordonnancement `layout`) ; « Ton 1er choix vaut le plus de points ».
   * *Commande* : menu du restaurant, panier perso en sheet, statut « prêt » des autres.
   * *Récap* : totaux par personne, récap consolidé, choix d'envoi (Uber Eats,
     Takeaway, export, téléphone), choix du payeur.
   * *Paiement* : ma part + QR EPC (montant + communication) / liens Revolut, PayPal
     avec montant / espèces / plus tard ; vue payeur avec ses
     moyens visibles, la liste des parts (moyen déclaré en badge) et confirmation.
5. **Authentification** (`features/auth`, mise en page `AuthLayout` : titre display 32 px centré, carte,
   pied `muted`) — *Connexion* : lien `text-brand` « Mot de passe oublié ? » aligné à droite sous le mot de
   passe (pré-remplit l'e-mail saisi) ; identifiants refusés → « E-mail ou mot de passe incorrect. », compte
   suspendu → message serveur « Compte suspendu… ». *Inscription* : jauge de robustesse (`StrengthMeter` :
   3 segments 6 px `danger` / `warning` / `success`, libellé « Robustesse : Faible / Correct / Solide » + conseil
   `muted`, `aria-live`), toast de bienvenue avec rappel de l'e-mail de confirmation. **« Continuer avec
   Google »** : bouton `secondary` pleine largeur sous un séparateur « ou », affiché seulement si le serveur
   l'annonce ; fenêtre surgissante ouverte dans le clic, pop-up bloquée → toast d'erreur explicite (8 s) ;
   pendant l'attente « Termine la connexion dans la fenêtre qui s'est ouverte. **Annuler** ».
   Pages des liens d'e-mail (`/auth/mot-de-passe-oublie`, `/auth/reinitialiser/:token`,
   `/auth/verifier/:token`, `/auth/changer-email/:token`) : formulaire court (un seul `primary` `lg` pleine
   largeur) puis **état de résultat** (`Outcome`) — pastille ronde 56 px (`success/12` coche, `danger/12`
   triangle, `info/12` enveloppe), titre display 20 px, texte `muted`, action suivante (`primary` « Se
   connecter » / « C'est parti », `secondary` sinon) ; `role="status"` ou `alert`. Nouveau mot de passe :
   `PasswordFields` (nouveau + confirmation, erreurs seulement après saisie, bouton désactivé tant que
   invalide). Lien expiré : encadré `danger/10` + lien « Nouveau lien ». Tout l'espace `/auth/*` masque le
   bandeau « Commande en cours » et le CTA d'en-tête.
   **Bandeau « Confirme ton adresse e-mail »** (`VerifyEmailBanner`) : `aside` `warning/10` bord
   `warning/30`, icône `MailWarning`, texte court + adresse en `muted`, bouton `ghost sm` « Renvoyer
   l'e-mail » (→ « Envoyé »), croix 36 px « Masquer ce rappel » (session) dans le shell ; non masquable dans
   le profil. Affiché seulement si le compte n'est pas vérifié **et** que le serveur envoie des e-mails :
   on rappelle, on ne bloque jamais.
   **E-mails** (`backend/internal/app/mailtemplates.go`) : palette « Ember » claire en styles en ligne (les
   clients mail ignorent les variables CSS) — fond `#FAF8F4`, carte blanche `radius 20px`, titre 24 px 750,
   bouton dégradé braise → ambre, texte `#1A0B05` (= `brand-fg`, AA), lien brut de secours en `brand-ink`,
   pied « Développé par OCC Interactive, une division de CARDOR Media ». Ton tutoyé, durée de validité rappelée, « Ignore cet e-mail »
   si ce n'est pas toi.
6. **Profil** — en-tête (avatar, nom, e-mail) puis onglets : *Mes commandes* (4 tuiles
   chiffrées — Commandes, Dépensé, Resto chouchou, Plat préféré —, puis cartes d'historique
   10 par 10 avec « Voir plus de commandes », état vide illustré par l'assiette + « Lancer une
   commande ») ; *Mes infos* (nom, couleur, coordonnées de remboursement, **Sécurité**, **Supprimer mon
   compte**, thème, déconnexion). Carte *Sécurité* en trois sections bordées (titres `h3` avec icône braise
   16 px) : *Adresse e-mail* (adresse + badge `success` Vérifiée / `warning` Non vérifiée, « Renvoyer
   l'e-mail de confirmation » en `ghost sm`, champ « Nouvelle adresse » + `secondary` « Changer d'adresse ») ;
   *Mot de passe* (actuel + `PasswordFields`, `secondary` « Changer le mot de passe » ; compte Google sans mot
   de passe : texte explicatif + « Recevoir un lien pour choisir un mot de passe ») ; *Comptes connectés*
   (ligne `elevated/40` par fournisseur : badge `success` « Google connecté » + « depuis … » + `ghost`
   « Dissocier » — désactivé avec explication sans mot de passe —, ou `secondary` « Associer Google »).
   Fonctionnalités indisponibles sans SMTP : phrase `subtle` au lieu du formulaire. Carte *Supprimer mon
   compte* bordée `danger/30` (icône `ShieldAlert`), explication de l'anonymisation, bouton `danger` →
   `Sheet` « Supprimer ton compte ? » avec champ « Tape SUPPRIMER pour confirmer » (autofocus) ; le bouton
   `danger` « Supprimer définitivement » reste désactivé tant que le mot n'est pas tapé.
7. **Administration** (`/admin`, rôle `admin`) — mêmes tokens et composants, densité
   plus « outil » (listes compactes, actions icônes 44 px avec `aria-label`).
   * *Mise en page* : barre latérale 220 px (≥ 768 px, entrées `rounded-md`, icône braise
     sur l'entrée active) ; en mobile, onglets en pills défilants horizontalement sous
     l'en-tête (conteneur `relative` pour les `sr-only`). Accès : lien « Admin » dans la
     nav desktop, icône bouclier dans l'en-tête mobile.
   * *Tableau de bord* : 4 tuiles chiffrées (display 32 px, `tabular-nums`, méta en `subtle`),
     histogramme SVG maison des commandes par jour (série unique en braise, barres fines
     à bouts arrondis 4 px posées sur la ligne de base, écart 2 px, grille discrète, infobulle
     au survol/focus, tableau `sr-only` en alternative — pas de librairie de graphiques),
     répartition par statut en badges, top restaurants.
   * *Restaurants* : carte **Cartes incomplètes** en tête (`IncompleteMenusCard`) — pastille icône
     `EyeOff` 40 px, `CardTitle`, texte d'explication `muted` ; ligne de réglage qui passe à la ligne
     en mobile : `Toggle` (`role="switch"`, « Masquer les restaurants incomplets ») + libellé
     « Masquer les restaurants de moins de » + `Input` numérique 80 px centré `tabular-nums` + « plats »,
     bouton `sm` « Appliquer » seulement si le seuil saisi diffère du seuil actif ; erreur de saisie
     en `danger` (`role="alert"`, `aria-invalid`). Pied séparé d'une bordure : compteur `aria-live`
     (« **N restaurants masqués** sur M actifs », ou aperçu « N restaurants seraient masqués avec ce
     seuil » quand le filtre est coupé / le seuil modifié), bouton `ghost` « Voir les restaurants
     masqués » (icône `ListFilter`) qui active le filtre *Incomplets* ; note `subtle` vers le favori
     « Exporter vers OCC ».
     Puis recherche + `Segmented` (Tous / Visibles / Masqués / Obsolètes / Incomplets, défilant en
     mobile, `?filtre=` dans l'URL), lignes avec emoji, badges « Masqué » / « Masqué : carte
     incomplète (N plats) » (`warning`, icône `EyeOff`) / « Sans coordonnées » (warning) / « Verrouillé » (`info`, icône `Lock`) /
     « Obsolète » (`warning`, icône `TriangleAlert`, date en infobulle), interrupteur de visibilité
     (`Toggle`, `role="switch"`), actions Menu / Modifier. Formulaire en `Sheet lg` par
     sections (Identité, Adresse + bouton *Géocoder*, Livraison, Plateformes — Uber Eats,
     Takeaway, Deliveroo, weloveat —, Synchronisation), montants saisis en euros (`12,50`) et
     convertis en centimes. L'interrupteur « Verrouillé » est **activé par défaut** à
     l'enregistrement : il montre ce que fera le serveur (toute modification verrouille).
   * *Éditeur de menu* : une carte par catégorie (↑ ↓, renommer, supprimer), lignes
     d'articles (prix à droite, étiquettes en badges, interrupteur « disponible », étoile
     « populaire », cadenas `Lock` / `LockOpen` (`aria-pressed`, couleur `info` si verrouillé),
     ↑ ↓, modifier, supprimer) ; article barré si indisponible ; badges Verrouillé / Obsolète
     dans l'en-tête. Sheet article :
     étiquettes en `Chip` (presets + libres), options en groupes encadrés (min / max, choix
     + supplément en euros). Confirmations destructives en `Sheet` avec bouton `danger`.
   * *Import* : zone de dépôt pointillée (bordure braise au survol), aperçu par restaurant
     (badge Nouveau / Mise à jour, erreurs `danger`, avertissements `warning`, menu dans un
     `details`), barre d'action collante avec l'unique `primary` « Importer » (désactivé tant
     qu'il reste des erreurs) ; cartes latérales Modèles et « Depuis Uber Eats / Takeaway ».
   * *Synchronisation* (`/admin/synchronisation`, icône `RefreshCw`) : unique `primary`
     « Synchroniser maintenant » dans l'en-tête (icône qui tourne pendant l'exécution, désactivé
     si une exécution tourne ou si `OCC_SYNC_ENABLED=false`). **Carte d'état** (`aria-live`) :
     en cours → `Spinner` + durée + journal en direct (`pre` 12 dernières lignes, sondage 3 s) ;
     sinon badge de statut (`success` Réussie, `warning` Partielle, `danger` Échec / Bloquée)
     + résumé des compteurs sans les zéros + prochaine exécution (heure de Bruxelles).
     **Historique** : une carte repliable par exécution (bouton plein largeur `aria-expanded`,
     chevron qui pivote, `motion-reduce` respecté) → sources (badge OK / Bloquée / Erreur,
     requêtes, cache, durée), liste des changements (« Tomo — Miso ramen : 14,50 € → 15,00 € »,
     zone défilante bordée) et journal technique dans un `details`. **Sources** : lignes avec
     interrupteur d'activation, nom, badge plateforme (couleurs Deliveroo / weloveat / Takeaway),
     priorité en `tabular-nums`, URL tronquée, dernier statut ; ajout / modification en `Sheet`
     (type, nom, URL avec aide propre au type, priorité, activée, options) ; suppression
     confirmée en `Sheet` avec bouton `danger`. **Carte « Découvrir un site Takeaway »** (en tête
     des sources, `DiscoverCard`) : champ « Lien takeaway.com ou nom du resto » + bouton
     « Découvrir » (`Search`, état `loading`) ; pendant la recherche (10–20 s) `Spinner` + texte
     de progression qui avance avec les secondes, dans une zone `aria-live="polite"`. Chaque site
     trouvé = bloc `bg-elevated` bordé : nom (gras), adresse (`muted`), « N plats · N catégories ·
     à 900 m » (`subtle`), lien externe `text-brand` vers le site (`sr-only` « nouvel onglet »),
     puis bouton `sm` primaire « Ajouter et synchroniser » **ou** badge `success` « Déjà suivi » ;
     après l'ajout, ligne `role="status"` : `Spinner` puis badge de statut + résumé des compteurs.
     Rien trouvé : encadré « Aucun site Takeaway trouvé. », renvoi vers « Ajouter une source » et
     `details` « N adresses vérifiées » (badges de statut neutres / `warning` / `danger`).
   * *Commandes* : chips de statut, lignes cliquables vers le détail, annulation forcée
     (icône `Ban`, confirmation).
   * *Utilisateurs* : carte **E-mails** en tête (pastille `Mail` 40 px `success` / `warning`, badge Actifs /
     Désactivés, expéditeur et serveur en `subtle` — ou les variables à renseigner —, `secondary sm`
     « Envoyer un e-mail de test », désactivé sans SMTP, `aria-live`). Recherche + `Segmented` « Tous / Admins /
     Suspendus / Non vérifiés » (défilant en mobile). Lignes : avatar 40 px, nom, badges `brand` Admin,
     Toi, `danger` Suspendu (ou neutre Supprimé, ligne à 70 % d'opacité), `warning` Non vérifié, `info`
     Google ; méta `subtle` (e-mail · commandes · inscrit · connecté il y a …) ; motif de suspension en
     `danger` 12 px. Une seule action par ligne : bouton icône 44 px `MoreHorizontal` (« Actions pour X »)
     → `Sheet` titrée du nom, liste d'actions 48 px (icône 20 px + libellé gras + aide `muted`) :
     Promouvoir / Retirer admin, Envoyer un lien de réinitialisation, Forcer la déconnexion, Réactiver ou
     **Suspendre** (`danger`), **Supprimer le compte** (`danger`) — actions interdites désactivées avec
     leur raison (son propre compte, e-mails coupés). Confirmations en `Sheet` (« Suspendre Bob ? » avec
     `Textarea` « Motif » facultatif ≤ 300 ; « Supprimer le compte de Bob ? » qui rappelle que l'historique
     reste et que c'est irréversible) : `secondary` Annuler + `danger` / `primary` à droite.
     Filtre « Invités » dans le `Segmented` ; badge `info` **Invité** (remplace « Non vérifié »), méta
     « sans compte (prénom seulement) » à la place de l'e-mail réservé.
8. **Salon d'équipe** (`/equipes/:id`, `features/teams`) — colonne de lecture 680 px.
   * *En-tête* : `TeamEmblem` 64 px (emoji sur la couleur de l'équipe à 22 %, bordure à 45 % — donnée
     utilisateur comme `users.color`, jamais un token), nom display 32 px, lignes `muted` 14 px avec icônes
     `Clock` (« Du lundi au vendredi à 12:15 ») et `MapPin` (adresse) ; badge `warning` « Archivée » ;
     bouton icône `ghost` `Settings` « Réglages de l'équipe » (propriétaire / admins).
   * *Action principale* : **une seule** — s'il y a une commande d'équipe en cours, `Card selected` « Commande
     en cours » (badge `brand` du statut, emoji du resto ou 🗳️, titre, « Lancée par Bob · 2 participants · resto »)
     avec `primary lg` « Rejoindre la commande » (un geste, sans code) ou lien `secondary` « Ouvrir la commande »
     si j'y suis ; sinon `primary lg` « Lancer la commande du jour » (icône `Rocket`). Invité·e : carte `muted`
     « En invité·e, tu rejoins en un geste celles que lance l'équipe » (pas de lancement).
   * *Membres* : carte « N membres », lignes avatar 40 px + nom (« (toi) » en `subtle`) + badges `brand`
     Propriétaire / `info` Admin / neutre Invité·e ; propriétaire : bouton icône `ShieldCheck` (`aria-pressed`,
     `info` si admin) « Nommer X admin » (jamais pour un·e invité·e) ; propriétaire / admins : `UserMinus`
     « Retirer X de l'équipe » (cibles 44 px).
   * *Lien de l'équipe* (`TeamInviteCard`, même gabarit que `InviteCard`) : « Copier le lien », « Partager »
     (Web Share), `ghost` « Nouveau lien » (admins), URL `/e/:code` en `subtle`, QR 132 px.
   * *Historique de l'équipe* : lignes compactes (emoji 40 px, resto ou titre, « date · n participants · statut »,
     ma part en `Money` si j'y étais — ligne cliquable vers la party seulement dans ce cas), « Voir plus ».
   * `ghost` « Quitter l'équipe » en bas (sauf propriétaire).
   * *Réglages* (`Sheet`) : nom, emoji en `Chip` 44 px, couleur (pastilles d'avatar), adresse + bouton icône
     `LocateFixed` « Localiser l'adresse sur la carte » (Nominatim, position en aide), heure (`input time`,
     « Heure de Bruxelles »), jours en `Chip` (Lun…Dim, `aria-label` du jour complet), `Segmented` partage
     (Parts égales / Au prorata), candidats par défaut (chips retirables + recherche dans les restos proches,
     20 max), `danger sm` « Archiver l'équipe » (propriétaire).
   * *Accueil* : section **Mes équipes** (sous les commandes en cours) — cartes `interactive` (ou `selected`
     si une commande d'équipe tourne, badge « Commande en cours — rejoindre »), emblème, horaire habituel en
     `subtle`, `AvatarStack` 24 ; `ghost sm` « Créer une équipe » (comptes) → `Sheet` nom / adresse / heure.
     Feuille « Lancer une commande » : `select` « Pour l'équipe… » (si j'ai des équipes) avec aide « Toute
     l'équipe est prévenue… ».
   * *Party d'équipe* : sous l'en-tête, carte « Membres de l'équipe pas encore là · 🍕 OCC Mons » (avatars
     24 px à 60 % d'opacité + prénom `muted`, « (invité·e) »), tant que la commande est en salon / vote / commande.
   * *Toast realtime* « 🍕 OCC Mons : la commande du jour est lancée ! » avec action « Rejoindre » (15 s, un
     seul par commande), seulement pour les membres pas encore dedans.
9. **Invitation & invités** (`/j/:code`, `/e/:code` sans session — `InviteGate`, gabarit `AuthLayout`) :
   surtitre braise « Invitation à une commande » / « Invitation d'équipe », titre display (emoji + nom),
   « Bob t'invite · 3 membres » (`Users`) ; carte : `primary lg` « Se connecter », `secondary` « Créer un compte »
   (retour automatique sur le lien), `ghost` « Continuer en invité·e » → formulaire **prénom seul** (aide « Pas
   d'e-mail, pas de mot de passe… »), couleur d'avatar facultative (pastilles 44 px, `aria-pressed`), erreur
   `role="alert"`, `primary lg` « Rejoindre la commande / l'équipe », lien « J'ai déjà un compte ». Commande
   fermée / équipe archivée : carte `muted` sans formulaire.
   * *Bandeau invité* (`GuestBanner`, shell, hors party / auth / profil) : encadré `info/10` bordure `info/30`,
     icône `UserPlus`, « **Tu es invité·e** — crée un compte pour garder ton historique », `ghost sm` « Créer mon
     compte » (→ Profil › Mes infos), croix « Masquer ce rappel » (mémorisé dans `localStorage`). Le bandeau
     « Confirme ton adresse e-mail » n'est jamais montré à un·e invité·e.
   * *Profil limité* : sous-titre « Invité·e — sans compte » à la place de l'e-mail ; carte **Créer mon compte**
     (pastille `info` `UserPlus`, e-mail, mot de passe + confirmation, `primary` « Créer mon compte », puis
     « Continuer avec Google ») ; pas de carte Remboursements ni Sécurité. Feuille « Lancer une commande » :
     explication + `primary` « Créer mon compte ».

### Notifications, hors ligne, heures limites (PWA)
* **Carte « Notifications »** (Profil → Mes infos, après Sécurité ; `features/notifications/NotificationsCard.tsx`) :
  titre avec icône `Bell` en `text-brand`, badge d'état (`success` « Activées ici », `danger` « Bloquées », neutre
  « Désactivées »), une phrase d'explication, puis **une seule action principale** : « Activer les notifications »
  (la permission est demandée dans le clic, jamais au chargement) ou, une fois abonné, « Envoyer un test »
  (secondaire) + « Désactiver sur cet appareil » (ghost). Trois interrupteurs (`role="switch"`, ≥ 44 px) :
  *Étapes des commandes*, *Remboursements*, *Rappels d'heure limite*, avec une ligne d'aide `text-xs text-muted`.
  Cas particuliers en encadré : iPhone hors écran d'accueil (`info/10` : « Partager → Sur l'écran d'accueil »),
  navigateur incompatible ou serveur sans push (neutre), permission refusée (`danger/10`). Ligne « Installer l'app »
  (invite native Chrome / Edge / Android, mode d'emploi sur iOS), masquée si l'app est déjà installée.
* **Bandeau hors ligne** (`pwa/PwaRuntime.tsx`, en tête de `<main>`, `role="status"`, `aria-live="polite"`) :
  `warning/10` + bordure `warning/30`, icône `CloudOff`, « **Hors ligne** — les données affichées peuvent dater. »,
  puis « N actions en attente d'envoi » ; au retour du réseau « Connexion rétablie : tes actions hors ligne vont être
  envoyées. » puis toast « Synchronisé : Ajout : Margherita » / « N actions hors ligne synchronisées » (erreur :
  toast `error` « Non synchronisé : … » + raison du serveur). Les boutons non rejouables (Clore le vote, Passer au
  récap, heures limites) sont **désactivés** avec l'infobulle « Indisponible hors ligne » ; vote, « prêt·e » et
  ajout au panier restent actifs (toast « … sera ajouté dès le retour du réseau »).
* **Heures limites** (`features/deadlines/PartyDeadlines.tsx`, sous l'en-tête de la party, en vote / commande) :
  carte compacte « Fin du vote à **11:45** » (heure de Bruxelles, `tabular`) + `Countdown` + badge `info`
  « Clôture automatique » ou neutre « Clôture par l'hôte ». Hôte : réglages **repliés** derrière un bouton `ghost`
  « Modifier » / « Fixer une heure limite » (`aria-expanded`, ouverts d'office seulement ≥ 1024 px) : chips
  `+5/+10/+15/+20 min` (`min-h-11 px-4`), champ « ou à HH:MM » (`h-11`) + « Fixer » (`md`), chip « Retirer »,
  interrupteur « Clôturer automatiquement » (ligne `flex items-center justify-between gap-3`). Les membres ne voient
  que la ligne (rien sans heure limite). C'est le **seul** `Countdown` de l'écran (le vote n'en affiche pas d'autre).
* **Chips d'évènements automatiques** (journal `AutoEvents`, sous la carte, toutes étapes) : encadré `fg/[0.03]`,
  3 dernières lignes (la plus récente en `text-fg`, les autres `text-muted`), « Tout afficher (n) ». Icônes :
  rappel `BellRing` `info`, clôture `CheckCircle2` `success`, prolongation `Hourglass` `warning`, décision de l'hôte
  `AlertTriangle` `warning`, échec `AlertTriangle` `danger`. Textes rédigés par le serveur (« Vote clôturé
  automatiquement à 11:45 — Pizza Nonna »).
* **Notifications système** : titre court avec un emoji au plus (« Le vote est ouvert 🗳️ »), corps d'une phrase
  qui nomme la commande entre « », montants au format `12,40 €`, tutoiement ; icône = logo, badge = flamme blanche.

## 5. Ton éditorial

Français, tutoiement léger et chaleureux (« On commande où ? », « T'es prêt·e ? »),
phrases courtes, pas de jargon technique. Inclusif (point médian avec parcimonie).

## 6. Revue design — 2026-10-09

Revue sur captures Playwright (390×844 et 1280×800, sombre + clair) d'un parcours
complet à deux (salon → vote → commande → récap → paiement Wero → clôture).
Captures de référence : `docs/screenshots/<écran>-<mobile|desktop>-<dark|light>.png`.

### Corrigé pendant la revue
* **P0** — Défilement horizontal de l'accueil (+1 800 px en mobile) : les `sr-only`
  des cartes du carrousel « À deux pas » s'échappaient vers `<main>`. Le carrousel
  et `RestaurantMeta` sont `relative`. → *Règle : tout conteneur `overflow-x-auto`
  contenant des `sr-only` est `relative`.*
* **P0** — QR d'invitation qui débordait de sa carte dans la colonne de 360 px du salon
  (desktop) : `InviteCard` passe en *container query* (`@container` + `@md:`).
* **P1** — Deux `primary` visibles (CTA d'en-tête + CTA de l'écran) : le raccourci
  d'en-tête est `secondary` et masqué sur l'accueil, l'auth et la party.
* **P1** — Ingrédients du héros posés sur les lettres du titre : placement en trois
  paliers (creux des lignes en mobile, au-dessus en tablette, moitié droite vide en desktop).
* **P1** — Contraste AA en clair : badges d'état, braise en texte, badges Uber Eats /
  Takeaway, `subtle` (sombre et clair), montant « Ta part » en dégradé ambre.
* **P1** — « € » orphelin sur sa propre ligne dans les cartes resto : la gamme de prix
  suit la note.
* **P1** — Écran clôturé qui annonçait « Tout est réglé ! » alors que des parts
  restaient en attente (clôture manuelle) → « Commande clôturée » + nombre restant.
* **P1** — Vue payeur : « Merci pour l'ava… » tronqué en 390 px → le titre passe à la ligne.
* **P2** — Stepper mobile : coche masquée < `sm` (fini les « Comma… ») ; titre
  « On commande chez … » sur deux lignes max au lieu d'être tronqué.

### Patterns validés
* Barre d'action collante en bas (panier + « Je suis prêt·e », « Rouvrir » + « Qui a payé ? ») :
  un `primary` large, un `secondary` à gauche, au-dessus de la tab bar.
* En-tête de party : titre + badge d'état + avatars (anneau vert = prêt) + « n membres · n prêts »
  + code copiable, puis stepper — l'état du groupe reste visible à chaque étape.
* Montant héros « Ma part » (display 40 px, `tabular-nums`) + communication en clair,
  tuiles de paiement 2 colonnes, détails copiables et guide en 3 étapes numérotées.
* Sheet d'options : groupes avec badge « Obligatoire » / « Jusqu'à n », lignes de 48 px,
  prix de supplément alignés à droite, CTA collant en pied.
* États vides illustrés (assiette + couverts) avec une action secondaire.

### Patterns — passe 2 (2026-10-09)
* **Grilles mobiles explicites** : toute grille de cartes s'écrit `grid grid-cols-1 … sm:grid-cols-2` (jamais `grid`
  seul : la piste implicite `auto` s'élargit au contenu et fait déborder l'écran) ; les enfants ont `min-w-0`, les
  badges `shrink-0`, les textes tronqués `min-w-0 truncate`.
* **En-tête desktop sans débordement à 1024 et 1280 px** : logo `shrink-0 whitespace-nowrap`, libellés longs
  (« Rechercher… », « Lancer une commande ») seulement dès `xl:` (icône + `aria-label` en dessous), pastille
  « Reprendre » `max-w-28 xl:max-w-44`.
* **Commande en cours, une seule mise en avant** : sur `/`, la pastille d'en-tête et le dock mobile sont masqués dès
  que l'accueil liste des commandes (héros ou cartes) ; masqués aussi sur `/j/…`, `/e/…`, l'auth et dans la party ;
  pas de dock sur `/admin`. Le « + » de la tab bar n'apparaît que pour un compte (pas un invité), hors `/j/` et `/e/`.
* **Invités** : jamais de `primary` « Lancer une commande » ; à la place un `secondary`
  « Créer mon compte pour lancer une commande » → `/profile?onglet=infos`.
* **Libellés d'état distincts** : l'étape `ordering` s'appelle « Paniers ouverts » ; « Commande en cours » est réservé
  au bandeau / dock de reprise (toutes étapes confondues). Le titre de la carte d'équipe est le titre de la party
  (« Commande du jour » par défaut). « n prêts » n'apparaît dans l'en-tête qu'à l'étape `ordering` ; « + … de frais »
  seulement si la part de frais est > 0.
* **Paiements : un primary par moyen** : quand le moyen choisi a un lien à montant pré-rempli (Revolut, PayPal…),
  ce lien est le `primary` et « J'ai payé » passe en `secondary`, en dessous.
* **Barres d'action collantes** : en desktop, le `primary` est `sm:min-w-56 sm:flex-none` aligné à droite ; pas de
  raccourci qui double une action déjà présente dans la colonne latérale (≥ 1024 px). Au salon, la durée du vote
  (mêmes valeurs que les heures limites : Sans limite, 5, 10, 15, 20 min, même arrondi) est un `select` sur une ligne
  **au-dessus** de la barre, qui ne garde que « Choisir directement » + « Lancer le vote ».
* **AvatarStack** : chevauchement `-space-x-1` à 24 px (`-space-x-2` au-delà) pour ne pas rogner les initiales.
  Avatars côte à côte (pile, membres d'une party ou d'une équipe) : couleurs passées par `distinctColors`
  (`lib/colors.ts`) — chacun garde la sienne sauf si un membre précédent l'a déjà. Dans une party, c'est appliqué
  une fois dans le contexte (`PartyPage`), donc une personne a la même couleur à toutes les étapes.
* **Bas de page** : `<main>` en `flex min-h-[calc(100dvh-4rem)] flex-col`, footer `mt-auto` (jamais caché sous le dock
  sur une page courte ; le padding du dock est conservé).
* **Bandeaux** (invité, e-mail à confirmer) : texte et action en `flex-wrap` (le texte garde au moins 14rem à 390 px),
  bouton de fermeture `size-11` (44 px), comme toutes les croix de fermeture.
* **Profil « Mes infos »** : la zone de danger (suppression du compte) est toujours la dernière carte.

### Idées P2 restantes
* Accueil desktop : le carrousel « À deux pas » est coupé net à 1 240 px — ajouter un
  masque en fondu sur les bords ; harmoniser « Créer un salon » / « Lancer une commande ».
* Récap mobile : « Passer la commande » (envoi fournisseur) arrive après le CTA « Qui a payé ? » ;
  remonter le bloc d'envoi avant le choix du payeur pour suivre l'ordre réel.
* Salon desktop : la liste « Dans le salon » passe sous la ligne de flottaison depuis que
  le QR est empilé — envisager un QR plus petit (120 px) ou un onglet Lien / QR.
* Couleurs d'avatar générées parfois trop proches (deux oranges côte à côte) : imposer un
  écart de teinte minimal dans une même party.
* Thème clair : le cœur de vote et les icônes `text-brand` passent en braise foncée ;
  envisager une classe `icon-brand` qui garde la braise vive (3:1 suffit pour une icône).
* Mobile : le bouton « + » de la tab bar double le CTA du héros sur l'accueil.
* Code de party affiché deux fois dans le salon (pastille d'en-tête + carte d'invitation).

## Signature studio (pied de page)

« Développé par **OCC Interactive** · une division de **CARDOR Media** · © année ». Emblèmes de la charte CARDOR
(`cardormedia.com/marque`) via `components/brand/BrandEmblem` : Dragon (OCC Interactive, ratio 1,23, h-6) et roue du
Car d'Or (CARDOR Media, size-4). Mouvement repris du site CARDOR : « boot » glitch au premier affichage, glitch du
Dragon et tour de roue au survol / focus du lien ; jamais en boucle (principe « Silence »). Couleurs de marque en thème
sombre, version mono (`currentColor`) en clair. Animations réduites : statique. Ne pas déformer, ni ajouter d'effet.
