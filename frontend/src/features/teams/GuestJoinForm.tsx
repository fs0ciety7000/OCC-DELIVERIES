import { useMutation } from '@tanstack/react-query'
import { Check } from 'lucide-react'
import { useId, useState } from 'react'
import { Button, Field, Input } from '@/components/ui'
import { guestApi } from '@/lib/api'
import { cn } from '@/lib/cn'
import { AVATAR_COLORS } from '@/lib/colors'
import { errorMessage } from '@/lib/errors'
import type { GuestAuthResponse } from '@/lib/types'

export interface GuestJoinFormProps {
  /** Code d'une commande (/j/:code) ou d'une équipe (/e/:code). */
  kind: 'party' | 'team'
  code: string
  onJoined: (res: GuestAuthResponse) => void
}

/**
 * « Continuer en invité·e » : juste un prénom (+ couleur d'avatar facultative).
 * Le serveur crée un compte invité, renvoie un jeton et fait rejoindre la commande / l'équipe.
 */
export function GuestJoinForm({ kind, code, onJoined }: GuestJoinFormProps) {
  const [name, setName] = useState('')
  const [color, setColor] = useState<string>('')
  const [touched, setTouched] = useState(false)
  const legendId = useId()
  const join = useMutation({
    mutationFn: () => guestApi.join({ name: name.trim(), color: color || undefined, ...(kind === 'party' ? { partyCode: code } : { teamCode: code }) }),
    onSuccess: (res) => onJoined(res),
  })
  const nameError = touched && !name.trim() ? 'Indique ton prénom pour que les collègues te reconnaissent.' : undefined

  return (
    <form
      className="space-y-4"
      aria-label="Continuer en invité·e"
      onSubmit={(e) => {
        e.preventDefault()
        setTouched(true)
        if (!name.trim()) return
        join.mutate()
      }}
    >
      <Field label="Ton prénom" error={nameError} hint="Pas d'e-mail, pas de mot de passe : tu pourras créer ton compte plus tard.">
        {(p) => <Input {...p} value={name} onChange={(e) => setName(e.target.value)} maxLength={40} autoComplete="given-name" placeholder="Léa" />}
      </Field>
      <fieldset className="space-y-2" aria-describedby={legendId}>
        <legend className="text-sm font-medium">
          Couleur d'avatar <span className="font-normal text-subtle">(facultatif)</span>
        </legend>
        <p id={legendId} className="sr-only">
          Choisis la couleur de ta pastille dans la liste des participants.
        </p>
        <div className="flex flex-wrap gap-2">
          {AVATAR_COLORS.map((c) => (
            <button
              key={c}
              type="button"
              onClick={() => setColor(color === c ? '' : c)}
              aria-label={`Couleur ${c}`}
              aria-pressed={color === c}
              className={cn('grid size-11 place-items-center rounded-full ring-2 ring-offset-2 ring-offset-surface transition-shadow motion-reduce:transition-none', color === c ? 'ring-fg' : 'ring-transparent')}
              style={{ backgroundColor: c }}
            >
              {color === c && <Check aria-hidden className="size-5 text-ink" />}
            </button>
          ))}
        </div>
      </fieldset>
      {join.isError && (
        <p role="alert" className="text-sm text-danger">
          {errorMessage(join.error)}
        </p>
      )}
      <Button type="submit" block size="lg" loading={join.isPending}>
        {kind === 'party' ? 'Rejoindre la commande' : "Rejoindre l'équipe"}
      </Button>
    </form>
  )
}
