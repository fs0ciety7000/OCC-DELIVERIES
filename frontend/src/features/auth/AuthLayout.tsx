import type { ReactNode } from 'react'
import { Card } from '@/components/ui'

export function AuthLayout({ title, subtitle, children, footer }: { title: string; subtitle: string; children: ReactNode; footer: ReactNode }) {
  return (
    <div className="mx-auto flex w-full max-w-md flex-col gap-6 py-6 sm:py-12">
      <div className="space-y-2 text-center">
        <h1 className="font-display text-[32px] leading-9 font-bold">{title}</h1>
        <p className="text-muted">{subtitle}</p>
      </div>
      <Card className="p-5 sm:p-6">{children}</Card>
      <p className="text-center text-sm text-muted">{footer}</p>
    </div>
  )
}
