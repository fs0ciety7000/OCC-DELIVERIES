import { useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Share2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button, Card, CopyButton, QRCodeCard } from '@/components/ui'
import { teamsApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import type { Team } from '@/lib/types'
import { canManage, teamInviteUrl } from './format'
import { teamKeys } from './keys'

/** Lien permanent /e/:code de l'équipe (copie, partage, QR ; nouveau lien pour les admins). */
export function TeamInviteCard({ team }: { team: Team }) {
  const qc = useQueryClient()
  const url = teamInviteUrl(team.code)
  const canShare = typeof navigator !== 'undefined' && 'share' in navigator
  const renew = useMutation({
    mutationFn: () => teamsApi.newCode(team.id),
    onSuccess: () => {
      toast.success("Nouveau lien créé : l'ancien ne fonctionne plus.")
      void qc.invalidateQueries({ queryKey: teamKeys.team(team.id) })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  return (
    <Card className="@container overflow-hidden">
      <div className="grid gap-5 p-4 @md:grid-cols-[minmax(0,1fr)_auto] sm:p-5">
        <div className="min-w-0 space-y-3">
          <h2 className="font-display text-lg font-semibold">Lien de l'équipe</h2>
          <p className="text-sm text-muted">Un lien fixe à épingler dans le canal de l'équipe : on rejoint en un clic, même sans compte (juste un prénom).</p>
          <div className="flex flex-wrap gap-2">
            <CopyButton value={url} label="Copier le lien" toastMessage="Lien de l'équipe copié" />
            {canShare && (
              <Button
                variant="secondary"
                size="sm"
                leftIcon={<Share2 className="size-4" />}
                onClick={() => void navigator.share({ title: 'OCC Deliveries', text: `Rejoins l'équipe « ${team.name} » sur OCC Deliveries`, url }).catch(() => undefined)}
              >
                Partager
              </Button>
            )}
            {canManage(team.myRole) && (
              <Button variant="ghost" size="sm" leftIcon={<RefreshCw className="size-4" />} loading={renew.isPending} onClick={() => renew.mutate()}>
                Nouveau lien
              </Button>
            )}
          </div>
          <p className="truncate text-xs text-subtle">{url}</p>
        </div>
        <QRCodeCard value={url} size={132} label={`QR code pour rejoindre l'équipe ${team.name}`} className="border-0 bg-transparent p-0 shadow-none" caption="Scanne pour rejoindre" />
      </div>
    </Card>
  )
}
