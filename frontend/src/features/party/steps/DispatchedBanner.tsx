import { Suspense } from 'react'
import { DispatchedScene } from '@/components/food'
import { formatTime } from '@/lib/format'
import { useFirstTime } from '@/lib/once'
import type { Party } from '@/lib/types'
import { dispatchHeadline, etaEstimate } from './sceneText'

/**
 * Carte « Commande envoyée » (récap / remboursements) : la traversée du scooter ne joue
 * qu'une fois par party et par appareil (localStorage), ensuite carte statique.
 * À ne monter que si `party.dispatch` est renseigné.
 */
export function DispatchedBanner({ party, paused }: { party: Party; paused?: boolean }) {
  const play = useFirstTime(`occ-dispatched-${party.id}`)
  const d = party.dispatch
  if (!d) return null
  const at = formatTime(d.at)
  const headline = `${dispatchHeadline(d.method)}${at ? ` à ${at}` : ''}`
  const eta = d.method === 'export' ? null : etaEstimate(party.expand?.restaurant?.eta_min, party.expand?.restaurant?.eta_max)
  return (
    <Suspense fallback={<div className="h-[124px] rounded-lg border border-success/30 bg-success/[0.06]" />}>
      <DispatchedScene headline={headline} eta={eta} play={play} paused={paused} />
    </Suspense>
  )
}
