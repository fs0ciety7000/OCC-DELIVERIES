import { useId, type InputHTMLAttributes, type ReactNode, type Ref, type TextareaHTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

const control =
  'w-full rounded-sm border border-border bg-elevated px-3.5 text-fg placeholder:text-subtle transition-[border-color,box-shadow] duration-[120ms] hover:border-border-strong focus:border-brand/70 focus:outline-none focus:ring-3 focus:ring-brand/20 disabled:opacity-60 aria-[invalid=true]:border-danger/70'

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  ref?: Ref<HTMLInputElement>
  leftIcon?: ReactNode
}

export function Input({ className, leftIcon, ref, ...rest }: InputProps) {
  if (leftIcon) {
    return (
      <div className="relative">
        <span aria-hidden className="pointer-events-none absolute top-1/2 left-3.5 -translate-y-1/2 text-subtle">{leftIcon}</span>
        <input ref={ref} className={cn(control, 'min-h-11 pl-10', className)} {...rest} />
      </div>
    )
  }
  return <input ref={ref} className={cn(control, 'min-h-11', className)} {...rest} />
}

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  ref?: Ref<HTMLTextAreaElement>
}

export function Textarea({ className, ref, ...rest }: TextareaProps) {
  return <textarea ref={ref} className={cn(control, 'min-h-22 py-2.5 leading-6', className)} {...rest} />
}

export interface FieldProps {
  label: ReactNode
  hint?: ReactNode
  error?: ReactNode
  className?: string
  optional?: boolean
  children: (props: { id: string; 'aria-describedby'?: string; 'aria-invalid'?: boolean }) => ReactNode
}

/** Label + aide + erreur, reliés au contrôle par `id` / `aria-describedby`. */
export function Field({ label, hint, error, className, optional, children }: FieldProps) {
  const id = useId()
  const hintId = hint ? `${id}-hint` : undefined
  const errId = error ? `${id}-err` : undefined
  const describedBy = [hintId, errId].filter(Boolean).join(' ') || undefined
  return (
    <div className={cn('space-y-1.5', className)}>
      <label htmlFor={id} className="flex items-baseline justify-between text-sm font-medium">
        <span>{label}</span>
        {optional && <span className="text-xs font-normal text-subtle">facultatif</span>}
      </label>
      {children({ id, 'aria-describedby': describedBy, 'aria-invalid': error ? true : undefined })}
      {hint && !error && (
        <p id={hintId} className="text-xs leading-4 text-muted">
          {hint}
        </p>
      )}
      {error && (
        <p id={errId} className="text-xs leading-4 font-medium text-danger" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}
