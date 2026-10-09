import { AnimatePresence, motion } from 'motion/react'
import { X } from 'lucide-react'
import { useEffect, useId, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/cn'
import { useMediaQuery } from '@/lib/hooks'

export interface SheetProps {
  open: boolean
  onClose: () => void
  title: ReactNode
  description?: ReactNode
  children: ReactNode
  footer?: ReactNode
  className?: string
  size?: 'md' | 'lg'
  /** Masque le titre visuellement (reste annoncé). */
  hideTitle?: boolean
}

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

/** Bottom sheet sur mobile, dialogue centré dès 640 px. Piège le focus, Échap ferme. */
export function Sheet({ open, onClose, title, description, children, footer, className, size = 'md', hideTitle }: SheetProps) {
  const desktop = useMediaQuery('(min-width: 640px)')
  const titleId = useId()
  const descId = useId()
  const panelRef = useRef<HTMLDivElement>(null)
  const closeRef = useRef(onClose)
  useEffect(() => {
    closeRef.current = onClose
  }, [onClose])

  useEffect(() => {
    if (!open) return
    const previouslyFocused = document.activeElement as HTMLElement | null
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const raf = requestAnimationFrame(() => {
      const panel = panelRef.current
      if (!panel) return
      const auto = panel.querySelector<HTMLElement>('[data-autofocus]')
      const first = auto ?? panel.querySelector<HTMLElement>(FOCUSABLE)
      ;(first ?? panel).focus()
    })
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        closeRef.current()
        return
      }
      if (e.key !== 'Tab' || !panelRef.current) return
      const nodes = Array.from(panelRef.current.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((n) => n.offsetParent !== null || n === document.activeElement)
      if (nodes.length === 0) return
      const first = nodes[0]!
      const last = nodes[nodes.length - 1]!
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      cancelAnimationFrame(raf)
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prevOverflow
      previouslyFocused?.focus?.()
    }
  }, [open])

  if (typeof document === 'undefined') return null

  return createPortal(
    <AnimatePresence>
      {open && (
        <div className="fixed inset-0 z-50 flex items-end justify-center sm:items-center sm:p-6">
          <motion.div
            className="absolute inset-0 bg-scrim backdrop-blur-[2px]"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            onClick={onClose}
            aria-hidden
          />
          <motion.div
            ref={panelRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
            aria-describedby={description ? descId : undefined}
            tabIndex={-1}
            initial={desktop ? { opacity: 0, scale: 0.97, y: 8 } : { y: '100%' }}
            animate={desktop ? { opacity: 1, scale: 1, y: 0 } : { y: 0 }}
            exit={desktop ? { opacity: 0, scale: 0.98, y: 4 } : { y: '100%' }}
            transition={{ duration: 0.32, ease: [0.2, 0.8, 0.2, 1] }}
            className={cn(
              'relative flex max-h-[92dvh] w-full flex-col overflow-hidden rounded-t-xl border border-border bg-elevated shadow-sheet outline-none sm:rounded-xl',
              size === 'lg' ? 'sm:max-w-2xl' : 'sm:max-w-lg',
              className,
            )}
          >
            <div aria-hidden className="mx-auto mt-2.5 h-1 w-10 shrink-0 rounded-full bg-fg/20 sm:hidden" />
            <header className={cn('flex items-start gap-3 px-5 pt-4 pb-3 sm:px-6 sm:pt-5', hideTitle && 'pb-0')}>
              <div className="min-w-0 flex-1">
                <h2 id={titleId} className={cn('font-display text-xl leading-7 font-semibold', hideTitle && 'sr-only')}>
                  {title}
                </h2>
                {description && (
                  <p id={descId} className="mt-0.5 text-sm text-muted">
                    {description}
                  </p>
                )}
              </div>
              <button
                type="button"
                onClick={onClose}
                className="-mt-1 -mr-2 grid size-11 shrink-0 place-items-center rounded-full text-muted hover:bg-fg/[0.06] hover:text-fg"
                aria-label="Fermer"
              >
                <X className="size-5" />
              </button>
            </header>
            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 pb-5 sm:px-6">{children}</div>
            {footer && <footer className="border-t border-border bg-elevated px-5 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:px-6">{footer}</footer>}
          </motion.div>
        </div>
      )}
    </AnimatePresence>,
    document.body,
  )
}
