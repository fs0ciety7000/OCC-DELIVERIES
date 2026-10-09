import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, ExternalLink, Plus, Search } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { Badge, Button, Card, CardBody, Field, Input, Spinner } from '@/components/ui'
import { adminApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { formatDistance, plural } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import type { SyncDiscoverFound, SyncDiscoverResult } from '@/lib/types'
import { DISCOVER_STATUS_LABEL, DISCOVER_STATUS_VARIANT, RUN_STATUS_LABEL, RUN_STATUS_VARIANT, statsSummary } from './syncFormat'

/** Étapes affichées pendant la recherche (10 à 20 s). */
function progressText(seconds: number): string {
  if (seconds < 3) return 'Préparation des adresses candidates…'
  if (seconds < 12) return `Vérification des sites candidats, un à la fois (${seconds} s)…`
  return `Lecture de la carte du site trouvé (${seconds} s)… encore quelques secondes.`
}

/**
 * « Découvrir un site Takeaway » : à partir d'un lien takeaway.com, d'un nom ou
 * d'une adresse, le serveur cherche le site satellite officiel du restaurant
 * (takeaway.com lui-même n'est jamais interrogé) et propose de le suivre.
 */
export function DiscoverCard() {
  const [query, setQuery] = useState('')
  const [error, setError] = useState<string>()
  const [startedAt, setStartedAt] = useState(0)
  const [now, setNow] = useState(0)

  const discover = useMutation({ mutationFn: (q: string) => adminApi.discoverSync(q) })

  useEffect(() => {
    if (!discover.isPending) return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [discover.isPending])
  const seconds = Math.max(0, Math.floor((now - startedAt) / 1000))

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const q = query.trim()
    if (!q) {
      setError('Colle un lien takeaway.com ou tape le nom du restaurant.')
      return
    }
    setError(undefined)
    const t = Date.now()
    setStartedAt(t)
    setNow(t)
    discover.mutate(q)
  }

  const data = discover.data
  return (
    <Card>
      <CardBody className="space-y-4">
        <div>
          <h3 className="font-display text-lg font-semibold">Découvrir un site Takeaway</h3>
          <p className="text-sm text-muted">
            Beaucoup de restaurants Takeaway ont un site officiel au même modèle (ex. tomomons.be) : on le cherche, puis on le suit en un clic.
          </p>
        </div>
        <form onSubmit={submit} className="flex flex-col gap-3 sm:flex-row sm:items-end" noValidate>
          <Field label="Lien takeaway.com ou nom du resto" error={error} className="min-w-0 flex-1">
            {(p) => (
              <Input
                {...p}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="https://www.takeaway.com/be-fr/menu/… ou Snack à la Gare"
                maxLength={300}
                autoComplete="off"
              />
            )}
          </Field>
          <Button type="submit" leftIcon={<Search className="size-4" />} loading={discover.isPending} className="sm:mb-0">
            Découvrir
          </Button>
        </form>

        <div aria-live="polite" className="space-y-3">
          {discover.isPending && (
            <p className="flex items-center gap-2 text-sm text-muted">
              <Spinner className="size-4" label="Recherche en cours" />
              {progressText(seconds)}
            </p>
          )}
          {discover.isError && <p className="text-sm text-danger">{errorMessage(discover.error)}</p>}
          {data && !discover.isPending && <DiscoverResults result={data} />}
        </div>
      </CardBody>
    </Card>
  )
}

function DiscoverResults({ result }: { result: SyncDiscoverResult }) {
  if (result.found.length === 0) {
    const checked = result.tried.filter((t) => t.status !== 'skipped')
    return (
      <div className="space-y-2 rounded-md border border-border bg-elevated p-3">
        <p className="font-semibold">Aucun site Takeaway trouvé.</p>
        <p className="text-sm text-muted">
          Si tu connais l'adresse du site, ajoute-la à la main avec « Ajouter une source » (type « Site Takeaway »), ou colle-la ici pour la vérifier.
        </p>
        {checked.length > 0 && (
          <details>
            <summary className="cursor-pointer text-sm font-medium">{plural(checked.length, 'adresse vérifiée', 'adresses vérifiées')}</summary>
            <ul className="mt-2 space-y-1 text-xs" aria-label="Adresses vérifiées">
              {checked.map((t) => (
                <li key={t.host} className="flex flex-wrap items-center gap-2">
                  <Badge variant={DISCOVER_STATUS_VARIANT[t.status]} className="px-2 py-0">
                    {DISCOVER_STATUS_LABEL[t.status]}
                  </Badge>
                  <span className="font-medium">{t.host}</span>
                  {t.message && t.status !== 'absent' && <span className="text-subtle">{t.message}</span>}
                </li>
              ))}
            </ul>
          </details>
        )}
      </div>
    )
  }
  return (
    <ul className="space-y-2" aria-label="Sites trouvés">
      {result.found.map((f) => (
        <li key={f.url}>
          <FoundSite site={f} />
        </li>
      ))}
    </ul>
  )
}

function FoundSite({ site }: { site: SyncDiscoverFound }) {
  const qc = useQueryClient()
  const [runId, setRunId] = useState<string>()
  const [note, setNote] = useState<string>()
  const [followed, setFollowed] = useState(site.alreadySource)

  const add = useMutation({
    mutationFn: async () => {
      const { source } = await adminApi.addSyncSite(site.url, site.name)
      setFollowed(true)
      void qc.invalidateQueries({ queryKey: qk.admin.syncSources })
      try {
        const { run } = await adminApi.startSync(source.id)
        return { runId: run.id, note: undefined }
      } catch (e) {
        return { runId: undefined, note: `Source ajoutée, synchronisation non lancée : ${errorMessage(e)} Elle sera lue à la prochaine synchronisation.` }
      }
    },
    onSuccess: (r) => {
      setRunId(r.runId)
      setNote(r.note)
      if (r.runId) {
        toast.success('Source ajoutée', { description: `Lecture de ${site.name} en cours…` })
        void qc.invalidateQueries({ queryKey: qk.admin.syncStatus })
      }
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const run = useQuery({
    queryKey: qk.admin.syncRun(runId ?? ''),
    queryFn: () => adminApi.syncRun(runId!),
    enabled: !!runId,
    refetchInterval: (q) => (q.state.data?.run.status === 'running' || !q.state.data ? 2000 : false),
  })
  const done = run.data && run.data.run.status !== 'running' ? run.data.run : undefined
  useEffect(() => {
    if (done) void qc.invalidateQueries({ queryKey: qk.admin.all })
  }, [done, qc])

  const facts = [plural(site.items, 'plat'), plural(site.categories, 'catégorie'), site.distanceKm != null ? `à ${formatDistance(site.distanceKm)}` : '']
    .filter(Boolean)
    .join(' · ')

  return (
    <div className="flex flex-wrap items-start gap-3 rounded-md border border-border bg-elevated p-3">
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{site.name}</span>
          {followed && !runId && (
            <Badge variant="success">
              <Check className="size-3" aria-hidden />
              Déjà suivi
            </Badge>
          )}
        </p>
        {site.address && <p className="text-sm text-muted">{site.address}</p>}
        <p className="text-sm text-subtle">{facts}</p>
        <a href={site.url} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-1 text-sm text-brand hover:underline">
          {site.host} <ExternalLink className="size-3" aria-hidden />
          <span className="sr-only">(ouvre le site dans un nouvel onglet)</span>
        </a>
        {runId && (
          <p className="flex flex-wrap items-center gap-2 pt-1 text-sm" role="status">
            {done ? (
              <>
                <Badge variant={RUN_STATUS_VARIANT[done.status]} dot>
                  {RUN_STATUS_LABEL[done.status]}
                </Badge>
                <span className="text-muted">{done.error || statsSummary(done.stats).join(' · ') || 'Aucun changement : déjà à jour.'}</span>
              </>
            ) : (
              <>
                <Spinner className="size-4" label="Synchronisation en cours" />
                <span className="text-muted">Synchronisation de cette source…</span>
              </>
            )}
          </p>
        )}
        {note && <p className="pt-1 text-sm text-warning">{note}</p>}
      </div>
      {!followed && (
        <Button size="sm" leftIcon={<Plus className="size-4" />} loading={add.isPending} onClick={() => add.mutate()}>
          Ajouter et synchroniser
        </Button>
      )}
    </div>
  )
}
