import { render, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BrandEmblem } from './BrandEmblem'

const SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><g class="t1" fill="var(--logo-t1, #D4A84B)"><path d="M0 0h10v10z" transform="translate(5,5)"/><script>alert(1)</script></g><g class="t3"><circle cx="50" cy="50" r="2"/></g></svg>`

afterEach(() => vi.unstubAllGlobals())

describe('BrandEmblem', () => {
  it('inline l’emblème, enveloppe chaque forme dans une partie animable et retire les scripts', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(SVG, { status: 200 })))
    const { container } = render(<BrandEmblem name="cardor-monogram" className="size-4" />)
    const svg = container.querySelector('svg[data-emblem="cardor-monogram"]')!
    expect(svg).toHaveAttribute('aria-hidden')
    await waitFor(() => expect(svg.querySelectorAll('.part')).toHaveLength(2))
    expect(svg.querySelector('script')).toBeNull()
    // la position d'origine reste sur le groupe parent, la partie est libre d'être animée
    const part = svg.querySelector('.part')!
    expect(part.parentElement).toHaveAttribute('transform', 'translate(5,5)')
    expect(part.firstElementChild).not.toHaveAttribute('transform')
  })

  it('reste vide (sans erreur) si le fichier est introuvable', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('', { status: 404 })))
    const { container } = render(<BrandEmblem name="interactive" />)
    await new Promise((r) => setTimeout(r, 20))
    expect(container.querySelector('[data-emblem="interactive"] g[data-root]')).toBeNull()
  })
})
