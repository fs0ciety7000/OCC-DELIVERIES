import { lazy } from 'react'

/** Scènes lourdes chargées à la demande (React.lazy) — ADR 0004. */
export const FoodHero = lazy(() => import('./FoodHero'))
export const FoodRain = lazy(() => import('./FoodRain'))
export const DispatchScooter = lazy(() => import('./DispatchScooter'))
export const PaymentCoin = lazy(() => import('./PaymentCoin'))
export const VoteBurst = lazy(() => import('./VoteBurst'))
export const DispatchedScene = lazy(() => import('./DispatchedScene'))
export const WinnerReveal = lazy(() => import('./WinnerReveal'))
export const WaitingRider = lazy(() => import('./WaitingRider'))

export { FoodLoader } from './FoodLoader'
export { EmptyPlate } from './EmptyPlate'
export { ReadyBell } from './ReadyBell'
export { flyToCart } from './AddToCartFlight'
// Petits composants propres à la party (AnimatedMoney, PulseOnChange, WaitingDots, DrawCheck,
// LiquidHeart, EmptyBag, ColleagueFlight) : importés directement depuis leur fichier, pas
// d'ici — ce barrel est aussi importé par le shell, ils finiraient dans le bundle initial.
