import type { Transition, Variants } from 'motion/react'

export const ease = [0.2, 0.8, 0.2, 1] as const
export const spring: Transition = { type: 'spring', stiffness: 380, damping: 30 }

export const listVariants: Variants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.03 } },
}

export const itemVariants: Variants = {
  hidden: { opacity: 0, y: 8 },
  show: { opacity: 1, y: 0, transition: { duration: 0.2, ease } },
}

export const fadeUp = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
  transition: { duration: 0.2, ease },
}
