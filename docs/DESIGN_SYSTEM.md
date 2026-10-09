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
| `--color-subtle` | `#6E6C77` | placeholders, méta |
| `--color-brand` | `#FF6A3D` | braise — accent unique |
| `--color-brand-2` | `#FFB547` | ambre — fin du dégradé |
| `--color-brand-fg` | `#1A0B05` | texte sur braise |
| `--color-success` | `#3DD68C` | prêt, payé |
| `--color-warning` | `#F5B83D` | en attente |
| `--color-danger` | `#F26D6D` | erreurs, annulation |
| `--color-info` | `#6AA8FF` | infos |
| `--color-ubereats` | `#06C167` | marque fournisseur |
| `--color-takeaway` | `#FF8000` | marque fournisseur |

### Thème clair (`[data-theme="light"]`)
`bg #FAF8F4`, `surface #FFFFFF`, `elevated #F3F0EA`, `border rgb(20 16 12 / 0.08)`,
`fg #17151A`, `muted #5F5B66`, `subtle #8E8A95`, `brand #F2542D`, `brand-2 #F59E0B`.

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

## 3. Composants (`frontend/src/components/ui`)

| composant | variantes | notes |
|---|---|---|
| `Button` | `primary` (dégradé braise + glow), `secondary` (surface + bordure), `ghost`, `danger`; tailles `sm` `md` `lg`, `icon` | état `loading` (spinner), `asChild` non requis |
| `Card` | `default`, `interactive` (hover lift), `selected` (bordure braise) | |
| `Badge` | `neutral`, `brand`, `success`, `warning`, `danger`, `ubereats`, `takeaway` | pill 12 px |
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
| `QRCodeCard` | QR + montant + bouton copier | `qrcode.react`, fond blanc obligatoire (lisibilité scanners) |

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
