import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Button, Field, Input, Sheet } from '@/components/ui'
import { teamsApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { teamKeys } from './keys'

/** Création d'un salon d'équipe permanent (comptes uniquement, pas les invité·es). */
export function CreateTeamSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [address, setAddress] = useState('')
  const [time, setTime] = useState('12:00')
  const create = useMutation({
    mutationFn: () => teamsApi.create({ name: name.trim(), address: address.trim(), usual_time: time, usual_days: ['mon', 'tue', 'wed', 'thu', 'fri'], emoji: '🍽️' }),
    onSuccess: (team) => {
      toast.success(`Équipe « ${team.name} » créée — partage son lien !`)
      void qc.invalidateQueries({ queryKey: teamKeys.all })
      onClose()
      navigate(`/equipes/${team.id}`)
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  return (
    <Sheet
      open={open}
      onClose={onClose}
      title="Créer une équipe"
      description="Un salon permanent avec un lien fixe : chaque midi, la commande du jour se lance en un geste."
      footer={
        <Button block size="lg" type="submit" form="create-team" loading={create.isPending} disabled={name.trim().length < 2}>
          Créer l'équipe
        </Button>
      }
    >
      <form
        id="create-team"
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate()
        }}
      >
        <Field label="Nom de l'équipe">{(p) => <Input {...p} value={name} onChange={(e) => setName(e.target.value)} maxLength={80} placeholder="OCC Mons — midi" required data-autofocus />}</Field>
        <Field label="Adresse du bureau" optional hint="Adresse de livraison par défaut.">
          {(p) => <Input {...p} value={address} onChange={(e) => setAddress(e.target.value)} maxLength={300} placeholder="Rue de Nimy 7, 7000 Mons" autoComplete="street-address" />}
        </Field>
        <Field label="Heure habituelle" optional hint="Du lundi au vendredi (modifiable ensuite).">
          {(p) => <Input {...p} type="time" value={time} onChange={(e) => setTime(e.target.value)} />}
        </Field>
      </form>
    </Sheet>
  )
}
