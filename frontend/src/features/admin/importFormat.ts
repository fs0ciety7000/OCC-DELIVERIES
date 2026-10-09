import type { RestaurantImport } from '@/lib/types'

/** Colonnes du CSV d'import (une ligne par article). Doit rester aligné sur `catalog.ParseCSV`. */
export const CSV_COLUMNS = ['restaurant_slug', 'restaurant_name', 'category', 'item_name', 'description', 'price_eur', 'tags', 'popular'] as const

/** Colonnes facultatives (infos restaurant, à remplir une fois par restaurant). */
export const CSV_OPTIONAL_COLUMNS = ['address', 'lat', 'lng', 'cuisines', 'phone', 'ubereats_url', 'takeaway_url'] as const

const ALL_COLUMNS = [...CSV_COLUMNS, ...CSV_OPTIONAL_COLUMNS]

const EXAMPLE_ROWS: Record<(typeof ALL_COLUMNS)[number], string>[] = [
  {
    restaurant_slug: 'chez-mario',
    restaurant_name: 'Chez Mario',
    category: 'Pizzas',
    item_name: 'Margherita',
    description: 'Tomate; mozzarella; basilic',
    price_eur: '12,50',
    tags: 'veggie',
    popular: 'oui',
    address: 'Rue de Nimy 1, 7000 Mons',
    lat: '50,4542',
    lng: '3,9567',
    cuisines: 'pizza|italien',
    phone: '065 00 00 00',
    ubereats_url: 'https://www.ubereats.com/be/store/chez-mario',
    takeaway_url: '',
  },
  {
    restaurant_slug: 'chez-mario',
    restaurant_name: '',
    category: 'Desserts',
    item_name: 'Tiramisu',
    description: '',
    price_eur: '6',
    tags: '',
    popular: '',
    address: '',
    lat: '',
    lng: '',
    cuisines: '',
    phone: '',
    ubereats_url: '',
    takeaway_url: '',
  },
]

function csvCell(value: string, sep: string): string {
  return /["\n\r]/.test(value) || value.includes(sep) ? `"${value.replace(/"/g, '""')}"` : value
}

/**
 * Modèle CSV : séparateur « ; » (Excel FR, la virgule étant le séparateur décimal),
 * BOM UTF-8 pour qu'Excel lise les accents.
 */
export function csvTemplate(sep = ';'): string {
  const lines = [ALL_COLUMNS.join(sep), ...EXAMPLE_ROWS.map((row) => ALL_COLUMNS.map((c) => csvCell(row[c], sep)).join(sep))]
  return '\ufeff' + lines.join('\r\n') + '\r\n'
}

/** Exemple JSON (un tableau de restaurants, mêmes clés que l'export). */
export const JSON_EXAMPLE: RestaurantImport[] = [
  {
    slug: 'chez-mario',
    name: 'Chez Mario',
    description: 'Pizzas au feu de bois.',
    emoji: '🍕',
    cuisines: ['pizza', 'italien'],
    address: 'Rue de Nimy 1, 7000 Mons',
    lat: 50.4542,
    lng: 3.9567,
    phone: '065 00 00 00',
    price_level: 2,
    eta_min: 25,
    eta_max: 40,
    delivery_fee: 299,
    min_order: 1500,
    providers: [{ id: 'ubereats', url: 'https://www.ubereats.com/be/store/chez-mario' }],
    active: true,
    categories: [
      {
        name: 'Pizzas',
        items: [
          {
            name: 'Margherita',
            description: 'Tomate, mozzarella, basilic.',
            price: 1250,
            tags: ['veggie'],
            popular: true,
            option_groups: [
              { id: 'size', name: 'Taille', min: 1, max: 1, choices: [{ id: 'm', name: 'Moyenne', price: 0 }, { id: 'l', name: 'Large', price: 300 }] },
            ],
          },
        ],
      },
    ],
  },
]

export type ImportKind = 'json' | 'csv'

/** Devine le format d'un fichier déposé (extension, puis contenu). */
export function detectImportKind(filename: string, content: string): ImportKind | null {
  const lower = filename.toLowerCase()
  if (lower.endsWith('.json')) return 'json'
  if (lower.endsWith('.csv') || lower.endsWith('.txt')) return 'csv'
  const t = content.replace(/^\ufeff/, '').trimStart()
  if (t.startsWith('[') || t.startsWith('{')) return 'json'
  if (/restaurant_slug/i.test(t.split(/\r?\n/, 1)[0] ?? '')) return 'csv'
  return null
}

/** Lit un JSON d'import : un objet ou un tableau (ou `{ restaurants: [...] }`). */
export function parseImportJson(content: string): RestaurantImport | RestaurantImport[] {
  const data: unknown = JSON.parse(content.replace(/^\ufeff/, ''))
  if (Array.isArray(data)) return data as RestaurantImport[]
  if (data && typeof data === 'object') {
    const wrapped = (data as { restaurants?: unknown }).restaurants
    if (Array.isArray(wrapped)) return wrapped as RestaurantImport[]
    return data as RestaurantImport
  }
  throw new Error('Le JSON doit contenir un restaurant ou une liste de restaurants.')
}
