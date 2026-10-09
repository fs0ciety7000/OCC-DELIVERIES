import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AvatarStack } from '../Avatar'
import { Button } from '../Button'
import { Money } from '../Money'
import { Stepper } from '../Stepper'

describe('Button', () => {
  it('déclenche onClick et applique la variante', async () => {
    const onClick = vi.fn()
    render(<Button onClick={onClick}>Commander</Button>)
    const btn = screen.getByRole('button', { name: 'Commander' })
    expect(btn).toHaveAttribute('data-variant', 'primary')
    expect(btn).toHaveAttribute('type', 'button')
    await userEvent.click(btn)
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('est désactivé et occupé en chargement', async () => {
    const onClick = vi.fn()
    render(
      <Button loading variant="secondary" onClick={onClick}>
        Envoyer
      </Button>,
    )
    const btn = screen.getByRole('button', { name: /Envoyer/ })
    expect(btn).toBeDisabled()
    expect(btn).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByRole('status', { name: 'Chargement' })).toBeInTheDocument()
    await userEvent.click(btn)
    expect(onClick).not.toHaveBeenCalled()
  })
})

describe('Money', () => {
  const norm = (s: string | null) => (s ?? '').replace(/[\u00a0\u202f]/g, ' ')
  it('affiche des centimes en euros', () => {
    const { container } = render(<Money cents={1250} />)
    expect(norm(container.textContent)).toBe('12,50 €')
    expect(container.firstChild).toHaveClass('tabular')
  })
  it('affiche un signe pour les suppléments', () => {
    const { container } = render(<Money cents={150} signed />)
    expect(norm(container.textContent)).toBe('+ 1,50 €')
  })
})

describe('Stepper', () => {
  it("marque l'étape courante et les étapes terminées", () => {
    render(<Stepper status="ordering" />)
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(5)
    expect(items[2]).toHaveAttribute('aria-current', 'step')
    expect(items[0]).toHaveAttribute('data-state', 'done')
    expect(items[4]).toHaveAttribute('data-state', 'todo')
    expect(screen.getByText('Commande')).toBeInTheDocument()
  })
  it('tout est terminé quand la party est close', () => {
    render(<Stepper status="closed" />)
    screen.getAllByRole('listitem').forEach((li) => expect(li).toHaveAttribute('data-state', 'done'))
  })
})

describe('AvatarStack', () => {
  it('donne des couleurs distinctes à deux collègues de même couleur, et se resserre à 24 px', () => {
    const { container } = render(
      <AvatarStack
        size={24}
        users={[
          { id: 'u1', name: 'Bob Dupont', color: '#FF6A3D' },
          { id: 'u2', name: 'Chloé Lambert', color: '#ff6a3d' },
        ]}
      />,
    )
    const [bob, chloe] = Array.from(container.querySelectorAll<HTMLElement>('[data-avatar-user]'))
    expect(bob!.style.backgroundColor).not.toBe(chloe!.style.backgroundColor)
    expect(screen.getByRole('group')).toHaveClass('-space-x-1')
  })
})
