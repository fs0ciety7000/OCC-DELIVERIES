import { useRef } from 'react'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'
import { useBell } from '@/lib/pulse'
import { ServiceBell } from './illustrations'

/** Cloche de service « ding » posée sur un avatar quand le membre devient prêt (déclenchée par le realtime). */
export function ReadyBell({ userId }: { userId: string }) {
  const count = useBell(userId)
  const scope = useRef<HTMLSpanElement>(null)
  useGSAP(
    () => {
      if (count === 0) return
      return withMotion(() => {
        const tl = gsap.timeline()
        tl.fromTo('[data-bell]', { opacity: 0, y: 6, scale: 0.6 }, { opacity: 1, y: 0, scale: 1, duration: 0.2, ease: 'back.out(3)' })
          .to('[data-bell]', { rotate: 18, duration: 0.08, yoyo: true, repeat: 5, ease: 'sine.inOut', transformOrigin: '50% 20%' })
          .fromTo('[data-wave]', { scale: 0.4, opacity: 0.8 }, { scale: 2.2, opacity: 0, duration: 0.6, ease: 'power2.out' }, '<')
          .to('[data-bell]', { opacity: 0, y: -4, duration: 0.25, delay: 0.5 })
      })
    },
    { scope, dependencies: [count], revertOnUpdate: true },
  )
  if (count === 0) return null
  return (
    <span ref={scope} aria-hidden className="pointer-events-none absolute -top-3 -right-2 z-10 size-5">
      <span data-wave className="absolute inset-0 rounded-full border-2 border-success opacity-0" />
      <ServiceBell data-bell className="size-5 opacity-0" />
    </span>
  )
}

export default ReadyBell
