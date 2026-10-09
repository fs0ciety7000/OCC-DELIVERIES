import { hashString } from '@/lib/colors'
import { fileUrl } from '@/lib/pb'
import type { MenuItem, Restaurant } from '@/lib/types'

export function coverUrl(r: Pick<Restaurant, 'id' | 'cover' | 'cover_url'>, thumb?: string): string | undefined {
  return fileUrl('restaurants', r.id, r.cover, thumb) ?? (r.cover_url || undefined)
}

export function itemImageUrl(i: Pick<MenuItem, 'id' | 'image'>): string | undefined {
  return fileUrl('menu_items', i.id, i.image, '200x200')
}

/** Tuile générée (dégradé token) déterministe pour un restaurant sans visuel. */
export function tileStyle(seed: string): { backgroundImage: string } {
  return { backgroundImage: `var(--tile-${(hashString(seed) % 6) + 1})` }
}

export const CUISINE_LABELS: Record<string, string> = {
  pizza: 'Pizza',
  italien: 'Italien',
  burger: 'Burgers',
  burgers: 'Burgers',
  sushi: 'Sushi',
  japonais: 'Japonais',
  asiatique: 'Asiatique',
  thai: 'Thaï',
  indien: 'Indien',
  libanais: 'Libanais',
  kebab: 'Kebab',
  poke: 'Poke',
  salade: 'Salades',
  salades: 'Salades',
  sandwich: 'Sandwichs',
  sandwichs: 'Sandwichs',
  veggie: 'Veggie',
  vegan: 'Vegan',
  frites: 'Frites',
  friterie: 'Friterie',
  belge: 'Belge',
  mexicain: 'Mexicain',
  chinois: 'Chinois',
  healthy: 'Healthy',
  desserts: 'Desserts',
  boulangerie: 'Boulangerie',
}

export function cuisineLabel(c: string): string {
  return CUISINE_LABELS[c.toLowerCase()] ?? c.charAt(0).toUpperCase() + c.slice(1)
}

export const TAG_LABELS: Record<string, { label: string; emoji: string }> = {
  veggie: { label: 'Végé', emoji: '🌱' },
  vegan: { label: 'Vegan', emoji: '🌿' },
  spicy: { label: 'Épicé', emoji: '🌶️' },
  gluten_free: { label: 'Sans gluten', emoji: '🌾' },
  new: { label: 'Nouveau', emoji: '✨' },
}

export const PROVIDER_LABELS: Record<string, string> = {
  ubereats: 'Uber Eats',
  takeaway: 'Takeaway',
  manual: 'Manuel',
}
