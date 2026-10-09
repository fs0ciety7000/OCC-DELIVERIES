import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowLeft, ArrowUp, Check, ExternalLink, Lock, LockOpen, Pencil, Plus, Trash2, TriangleAlert } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router'
import { toast } from 'sonner'
import { Badge, Button, buttonClass, Card, CardBody, EmptyState, Input, Money, Sheet, Skeleton } from '@/components/ui'
import { adminApi, restaurantsApi } from '@/lib/api'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { plural } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import type { MenuCategory, MenuItem } from '@/lib/types'
import { AdminHeader } from './AdminLayout'
import { ItemForm } from './ItemForm'
import { PRESET_TAGS } from './labels'
import { RestaurantForm } from './RestaurantForm'
import { Toggle } from './Toggle'

interface Menu {
  categories: MenuCategory[]
  items: MenuItem[]
}

/** Réécrit les positions 0..n-1 dans l'ordre donné ; n'enregistre que ce qui change. */
async function persistOrder<T extends { id: string; position: number }>(list: T[], save: (id: string, position: number) => Promise<unknown>) {
  await Promise.all(list.map((x, i) => (x.position === i ? null : save(x.id, i))))
}

function move<T>(list: T[], index: number, delta: number): T[] {
  const j = index + delta
  if (j < 0 || j >= list.length) return list
  const next = [...list]
  ;[next[index], next[j]] = [next[j]!, next[index]!]
  return next
}

const tagLabel = (t: string) => PRESET_TAGS.find((p) => p.id === t)?.label ?? t

export function MenuEditorPage() {
  const { id = '' } = useParams()
  const qc = useQueryClient()
  const restaurant = useQuery({ queryKey: qk.admin.restaurant(id), queryFn: () => adminApi.restaurant(id), enabled: !!id })
  const menu = useQuery({
    queryKey: qk.admin.menu(id),
    queryFn: async (): Promise<Menu> => {
      const [categories, items] = await Promise.all([restaurantsApi.categories(id), restaurantsApi.items(id)])
      return { categories, items }
    },
    enabled: !!id,
  })
  const [editInfo, setEditInfo] = useState(false)
  const [itemEditor, setItemEditor] = useState<{ item: MenuItem | null; category: string } | null>(null)
  const [newCategory, setNewCategory] = useState('')
  const [renaming, setRenaming] = useState<{ id: string; name: string } | null>(null)
  const [confirm, setConfirm] = useState<{ kind: 'category'; cat: MenuCategory } | { kind: 'item'; item: MenuItem } | null>(null)

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: qk.admin.menu(id) })
    void qc.invalidateQueries({ queryKey: qk.menu(id) })
  }
  const run = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onError: (e) => toast.error(errorMessage(e)),
    onSettled: refresh,
  })

  if (restaurant.isError) {
    return <EmptyState tone="danger" emoji="🍽️" title="Restaurant introuvable" description={errorMessage(restaurant.error)} action={<Link to="/admin/restaurants" className={buttonClass('secondary')}>Retour à la liste</Link>} />
  }
  const r = restaurant.data
  const cats = menu.data?.categories ?? []
  const items = menu.data?.items ?? []
  const itemsOf = (catId: string) => items.filter((i) => i.category === catId)
  const orphans = items.filter((i) => !i.category || !cats.some((c) => c.id === i.category))
  const sections: { cat: MenuCategory | null; list: MenuItem[] }[] = [...cats.map((c) => ({ cat: c, list: itemsOf(c.id) })), ...(orphans.length ? [{ cat: null, list: orphans }] : [])]

  const addCategory = (e: FormEvent) => {
    e.preventDefault()
    const name = newCategory.trim()
    if (!name) return
    run.mutate(() => adminApi.saveCategory(null, { restaurant: id, name, position: cats.length }), { onSuccess: () => setNewCategory('') })
  }

  const moveCategory = (index: number, delta: number) => run.mutate(() => persistOrder(move(cats, index, delta), (cid, position) => adminApi.saveCategory(cid, { position })))
  const moveItem = (list: MenuItem[], index: number, delta: number) => run.mutate(() => persistOrder(move(list, index, delta), (iid, position) => adminApi.saveItem(iid, { position })))
  const patchItem = (item: MenuItem, data: Partial<MenuItem>) => {
    // toute modification d'un article le verrouille côté serveur (sauf l'interrupteur lui-même)
    const next = data
    qc.setQueryData<Menu>(qk.admin.menu(id), (old) => old && { ...old, items: old.items.map((x) => (x.id === item.id ? { ...x, ...next } : x)) })
    run.mutate(() => adminApi.saveItem(item.id, data))
  }

  return (
    <div>
      <Link to="/admin/restaurants" className={cn(buttonClass('ghost', 'sm'), '-ml-3 mb-2')}>
        <ArrowLeft className="size-4" aria-hidden /> Restaurants
      </Link>
      {r ? (
        <AdminHeader
          title={`${r.emoji ? r.emoji + ' ' : ''}${r.name}`}
          description={
            <span className="flex flex-wrap items-center gap-2">
              {!r.active && <Badge>Masqué</Badge>}
              {r.locked && (
                <Badge variant="info">
                  <Lock className="size-3" aria-hidden /> Verrouillé
                </Badge>
              )}
              {r.stale_since && (
                <Badge variant="warning">
                  <TriangleAlert className="size-3" aria-hidden /> Obsolète
                </Badge>
              )}
              <span>{plural(items.length, 'article')} · {plural(cats.length, 'catégorie')}</span>
            </span>
          }
          actions={
            <>
              {r.active && (
                <Link to={`/restaurants/${r.id}`} className={buttonClass('ghost', 'sm')}>
                  <ExternalLink className="size-4" aria-hidden /> Voir dans l'app
                </Link>
              )}
              <Button variant="secondary" size="sm" leftIcon={<Pencil className="size-4" />} onClick={() => setEditInfo(true)}>
                Infos du restaurant
              </Button>
            </>
          }
        />
      ) : (
        <Skeleton className="mb-5 h-14 w-2/3" />
      )}

      {menu.isPending && <Skeleton className="h-64 rounded-lg" />}
      {menu.data && sections.length === 0 && <EmptyState title="Menu vide" description="Ajoute une première catégorie (ex. « Pizzas »), puis ses articles." />}

      <div className="space-y-4">
        {sections.map(({ cat, list }, ci) => (
          <Card key={cat?.id ?? '__orphans'}>
            <CardBody className="space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                {renaming && cat && renaming.id === cat.id ? (
                  <form
                    className="flex min-w-0 flex-1 gap-2"
                    onSubmit={(e) => {
                      e.preventDefault()
                      const name = renaming.name.trim()
                      if (name) run.mutate(() => adminApi.saveCategory(cat.id, { name }), { onSuccess: () => setRenaming(null) })
                    }}
                  >
                    <Input aria-label="Nom de la catégorie" value={renaming.name} onChange={(e) => setRenaming({ id: cat.id, name: e.target.value })} autoFocus />
                    <Button type="submit" size="icon" variant="secondary" aria-label="Valider le nom">
                      <Check className="size-4" />
                    </Button>
                  </form>
                ) : (
                  <h2 className="min-w-0 flex-1 truncate font-display text-lg font-semibold">
                    {cat ? cat.name : 'Sans catégorie'} <span className="text-sm font-normal text-subtle">· {list.length}</span>
                  </h2>
                )}
                {cat && (
                  <div className="flex items-center">
                    <Button variant="ghost" size="icon" aria-label={`Monter ${cat.name}`} disabled={ci === 0 || run.isPending} onClick={() => moveCategory(ci, -1)}>
                      <ArrowUp className="size-4" />
                    </Button>
                    <Button variant="ghost" size="icon" aria-label={`Descendre ${cat.name}`} disabled={ci === cats.length - 1 || run.isPending} onClick={() => moveCategory(ci, 1)}>
                      <ArrowDown className="size-4" />
                    </Button>
                    <Button variant="ghost" size="icon" aria-label={`Renommer ${cat.name}`} onClick={() => setRenaming({ id: cat.id, name: cat.name })}>
                      <Pencil className="size-4" />
                    </Button>
                    <Button variant="ghost" size="icon" aria-label={`Supprimer ${cat.name}`} onClick={() => setConfirm({ kind: 'category', cat })}>
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                )}
              </div>

              <ul className="divide-y divide-border">
                {list.map((it, ii) => (
                  <li key={it.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2.5 sm:flex-nowrap">
                    <span aria-hidden className="w-6 text-center text-lg">
                      {it.emoji || '·'}
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className={cn('truncate font-medium', !it.available && 'text-muted line-through')}>{it.name}</p>
                      <p className="flex flex-wrap gap-1 text-xs text-subtle">
                        {(it.tags ?? []).map((t) => (
                          <Badge key={t} className="px-2 py-0">
                            {tagLabel(t)}
                          </Badge>
                        ))}
                        {(it.option_groups?.length ?? 0) > 0 && <span>{plural(it.option_groups!.length, 'groupe')} d'options</span>}
                      </p>
                    </div>
                    <Money cents={it.price} className="font-semibold" />
                    <div className="flex items-center gap-1">
                      <Toggle checked={it.available} onChange={(v) => patchItem(it, { available: v })} label={`Disponible : ${it.name}`} />
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-pressed={!!it.locked}
                        aria-label={`Verrouillé (hors synchronisation) : ${it.name}`}
                        title={it.locked ? 'Verrouillé : la synchronisation ne le modifie pas' : 'Suit la synchronisation'}
                        onClick={() => patchItem(it, { locked: !it.locked })}
                        className={it.locked ? 'text-info' : 'text-subtle'}
                      >
                        {it.locked ? <Lock className="size-4" /> : <LockOpen className="size-4" />}
                      </Button>
                      <Button variant="ghost" size="icon" aria-pressed={it.popular} aria-label={`Populaire : ${it.name}`} onClick={() => patchItem(it, { popular: !it.popular })} className={it.popular ? 'text-brand' : 'text-subtle'}>
                        <span aria-hidden>{it.popular ? '★' : '☆'}</span>
                      </Button>
                      <Button variant="ghost" size="icon" aria-label={`Monter ${it.name}`} disabled={ii === 0 || run.isPending} onClick={() => moveItem(list, ii, -1)}>
                        <ArrowUp className="size-4" />
                      </Button>
                      <Button variant="ghost" size="icon" aria-label={`Descendre ${it.name}`} disabled={ii === list.length - 1 || run.isPending} onClick={() => moveItem(list, ii, 1)}>
                        <ArrowDown className="size-4" />
                      </Button>
                      <Button variant="ghost" size="icon" aria-label={`Modifier ${it.name}`} onClick={() => setItemEditor({ item: it, category: it.category })}>
                        <Pencil className="size-4" />
                      </Button>
                      <Button variant="ghost" size="icon" aria-label={`Supprimer ${it.name}`} onClick={() => setConfirm({ kind: 'item', item: it })}>
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </li>
                ))}
              </ul>
              <Button variant="ghost" size="sm" leftIcon={<Plus className="size-4" />} onClick={() => setItemEditor({ item: null, category: cat?.id ?? '' })}>
                Ajouter un article
              </Button>
            </CardBody>
          </Card>
        ))}

        <Card>
          <CardBody>
            <form onSubmit={addCategory} className="flex gap-2">
              <Input aria-label="Nouvelle catégorie" placeholder="Nouvelle catégorie (ex. Desserts)" value={newCategory} onChange={(e) => setNewCategory(e.target.value)} />
              <Button type="submit" variant="secondary" leftIcon={<Plus className="size-4" />} disabled={!newCategory.trim()} loading={run.isPending}>
                Catégorie
              </Button>
            </form>
          </CardBody>
        </Card>
      </div>

      {editInfo && r && <RestaurantForm restaurant={r} open onClose={() => setEditInfo(false)} onSaved={(saved) => qc.setQueryData(qk.admin.restaurant(id), saved)} />}
      {itemEditor && (
        <ItemForm
          key={itemEditor.item?.id ?? 'new'}
          restaurantId={id}
          item={itemEditor.item}
          categories={cats}
          defaultCategory={itemEditor.category}
          nextPosition={itemsOf(itemEditor.category).length}
          open
          onClose={() => setItemEditor(null)}
        />
      )}
      <Sheet
        open={!!confirm}
        onClose={() => setConfirm(null)}
        title={confirm?.kind === 'category' ? `Supprimer « ${confirm.cat.name} » ?` : confirm ? `Supprimer « ${confirm.item.name} » ?` : ''}
        description={confirm?.kind === 'category' ? 'Ses articles sont conservés et passent dans « Sans catégorie ».' : "L'historique des commandes passées garde le nom et le prix."}
        footer={
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setConfirm(null)}>
              Annuler
            </Button>
            <Button
              variant="danger"
              loading={run.isPending}
              onClick={() => {
                if (!confirm) return
                const fn = confirm.kind === 'category' ? () => adminApi.deleteCategory(confirm.cat.id) : () => adminApi.deleteItem(confirm.item.id)
                run.mutate(fn, { onSuccess: () => (setConfirm(null), toast.success('Supprimé')) })
              }}
            >
              Supprimer
            </Button>
          </div>
        }
      >
        <span className="sr-only">Confirmation</span>
      </Sheet>
    </div>
  )
}
