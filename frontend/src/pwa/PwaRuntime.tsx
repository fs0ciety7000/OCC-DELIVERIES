import { useQueryClient } from '@tanstack/react-query'
import { CloudOff, RefreshCw } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { useAuth } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'
import { replayOutbox, useOutboxCount } from '@/lib/offlineActions'
import { useOnline } from '@/lib/online'
import { actionLabel } from '@/lib/outbox'
import { pb } from '@/lib/pb'
import { syncPush } from '@/lib/push'
import { qk } from '@/lib/queryKeys'
import type { NotificationPayload } from '@/lib/types'
import { clearUserCache, onOpenUrl } from './register'

/** Sujet temps réel des toasts in-app (backend/internal/app/push.go). */
export const IN_APP_TOPIC = 'occ/notifications'

/** Bandeau « Hors ligne » (+ actions en attente d'envoi). */
export function OfflineBanner({ online, pending }: { online: boolean; pending: number }) {
  if (online && pending === 0) return null
  return (
    <div role="status" aria-live="polite" className="mb-4 flex items-start gap-3 rounded-md border border-warning/30 bg-warning/10 px-3.5 py-2.5 text-sm text-fg">
      {online ? <RefreshCw aria-hidden className="mt-0.5 size-4 shrink-0 text-warning" /> : <CloudOff aria-hidden className="mt-0.5 size-4 shrink-0 text-warning" />}
      <p className="min-w-0">
        {online ? (
          <>Connexion rétablie : tes actions hors ligne vont être envoyées.</>
        ) : (
          <>
            <strong className="font-semibold">Hors ligne</strong> — les données affichées peuvent dater.
          </>
        )}
        {pending > 0 && (
          <span className="block text-muted">
            {pending} action{pending > 1 ? 's' : ''} en attente d'envoi (votes, panier, « prêt·e »).
          </span>
        )}
      </p>
    </div>
  )
}

/**
 * Exécution PWA montée une fois dans le shell : bandeau hors ligne, rejeu de la file au
 * retour du réseau, toasts temps réel (rappels, remboursements…), rattachement de
 * l'abonnement push au compte connecté, nettoyage du cache à la déconnexion.
 */
export function PwaRuntime() {
  const online = useOnline()
  const pending = useOutboxCount()
  const { user } = useAuth()
  const qc = useQueryClient()
  const navigate = useNavigate()

  // clic sur une notification avec l'app déjà ouverte → navigation interne
  useEffect(() => {
    onOpenUrl((url) => navigate(url))
    return () => onOpenUrl(null)
  }, [navigate])

  // rejeu de la file : au démarrage et à chaque retour du réseau
  useEffect(() => {
    if (!online || !user) return
    let cancelled = false
    void replayOutbox().then((res) => {
      if (cancelled) return
      const parties = new Set([...res.done, ...res.failed.map((f) => f.entry)].map((e) => e.action.partyId))
      parties.forEach((id) => void qc.invalidateQueries({ queryKey: qk.party(id) }))
      if (res.done.length > 0) toast.success(res.done.length > 1 ? `${res.done.length} actions hors ligne synchronisées` : `Synchronisé : ${actionLabel(res.done[0]!.action)}`)
      for (const f of res.failed) toast.error(`Non synchronisé : ${actionLabel(f.entry.action)}`, { description: errorMessage(f.error) })
    })
    return () => {
      cancelled = true
    }
  }, [online, user, qc])

  // abonnement push de ce navigateur → compte courant ; cache vidé à la déconnexion
  const prevUser = useRef<string | null>(user?.id ?? null)
  useEffect(() => {
    const id = user?.id ?? null
    if (prevUser.current && prevUser.current !== id) clearUserCache()
    prevUser.current = id
    if (id) void syncPush()
  }, [user?.id])

  // toasts in-app envoyés par le serveur (sujet temps réel personnalisé)
  useEffect(() => {
    if (!user) return
    let unsub: (() => Promise<void>) | null = null
    let cancelled = false
    pb.realtime
      .subscribe(IN_APP_TOPIC, (data: NotificationPayload) => {
        if (!data?.title) return
        toast(data.title, {
          id: data.tag || undefined,
          description: data.body,
          action: data.url && data.url !== window.location.pathname ? { label: 'Voir', onClick: () => navigate(data.url) } : undefined,
        })
        if (data.partyId) void qc.invalidateQueries({ queryKey: qk.party(data.partyId) })
      })
      .then((u) => {
        if (cancelled) void u().catch(() => undefined)
        else unsub = u
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
      void unsub?.().catch(() => undefined)
    }
  }, [user, navigate, qc])

  return <OfflineBanner online={online} pending={pending} />
}
