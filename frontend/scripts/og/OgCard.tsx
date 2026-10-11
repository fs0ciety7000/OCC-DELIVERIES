/* eslint-disable react-refresh/only-export-components -- point d'entrée de rendu (scripts/og), pas un module de composants */
import '@fontsource-variable/inter'
import '@fontsource-variable/bricolage-grotesque'
import '@/styles/index.css'
import { createRoot } from 'react-dom/client'
import { Basil, Burger, Chili, Fries, Pizza, RamenBowl, Sushi, Tomato } from '@/components/food/illustrations'

/**
 * Carte Open Graph 1200×630 (accueil + liens d'invitation partagés en messagerie).
 * Design system « Ember », thème sombre ; uniquement des tokens. Rendu : `node scripts/og/render.mjs`.
 */
function OgCard() {
  return (
    <div className="relative isolate h-[630px] w-[1200px] overflow-hidden bg-bg text-fg">
      <div aria-hidden className="app-aurora" />
      {/* halo braise derrière la composition culinaire */}
      <div aria-hidden className="absolute top-[115px] right-[70px] -z-0 size-[440px] rounded-full bg-brand/15 blur-3xl" />

      <div className="relative z-10 flex h-full flex-col justify-between px-[72px] pt-[64px] pb-[52px]">
        <div className="flex items-center gap-4">
          <img src="/favicon.svg" alt="" className="size-14 rounded-[16px] shadow-[var(--shadow-glow)]" />
          <span className="font-display text-[34px] leading-none font-bold tracking-[-0.02em]">OCC Deliveries</span>
        </div>

        <div className="max-w-[640px] space-y-6">
          <p className="text-[20px] font-semibold tracking-[0.08em] text-brand uppercase">Commandes groupées entre collègues</p>
          <h1 className="font-display text-[86px] leading-[88px] font-bold tracking-[-0.03em]">
            Qu'est-ce qu'on <span className="text-ember">mange ?</span>
          </h1>
          <p className="max-w-[600px] text-[26px] leading-[36px] text-muted">
            On vote pour le resto, chacun compose son panier, et on se rembourse en un scan.
          </p>
        </div>

        <div className="flex items-center gap-3 text-[18px] text-subtle">
          <img src="/brand/interactive.svg" alt="" className="h-8 w-auto" />
          <span>
            Développé par <span className="font-semibold text-muted"><span className="font-mono tracking-[0.15em]">CM</span> Interactive</span>
          </span>
          <span aria-hidden>·</span>
          <span>une division de</span>
          <img src="/brand/cardor-monogram.svg" alt="" className="size-6" />
          <span className="font-semibold text-muted">CARDOR Media</span>
        </div>
      </div>

      {/* Composition culinaire (illustrations maison de components/food) */}
      <div aria-hidden className="absolute top-[80px] right-[56px] h-[470px] w-[470px]">
        <div className="absolute inset-[70px] rounded-full border border-border bg-surface shadow-[var(--shadow-card)]" />
        <Pizza className="absolute top-[118px] left-[118px] size-[234px] rotate-[-12deg] drop-shadow-2xl" />
        <Burger className="absolute top-[0px] left-[16px] size-[132px] rotate-[-8deg] drop-shadow-xl" />
        <RamenBowl className="absolute top-[18px] right-[0px] size-[128px] rotate-[6deg] drop-shadow-xl" />
        <Sushi className="absolute right-[6px] bottom-[34px] size-[120px] rotate-[10deg] drop-shadow-xl" />
        <Fries className="absolute bottom-[10px] left-[34px] size-[112px] rotate-[-10deg] drop-shadow-xl" />
        <Tomato className="absolute top-[34px] left-[214px] size-[52px] drop-shadow-lg" />
        <Basil className="absolute top-[150px] right-[-34px] size-[58px] rotate-[24deg] drop-shadow-lg" />
        <Chili className="absolute bottom-[-12px] left-[208px] size-[54px] rotate-[-30deg] drop-shadow-lg" />
      </div>
    </div>
  )
}

createRoot(document.getElementById('og')!).render(<OgCard />)
