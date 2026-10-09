# ADR 0004 — Animations : `motion` pour l'UI, GSAP pour les moments signature

* Statut : accepté — 2026-10-09

## Contexte
On veut une app « premium » avec des animations liées à la nourriture
(ingrédients flottants, panier qui se remplit, scooter de livraison, pluie
d'emojis…). Ces scènes demandent timelines, trajectoires, morphing SVG et
physique, ce que `motion` fait moins bien. GSAP 3.15 est désormais gratuit, plugins
inclus (Flip, MotionPath, MorphSVG, Physics2D, SplitText).

## Décision
* `motion` pour les animations d'interface liées à l'état React (listes, sheets, layout).
* GSAP + `@gsap/react` (`useGSAP`) pour les scènes culinaires et les célébrations,
  isolées dans `src/components/food/` et chargées à la demande.
* Illustrations SVG maison, colorées via les tokens. Pas de Lottie.

## Conséquences
* Deux bibliothèques (≈ +55 Ko gzip mesurés pour GSAP core + Flip, MorphSVG, SplitText, Physics2D), compensé
  par le chargement différé des scènes.
* `prefers-reduced-motion` géré via `gsap.matchMedia()` côté GSAP, et
  `useReducedMotion` côté `motion`.
