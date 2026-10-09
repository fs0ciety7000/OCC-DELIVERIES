import { useId } from 'react'
import { useAuth } from '@/lib/auth'
import { useMyTeams } from './hooks'

/**
 * Feuille « Lancer une commande » : option « Pour l'équipe … » quand on fait partie
 * d'équipes (adresse du bureau, candidats et partage par défaut repris par le serveur).
 */
export function TeamPicker({ value, onChange }: { value: string; onChange: (teamId: string) => void }) {
  const { user } = useAuth()
  const teams = useMyTeams(user?.id)
  const id = useId()
  const list = (teams.data ?? []).filter((t) => !t.archived)
  if (list.length === 0) return null
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="text-sm font-medium">
        Pour l'équipe… <span className="font-normal text-subtle">(facultatif)</span>
      </label>
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-11 w-full rounded-sm border border-border bg-elevated px-3 text-fg focus-visible:border-border-strong focus-visible:ring-2 focus-visible:ring-brand focus-visible:outline-none"
      >
        <option value="">Personne en particulier (code d'invitation)</option>
        {list.map((t) => (
          <option key={t.id} value={t.id}>
            {t.emoji ? `${t.emoji} ` : ''}
            {t.name}
          </option>
        ))}
      </select>
      {value && <p className="text-xs text-subtle">Toute l'équipe est prévenue et rejoint en un geste ; adresse et restos par défaut de l'équipe si tu les laisses vides.</p>}
    </div>
  )
}
