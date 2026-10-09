import { Share2 } from 'lucide-react'
import { Button, Card, CopyButton, QRCodeCard } from '@/components/ui'
import type { Party } from '@/lib/types'
import { inviteUrl } from './hooks'
import { shareInvite } from './share'

export function InviteCard({ party }: { party: Party }) {
  const url = inviteUrl(party.code)
  const canShare = typeof navigator !== 'undefined' && 'share' in navigator
  return (
    <Card className="@container overflow-hidden">
      <div className="grid gap-5 p-4 @md:grid-cols-[minmax(0,1fr)_auto] sm:p-5">
        <div className="min-w-0 space-y-3">
          <h2 className="font-display text-lg font-semibold">Invite ton équipe</h2>
          <p className="text-sm text-muted">Partage le lien, fais scanner le QR ou donne le code.</p>
          <div className="flex items-center gap-3">
            <span className="font-display text-[32px] leading-9 font-bold tracking-[0.25em] text-ember" aria-label={`Code ${party.code.split('').join(' ')}`}>
              {party.code}
            </span>
            <CopyButton value={party.code} label="" ariaLabel="Copier le code" size="icon" variant="ghost" toastMessage="Code copié" />
          </div>
          <div className="flex flex-wrap gap-2">
            <CopyButton value={url} label="Copier le lien" toastMessage="Lien d'invitation copié" />
            {canShare && (
              <Button variant="secondary" size="sm" leftIcon={<Share2 className="size-4" />} onClick={() => void shareInvite(party)}>
                Partager
              </Button>
            )}
          </div>
          <p className="truncate text-xs text-subtle">{url}</p>
        </div>
        <QRCodeCard value={url} size={148} label={`QR code pour rejoindre la commande ${party.code}`} className="border-0 bg-transparent p-0 shadow-none" caption="Scanne pour rejoindre" />
      </div>
    </Card>
  )
}
