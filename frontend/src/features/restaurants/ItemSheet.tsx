import { Check, Info } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Badge, Button, Field, Money, QuantityStepper, Sheet, Textarea } from '@/components/ui'
import { cn } from '@/lib/cn'
import { defaultSelections, groupHint, groupsOf, linePrice, toggleChoice, validateSelections, type Selections } from '@/lib/price'
import type { MenuItem } from '@/lib/types'
import { itemImageUrl, TAG_LABELS } from './visual'

export interface ItemDraft {
  selections: Selections
  quantity: number
  note: string
}

export interface ItemSheetProps {
  item: MenuItem | null
  open: boolean
  onClose: () => void
  /** Absent = mode consultation (aperçu des prix seulement). */
  onSubmit?: (draft: ItemDraft) => Promise<void> | void
  submitLabel?: string
  initial?: Partial<ItemDraft>
  browseFooter?: ReactNode
}

export function ItemSheet(props: ItemSheetProps) {
  // Remonte le contenu à chaque article pour réinitialiser l'état local proprement.
  return (
    <Sheet open={props.open && !!props.item} onClose={props.onClose} title={props.item?.name ?? ''} hideTitle>
      {props.item && <ItemSheetBody key={props.item.id} {...props} item={props.item} />}
    </Sheet>
  )
}

function ItemSheetBody({ item, onSubmit, submitLabel = 'Ajouter', initial, browseFooter, onClose }: ItemSheetProps & { item: MenuItem }) {
  const [selections, setSelections] = useState<Selections>(() => initial?.selections ?? defaultSelections(item))
  const [quantity, setQuantity] = useState(initial?.quantity ?? 1)
  const [note, setNote] = useState(initial?.note ?? '')
  const [showErrors, setShowErrors] = useState(false)
  const [busy, setBusy] = useState(false)
  const groups = groupsOf(item)
  const errors = validateSelections(item, selections)
  const total = linePrice(item, selections, quantity)
  const img = itemImageUrl(item)
  const unavailable = item.available === false

  const submit = async () => {
    if (!onSubmit) return
    if (errors.length) {
      setShowErrors(true)
      return
    }
    setBusy(true)
    try {
      await onSubmit({ selections, quantity, note: note.trim() })
      onClose()
    } catch {
      /* l'appelant affiche le toast d'erreur */
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex gap-4">
        <div className="grid size-20 shrink-0 place-items-center overflow-hidden rounded-md border border-border bg-surface text-4xl" aria-hidden>
          {img ? <img src={img} alt="" className="size-full object-cover" /> : item.emoji || '🍽️'}
        </div>
        <div className="min-w-0 space-y-1">
          <h3 className="font-display text-xl leading-7 font-semibold">{item.name}</h3>
          <Money cents={item.price} className="font-semibold text-muted" />
          <div className="flex flex-wrap gap-1.5 pt-0.5">
            {item.popular && <Badge variant="brand">Populaire</Badge>}
            {(item.tags ?? []).map((t) => (
              <Badge key={t}>
                {TAG_LABELS[t]?.emoji} {TAG_LABELS[t]?.label ?? t}
              </Badge>
            ))}
          </div>
        </div>
      </div>
      {item.description && <p className="text-sm text-muted">{item.description}</p>}

      {groups.map((g) => {
        const chosen = selections[g.id] ?? []
        const err = showErrors ? errors.find((e) => e.group === g.id) : undefined
        const radio = g.max === 1
        const full = g.max > 0 && chosen.length >= g.max
        return (
          <fieldset key={g.id} className="space-y-2" aria-describedby={err ? `err-${g.id}` : undefined}>
            <legend className="flex w-full items-center justify-between gap-2 pb-1">
              <span className="font-semibold">{g.name}</span>
              <Badge variant={g.min >= 1 ? (err ? 'danger' : 'warning') : 'neutral'}>{groupHint(g)}</Badge>
            </legend>
            <div className="overflow-hidden rounded-md border border-border">
              {g.choices.map((c) => {
                const checked = chosen.includes(c.id)
                const disabled = !checked && full && !radio
                return (
                  <label
                    key={c.id}
                    className={cn(
                      'flex min-h-12 items-center gap-3 border-b border-border px-3.5 last:border-b-0 transition-colors',
                      checked ? 'bg-brand/[0.07]' : 'hover:bg-fg/[0.03]',
                      disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer',
                    )}
                  >
                    <input
                      type={radio ? 'radio' : 'checkbox'}
                      name={`opt-${item.id}-${g.id}`}
                      checked={checked}
                      disabled={disabled}
                      onChange={() => setSelections((s) => toggleChoice(s, g, c.id))}
                      className="peer sr-only"
                    />
                    <span
                      aria-hidden
                      className={cn(
                        'grid size-5 shrink-0 place-items-center border-2 transition-colors peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-brand',
                        radio ? 'rounded-full' : 'rounded-[6px]',
                        checked ? 'border-brand bg-brand text-brand-fg' : 'border-border-strong',
                      )}
                    >
                      {checked && (radio ? <span className="size-2 rounded-full bg-brand-fg" /> : <Check className="size-3.5" strokeWidth={3} />)}
                    </span>
                    <span className="flex-1 text-[15px]">{c.name}</span>
                    {c.price !== 0 && <Money cents={c.price} signed className="text-sm text-muted" />}
                  </label>
                )
              })}
            </div>
            {err && (
              <p id={`err-${g.id}`} role="alert" className="text-xs font-medium text-danger">
                {err.message}
              </p>
            )}
          </fieldset>
        )
      })}

      {onSubmit && (
        <Field label="Une précision ?" optional hint="Ex. sans oignon, sauce à part…">
          {(p) => <Textarea {...p} value={note} maxLength={200} onChange={(e) => setNote(e.target.value)} placeholder="Note pour le resto" className="min-h-18" />}
        </Field>
      )}

      <p className="flex items-center gap-1.5 text-xs text-subtle">
        <Info aria-hidden className="size-3.5" /> Prix indicatifs — le total exact est calculé par le serveur.
      </p>

      <div className="sticky bottom-0 -mx-5 flex items-center gap-3 border-t border-border bg-elevated px-5 pt-3 pb-[max(0.25rem,env(safe-area-inset-bottom))] sm:-mx-6 sm:px-6">
        {onSubmit ? (
          <>
            <QuantityStepper value={quantity} onChange={setQuantity} min={1} max={20} />
            <Button className="flex-1" size="lg" onClick={submit} loading={busy} disabled={unavailable}>
              {unavailable ? 'Indisponible' : (
                <>
                  {submitLabel} · <Money cents={total} />
                </>
              )}
            </Button>
          </>
        ) : (
          browseFooter ?? (
            <div className="flex w-full items-center justify-between">
              <span className="text-sm text-muted">Prix avec options</span>
              <Money cents={total} className="font-display text-xl font-semibold" />
            </div>
          )
        )}
      </div>
    </div>
  )
}
