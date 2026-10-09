import { Download, ExternalLink, Phone } from 'lucide-react'
import { Suspense, useState } from 'react'
import { DispatchScooter } from '@/components/food'
import { toast } from 'sonner'
import { Badge, Button, CopyButton, Sheet } from '@/components/ui'
import { occ } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import type { Dispatch, ExportFormat, Party } from '@/lib/types'
import { DISPATCH_LABELS } from '../labels'

export function ExportButtons({ party }: { party: Party }) {
  const [busy, setBusy] = useState<ExportFormat | null>(null)
  const run = async (f: ExportFormat) => {
    setBusy(f)
    try {
      await occ.downloadExport(party.id, f, `commande-${party.code}`)
      toast.success(`Export ${f.toUpperCase()} téléchargé`)
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(null)
    }
  }
  return (
    <div className="flex flex-wrap gap-2">
      {(['csv', 'txt', 'json'] as const).map((f) => (
        <Button key={f} variant="secondary" size="sm" leftIcon={<Download className="size-4" />} loading={busy === f} onClick={() => run(f)}>
          {f.toUpperCase()}
        </Button>
      ))}
    </div>
  )
}

export function DispatchSheet({ dispatch, party, phone, onClose }: { dispatch: Dispatch | null; party: Party; phone?: string; onClose: () => void }) {
  const method = dispatch?.method
  return (
    <Sheet
      open={!!dispatch}
      onClose={onClose}
      size="lg"
      title={method ? `Envoyer via ${DISPATCH_LABELS[method]}` : ''}
      description={method && method !== 'export' && method !== 'phone' ? 'Pas d’API publique pour remplir le panier : on te guide, ça prend 2 minutes.' : undefined}
      footer={
        dispatch?.url ? (
          <a
            href={dispatch.url}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex min-h-13 w-full items-center justify-center gap-2 rounded-md bg-ember px-6 font-semibold text-brand-fg shadow-glow"
          >
            Ouvrir {method ? DISPATCH_LABELS[method] : ''} <ExternalLink className="size-4" />
          </a>
        ) : method === 'phone' && phone ? (
          <a href={`tel:${phone}`} className="inline-flex min-h-13 w-full items-center justify-center gap-2 rounded-md bg-ember px-6 font-semibold text-brand-fg shadow-glow">
            <Phone className="size-4" /> Appeler {phone}
          </a>
        ) : undefined
      }
    >
      {dispatch && (
        <div className="space-y-5">
          <Suspense fallback={<div className="h-24" />}>
            <DispatchScooter />
          </Suspense>
          {dispatch.instructions.length > 0 && (
            <ol className="space-y-2.5">
              {dispatch.instructions.map((step, i) => (
                <li key={i} className="flex gap-3">
                  <span className="grid size-7 shrink-0 place-items-center rounded-full bg-brand/15 text-sm font-bold text-brand tabular">{i + 1}</span>
                  <span className="pt-0.5 text-[15px]">{step}</span>
                </li>
              ))}
            </ol>
          )}
          {dispatch.cartText && (
            <div className="space-y-2">
              <div className="flex items-center justify-between gap-2">
                <h3 className="font-semibold">{method === 'phone' ? 'Script à dicter' : 'Récap à copier'}</h3>
                <CopyButton value={dispatch.cartText} label="Copier le récap" toastMessage="Récap copié" />
              </div>
              <pre className="max-h-72 overflow-auto rounded-md border border-border bg-surface p-3.5 font-sans text-sm leading-6 whitespace-pre-wrap">{dispatch.cartText}</pre>
            </div>
          )}
          {(method === 'export' || method === 'phone') && (
            <div className="space-y-2">
              <h3 className="font-semibold">Télécharger</h3>
              <ExportButtons party={party} />
            </div>
          )}
          <Badge variant="info">Prix indicatifs : vérifie le total sur la plateforme avant de payer.</Badge>
        </div>
      )}
    </Sheet>
  )
}
