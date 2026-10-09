import { EyeOff, ListFilter } from 'lucide-react'
import { useId, useState, type FormEvent } from 'react'
import { Button, Card, CardBody, CardTitle, Input } from '@/components/ui'
import { plural } from '@/lib/format'
import type { AdminSettings, Restaurant } from '@/lib/types'
import { countIncomplete, DEFAULT_MIN_MENU_ITEMS, MAX_MIN_MENU_ITEMS, parseThreshold } from './incomplete'
import { Toggle } from './Toggle'

export interface IncompleteMenusCardProps {
  settings: AdminSettings
  /** Liste admin (tous les restaurants) : aperçu du nombre masqué pour le seuil saisi. */
  restaurants?: readonly Restaurant[]
  saving?: boolean
  /** Enregistre le seuil (0 = tout afficher). */
  onSave: (minMenuItems: number) => void
  /** Affiche la liste des restaurants masqués (filtre « Incomplètes »). */
  onShowHidden?: () => void
}

/**
 * Carte « Cartes incomplètes » : masque des listes publiques (accueil, page
 * Restos, choix des candidats) les restaurants de moins de N plats disponibles.
 * Réversible, recalculé automatiquement quand un menu se complète.
 */
export function IncompleteMenusCard({ settings, restaurants, saving, onSave, onShowHidden }: IncompleteMenusCardProps) {
  const enabled = settings.minMenuItems > 0
  const [draft, setDraft] = useState(String(enabled ? settings.minMenuItems : DEFAULT_MIN_MENU_ITEMS))
  const inputId = useId()
  const errId = `${inputId}-err`
  const parsed = parseThreshold(draft)
  const error = 'error' in parsed ? parsed.error : null
  const threshold = 'value' in parsed ? parsed.value : null
  const dirty = enabled && threshold !== null && threshold !== settings.minMenuItems

  // aperçu du nombre masqué pour le seuil saisi (le serveur fait foi une fois appliqué)
  const preview = threshold !== null && restaurants ? countIncomplete(restaurants, threshold) : null

  const submit = (ev: FormEvent) => {
    ev.preventDefault()
    if (threshold !== null && enabled) onSave(threshold)
  }

  return (
    <Card className="mb-4">
      <CardBody className="space-y-3">
        <div className="flex items-start gap-3">
          <span aria-hidden className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-elevated text-muted">
            <EyeOff className="size-5" />
          </span>
          <div className="min-w-0 flex-1">
            <CardTitle>Cartes incomplètes</CardTitle>
            <p className="text-sm text-muted">
              Les restaurants masqués n'apparaissent plus à l'accueil, dans « Restos » ni dans le choix des candidats. Ils restent accessibles par lien
              et dans les commandes en cours ; ils réapparaissent seuls dès que leur menu est complété.
            </p>
          </div>
        </div>

        <form onSubmit={submit} className="flex flex-wrap items-center gap-x-3 gap-y-2" noValidate>
          <Toggle
            checked={enabled}
            disabled={saving || (!enabled && threshold === null)}
            onChange={(on) => {
              if (!on) onSave(0)
              else if (threshold !== null) onSave(threshold)
            }}
            label="Masquer les restaurants incomplets"
          />
          <label htmlFor={inputId} className="text-sm font-medium">
            Masquer les restaurants de moins de
          </label>
          <Input
            id={inputId}
            type="number"
            inputMode="numeric"
            min={1}
            max={MAX_MIN_MENU_ITEMS}
            step={1}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errId : undefined}
            className="w-20 text-center tabular-nums"
          />
          <span className="text-sm font-medium">plats</span>
          {dirty && (
            <Button type="submit" size="sm" loading={saving}>
              Appliquer
            </Button>
          )}
        </form>
        {error && (
          <p id={errId} role="alert" className="text-xs font-medium text-danger">
            {error}
          </p>
        )}

        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-3">
          <p className="text-sm" aria-live="polite">
            {enabled && !dirty ? (
              <>
                <strong className="font-semibold tabular-nums">{plural(settings.hiddenRestaurants, 'restaurant masqué', 'restaurants masqués')}</strong>
                <span className="text-muted"> sur {settings.activeRestaurants} actifs</span>
              </>
            ) : (
              <span className="text-muted">
                {!enabled && 'Filtre désactivé : tous les restaurants actifs sont listés. '}
                {preview !== null &&
                  (preview === 0
                    ? 'Aucun restaurant ne serait masqué avec ce seuil.'
                    : `${plural(preview, 'restaurant serait masqué', 'restaurants seraient masqués')} avec ce seuil.`)}
              </span>
            )}
          </p>
          {enabled && settings.hiddenRestaurants > 0 && onShowHidden && (
            <Button type="button" variant="ghost" size="sm" onClick={onShowHidden} leftIcon={<ListFilter className="size-4" />}>
              Voir les restaurants masqués
            </Button>
          )}
        </div>
        <p className="text-xs text-subtle">
          Pour compléter un menu : ouvre le restaurant sur Uber Eats ou Takeaway et utilise le favori{' '}
          <a href="/outils/export-menu.html" className="font-semibold text-brand hover:underline">
            « Exporter vers OCC »
          </a>
          , ou modifie son menu ici.
        </p>
      </CardBody>
    </Card>
  )
}
