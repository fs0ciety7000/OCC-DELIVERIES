import { act, render, renderHook, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatMoney } from '@/lib/format'
import { haptic, HAPTICS } from '@/lib/haptics'
import { withMotion } from '@/lib/gsap'
import { motionReduced, setUserReducedMotion, useMotionPref } from '@/lib/motionPref'
import { useFirstTime } from '@/lib/once'
import { AnimatedMoney } from './AnimatedMoney'
import { flyToAvatar, resetFlightsForTesting, useColleagueFlights, type CartLine } from './ColleagueFlight'
import { counterValue } from './counter'
import { DispatchedScene } from './DispatchedScene'
import { DrawCheck } from './DrawCheck'
import { EmptyBag } from './EmptyBag'
import { LiquidHeart } from './LiquidHeart'
import { PulseOnChange } from './PulseOnChange'
import { WaitingDots } from './WaitingDots'
import { WaitingRider } from './WaitingRider'
import { WinnerReveal } from './WinnerReveal'

/** matchMedia simulé : `full` = animations acceptées, sinon `prefers-reduced-motion: reduce`. */
function mockMotion(full: boolean) {
  const original = window.matchMedia
  window.matchMedia = ((query: string) => ({
    matches: full ? query.includes('no-preference') : query.includes(': reduce'),
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  })) as typeof window.matchMedia
  return () => {
    window.matchMedia = original
  }
}

afterEach(() => {
  localStorage.clear()
  document.documentElement.removeAttribute('data-motion')
})

describe('compteur animé', () => {
  it('reste en centimes entiers à chaque image', () => {
    expect(counterValue(1000, 2000, 0)).toBe(1000)
    expect(counterValue(1000, 2000, 0.5)).toBe(1500)
    expect(counterValue(0, 999, 0.3333)).toBe(333)
    expect(counterValue(1250, 0, 1)).toBe(0)
    expect(counterValue(100, 200, 1.4)).toBe(200)
    expect(counterValue(100, 200, -1)).toBe(100)
    for (let p = 0; p <= 1; p += 0.07) expect(Number.isInteger(counterValue(17, 4321, p))).toBe(true)
  })

  it('affiche le montant formaté (fr-BE) et annonce la valeur finale', () => {
    const restore = mockMotion(false)
    try {
      const { container, rerender } = render(<AnimatedMoney cents={1250} />)
      expect(container.querySelector('[data-counter]')?.textContent).toBe(formatMoney(1250))
      rerender(<AnimatedMoney cents={4520} />)
      // Mouvement réduit : saut direct à la valeur finale.
      expect(container.querySelector('[data-counter]')?.textContent).toBe(formatMoney(4520))
      expect(container.querySelector('.sr-only')?.textContent).toBe(formatMoney(4520))
    } finally {
      restore()
    }
  })

  it('anime sans planter et garde la valeur finale en sr-only', () => {
    const restore = mockMotion(true)
    try {
      const { container, rerender, unmount } = render(<AnimatedMoney cents={0} />)
      rerender(<AnimatedMoney cents={3399} />)
      rerender(<AnimatedMoney cents={1200} />)
      expect(container.querySelector('.sr-only')?.textContent).toBe(formatMoney(1200))
      unmount()
    } finally {
      restore()
    }
  })
})

describe('nouvelles scènes (smoke, jsdom)', () => {
  for (const full of [true, false]) {
    it(`montent et se démontent sans erreur (${full ? 'animations' : 'mouvement réduit'})`, () => {
      const restore = mockMotion(full)
      const onDone = vi.fn()
      try {
        const { rerender, unmount } = render(
          <div>
            <PulseOnChange value={1}>1/3 prêts</PulseOnChange>
            <WaitingDots label="En attente des collègues…" />
            <EmptyBag />
            <DrawCheck />
            <LiquidHeart filled={false} />
            <WaitingRider />
            <DispatchedScene headline="Commande envoyée via Uber Eats" eta="arrivée estimée ~35 min" play />
            <WinnerReveal emoji="🍕" name="Pizzeria Mons" onDone={onDone} />
          </div>,
        )
        rerender(
          <div>
            <PulseOnChange value={2}>2/3 prêts</PulseOnChange>
            <WaitingDots label="En attente des collègues…" />
            <EmptyBag />
            <DrawCheck />
            <LiquidHeart filled />
            <WaitingRider />
            <DispatchedScene headline="Commande envoyée via Uber Eats" eta="arrivée estimée ~35 min" play paused />
            <WinnerReveal emoji="🍕" name="Pizzeria Mons" onDone={onDone} />
          </div>,
        )
        expect(screen.getByText('En attente des collègues…')).toBeInTheDocument()
        expect(screen.getByRole('status')).toHaveTextContent(/Commande envoyée via Uber Eats.*arrivée estimée ~35 min/)
        // Mouvement réduit : pas de révélation, on rend la main tout de suite ; pas de bouton « Passer ».
        if (!full) {
          expect(onDone).toHaveBeenCalled()
          expect(screen.queryByRole('button', { name: "Passer l'animation" })).toBeNull()
        } else {
          expect(screen.getByRole('button', { name: "Passer l'animation" })).toBeInTheDocument()
        }
        unmount()
      } finally {
        restore()
      }
    })
  }

  it('carte « Commande envoyée » statique quand elle a déjà été jouée', () => {
    const restore = mockMotion(true)
    try {
      render(<DispatchedScene headline="Commande passée par téléphone" play={false} />)
      expect(screen.getByRole('status')).toHaveTextContent('Commande passée par téléphone')
      expect(screen.queryByRole('button', { name: "Passer l'animation" })).toBeNull()
    } finally {
      restore()
    }
  })
})

describe('panier vivant', () => {
  beforeEach(() => resetFlightsForTesting())

  it("flyToAvatar n'échoue pas sans cible, ni avec une cible", () => {
    const restore = mockMotion(true)
    try {
      expect(() => flyToAvatar('u2', '🍣')).not.toThrow()
      document.body.innerHTML = '<div data-party-avatars><span data-avatar-user="u2">B</span></div><div data-menu-item="m1"></div>'
      for (let i = 0; i < 6; i++) expect(() => flyToAvatar('u2', '🍣', document.querySelector('[data-menu-item]'))).not.toThrow()
    } finally {
      restore()
      document.body.innerHTML = ''
    }
  })

  it('ne fait rien au premier chargement ni pour mes propres plats', () => {
    const restore = mockMotion(true)
    const emoji = vi.fn(() => '🍕')
    try {
      const line = (id: string, user: string, quantity = 1): CartLine => ({ id, user, quantity, menu_item: 'm1' })
      const { rerender } = renderHook(({ lines }) => useColleagueFlights(lines, 'me', emoji), { initialProps: { lines: [line('a', 'u2')] } })
      expect(emoji).not.toHaveBeenCalled()
      rerender({ lines: [line('a', 'u2'), line('b', 'me')] })
      expect(emoji).not.toHaveBeenCalled()
      rerender({ lines: [line('a', 'u2', 2), line('b', 'me'), line('c', 'u3')] })
      expect(emoji).toHaveBeenCalledTimes(2)
    } finally {
      restore()
    }
  })
})

describe('haptique', () => {
  const original = Object.getOwnPropertyDescriptor(navigator, 'vibrate')
  afterEach(() => {
    if (original) Object.defineProperty(navigator, 'vibrate', original)
    else delete (navigator as { vibrate?: unknown }).vibrate
  })

  it('sans API vibrate : rien, sans erreur', () => {
    delete (navigator as { vibrate?: unknown }).vibrate
    expect(haptic('vote')).toBe(false)
  })

  it('motifs courts, coupés par « Animations réduites », erreurs avalées', () => {
    const vibrate = vi.fn(() => true)
    Object.defineProperty(navigator, 'vibrate', { configurable: true, value: vibrate })
    expect(haptic('vote')).toBe(true)
    expect(vibrate).toHaveBeenLastCalledWith(10)
    haptic('ready')
    expect(vibrate).toHaveBeenLastCalledWith([15, 40, 15])
    haptic('paid')
    expect(vibrate).toHaveBeenLastCalledWith([...HAPTICS.paid])
    setUserReducedMotion(true)
    expect(haptic('vote')).toBe(false)
    expect(vibrate).toHaveBeenCalledTimes(3)
    setUserReducedMotion(false)
    Object.defineProperty(navigator, 'vibrate', {
      configurable: true,
      value: () => {
        throw new Error('bloqué')
      },
    })
    expect(haptic('vote')).toBe(false)
  })
})

describe('préférence « Animations réduites »', () => {
  it('useMotionPref : stockage local, attribut data-motion, combinaison avec le système', () => {
    const restore = mockMotion(true)
    try {
      const { result } = renderHook(() => useMotionPref())
      expect(result.current).toMatchObject({ reduced: false, userReduced: false, systemReduced: false })
      act(() => result.current.setUserReduced(true))
      expect(result.current.reduced).toBe(true)
      expect(localStorage.getItem('occ-motion')).toBe('reduced')
      expect(document.documentElement.getAttribute('data-motion')).toBe('reduced')
      expect(motionReduced()).toBe(true)
      act(() => result.current.setUserReduced(false))
      expect(result.current.reduced).toBe(false)
      expect(localStorage.getItem('occ-motion')).toBeNull()
      expect(document.documentElement.hasAttribute('data-motion')).toBe(false)
    } finally {
      restore()
    }
    const restoreReduced = mockMotion(false)
    try {
      const { result } = renderHook(() => useMotionPref())
      expect(result.current).toMatchObject({ reduced: true, userReduced: false, systemReduced: true })
    } finally {
      restoreReduced()
    }
  })

  it('withMotion joue la version statique quand le réglage est actif', () => {
    const restore = mockMotion(true)
    try {
      const animate = vi.fn()
      const still = vi.fn()
      setUserReducedMotion(true)
      withMotion(animate, still)()
      expect(animate).not.toHaveBeenCalled()
      expect(still).toHaveBeenCalledTimes(1)
      setUserReducedMotion(false)
      withMotion(animate, still)()
      expect(animate).toHaveBeenCalledTimes(1)
    } finally {
      restore()
    }
  })

  it('useFirstTime : vrai une seule fois par clé', () => {
    const a = renderHook(() => useFirstTime('occ-test-once'))
    expect(a.result.current).toBe(true)
    const b = renderHook(() => useFirstTime('occ-test-once'))
    expect(b.result.current).toBe(false)
    const c = renderHook(() => useFirstTime('occ-test-other', false))
    expect(c.result.current).toBe(false)
  })
})
