import type { MenuItem, OptionGroup, SelectedOption } from './types'

/** Sélection locale : id du groupe → ids des choix cochés. */
export type Selections = Record<string, string[]>

export function groupsOf(item: Pick<MenuItem, 'option_groups'>): OptionGroup[] {
  return Array.isArray(item.option_groups) ? item.option_groups : []
}

/** Pré-sélection : premier choix des groupes obligatoires à choix unique. */
export function defaultSelections(item: Pick<MenuItem, 'option_groups'>): Selections {
  const out: Selections = {}
  for (const g of groupsOf(item)) {
    const first = g.choices[0]
    out[g.id] = g.min >= 1 && g.max === 1 && first ? [first.id] : []
  }
  return out
}

/** Bascule un choix en respectant `max` (radio si max = 1). */
export function toggleChoice(selections: Selections, group: OptionGroup, choiceId: string): Selections {
  const current = selections[group.id] ?? []
  let next: string[]
  if (current.includes(choiceId)) {
    // Un groupe radio obligatoire ne peut pas être vidé.
    next = group.max === 1 && group.min >= 1 ? current : current.filter((c) => c !== choiceId)
  } else if (group.max === 1) {
    next = [choiceId]
  } else if (group.max > 0 && current.length >= group.max) {
    next = current
  } else {
    next = [...current, choiceId]
  }
  return { ...selections, [group.id]: next }
}

/** Prix unitaire indicatif = prix de base + suppléments choisis. */
export function unitPrice(item: Pick<MenuItem, 'price' | 'option_groups'>, selections: Selections): number {
  let total = item.price
  for (const g of groupsOf(item)) {
    const chosen = selections[g.id] ?? []
    for (const c of g.choices) if (chosen.includes(c.id)) total += c.price
  }
  return total
}

export function linePrice(item: Pick<MenuItem, 'price' | 'option_groups'>, selections: Selections, quantity: number): number {
  return unitPrice(item, selections) * Math.max(0, Math.floor(quantity))
}

export interface SelectionError {
  group: string
  message: string
}

/** Vérifie min/max pour chaque groupe ; messages en français. */
export function validateSelections(item: Pick<MenuItem, 'option_groups'>, selections: Selections): SelectionError[] {
  const errors: SelectionError[] = []
  for (const g of groupsOf(item)) {
    const n = (selections[g.id] ?? []).filter((id) => g.choices.some((c) => c.id === id)).length
    if (n < g.min) {
      errors.push({ group: g.id, message: g.min === 1 ? `Choisis une option « ${g.name} ».` : `Choisis au moins ${g.min} options « ${g.name} ».` })
    } else if (g.max > 0 && n > g.max) {
      errors.push({ group: g.id, message: `Maximum ${g.max} pour « ${g.name} ».` })
    }
  }
  return errors
}

/** Convertit en `selected_options` (contrat) — on omet les groupes vides. */
export function toSelectedOptions(item: Pick<MenuItem, 'option_groups'>, selections: Selections): SelectedOption[] {
  return groupsOf(item)
    .map((g) => ({ group: g.id, choices: (selections[g.id] ?? []).filter((id) => g.choices.some((c) => c.id === id)) }))
    .filter((s) => s.choices.length > 0)
}

export function fromSelectedOptions(options: SelectedOption[] | null | undefined): Selections {
  const out: Selections = {}
  for (const o of options ?? []) out[o.group] = [...o.choices]
  return out
}

/** Libellé lisible « Large, Fromage » (aperçu côté client). */
export function optionsLabel(item: Pick<MenuItem, 'option_groups'>, selections: Selections): string {
  const names: string[] = []
  for (const g of groupsOf(item)) {
    const chosen = selections[g.id] ?? []
    for (const c of g.choices) if (chosen.includes(c.id)) names.push(c.name)
  }
  return names.join(', ')
}

export function groupHint(group: OptionGroup): string {
  if (group.min >= 1 && group.max === 1) return 'Obligatoire'
  if (group.min >= 1) return group.max > 0 && group.max !== group.min ? `${group.min} à ${group.max}` : `${group.min} obligatoires`
  if (group.max === 1) return 'Facultatif'
  return group.max > 0 ? `Jusqu'à ${group.max}` : 'Facultatif'
}
