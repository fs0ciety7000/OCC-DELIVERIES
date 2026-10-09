import { fireEvent, render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { afterEach, describe, expect, it } from 'vitest'
import { isActiveStatus, readLastParty, rememberParty } from '@/lib/lastParty'
import { ResumeBanner, ResumeHero } from './ActiveParties'
import { stepText, type ActiveParty } from './resume'

const one: ActiveParty = { id: 'p1', title: 'Midi du vendredi', status: 'ordering', restaurant: { id: 'r1', name: 'Pizza Nonna', emoji: '🍕', cover: '', cover_url: '' } }
const two: ActiveParty = { id: 'p2', title: 'Soirée', status: 'voting' }

function renderAt(ui: React.ReactNode) {
  const router = createMemoryRouter([{ path: '/', element: ui }, { path: '/party/:id', element: <p>salle</p> }])
  return render(<RouterProvider router={router} />)
}

describe('stepText', () => {
  it('donne l’étape et le statut', () => {
    expect(stepText('lobby')).toBe('Étape 1/5 · Salon')
    expect(stepText('ordering')).toBe('Étape 3/5 · Commande')
    expect(stepText('paying')).toBe('Étape 5/5 · Paiement')
  })
})

describe('ResumeBanner', () => {
  it('rien sans commande en cours', () => {
    const { container } = renderAt(<ResumeBanner parties={[]} variant="dock" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('une commande : lien direct « Reprendre »', async () => {
    renderAt(<ResumeBanner parties={[one]} variant="dock" />)
    expect(screen.getByRole('complementary', { name: 'Commande en cours' })).toBeInTheDocument()
    const link = screen.getByRole('link', { name: 'Reprendre la commande « Midi du vendredi » — Commande en cours' })
    expect(link).toHaveAttribute('href', '/party/p1')
    expect(screen.getByText('Étape 3/5 · Commande')).toBeInTheDocument()
    fireEvent.click(link)
    expect(await screen.findByText('salle')).toBeInTheDocument()
  })

  it('plusieurs commandes : feuille de choix', async () => {
    renderAt(<ResumeBanner parties={[one, two]} variant="header" />)
    fireEvent.click(screen.getByRole('button', { name: '2 commandes en cours — choisir' }))
    const sheet = await screen.findByRole('dialog', { name: 'Mes commandes en cours' })
    expect(sheet).toHaveTextContent('Midi du vendredi')
    expect(sheet).toHaveTextContent('Vote en cours')
    expect(screen.getByRole('link', { name: /Soirée/ })).toHaveAttribute('href', '/party/p2')
  })

  it('ResumeHero met la commande en avant', () => {
    renderAt(<ResumeHero party={one} />)
    expect(screen.getByRole('link', { name: /Reprendre la commande « Midi du vendredi »/ })).toHaveAttribute('href', '/party/p1')
    expect(screen.getByText(/Pizza Nonna/)).toBeInTheDocument()
  })
})

describe('lastParty (localStorage)', () => {
  afterEach(() => localStorage.clear())

  it('mémorise la commande active et l’oublie quand elle se termine', () => {
    rememberParty('u1', { id: 'p1', title: 'Midi', status: 'voting', code: 'K7M2QX' })
    expect(readLastParty('u1')).toMatchObject({ id: 'p1', status: 'voting', code: 'K7M2QX' })
    expect(readLastParty('u2')).toBeNull()
    rememberParty('u1', { id: 'p1', title: 'Midi', status: 'closed', code: 'K7M2QX' })
    expect(readLastParty('u1')).toBeNull()
  })

  it('résiste à un contenu illisible', () => {
    localStorage.setItem('occ:lastParty:u1', '{oops')
    expect(readLastParty('u1')).toBeNull()
    expect(isActiveStatus('paying')).toBe(true)
    expect(isActiveStatus('cancelled')).toBe(false)
  })
})
