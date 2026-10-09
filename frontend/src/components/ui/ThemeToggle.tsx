import { Moon, Sun } from 'lucide-react'
import { cn } from '@/lib/cn'
import { useTheme } from '@/lib/theme'

export function ThemeToggle({ className }: { className?: string }) {
  const { resolved, setPref } = useTheme()
  const next = resolved === 'dark' ? 'light' : 'dark'
  return (
    <button
      type="button"
      onClick={() => setPref(next)}
      className={cn('grid size-11 place-items-center rounded-full text-muted transition-colors hover:bg-fg/[0.06] hover:text-fg', className)}
      aria-label={next === 'light' ? 'Passer au thème clair' : 'Passer au thème sombre'}
      title={next === 'light' ? 'Thème clair' : 'Thème sombre'}
    >
      {resolved === 'dark' ? <Sun className="size-5" /> : <Moon className="size-5" />}
    </button>
  )
}
