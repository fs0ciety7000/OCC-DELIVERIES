import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { dispatchHeadline, etaEstimate, stepAnnouncement } from './sceneText'
import { StepTransition } from './StepTransition'

describe('textes des scènes', () => {
  it('annonce « Étape n sur 5 : … »', () => {
    expect(stepAnnouncement('lobby')).toBe('Étape 1 sur 5 : Salon')
    expect(stepAnnouncement('ordering')).toBe('Étape 3 sur 5 : Commande')
    expect(stepAnnouncement('paying')).toBe('Étape 5 sur 5 : Paiement')
    expect(stepAnnouncement('closed')).toBe('Commande terminée')
    expect(stepAnnouncement('cancelled')).toBe('Commande annulée')
  })

  it('estime l’arrivée d’après eta_min / eta_max, sinon rien', () => {
    expect(etaEstimate(30, 40)).toBe('arrivée estimée ~35 min')
    expect(etaEstimate(25, 35)).toBe('arrivée estimée ~30 min')
    expect(etaEstimate(0, 45)).toBe('arrivée estimée ~45 min')
    expect(etaEstimate(22)).toBe('arrivée estimée ~20 min')
    expect(etaEstimate(1, 2)).toBe('arrivée estimée ~5 min')
    expect(etaEstimate(0, 0)).toBeNull()
    expect(etaEstimate()).toBeNull()
  })

  it('titre selon le mode d’envoi', () => {
    expect(dispatchHeadline('ubereats')).toBe('Commande envoyée via Uber Eats')
    expect(dispatchHeadline('phone')).toBe('Commande passée par téléphone')
    expect(dispatchHeadline('export')).toBe('Commande exportée')
  })
})

describe('StepTransition', () => {
  it("n'annonce rien au premier rendu puis annonce la nouvelle étape", () => {
    const { container, rerender } = render(
      <StepTransition status="voting">
        <p>Vote</p>
      </StepTransition>,
    )
    const live = container.querySelector('[aria-live="polite"]')!
    expect(live.textContent).toBe('')
    rerender(
      <StepTransition status="ordering">
        <p>Commande</p>
      </StepTransition>,
    )
    expect(live.textContent).toBe('Étape 3 sur 5 : Commande')
    // Mode « wait » : l'ancienne étape sort avant que la nouvelle n'entre.
    expect(screen.getByText('Vote')).toBeInTheDocument()
  })
})
