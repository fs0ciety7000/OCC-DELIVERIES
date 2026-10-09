import { centsToEuros, parseEuros } from '@/lib/euros'
import type { OptionGroup } from '@/lib/types'
import { slugify } from './geocode'

export interface ChoiceDraft {
  key: string
  id: string
  name: string
  price: string
}

export interface GroupDraft {
  key: string
  id: string
  name: string
  min: string
  max: string
  choices: ChoiceDraft[]
}

let seq = 0
export const draftKey = () => `d${++seq}`

export function toDrafts(groups: OptionGroup[] | null | undefined): GroupDraft[] {
  return (groups ?? []).map((g) => ({
    key: draftKey(),
    id: g.id,
    name: g.name,
    min: String(g.min),
    max: String(g.max),
    choices: g.choices.map((c) => ({ key: draftKey(), id: c.id, name: c.name, price: c.price ? centsToEuros(c.price) : '' })),
  }))
}

export const emptyChoice = (): ChoiceDraft => ({ key: draftKey(), id: '', name: '', price: '' })
export const emptyGroup = (): GroupDraft => ({ key: draftKey(), id: '', name: '', min: '0', max: '1', choices: [emptyChoice()] })

function uniqueId(base: string, used: Set<string>, fallback: string): string {
  const root = slugify(base) || fallback
  let id = root
  for (let i = 2; used.has(id); i++) id = `${root}-${i}`
  used.add(id)
  return id
}

/**
 * Brouillons → `option_groups` du contrat. Identifiants générés depuis les noms
 * s'ils sont vides (uniques), prix en centimes. Erreurs en français (mêmes règles
 * que `domain.ValidateOptionGroups`).
 */
export function fromDrafts(drafts: GroupDraft[]): { groups: OptionGroup[]; errors: string[] } {
  const errors: string[] = []
  const groupIds = new Set<string>()
  const groups: OptionGroup[] = []
  drafts.forEach((d, gi) => {
    const label = d.name.trim() || `Groupe ${gi + 1}`
    if (!d.name.trim()) errors.push(`${label} : nom requis.`)
    const min = Number(d.min || 0)
    const max = Number(d.max || 0)
    if (!Number.isInteger(min) || min < 0) errors.push(`${label} : minimum invalide.`)
    if (!Number.isInteger(max) || max < 0) errors.push(`${label} : maximum invalide.`)
    if (max > 0 && min > max) errors.push(`${label} : le minimum dépasse le maximum.`)
    const choiceIds = new Set<string>()
    const choices = d.choices
      .filter((c) => c.name.trim() || c.price.trim())
      .map((c, ci) => {
        if (!c.name.trim()) errors.push(`${label} : choix ${ci + 1} sans nom.`)
        const price = c.price.trim() === '' ? 0 : parseEuros(c.price)
        if (price === null) errors.push(`${label} : supplément invalide pour « ${c.name.trim() || ci + 1} ».`)
        return { id: c.id.trim() || uniqueId(c.name, choiceIds, `c${ci + 1}`), name: c.name.trim(), price: price ?? 0 }
      })
    for (const c of choices) choiceIds.add(c.id)
    if (choices.length === 0) errors.push(`${label} : ajoute au moins un choix.`)
    if (min > choices.length) errors.push(`${label} : exige plus de choix qu'il n'en propose.`)
    if (new Set(choices.map((c) => c.id)).size !== choices.length) errors.push(`${label} : identifiants de choix en double.`)
    const id = d.id.trim() || uniqueId(d.name, groupIds, `g${gi + 1}`)
    if (d.id.trim()) {
      if (groupIds.has(id)) errors.push(`${label} : identifiant de groupe en double.`)
      groupIds.add(id)
    }
    groups.push({ id, name: d.name.trim(), min, max, choices })
  })
  return { groups, errors }
}
