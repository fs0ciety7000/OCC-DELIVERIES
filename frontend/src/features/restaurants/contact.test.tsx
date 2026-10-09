import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DispatchSheet } from '@/features/party/steps/DispatchSheet'
import type { Dispatch, Party } from '@/lib/types'
import { OsmAttribution } from './OsmAttribution'

const osm = { provider: 'osm', url: 'https://www.openstreetmap.org/node/42', fields: ['phone'], checked_at: '2026-10-09' }

describe('OsmAttribution', () => {
  it('crédite OpenStreetMap quand un champ en vient', () => {
    render(<OsmAttribution restaurant={{ enriched_from: osm }} />)
    expect(screen.getByText(/Coordonnées : ©/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'contributeurs OpenStreetMap' })).toHaveAttribute('href', 'https://www.openstreetmap.org/copyright')
  })
  it('ne montre rien sans enrichissement', () => {
    const { container } = render(<OsmAttribution restaurant={{ enriched_from: null }} />)
    expect(container).toBeEmptyDOMElement()
  })
})

describe('DispatchSheet (téléphone)', () => {
  it('met le numéro du restaurant en avant', () => {
    const dispatch: Dispatch = { method: 'phone', url: 'tel:+3265352964', cartText: '1× Ramen', instructions: ['Appelez « Tomo » au +32 65 35 29 64.'] }
    render(
      <DispatchSheet
        dispatch={dispatch}
        party={{ id: 'p1', code: 'K7M2QX' } as Party}
        phone="+3265352964"
        restaurant={{ name: 'Tomo', enriched_from: osm }}
        onClose={() => {}}
      />,
    )
    expect(screen.getByText('Numéro de Tomo')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '+32 65 35 29 64' })).toHaveAttribute('href', 'tel:+3265352964')
    expect(screen.getByRole('link', { name: 'Appeler le +32 65 35 29 64' })).toHaveAttribute('href', 'tel:+3265352964')
    expect(screen.getAllByText(/Coordonnées : ©/).length).toBeGreaterThan(0)
  })
})
