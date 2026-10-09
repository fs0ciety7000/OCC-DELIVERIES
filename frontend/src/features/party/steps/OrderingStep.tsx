import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, CheckCircle2, Circle, Pencil, ShoppingBag } from 'lucide-react'
import { motion } from 'motion/react'
import { Suspense, useCallback, useMemo, useRef, useState } from 'react'
import { flyToCart, ReadyBell, WinnerReveal } from '@/components/food'
import { AnimatedMoney } from '@/components/food/AnimatedMoney'
import { useColleagueFlights } from '@/components/food/ColleagueFlight'
import { DrawCheck } from '@/components/food/DrawCheck'
import { EmptyBag } from '@/components/food/EmptyBag'
import { PulseOnChange } from '@/components/food/PulseOnChange'
import { toast } from 'sonner'
import { Avatar, Badge, Button, Card, CardBody, Countdown, EmptyState, Money, QuantityStepper, Sheet } from '@/components/ui'
import { useMenu } from '@/features/restaurants/hooks'
import { ItemSheet, type ItemDraft } from '@/features/restaurants/ItemSheet'
import { MenuSkeleton, MenuView } from '@/features/restaurants/MenuView'
import { PartialMenuBanner } from '@/features/restaurants/PartialMenu'
import { RestaurantCover } from '@/features/restaurants/RestaurantCover'
import { partiesApi } from '@/lib/api'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { plural } from '@/lib/format'
import { addItemAction } from '@/lib/offlineActions'
import { OFFLINE_HINT, useOnline } from '@/lib/online'
import { haptic } from '@/lib/haptics'
import { useFirstTime } from '@/lib/once'
import { fromSelectedOptions, toSelectedOptions } from '@/lib/price'
import { qk } from '@/lib/queryKeys'
import type { MenuItem, OrderItem } from '@/lib/types'
import type { PartyCtx } from '../context'
import { useOrderItems, useReady, useTransition } from '../hooks'
import { ReorderCard } from '../ReorderCard'
import { cartStatsByUser } from '../logic'

export function OrderingStep({ ctx }: { ctx: PartyCtx }) {
  const { party, me, isHost, members } = ctx
  const restaurant = party.expand?.restaurant
  const menu = useMenu(party.restaurant)
  const items = useOrderItems(party.id)
  const qc = useQueryClient()
  const ready = useReady(party.id)
  const transition = useTransition(party.id)
  const online = useOnline()
  const [picked, setPicked] = useState<MenuItem | null>(null)
  const [editing, setEditing] = useState<OrderItem | null>(null)
  const [cartOpen, setCartOpen] = useState(false)
  const pickEl = useRef<HTMLElement | null>(null)
  const [reviewOpen, setReviewOpen] = useState(false)

  const allItems = useMemo(() => items.data ?? [], [items.data])
  const mine = allItems.filter((i) => i.user === me.id)
  const stats = useMemo(() => cartStatsByUser(allItems), [allItems])
  const myStats = stats.get(me.id) ?? { count: 0, total: 0 }
  const inCart = useMemo(() => {
    const m: Record<string, number> = {}
    for (const i of mine) m[i.menu_item] = (m[i.menu_item] ?? 0) + i.quantity
    return m
  }, [mine])
  const myMember = members.find((m) => m.user === me.id)
  const amReady = !!myMember?.ready
  const readyCount = members.filter((m) => m.ready).length
  const menuItemsById = useMemo(() => new Map((menu.data?.items ?? []).map((i) => [i.id, i])), [menu.data])
  // Total indicatif du groupe : somme des lignes calculées par le serveur.
  const groupTotal = useMemo(() => allItems.reduce((sum, i) => sum + i.total, 0), [allItems])
  const groupCarts = stats.size

  // « Panier vivant » : les plats ajoutés par les collègues volent vers leur avatar.
  const emojiFor = useCallback((l: OrderItem) => menuItemsById.get(l.menu_item)?.emoji || '🍽️', [menuItemsById])
  useColleagueFlights(items.data, me.id, emojiFor)

  // Révélation du gagnant, une fois par party, seulement après un vrai vote.
  const firstReveal = useFirstTime(`occ-winner-${party.id}`, (party.candidates?.length ?? 0) >= 2 && !!restaurant)
  const [revealing, setRevealing] = useState(firstReveal)

  const toggleReady = (onDone?: () => void) =>
    ready.mutate(!amReady, {
      onSuccess: () => {
        if (!amReady) haptic('ready')
        onDone?.()
      },
    })

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: qk.items(party.id) })
    void qc.invalidateQueries({ queryKey: qk.members(party.id) })
  }

  const add = async (item: MenuItem, d: ItemDraft) => {
    try {
      const sent = await addItemAction(
        {
          party: party.id,
          user: me.id,
          menu_item: item.id,
          quantity: d.quantity,
          selected_options: toSelectedOptions(item, d.selections),
          note: d.note,
        },
        item.name,
      )
      if (sent === 'queued') toast(`${item.name} sera ajouté dès le retour du réseau`, { description: 'Hors ligne : action mise en attente.' })
      else toast.success(`${item.name} ajouté à ton panier`)
      flyToCart(pickEl.current, item.emoji || '🍽️')
      refresh()
    } catch (err) {
      toast.error(errorMessage(err))
      throw err
    }
  }

  const edit = async (line: OrderItem, item: MenuItem, d: ItemDraft) => {
    try {
      await partiesApi.updateItem(line.id, { quantity: d.quantity, note: d.note, selected_options: toSelectedOptions(item, d.selections) })
      toast.success('Article modifié')
      refresh()
    } catch (err) {
      toast.error(errorMessage(err))
      throw err
    }
  }

  const changeQty = useMutation({
    mutationFn: (v: { id: string; quantity: number }) => partiesApi.updateItem(v.id, { quantity: v.quantity }),
    onMutate: async (v) => {
      await qc.cancelQueries({ queryKey: qk.items(party.id) })
      const prev = qc.getQueryData<OrderItem[]>(qk.items(party.id))
      qc.setQueryData<OrderItem[]>(qk.items(party.id), (list) =>
        list?.map((i) => (i.id === v.id ? { ...i, quantity: v.quantity, total: i.unit_price * v.quantity } : i)),
      )
      return { prev }
    },
    onError: (err, _v, c) => {
      if (c?.prev) qc.setQueryData(qk.items(party.id), c.prev)
      toast.error(errorMessage(err))
    },
    onSettled: refresh,
  })

  const remove = useMutation({
    mutationFn: (id: string) => partiesApi.removeItem(id),
    onSuccess: () => toast('Article retiré'),
    onError: (err) => toast.error(errorMessage(err)),
    onSettled: refresh,
  })


  if (!party.restaurant) return <EmptyState emoji="🤷" title="Aucun resto choisi" description="L'hôte doit choisir un restaurant." />

  return (
    <div className="grid gap-6 pb-36 lg:grid-cols-[minmax(0,1fr)_320px]">
      {revealing && restaurant && (
        <Suspense fallback={null}>
          <WinnerReveal emoji={restaurant.emoji || '🍽️'} name={restaurant.name} onDone={() => setRevealing(false)} />
        </Suspense>
      )}
      <div className="min-w-0 space-y-4">
        {restaurant && (
          <div className="flex items-center gap-3">
            <RestaurantCover restaurant={restaurant} thumb="120x120" className="size-14 shrink-0 rounded-md" emojiClassName="text-2xl" />
            <div className="min-w-0 flex-1">
              <h2 className="line-clamp-2 font-display text-xl leading-7 font-semibold">On commande chez {restaurant.name}</h2>
              <p className="text-sm text-muted">Compose ton panier puis dis-nous quand t'es prêt·e.</p>
            </div>
            <Countdown to={party.ordering_ends_at} label="Fin de la commande" />
          </div>
        )}
        <p className="text-xs text-subtle">Prix indicatifs — les totaux sont recalculés par le serveur.</p>
        {restaurant && <PartialMenuBanner restaurant={restaurant} />}
        {items.isSuccess && mine.length === 0 && <ReorderCard partyId={party.id} />}
        {menu.isPending ? (
          <MenuSkeleton />
        ) : menu.isError ? (
          <EmptyState tone="danger" emoji="📡" title="Menu indisponible" description={errorMessage(menu.error)} action={<Button onClick={() => menu.refetch()}>Réessayer</Button>} />
        ) : (
          <MenuView
            sections={menu.data.sections}
            onPick={(item, el) => {
              pickEl.current = el ?? null
              setPicked(item)
            }}
            inCart={inCart}
          />
        )}
      </div>

      <aside className="space-y-4 lg:sticky lg:top-[calc(var(--header-h)+16px)] lg:self-start">
        <Card>
          <CardBody className="space-y-3">
            <div className="flex items-center justify-between">
              <h2 className="font-display text-lg font-semibold">L'équipe</h2>
              <PulseOnChange value={readyCount}>
                <Badge variant={readyCount === members.length ? 'success' : 'neutral'} className="tabular">
                  {readyCount}/{members.length} prêts
                </Badge>
              </PulseOnChange>
            </div>
            <ul className="space-y-1">
              {members.map((m) => {
                const u = m.expand?.user ?? ctx.people.get(m.user) ?? { id: m.user, name: '' }
                const s = stats.get(m.user)
                return (
                  <li key={m.id} className="flex min-h-12 items-center gap-3">
                    <span className="relative">
                      <Avatar user={u} size={32} ready={m.ready} />
                      <ReadyBell userId={m.user} />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{m.user === me.id ? 'Toi' : u.name}</p>
                      <p className="text-xs text-muted tabular">{s ? plural(s.count, 'article') : 'Panier vide'}</p>
                    </div>
                    {m.ready ? <CheckCircle2 aria-label="Prêt·e" className="size-5 text-success" /> : <Circle aria-label="Pas encore prêt·e" className="size-5 text-subtle" />}
                  </li>
                )
              })}
            </ul>
          </CardBody>
        </Card>
        {isHost && (
          <Card>
            <CardBody className="space-y-3">
              {/* heure limite + clôture automatique : features/deadlines (sous l'en-tête) */}
              <Button block variant="secondary" rightIcon={<ArrowRight className="size-4" />} onClick={() => setReviewOpen(true)} disabled={allItems.length === 0 || !online} title={online ? undefined : OFFLINE_HINT}>
                Passer au récap
              </Button>
              {allItems.length === 0 && <p className="text-xs text-muted">Il faut au moins un article dans un panier.</p>}
            </CardBody>
          </Card>
        )}
      </aside>

      {/* Barre d'action collante : panier + prêt */}
      <div className="fixed inset-x-0 bottom-[calc(var(--tabbar-h)+env(safe-area-inset-bottom))] z-30 border-t border-border bg-bg/85 backdrop-blur-xl">
        <div className="mx-auto flex max-w-[1200px] items-center justify-between gap-2 px-4 pt-2 text-xs text-muted sm:px-8">
          <span>
            Total du groupe <span className="text-subtle">· {plural(groupCarts, 'panier')}</span>
          </span>
          <span data-group-total className="inline-flex">
            <AnimatedMoney cents={groupTotal} className="text-sm font-semibold text-fg" />
          </span>
        </div>
        <div className="mx-auto flex max-w-[1200px] items-center gap-2 px-4 pt-1.5 pb-3 sm:px-8">
          <Button data-cart-target variant="secondary" size="lg" className="flex-1 justify-between sm:flex-none sm:gap-4" onClick={() => setCartOpen(true)} aria-label={`Mon panier : ${plural(myStats.count, 'article')}`}>
            <span className="inline-flex items-center gap-2">
              <span className="relative">
                <ShoppingBag className="size-5" />
                {myStats.count > 0 && (
                  <motion.span key={myStats.count} initial={{ scale: 0.4 }} animate={{ scale: 1 }} className="absolute -top-2 -right-2 grid size-5 place-items-center rounded-full bg-brand text-[11px] font-bold text-brand-fg tabular">
                    {myStats.count}
                  </motion.span>
                )}
              </span>
              <span className="hidden sm:inline">Mon panier</span>
            </span>
            <Money cents={myStats.total} />
          </Button>
          <Button
            size="lg"
            variant={amReady ? 'secondary' : 'primary'}
            className={cn('flex-1 sm:ml-auto sm:min-w-56 sm:flex-none', amReady && 'border-success/40 text-success')}
            loading={ready.isPending}
            disabled={!amReady && myStats.count === 0}
            onClick={() => toggleReady(() => toast(amReady ? 'OK, tu peux encore modifier' : 'Top, t’es prêt·e ! ✅'))}
            leftIcon={amReady ? <DrawCheck /> : undefined}
          >
            {amReady ? 'Prêt·e — modifier' : myStats.count === 0 ? 'Ajoute un article' : 'Je suis prêt·e'}
          </Button>
          {/* ≥ 1024 px : « Passer au récap » est déjà dans la colonne de droite. */}
          {isHost && (
            <Button size="lg" variant="ghost" className="hidden md:inline-flex lg:hidden" onClick={() => setReviewOpen(true)} disabled={allItems.length === 0 || !online} title={online ? undefined : OFFLINE_HINT}>
              Récap
            </Button>
          )}
        </div>
      </div>

      <ItemSheet item={picked} open={!!picked} onClose={() => setPicked(null)} onSubmit={(d) => (picked ? add(picked, d) : undefined)} />
      <ItemSheet
        item={editing ? (menuItemsById.get(editing.menu_item) ?? null) : null}
        open={!!editing}
        onClose={() => setEditing(null)}
        submitLabel="Enregistrer"
        initial={editing ? { quantity: editing.quantity, note: editing.note, selections: fromSelectedOptions(editing.selected_options) } : undefined}
        onSubmit={(d) => {
          const item = editing && menuItemsById.get(editing.menu_item)
          return editing && item ? edit(editing, item, d) : undefined
        }}
      />

      <Sheet
        open={cartOpen}
        onClose={() => setCartOpen(false)}
        title="Mon panier"
        description={mine.length ? `${plural(myStats.count, 'article')} · chaque modification te repasse en « pas prêt »` : undefined}
        footer={
          mine.length > 0 && (
            <div className="space-y-3">
              <div className="flex items-baseline justify-between">
                <span className="text-muted">Sous-total</span>
                <Money cents={myStats.total} className="font-display text-2xl font-bold" />
              </div>
              <p className="text-xs text-subtle">Hors frais partagés (livraison, service, pourboire) répartis au récap.</p>
              <Button block size="lg" variant={amReady ? 'secondary' : 'primary'} onClick={() => toggleReady()} loading={ready.isPending}>
                {amReady ? 'Je ne suis plus prêt·e' : 'Je suis prêt·e'}
              </Button>
            </div>
          )
        }
      >
        {mine.length === 0 ? (
          <EmptyState illustration={<EmptyBag />} title="Panier vide" description="Pioche dans le menu, on t'attend !" action={<Button onClick={() => setCartOpen(false)}>Voir le menu</Button>} />
        ) : (
          <ul className="divide-y divide-border">
            {mine.map((line) => (
              <li key={line.id} className="flex gap-3 py-3">
                <div className="min-w-0 flex-1 space-y-1">
                  <p className="font-semibold">{line.name || menuItemsById.get(line.menu_item)?.name}</p>
                  {line.options_label && <p className="text-sm text-muted">{line.options_label}</p>}
                  {line.note && <p className="text-sm text-subtle italic">« {line.note} »</p>}
                  <div className="flex items-center gap-2 pt-1">
                    <QuantityStepper
                      size="sm"
                      value={line.quantity}
                      onChange={(q) => changeQty.mutate({ id: line.id, quantity: q })}
                      onRemove={() => remove.mutate(line.id)}
                      label={`Quantité de ${line.name}`}
                    />
                    {menuItemsById.has(line.menu_item) && (
                      <Button variant="ghost" size="sm" leftIcon={<Pencil className="size-3.5" />} onClick={() => setEditing(line)}>
                        Modifier
                      </Button>
                    )}
                  </div>
                </div>
                <Money cents={line.total} className="font-semibold" />
              </li>
            ))}
          </ul>
        )}
      </Sheet>

      <Sheet
        open={reviewOpen}
        onClose={() => setReviewOpen(false)}
        title="Passer au récap ?"
        description={readyCount < members.length ? `${members.length - readyCount} membre(s) ne sont pas encore prêt·es.` : 'Tout le monde est prêt 🎉'}
        footer={
          <Button block size="lg" loading={transition.isPending} onClick={() => transition.mutate({ to: 'review' }, { onSuccess: () => setReviewOpen(false) })}>
            Verrouiller les paniers
          </Button>
        }
      >
        <p className="text-sm text-muted">Les paniers seront figés. Tu pourras rouvrir la commande depuis le récap si besoin.</p>
      </Sheet>
    </div>
  )
}
