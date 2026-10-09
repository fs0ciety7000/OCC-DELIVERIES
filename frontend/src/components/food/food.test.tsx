import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { flyToCart } from './AddToCartFlight'
import { DispatchScooter } from './DispatchScooter'
import { EmptyPlate } from './EmptyPlate'
import { FoodHero } from './FoodHero'
import { FoodLoader } from './FoodLoader'
import { FoodRain } from './FoodRain'
import { PaymentCoin } from './PaymentCoin'
import { ReadyBell } from './ReadyBell'
import { VoteBurst } from './VoteBurst'

describe('scènes culinaires (smoke, jsdom)', () => {
  it('rendent sans planter', () => {
    render(
      <div>
        <FoodHero title="Qu'est-ce qu'on mange ?" />
        <FoodLoader label="Chargement du menu" />
        <FoodLoader variant="ramen" />
        <EmptyPlate />
        <VoteBurst trigger={1} />
        <ReadyBell userId="u1" />
        <DispatchScooter />
        <PaymentCoin trigger={2} />
        <FoodRain count={4} />
      </div>,
    )
    expect(screen.getByRole('heading', { name: /Qu'est-ce qu'on mange/ })).toBeInTheDocument()
    expect(screen.getByText('Chargement du menu')).toBeInTheDocument()
  })

  it("flyToCart n'échoue pas sans cible ni source", () => {
    expect(() => flyToCart(null)).not.toThrow()
    document.body.innerHTML = '<button data-cart-target>Panier</button><div id="src"></div>'
    expect(() => flyToCart(document.getElementById('src'), '🍕')).not.toThrow()
  })
})

describe('scènes culinaires avec animations actives', () => {
  it('exécutent leurs timelines sans planter', () => {
    const original = window.matchMedia
    window.matchMedia = ((query: string) => ({
      matches: query.includes('no-preference'),
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    })) as typeof window.matchMedia
    try {
      const { unmount } = render(
        <div>
          <FoodHero title="Miam" />
          <FoodLoader />
          <FoodLoader variant="ramen" />
          <EmptyPlate />
          <VoteBurst trigger={1} />
          <PaymentCoin />
          <FoodRain count={3} />
        </div>,
      )
      unmount()
    } finally {
      window.matchMedia = original
    }
  })
})
