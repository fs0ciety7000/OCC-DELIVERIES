import { ArrowLeft, Clock, CornerDownLeft, History, Plus, Search, ShieldCheck, Sparkles, Store, Users, UtensilsCrossed, X, type LucideIcon } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link, useNavigate } from 'react-router'
import { Avatar, Badge, Button, Chip, Money, Skeleton, Spinner, type BadgeVariant } from '@/components/ui'
import { cuisineLabel } from '@/features/restaurants/visual'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { formatDistance, formatRelativeTime, plural } from '@/lib/format'
import { useDebounced } from '@/lib/hooks'
import type { PartyStatus, SearchAction, SearchActionId, SearchDishHit, SearchPerson, SearchRestaurantHit } from '@/lib/types'
import { SUGGESTIONS, shortcutLabel } from './constants'
import { searchTerms } from './fold'
import { Kbd } from './Kbd'
import { Highlight } from './Highlight'
import { clearRecent, pushRecent, readRecent } from './recent'
import { closeSearch, useSearchPalette } from './store'
import { SEARCH_DEBOUNCE_MS, useSearch } from './useSearch'

const ACTION_ICONS: Record<SearchActionId, LucideIcon> = {
  'new-party': Plus,
  restaurants: UtensilsCrossed,
  'my-orders': History,
  admin: ShieldCheck,
}

const ACTION_HINTS: Record<SearchActionId, string> = {
  'new-party': 'Ouvre un salon et invite tes collègues',
  restaurants: 'Tous les restos autour de toi',
  'my-orders': 'Historique et commandes en cours',
  admin: 'Panneau d’administration',
}

const STATUS: Record<PartyStatus, { label: string; variant: BadgeVariant }> = {
  lobby: { label: 'Salon', variant: 'info' },
  voting: { label: 'Vote', variant: 'info' },
  ordering: { label: 'Commande', variant: 'brand' },
  review: { label: 'Récap', variant: 'warning' },
  paying: { label: 'Paiement', variant: 'warning' },
  closed: { label: 'Terminée', variant: 'success' },
  cancelled: { label: 'Annulée', variant: 'danger' },
}

type Option =
  | { kind: 'recent'; key: string; label: string }
  | { kind: 'restaurant'; key: string; hit: SearchRestaurantHit }
  | { kind: 'dish'; key: string; hit: SearchDishHit }
  | { kind: 'person'; key: string; hit: SearchPerson }
  | { kind: 'action'; key: string; hit: SearchAction }

interface Group {
  id: string
  label: string
  icon: LucideIcon
  options: Option[]
}

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])'

export interface CommandPaletteProps {
  /** « Lancer une commande » : ouvre la feuille de création (ou la connexion). */
  onLaunch: () => void
}

/** Palette de recherche globale (Ctrl K / ⌘K / « / »). Plein écran en mobile, dialogue centré dès 768 px. */
export function CommandPalette({ onLaunch }: CommandPaletteProps) {
  const { open, initial } = useSearchPalette()
  if (!open || typeof document === 'undefined') return null
  return createPortal(<PaletteDialog initial={initial} onLaunch={onLaunch} />, document.body)
}

function PaletteDialog({ initial, onLaunch }: { initial: string; onLaunch: () => void }) {
  const navigate = useNavigate()
  const reduce = useReducedMotion()
  const uid = useId()
  const listboxId = `${uid}-listbox`
  const inputRef = useRef<HTMLInputElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const [input, setInput] = useState(initial)
  const [recent, setRecent] = useState<string[]>(readRecent)
  const [person, setPerson] = useState<SearchPerson | null>(null)
  const [cursor, setCursor] = useState({ key: '', index: 0 })

  const debounced = useDebounced(input.trim(), SEARCH_DEBOUNCE_MS)
  const searching = debounced.length > 0
  const search = useSearch(debounced)
  const data = search.data
  // les termes du serveur (corrections comprises) ; repli sur la saisie tant qu'il n'a pas répondu
  const terms = data && data.query === debounced && data.terms.length ? data.terms : searchTerms(debounced)

  // focus : champ à l'ouverture, élément d'origine à la fermeture ; défilement de la page bloqué
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    inputRef.current?.focus()
    return () => {
      document.body.style.overflow = overflow
      if (previous && document.contains(previous)) previous.focus({ preventScroll: true })
    }
  }, [])

  const groups = useMemo<Group[]>(() => {
    const out: Group[] = []
    if (!searching) {
      if (recent.length) out.push({ id: 'recent', label: 'Recherches récentes', icon: Clock, options: recent.map((r) => ({ kind: 'recent', key: `recent-${r}`, label: r })) })
      if (data?.actions.length) out.push({ id: 'actions', label: 'Raccourcis', icon: Sparkles, options: data.actions.map((a) => ({ kind: 'action', key: `action-${a.id}`, hit: a })) })
      return out
    }
    if (!data) return out
    if (data.restaurants.length) out.push({ id: 'restaurants', label: 'Restaurants', icon: Store, options: data.restaurants.map((h) => ({ kind: 'restaurant', key: `r-${h.id}`, hit: h })) })
    if (data.dishes.length) out.push({ id: 'dishes', label: 'Plats', icon: UtensilsCrossed, options: data.dishes.map((h) => ({ kind: 'dish', key: `d-${h.id}`, hit: h })) })
    if (data.people.length) out.push({ id: 'people', label: 'Collègues', icon: Users, options: data.people.map((h) => ({ kind: 'person', key: `p-${h.id}`, hit: h })) })
    if (data.actions.length) out.push({ id: 'actions', label: 'Raccourcis', icon: Sparkles, options: data.actions.map((a) => ({ kind: 'action', key: `action-${a.id}`, hit: a })) })
    return out
  }, [searching, recent, data])

  const options = useMemo(() => groups.flatMap((g) => g.options), [groups])
  // index de la première option de chaque groupe dans la liste à plat (navigation clavier)
  const starts = useMemo(() => {
    const out: number[] = []
    let n = 0
    for (const g of groups) {
      out.push(n)
      n += g.options.length
    }
    return out
  }, [groups])
  // l'option active revient en tête dès que la liste change (nouvelle recherche, nouveaux résultats)
  const listKey = `${searching ? debounced : ''}|${options.map((o) => o.key).join(',')}`
  const active = options.length === 0 ? -1 : cursor.key === listKey ? Math.min(cursor.index, options.length - 1) : 0
  const optionId = (i: number) => `${uid}-opt-${i}`
  const activeId = active >= 0 ? optionId(active) : undefined
  const setActive = (index: number) => setCursor({ key: listKey, index })

  useEffect(() => {
    if (!activeId) return
    document.getElementById(activeId)?.scrollIntoView?.({ block: 'nearest' })
  }, [activeId])

  const resultCount = searching && data ? data.restaurants.length + data.dishes.length + data.people.length : 0
  const noResults = searching && !!data && data.query === debounced && resultCount === 0 && !search.isFetching

  const remember = () => setRecent(pushRecent(input))

  const go = (to: string) => {
    closeSearch()
    navigate(to)
  }

  const select = (opt: Option) => {
    switch (opt.kind) {
      case 'recent':
        setInput(opt.label)
        inputRef.current?.focus()
        return
      case 'restaurant':
        remember()
        go(`/restaurants/${opt.hit.id}`)
        return
      case 'dish':
        remember()
        go(`/restaurants/${opt.hit.restaurant.id}?plat=${encodeURIComponent(opt.hit.id)}`)
        return
      case 'person':
        remember()
        setPerson(opt.hit)
        return
      case 'action':
        if (opt.hit.id === 'new-party') {
          closeSearch()
          onLaunch()
          return
        }
        go(opt.hit.href)
    }
  }

  const onInputKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      if (!options.length) return
      const delta = e.key === 'ArrowDown' ? 1 : -1
      setActive((Math.max(active, 0) + delta + options.length) % options.length)
    } else if (e.key === 'Enter') {
      if (active >= 0 && options[active]) {
        e.preventDefault()
        select(options[active])
      }
    }
  }

  // Échap (fiche collègue → résultats → fermeture) et piège à focus
  const onDialogKeyDown = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      if (person) {
        setPerson(null)
        requestAnimationFrame(() => inputRef.current?.focus())
      } else closeSearch()
      return
    }
    if (e.key !== 'Tab' || !panelRef.current) return
    const items = Array.from(panelRef.current.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => el.offsetParent !== null || el === document.activeElement)
    if (!items.length) return
    const first = items[0]
    const last = items[items.length - 1]
    if (!first || !last) return
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }

  const pickSuggestion = (s: string) => {
    setInput(s)
    inputRef.current?.focus()
  }

  return (
    <div className="fixed inset-0 z-[70]" onKeyDown={onDialogKeyDown}>
      <motion.div
        aria-hidden
        className="absolute inset-0 bg-bg/70 backdrop-blur-sm"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: reduce ? 0 : 0.2 }}
        onClick={closeSearch}
      />
      <motion.div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label="Recherche"
        initial={reduce ? { opacity: 0 } : { opacity: 0, y: 8 }}
        animate={reduce ? { opacity: 1 } : { opacity: 1, y: 0 }}
        transition={{ duration: 0.2, ease: [0.2, 0.8, 0.2, 1] }}
        className="pt-safe absolute inset-0 flex flex-col bg-elevated md:inset-x-0 md:top-[12vh] md:bottom-auto md:mx-auto md:max-h-[72vh] md:w-[min(640px,calc(100vw-4rem))] md:overflow-hidden md:rounded-xl md:border md:border-border md:shadow-card"
      >
        {/* Champ */}
        <div className="flex items-center gap-1 border-b border-border px-2 md:px-3">
          <button type="button" onClick={closeSearch} className="grid size-11 shrink-0 place-items-center rounded-full text-muted hover:text-fg md:hidden" aria-label="Fermer la recherche">
            <ArrowLeft className="size-5" aria-hidden />
          </button>
          <Search className="hidden size-5 shrink-0 text-subtle md:block" aria-hidden />
          <input
            ref={inputRef}
            type="text"
            role="combobox"
            aria-expanded={options.length > 0 && !person}
            aria-controls={listboxId}
            aria-activedescendant={person ? undefined : activeId}
            aria-autocomplete="list"
            aria-label="Rechercher un resto, un plat, un·e collègue"
            placeholder="Resto, plat, collègue…"
            value={input}
            onChange={(e) => {
              setInput(e.target.value)
              setPerson(null)
            }}
            onKeyDown={onInputKeyDown}
            autoComplete="off"
            autoCorrect="off"
            spellCheck={false}
            enterKeyHint="search"
            className="h-14 min-w-0 flex-1 bg-transparent px-2 text-lg text-fg placeholder:text-subtle focus:outline-none md:text-base"
          />
          {search.isFetching && searching && (
            <span aria-hidden className="grid shrink-0 place-items-center">
              <Spinner className="size-4 text-subtle" />
            </span>
          )}
          {input && (
            <button
              type="button"
              onClick={() => {
                setInput('')
                setPerson(null)
                inputRef.current?.focus()
              }}
              className="grid size-11 shrink-0 place-items-center rounded-full text-subtle hover:text-fg"
              aria-label="Effacer la recherche"
            >
              <X className="size-4" aria-hidden />
            </button>
          )}
          <button type="button" onClick={closeSearch} className="hidden h-7 shrink-0 items-center rounded-sm border border-border px-2 text-xs font-semibold text-subtle hover:text-fg md:inline-flex" aria-label="Fermer la recherche">
            Échap
          </button>
        </div>

        <p className="sr-only" role="status" aria-live="polite">
          {person ? '' : searching ? (search.isPending ? 'Recherche…' : noResults ? 'Aucun résultat' : data ? plural(resultCount, 'résultat') : '') : ''}
        </p>

        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 py-2 md:px-3">
          {person ? (
            <PersonCard person={person} terms={terms} onBack={() => setPerson(null)} onNavigate={closeSearch} />
          ) : (
            <>
              {!searching && (
                <div className="space-y-2 px-1 pt-1 pb-3">
                  <div className="flex items-center justify-between gap-2">
                    <h2 className="text-xs font-semibold tracking-wide text-subtle uppercase">Envie de…</h2>
                    {recent.length > 0 && (
                      <button type="button" onClick={() => setRecent(clearRecent())} className="min-h-9 rounded-md px-2 text-xs font-semibold text-muted hover:text-fg">
                        Effacer l’historique
                      </button>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-2" role="group" aria-label="Suggestions">
                    {SUGGESTIONS.map((s) => (
                      <Chip key={s} onClick={() => pickSuggestion(s)}>
                        {s}
                      </Chip>
                    ))}
                  </div>
                </div>
              )}

              {searching && data?.fuzzy && data.query === debounced && resultCount > 0 && (
                <p className="flex items-center gap-1.5 px-2 pb-2 text-xs text-subtle">
                  <Sparkles className="size-3.5 text-brand" aria-hidden />
                  Résultats approchants pour « {debounced} »
                </p>
              )}

              {searching && search.isError && !data ? (
                <div role="alert" className="flex flex-col items-center gap-3 px-4 py-10 text-center">
                  <p className="font-semibold">La recherche n’a pas abouti</p>
                  <p className="text-sm text-muted">{errorMessage(search.error)}</p>
                  <Button variant="secondary" size="sm" onClick={() => search.refetch()}>
                    Réessayer
                  </Button>
                </div>
              ) : searching && search.isPending ? (
                <div aria-hidden className="space-y-2 p-1">
                  {[0, 1, 2, 3].map((i) => (
                    <div key={i} className="flex items-center gap-3 px-2 py-2">
                      <Skeleton className="size-10 rounded-md" />
                      <div className="flex-1 space-y-1.5">
                        <Skeleton className="h-4 w-1/2" />
                        <Skeleton className="h-3 w-1/3" />
                      </div>
                    </div>
                  ))}
                </div>
              ) : noResults ? (
                <div className="flex flex-col items-center gap-3 px-4 pt-8 pb-4 text-center">
                  <span aria-hidden className="text-4xl">
                    🔍
                  </span>
                  <p className="font-semibold">Aucun résultat pour « {debounced} »</p>
                  <p className="text-sm text-muted">Essaie un plat, une cuisine ou le nom d’un resto.</p>
                  <div className="flex flex-wrap justify-center gap-2" role="group" aria-label="Suggestions">
                    {SUGGESTIONS.slice(0, 6).map((s) => (
                      <Chip key={s} onClick={() => pickSuggestion(s)}>
                        {s}
                      </Chip>
                    ))}
                  </div>
                </div>
              ) : null}

              <div role="listbox" id={listboxId} aria-label="Résultats de la recherche" className={cn(options.length === 0 && 'hidden')}>
                {groups.map((g, gi) => (
                  <div key={g.id} role="group" aria-labelledby={`${uid}-g-${g.id}`} className="pb-2">
                    <div id={`${uid}-g-${g.id}`} role="presentation" className="flex items-center gap-1.5 px-2 pt-2 pb-1 text-xs font-semibold tracking-wide text-subtle uppercase">
                      <g.icon className="size-3.5" aria-hidden />
                      {g.label}
                    </div>
                    {g.options.map((opt, oi) => {
                      const i = (starts[gi] ?? 0) + oi
                      return (
                        <div
                          key={opt.key}
                          id={optionId(i)}
                          role="option"
                          aria-selected={i === active}
                          onMouseMove={() => i !== active && setActive(i)}
                          onMouseDown={(e) => e.preventDefault()}
                          onClick={() => select(opt)}
                          className={cn(
                            'group flex min-h-14 cursor-pointer items-center gap-3 rounded-md px-2 py-2 transition-colors duration-[120ms] select-none',
                            i === active ? 'bg-fg/[0.07]' : 'hover:bg-fg/[0.04]',
                          )}
                        >
                          <OptionContent opt={opt} terms={terms} />
                          <CornerDownLeft className={cn('hidden size-4 shrink-0 text-subtle', i === active && 'md:block')} aria-hidden />
                        </div>
                      )
                    })}
                  </div>
                ))}
              </div>
            </>
          )}
        </div>

        {/* Aide clavier (desktop) */}
        <div className="hidden items-center gap-4 border-t border-border px-4 py-2.5 text-xs text-subtle md:flex" aria-hidden>
          <span className="flex items-center gap-1.5">
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd> naviguer
          </span>
          <span className="flex items-center gap-1.5">
            <Kbd>↵</Kbd> ouvrir
          </span>
          <span className="flex items-center gap-1.5">
            <Kbd>Échap</Kbd> fermer
          </span>
          <span className="ml-auto flex items-center gap-1.5">
            <Kbd>/</Kbd> ou <Kbd>{shortcutLabel()}</Kbd> partout
          </span>
        </div>
      </motion.div>
    </div>
  )
}

function Tile({ children, className }: { children: ReactNode; className?: string }) {
  return <span aria-hidden className={cn('grid size-10 shrink-0 place-items-center rounded-md bg-surface text-xl', className)}>{children}</span>
}

function OptionContent({ opt, terms }: { opt: Option; terms: string[] }) {
  switch (opt.kind) {
    case 'recent':
      return (
        <>
          <Tile className="text-subtle">
            <Clock className="size-4" />
          </Tile>
          <span className="min-w-0 flex-1 truncate font-medium">{opt.label}</span>
        </>
      )
    case 'restaurant': {
      const h = opt.hit
      const meta = [h.cuisines.slice(0, 2).map(cuisineLabel).join(' · '), h.distanceKm !== undefined ? formatDistance(h.distanceKm) : '', plural(h.itemsCount, 'plat')].filter(Boolean)
      return (
        <>
          <Tile>{h.emoji || '🍽️'}</Tile>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-semibold">
              <Highlight text={h.name} terms={terms} />
            </span>
            <span className="block truncate text-xs text-muted">{meta.join(' · ')}</span>
          </span>
        </>
      )
    }
    case 'dish': {
      const h = opt.hit
      return (
        <>
          <Tile>{h.emoji || h.restaurant.emoji || '🍽️'}</Tile>
          <span className="min-w-0 flex-1">
            <span className="flex items-baseline gap-2">
              <span className="truncate font-semibold">
                <Highlight text={h.name} terms={terms} />
              </span>
              <Money cents={h.price} className="ml-auto shrink-0 text-sm font-semibold" />
            </span>
            <span className="block truncate text-xs text-muted">
              Chez {h.restaurant.name}
              {h.snippet && (
                <>
                  {' · '}
                  <span className="text-subtle">
                    <Highlight text={h.snippet} terms={terms} />
                  </span>
                </>
              )}
            </span>
          </span>
        </>
      )
    }
    case 'person': {
      const h = opt.hit
      return (
        <>
          <Avatar user={h} size={40} decorative />
          <span className="min-w-0 flex-1">
            <span className="block truncate font-semibold">
              <Highlight text={h.name} terms={terms} />
            </span>
            <span className="block truncate text-xs text-muted">{h.sharedParties > 0 ? `${plural(h.sharedParties, 'commande')} en commun` : 'Dans ton équipe'}</span>
          </span>
        </>
      )
    }
    case 'action': {
      const Icon = ACTION_ICONS[opt.hit.id] ?? Sparkles
      return (
        <>
          <Tile className="bg-brand/12 text-brand">
            <Icon className="size-5" />
          </Tile>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-semibold">{opt.hit.label}</span>
            <span className="block truncate text-xs text-muted">{ACTION_HINTS[opt.hit.id] ?? ''}</span>
          </span>
        </>
      )
    }
  }
}

/** Petite fiche d'un·e collègue : commandes partagées (liens vers les salons). */
function PersonCard({ person, terms, onBack, onNavigate }: { person: SearchPerson; terms: string[]; onBack: () => void; onNavigate: () => void }) {
  const backRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    backRef.current?.focus()
  }, [])
  return (
    <section aria-labelledby="search-person-name" className="space-y-4 px-1 py-2">
      <button ref={backRef} type="button" onClick={onBack} className="inline-flex min-h-11 items-center gap-1.5 rounded-md px-2 text-sm font-semibold text-muted hover:text-fg">
        <ArrowLeft className="size-4" aria-hidden /> Retour aux résultats
      </button>
      <div className="flex items-center gap-4 rounded-lg border border-border bg-surface p-4 shadow-card">
        <Avatar user={person} size={56} decorative />
        <div className="min-w-0">
          <h2 id="search-person-name" className="truncate font-display text-xl font-semibold">
            <Highlight text={person.name} terms={terms} />
          </h2>
          <p className="text-sm text-muted">{person.sharedParties > 0 ? `${plural(person.sharedParties, 'commande')} en commun` : 'Vous êtes dans la même équipe'}</p>
        </div>
      </div>
      {person.recentParties.length > 0 && (
        <div className="space-y-1">
          <h3 className="px-2 text-xs font-semibold tracking-wide text-subtle uppercase">Dernières commandes ensemble</h3>
          <ul>
            {person.recentParties.map((p) => (
              <li key={p.id}>
                <Link to={`/party/${p.id}`} onClick={onNavigate} className="flex min-h-12 items-center gap-3 rounded-md px-2 py-2 hover:bg-fg/[0.04]">
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium">{p.title || 'Commande groupée'}</span>
                    <span className="block text-xs text-subtle">{formatRelativeTime(p.created)}</span>
                  </span>
                  <Badge variant={STATUS[p.status]?.variant ?? 'neutral'}>{STATUS[p.status]?.label ?? p.status}</Badge>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}
