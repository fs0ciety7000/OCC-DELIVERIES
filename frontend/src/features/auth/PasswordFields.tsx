import { Field, Input } from '@/components/ui'
import { cn } from '@/lib/cn'
import { MIN_PASSWORD, passwordStrength, type PasswordPair } from './password'

const BAR = ['bg-danger', 'bg-danger', 'bg-warning', 'bg-success'] as const
const TEXT = ['text-danger', 'text-danger', 'text-warning', 'text-success'] as const

/** Jauge de robustesse (indicative) : 3 segments + libellé annoncé poliment. */
export function StrengthMeter({ password }: { password: string }) {
  if (!password) return null
  const s = passwordStrength(password)
  return (
    <div className="space-y-1" aria-live="polite">
      <div className="flex gap-1" aria-hidden>
        {[1, 2, 3].map((i) => (
          <span key={i} className={cn('h-1.5 flex-1 rounded-full transition-colors motion-reduce:transition-none', s.level >= i ? BAR[s.level] : 'bg-fg/10')} />
        ))}
      </div>
      <p className="text-xs">
        <span className={cn('font-semibold', TEXT[s.level])}>Robustesse : {s.label}</span>
        {s.hint && <span className="text-muted"> — {s.hint}</span>}
      </p>
    </div>
  )
}

/** Nouveau mot de passe ×2 + jauge. Les erreurs n'apparaissent qu'une fois la saisie entamée. */
export function PasswordFields({ value, onChange, label = 'Nouveau mot de passe' }: { value: PasswordPair; onChange: (v: PasswordPair) => void; label?: string }) {
  const tooShort = value.password.length > 0 && value.password.length < MIN_PASSWORD
  const mismatch = value.confirm.length > 0 && value.confirm !== value.password
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Field label={label} error={tooShort ? `Encore un petit effort : ${MIN_PASSWORD} caractères minimum.` : undefined}>
          {(p) => <Input {...p} type="password" autoComplete="new-password" required minLength={MIN_PASSWORD} value={value.password} onChange={(e) => onChange({ ...value, password: e.target.value })} />}
        </Field>
        <StrengthMeter password={value.password} />
      </div>
      <Field label="Confirme le mot de passe" error={mismatch ? 'Les deux mots de passe ne correspondent pas.' : undefined}>
        {(p) => <Input {...p} type="password" autoComplete="new-password" required value={value.confirm} onChange={(e) => onChange({ ...value, confirm: e.target.value })} />}
      </Field>
    </div>
  )
}
