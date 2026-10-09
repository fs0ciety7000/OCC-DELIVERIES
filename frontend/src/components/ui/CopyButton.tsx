import { Check, Copy } from 'lucide-react'
import { toast } from 'sonner'
import { useCopy } from '@/lib/hooks'
import { Button, type ButtonSize, type ButtonVariant } from './Button'

export interface CopyButtonProps {
  value: string
  label?: string
  /** Libellé accessible si `label` est vide (bouton icône). */
  ariaLabel?: string
  toastMessage?: string
  variant?: ButtonVariant
  size?: ButtonSize
  className?: string
}

export function CopyButton({ value, label = 'Copier', ariaLabel, toastMessage = 'Copié !', variant = 'secondary', size = 'sm', className }: CopyButtonProps) {
  const { copied, copy } = useCopy()
  const icon = copied ? <Check className="size-4 text-success" /> : <Copy className="size-4" />
  return (
    <Button
      variant={variant}
      size={size}
      className={className}
      aria-label={ariaLabel ?? (label || 'Copier')}
      onClick={async () => {
        const ok = await copy(value)
        if (ok) toast.success(toastMessage)
        else toast.error('Copie impossible — sélectionne le texte manuellement.')
      }}
      leftIcon={icon}
    >
      {size === 'icon' ? null : copied ? 'Copié' : label}
    </Button>
  )
}
