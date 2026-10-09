/**
 * Palette des couleurs d'avatar (donnée utilisateur stockée en `#RRGGBB`,
 * cf. contrat `users.color`) — pas des couleurs d'interface.
 */
export const AVATAR_COLORS = [
  '#FF6A3D', '#FFB547', '#3DD68C', '#6AA8FF', '#B58CFF',
  '#FF7EB6', '#2EC4B6', '#F26D6D', '#C9A227', '#8BD450',
] as const

/** Hachage simple (Java `hashCode`) — gardé tel quel : les vignettes de restaurants en dépendent. */
export function hashString(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (Math.imul(31, h) + s.charCodeAt(i)) | 0
  return Math.abs(h)
}

/**
 * Hachage bien réparti (FNV-1a 32 bits + finaliseur murmur3) : des graines voisines
 * (« bob », « chloe », ids PocketBase proches) tombent sur des cases éloignées.
 */
export function spreadHash(s: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  h ^= h >>> 16
  h = Math.imul(h, 0x85ebca6b)
  h ^= h >>> 13
  h = Math.imul(h, 0xc2b2ae35)
  h ^= h >>> 16
  return h >>> 0
}

/**
 * Palette des couleurs **attribuées** (repli sans couleur, dédoublonnage d'un groupe) :
 * celle du sélecteur + celle que le serveur tire à l'inscription (`backend/internal/app/hooks.go`),
 * soit des couleurs déjà présentes dans les données. Le texte posé dessus suit `readableOn`.
 */
export const AVATAR_FALLBACK_PALETTE: readonly string[] = [
  ...AVATAR_COLORS,
  '#E4572E', '#F3A712', '#29335C', '#669BBC', '#2A9D8F', '#E76F51',
  '#8E7DBE', '#3D5A80', '#D1495B', '#00798C', '#6A994E', '#BC6C25',
]

export function fallbackColor(seed: string): string {
  return AVATAR_FALLBACK_PALETTE[spreadHash(seed) % AVATAR_FALLBACK_PALETTE.length] ?? AVATAR_COLORS[0]
}

/** Couleur d'un utilisateur : la sienne si elle est valide, sinon une couleur stable dérivée de son id. */
export function avatarColor(user: { id: string; color?: string | null }): string {
  return isHexColor(user.color) ? user.color : fallbackColor(user.id)
}

/**
 * Couleurs d'un **groupe** affiché ensemble (pile d'avatars, membres d'une party) : chacun garde
 * la sienne, sauf si un membre placé avant l'a déjà (comparaison insensible à la casse) ; il reçoit
 * alors la première couleur libre de la palette, à partir d'une case dérivée de son id.
 * Déterministe pour un ordre donné ; au-delà de la taille de la palette, les doublons sont inévitables.
 */
export function distinctColors(users: readonly { id: string; color?: string | null }[]): Map<string, string> {
  const out = new Map<string, string>()
  const used = new Set<string>()
  const palette = AVATAR_FALLBACK_PALETTE
  for (const u of users) {
    if (out.has(u.id)) continue
    let color = avatarColor(u)
    if (used.has(color.toUpperCase())) {
      const start = spreadHash(u.id) % palette.length
      for (let k = 0; k < palette.length; k++) {
        const c = palette[(start + k) % palette.length]!
        if (!used.has(c.toUpperCase())) {
          color = c
          break
        }
      }
    }
    used.add(color.toUpperCase())
    out.set(u.id, color)
  }
  return out
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
