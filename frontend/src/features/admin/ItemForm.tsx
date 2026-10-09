import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, X } from 'lucide-react'
import { useState, type FormEvent, type KeyboardEvent } from 'react'
import { toast } from 'sonner'
import { Button, Chip, Field, Input, Sheet, Textarea } from '@/components/ui'
import { adminApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { centsToEuros, parseEuros } from '@/lib/euros'
import { qk } from '@/lib/queryKeys'
import type { MenuCategory, MenuItem } from '@/lib/types'
import { emptyChoice, emptyGroup, fromDrafts, toDrafts, type GroupDraft } from './optionDrafts'
import { PRESET_TAGS } from './labels'
import { Toggle } from './Toggle'


const selectClass =
  'min-h-11 w-full rounded-sm border border-border bg-elevated px-3 text-fg hover:border-border-strong focus:border-brand/70 focus:outline-none focus:ring-3 focus:ring-brand/20'

export function ItemForm({
  restaurantId,
  item,
  categories,
  defaultCategory,
  nextPosition,
  open,
  onClose,
}: {
  restaurantId: string
  item: MenuItem | null
  categories: MenuCategory[]
  defaultCategory: string
  nextPosition: number
  open: boolean
  onClose: () => void
}) {
  const [name, setName] = useState(item?.name ?? '')
  const [emoji, setEmoji] = useState(item?.emoji ?? '')
  const [description, setDescription] = useState(item?.description ?? '')
  const [price, setPrice] = useState(item ? centsToEuros(item.price) : '')
  const [category, setCategory] = useState(item?.category || defaultCategory)
  const [tags, setTags] = useState<string[]>(item?.tags ?? [])
  const [tagInput, setTagInput] = useState('')
  const [available, setAvailable] = useState(item?.available ?? true)
  const [popular, setPopular] = useState(item?.popular ?? false)
  const [groups, setGroups] = useState<GroupDraft[]>(() => toDrafts(item?.option_groups))
  const [errors, setErrors] = useState<{ name?: string; price?: string; options?: string[] }>({})
  const qc = useQueryClient()

  const save = useMutation({
    mutationFn: (data: Partial<MenuItem>) => adminApi.saveItem(item?.id ?? null, data),
    onSuccess: () => {
      toast.success(item ? 'Article enregistré' : 'Article ajouté')
      void qc.invalidateQueries({ queryKey: qk.admin.menu(restaurantId) })
      void qc.invalidateQueries({ queryKey: qk.menu(restaurantId) })
      onClose()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const addTag = (raw: string) => {
    const t = raw.trim().toLowerCase().replace(/\s+/g, '_')
    if (t && !tags.includes(t)) setTags([...tags, t])
    setTagInput('')
  }
  const onTagKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      addTag(tagInput)
    }
  }
  const updateGroup = (key: string, patch: Partial<GroupDraft>) => setGroups((gs) => gs.map((g) => (g.key === key ? { ...g, ...patch } : g)))

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const cents = parseEuros(price)
    const opts = fromDrafts(groups)
    const next = {
      name: name.trim() ? undefined : 'Nom requis.',
      price: cents === null ? 'Prix invalide (ex. 12,50).' : undefined,
      options: opts.errors.length ? opts.errors : undefined,
    }
    setErrors(next)
    if (next.name || next.price || next.options) return
    save.mutate({
      restaurant: restaurantId,
      category,
      name: name.trim(),
      emoji: emoji.trim(),
      description: description.trim(),
      price: cents!,
      tags,
      available,
      popular,
      option_groups: opts.groups,
      ...(item ? {} : { position: nextPosition }),
    })
  }

  const custom = tags.filter((t) => !PRESET_TAGS.some((p) => p.id === t))

  return (
    <Sheet
      open={open}
      onClose={onClose}
      size="lg"
      title={item ? `Modifier « ${item.name} »` : 'Nouvel article'}
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Annuler
          </Button>
          <Button type="submit" form="item-form" loading={save.isPending}>
            Enregistrer
          </Button>
        </div>
      }
    >
      <form id="item-form" onSubmit={submit} className="space-y-5" noValidate>
        <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_88px_140px]">
          <Field label="Nom" error={errors.name}>
            {(p) => <Input {...p} data-autofocus value={name} onChange={(e) => setName(e.target.value)} maxLength={160} />}
          </Field>
          <Field label="Emoji">
            {(p) => <Input {...p} value={emoji} onChange={(e) => setEmoji(e.target.value)} maxLength={16} />}
          </Field>
          <Field label="Prix (€)" error={errors.price}>
            {(p) => <Input {...p} value={price} onChange={(e) => setPrice(e.target.value)} inputMode="decimal" placeholder="12,50" />}
          </Field>
        </div>
        <Field label="Description" optional>
          {(p) => <Textarea {...p} value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />}
        </Field>
        <Field label="Catégorie">
          {(p) => (
            <select {...p} className={selectClass} value={category} onChange={(e) => setCategory(e.target.value)}>
              <option value="">Sans catégorie</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          )}
        </Field>

        <div className="space-y-2">
          <p className="text-sm font-medium" id="tags-label">
            Étiquettes
          </p>
          <div className="flex flex-wrap gap-2" role="group" aria-labelledby="tags-label">
            {PRESET_TAGS.map((t) => (
              <Chip key={t.id} selected={tags.includes(t.id)} onClick={() => setTags(tags.includes(t.id) ? tags.filter((x) => x !== t.id) : [...tags, t.id])}>
                {t.label}
              </Chip>
            ))}
            {custom.map((t) => (
              <Chip key={t} selected onClick={() => setTags(tags.filter((x) => x !== t))} aria-label={`Retirer l'étiquette ${t}`}>
                {t} <X className="size-3.5" aria-hidden />
              </Chip>
            ))}
          </div>
          <Input aria-label="Ajouter une étiquette" placeholder="Autre étiquette + Entrée" value={tagInput} onChange={(e) => setTagInput(e.target.value)} onKeyDown={onTagKey} onBlur={() => tagInput && addTag(tagInput)} />
        </div>

        <div className="flex flex-wrap gap-x-6">
          <Toggle checked={available} onChange={setAvailable} label="Disponible" showLabel />
          <Toggle checked={popular} onChange={setPopular} label="Populaire" showLabel />
        </div>

        <section className="space-y-3" aria-labelledby="options-title">
          <div className="flex items-center justify-between">
            <h3 id="options-title" className="font-display text-base font-semibold">
              Options
            </h3>
            <Button variant="secondary" size="sm" leftIcon={<Plus className="size-4" />} onClick={() => setGroups([...groups, emptyGroup()])}>
              Groupe
            </Button>
          </div>
          {groups.length === 0 && <p className="text-sm text-muted">Aucune option (taille, sauce, suppléments…).</p>}
          {errors.options && (
            <ul role="alert" className="list-disc space-y-0.5 pl-5 text-xs font-medium text-danger">
              {errors.options.map((m) => (
                <li key={m}>{m}</li>
              ))}
            </ul>
          )}
          {groups.map((g, gi) => (
            <fieldset key={g.key} className="space-y-3 rounded-md border border-border p-3">
              <legend className="sr-only">Groupe {gi + 1}</legend>
              <div className="grid grid-cols-[minmax(0,1fr)_64px_64px_auto] items-end gap-2">
                <Field label="Groupe">{(p) => <Input {...p} value={g.name} placeholder="Taille" onChange={(e) => updateGroup(g.key, { name: e.target.value })} />}</Field>
                <Field label="Min">{(p) => <Input {...p} value={g.min} inputMode="numeric" onChange={(e) => updateGroup(g.key, { min: e.target.value })} />}</Field>
                <Field label="Max">{(p) => <Input {...p} value={g.max} inputMode="numeric" onChange={(e) => updateGroup(g.key, { max: e.target.value })} />}</Field>
                <Button variant="ghost" size="icon" aria-label={`Supprimer le groupe ${g.name || gi + 1}`} onClick={() => setGroups(groups.filter((x) => x.key !== g.key))}>
                  <Trash2 className="size-4" />
                </Button>
              </div>
              <p className="text-xs text-subtle">Min 1 + Max 1 = choix obligatoire · Max 0 = sans limite.</p>
              <ul className="space-y-2">
                {g.choices.map((c, ci) => (
                  <li key={c.key} className="grid grid-cols-[minmax(0,1fr)_96px_auto] gap-2">
                    <Input
                      aria-label={`Choix ${ci + 1}`}
                      placeholder="Large"
                      value={c.name}
                      onChange={(e) => updateGroup(g.key, { choices: g.choices.map((x) => (x.key === c.key ? { ...x, name: e.target.value } : x)) })}
                    />
                    <Input
                      aria-label={`Supplément en euros du choix ${ci + 1}`}
                      placeholder="+ 0,00"
                      inputMode="decimal"
                      value={c.price}
                      onChange={(e) => updateGroup(g.key, { choices: g.choices.map((x) => (x.key === c.key ? { ...x, price: e.target.value } : x)) })}
                    />
                    <Button variant="ghost" size="icon" aria-label={`Retirer le choix ${c.name || ci + 1}`} onClick={() => updateGroup(g.key, { choices: g.choices.filter((x) => x.key !== c.key) })}>
                      <X className="size-4" />
                    </Button>
                  </li>
                ))}
              </ul>
              <Button variant="ghost" size="sm" leftIcon={<Plus className="size-4" />} onClick={() => updateGroup(g.key, { choices: [...g.choices, emptyChoice()] })}>
                Choix
              </Button>
            </fieldset>
          ))}
        </section>
      </form>
    </Sheet>
  )
}
