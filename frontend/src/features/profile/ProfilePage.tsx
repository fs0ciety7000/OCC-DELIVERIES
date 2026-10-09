import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, ImagePlus, LogOut, Trash2 } from 'lucide-react'
import { useEffect, useState, type ChangeEvent } from 'react'
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
import { checkQrFile, isValidMobile, isValidWeroId, normalizeMobile, normalizePaymentLink, normalizePayPalMe, normalizeRevolutTag, normalizeWeroId } from '@/lib/wallet'
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
      <NotificationsCard userId={user.id} />
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

type QrField = 'wero_qr' | 'bancontact_qr'

function PayoutForm({ userId, profile }: { userId: string; profile: PayoutProfile | null }) {
  const qc = useQueryClient()
  const [v, setV] = useState<PayoutProfileInput>({
    holder_name: profile?.holder_name ?? '',
    iban: profile?.iban ? formatIban(profile.iban) : '',
    bic: profile?.bic ?? '',
    revolut_tag: profile?.revolut_tag ?? '',
    paypal_me: profile?.paypal_me ?? '',
    payment_link: profile?.payment_link ?? '',
    wero_id: profile?.wero_id ?? '',
    bancontact_phone: profile?.bancontact_phone ?? '',
  })
  const [files, setFiles] = useState<Partial<Record<QrField, File | null>>>({})
  const set = (k: keyof PayoutProfileInput) => (e: ChangeEvent<HTMLInputElement>) => setV((s) => ({ ...s, [k]: e.target.value }))

  const errors = {
    iban: v.iban && !isValidIban(v.iban) ? 'IBAN invalide (vérifie les chiffres).' : undefined,
    bic: !isValidBic(v.bic) ? 'BIC invalide (8 ou 11 caractères).' : undefined,
    revolut_tag: normalizeRevolutTag(v.revolut_tag) === null ? 'Revtag invalide (ex. @jdoe ou revolut.me/jdoe).' : undefined,
    paypal_me: normalizePayPalMe(v.paypal_me) === null ? 'Nom PayPal.me invalide (ex. jdoe ou paypal.me/jdoe).' : undefined,
    payment_link: normalizePaymentLink(v.payment_link) === null ? 'Lien invalide (ex. https://…).' : undefined,
    wero_id: !isValidWeroId(v.wero_id) ? 'Numéro de mobile (ex. 0470 12 34 56) ou e-mail.' : undefined,
    bancontact_phone: !isValidMobile(v.bancontact_phone) ? 'Numéro de mobile invalide (ex. 0470 12 34 56).' : undefined,
  }
  const hasErrors = Object.values(errors).some(Boolean)

  const save = useMutation({
    mutationFn: () =>
      payoutApi.save(
        userId,
        profile?.id ?? null,
        {
          holder_name: v.holder_name.trim(),
          iban: normalizeIban(v.iban),
          bic: v.bic.replace(/\s/g, '').toUpperCase(),
          revolut_tag: normalizeRevolutTag(v.revolut_tag) ?? '',
          paypal_me: normalizePayPalMe(v.paypal_me) ?? '',
          payment_link: normalizePaymentLink(v.payment_link) ?? '',
          wero_id: normalizeWeroId(v.wero_id),
          bancontact_phone: v.bancontact_phone.trim() ? normalizeMobile(v.bancontact_phone) : '',
        },
        files,
      ),
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
            <p className="text-sm text-muted">Quand tu avances la commande, tes collègues te remboursent avec ces infos. Elles restent privées : seul le QR de paiement est partagé, et uniquement avec les membres de la commande.</p>
          </div>

          <section className="space-y-3" aria-labelledby="h-sepa">
            <h3 id="h-sepa" className="flex items-center gap-2 font-semibold">
              <MethodMark method="qr" /> Virement (QR SEPA)
            </h3>
            <p className="text-sm text-muted">Recommandé : tes collègues scannent un QR qui remplit le montant et la communication dans leur app bancaire.</p>
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

          <section className="space-y-3 border-t border-border pt-5" aria-labelledby="h-wero">
            <h3 id="h-wero" className="flex items-center gap-2 font-semibold">
              <MethodMark method="wero" /> Wero
            </h3>
            <Field label="Mobile ou e-mail Wero" optional error={errors.wero_id} hint="Celui enregistré dans ton app bancaire (KBC, BNP Paribas Fortis, ING, Belfius…).">
              {(p) => <Input {...p} value={v.wero_id} onChange={set('wero_id')} placeholder="0470 12 34 56 ou toi@mail.be" autoComplete="tel" />}
            </Field>
            <QrUpload
              label="QR « recevoir » Wero"
              help="Dans ton app bancaire : Wero → Recevoir → Mon QR code, puis fais une capture d'écran. Ce QR ne contient pas de montant : tes collègues devront le saisir (préfère ton numéro ou ton IBAN)."
              profile={profile}
              field="wero_qr"
              value={files.wero_qr}
              onChange={(f) => setFiles((s) => ({ ...s, wero_qr: f }))}
            />
          </section>

          <section className="space-y-3 border-t border-border pt-5" aria-labelledby="h-bcp">
            <h3 id="h-bcp" className="flex items-center gap-2 font-semibold">
              <MethodMark method="bancontact" /> Bancontact Pay
            </h3>
            <Field label="Mobile lié à Bancontact Pay" optional error={errors.bancontact_phone}>
              {(p) => <Input {...p} type="tel" value={v.bancontact_phone} onChange={set('bancontact_phone')} placeholder="0470 12 34 56" autoComplete="tel" />}
            </Field>
            <QrUpload
              label="QR « recevoir » Bancontact Pay"
              help="Dans l'app Bancontact Pay : Recevoir de l'argent → Afficher mon QR code, sans montant, puis capture d'écran. Tes collègues devront saisir leur montant."
              profile={profile}
              field="bancontact_qr"
              value={files.bancontact_qr}
              onChange={(f) => setFiles((s) => ({ ...s, bancontact_qr: f }))}
            />
          </section>

          <Button type="submit" block size="lg" disabled={hasErrors} loading={save.isPending}>
            Enregistrer
          </Button>
        </form>
      </CardBody>
    </Card>
  )
}

function QrUpload({
  label,
  help,
  profile,
  field,
  value,
  onChange,
}: {
  label: string
  help: string
  profile: PayoutProfile | null
  field: QrField
  value: File | null | undefined
  onChange: (f: File | null | undefined) => void
}) {
  const existing = useQuery({
    queryKey: ['payoutFile', profile?.id, profile?.[field]],
    queryFn: () => payoutApi.fileUrl(profile!, field),
    enabled: !!profile?.[field],
    staleTime: 60_000,
  })
  const [localUrl, setLocalUrl] = useState<string | null>(null)
  useEffect(() => {
    if (!value) return
    const url = URL.createObjectURL(value)
    // eslint-disable-next-line react-hooks/set-state-in-effect -- synchronise une ressource externe (blob URL)
    setLocalUrl(url)
    return () => URL.revokeObjectURL(url)
  }, [value])
  const preview = value ? localUrl : value === null ? null : (existing.data ?? null)
  const inputId = `qr-${field}`

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">{label}</span>
        <span className="text-xs text-subtle">facultatif</span>
      </div>
      <div className="flex items-start gap-3">
        <div className="grid size-24 shrink-0 place-items-center overflow-hidden rounded-md border border-dashed border-border-strong bg-qr-bg">
          {preview ? <img src={preview} alt={`Aperçu ${label}`} className="size-full object-contain" /> : <ImagePlus aria-hidden className="size-6 text-subtle" />}
        </div>
        <div className="min-w-0 flex-1 space-y-2">
          <p className="text-xs text-muted">{help}</p>
          <div className="flex flex-wrap gap-2">
            <label htmlFor={inputId} className="inline-flex min-h-9 cursor-pointer items-center gap-2 rounded-sm border border-border-strong bg-surface px-3 text-sm font-semibold hover:bg-elevated focus-within:outline-2 focus-within:outline-brand">
              <ImagePlus aria-hidden className="size-4" /> {preview ? 'Remplacer' : 'Téléverser'}
              <input
                id={inputId}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                className="sr-only"
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  e.target.value = ''
                  if (!f) return
                  const err = checkQrFile(f)
                  if (err) toast.error(err)
                  else onChange(f)
                }}
              />
            </label>
            {preview && (
              <Button variant="ghost" size="sm" leftIcon={<Trash2 className="size-4" />} onClick={() => onChange(profile?.[field] ? null : undefined)}>
                Retirer
              </Button>
            )}
            {value && <Badge variant="warning">À enregistrer</Badge>}
            {value === null && <Badge variant="warning">Sera supprimé</Badge>}
          </div>
        </div>
      </div>
    </div>
  )
}
