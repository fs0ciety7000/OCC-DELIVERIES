import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it } from 'vitest'
import { pb } from '@/lib/pb'
import { RequireAdmin } from './AdminLayout'
import { BarChart } from './BarChart'

function fakeLogin(role: 'user' | 'admin') {
  // jeton non signé mais valide côté client (exp lointain)
  const payload = btoa(JSON.stringify({ exp: 4102444800, id: 'u1', type: 'auth' }))
  pb.authStore.save(`h.${payload}.s`, { id: 'u1', collectionId: 'c', collectionName: 'users', name: 'Ana', role } as never)
}

afterEach(() => pb.authStore.clear())

describe('RequireAdmin', () => {
  it('bloque un utilisateur sans le rôle admin', () => {
    fakeLogin('user')
    render(
      <MemoryRouter>
        <RequireAdmin>
          <p>secret</p>
        </RequireAdmin>
      </MemoryRouter>,
    )
    expect(screen.getByRole('heading', { name: 'Accès réservé' })).toBeInTheDocument()
    expect(screen.queryByText('secret')).not.toBeInTheDocument()
  })

  it('laisse passer un admin', () => {
    fakeLogin('admin')
    render(
      <MemoryRouter>
        <RequireAdmin>
          <p>secret</p>
        </RequireAdmin>
      </MemoryRouter>,
    )
    expect(screen.getByText('secret')).toBeInTheDocument()
  })
})

describe('BarChart', () => {
  it('rend une barre par valeur non nulle et un tableau accessible', () => {
    const { container } = render(
      <BarChart
        label="Commandes par jour"
        data={[
          { label: '1 oct.', title: 'mer. 1 oct.', value: 0 },
          { label: '2 oct.', title: 'jeu. 2 oct.', value: 3 },
          { label: '3 oct.', title: 'ven. 3 oct.', value: 1 },
        ]}
      />,
    )
    expect(screen.getByRole('img', { name: /Commandes par jour — maximum 3/ })).toBeInTheDocument()
    expect(container.querySelectorAll('path.fill-brand')).toHaveLength(2)
    expect(screen.getByRole('table', { hidden: true })).toHaveTextContent('jeu. 2 oct.3')
  })
})
