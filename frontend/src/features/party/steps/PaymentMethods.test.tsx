import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { PaymentQR } from '@/lib/types'
import { availableMethods, declareLabel, METHOD_LABELS, methodHint, orderForDevice, payoutMethods } from '../labels'
import { MethodDetails, MethodMark, MethodTiles } from './PaymentMethods'

const qr: PaymentQR = {
  amount: 1240,
  reference: 'OCC K7M2QX Alice',
  beneficiary: 'Bob Martin',
  epc: 'BCD\n002\n1\nSCT\n\nBob Martin\nBE71096123456769\nEUR12.40\n\n\nOCC K7M2QX Alice',
  iban: 'BE71096123456769',
  links: [
    { kind: 'revolut', label: 'Revolut', url: 'https://revolut.me/bobm?amount=1240&currency=EUR&note=OCC+K7M2QX+Alice', amountPrefilled: true },
    { kind: 'paypal', label: 'PayPal', url: 'https://paypal.me/bob/12.40EUR', amountPrefilled: true },
    { kind: 'link', label: 'lydia-app.com', url: 'https://lydia-app.com/collect/x', amountPrefilled: false },
  ],
  methods: ['qr', 'revolut', 'paypal', 'link', 'cash', 'later'],
}

describe('moyens de paiement — liens avec montant', () => {
  it.each([
    ['revolut', /Payer 12,40\s€ avec Revolut/, 'https://revolut.me/bobm?amount=1240&currency=EUR&note=OCC+K7M2QX+Alice'],
    ['paypal', /Payer 12,40\s€ avec PayPal/, 'https://paypal.me/bob/12.40EUR'],
  ] as const)('%s : bouton avec le montant exact', (method, name, href) => {
    render(<MethodDetails method={method} qr={qr} />)
    expect(screen.getByRole('link', { name })).toHaveAttribute('href', href)
  })

  it('lien libre sans montant : avertissement', () => {
    render(<MethodDetails method="link" qr={qr} />)
    expect(screen.getByRole('link', { name: /Ouvrir lydia-app.com/ })).toHaveAttribute('href', 'https://lydia-app.com/collect/x')
    expect(screen.getByText(/ne pré-remplit pas le montant/)).toBeInTheDocument()
  })

  it('une tuile par wallet, avec indication du montant', () => {
    const methods = availableMethods(qr)
    const hints = Object.fromEntries(methods.map((m) => [m, methodHint(m, qr)]))
    render(<MethodTiles methods={methods} value="qr" onChange={() => {}} hints={hints} />)
    const group = screen.getByRole('radiogroup', { name: 'Moyen de remboursement' })
    expect(within(group).getByRole('radio', { name: /Revolut.*Montant pré-rempli/ })).toBeInTheDocument()
    expect(within(group).getByRole('radio', { name: /PayPal.*Montant pré-rempli/ })).toBeInTheDocument()
    expect(within(group).getByRole('radio', { name: /Virement \(QR\).*Scanne le QR avec ton app bancaire \(montant déjà rempli\)/ })).toBeInTheDocument()
    expect(within(group).queryByRole('radio', { name: /Wero|Bancontact/ })).not.toBeInTheDocument()
  })

  it('repli sans `methods` : les liens deviennent des tuiles', () => {
    expect(availableMethods({ ...qr, methods: [] })).toEqual(['qr', 'revolut', 'paypal', 'link', 'cash', 'later'])
  })

  it('mobile : les liens pré-remplis passent devant le QR', () => {
    expect(orderForDevice(qr.methods, qr, true)).toEqual(['revolut', 'paypal', 'qr', 'link', 'cash', 'later'])
    expect(orderForDevice(qr.methods, qr, false)).toEqual(qr.methods)
  })
})

describe('moyens de paiement — QR', () => {
  it('desktop : QR EPC affiché en grand + copie IBAN / montant / communication', () => {
    render(<MethodDetails method="qr" qr={qr} />)
    expect(screen.getByRole('img', { name: /QR virement SEPA de 12,40\s€ vers Bob Martin/ })).toBeInTheDocument()
    expect(screen.getByText('Ouvre ton app bancaire et scanne')).toBeInTheDocument()
    expect(screen.getByText('BE71 0961 2345 6769')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Copier iban/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Copier communication/i })).toBeInTheDocument()
  })

  it('mobile : QR derrière « Afficher le QR pour un collègue »', async () => {
    render(<MethodDetails method="qr" qr={qr} mobile />)
    expect(screen.queryByRole('img', { name: /QR virement SEPA/ })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Afficher le QR pour un collègue' }))
    expect(screen.getByRole('img', { name: /QR virement SEPA/ })).toBeInTheDocument()
  })
})

describe('Wero / Bancontact Pay retirés', () => {
  it('anciens paiements : libellé conservé, sans tuile ni détails', () => {
    expect(METHOD_LABELS.wero).toBe('Wero')
    expect(METHOD_LABELS.bancontact).toBe('Bancontact Pay')
    render(<MethodMark method="wero" />)
    expect(availableMethods(undefined)).toEqual(['cash', 'later'])
    expect(declareLabel('qr')).toBe("J'ai payé par virement")
  })

  it('le QR virement rappelle que l’app bancaire suffit', () => {
    render(<MethodDetails method="qr" qr={qr} />)
    expect(screen.getByText(/pas besoin de Wero/)).toBeInTheDocument()
  })
})

describe('payoutMethods', () => {
  it('liste les moyens visibles par les collègues', () => {
    const base = { id: 'x', collectionId: '', collectionName: 'payout_profiles', created: '', updated: '', user: 'u', holder_name: '', iban: '', bic: '', revolut_tag: '', paypal_me: '', payment_link: '' }
    expect(payoutMethods(null)).toEqual([])
    expect(payoutMethods({ ...base, iban: 'BE71', revolut_tag: 'bob' })).toEqual(['qr', 'revolut'])
  })
})
