import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, LogOut } from 'lucide-react'
import { useState, type ChangeEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Avatar, Badge, Button, Card, CardBody, Field, Input, Segmented, Skeleton } from '@/components/ui'
import { MethodMark } from '@/features/party/steps/PaymentMethods'
import { payoutApi, usersApi, type PayoutProfileInput } from '@/lib/api'
import { logout, useAuth } from '@/lib/auth'
import { cn } from '@/lib/cn'
import { AVATAR_COLORS } from '@/lib/colors'
import { errorMessage } from '@/lib/errors'
import { formatIban, isValidBic, isValidIban, normalizeIban } from '@/lib/iban'
import { qk } from '@/lib/queryKeys'
import { useTheme, type ThemePref } from '@/lib/theme'
import type { PayoutProfile, User } from '@/lib/types'
import { normalizePaymentLink, normalizePayPalMe, normalizeRevolutTag } from '@/lib/wallet'
import { OrderHistory } from './OrderHistory'
import { panelId, tabId, type ProfileTab } from './tabs'
import { MotionPrefRow } from './MotionPrefRow'
import { ProfileTabs } from './ProfileTabs'
import { DeleteAccountCard, SecurityCard } from './SecurityCard'
import { NotificationsCard } from '@/features/notifications/NotificationsCard'
import { VerifyEmailBanner } from './VerifyEmailBanner'
import { UpgradeCard } from '@/features/teams/UpgradeCard'

export function ProfilePage() {
  const { user } = useAuth()
  const navigate = useNavigate()
  const { pref, setPref } = useTheme()
  // Onglets : « Mes commandes » par défaut, « Mes infos » via ?onglet=infos (lien partageable).
  const [params, setParams] = useSearchParams()
  const tab: ProfileTab = params.get('onglet') === 'infos' ? 'infos' : 'commandes'
  if (!user) return null
  return (
    <div className="mx-auto max-w-[680px] space-y-6">
      <header className="flex items-center gap-4">
        <Avatar user={user} size={56} />
        <div className="min-w-0 flex-1">
          <h1 className="truncate font-display text-[32px] leading-9 font-bold">{user.name || 'Mon profil'}</h1>
          <p className="truncate text-sm text-muted">{user.is_guest ? 'Invité·e — sans compte' : user.email}</p>
        </div>
        <Button
          variant="secondary"
          size="sm"
          leftIcon={<LogOut className="size-4" />}
          onClick={() => {
            logout()
            toast('À bientôt !')
            navigate('/')
          }}
        >
          <span className="max-sm:sr-only">Se déconnecter</span>
        </Button>
      </header>
      <VerifyEmailBanner user={user} />
      <ProfileTabs value={tab} onChange={(t) => setParams(t === 'infos' ? { onglet: 'infos' } : {}, { replace: true })} />
      {tab === 'commandes' ? (
        <div role="tabpanel" id={panelId('commandes')} aria-labelledby={tabId('commandes')}>
          <OrderHistory userId={user.id} />
        </div>
      ) : (
      <div role="tabpanel" id={panelId('infos')} aria-labelledby={tabId('infos')} className="space-y-6">
      {user.is_guest && <UpgradeCard user={user} />}
      <IdentityCard key={user.id} user={user} />
      {/* Invité·e : pas de coordonnées de remboursement ni de sécurité avant de créer son compte (refusé côté serveur). */}
      {!user.is_guest && <PayoutCard userId={user.id} />}
      {!user.is_guest && <SecurityCard user={user} />}
      <NotificationsCard userId={user.id} guest={!!user.is_guest} />
      <Card>
        <CardBody className="space-y-3">
          <h2 className="font-display text-lg font-semibold">Apparence</h2>
          <Segmented<ThemePref>
            label="Thème"
            value={pref}
            onChange={setPref}
            className="w-full"
            options={[
              { value: 'dark', label: 'Sombre' },
              { value: 'light', label: 'Clair' },
              { value: 'system', label: 'Système' },
            ]}
          />
          <MotionPrefRow />
        </CardBody>
      </Card>
      {!user.is_guest && <DeleteAccountCard />}
      <Button
        variant="danger"
        block
        leftIcon={<LogOut className="size-4" />}
        onClick={() => {
          logout()
          toast('À bientôt !')
          navigate('/')
        }}
      >
        Se déconnecter
      </Button>
      </div>
      )}
    </div>
  )
}

function IdentityCard({ user }: { user: User }) {
  const [name, setName] = useState(user.name ?? '')
  const [color, setColor] = useState(user.color ?? '')
  const save = useMutation({
    mutationFn: () => usersApi.update(user.id, { name: name.trim(), color }),
    onSuccess: () => toast.success('Profil enregistré'),
    onError: (e) => toast.error(errorMessage(e)),
  })
  const dirty = name.trim() !== (user.name ?? '') || color !== (user.color ?? '')
  return (
    <Card>
      <CardBody>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (name.trim()) save.mutate()
          }}
        >
          <h2 className="font-display text-lg font-semibold">Identité</h2>
          <Field label="Nom affiché" error={!name.trim() ? 'Ton nom ne peut pas être vide.' : undefined}>
            {(p) => <Input {...p} value={name} maxLength={60} onChange={(e) => setName(e.target.value)} autoComplete="name" />}
          </Field>
          <fieldset className="space-y-2">
            <legend className="text-sm font-medium">Couleur d'avatar</legend>
            <div className="flex flex-wrap gap-2">
              {AVATAR_COLORS.map((c) => (
                <button
                  key={c}
                  type="button"
                  onClick={() => setColor(c)}
                  aria-label={`Couleur ${c}`}
                  aria-pressed={color.toUpperCase() === c}
                  className={cn('grid size-11 place-items-center rounded-full ring-2 ring-offset-2 ring-offset-surface transition-shadow', color.toUpperCase() === c ? 'ring-fg' : 'ring-transparent')}
                  style={{ backgroundColor: c }}
                >
                  {color.toUpperCase() === c && <Check className="size-5 text-ink" />}
                </button>
              ))}
            </div>
          </fieldset>
          <Button type="submit" variant="secondary" disabled={!dirty || !name.trim()} loading={save.isPending}>
            Enregistrer
          </Button>
        </form>
      </CardBody>
    </Card>
  )
}

function PayoutCard({ userId }: { userId: string }) {
  const profile = useQuery({ queryKey: qk.payout(userId), queryFn: () => payoutApi.mine(userId) })
  if (profile.isPending) return <Skeleton className="h-96 rounded-lg" />
  if (profile.isError)
    return (
      <Card>
        <CardBody className="text-sm text-danger">{errorMessage(profile.error)}</CardBody>
      </Card>
    )
  return <PayoutForm key={profile.data?.updated ?? 'new'} userId={userId} profile={profile.data} />
}

function PayoutForm({ userId, profile }: { userId: string; profile: PayoutProfile | null }) {
  const qc = useQueryClient()
  const [v, setV] = useState<PayoutProfileInput>({
    holder_name: profile?.holder_name ?? '',
    iban: profile?.iban ? formatIban(profile.iban) : '',
    bic: profile?.bic ?? '',
    revolut_tag: profile?.revolut_tag ?? '',
    paypal_me: profile?.paypal_me ?? '',
    payment_link: profile?.payment_link ?? '',
  })
  const set = (k: keyof PayoutProfileInput) => (e: ChangeEvent<HTMLInputElement>) => setV((s) => ({ ...s, [k]: e.target.value }))

  const errors = {
    iban: v.iban && !isValidIban(v.iban) ? 'IBAN invalide (vérifie les chiffres).' : undefined,
    bic: !isValidBic(v.bic) ? 'BIC invalide (8 ou 11 caractères).' : undefined,
    revolut_tag: normalizeRevolutTag(v.revolut_tag) === null ? 'Revtag invalide (ex. @jdoe ou revolut.me/jdoe).' : undefined,
    paypal_me: normalizePayPalMe(v.paypal_me) === null ? 'Nom PayPal.me invalide (ex. jdoe ou paypal.me/jdoe).' : undefined,
    payment_link: normalizePaymentLink(v.payment_link) === null ? 'Lien invalide (ex. https://…).' : undefined,
  }
  const hasErrors = Object.values(errors).some(Boolean)

  const save = useMutation({
    mutationFn: () =>
      payoutApi.save(userId, profile?.id ?? null, {
        holder_name: v.holder_name.trim(),
        iban: normalizeIban(v.iban),
        bic: v.bic.replace(/\s/g, '').toUpperCase(),
        revolut_tag: normalizeRevolutTag(v.revolut_tag) ?? '',
        paypal_me: normalizePayPalMe(v.paypal_me) ?? '',
        payment_link: normalizePaymentLink(v.payment_link) ?? '',
      }),
    onSuccess: (p) => {
      qc.setQueryData(qk.payout(userId), p)
      toast.success('Coordonnées de remboursement enregistrées')
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <Card>
      <CardBody>
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (!hasErrors) save.mutate()
          }}
        >
          <div className="space-y-1">
            <h2 className="font-display text-lg font-semibold">Recevoir des remboursements</h2>
            <p className="text-sm text-muted">Quand tu avances la commande, tes collègues te remboursent avec ces infos (sans elles, uniquement en espèces). Elles restent privées : seul le QR de paiement est partagé, et uniquement avec les membres de la commande.</p>
          </div>

          <section className="space-y-3" aria-labelledby="h-sepa">
            <h3 id="h-sepa" className="flex items-center gap-2 font-semibold">
              <MethodMark method="qr" /> Virement (QR SEPA) <Badge variant="brand">Recommandé</Badge>
            </h3>
            <p className="text-sm text-muted">
              Tes collègues scannent un QR avec leur app bancaire (KBC, BNP Paribas Fortis, ING, Belfius, Argenta…) — <strong className="text-fg">montant et communication déjà remplis</strong>.
            </p>
            <Field label="Titulaire du compte" optional>
              {(p) => <Input {...p} value={v.holder_name} onChange={set('holder_name')} autoComplete="name" />}
            </Field>
            <div className="grid gap-3 sm:grid-cols-[1fr_160px]">
              <Field label="IBAN" optional error={errors.iban} hint={v.iban && !errors.iban ? <span className="text-success">IBAN valide ✓</span> : undefined}>
                {(p) => (
                  <Input
                    {...p}
                    value={v.iban}
                    onChange={(e) => setV((s) => ({ ...s, iban: formatIban(e.target.value) }))}
                    placeholder="BE71 0961 2345 6769"
                    autoComplete="off"
                    spellCheck={false}
                    className="tabular uppercase"
                  />
                )}
              </Field>
              <Field label="BIC" optional error={errors.bic}>
                {(p) => <Input {...p} value={v.bic} onChange={set('bic')} placeholder="GKCCBEBB" className="uppercase" autoComplete="off" />}
              </Field>
            </div>
          </section>

          <section className="space-y-3 border-t border-border pt-5" aria-labelledby="h-links">
            <h3 id="h-links" className="flex items-center gap-2 font-semibold">
              <MethodMark method="revolut" /> <MethodMark method="paypal" /> Liens de paiement
            </h3>
            <p className="text-sm text-muted">Chaque collègue reçoit un lien avec <strong className="text-fg">son montant</strong> déjà rempli.</p>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Revtag Revolut" optional error={errors.revolut_tag} hint="Ton @revtag ou ton lien revolut.me.">
                {(p) => <Input {...p} value={v.revolut_tag} onChange={set('revolut_tag')} placeholder="@toi" autoComplete="off" autoCapitalize="none" spellCheck={false} />}
              </Field>
              <Field label="PayPal.me" optional error={errors.paypal_me} hint="Ton nom PayPal.me ou ton lien.">
                {(p) => <Input {...p} value={v.paypal_me} onChange={set('paypal_me')} placeholder="toi" autoComplete="off" autoCapitalize="none" spellCheck={false} />}
              </Field>
            </div>
            <Field label="Autre lien de paiement" optional error={errors.payment_link} hint="Lydia/Sumeria, Wise Business… (montant pré-rempli seulement si le service le permet).">
              {(p) => <Input {...p} type="url" inputMode="url" value={v.payment_link} onChange={set('payment_link')} placeholder="https://…" autoCapitalize="none" spellCheck={false} />}
            </Field>
          </section>

          <Button type="submit" block size="lg" disabled={hasErrors} loading={save.isPending}>
            Enregistrer
          </Button>
        </form>
      </CardBody>
    </Card>
  )
}
