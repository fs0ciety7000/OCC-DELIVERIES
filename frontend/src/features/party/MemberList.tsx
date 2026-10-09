import { Crown } from 'lucide-react'
import type { ReactNode } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { ReadyBell } from '@/components/food'
import { Avatar, Badge } from '@/components/ui'
import { ease } from '@/lib/motion'
import type { PartyCtx } from './context'

export function MemberList({ ctx, showReady, extra }: { ctx: PartyCtx; showReady?: boolean; extra?: (userId: string) => ReactNode }) {
  return (
    <ul className="divide-y divide-border" aria-live="polite">
      <AnimatePresence initial={false}>
        {ctx.members.map((m) => {
          const u = m.expand?.user ?? ctx.people.get(m.user) ?? { id: m.user, name: '' }
          const isMe = m.user === ctx.me.id
          return (
            <motion.li
              key={m.id}
              layout
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2, ease }}
              className="flex min-h-14 items-center gap-3 py-2"
            >
              <span className="relative">
                <Avatar user={u} size={40} ready={showReady && m.ready} />
                <ReadyBell userId={m.user} />
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium">
                  {u.name || 'Membre'} {isMe && <span className="text-muted">(toi)</span>}
                </p>
                {extra && <div className="text-xs text-muted">{extra(m.user)}</div>}
              </div>
              {m.role === 'host' && (
                <Badge variant="brand">
                  <Crown aria-hidden className="size-3" /> Hôte
                </Badge>
              )}
              {showReady && (m.ready ? <Badge variant="success">Prêt·e</Badge> : <Badge variant="warning">En cours</Badge>)}
            </motion.li>
          )
        })}
      </AnimatePresence>
    </ul>
  )
}
