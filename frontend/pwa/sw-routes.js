// Règles de cache du service worker (pures, testées : pwa/sw-routes.test.js).
// Ce fichier est concaténé tel quel dans dist/sw.js par pwa/plugin.ts (les `export`
// sont retirés) : pas d'import, uniquement des fonctions.

/** Préfixes jamais mis en cache (authentification, temps réel, admin, actions personnelles). */
export const NEVER_CACHE = [
  '/api/realtime',
  '/api/oauth2-redirect',
  '/api/collections/users/',
  '/api/collections/_',
  '/api/collections/payout_profiles/',
  '/api/collections/push_subscriptions/',
  '/api/occ/me/',
  '/api/occ/push/',
  '/api/occ/admin/',
  '/api/occ/payments/',
  '/api/occ/health',
  '/api/files/payout_profiles/',
  '/_/',
]

/** Catalogue public servi « stale-while-revalidate » (affichage immédiat, mis à jour en fond). */
export const SWR_PATTERNS = [
  /^\/api\/occ\/config$/,
  /^\/api\/occ\/restaurants\/nearby$/,
  /^\/api\/collections\/(restaurants|menu_categories|menu_items)\/records(\/[a-z0-9]+)?$/,
]

/**
 * Données vivantes d'une commande : réseau d'abord, cache seulement hors ligne. (Un
 * stale-while-revalidate renverrait l'ancienne liste juste après un ajout au panier ou un
 * évènement temps réel : l'invalidation TanStack recevrait une réponse périmée.)
 */
export const LIVE_PATTERNS = [
  /^\/api\/occ\/parties\/[a-z0-9]+\/(summary|tally)$/,
  /^\/api\/collections\/(parties|party_members|votes|order_items|payments)\/records(\/[a-z0-9]+)?$/,
]

/**
 * Stratégie pour une requête :
 * - `none` : réseau seul (POST, auth, temps réel…) ;
 * - `navigate` : réseau d'abord, repli sur la coquille `/` en cache ;
 * - `shell` : fichier précaché (cache d'abord) ;
 * - `api` : stale-while-revalidate (catalogue public) ;
 * - `live` : réseau d'abord, repli sur le cache hors ligne (données d'une commande) ;
 * - `image` : cache d'abord, cache borné.
 * @param {{ method: string, url: string, mode?: string, destination?: string }} req
 * @param {string} origin origine de l'app (self.location.origin)
 * @param {Set<string>} precached chemins précachés
 * @returns {'none'|'navigate'|'shell'|'api'|'live'|'image'}
 */
export function classify(req, origin, precached) {
  if (req.method !== 'GET') return 'none'
  let url
  try {
    url = new URL(req.url)
  } catch {
    return 'none'
  }
  if (url.origin !== origin) {
    // images distantes des restaurants (cover_url) uniquement
    return req.destination === 'image' && url.protocol === 'https:' ? 'image' : 'none'
  }
  const path = url.pathname
  if (NEVER_CACHE.some((p) => path.startsWith(p))) return 'none'
  if (path.startsWith('/api/files/')) {
    // fichiers protégés (jeton) : jamais en cache
    return url.searchParams.has('token') ? 'none' : 'image'
  }
  if (path.startsWith('/api/')) {
    if (SWR_PATTERNS.some((re) => re.test(path))) return 'api'
    return LIVE_PATTERNS.some((re) => re.test(path)) ? 'live' : 'none'
  }
  if (req.mode === 'navigate') return 'navigate'
  if (precached.has(path)) return 'shell'
  if (path.startsWith('/assets/')) return 'shell'
  if (req.destination === 'image') return 'image'
  return 'none'
}

/**
 * Noms de cache versionnés et ceux à supprimer à l'activation.
 * @param {string} version
 */
export function cacheNames(version) {
  return { shell: `occ-shell-${version}`, api: 'occ-api-v1', images: 'occ-images-v1' }
}

/**
 * Caches obsolètes : anciennes coquilles et caches OCC inconnus.
 * @param {string[]} existing
 * @param {string} version
 */
export function staleCaches(existing, version) {
  const keep = new Set(Object.values(cacheNames(version)))
  return existing.filter((name) => name.startsWith('occ-') && !keep.has(name))
}

/** Bornes des caches d'exécution (entrées). */
export const LIMITS = { api: 120, images: 150 }

/**
 * Lit la charge utile d'une notification push (JSON du serveur, voir backend/internal/notify).
 * @param {string | null | undefined} text
 */
export function notificationFromPush(text) {
  let data = {}
  try {
    data = text ? JSON.parse(text) : {}
  } catch {
    data = { body: String(text) }
  }
  const url = typeof data.url === 'string' && data.url.startsWith('/') ? data.url : '/'
  return {
    title: data.title || 'OCC Deliveries',
    options: {
      body: data.body || '',
      icon: data.icon || '/icons/icon-192.png',
      badge: data.badge || '/icons/badge-72.png',
      tag: data.tag || undefined,
      renotify: Boolean(data.tag && data.renotify),
      timestamp: typeof data.ts === 'number' ? data.ts : undefined,
      lang: 'fr',
      data: { url, kind: data.kind || '', partyId: data.partyId || '' },
    },
  }
}
