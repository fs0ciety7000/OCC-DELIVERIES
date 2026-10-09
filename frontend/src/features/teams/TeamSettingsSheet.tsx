import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, LocateFixed, X } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button, Chip, Field, Input, Segmented, Sheet } from '@/components/ui'
import { geocode } from '@/features/admin/geocode'
import { useNearby } from '@/features/restaurants/hooks'
import { teamsApi, type TeamSettingsInput } from '@/lib/api'
import { cn } from '@/lib/cn'
import { AVATAR_COLORS } from '@/lib/colors'
import { errorMessage } from '@/lib/errors'
import type { HistoryRestaurant, SplitMode, Team, Weekday } from '@/lib/types'
import { WEEKDAYS } from './format'
import { teamKeys } from './keys'

const EMOJIS = ['🍽️', '🍕', '🍔', '🍣', '🥗', '🌮', '🍜', '🥙', '☕', '🏢', '🚀', '🔥']

/** Réglages d'équipe (propriétaire / admins) : écrits par la collection `teams` (rule + hook serveur). */
export function TeamSettingsSheet({ team, open, onClose }: { team: Team; open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState(team.name)
  const [emoji, setEmoji] = useState(team.emoji || '🍽️')
  const [color, setColor] = useState(team.color)
  const [address, setAddress] = useState(team.address)
  const [geo, setGeo] = useState<{ lat: number; lng: number } | null>(team.lat || team.lng ? { lat: team.lat, lng: team.lng } : null)
  const [time, setTime] = useState(team.usualTime)
  const [days, setDays] = useState<Weekday[]>(team.usualDays)
  const [split, setSplit] = useState<SplitMode>(team.defaultSplit || 'equal')
  const [cands, setCands] = useState<HistoryRestaurant[]>(team.defaultCandidates)
  const [q, setQ] = useState('')
  const nearby = useNearby({ q, radiusKm: 8 })
  const [locating, setLocating] = useState(false)

  const save = useMutation({
    mutationFn: (extra: TeamSettingsInput = {}) =>
      teamsApi.update(team.id, {
        name: name.trim(),
        emoji,
        color,
        address: address.trim(),
        lat: geo?.lat ?? 0,
        lng: geo?.lng ?? 0,
        usual_time: time,
        usual_days: days,
        default_split: split,
        default_candidates: cands.map((c) => c.id),
        ...extra,
      }),
    onSuccess: (_, extra) => {
      toast.success(extra?.archived === true ? 'Équipe archivée' : extra?.archived === false ? 'Équipe réactivée' : 'Réglages enregistrés')
      void qc.invalidateQueries({ queryKey: teamKeys.all })
      onClose()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const locate = async () => {
    setLocating(true)
    try {
      const r = await geocode(address)
      if (r) {
        setGeo({ lat: r.lat, lng: r.lng })
        toast.success('Adresse localisée', { description: r.label })
      } else toast.error('Adresse introuvable sur la carte : vérifie la rue et la ville.')
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setLocating(false)
    }
  }

  const toggleCand = (r: HistoryRestaurant) =>
    setCands((list) => (list.some((c) => c.id === r.id) ? list.filter((c) => c.id !== r.id) : list.length >= 20 ? list : [...list, r]))

  return (
    <Sheet
      open={open}
      onClose={onClose}
      title="Réglages de l'équipe"
      description="Repris à chaque « commande du jour »."
      footer={
        <Button block size="lg" type="submit" form="team-settings" loading={save.isPending} disabled={name.trim().length < 2}>
          Enregistrer
        </Button>
      }
    >
      <form
        id="team-settings"
        className="space-y-5"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate({})
        }}
      >
        <Field label="Nom de l'équipe">{(p) => <Input {...p} value={name} onChange={(e) => setName(e.target.value)} maxLength={80} required />}</Field>
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">Emoji</legend>
          <div className="flex flex-wrap gap-1.5">
            {EMOJIS.map((em) => (
              <Chip key={em} type="button" selected={emoji === em} aria-pressed={emoji === em} aria-label={`Emoji ${em}`} onClick={() => setEmoji(em)} className="min-h-11 min-w-11 text-lg">
                {em}
              </Chip>
            ))}
          </div>
        </fieldset>
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">Couleur</legend>
          <div className="flex flex-wrap gap-2">
            {AVATAR_COLORS.map((c) => (
              <button
                key={c}
                type="button"
                onClick={() => setColor(color === c ? '' : c)}
                aria-label={`Couleur ${c}`}
                aria-pressed={color.toUpperCase() === c}
                className={cn('grid size-11 place-items-center rounded-full ring-2 ring-offset-2 ring-offset-elevated', color.toUpperCase() === c ? 'ring-fg' : 'ring-transparent')}
                style={{ backgroundColor: c }}
              >
                {color.toUpperCase() === c && <Check aria-hidden className="size-5 text-ink" />}
              </button>
            ))}
          </div>
        </fieldset>
        <Field label="Adresse du bureau" optional hint={geo ? `Position : ${geo.lat.toFixed(5)}, ${geo.lng.toFixed(5)}` : 'Reprise comme adresse de livraison.'}>
          {(p) => (
            <div className="flex gap-2">
              <Input {...p} value={address} onChange={(e) => setAddress(e.target.value)} maxLength={300} placeholder="Rue de Nimy 7, 7000 Mons" autoComplete="street-address" />
              <Button type="button" variant="secondary" size="icon" aria-label="Localiser l'adresse sur la carte" loading={locating} disabled={!address.trim()} onClick={() => void locate()}>
                <LocateFixed className="size-4" />
              </Button>
            </div>
          )}
        </Field>
        <Field label="Heure habituelle" optional hint="Heure de Bruxelles.">
          {(p) => <Input {...p} type="time" value={time} onChange={(e) => setTime(e.target.value)} />}
        </Field>
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">Jours habituels</legend>
          <div className="flex flex-wrap gap-1.5">
            {WEEKDAYS.map((d) => {
              const on = days.includes(d.id)
              return (
                <Chip
                  key={d.id}
                  type="button"
                  selected={on}
                  aria-pressed={on}
                  aria-label={d.long}
                  onClick={() => setDays((list) => (on ? list.filter((x) => x !== d.id) : [...list, d.id]))}
                  className="min-h-11"
                >
                  {d.short}
                </Chip>
              )
            })}
          </div>
        </fieldset>
        <Segmented<SplitMode>
          label="Partage des frais par défaut"
          value={split}
          onChange={setSplit}
          className="w-full"
          options={[
            { value: 'equal', label: 'Parts égales' },
            { value: 'proportional', label: 'Au prorata' },
          ]}
        />
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">
            Restos candidats par défaut <span className="font-normal text-subtle">(facultatif)</span>
          </legend>
          {cands.length > 0 && (
            <ul className="flex flex-wrap gap-1.5" aria-label="Candidats choisis">
              {cands.map((c) => (
                <li key={c.id}>
                  <Chip type="button" selected onClick={() => toggleCand(c)} aria-label={`Retirer ${c.name}`} className="min-h-11">
                    {c.emoji} {c.name} <X aria-hidden className="size-3.5" />
                  </Chip>
                </li>
              ))}
            </ul>
          )}
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Chercher un resto…" aria-label="Chercher un resto à ajouter" />
          <div className="flex max-h-40 flex-wrap gap-1.5 overflow-y-auto">
            {(nearby.data ?? [])
              .filter((r) => !cands.some((c) => c.id === r.id))
              .slice(0, 12)
              .map((r) => (
                <Chip key={r.id} type="button" onClick={() => toggleCand({ id: r.id, name: r.name, emoji: r.emoji, cover: r.cover ?? '', cover_url: r.cover_url ?? '', active: r.active })} className="min-h-11">
                  {r.emoji} {r.name}
                </Chip>
              ))}
          </div>
        </fieldset>
        {team.myRole === 'owner' && (
          <div className="border-t border-border pt-4">
            <Button type="button" variant={team.archived ? 'secondary' : 'danger'} size="sm" onClick={() => save.mutate({ archived: !team.archived })}>
              {team.archived ? "Réactiver l'équipe" : "Archiver l'équipe"}
            </Button>
            <p className="mt-1.5 text-xs text-subtle">Une équipe archivée garde son historique mais n'accepte plus de membres ni de commande.</p>
          </div>
        )}
      </form>
    </Sheet>
  )
}
