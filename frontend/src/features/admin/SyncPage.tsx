import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Clock, ExternalLink, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { Badge, Button, Card, CardBody, EmptyState, Field, Input, Sheet, Skeleton, Spinner } from '@/components/ui'
import { adminApi } from '@/lib/api'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { formatRelativeTime, parseDate, plural } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import type { SyncProvider, SyncRun, SyncSource, SyncStatus } from '@/lib/types'
import { AdminHeader } from './AdminLayout'
import {
  formatDuration,
  parseLastStatus,
  PROVIDER_HINT,
  PROVIDER_LABEL,
  RUN_STATUS_LABEL,
  RUN_STATUS_VARIANT,
  runDuration,
  SOURCE_STATUS_LABEL,
  SOURCE_STATUS_VARIANT,
  statsSummary,
  TRIGGER_LABEL,
} from './syncFormat'
import { Toggle } from './Toggle'

const dateTimeFmt = new Intl.DateTimeFormat('fr-BE', { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })

function formatDateTime(value: string | null | undefined): string {
  const d = parseDate(value)
  return d ? dateTimeFmt.format(d) : '—'
}

const selectClass =
  'min-h-11 w-full rounded-sm border border-border bg-elevated px-3 text-fg hover:border-border-strong focus:border-brand/70 focus:outline-none focus:ring-3 focus:ring-brand/20'

export function SyncPage() {
  const qc = useQueryClient()
  const status = useQuery({
    queryKey: qk.admin.syncStatus,
    queryFn: adminApi.syncStatus,
    refetchInterval: (q) => (q.state.data?.running ? 3000 : 60_000),
  })
  const running = !!status.data?.running

  // à la fin d'une exécution : historique, restaurants et menus à jour
  const wasRunning = useRef(false)
  useEffect(() => {
    if (wasRunning.current && !running) {
      void qc.invalidateQueries({ queryKey: qk.admin.all })
      const last = status.data?.lastRun
      if (last) toast(last.status === 'success' ? 'Synchronisation terminée' : `Synchronisation : ${RUN_STATUS_LABEL[last.status].toLowerCase()}`)
    }
    wasRunning.current = running
  }, [running, qc, status.data?.lastRun])

  const start = useMutation({
    mutationFn: adminApi.startSync,
    onSuccess: () => {
      toast.success('Synchronisation lancée', { description: 'Les sources sont lues une par une, poliment : comptez quelques minutes.' })
      void qc.invalidateQueries({ queryKey: qk.admin.syncStatus })
      void qc.invalidateQueries({ queryKey: ['admin', 'sync', 'runs'] })
    },
    onError: (e) => {
      toast.error(errorMessage(e))
      void qc.invalidateQueries({ queryKey: qk.admin.syncStatus })
    },
  })

  return (
    <div className="space-y-8">
      <AdminHeader
        title="Synchronisation"
        description="Restaurants et menus relus automatiquement sur Deliveroo, weloveat et les sites des restaurants."
        actions={
          <Button
            leftIcon={<RefreshCw className={cn('size-4', running && 'animate-spin motion-reduce:animate-none')} />}
            onClick={() => start.mutate()}
            loading={start.isPending}
            disabled={running || status.data?.enabled === false}
          >
            {running ? 'Synchronisation en cours…' : 'Synchroniser maintenant'}
          </Button>
        }
      />
      <StatusCard status={status.data} error={status.error} pending={status.isPending} />
      <RunsSection />
      <SourcesSection />
    </div>
  )
}

/* ------------------------------------------------------------------ état */

function StatusCard({ status, error, pending }: { status?: SyncStatus; error: Error | null; pending: boolean }) {
  if (pending) return <Skeleton className="h-[132px] rounded-lg" />
  if (!status) return <EmptyState tone="danger" emoji="⚠️" title="État indisponible" description={errorMessage(error)} />
  const run = status.running ?? status.lastRun
  return (
    <Card aria-live="polite">
      <CardBody className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          {status.running ? (
            <>
              <Spinner className="size-4" label="Synchronisation en cours" />
              <span className="font-semibold">En cours depuis {formatDuration(runDuration(status.running.started_at, ''))}</span>
              <Badge variant="info">{TRIGGER_LABEL[status.running.trigger]}</Badge>
            </>
          ) : run ? (
            <>
              <Badge variant={RUN_STATUS_VARIANT[run.status]} dot>
                {RUN_STATUS_LABEL[run.status]}
              </Badge>
              <span className="font-semibold">Dernière synchronisation {formatRelativeTime(run.finished_at || run.started_at)}</span>
              <span className="text-sm text-subtle">· {formatDuration(runDuration(run.started_at, run.finished_at))}</span>
            </>
          ) : (
            <span className="font-semibold">Aucune synchronisation pour l'instant.</span>
          )}
        </div>
        {status.running?.logTail && (
          <pre className="max-h-48 overflow-auto rounded-md border border-border bg-elevated p-3 text-xs leading-5 whitespace-pre-wrap" aria-label="Journal en direct">
            {status.running.logTail}
          </pre>
        )}
        {!status.running && run && (
          <p className="text-sm text-muted">{statsSummary(run.stats).join(' · ') || 'Aucun changement.'}</p>
        )}
        {!status.running && run?.error && <p className="text-sm text-danger">{run.error}</p>}
        <p className="flex flex-wrap items-center gap-1.5 text-sm text-subtle">
          <Clock className="size-4" aria-hidden />
          {status.enabled ? (
            status.nextRunAt ? (
              <>
                Prochaine synchronisation : <span className="font-medium text-fg">{formatDateTime(status.nextRunAt)}</span>
                <span>
                  ({status.cron}, heure de {status.timezone === 'Europe/Brussels' ? 'Bruxelles' : status.timezone})
                </span>
              </>
            ) : (
              'Aucune exécution planifiée.'
            )
          ) : (
            <>Synchronisation désactivée sur ce serveur (OCC_SYNC_ENABLED=false).</>
          )}
        </p>
      </CardBody>
    </Card>
  )
}

/* ------------------------------------------------------------ historique */

function RunsSection() {
  const [page, setPage] = useState(1)
  const runs = useQuery({ queryKey: qk.admin.syncRuns(page), queryFn: () => adminApi.syncRuns(page), placeholderData: keepPreviousData, refetchInterval: 15_000 })
  const pages = runs.data ? Math.max(1, Math.ceil(runs.data.totalItems / runs.data.perPage)) : 1
  return (
    <section aria-labelledby="sync-runs" className="space-y-3">
      <h2 id="sync-runs" className="font-display text-xl font-semibold">
        Historique
      </h2>
      {runs.isPending && <Skeleton className="h-40 rounded-lg" />}
      {runs.isError && <EmptyState tone="danger" emoji="⚠️" title="Historique indisponible" description={errorMessage(runs.error)} />}
      {runs.data?.items.length === 0 && <EmptyState emoji="🕰️" title="Aucune exécution" description="Lance une première synchronisation, ou attends la prochaine planifiée." />}
      <ul className="space-y-2">
        {runs.data?.items.map((r) => (
          <li key={r.id}>
            <RunRow run={r} />
          </li>
        ))}
      </ul>
      {pages > 1 && (
        <div className="flex items-center justify-end gap-2 text-sm">
          <Button variant="ghost" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            Précédent
          </Button>
          <span className="text-subtle">
            {page} / {pages}
          </span>
          <Button variant="ghost" size="sm" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>
            Suivant
          </Button>
        </div>
      )}
    </section>
  )
}

function RunRow({ run }: { run: SyncRun }) {
  const [open, setOpen] = useState(false)
  const detail = useQuery({ queryKey: qk.admin.syncRun(run.id), queryFn: () => adminApi.syncRun(run.id), enabled: open && run.status !== 'running' })
  const summary = statsSummary(run.stats)
  const panel = `run-${run.id}`
  return (
    <Card>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panel}
        onClick={() => setOpen((o) => !o)}
        className="flex min-h-11 w-full flex-wrap items-center gap-x-3 gap-y-1 rounded-lg p-3 text-left hover:bg-fg/[0.03] focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-brand/30 sm:p-4"
      >
        <Badge variant={RUN_STATUS_VARIANT[run.status]} dot>
          {RUN_STATUS_LABEL[run.status]}
        </Badge>
        <span className="font-medium">{formatDateTime(run.started_at)}</span>
        <span className="text-xs text-subtle">
          {TRIGGER_LABEL[run.trigger]} · {formatDuration(runDuration(run.started_at, run.finished_at))}
        </span>
        <span className="min-w-0 flex-1 basis-full truncate text-sm text-muted sm:basis-auto sm:text-right">
          {run.status === 'running' ? 'en cours…' : summary.length ? summary.join(' · ') : 'aucun changement'}
        </span>
        <ChevronDown className={cn('size-4 shrink-0 text-subtle transition-transform motion-reduce:transition-none', open && 'rotate-180')} aria-hidden />
      </button>
      {open && (
        <div id={panel} className="space-y-4 border-t border-border p-3 sm:p-4">
          {run.error && <p className="text-sm text-danger">{run.error}</p>}
          {run.sources.length > 0 && (
            <div>
              <h3 className="mb-2 text-sm font-semibold">Sources</h3>
              <ul className="space-y-1 text-sm">
                {run.sources.map((s) => (
                  <li key={s.id} className="flex flex-wrap items-center gap-2">
                    <Badge variant={SOURCE_STATUS_VARIANT[s.status]}>{SOURCE_STATUS_LABEL[s.status]}</Badge>
                    <span className="font-medium">{s.label}</span>
                    <span className="text-subtle">
                      {s.message} · {plural(s.network, 'requête')}, {s.cached} en cache · {formatDuration(s.durationMs)}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {run.status === 'running' ? (
            <p className="text-sm text-subtle">Le détail sera disponible à la fin de l'exécution.</p>
          ) : detail.isPending ? (
            <Skeleton className="h-24 rounded-md" />
          ) : detail.isError ? (
            <p className="text-sm text-danger">{errorMessage(detail.error)}</p>
          ) : (
            <>
              <div>
                <h3 className="mb-2 text-sm font-semibold">Changements ({run.changesCount})</h3>
                {detail.data.run.changes?.length ? (
                  <ul className="max-h-80 space-y-0.5 overflow-y-auto rounded-md border border-border bg-elevated p-3 text-sm">
                    {detail.data.run.changes.map((c, i) => (
                      <li key={i}>{c}</li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-sm text-subtle">Aucun changement : tout était déjà à jour.</p>
                )}
              </div>
              {detail.data.run.log && (
                <details>
                  <summary className="cursor-pointer text-sm font-semibold">Journal technique</summary>
                  <pre className="mt-2 max-h-80 overflow-auto rounded-md border border-border bg-elevated p-3 text-xs leading-5 whitespace-pre-wrap">{detail.data.run.log}</pre>
                </details>
              )}
            </>
          )}
        </div>
      )}
    </Card>
  )
}

/* --------------------------------------------------------------- sources */

function SourcesSection() {
  const qc = useQueryClient()
  const sources = useQuery({ queryKey: qk.admin.syncSources, queryFn: adminApi.syncSources, refetchInterval: 30_000 })
  const [editing, setEditing] = useState<SyncSource | null | undefined>(undefined)
  const [deleting, setDeleting] = useState<SyncSource | null>(null)

  const patch = useMutation({
    mutationFn: ({ s, data }: { s: SyncSource; data: Partial<SyncSource> }) => adminApi.saveSyncSource(s.id, data),
    onMutate: async ({ s, data }) => {
      await qc.cancelQueries({ queryKey: qk.admin.syncSources })
      qc.setQueryData<SyncSource[]>(qk.admin.syncSources, (old) => old?.map((x) => (x.id === s.id ? { ...x, ...data } : x)))
    },
    onError: (e) => toast.error(errorMessage(e)),
    onSettled: () => void qc.invalidateQueries({ queryKey: qk.admin.syncSources }),
  })
  const remove = useMutation({
    mutationFn: (s: SyncSource) => adminApi.deleteSyncSource(s.id),
    onSuccess: () => {
      toast.success('Source supprimée')
      setDeleting(null)
      void qc.invalidateQueries({ queryKey: qk.admin.syncSources })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const enabled = sources.data?.filter((s) => s.enabled).length ?? 0
  return (
    <section aria-labelledby="sync-sources" className="space-y-3">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 id="sync-sources" className="font-display text-xl font-semibold">
            Sources
          </h2>
          <p className="text-sm text-muted">
            {sources.data ? `${enabled} activée${enabled > 1 ? 's' : ''} sur ${sources.data.length}. ` : ''}
            Priorité : la plus petite fournit le menu quand un restaurant est présent sur plusieurs sources.
          </p>
        </div>
        <Button variant="secondary" leftIcon={<Plus className="size-4" />} onClick={() => setEditing(null)}>
          Ajouter une source
        </Button>
      </div>
      {sources.isPending && <Skeleton className="h-40 rounded-lg" />}
      {sources.isError && <EmptyState tone="danger" emoji="⚠️" title="Sources indisponibles" description={errorMessage(sources.error)} />}
      {sources.data?.length === 0 && <EmptyState emoji="🔌" title="Aucune source" description="Ajoute une page Deliveroo, weloveat ou le site d'un restaurant." />}
      <ul className="space-y-2" aria-label="Sources de synchronisation">
        {sources.data?.map((s) => {
          const last = parseLastStatus(s.last_status)
          return (
            <li key={s.id}>
              <Card className={cn('flex flex-wrap items-center gap-3 p-3 sm:flex-nowrap sm:p-4', !s.enabled && 'opacity-70')}>
                <Toggle checked={s.enabled} onChange={(v) => patch.mutate({ s, data: { enabled: v } })} label={`Activer : ${s.label}`} />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="truncate font-semibold">{s.label}</span>
                    <Badge
                      variant={
                        s.provider === 'deliveroo'
                          ? 'deliveroo'
                          : s.provider === 'weloveat'
                            ? 'weloveat'
                            : s.provider === 'takeaway-site'
                              ? 'takeaway'
                              : s.provider === 'ubereats-snapshot'
                                ? 'ubereats'
                                : 'neutral'
                      }
                    >
                      {PROVIDER_LABEL[s.provider]}
                    </Badge>
                    <span className="text-xs text-subtle tabular-nums">priorité {s.priority}</span>
                    {s.options && <Badge variant="info">options</Badge>}
                  </div>
                  <p className="truncate text-xs text-subtle">
                    {s.url ? (
                      <a href={s.url} target="_blank" rel="noreferrer noopener" className="hover:underline">
                        {s.url} <ExternalLink className="inline size-3" aria-hidden />
                      </a>
                    ) : (
                      'URL par défaut'
                    )}
                  </p>
                  {last && (
                    <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-xs">
                      <Badge variant={SOURCE_STATUS_VARIANT[last.status]} className="px-2 py-0">
                        {SOURCE_STATUS_LABEL[last.status]}
                      </Badge>
                      <span className="min-w-0 truncate text-subtle" title={last.message}>
                        {last.message} · {formatRelativeTime(s.last_run_at)}
                      </span>
                    </p>
                  )}
                </div>
                <div className="ml-auto flex items-center gap-1">
                  <Button variant="ghost" size="icon" aria-label={`Modifier ${s.label}`} onClick={() => setEditing(s)}>
                    <Pencil className="size-4" />
                  </Button>
                  <Button variant="ghost" size="icon" aria-label={`Supprimer ${s.label}`} onClick={() => setDeleting(s)}>
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              </Card>
            </li>
          )
        })}
      </ul>
      {editing !== undefined && <SourceForm key={editing?.id ?? 'new'} source={editing} nextPriority={(sources.data?.at(-1)?.priority ?? 0) + 10} onClose={() => setEditing(undefined)} />}
      <Sheet
        open={!!deleting}
        onClose={() => setDeleting(null)}
        title="Supprimer cette source ?"
        description={deleting ? `« ${deleting.label} » ne sera plus lue. Les restaurants déjà importés restent ; ceux qu'aucune autre source ne propose passeront « obsolètes » à la prochaine synchronisation.` : undefined}
        footer={
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDeleting(null)}>
              Annuler
            </Button>
            <Button variant="danger" loading={remove.isPending} onClick={() => deleting && remove.mutate(deleting)}>
              Supprimer
            </Button>
          </div>
        }
      >
        <p className="text-sm text-muted">Pour une pause, désactive-la plutôt.</p>
      </Sheet>
    </section>
  )
}

const PROVIDERS: SyncProvider[] = ['takeaway-site', 'deliveroo', 'weloveat', 'jsonld', 'ubereats-snapshot']

/** Sources sans adresse : weloveat (racine par défaut), instantané Uber Eats (fichier embarqué). */
const URL_OPTIONAL: SyncProvider[] = ['weloveat', 'ubereats-snapshot']

function SourceForm({ source, nextPriority, onClose }: { source: SyncSource | null; nextPriority: number; onClose: () => void }) {
  const qc = useQueryClient()
  const [provider, setProvider] = useState<SyncProvider>(source?.provider ?? 'takeaway-site')
  const [label, setLabel] = useState(source?.label ?? '')
  const [url, setUrl] = useState(source?.url ?? '')
  const [priority, setPriority] = useState(String(source?.priority ?? nextPriority))
  const [enabled, setEnabled] = useState(source?.enabled ?? true)
  const [options, setOptions] = useState(source?.options ?? false)
  const [errors, setErrors] = useState<{ url?: string; priority?: string }>({})

  const save = useMutation({
    mutationFn: (data: Partial<SyncSource>) => adminApi.saveSyncSource(source?.id ?? null, data),
    onSuccess: () => {
      toast.success(source ? 'Source enregistrée' : 'Source ajoutée', { description: 'Elle sera lue à la prochaine synchronisation.' })
      void qc.invalidateQueries({ queryKey: qk.admin.syncSources })
      onClose()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const next: typeof errors = {}
    const u = url.trim()
    if (u && provider !== 'ubereats-snapshot' && !/^https?:\/\/[^/\s]+/.test(u)) next.url = 'Adresse en https:// attendue.'
    if (!u && !URL_OPTIONAL.includes(provider)) next.url = 'Adresse requise.'
    const p = Number(priority)
    if (!Number.isInteger(p)) next.priority = 'Nombre entier attendu.'
    setErrors(next)
    if (next.url || next.priority) return
    save.mutate({ provider, label: label.trim(), url: provider === 'ubereats-snapshot' ? '' : u, priority: p, enabled, options, city: source?.city || 'mons' })
  }

  return (
    <Sheet
      open
      onClose={onClose}
      title={source ? `Modifier « ${source.label} »` : 'Nouvelle source'}
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Annuler
          </Button>
          <Button type="submit" form="sync-source-form" loading={save.isPending}>
            Enregistrer
          </Button>
        </div>
      }
    >
      <form id="sync-source-form" onSubmit={submit} className="space-y-4" noValidate>
        <Field label="Type de source" hint={PROVIDER_HINT[provider]}>
          {(p) => (
            <select {...p} className={selectClass} value={provider} onChange={(e) => setProvider(e.target.value as SyncProvider)}>
              {PROVIDERS.map((id) => (
                <option key={id} value={id}>
                  {PROVIDER_LABEL[id]}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Nom" optional>
          {(p) => <Input {...p} data-autofocus value={label} onChange={(e) => setLabel(e.target.value)} maxLength={120} placeholder="Site Tomo" />}
        </Field>
        {provider !== 'ubereats-snapshot' && (
          <Field label="Adresse (URL)" error={errors.url} optional={provider === 'weloveat'}>
            {(p) => <Input {...p} value={url} onChange={(e) => setUrl(e.target.value)} inputMode="url" placeholder="https://…" />}
          </Field>
        )}
        <Field label="Priorité" error={errors.priority} hint="Plus petit = préféré pour le menu et lu en premier.">
          {(p) => <Input {...p} value={priority} onChange={(e) => setPriority(e.target.value)} inputMode="numeric" />}
        </Field>
        <div className="flex flex-col gap-1">
          <Toggle checked={enabled} onChange={setEnabled} label="Activée" showLabel />
          <Toggle checked={options} onChange={setOptions} label="Lire aussi les options (suppléments)" showLabel />
          <p className="text-xs text-subtle">
            Options : une requête de plus par plat sur weloveat (plusieurs heures pour une ville). Deliveroo fournit ses options sans requête supplémentaire.
          </p>
        </div>
      </form>
    </Sheet>
  )
}
