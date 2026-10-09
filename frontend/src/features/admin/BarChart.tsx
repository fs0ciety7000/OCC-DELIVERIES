import { useId, useState } from 'react'

export interface BarDatum {
  label: string
  /** Libellé long pour l'infobulle / le tableau (ex. « mer. 8 oct. »). */
  title: string
  value: number
}

/**
 * Histogramme léger en SVG (une seule série, braise) : barres fines à bouts arrondis
 * posées sur la ligne de base, écart de 2 px, axe discret, infobulle au survol / focus,
 * tableau accessible en alternative.
 */
export function BarChart({ data, label, height = 140, unit = '' }: { data: BarDatum[]; label: string; height?: number; unit?: string }) {
  const [active, setActive] = useState<number | null>(null)
  const tableId = useId()
  const max = Math.max(1, ...data.map((d) => d.value))
  const width = 600
  const top = 18
  const bottom = 20
  const plot = height - top - bottom
  const slot = width / Math.max(1, data.length)
  const barW = Math.max(2, slot - 2)
  const ticks = [0, Math.ceil(max / 2), max].filter((v, i, a) => a.indexOf(v) === i)
  const current = active !== null ? data[active] : null

  return (
    <figure className="relative">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        className="h-auto w-full overflow-hidden"
        role="img"
        aria-label={`${label} — maximum ${max}${unit}`}
        aria-describedby={tableId}
        onMouseLeave={() => setActive(null)}
      >
        {ticks.map((t) => {
          const y = top + plot - (t / max) * plot
          return (
            <g key={t} className="text-subtle">
              <line x1={0} x2={width} y1={y} y2={y} stroke="currentColor" strokeOpacity={t === 0 ? 0.5 : 0.18} strokeWidth={1} />
              {t > 0 && (
                <text x={width} y={y - 4} textAnchor="end" fontSize={11} fill="currentColor">
                  {t}
                </text>
              )}
            </g>
          )
        })}
        {data.map((d, i) => {
          const h = d.value > 0 ? Math.max(3, (d.value / max) * plot) : 0
          const x = i * slot + (slot - barW) / 2
          const y = top + plot - h
          const r = Math.min(4, barW / 2, h)
          return (
            <g
              key={d.label + i}
              tabIndex={0}
              role="presentation"
              onMouseEnter={() => setActive(i)}
              onFocus={() => setActive(i)}
              onBlur={() => setActive(null)}
              className="outline-none focus-visible:[&>rect:last-child]:stroke-brand"
            >
              {/* zone de survol plus large que la barre */}
              <rect x={i * slot} y={top} width={slot} height={plot} fill="transparent" />
              {h > 0 && (
                <path
                  d={`M${x},${top + plot} V${y + r} Q${x},${y} ${x + r},${y} H${x + barW - r} Q${x + barW},${y} ${x + barW},${y + r} V${top + plot} Z`}
                  className={active === null || active === i ? 'fill-brand' : 'fill-brand/45'}
                />
              )}
              <rect x={Math.max(1, x - 1)} y={top} width={Math.min(width - 1, x + barW + 1) - Math.max(1, x - 1)} height={plot} fill="none" strokeWidth={2} rx={4} stroke="transparent" />
            </g>
          )
        })}
        {data.length > 0 && (
          <g className="text-subtle" fontSize={11} fill="currentColor">
            <text x={0} y={height - 4}>
              {data[0]!.label}
            </text>
            <text x={width} y={height - 4} textAnchor="end">
              {data[data.length - 1]!.label}
            </text>
          </g>
        )}
      </svg>
      {current && (
        <div
          className="pointer-events-none absolute top-0 rounded-sm border border-border bg-elevated px-2.5 py-1.5 text-xs shadow-card"
          style={{ left: `${Math.min(80, Math.max(0, ((active ?? 0) / Math.max(1, data.length)) * 100 - 6))}%` }}
          aria-hidden
        >
          <span className="font-semibold">{current.title}</span> · <span className="tabular-nums">{current.value}{unit}</span>
        </div>
      )}
      <table id={tableId} className="sr-only">
        <caption>{label}</caption>
        <tbody>
          {data.map((d, i) => (
            <tr key={d.label + i}>
              <th scope="row">{d.title}</th>
              <td>{d.value}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  )
}
