import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { adminApi } from '@/lib/api'
import type { BackupCheck, BackupStatus } from '@/lib/types'
import { BackupsCard, BackupsSection } from './BackupsCard'
import { errorLines, formatAge, formatBackupDate, formatBytes } from './backupFormat'

const verifyOK: BackupCheck = {
  id: 'v1', kind: 'verify', trigger: 'manual', ok: true,
  backupKey: '@auto_pb_backup_occ_deliveries_20261011001500.zip', backupSize: 101564, backupModified: '2026-10-11T00:15:00Z',
  durationMs: 480, integrity: 'integrity_check: ok\nquick_check: ok', error: '', warnings: [],
  counts: [
    { name: 'users', label: 'utilisateurs', backup: 42, live: 43 },
    { name: 'parties', label: 'commandes', backup: 30, live: 30 },
  ],
  created: '2026-10-11T00:15:10Z',
}

const base: BackupStatus = {
  configured: true, fromEnv: true, storage: 's3', bucket: 'occ-deliveries-backups', cron: '15 0 * * *', timezone: 'UTC', keep: 14,
  nextBackupAt: '2026-10-12T00:15:00Z', running: null, health: 'ok', maxAgeHours: 36,
  lastBackup: { key: verifyOK.backupKey, size: 101564, modified: '2026-10-11T00:15:00Z' },
  backups: [{ key: verifyOK.backupKey, size: 101564, modified: '2026-10-11T00:15:00Z' }],
  totalBackups: 1, listError: '', lastVerification: verifyOK, lastBackupRun: null, lastAlertAt: '',
}

const now = new Date('2026-10-11T03:15:00Z')

describe('backupFormat', () => {
  it('formate tailles, dates et âges', () => {
    expect(formatBytes(512)).toBe('512 o')
    expect(formatBytes(101564)).toMatch(/^99 Ko$/)
    expect(formatBytes(5 * 1024 * 1024)).toMatch(/^5,0 Mo$/)
    expect(formatBytes(-1)).toBe('—')
    // 00:15 UTC = 02:15 à Bruxelles (heure d'été)
    expect(formatBackupDate('2026-10-11T00:15:00Z')).toMatch(/11 oct\.? 2026.*02:15/)
    expect(formatBackupDate('')).toBe('')
    expect(formatBackupDate('nope')).toBe('')
    expect(formatAge('2026-10-11T00:15:00Z', now)).toBe('il y a 3 h')
    expect(formatAge('2026-10-11T03:10:00Z', now)).toBe('il y a 5 min')
    expect(formatAge('2026-10-08T03:15:00Z', now)).toBe('il y a 3 j')
    expect(errorLines('a\n\n b \n')).toEqual(['a', 'b'])
  })
})

describe('BackupsCard', () => {
  it('affiche la dernière sauvegarde et la vérification réussie', async () => {
    const onRun = vi.fn()
    const onVerify = vi.fn()
    render(<BackupsCard status={base} onRun={onRun} onVerify={onVerify} now={now} />)
    expect(screen.getByRole('heading', { name: 'Sauvegardes' })).toBeInTheDocument()
    expect(screen.getByText('À jour')).toBeInTheDocument()
    expect(screen.getByText(/Cloudflare R2 \/ S3 — occ-deliveries-backups/)).toBeInTheDocument()
    expect(screen.getByText('Restauration vérifiée')).toBeInTheDocument()
    const table = screen.getByRole('table', { name: /sauvegarde restaurée/ })
    expect(within(table).getByRole('rowheader', { name: 'utilisateurs' })).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Sauvegarder maintenant' }))
    expect(onRun).toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Vérifier la restauration' }))
    expect(onVerify).toHaveBeenCalled()
  })

  it('alerte si aucune sauvegarde depuis 36 h et affiche l’échec de vérification', () => {
    const failed: BackupCheck = { ...verifyOK, ok: false, error: 'PRAGMA integrity_check : Page 3 is never used\nTables absentes de la sauvegarde : users.', counts: [] }
    render(<BackupsCard status={{ ...base, health: 'stale', lastVerification: failed }} onRun={vi.fn()} onVerify={vi.fn()} now={now} />)
    expect(screen.getByText('En retard')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('depuis plus de 36 h')
    expect(screen.getByText('Échec de la vérification')).toBeInTheDocument()
    expect(screen.getByText(/Tables absentes/)).toBeInTheDocument()
  })

  it('désactive les boutons pendant une opération et sans sauvegarde', () => {
    const { rerender } = render(<BackupsCard status={{ ...base, running: 'backup' }} onRun={vi.fn()} onVerify={vi.fn()} />)
    expect(screen.getByRole('button', { name: /Sauvegarde en cours/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Vérifier la restauration' })).toBeDisabled()

    rerender(
      <BackupsCard
        status={{ ...base, health: 'disabled', configured: false, storage: 'local', bucket: '', lastBackup: null, backups: [], totalBackups: 0, lastVerification: null }}
        onRun={vi.fn()}
        onVerify={vi.fn()}
      />,
    )
    expect(screen.getByText('Non configurées')).toBeInTheDocument()
    expect(screen.getByText(/OCC_BACKUP_S3_\*/)).toBeInTheDocument()
    expect(screen.getByText('Local (pb_data/backups)')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sauvegarder maintenant' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Vérifier la restauration' })).toBeDisabled()
  })
})

describe('BackupsSection', () => {
  afterEach(() => vi.restoreAllMocks())

  it('lance une sauvegarde puis relit l’état', async () => {
    const get = vi.spyOn(adminApi, 'backups').mockResolvedValue(base)
    const run = vi.spyOn(adminApi, 'runBackup').mockResolvedValue({ started: true, running: 'backup' })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <BackupsSection />
      </QueryClientProvider>,
    )
    await userEvent.click(await screen.findByRole('button', { name: 'Sauvegarder maintenant' }))
    expect(run).toHaveBeenCalled()
    await vi.waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  })
})
