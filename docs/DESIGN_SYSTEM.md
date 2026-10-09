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
| `--color-wero` / `--color-wero-fg` | `#FFE500` / `#1A1A1A` | wallet Wero — aplat + texte (contraste ≈ 15:1) |
| `--color-bancontact` / `--color-bancontact-fg` | `#005498` / `#FFFFFF` | Bancontact Pay — aplat + texte (contraste ≈ 7,6:1) |
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
* `prefers-reduced-motion` → opacité uniquement.

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
| États vides | assiette vide avec fourchette qui tapote, oscillation douce | boucle GSAP lente |

Budget : chaque scène ≤ 6 Ko gzip, aucune animation bloquante (> 1,5 s) sur
un parcours critique, toutes interruptibles.

## 3. Composants (`frontend/src/components/ui`)

| composant | variantes | notes |
|---|---|---|
| `Button` | `primary` (dégradé braise + glow), `secondary` (surface + bordure), `ghost`, `danger`; tailles `sm` `md` `lg`, `icon` | état `loading` (spinner), `asChild` non requis |
| `Card` | `default`, `interactive` (hover lift), `selected` (bordure braise) | |
| `Badge` | `neutral`, `brand`, `success`, `warning`, `danger`, `info`, `ubereats`, `takeaway`, `deliveroo`, `weloveat`, `wero`, `bancontact` | pill 12 px ; `wero`/`bancontact` en aplat de marque (wordmark, pas de logo officiel) |
| `Avatar` / `AvatarStack` | tailles 24/32/40/56 ; anneau `ready` vert | initiales sur `user.color` si pas d'image |
| `Input`, `Textarea`, `Field` | label + aide + erreur | |
| `Sheet` | bottom sheet mobile / dialog centré desktop | focus trap, Échap |
| `Stepper` | étapes de la party (Salon → Vote → Commande → Récap → Paiement) | |
| `Money` | `<Money cents={1250} />` | tabular-nums |
| `EmptyState` | illustration emoji + titre + action | |
| `Skeleton` | shimmer | |
| `QuantityStepper` | − 1 + | |
| `Countdown` | mm:ss | |
| `Toast` | via `sonner`, thème sombre | |
| `QRCodeCard` | QR généré (`value`) **ou** image de QR (`imageSrc`, QR « recevoir » Wero / Bancontact) + montant + bouton copier | `qrcode.react`, fond blanc obligatoire (lisibilité scanners), `fgColor="currentColor"` sur `--color-qr-fg` |
| Tuiles de paiement (`MethodTiles`) | une tuile ≥ 80 px par moyen, dans l'ordre de `PaymentQR.methods` : Wero, Bancontact Pay, Virement QR, Lien, Espèces, Plus tard | `role="radiogroup"` ; Wero/Bancontact : wordmark en aplat, montant + communication + identifiant copiables, QR du payeur si dispo, mini-guide en 3 étapes. **Aucun faux deep link** (pas d'API P2P tierce) |
| `Segmented`, `Chip`, `CopyButton` | | radiogroup en pills ; filtre `aria-pressed` ; copie + toast |

## 4. Écrans clés

1. **Accueil** — héros « Qu'est-ce qu'on mange ? », CTA *Lancer une commande*,
   champ *Rejoindre avec un code*, mes commandes en cours, restaurants à proximité.
2. **Restaurants** — recherche, filtres cuisine en chips, cartes (cover/emoji,
   note, ETA, frais, distance, badges fournisseurs).
3. **Restaurant** — héros, catégories en onglets collants, items avec options.
4. **Party** — en-tête (titre, code, avatars, stepper) + contenu par étape :
   * *Salon* : lien + QR d'invitation, membres live, sélection des candidats.
   * *Vote* : cartes restaurants, cœur pour voter, barres de score live.
   * *Commande* : menu du restaurant, panier perso en sheet, statut « prêt » des autres.
   * *Récap* : totaux par personne, récap consolidé, choix d'envoi (Uber Eats,
     Takeaway, export, téléphone), choix du payeur.
   * *Paiement* : ma part + QR EPC / lien / espèces / plus tard ; vue payeur avec
     la liste des parts et confirmation.
5. **Profil** — nom, couleur, IBAN, lien de paiement, thème.

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
