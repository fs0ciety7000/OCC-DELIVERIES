import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LocateFixed } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { Button, Field, Input, Segmented, Sheet, Textarea } from '@/components/ui'
import { adminApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { centsToEuros, parseDecimal, parseEuros } from '@/lib/euros'
import { qk } from '@/lib/queryKeys'
import type { Restaurant, RestaurantProviderLink } from '@/lib/types'
import { geocode, slugify } from './geocode'
import { Toggle } from './Toggle'

interface FormState {
  name: string
  slug: string
  description: string
  emoji: string
  cover_url: string
  cuisines: string
  address: string
  lat: string
  lng: string
  phone: string
  rating: string
  rating_count: string
  price_level: '1' | '2' | '3' | '4'
  eta_min: string
  eta_max: string
  delivery_fee: string
  min_order: string
  ubereats: string
  takeaway: string
  active: boolean
}

function providerUrl(r: Restaurant | null, id: RestaurantProviderLink['id']): string {
  return r?.providers?.find((p) => p.id === id)?.url ?? ''
}

function initial(r: Restaurant | null): FormState {
  const num = (v: number | undefined) => (v ? String(v).replace('.', ',') : '')
  return {
    name: r?.name ?? '',
    slug: r?.slug ?? '',
    description: r?.description ?? '',
    emoji: r?.emoji ?? '',
    cover_url: r?.cover_url ?? '',
    cuisines: (r?.cuisines ?? []).join(', '),
    address: r?.address ?? '',
    lat: num(r?.lat),
    lng: num(r?.lng),
    phone: r?.phone ?? '',
    rating: num(r?.rating),
    rating_count: r?.rating_count ? String(r.rating_count) : '',
    price_level: String(r?.price_level || 2) as FormState['price_level'],
    eta_min: r?.eta_min ? String(r.eta_min) : '',
    eta_max: r?.eta_max ? String(r.eta_max) : '',
    delivery_fee: r ? centsToEuros(r.delivery_fee) : '',
    min_order: r ? centsToEuros(r.min_order) : '',
    ubereats: providerUrl(r, 'ubereats'),
    takeaway: providerUrl(r, 'takeaway'),
    active: r?.active ?? true,
  }
}

type Errors = Partial<Record<keyof FormState, string>>

/** Valide la saisie et la convertit en champs PocketBase (montants en centimes). */
function toRestaurantData(f: FormState): { data?: Partial<Restaurant>; errors: Errors } {
  const errors: Errors = {}
  const int = (key: keyof FormState, v: string, min = 0) => {
    if (v.trim() === '') return 0
    const n = Number(v.trim())
    if (!Number.isInteger(n) || n < min) errors[key] = 'Nombre entier attendu.'
    return n
  }
  const dec = (key: keyof FormState, v: string, min: number, max: number) => {
    if (v.trim() === '') return 0
    const n = parseDecimal(v)
    if (n === null || n < min || n > max) errors[key] = `Entre ${min} et ${max}.`
    return n ?? 0
  }
  const money = (key: keyof FormState, v: string) => {
    if (v.trim() === '') return 0
    const c = parseEuros(v)
    if (c === null) errors[key] = 'Montant invalide (ex. 2,50).'
    return c ?? 0
  }
  const url = (key: keyof FormState, v: string) => {
    if (v.trim() && !/^https?:\/\//.test(v.trim())) errors[key] = 'Lien en https:// attendu.'
    return v.trim()
  }
  if (!f.name.trim()) errors.name = 'Nom requis.'
  const slug = f.slug.trim() || slugify(f.name)
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(slug)) errors.slug = 'Minuscules, chiffres et tirets.'
  const providers: RestaurantProviderLink[] = []
  const ue = url('ubereats', f.ubereats)
  const ta = url('takeaway', f.takeaway)
  if (ue) providers.push({ id: 'ubereats', url: ue })
  if (ta) providers.push({ id: 'takeaway', url: ta })
  const data: Partial<Restaurant> = {
    name: f.name.trim(),
    slug,
    description: f.description.trim(),
    emoji: f.emoji.trim(),
    cover_url: url('cover_url', f.cover_url),
    cuisines: f.cuisines.split(/[,|;]/).map((c) => c.trim().toLowerCase()).filter(Boolean),
    address: f.address.trim(),
    lat: dec('lat', f.lat, -90, 90),
    lng: dec('lng', f.lng, -180, 180),
    phone: f.phone.trim(),
    rating: dec('rating', f.rating, 0, 5),
    rating_count: int('rating_count', f.rating_count),
    price_level: Number(f.price_level),
    eta_min: int('eta_min', f.eta_min),
    eta_max: int('eta_max', f.eta_max),
    delivery_fee: money('delivery_fee', f.delivery_fee),
    min_order: money('min_order', f.min_order),
    providers,
    active: f.active,
  }
  if (!errors.eta_max && data.eta_min && data.eta_max && data.eta_min > data.eta_max) errors.eta_max = 'Doit dépasser le minimum.'
  return Object.keys(errors).length ? { errors } : { data, errors }
}

export function RestaurantForm({ restaurant, open, onClose, onSaved }: { restaurant: Restaurant | null; open: boolean; onClose: () => void; onSaved?: (r: Restaurant) => void }) {
  const [form, setForm] = useState<FormState>(() => initial(restaurant))
  const [errors, setErrors] = useState<Errors>({})
  const [geocoding, setGeocoding] = useState(false)
  const qc = useQueryClient()
  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setForm((f) => ({ ...f, [k]: v }))

  const save = useMutation({
    mutationFn: (data: Partial<Restaurant>) => adminApi.saveRestaurant(restaurant?.id ?? null, data),
    onSuccess: (r) => {
      toast.success(restaurant ? 'Restaurant enregistré' : 'Restaurant créé')
      void qc.invalidateQueries({ queryKey: qk.admin.all })
      void qc.invalidateQueries({ queryKey: qk.restaurant(r.id) })
      onSaved?.(r)
      onClose()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const { data, errors } = toRestaurantData(form)
    setErrors(errors)
    if (data) save.mutate(data)
  }

  const runGeocode = async () => {
    setGeocoding(true)
    try {
      const hit = await geocode(form.address)
      if (!hit) {
        toast.error('Adresse introuvable. Précise la rue et la ville.')
        return
      }
      setForm((f) => ({ ...f, lat: String(hit.lat).replace('.', ','), lng: String(hit.lng).replace('.', ',') }))
      toast.success('Coordonnées trouvées', { description: hit.label })
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setGeocoding(false)
    }
  }

  const text = (key: keyof FormState, label: string, props: Partial<Parameters<typeof Input>[0]> & { hint?: string; optional?: boolean } = {}) => {
    const { hint, optional, ...rest } = props
    return (
      <Field label={label} hint={hint} optional={optional} error={errors[key]}>
        {(p) => <Input {...p} {...rest} value={form[key] as string} onChange={(e) => set(key, e.target.value as never)} />}
      </Field>
    )
  }

  return (
    <Sheet
      open={open}
      onClose={onClose}
      size="lg"
      title={restaurant ? `Modifier « ${restaurant.name} »` : 'Nouveau restaurant'}
      footer={
        <div className="flex items-center justify-between gap-3">
          <Toggle checked={form.active} onChange={(v) => set('active', v)} label="Visible dans l'app" showLabel />
          <div className="flex gap-2">
            <Button variant="ghost" onClick={onClose}>
              Annuler
            </Button>
            <Button type="submit" form="restaurant-form" loading={save.isPending}>
              Enregistrer
            </Button>
          </div>
        </div>
      }
    >
      <form id="restaurant-form" onSubmit={submit} className="space-y-5" noValidate>
        <fieldset className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_88px]">
          <legend className="sr-only">Identité</legend>
          {text('name', 'Nom', { required: true, 'data-autofocus': true } as never)}
          {text('emoji', 'Emoji', { optional: false, maxLength: 16 })}
          {text('slug', 'Slug', { placeholder: slugify(form.name) || 'chez-mario', hint: "Identifiant unique (sert à l'import)." })}
          <div className="sm:col-span-2">
            <Field label="Description" optional>
              {(p) => <Textarea {...p} value={form.description} onChange={(e) => set('description', e.target.value)} rows={2} />}
            </Field>
          </div>
          <div className="sm:col-span-2">{text('cuisines', 'Cuisines', { placeholder: 'pizza, italien', hint: 'Séparées par des virgules.' })}</div>
          <div className="sm:col-span-2">{text('cover_url', 'Image de couverture (URL)', { optional: true, inputMode: 'url' })}</div>
        </fieldset>

        <fieldset className="space-y-3">
          <legend className="mb-2 font-display text-base font-semibold">Adresse</legend>
          <div className="flex items-end gap-2">
            <div className="min-w-0 flex-1">{text('address', 'Adresse', { placeholder: 'Rue de Nimy 1, 7000 Mons', autoComplete: 'street-address' })}</div>
            <Button variant="secondary" onClick={runGeocode} loading={geocoding} disabled={!form.address.trim()} leftIcon={<LocateFixed className="size-4" />}>
              Géocoder
            </Button>
          </div>
          <div className="grid grid-cols-2 gap-3">
            {text('lat', 'Latitude', { inputMode: 'decimal', placeholder: '50,4542' })}
            {text('lng', 'Longitude', { inputMode: 'decimal', placeholder: '3,9567' })}
          </div>
          <p className="text-xs text-subtle">Géocodage par OpenStreetMap (Nominatim). Sans coordonnées, le resto n'apparaît pas dans « À proximité ».</p>
          {text('phone', 'Téléphone', { inputMode: 'tel', optional: true })}
        </fieldset>

        <fieldset className="space-y-3">
          <legend className="mb-2 font-display text-base font-semibold">Livraison</legend>
          <div className="grid grid-cols-2 gap-3">
            {text('delivery_fee', 'Frais de livraison (€)', { inputMode: 'decimal', placeholder: '2,99' })}
            {text('min_order', 'Minimum de commande (€)', { inputMode: 'decimal', placeholder: '15,00' })}
            {text('eta_min', 'Délai min. (min)', { inputMode: 'numeric' })}
            {text('eta_max', 'Délai max. (min)', { inputMode: 'numeric' })}
            {text('rating', 'Note (0–5)', { inputMode: 'decimal' })}
            {text('rating_count', "Nombre d'avis", { inputMode: 'numeric' })}
          </div>
          <div className="space-y-1.5">
            <p className="text-sm font-medium">Gamme de prix</p>
            <Segmented
              label="Gamme de prix"
              value={form.price_level}
              onChange={(v) => set('price_level', v)}
              options={[
                { value: '1', label: '€' },
                { value: '2', label: '€€' },
                { value: '3', label: '€€€' },
                { value: '4', label: '€€€€' },
              ]}
            />
          </div>
        </fieldset>

        <fieldset className="space-y-3">
          <legend className="mb-2 font-display text-base font-semibold">Plateformes</legend>
          {text('ubereats', 'Lien Uber Eats', { inputMode: 'url', optional: true, placeholder: 'https://www.ubereats.com/be/store/…' })}
          {text('takeaway', 'Lien Takeaway', { inputMode: 'url', optional: true, placeholder: 'https://www.takeaway.com/be-fr/…' })}
        </fieldset>
      </form>
    </Sheet>
  )
}
