import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

describe('App', () => {
  it("affiche l'accueil sans backend", async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('offline'))))
    const { App } = await import('./App')
    render(<App />)
    expect(await screen.findByRole('heading', { level: 1, name: /Qu'est-ce qu'on mange/ })).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /Lancer une commande/ }).length).toBeGreaterThan(0)
    expect(screen.getByLabelText(/Code de la commande/)).toBeInTheDocument()
    vi.unstubAllGlobals()
  })
})
