import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CircleCheck, CircleX, DatabaseBackup, ShieldCheck, TriangleAlert } from 'lucide-react'
import { type ReactNode, useEffect, useRef } from 'react'
import { toast } from 'sonner'
import { Badge, Button, Card, CardBody, Skeleton } from '@/components/ui'
import { adminApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { plural } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import type { BackupCheck, BackupStatus } from '@/lib/types'
import { CHECK_TRIGGER_LABEL, errorLines, formatAge, formatBackupDate, formatBytes, HEALTH_LABEL, HEALTH_VARIANT } from './backupFormat'

export interface BackupsCardProps {
  status: BackupStatus
  onRun: () => void
  onVerify: () => void
  /** Requête de lancement en vol (avant la réponse 202). */
  starting?: 'backup' | 'verify' | null
  now?: Date
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5 py-2">
      <dt className="text-sm text-muted">{label}</dt>
      <dd className="min-w-0 text-right text-sm font-semibold break-words">{children}</dd>
    </div>
  )
}

function Verification({ check }: { check: BackupCheck | null }) {
  if (!check) {
    return <p className="text-sm text-muted">Aucune vérification de restauration pour l'instant.</p>
  }
  const errors = errorLines(check.error)
  return (
    <div className="space-y-2">
      <p className="flex items-center gap-2 font-semibold">
        {check.ok ? (
          <CircleCheck className="size-5 shrink-0 text-success" aria-hidden />
        ) : (
          <CircleX className="size-5 shrink-0 text-danger" aria-hidden />
        )}
        <span>{check.ok ? 'Restauration vérifiée' : 'Échec de la vérification'}</span>
      </p>
      <p className="text-xs text-subtle">
        {formatBackupDate(check.created)} · {CHECK_TRIGGER_LABEL[check.trigger] ?? check.trigger}
        {check.backupKey && (
          <>
            {' · '}
            <span className="break-all">{check.backupKey}</span>
          </>
        )}
      </p>
      {errors.length > 0 && (
        <ul className="space-y-1 rounded-md border border-danger/25 bg-danger/8 p-3 text-sm text-danger">
          {errors.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
      )}
      {check.warnings.length > 0 && (
        <ul className="space-y-1 rounded-md border border-warning/25 bg-warning/8 p-3 text-sm text-warning">
          {check.warnings.map((w) => (
            <li key={w}>{w}</li>
          ))}
        </ul>
      )}
      {check.counts.length > 0 && (
        <table className="w-full text-sm">
          <caption className="sr-only">Enregistrements dans la sauvegarde restaurée et dans la base actuelle</caption>
          <thead>
            <tr className="text-left text-xs text-subtle">
              <th scope="col" className="py-1 font-medium">Table</th>
              <th scope="col" className="py-1 text-right font-medium">Sauvegarde</th>
              <th scope="col" className="py-1 text-right font-medium">Actuel</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {check.counts.map((c) => (
              <tr key={c.name}>
                <th scope="row" className="py-1 text-left font-normal capitalize">{c.label || c.name}</th>
                <td className="py-1 text-right tabular-nums">{c.backup < 0 ? '—' : c.backup}</td>
                <td className="py-1 text-right text-muted tabular-nums">{c.live < 0 ? '—' : c.live}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {check.ok && check.integrity && (
        <p className="flex items-center gap-1.5 text-xs text-subtle">
          <ShieldCheck className="size-3.5" aria-hidden />
          Intégrité SQLite contrôlée (integrity_check, quick_check)
        </p>
      )}
    </div>
  )
}

/** Carte « Sauvegardes » (présentation pure, testée). */
export function BackupsCard({ status: s, onRun, onVerify, starting = null, now }: BackupsCardProps) {
  const running = s.running ?? starting
  const off = s.disabledByEnv
  const lastRun = s.lastBackupRun
  return (
    <Card>
      <CardBody className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="flex items-center gap-2 font-display text-lg font-semibold">
            <DatabaseBackup className="size-5 text-brand" aria-hidden />
            Sauvegardes
          </h2>
          <Badge variant={off ? 'neutral' : HEALTH_VARIANT[s.health]} dot>
            {off ? 'Désactivées' : HEALTH_LABEL[s.health]}
          </Badge>
        </div>

        {s.health === 'stale' && (
          <p role="alert" className="flex items-start gap-2 rounded-md border border-danger/25 bg-danger/8 p-3 text-sm text-danger">
            <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
            <span>
              Aucune sauvegarde réussie depuis plus de {s.maxAgeHours} h. Lance une sauvegarde et vérifie la configuration du stockage
              (variables OCC_BACKUP_S3_*).
            </span>
          </p>
        )}
        {off && (
          <p role="status" className="rounded-md border border-border bg-fg/[0.04] p-3 text-sm">
            <span className="font-semibold">Sauvegardes désactivées sur cette instance</span> (OCC_BACKUP_DISABLED) : copie restaurée
            ou préproduction, aucune sauvegarde n'est envoyée vers le stockage de production.
          </p>
        )}
        {!off && s.health === 'disabled' && (
          <p className="text-sm text-muted">
            Les sauvegardes automatiques ne sont pas configurées : définir les variables OCC_BACKUP_S3_* (Cloudflare R2) puis redéployer.
          </p>
        )}
        {s.listError && (
          <p role="alert" className="rounded-md border border-danger/25 bg-danger/8 p-3 text-sm text-danger">
            {s.listError}
          </p>
        )}
        {lastRun && !lastRun.ok && (
          <p role="alert" className="rounded-md border border-danger/25 bg-danger/8 p-3 text-sm text-danger">
            Dernière sauvegarde en échec ({formatBackupDate(lastRun.created)}) : {lastRun.error}
          </p>
        )}

        <dl className="divide-y divide-border">
          <Row label="Dernière sauvegarde">
            {s.lastBackup ? (
              <>
                {formatBackupDate(s.lastBackup.modified)}{' '}
                <span className="font-normal text-muted">
                  ({formatAge(s.lastBackup.modified, now)}, {formatBytes(s.lastBackup.size)})
                </span>
              </>
            ) : (
              <span className="font-normal text-muted">Aucune</span>
            )}
          </Row>
          <Row label="Stockage">
            {s.storage === 's3' ? `Cloudflare R2 / S3${s.bucket ? ` — ${s.bucket}` : ''}` : 'Local (pb_data/backups)'}
          </Row>
          {s.configured && (
            <>
              <Row label="Prochaine sauvegarde">{s.nextBackupAt ? formatBackupDate(s.nextBackupAt) : '—'}</Row>
              <Row label="Rétention">
                {plural(s.keep, 'sauvegarde')} · {plural(s.totalBackups, 'archive')} en stock
              </Row>
            </>
          )}
        </dl>

        <div className="border-t border-border pt-4">
          <h3 className="mb-2 text-sm font-semibold text-muted">Dernière vérification de restauration</h3>
          <Verification check={s.lastVerification} />
        </div>

        <div className="flex flex-wrap gap-2">
          <Button onClick={onRun} loading={running === 'backup'} disabled={off || running !== null}>
            {running === 'backup' ? 'Sauvegarde en cours…' : 'Sauvegarder maintenant'}
          </Button>
          <Button variant="secondary" onClick={onVerify} loading={running === 'verify'} disabled={off || running !== null || !s.lastBackup}>
            {running === 'verify' ? 'Vérification en cours…' : 'Vérifier la restauration'}
          </Button>
        </div>
      </CardBody>
    </Card>
  )
}

/** Conteneur : données, sondage pendant une opération, toasts de fin. */
export function BackupsSection() {
  const qc = useQueryClient()
  const status = useQuery({
    queryKey: qk.admin.backups,
    queryFn: adminApi.backups,
    staleTime: 30_000,
    refetchInterval: (q) => (q.state.data?.running ? 3_000 : false),
  })
  const run = useMutation({
    mutationFn: adminApi.runBackup,
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.admin.backups }),
    onError: (e) => toast.error(errorMessage(e)),
  })
  const verify = useMutation({
    mutationFn: adminApi.verifyBackup,
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.admin.backups }),
    onError: (e) => toast.error(errorMessage(e)),
  })

  // fin d'une opération : toast selon le résultat
  const prev = useRef<BackupStatus['running']>(null)
  const data = status.data
  useEffect(() => {
    if (!data) return
    if (prev.current && !data.running) {
      const v = data.lastVerification
      if (v?.ok) toast.success('Sauvegarde vérifiée', { description: 'La dernière sauvegarde se restaure correctement.' })
      else if (v) toast.error('Vérification de la restauration en échec', { description: errorLines(v.error)[0] })
    }
    prev.current = data.running
  }, [data])

  if (status.isError) {
    return (
      <Card>
        <CardBody className="space-y-3">
          <h2 className="font-display text-lg font-semibold">Sauvegardes</h2>
          <p className="text-sm text-danger">{errorMessage(status.error)}</p>
          <Button variant="secondary" onClick={() => status.refetch()}>
            Réessayer
          </Button>
        </CardBody>
      </Card>
    )
  }
  if (!data) return <Skeleton className="h-[260px] rounded-lg" />
  const starting = run.isPending ? 'backup' : verify.isPending ? 'verify' : null
  return <BackupsCard status={data} starting={starting} onRun={() => run.mutate()} onVerify={() => verify.mutate()} />
}
