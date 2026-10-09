import type { SVGProps } from 'react'

/**
 * Illustrations SVG maison, style flat, trait 2 px, couleurs = tokens `--color-food-*`.
 * Toutes décoratives (aria-hidden) : le sens est porté par le texte autour.
 */
type P = SVGProps<SVGSVGElement>

function Svg({ children, viewBox = '0 0 64 64', ...rest }: P) {
  return (
    <svg viewBox={viewBox} aria-hidden focusable="false" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" {...rest}>
      {children}
    </svg>
  )
}

const ink = 'stroke-food-ink'

/** Parts de pizza (6) — `data-slice` sur chaque part pour FoodLoader. */
export function Pizza(props: P) {
  const slices = Array.from({ length: 6 }, (_, i) => i * 60)
  return (
    <Svg {...props}>
      {slices.map((a) => (
        <g key={a} data-slice={a} style={{ transformOrigin: '32px 32px', transformBox: 'view-box' }}>
          <path
            d={`M32 32 L${32 + 26 * Math.cos(((a - 90) * Math.PI) / 180)} ${32 + 26 * Math.sin(((a - 90) * Math.PI) / 180)} A26 26 0 0 1 ${32 + 26 * Math.cos(((a - 30) * Math.PI) / 180)} ${32 + 26 * Math.sin(((a - 30) * Math.PI) / 180)} Z`}
            className={`fill-food-cheese ${ink}`}
          />
          <circle cx={32 + 15 * Math.cos(((a - 60) * Math.PI) / 180)} cy={32 + 15 * Math.sin(((a - 60) * Math.PI) / 180)} r="3.2" className="fill-food-tomato" />
        </g>
      ))}
      <circle cx="32" cy="32" r="26" fill="none" className="stroke-food-crust" strokeWidth={4} />
    </Svg>
  )
}

export function RamenBowl(props: P) {
  return (
    <Svg {...props}>
      <path data-steam="1" d="M24 18c-3-4 3-6 0-10" fill="none" className="stroke-food-steel" />
      <path data-steam="2" d="M32 18c-3-4 3-6 0-10" fill="none" className="stroke-food-steel" />
      <path data-steam="3" d="M40 18c-3-4 3-6 0-10" fill="none" className="stroke-food-steel" />
      <path d="M8 30h48c0 13-10 22-24 22S8 43 8 30Z" className={`fill-food-bowl ${ink}`} />
      <ellipse cx="32" cy="30" rx="24" ry="5" className={`fill-food-broth ${ink}`} />
      <path d="M22 29c4-2 8 2 12 0s8 2 10 0" fill="none" className="stroke-food-cheese" />
      <path d="M42 8 30 30M48 10 34 30" className="stroke-food-patty" />
    </Svg>
  )
}

export function Burger(props: P) {
  return (
    <Svg {...props}>
      <path d="M10 28c0-11 10-18 22-18s22 7 22 18Z" className={`fill-food-bun ${ink}`} />
      <path d="M8 32c4 3 8-3 12 0s8-3 12 0 8-3 12 0 8-3 12 0" fill="none" className="stroke-food-lettuce" strokeWidth={4} />
      <rect x="9" y="35" width="46" height="8" rx="4" className={`fill-food-patty ${ink}`} />
      <path d="M10 46h44c0 5-4 8-9 8H19c-5 0-9-3-9-8Z" className={`fill-food-bun ${ink}`} />
      <circle cx="24" cy="18" r="1.2" className="fill-food-rice" />
      <circle cx="34" cy="15" r="1.2" className="fill-food-rice" />
      <circle cx="42" cy="20" r="1.2" className="fill-food-rice" />
    </Svg>
  )
}

export function Sushi(props: P) {
  return (
    <Svg {...props}>
      <ellipse cx="32" cy="42" rx="20" ry="9" className={`fill-food-rice ${ink}`} />
      <path d="M12 38c2-10 38-12 40 0-6 6-34 6-40 0Z" className={`fill-food-salmon ${ink}`} />
      <path d="M20 34l4 6M28 32l4 7M36 32l4 7" className="stroke-food-rice" />
      <rect x="28" y="30" width="8" height="20" className="fill-food-nori" />
    </Svg>
  )
}

export function Fries(props: P) {
  return (
    <Svg {...props}>
      {[18, 24, 30, 36, 42].map((x, i) => (
        <rect key={x} x={x} y={10 + (i % 2) * 5} width="5" height="26" rx="1.5" className={`fill-food-cheese ${ink}`} transform={`rotate(${(i - 2) * 6} ${x} 36)`} />
      ))}
      <path d="M14 30h36l-5 26H19Z" className={`fill-food-tomato ${ink}`} />
      <path d="M24 40h16" className="stroke-food-rice" />
    </Svg>
  )
}

export function Chili(props: P) {
  return (
    <Svg {...props}>
      <path d="M44 14c-2 18-14 34-32 38 18-14 18-26 22-34 3-5 7-6 10-4Z" className={`fill-food-chili ${ink}`} />
      <path d="M44 14c2-4 6-6 9-5" fill="none" className="stroke-food-basil" strokeWidth={3} />
    </Svg>
  )
}

export function Basil(props: P) {
  return (
    <Svg {...props}>
      <path d="M12 52C10 30 26 12 52 12c0 26-18 42-40 40Z" className={`fill-food-basil ${ink}`} />
      <path d="M14 50 44 20" fill="none" className="stroke-food-lettuce" />
    </Svg>
  )
}

export function Tomato(props: P) {
  return (
    <Svg {...props}>
      <circle cx="32" cy="36" r="20" className={`fill-food-tomato ${ink}`} />
      <path d="M32 16l-6 6M32 16l6 6M32 16v-6M32 16l-9 1M32 16l9 1" className="stroke-food-basil" strokeWidth={3} />
      <path d="M22 32c1-4 4-7 8-8" fill="none" className="stroke-food-rice" opacity=".6" />
    </Svg>
  )
}

export function Scooter(props: P) {
  return (
    <Svg {...props}>
      <rect x="6" y="18" width="18" height="16" rx="3" className={`fill-food-tomato ${ink}`} />
      <path d="M10 26h10" className="stroke-food-rice" />
      <path d="M24 40h18l6-14h6" fill="none" className="stroke-food-ink" strokeWidth={3} />
      <path d="M14 40c0-6 6-8 12-8h14l-4 8Z" className={`fill-food-crust ${ink}`} />
      <circle cx="16" cy="46" r="6" className={`fill-food-nori ${ink}`} />
      <circle cx="48" cy="46" r="6" className={`fill-food-nori ${ink}`} />
      <circle cx="44" cy="14" r="5" className={`fill-food-cheese ${ink}`} />
    </Svg>
  )
}

export function ServiceBell(props: P) {
  return (
    <Svg {...props}>
      <circle cx="32" cy="16" r="3" className={`fill-food-gold ${ink}`} />
      <path d="M12 44c0-12 9-24 20-24s20 12 20 24Z" className={`fill-food-gold ${ink}`} />
      <rect x="8" y="44" width="48" height="6" rx="3" className={`fill-food-steel ${ink}`} />
      <path d="M22 34c2-5 5-8 9-9" fill="none" className="stroke-food-rice" opacity=".7" />
    </Svg>
  )
}

export function DeliveryBag(props: P) {
  return (
    <Svg {...props}>
      <path d="M24 22v-4a8 8 0 0 1 16 0v4" fill="none" className="stroke-food-ink" strokeWidth={3} />
      <path d="M12 22h40l-4 32H16Z" className={`fill-food-crust ${ink}`} />
      <path d="M26 36c2 3 10 3 12 0" fill="none" className="stroke-food-ink" />
    </Svg>
  )
}

/** Tirelire-burger : la fente reçoit les pièces (PaymentCoin). */
export function PiggyBurger(props: P) {
  return (
    <Svg {...props}>
      <path d="M8 30c0-10 11-16 24-16s24 6 24 16Z" className={`fill-food-bun ${ink}`} />
      <rect x="26" y="17" width="12" height="3" rx="1.5" className="fill-food-ink" />
      <path d="M6 34c4 3 8-3 13 0s9-3 13 0 9-3 13 0 9-3 13 0" fill="none" className="stroke-food-lettuce" strokeWidth={4} />
      <rect x="7" y="37" width="50" height="8" rx="4" className={`fill-food-patty ${ink}`} />
      <path d="M8 48h48c0 5-5 8-10 8H18c-5 0-10-3-10-8Z" className={`fill-food-bun ${ink}`} />
      <circle cx="20" cy="26" r="1.8" className="fill-food-ink" />
      <path d="M56 26c4 0 4 6 0 6" fill="none" className="stroke-food-ink" />
      <path d="M14 56v4M50 56v4" className="stroke-food-ink" strokeWidth={3} />
    </Svg>
  )
}

export function Coin(props: P) {
  return (
    <Svg viewBox="0 0 24 24" {...props}>
      <circle cx="12" cy="12" r="10" className={`fill-food-gold ${ink}`} />
      <path d="M15 8.5a4 4 0 1 0 0 7M7.5 11h6M7.5 13.5h6" fill="none" className="stroke-food-ink" strokeWidth={1.6} />
    </Svg>
  )
}

export function Plate(props: P) {
  return (
    <Svg viewBox="0 0 96 64" {...props}>
      <ellipse cx="48" cy="40" rx="30" ry="14" className={`fill-food-rice ${ink}`} />
      <ellipse cx="48" cy="38" rx="18" ry="7" fill="none" className="stroke-food-steel" />
      <g data-fork style={{ transformOrigin: '84px 52px', transformBox: 'view-box' }}>
        <path d="M84 52V20M80 20v8a4 4 0 0 0 8 0v-8M84 20v8" fill="none" className="stroke-food-steel" strokeWidth={2.5} />
      </g>
      <path d="M12 52V20c4 2 5 8 5 14h-5" fill="none" className="stroke-food-steel" strokeWidth={2.5} />
    </Svg>
  )
}
