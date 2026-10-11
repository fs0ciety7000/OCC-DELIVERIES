import { Clock } from 'lucide-react'

/** Trop de tentatives (429) : message du serveur (« réessaie dans N minutes »), sans marquer le champ comme invalide. */
export function RateLimitNotice({ message }: { message: string }) {
  return (
    <p role="alert" className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
      <Clock aria-hidden className="mt-0.5 size-4 shrink-0" />
      <span>{message}</span>
    </p>
  )
}
