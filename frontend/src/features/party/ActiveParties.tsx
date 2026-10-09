import { ArrowRight, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { Badge, buttonClass, Sheet } from '@/components/ui'
import { RestaurantCover } from '@/features/restaurants/RestaurantCover'
import { cn } from '@/lib/cn'
import { STATUS_LABELS } from './hooks'
import { stepText, type ActiveParty } from './resume'

function LiveDot({ className }: { className?: string }) {
  return (
    <span aria-hidden className={cn('relative flex size-2.5 shrink-0', className)}>
      <span className="absolute inline-flex size-full animate-ping rounded-full bg-brand opacity-60 motion-reduce:animate-none" />
      <span className="relative inline-flex size-2.5 rounded-full bg-brand" />
    </span>
  )
}

function Thumb({ party, className }: { party: ActiveParty; className?: string }) {
  return party.restaurant ? (
    <RestaurantCover restaurant={party.restaurant} thumb="120x120" className={cn('shrink-0 rounded-sm', className)} emojiClassName="text-xl" />
  ) : (
    <span aria-hidden className={cn('grid shrink-0 place-items-center rounded-sm bg-surface text-xl', className)}>
      {party.status === 'voting' ? '🗳️' : '🍽️'}
    </span>
  )
}

export interface ResumeBannerProps {
  parties: ActiveParty[]
  /** `header` : pastille dans l'en-tête desktop ; `dock` : barre au-dessus de la tab bar mobile. */
  variant: 'header' | 'dock'
}

/** Bandeau « Commande en cours » : reprendre la commande (ou choisir parmi plusieurs). */
export function ResumeBanner({ parties, variant }: ResumeBannerProps) {
  const [open, setOpen] = useState(false)
  if (parties.length === 0) return null
  const single = parties.length === 1 ? parties[0]! : null
  const label = single ? `Reprendre la commande « ${single.title} » — ${STATUS_LABELS[single.status]}` : `${parties.length} commandes en cours — choisir`

  const body =
    variant === 'header' ? (
      <>
        <LiveDot />
        <span className="max-w-44 truncate">{single ? single.title : `${parties.length} commandes en cours`}</span>
        {single && <span className="hidden text-muted lg:inline">· {STATUS_LABELS[single.status]}</span>}
        <span className="text-brand">Reprendre</span>
        <ArrowRight aria-hidden className="size-4 text-brand" />
      </>
    ) : (
      <>
        {single ? <Thumb party={single} className="size-10" /> : <LiveDot className="mx-2" />}
        <span className="min-w-0 flex-1 text-left">
          <span className="flex items-center gap-1.5 text-[11px] font-semibold tracking-wide text-brand uppercase">
            {single && <LiveDot className="size-2" />}
            {single ? 'Commande en cours' : `${parties.length} commandes en cours`}
          </span>
          <span className="block truncate text-sm font-semibold">{single ? single.title : 'Choisis celle à reprendre'}</span>
          {single && <span className="block truncate text-xs text-muted">{stepText(single.status)}</span>}
        </span>
        <span className={buttonClass('secondary', 'sm', false, 'pointer-events-none shrink-0')}>Reprendre</span>
      </>
    )

  const cls =
    variant === 'header'
      ? 'hidden h-9 items-center gap-2 rounded-full border border-brand/30 bg-brand/10 pr-3 pl-3 text-sm font-semibold transition-colors hover:bg-brand/15 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand md:inline-flex'
      : 'flex w-full items-center gap-3 rounded-lg border border-brand/30 bg-elevated/95 p-2 pl-2.5 shadow-card backdrop-blur-xl focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand'

  const trigger = single ? (
    <Link to={`/party/${single.id}`} className={cls} aria-label={label} data-testid={`resume-${variant}`}>
      {body}
    </Link>
  ) : (
    <button type="button" className={cls} aria-label={label} aria-haspopup="dialog" onClick={() => setOpen(true)} data-testid={`resume-${variant}`}>
      {body}
    </button>
  )

  return (
    <>
      {variant === 'dock' ? (
        <aside aria-label="Commande en cours" className="fixed inset-x-0 bottom-[calc(64px+env(safe-area-inset-bottom))] z-40 px-3 pb-2 md:hidden">
          <div className="mx-auto max-w-md">{trigger}</div>
        </aside>
      ) : (
        trigger
      )}
      {!single && (
        <Sheet open={open} onClose={() => setOpen(false)} title="Mes commandes en cours" description="Reprends là où tu t'étais arrêté·e.">
          <ul className="space-y-2">
            {parties.map((p) => (
              <li key={p.id}>
                <Link
                  to={`/party/${p.id}`}
                  onClick={() => setOpen(false)}
                  className="flex min-h-14 items-center gap-3 rounded-md border border-border bg-surface p-2.5 hover:border-border-strong focus-visible:outline-2 focus-visible:outline-brand"
                >
                  <Thumb party={p} className="size-11" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-semibold">{p.title}</span>
                    <span className="mt-0.5 flex items-center gap-2">
                      <Badge variant="brand" dot>
                        {STATUS_LABELS[p.status]}
                      </Badge>
                      {p.restaurant && <span className="truncate text-xs text-muted">{p.restaurant.name}</span>}
                    </span>
                  </span>
                  <ChevronRight aria-hidden className="size-5 text-subtle" />
                </Link>
              </li>
            ))}
          </ul>
        </Sheet>
      )}
    </>
  )
}

/** Bandeau mis en avant sur l'accueil quand il y a exactement une commande en cours (ex. après connexion). */
export function ResumeHero({ party }: { party: ActiveParty }) {
  return (
    <Link
      to={`/party/${party.id}`}
      className="group flex items-center gap-4 rounded-xl border border-brand/40 bg-brand/10 p-3.5 shadow-glow focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand sm:p-4"
      aria-label={`Reprendre la commande « ${party.title} » — ${STATUS_LABELS[party.status]}`}
    >
      <Thumb party={party} className="size-14 rounded-md" />
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-2 text-xs font-semibold tracking-wide text-brand uppercase">
          <LiveDot className="size-2" /> Ta commande t'attend
        </span>
        <span className="block truncate font-display text-xl leading-7 font-semibold">{party.title}</span>
        <span className="block truncate text-sm text-muted">
          {stepText(party.status)}
          {party.restaurant ? ` · ${party.restaurant.name}` : ''}
        </span>
      </span>
      <span className={buttonClass('secondary', 'md', false, 'pointer-events-none hidden sm:inline-flex')}>
        Reprendre <ArrowRight aria-hidden className="size-4" />
      </span>
      <ChevronRight aria-hidden className="size-5 text-brand sm:hidden" />
    </Link>
  )
}
