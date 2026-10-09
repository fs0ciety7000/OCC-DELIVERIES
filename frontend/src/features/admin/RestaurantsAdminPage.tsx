import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { EyeOff, Lock, MapPinOff, Pencil, PhoneOff, Plus, Search, TriangleAlert, UtensilsCrossed } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Badge, Button, buttonClass, Card, EmptyState, Input, Segmented, Skeleton } from '@/components/ui'
import { adminApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { formatPhone, plural } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import type { Restaurant } from '@/lib/types'
import { AdminHeader } from './AdminLayout'
import { fold } from './text'
import { IncompleteMenusCard } from './IncompleteMenusCard'
import { isIncomplete } from './incomplete'
import { RestaurantForm } from './RestaurantForm'
import { Toggle } from './Toggle'

const dayFmt = new Intl.DateTimeFormat('fr-BE', { day: 'numeric', month: 'long' })

function formatDay(value: string): string {
  const d = new Date(value.replace(' ', 'T'))
  return Number.isNaN(d.getTime()) ? value : dayFmt.format(d)
}

type Filter = 'all' | 'active' | 'inactive' | 'stale' | 'incomplete'

const FILTERS: readonly Filter[] = ['all', 'active', 'inactive', 'stale', 'incomplete']

/** Valeur du paramètre `?filtre=` (lien « Voir les restaurants masqués »). */
const INCOMPLETE_PARAM = 'incompletes'

export function RestaurantsAdminPage() {
  const [q, setQ] = useState('')
  const [params, setParams] = useSearchParams()
  const raw = params.get('filtre')
  const filter: Filter = raw === INCOMPLETE_PARAM ? 'incomplete' : FILTERS.includes(raw as Filter) ? (raw as Filter) : 'all'
  const setFilter = (f: Filter) =>
    setParams(
      (p) => {
        if (f === 'all') p.delete('filtre')
        else p.set('filtre', f === 'incomplete' ? INCOMPLETE_PARAM : f)
        return p
      },
      { replace: true },
    )
  const [editing, setEditing] = useState<Restaurant | null | undefined>(undefined)
  const list = useQuery({ queryKey: qk.admin.restaurants, queryFn: adminApi.restaurants })
  const qc = useQueryClient()
  const settings = useQuery({ queryKey: qk.admin.settings, queryFn: adminApi.settings })
  const minItems = settings.data?.minMenuItems ?? 0
  const saveSettings = useMutation({
    mutationFn: adminApi.saveSettings,
    onSuccess: (s) => {
      qc.setQueryData(qk.admin.settings, s)
      toast.success(
        s.minMenuItems > 0
          ? `Restaurants de moins de ${plural(s.minMenuItems, 'plat')} masqués (${s.hiddenRestaurants})`
          : 'Tous les restaurants actifs sont de nouveau listés',
      )
      void qc.invalidateQueries({ queryKey: ['nearby'] })
      void qc.invalidateQueries({ queryKey: qk.config })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const toggle = useMutation({
    mutationFn: ({ r, active }: { r: Restaurant; active: boolean }) => adminApi.saveRestaurant(r.id, { active }),
    onMutate: async ({ r, active }) => {
      await qc.cancelQueries({ queryKey: qk.admin.restaurants })
      qc.setQueryData<Restaurant[]>(qk.admin.restaurants, (old) => old?.map((x) => (x.id === r.id ? { ...x, active } : x)))
    },
    onSuccess: (r) => toast.success(r.active ? `« ${r.name} » est visible` : `« ${r.name} » est masqué`),
    onError: (e) => toast.error(errorMessage(e)),
    onSettled: () => void qc.invalidateQueries({ queryKey: qk.admin.all }),
  })

  const shown = useMemo(() => {
    const needle = fold(q.trim())
    return (list.data ?? []).filter((r) => {
      if (filter === 'active' && !r.active) return false
      if (filter === 'inactive' && r.active) return false
      if (filter === 'stale' && !r.stale_since) return false
      if (filter === 'incomplete' && !(r.active && isIncomplete(r, minItems))) return false
      if (!needle) return true
      return fold(`${r.name} ${r.slug} ${r.address} ${(r.cuisines ?? []).join(' ')}`).includes(needle)
    })
  }, [list.data, q, filter, minItems])

  const counts = { all: list.data?.length ?? 0, active: list.data?.filter((r) => r.active).length ?? 0 }

  return (
    <div>
      <AdminHeader
        title="Restaurants"
        description={list.data ? `${counts.active} visibles sur ${counts.all}` : undefined}
        actions={
          <Button leftIcon={<Plus className="size-4" />} onClick={() => setEditing(null)}>
            Nouveau restaurant
          </Button>
        }
      />
      {settings.data && (
        <IncompleteMenusCard
          settings={settings.data}
          restaurants={list.data}
          saving={saveSettings.isPending}
          onSave={(n) => saveSettings.mutate(n)}
          onShowHidden={() => setFilter('incomplete')}
        />
      )}
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="min-w-0 flex-1">
          <Input type="search" aria-label="Rechercher un restaurant" placeholder="Nom, slug, adresse, cuisine…" value={q} onChange={(e) => setQ(e.target.value)} leftIcon={<Search className="size-4" />} />
        </div>
        <div className="-mx-1 max-w-full overflow-x-auto px-1">
          <Segmented<Filter>
            label="Filtrer par visibilité"
            value={filter}
            onChange={setFilter}
            options={[
              { value: 'all', label: 'Tous' },
              { value: 'active', label: 'Visibles' },
              { value: 'inactive', label: 'Masqués' },
              { value: 'stale', label: 'Obsolètes' },
              { value: 'incomplete', label: 'Incomplets' },
            ]}
          />
        </div>
      </div>

      {list.isPending && (
        <div className="space-y-2">
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} className="h-[76px] rounded-lg" />
          ))}
        </div>
      )}
      {list.isError && <EmptyState tone="danger" emoji="⚠️" title="Chargement impossible" description={errorMessage(list.error)} action={<Button variant="secondary" onClick={() => list.refetch()}>Réessayer</Button>} />}
      {list.data && shown.length === 0 && (
        <EmptyState
          title={list.data.length === 0 ? 'Aucun restaurant' : filter === 'incomplete' && !q ? 'Aucune carte incomplète' : 'Aucun résultat'}
          description={
            list.data.length === 0
              ? 'Crée un restaurant ou importe un fichier JSON / CSV.'
              : filter === 'incomplete' && !q
                ? minItems > 0
                  ? `Tous les restaurants actifs ont au moins ${plural(minItems, 'plat')}.`
                  : 'Le filtre « cartes incomplètes » est désactivé.'
                : 'Essaie un autre mot-clé.'
          }
          action={
            list.data.length === 0 && (
              <Link to="/admin/import" className={buttonClass('secondary')}>
                Importer des menus
              </Link>
            )
          }
        />
      )}

      <ul className="space-y-2" aria-label="Restaurants">
        {shown.map((r) => (
          <li key={r.id}>
            <Card className="flex flex-wrap items-center gap-3 p-3 sm:flex-nowrap sm:p-4">
              <span aria-hidden className="grid size-11 shrink-0 place-items-center rounded-md border border-border bg-elevated text-xl">
                {r.emoji || '🍽️'}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Link to={`/admin/restaurants/${r.id}`} className="truncate font-semibold hover:underline">
                    {r.name}
                  </Link>
                  {!r.active && <Badge>Masqué</Badge>}
                  {r.active && isIncomplete(r, minItems) && (
                    <Badge variant="warning" title={`Absent des listes publiques : moins de ${plural(minItems, 'plat')} disponibles.`}>
                      <EyeOff className="size-3" aria-hidden /> Masqué : carte incomplète ({plural(r.items_count ?? 0, 'plat')})
                    </Badge>
                  )}
                  {r.locked && (
                    <Badge variant="info" title="Modifié à la main : la synchronisation ne touche plus ce restaurant.">
                      <Lock className="size-3" aria-hidden /> Verrouillé
                    </Badge>
                  )}
                  {r.stale_since && (
                    <Badge variant="warning" title={`Plus proposé par aucune source depuis le ${formatDay(r.stale_since)} (toujours visible).`}>
                      <TriangleAlert className="size-3" aria-hidden /> Obsolète
                    </Badge>
                  )}
                  {!r.lat && !r.lng && (
                    <Badge variant="warning">
                      <MapPinOff className="size-3" aria-hidden /> Sans coordonnées
                    </Badge>
                  )}
                  {!r.phone?.trim() && (
                    <Badge title="Aucun numéro : la synchronisation tentera de le trouver (sources, OpenStreetMap).">
                      <PhoneOff className="size-3" aria-hidden /> sans tél.
                    </Badge>
                  )}
                  {!r.address?.trim() && (
                    <Badge title="Aucune adresse : la synchronisation tentera de la trouver (sources, OpenStreetMap).">
                      <MapPinOff className="size-3" aria-hidden /> sans adresse
                    </Badge>
                  )}
                </div>
                <p className="truncate text-xs text-subtle">
                  {r.slug}
                  {r.address ? ` · ${r.address}` : ''}
                  {r.phone ? ` · ${formatPhone(r.phone)}` : ''}
                  {r.cuisines?.length ? ` · ${r.cuisines.join(', ')}` : ''}
                </p>
              </div>
              <div className="ml-auto flex items-center gap-1">
                <Toggle checked={r.active} onChange={(active) => toggle.mutate({ r, active })} label={`Visible : ${r.name}`} />
                <Link to={`/admin/restaurants/${r.id}`} className={buttonClass('ghost', 'sm')} aria-label={`Menu de ${r.name}`}>
                  <UtensilsCrossed className="size-4" aria-hidden /> <span className="hidden sm:inline">Menu</span>
                </Link>
                <Button variant="ghost" size="sm" onClick={() => setEditing(r)} aria-label={`Modifier ${r.name}`}>
                  <Pencil className="size-4" aria-hidden /> <span className="hidden sm:inline">Modifier</span>
                </Button>
              </div>
            </Card>
          </li>
        ))}
      </ul>

      {editing !== undefined && <RestaurantForm key={editing?.id ?? 'new'} restaurant={editing} open onClose={() => setEditing(undefined)} />}
    </div>
  )
}
