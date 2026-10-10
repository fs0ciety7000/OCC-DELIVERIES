/**
 * Chargement des emblèmes détaillés de la charte CARDOR Media (`public/brand/<nom>.svg`),
 * sans GSAP (le mouvement vit dans `lib/brandEmblem.ts`, importé à la demande).
 * Chaque forme est enveloppée dans `<g transform>` (position d'origine) › `<g class="part">`
 * (partie animable).
 */
export type EmblemName = 'interactive' | 'cardor-monogram'

const SVG_NS = 'http://www.w3.org/2000/svg'
const cache = new Map<EmblemName, Promise<{ viewBox: string; markup: string }>>()

export function loadEmblem(name: EmblemName): Promise<{ viewBox: string; markup: string }> {
  let p = cache.get(name)
  if (!p) {
    p = fetch(`/brand/${name}.svg`)
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(r.statusText))))
      .then((txt) => {
        const doc = new DOMParser().parseFromString(txt, 'image/svg+xml')
        const svg = doc.documentElement
        if (svg.nodeName !== 'svg') throw new Error('emblème invalide')
        // Fichiers de la charte servis par notre origine ; on retire tout de même ce qui pourrait s'exécuter.
        svg.querySelectorAll('script, foreignObject').forEach((n) => n.remove())
        svg.querySelectorAll('path, circle').forEach((shape) => {
          const outer = doc.createElementNS(SVG_NS, 'g')
          const inner = doc.createElementNS(SVG_NS, 'g')
          inner.setAttribute('class', 'part')
          const t = shape.getAttribute('transform')
          if (t) {
            outer.setAttribute('transform', t)
            shape.removeAttribute('transform')
          }
          shape.replaceWith(outer)
          outer.appendChild(inner)
          inner.appendChild(shape)
        })
        return { viewBox: svg.getAttribute('viewBox') ?? '0 0 100 100', markup: svg.innerHTML }
      })
    p.catch(() => cache.delete(name))
    cache.set(name, p)
  }
  return p
}
