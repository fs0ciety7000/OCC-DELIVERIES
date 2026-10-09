import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlarmClock, AlertTriangle, BellRing, CheckCircle2, ChevronDown, Hourglass, Timer } from 'lucide-react'
import { useId, useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { Badge, Button, Card, CardBody, Chip, Countdown } from '@/components/ui'
import { Toggle } from '@/features/admin/Toggle'
import { partiesApi } from '@/lib/api'
import { brusselsTimeToISO, DEADLINE_MINUTES, formatClock, isoInMinutesRounded } from '@/lib/brussels'
import { cn } from '@/lib/cn'
import { useMediaQuery } from '@/lib/hooks'
import { errorMessage } from '@/lib/errors'
import { OFFLINE_HINT, useOnline } from '@/lib/online'
import { qk } from '@/lib/queryKeys'
import type { AutoEvent, Party } from '@/lib/types'

type Kind = 'voting' | 'ordering'

const FIELD: Record<Kind, 'voting_ends_at' | 'ordering_ends_at'> = { voting: 'voting_ends_at', ordering: 'ordering_ends_at' }
const LABEL: Record<Kind, string> = { voting: 'Fin du vote', ordering: 'Fin de la commande' }
const AUTO: Record<Kind, string> = {
  voting: 'À l’heure limite, le resto le plus voté est retenu (personne n’a voté : +5 min, une fois).',
  ordering: 'À l’heure limite, les paniers passent au récap (ceux qui ne sont pas prêts sont gardés tels quels).',
}

export interface PartyDeadlinesProps {
  party: Party
  isHost: boolean
}

/**
 * Heures limites de la party (« Fin du vote à 11:45 », heure de Bruxelles) et journal des
 * actions automatiques du serveur (rappels, clôtures). Monté une fois sous l'en-tête.
 */
export function PartyDeadlines({ party, isHost }: PartyDeadlinesProps) {
  const kind: Kind | null = party.status === 'voting' ? 'voting' : party.status === 'ordering' ? 'ordering' : null
  const events = party.auto_events ?? []
  const showControl = kind !== null && (isHost || !!party[FIELD[kind]])
  if (!showControl && events.length === 0) return null
  return (
    <div className="space-y-3">
      {showControl && <DeadlineControl key={kind} party={party} kind={kind} isHost={isHost} />}
      <AutoEvents events={events} />
    </div>
  )
}

function useSaveParty(partyId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Parameters<typeof partiesApi.update>[1]) => partiesApi.update(partyId, data),
    onSuccess: (p) => {
      qc.setQueryData<Party>(qk.partyDetail(partyId), (prev) => (prev ? { ...prev, ...p, expand: p.expand ?? prev.expand } : p))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
}

export function DeadlineControl({ party, kind, isHost }: { party: Party; kind: Kind; isHost: boolean }) {
  const field = FIELD[kind]
  const deadline = party[field]
  const save = useSaveParty(party.id)
  const online = useOnline()
  const [time, setTime] = useState('')
  const timeId = useId()
  const panelId = useId()
  const auto = !party.auto_close_disabled
  // Réglages repliés en mobile (la ligne d'état suffit), ouverts d'office dès 1024 px.
  const desktop = useMediaQuery('(min-width: 1024px)')
  const [userOpen, setOpen] = useState<boolean | null>(null)
  const open = userOpen ?? desktop

  if (!deadline && !isHost) return null

  const setDeadline = (iso: string, msg: string) => save.mutate({ [field]: iso }, { onSuccess: () => toast.success(msg) })
  const setAt = () => {
    const iso = brusselsTimeToISO(time)
    if (!iso) {
      toast.error('Heure invalide ou déjà passée.')
      return
    }
    setDeadline(iso, `${LABEL[kind]} à ${formatClock(iso)}`)
    setTime('')
  }
  const disabled = !online || save.isPending
  const offlineTitle = online ? undefined : OFFLINE_HINT

  return (
    <Card>
      <CardBody className="space-y-3 py-3 sm:py-4">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <Timer aria-hidden className="size-4 text-brand" />
          <p className="text-sm font-semibold">
            {deadline ? (
              <>
                {LABEL[kind]} à <span className="tabular">{formatClock(deadline)}</span>
              </>
            ) : (
              'Pas d’heure limite'
            )}
          </p>
          {deadline && <Countdown to={deadline} label={LABEL[kind]} />}
          {deadline && (
            <Badge variant={auto ? 'info' : 'neutral'} title={auto ? AUTO[kind] : undefined}>
              {auto ? 'Clôture automatique' : 'Clôture par l’hôte'}
            </Badge>
          )}
          {isHost && (
            <Button variant="ghost" size="sm" className="ml-auto min-h-11" aria-expanded={open} aria-controls={panelId} onClick={() => setOpen(!open)}>
              {open ? 'Masquer' : deadline ? 'Modifier' : 'Fixer une heure limite'}
              <ChevronDown aria-hidden className={cn('size-4 transition-transform motion-reduce:transition-none', open && 'rotate-180')} />
            </Button>
          )}
        </div>
        {isHost && open && (
          <div id={panelId} className="space-y-3">
            <div className="flex flex-wrap items-center gap-2" role="group" aria-label={`${LABEL[kind]} : raccourcis`}>
              {DEADLINE_MINUTES.map((m) => (
                <Chip key={m} className="min-h-11 px-4 disabled:opacity-50" disabled={disabled} title={offlineTitle} onClick={() => setDeadline(isoInMinutesRounded(m), `${LABEL[kind]} dans ${m} min`)}>
                  +{m} min
                </Chip>
              ))}
              {deadline && (
                <Chip className="min-h-11 px-4 disabled:opacity-50" disabled={disabled} title={offlineTitle} onClick={() => save.mutate({ [field]: '' }, { onSuccess: () => toast('Heure limite retirée') })}>
                  Retirer
                </Chip>
              )}
            </div>
            <form
              className="flex flex-wrap items-center gap-2"
              onSubmit={(e) => {
                e.preventDefault()
                setAt()
              }}
            >
              <label htmlFor={timeId} className="text-sm text-muted">
                ou à
              </label>
              <input
                id={timeId}
                type="time"
                step={60}
                value={time}
                onChange={(e) => setTime(e.target.value)}
                className="h-11 rounded-md border border-border-strong bg-surface px-3 text-[15px] tabular focus-visible:outline-2 focus-visible:outline-brand"
                aria-label={`${LABEL[kind]} à (heure de Bruxelles)`}
              />
              <Button type="submit" size="md" variant="secondary" disabled={disabled || !time} title={offlineTitle}>
                Fixer
              </Button>
            </form>
            <div className="flex items-center justify-between gap-3">
              <p className="text-xs text-muted">
                <span className="font-medium text-fg">Clôturer automatiquement</span> — {AUTO[kind]} Un rappel part 2 minutes avant.
              </p>
              <Toggle label="Clôturer automatiquement à l’heure limite" checked={auto} disabled={disabled} onChange={(v) => save.mutate({ auto_close_disabled: !v })} />
            </div>
          </div>
        )}
      </CardBody>
    </Card>
  )
}

const EVENT_ICON: Record<string, ReactNode> = {
  reminder_vote: <BellRing aria-hidden className="size-4 text-info" />,
  reminder_order: <BellRing aria-hidden className="size-4 text-info" />,
  vote_closed: <CheckCircle2 aria-hidden className="size-4 text-success" />,
  ordering_closed: <CheckCircle2 aria-hidden className="size-4 text-success" />,
  vote_extended: <Hourglass aria-hidden className="size-4 text-warning" />,
  vote_needs_host: <AlertTriangle aria-hidden className="size-4 text-warning" />,
  ordering_needs_host: <AlertTriangle aria-hidden className="size-4 text-warning" />,
  auto_failed: <AlertTriangle aria-hidden className="size-4 text-danger" />,
}

/** Journal « Vote clôturé automatiquement à 11:45 » (3 derniers, plus récent d'abord). */
export function AutoEvents({ events, max = 3 }: { events: AutoEvent[]; max?: number }) {
  const [all, setAll] = useState(false)
  if (events.length === 0) return null
  const list = [...events].reverse()
  const shown = all ? list : list.slice(0, max)
  return (
    <section aria-label="Actions automatiques" className="rounded-md border border-border bg-fg/[0.03] px-3 py-2">
      <ul className="space-y-1.5">
        {shown.map((e, i) => (
          <li key={`${e.at}-${i}`} className={cn('flex items-start gap-2 text-sm', i > 0 && 'text-muted')}>
            <span className="mt-0.5 shrink-0">{EVENT_ICON[e.kind] ?? <AlarmClock aria-hidden className="size-4 text-muted" />}</span>
            <span>{e.text}</span>
          </li>
        ))}
      </ul>
      {list.length > max && (
        <button type="button" onClick={() => setAll((v) => !v)} className="mt-1 text-xs font-semibold text-muted underline-offset-4 hover:text-fg hover:underline">
          {all ? 'Réduire' : `Tout afficher (${list.length})`}
        </button>
      )}
    </section>
  )
}
