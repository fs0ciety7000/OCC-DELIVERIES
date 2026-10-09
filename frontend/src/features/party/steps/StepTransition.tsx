import { AnimatePresence, motion, type Variants } from 'motion/react'
import { useRef, useState, type ReactNode } from 'react'
import { ease } from '@/lib/motion'
import type { PartyStatus } from '@/lib/types'
import { stepAnnouncement, stepOrder as order } from './sceneText'

const variants: Variants = {
  enter: (dir: number) => ({ opacity: 0, x: dir * 28 }),
  center: { opacity: 1, x: 0, transition: { duration: 0.26, ease } },
  exit: (dir: number) => ({ opacity: 0, x: dir * -28, transition: { duration: 0.16, ease } }),
}

/**
 * Transition entre étapes : fondu + glissement selon le sens (avancer = vers la gauche,
 * revenir — ex. « Rouvrir la commande » — = vers la droite). Annonce l'étape aux lecteurs
 * d'écran et, si le focus a été perdu (bouton de l'étape précédente démonté), le place sur
 * le nouveau contenu. Mouvement réduit : `MotionConfig` ne garde que l'opacité.
 */
export function StepTransition({ status, children }: { status: PartyStatus; children: ReactNode }) {
  const [shown, setShown] = useState(status)
  const [dir, setDir] = useState(1)
  const [announce, setAnnounce] = useState('')
  // Mise à jour pendant le rendu (motif React « état dérivé de la prop précédente »).
  if (shown !== status) {
    setDir(order(status) >= order(shown) ? 1 : -1)
    setShown(status)
    setAnnounce(stepAnnouncement(status))
  }
  const box = useRef<HTMLDivElement>(null)
  const changed = announce !== ''

  return (
    <>
      <p className="sr-only" aria-live="polite" aria-atomic="true">
        {announce}
      </p>
      <AnimatePresence mode="wait" initial={false} custom={dir}>
        <motion.div
          key={status}
          ref={box}
          tabIndex={-1}
          className="outline-none"
          custom={dir}
          variants={variants}
          initial="enter"
          animate="center"
          exit="exit"
          onAnimationComplete={(def) => {
            if (def !== 'center' || !changed) return
            const active = document.activeElement
            if (!active || active === document.body) box.current?.focus({ preventScroll: true })
          }}
        >
          {children}
        </motion.div>
      </AnimatePresence>
    </>
  )
}
