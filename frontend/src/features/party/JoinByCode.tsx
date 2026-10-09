import { ArrowRight } from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui'
import { cn } from '@/lib/cn'
import { normalizeCode } from './hooks'

export function JoinByCode({ className }: { className?: string }) {
  const [code, setCode] = useState('')
  const navigate = useNavigate()
  const valid = code.length === 6
  return (
    <form
      className={cn('flex gap-2', className)}
      onSubmit={(e) => {
        e.preventDefault()
        if (valid) navigate(`/j/${code}`)
      }}
    >
      <label htmlFor="join-code" className="sr-only">
        Code de la commande (6 caractères)
      </label>
      <input
        id="join-code"
        value={code}
        onChange={(e) => setCode(normalizeCode(e.target.value))}
        placeholder="CODE"
        inputMode="text"
        autoCapitalize="characters"
        autoComplete="off"
        spellCheck={false}
        maxLength={6}
        className="min-h-12 w-full min-w-0 flex-1 rounded-md border border-border bg-elevated px-4 font-display text-xl font-semibold tracking-[0.35em] uppercase placeholder:tracking-[0.2em] placeholder:text-subtle focus:border-brand/70 focus:ring-3 focus:ring-brand/20 focus:outline-none"
      />
      <Button type="submit" size="lg" variant={valid ? 'primary' : 'secondary'} disabled={!valid} rightIcon={<ArrowRight className="size-4" />}>
        Rejoindre
      </Button>
    </form>
  )
}
