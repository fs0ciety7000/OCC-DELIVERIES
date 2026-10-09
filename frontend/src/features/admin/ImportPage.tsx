import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, CircleAlert, Download, ExternalLink, FileJson, FileSpreadsheet, Upload } from 'lucide-react'
import { ClientResponseError } from 'pocketbase'
import { useRef, useState, type DragEvent } from 'react'
import { toast } from 'sonner'
import { Badge, Button, Card, CardBody, Money } from '@/components/ui'
import { adminApi, saveBlob, type ImportResponse } from '@/lib/api'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { plural } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import type { ImportReport, ImportRestaurantReport, RestaurantImport } from '@/lib/types'
import { AdminHeader } from './AdminLayout'
import { csvTemplate, detectImportKind, JSON_EXAMPLE, parseImportJson } from './importFormat'

type Pending = { kind: 'json'; name: string; body: RestaurantImport | RestaurantImport[] } | { kind: 'csv'; name: string; file: File }

function send(p: Pending, dryRun: boolean): Promise<ImportResponse> {
  return p.kind === 'json' ? adminApi.importJson(p.body, dryRun) : adminApi.importCsv(p.file, dryRun)
}

/** Le serveur renvoie 400 + `report` quand un import réel est refusé. */
function reportFromError(err: unknown): ImportReport | null {
  if (err instanceof ClientResponseError) {
    const report = (err.response as { report?: ImportReport }).report
    if (report) return report
  }
  return null
}

export function ImportPage() {
  const [pending, setPending] = useState<Pending | null>(null)
  const [report, setReport] = useState<ImportReport | null>(null)
  const [readError, setReadError] = useState<string | null>(null)
  const [dragging, setDragging] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const qc = useQueryClient()

  const check = useMutation({
    mutationFn: (p: Pending) => send(p, true),
    onSuccess: (res) => setReport(res.report),
    onError: (e) => {
      setReport(reportFromError(e))
      if (!reportFromError(e)) setReadError(errorMessage(e))
    },
  })
  const commit = useMutation({
    mutationFn: (p: Pending) => send(p, false),
    onSuccess: (res) => {
      setReport(res.report)
      toast.success(`Import terminé : ${plural(res.report.restaurants.length, 'restaurant')}, ${plural(res.report.items, 'article')}`)
      void qc.invalidateQueries({ queryKey: qk.admin.all })
      void qc.invalidateQueries({ queryKey: ['nearby'] })
      void qc.invalidateQueries({ queryKey: ['menu'] })
    },
    onError: (e) => {
      const rep = reportFromError(e)
      if (rep) setReport(rep)
      toast.error(errorMessage(e))
    },
  })

  const load = async (file: File) => {
    setReport(null)
    setReadError(null)
    commit.reset()
    if (file.size > 10 * 1024 * 1024) {
      setReadError('Fichier trop volumineux (10 Mo maximum).')
      return
    }
    const text = await file.text()
    const kind = detectImportKind(file.name, text)
    if (!kind) {
      setReadError('Format non reconnu : dépose un fichier .json ou .csv.')
      return
    }
    let p: Pending
    if (kind === 'json') {
      try {
        p = { kind, name: file.name, body: parseImportJson(text) }
      } catch (err) {
        setReadError(`JSON illisible : ${errorMessage(err)}`)
        return
      }
    } else {
      p = { kind, name: file.name, file }
    }
    setPending(p)
    check.mutate(p)
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    const f = e.dataTransfer.files[0]
    if (f) void load(f)
  }

  const done = commit.isSuccess
  const canImport = !!pending && !!report?.valid && !done

  return (
    <div className="space-y-6">
      <AdminHeader
        title="Import & sauvegarde"
        description="Ajoute ou mets à jour des restaurants et leurs menus en masse (mise à jour par slug : le menu est remplacé)."
        actions={
          <Button
            variant="secondary"
            leftIcon={<Download className="size-4" />}
            onClick={() => adminApi.downloadExport().then(() => toast.success('Export téléchargé'), (e: unknown) => toast.error(errorMessage(e)))}
          >
            Exporter tout (JSON)
          </Button>
        }
      />

      <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="space-y-4">
          <div
            onDragOver={(e) => {
              e.preventDefault()
              setDragging(true)
            }}
            onDragLeave={() => setDragging(false)}
            onDrop={onDrop}
            className={cn(
              'flex flex-col items-center gap-3 rounded-lg border-2 border-dashed px-6 py-10 text-center transition-colors duration-[120ms]',
              dragging ? 'border-brand bg-brand/8' : 'border-border-strong bg-surface',
            )}
          >
            <Upload className="size-8 text-brand" aria-hidden />
            <p className="font-display text-lg font-semibold">Dépose un fichier JSON ou CSV</p>
            <p className="max-w-md text-sm text-muted">On vérifie d'abord tout (aperçu, erreurs) sans rien modifier ; tu confirmes ensuite.</p>
            <Button variant="secondary" onClick={() => inputRef.current?.click()} loading={check.isPending}>
              Choisir un fichier
            </Button>
            <input
              ref={inputRef}
              type="file"
              accept=".json,.csv,application/json,text/csv"
              className="sr-only"
              tabIndex={-1}
              aria-label="Fichier à importer"
              onChange={(e) => {
                const f = e.target.files?.[0]
                if (f) void load(f)
                e.target.value = ''
              }}
            />
          </div>

          {readError && (
            <p role="alert" className="flex items-start gap-2 rounded-md border border-danger/30 bg-danger/10 p-3 text-sm text-danger">
              <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden /> {readError}
            </p>
          )}

          {report && pending && <ReportView report={report} fileName={pending.name} done={done} />}

          {report && pending && (
            <div className="sticky bottom-[calc(var(--tabbar-h)+env(safe-area-inset-bottom)+12px)] z-10 flex flex-wrap items-center justify-end gap-2 rounded-lg border border-border bg-elevated/95 p-3 shadow-card backdrop-blur md:bottom-4">
              <p className="mr-auto text-sm text-muted">
                {done ? 'Import effectué.' : report.valid ? `Prêt : ${plural(report.restaurants.length, 'restaurant')}, ${plural(report.items, 'article')}.` : 'Corrige les erreurs puis redépose le fichier.'}
              </p>
              <Button
                variant="ghost"
                onClick={() => {
                  setPending(null)
                  setReport(null)
                  commit.reset()
                }}
              >
                {done ? 'Nouvel import' : 'Annuler'}
              </Button>
              {!done && (
                <Button disabled={!canImport} loading={commit.isPending} onClick={() => pending && commit.mutate(pending)}>
                  Importer
                </Button>
              )}
            </div>
          )}
        </div>

        <aside className="space-y-4">
          <Card>
            <CardBody className="space-y-3">
              <h2 className="font-display text-lg font-semibold">Modèles</h2>
              <p className="text-sm text-muted">
                CSV : une ligne par article, prix en euros (<span className="tabular-nums">12,50</span>), séparateur « ; » ou « , ». Les infos du restaurant (adresse, lat, lng…) ne sont à remplir qu'une fois.
              </p>
              <div className="flex flex-wrap gap-2">
                <Button variant="secondary" size="sm" leftIcon={<FileSpreadsheet className="size-4" />} onClick={() => saveBlob(new Blob([csvTemplate()], { type: 'text/csv;charset=utf-8' }), 'occ-modele-menu.csv')}>
                  Modèle CSV
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  leftIcon={<FileJson className="size-4" />}
                  onClick={() => saveBlob(new Blob([JSON.stringify(JSON_EXAMPLE, null, 2)], { type: 'application/json' }), 'occ-exemple-menu.json')}
                >
                  Exemple JSON
                </Button>
              </div>
              <p className="text-xs text-subtle">Le JSON reprend le format de l'export : idéal pour sauvegarder, éditer puis réimporter. Options (tailles, suppléments) : JSON ou éditeur de menu.</p>
            </CardBody>
          </Card>
          <Card>
            <CardBody className="space-y-2">
              <h2 className="font-display text-lg font-semibold">Depuis Uber Eats / Takeaway</h2>
              <p className="text-sm text-muted">Un favori à glisser dans ton navigateur récupère le menu de la page du resto ouverte et produit un JSON à importer ici.</p>
              <a href="/outils/export-menu.html" className="inline-flex min-h-11 items-center gap-1.5 text-sm font-semibold text-brand hover:underline">
                Ouvrir l'outil d'export <ExternalLink className="size-4" aria-hidden />
              </a>
            </CardBody>
          </Card>
        </aside>
      </div>
    </div>
  )
}

function ReportView({ report, fileName, done }: { report: ImportReport; fileName: string; done: boolean }) {
  const errorCount = report.errors.length + report.restaurants.reduce((n, r) => n + r.errors.length, 0)
  return (
    <section aria-labelledby="report-title" className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 id="report-title" className="font-display text-xl font-semibold">
          {done ? 'Résultat' : 'Aperçu'} — <span className="font-normal text-muted">{fileName}</span>
        </h2>
        {report.valid ? <Badge variant="success" dot>Valide</Badge> : <Badge variant="danger" dot>{plural(errorCount, 'erreur')}</Badge>}
      </div>
      {report.errors.length > 0 && (
        <ul role="alert" className="space-y-1 rounded-md border border-danger/30 bg-danger/10 p-3 text-sm text-danger">
          {report.errors.map((e, i) => (
            <li key={i} className="flex gap-2">
              <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden /> {e}
            </li>
          ))}
        </ul>
      )}
      {report.restaurants.map((r, i) => (
        <RestaurantReportCard key={`${r.slug}-${i}`} r={r} />
      ))}
    </section>
  )
}

function RestaurantReportCard({ r }: { r: ImportRestaurantReport }) {
  return (
    <Card>
      <CardBody className="space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-semibold">{r.name || <span className="text-danger">Sans nom</span>}</p>
          <span className="text-xs text-subtle">{r.slug}</span>
          <Badge variant={r.exists ? 'info' : 'brand'}>{r.exists ? 'Mise à jour' : 'Nouveau'}</Badge>
          {!r.active && <Badge>Masqué</Badge>}
          <span className="ml-auto text-sm text-muted">
            {plural(r.categories, 'catégorie')} · {plural(r.items, 'article')}
          </span>
        </div>
        {r.errors.map((e, i) => (
          <p key={`e${i}`} className="flex gap-2 text-sm font-medium text-danger">
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden /> {e}
          </p>
        ))}
        {r.warnings.map((w, i) => (
          <p key={`w${i}`} className="flex gap-2 text-sm text-warning">
            <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden /> {w}
          </p>
        ))}
        {r.menu.length > 0 && (
          <details className="group">
            <summary className="cursor-pointer rounded-sm py-1 text-sm font-semibold text-muted hover:text-fg">Voir le menu</summary>
            <div className="mt-2 overflow-x-auto">
              <table className="w-full min-w-[480px] text-left text-sm">
                <thead className="text-xs text-subtle">
                  <tr>
                    <th className="py-1.5 pr-3 font-medium">Catégorie</th>
                    <th className="py-1.5 pr-3 font-medium">Article</th>
                    <th className="py-1.5 pr-3 text-right font-medium">Prix</th>
                    <th className="py-1.5 pr-3 font-medium">Étiquettes</th>
                    <th className="py-1.5 font-medium">Infos</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {r.menu.map((m, i) => (
                    <tr key={i}>
                      <td className="py-1.5 pr-3 text-muted">{m.category}</td>
                      <td className="py-1.5 pr-3">{m.name}</td>
                      <td className="py-1.5 pr-3 text-right">
                        <Money cents={m.price} />
                      </td>
                      <td className="py-1.5 pr-3 text-muted">{m.tags.join(', ')}</td>
                      <td className="py-1.5 text-xs text-subtle">
                        {[m.popular && '★ populaire', !m.available && 'indisponible', m.options > 0 && plural(m.options, 'option')].filter(Boolean).join(' · ')}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </details>
        )}
      </CardBody>
    </Card>
  )
}
