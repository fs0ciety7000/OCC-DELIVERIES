import { PartyPopper } from 'lucide-react'
import { motion } from 'motion/react'
import { Suspense, useState } from 'react'
import { Link } from 'react-router'
import { Avatar, Badge, buttonClass, Card, CardBody, Money, Skeleton } from '@/components/ui'
import { FoodRain, PaymentCoin } from '@/components/food'
import { formatRelativeTime } from '@/lib/format'
import type { PartyCtx } from '../context'
import { usePayments, useSummary } from '../hooks'
import { METHOD_BADGE, METHOD_LABELS } from '../labels'
import { StatusBadge } from './PaymentMethods'
import { ConsolidatedCard } from './SummaryViews'

function useOnce(key: string): boolean {
  const [first] = useState(() => {
    try {
      if (sessionStorage.getItem(key)) return false
      sessionStorage.setItem(key, '1')
      return true
    } catch {
      return true
    }
  })
  return first
}

export function ClosedStep({ ctx }: { ctx: PartyCtx }) {
  const { party, me } = ctx
  const summary = useSummary(party.id)
  const payments = usePayments(party.id)
  const celebrate = useOnce(`occ-rain-${party.id}`)
  const [raining, setRaining] = useState(celebrate)
  const mine = summary.data?.participants.find((p) => p.user.id === me.id)
  // Clôture manuelle possible avec des parts non confirmées : ne pas annoncer « tout est réglé ».
  const pendingCount = (payments.data ?? []).filter((p) => p.debtor !== p.creditor && p.method !== 'self' && p.status !== 'confirmed').length
  const settled = payments.isSuccess && pendingCount === 0

  return (
    <div className="mx-auto max-w-[680px] space-y-5">
      {raining && (
        <Suspense fallback={null}>
          <FoodRain onDone={() => setRaining(false)} />
        </Suspense>
      )}
      <motion.div initial={{ opacity: 0, scale: 0.96 }} animate={{ opacity: 1, scale: 1 }} transition={{ duration: 0.32 }}>
        <Card className="overflow-hidden text-center">
          <div className="bg-ember/10 relative space-y-2 px-6 pt-8 pb-6">
            <Suspense fallback={<div className="h-22" />}>
              <PaymentCoin size={88} />
            </Suspense>
            <h2 className="font-display text-[32px] leading-9 font-bold">{settled ? 'Tout est réglé !' : 'Commande clôturée'}</h2>
            <p className="text-muted">
              {settled || !payments.isSuccess ? 'Bon appétit 🍽️' : `Bon appétit ! ${pendingCount > 1 ? `${pendingCount} remboursements restent` : 'Un remboursement reste'} à régler entre vous.`}{' '}
              {party.closed_at && <span className="text-subtle">· clôturée {formatRelativeTime(party.closed_at)}</span>}
            </p>
          </div>
          {summary.data && (
            <CardBody className="grid grid-cols-2 gap-3 border-t border-border">
              <div>
                <p className="text-xs text-muted">Total commande</p>
                <Money cents={summary.data.grandTotal} className="font-display text-2xl font-bold" />
              </div>
              <div>
                <p className="text-xs text-muted">Ta part</p>
                <Money cents={mine?.total ?? 0} className="font-display text-2xl font-bold text-brand" />
              </div>
            </CardBody>
          )}
        </Card>
      </motion.div>

      {payments.isPending ? (
        <Skeleton className="h-40 rounded-lg" />
      ) : (
        <Card>
          <CardBody className="space-y-2">
            <h3 className="font-display text-lg font-semibold">Remboursements</h3>
            <ul className="divide-y divide-border">
              {(payments.data ?? [])
                .filter((p) => p.debtor !== p.creditor)
                .map((p) => {
                  const u = ctx.people.get(p.debtor) ?? p.expand?.debtor ?? { id: p.debtor, name: '' }
                  return (
                    <li key={p.id} className="flex items-center gap-3 py-2.5">
                      <Avatar user={u} size={32} />
                      <span className="flex-1 truncate">{p.debtor === me.id ? 'Toi' : u.name}</span>
                      {p.method && <Badge variant={METHOD_BADGE[p.method]}>{METHOD_LABELS[p.method]}</Badge>}
                      <StatusBadge status={p.status} />
                      <Money cents={p.amount} className="font-semibold" />
                    </li>
                  )
                })}
            </ul>
          </CardBody>
        </Card>
      )}

      {summary.data && summary.data.consolidated.length > 0 && <ConsolidatedCard summary={summary.data} />}

      <div className="flex justify-center gap-2">
        <Link to="/" className={buttonClass('primary', 'lg')}>
          <PartyPopper className="size-5" /> Nouvelle commande
        </Link>
      </div>
    </div>
  )
}
