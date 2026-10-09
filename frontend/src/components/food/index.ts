import { lazy } from 'react'

/** Scènes lourdes chargées à la demande (React.lazy) — ADR 0004. */
export const FoodHero = lazy(() => import('./FoodHero'))
export const FoodRain = lazy(() => import('./FoodRain'))
export const DispatchScooter = lazy(() => import('./DispatchScooter'))
export const PaymentCoin = lazy(() => import('./PaymentCoin'))
export const VoteBurst = lazy(() => import('./VoteBurst'))

export { FoodLoader } from './FoodLoader'
export { EmptyPlate } from './EmptyPlate'
export { ReadyBell } from './ReadyBell'
export { flyToCart } from './AddToCartFlight'
