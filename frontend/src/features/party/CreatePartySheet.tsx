import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Button, Field, Input, Sheet, Textarea } from '@/components/ui'
import { RestaurantCover } from '@/features/restaurants/RestaurantCover'
import { occ, partiesApi } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'
import type { Restaurant } from '@/lib/types'

function defaultTitle(): string {
  const now = new Date()
  const day = new Intl.DateTimeFormat('fr-BE', { weekday: 'long' }).format(now)
  return now.getHours() >= 16 ? `Soirée du ${day}` : `Midi du ${day}`
}

export interface CreatePartySheetProps {
  open: boolean
  onClose: () => void
  /** Restaurant imposé : la party passe directement en commande. */
  restaurant?: Restaurant
}

export function CreatePartySheet({ open, onClose, restaurant }: CreatePartySheetProps) {
  const { user } = useAuth()
  const navigate = useNavigate()
  const [title, setTitle] = useState(defaultTitle)
  const [address, setAddress] = useState('')
  const [notes, setNotes] = useState('')

  const create = useMutation({
    mutationFn: async () => {
      if (!user) throw new Error('Connecte-toi pour lancer une commande.')
      const party = await partiesApi.create({ title: title.trim() || defaultTitle(), delivery_address: address.trim(), notes: notes.trim() }, user.id)
      if (restaurant) {
        try {
          await occ.transition(party.id, { to: 'ordering', restaurant: restaurant.id })
        } catch (err) {
          toast.error(`Commande créée, mais le resto n'a pas pu être fixé : ${errorMessage(err)}`)
        }
      }
      return party
    },
    onSuccess: (party) => {
      toast.success(restaurant ? `C'est parti chez ${restaurant.name} !` : 'Commande lancée — invite tes collègues !')
      onClose()
      navigate(`/party/${party.id}`)
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  return (
    <Sheet
      open={open}
      onClose={onClose}
      title="Lancer une commande"
      description={restaurant ? 'Tout le monde commandera dans ce resto.' : 'Tes collègues pourront rejoindre avec un code ou un QR.'}
      footer={
        <Button block size="lg" type="submit" form="create-party" loading={create.isPending}>
          {restaurant ? 'Ouvrir la commande' : 'Créer le salon'}
        </Button>
      }
    >
      <form
        id="create-party"
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate()
        }}
      >
        {restaurant && (
          <div className="flex items-center gap-3 rounded-md border border-border bg-surface p-2.5">
            <RestaurantCover restaurant={restaurant} thumb="120x120" className="size-12 shrink-0 rounded-sm" emojiClassName="text-2xl" />
            <div className="min-w-0">
              <p className="truncate font-semibold">{restaurant.name}</p>
              <p className="text-xs text-muted">Pas de vote : on commande direct ici</p>
            </div>
          </div>
        )}
        <Field label="Nom de la commande">
          {(p) => <Input {...p} value={title} onChange={(e) => setTitle(e.target.value)} maxLength={80} required data-autofocus />}
        </Field>
        <Field label="Adresse de livraison" optional hint="Visible par les membres et reprise dans le récap.">
          {(p) => <Input {...p} value={address} onChange={(e) => setAddress(e.target.value)} placeholder="Bureau, étage, rue…" autoComplete="street-address" />}
        </Field>
        <Field label="Un mot pour l'équipe" optional>
          {(p) => <Textarea {...p} value={notes} onChange={(e) => setNotes(e.target.value)} maxLength={500} placeholder="On commande avant 12h15 !" />}
        </Field>
      </form>
    </Sheet>
  )
}
