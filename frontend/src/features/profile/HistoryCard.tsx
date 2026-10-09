import { ArrowRight, ChevronDown, RotateCcw } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useId, useState } from 'react'
import { Link } from 'react-router'
import { Badge, Button, buttonClass, Card, Money } from '@/components/ui'
import { METHOD_LABELS } from '@/features/party/labels'
import { STATUS_LABELS } from '@/features/party/hooks'
import { RestaurantCover } from '@/features/restaurants/RestaurantCover'
import { cn } from '@/lib/cn'
import { formatDate, plural } from '@/lib/format'
import type { HistoryEntry, HistoryRestaurant } from '@/lib/types'
import { historyBadge } from './historyBadge'

const ACTIVE = new Set(['lobby', 'voting', 'ordering', 'review', 'paying'])

export interface HistoryCardProps {
  entry: HistoryEntry
  /** « Relancer une commande ici » (restaurant encore actif). */
  onRelaunch?: (restaurant: HistoryRestaurant) => void
}

export function HistoryCard({ entry: e, onRelaunch }: HistoryCardProps) {
  const [open, setOpen] = useState(false)
  const reduce = useReducedMotion()
  const panelId = useId()
  const badge = historyBadge(e)
  const active = ACTIVE.has(e.status)
  const heading = e.restaurant?.name ?? e.title ?? 'Commande groupée'
  const count = e.items.reduce((n, i) => n + i.quantity, 0)
  const hasItems = e.items.length > 0
  // Statut de la commande en second badge, seulement s'il apporte une information.
  const statusLabel = STATUS_LABELS[e.status]
  const showStatus = active && hasItems && !!statusLabel && statusLabel !== badge.label

  return (
    <Card className="overflow-hidden">
      <div className="flex items-start gap-3 p-3.5 sm:p-4">
        {e.restaurant ? (
          <RestaurantCover restaurant={e.restaurant} thumb="120x120" className="size-14 shrink-0 rounded-md" emojiClassName="text-2xl" />
        ) : (
          <div aria-hidden className="grid size-14 shrink-0 place-items-center rounded-md bg-elevated text-2xl">
            {e.status === 'voting' ? '🗳️' : '🍽️'}
          </div>
        )}
        <div className="min-w-0 flex-1 space-y-1">
          <h3 className="truncate font-semibold">{heading}</h3>
          <p className="truncate text-xs text-muted">
            <time dateTime={e.created.replace(' ', 'T')}>{formatDate(e.created)}</time>
            {e.title && e.restaurant ? ` · ${e.title}` : ''}
          </p>
          <div className="flex flex-wrap items-center gap-1.5 pt-0.5">
            <Badge variant={badge.variant} dot={active}>
              {badge.label}
            </Badge>
            {showStatus && <Badge variant="neutral">{statusLabel}</Badge>}
          </div>
        </div>
        <div className="shrink-0 text-right">
          {hasItems ? (
            <Money cents={e.total} className="font-display text-lg font-bold" />
          ) : (
            <p className="font-display text-lg font-bold text-subtle">
              <span aria-hidden>—</span>
              <span className="sr-only">Aucun plat</span>
            </p>
          )}
          <p className="text-xs text-subtle">ma part</p>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-1 border-t border-border px-2 py-1.5 sm:px-3">
        {hasItems && (
          <button
            type="button"
            aria-expanded={open}
            aria-controls={panelId}
            onClick={() => setOpen((v) => !v)}
            className={cn(buttonClass('ghost', 'sm'), 'min-h-11')}
          >
            {`Mes plats (${count})`}
            <ChevronDown aria-hidden className={cn('size-4 transition-transform duration-200 motion-reduce:transition-none', open && 'rotate-180')} />
          </button>
        )}
        <span className="flex-1" />
        {e.status !== 'cancelled' && (
          <Link to={`/party/${e.id}`} className={cn(buttonClass('ghost', 'sm'), 'min-h-11')}>
            {active ? 'Reprendre' : 'Voir'}{' '}
            <span className="sr-only">la commande « {e.title || heading} »</span>
            <ArrowRight aria-hidden className="size-4" />
          </Link>
        )}
        {onRelaunch && e.restaurant?.active && !active && (
          <Button variant="ghost" size="sm" className="min-h-11 text-brand" leftIcon={<RotateCcw className="size-4" />} onClick={() => onRelaunch(e.restaurant!)}>
            Relancer ici{' '}
            <span className="sr-only">: nouvelle commande chez {e.restaurant.name}</span>
          </Button>
        )}
      </div>

      <AnimatePresence initial={false}>
        {open && hasItems && (
          <motion.div
            id={panelId}
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: reduce ? 0 : 0.2, ease: [0.2, 0.8, 0.2, 1] }}
            className="overflow-hidden"
          >
            <div className="space-y-3 border-t border-border bg-elevated/40 px-3.5 py-3 sm:px-4">
              <ul className="space-y-2" aria-label="Mes plats">
                {e.items.map((it, i) => (
                  <li key={`${it.menuItem}-${i}`} className="flex items-start gap-3 text-sm">
                    <span className="w-7 shrink-0 font-semibold text-muted tabular">{it.quantity}×</span>
                    <span className="min-w-0 flex-1">
                      <span className="font-medium">{it.name}</span>
                      {it.optionsLabel && <span className="block text-xs text-muted">{it.optionsLabel}</span>}
                      {it.note && <span className="block text-xs text-subtle italic">« {it.note} »</span>}
                    </span>
                    <Money cents={it.total} />
                  </li>
                ))}
              </ul>
              <dl className="space-y-1 border-t border-border pt-2 text-sm">
                <div className="flex justify-between gap-3">
                  <dt className="text-muted">Mes plats</dt>
                  <dd>
                    <Money cents={e.subtotal} />
                  </dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="text-muted">Ma part des frais</dt>
                  <dd>
                    <Money cents={e.sharedFees} />
                  </dd>
                </div>
                <div className="flex justify-between gap-3 font-semibold">
                  <dt>Ma part</dt>
                  <dd>
                    <Money cents={e.total} />
                  </dd>
                </div>
                <div className="flex justify-between gap-3 text-xs text-subtle">
                  <dt>Total de la commande ({plural(e.memberCount, 'membre')})</dt>
                  <dd>
                    <Money cents={e.grandTotal} />
                  </dd>
                </div>
              </dl>
              {e.payer && (
                <p className="text-xs text-muted">
                  {e.payment?.method === 'self' ? "C'est toi qui as avancé l'argent." : `Payé par ${e.payer.name}${e.payment?.method && e.payment.status !== 'pending' ? ` · remboursé via ${METHOD_LABELS[e.payment.method]}` : ''}.`}
                </p>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </Card>
  )
}
