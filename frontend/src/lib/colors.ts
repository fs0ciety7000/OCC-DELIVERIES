/**
 * Palette des couleurs d'avatar (donnée utilisateur stockée en `#RRGGBB`,
 * cf. contrat `users.color`) — pas des couleurs d'interface.
 */
export const AVATAR_COLORS = [
  '#FF6A3D', '#FFB547', '#3DD68C', '#6AA8FF', '#B58CFF',
  '#FF7EB6', '#2EC4B6', '#F26D6D', '#C9A227', '#8BD450',
] as const

export function hashString(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (Math.imul(31, h) + s.charCodeAt(i)) | 0
  return Math.abs(h)
}

export function fallbackColor(seed: string): string {
  return AVATAR_COLORS[hashString(seed) % AVATAR_COLORS.length] ?? AVATAR_COLORS[0]
}

/** Texte lisible (encre ou papier) sur une couleur hex donnée. */
export function readableOn(hex: string): 'ink' | 'paper' {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim())
  if (!m?.[1]) return 'paper'
  const n = parseInt(m[1], 16)
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }) as [number, number, number]
  const lum = 0.2126 * r + 0.7152 * g + 0.0722 * b
  return lum > 0.35 ? 'ink' : 'paper'
}

export function isHexColor(v: string | undefined | null): v is string {
  return !!v && /^#[0-9a-f]{6}$/i.test(v)
}
