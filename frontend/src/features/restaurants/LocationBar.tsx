import { LocateFixed, MapPin } from 'lucide-react'
import { Button } from '@/components/ui'
import { cn } from '@/lib/cn'
import { useGeo } from '@/lib/geo-context'

export function LocationBar({ className }: { className?: string }) {
  const { coords, status, request, reset } = useGeo()
  return (
    <div className={cn('flex flex-wrap items-center gap-2 text-sm', className)}>
      <span className="inline-flex items-center gap-1.5 text-muted">
        <MapPin aria-hidden className="size-4 text-brand" />
        {coords ? (
          <>
            Autour de <strong className="font-semibold text-fg">{coords.label}</strong>
          </>
        ) : (
          'Localisation…'
        )}
      </span>
      {coords?.source === 'device' ? (
        <Button variant="ghost" size="sm" onClick={reset}>
          Position par défaut
        </Button>
      ) : (
        <Button variant="ghost" size="sm" onClick={request} loading={status === 'locating'} leftIcon={<LocateFixed className="size-4" />}>
          Utiliser ma position
        </Button>
      )}
      {status === 'denied' && <span className="text-xs text-warning" role="status">Position indisponible — on reste sur la zone par défaut.</span>}
    </div>
  )
}
