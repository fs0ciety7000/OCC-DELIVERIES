import type { BadgeVariant } from '@/components/ui'
import type { SyncProvider, SyncRunStatus, SyncSourceStatus, SyncStats, SyncTrigger } from '@/lib/types'

export const RUN_STATUS_LABEL: Record<SyncRunStatus, string> = {
  running: 'En cours',
  success: 'Réussie',
  partial: 'Partielle',
  failed: 'Échec',
  blocked: 'Bloquée',
}

export const RUN_STATUS_VARIANT: Record<SyncRunStatus, BadgeVariant> = {
  running: 'info',
  success: 'success',
  partial: 'warning',
  failed: 'danger',
  blocked: 'danger',
}

export const TRIGGER_LABEL: Record<SyncTrigger, string> = {
  cron: 'Planifiée',
  manual: 'Manuelle',
  startup: 'Au démarrage',
}

export const PROVIDER_LABEL: Record<SyncProvider, string> = {
  deliveroo: 'Deliveroo',
  weloveat: 'weloveat',
  'takeaway-site': 'Site Takeaway',
  jsonld: 'Site (schema.org)',
}

export const PROVIDER_HINT: Record<SyncProvider, string> = {
  deliveroo: 'URL de la page liste Deliveroo de la ville (avec ?geohash=…).',
  weloveat: "Racine de l'API weloveat (vide = https://api.weloveat.be/api/).",
  'takeaway-site': 'Site satellite du restaurant construit par Takeaway (ex. https://www.tomomons.be/).',
  jsonld: 'Page carte du restaurant publiant un menu schema.org (JSON-LD ou microdonnées).',
}

export const SOURCE_STATUS_LABEL: Record<SyncSourceStatus, string> = { ok: 'OK', blocked: 'Bloquée', failed: 'Erreur' }
export const SOURCE_STATUS_VARIANT: Record<SyncSourceStatus, BadgeVariant> = { ok: 'success', blocked: 'danger', failed: 'warning' }

/** « ok: 40 restaurants » → { status: 'ok', message: '40 restaurants' } ; vide → null. */
export function parseLastStatus(raw: string | null | undefined): { status: SyncSourceStatus; message: string } | null {
  const s = (raw ?? '').trim()
  if (!s) return null
  const i = s.indexOf(':')
  const head = (i < 0 ? s : s.slice(0, i)).trim()
  const message = i < 0 ? '' : s.slice(i + 1).trim()
  const status: SyncSourceStatus = head === 'ok' || head === 'blocked' ? head : 'failed'
  return { status, message }
}

/** Durée lisible : 95 000 ms → « 1 min 35 s ». */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '—'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s} s`
  const m = Math.floor(s / 60)
  const rest = s % 60
  if (m < 60) return rest ? `${m} min ${rest} s` : `${m} min`
  const h = Math.floor(m / 60)
  return `${h} h ${String(m % 60).padStart(2, '0')}`
}

/** Durée d'une exécution (en cours : jusqu'à `now`). */
export function runDuration(startedAt: string, finishedAt: string, now: Date = new Date()): number {
  const start = Date.parse(startedAt.replace(' ', 'T'))
  const end = finishedAt ? Date.parse(finishedAt.replace(' ', 'T')) : now.getTime()
  return Number.isFinite(start) && Number.isFinite(end) ? Math.max(0, end - start) : NaN
}

/** Résumé court des compteurs d'une exécution, sans les zéros. */
export function statsSummary(s: SyncStats): string[] {
  const parts: [number, string, string][] = [
    [s.restaurants_created, 'restaurant créé', 'restaurants créés'],
    [s.restaurants_updated, 'restaurant mis à jour', 'restaurants mis à jour'],
    [s.restaurants_stale, 'restaurant obsolète', 'restaurants obsolètes'],
    [s.items_created, 'plat ajouté', 'plats ajoutés'],
    [s.items_price_changed, 'prix modifié', 'prix modifiés'],
    [Math.max(0, s.items_updated - s.items_price_changed - s.items_unavailable), 'autre plat modifié', 'autres plats modifiés'],
    [s.items_unavailable, 'plat indisponible', 'plats indisponibles'],
  ]
  return parts.filter(([n]) => n > 0).map(([n, one, many]) => `${n.toLocaleString('fr-BE')} ${n > 1 ? many : one}`)
}
