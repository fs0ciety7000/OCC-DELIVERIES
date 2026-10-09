import type { Party } from '@/lib/types'
import { inviteUrl } from './hooks'

export async function shareInvite(party: Party) {
  const url = inviteUrl(party.code)
  const text = `Rejoins « ${party.title || 'notre commande'} » sur OCC Deliveries — code ${party.code}`
  if (navigator.share) {
    try {
      await navigator.share({ title: 'OCC Deliveries', text, url })
      return true
    } catch {
      return false
    }
  }
  return false
}
