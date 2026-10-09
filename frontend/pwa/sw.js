// OCC Deliveries — service worker (gabarit). pwa/plugin.ts le complète au build :
// `SW_VERSION` (empreinte des fichiers), `PRECACHE` (coquille de l'app) et les règles
// de pwa/sw-routes.js. Rien n'est mis en cache en développement (pas de SW).
/* global SW_VERSION, PRECACHE, classify, cacheNames, staleCaches, LIMITS, notificationFromPush */

const CACHES = cacheNames(SW_VERSION)
const PRECACHED = new Set(PRECACHE)

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHES.shell)
      // un fichier manquant ne doit pas empêcher l'installation
      await Promise.all(
        PRECACHE.map((url) =>
          cache.add(new Request(url, { cache: 'reload' })).catch(() => undefined),
        ),
      )
      await self.skipWaiting()
    })(),
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const names = await caches.keys()
      await Promise.all(staleCaches(names, SW_VERSION).map((n) => caches.delete(n)))
      if (self.registration.navigationPreload) await self.registration.navigationPreload.enable().catch(() => undefined)
      await self.clients.claim()
    })(),
  )
})

async function trim(cacheName, max) {
  const cache = await caches.open(cacheName)
  const keys = await cache.keys()
  for (let i = 0; i < keys.length - max; i++) await cache.delete(keys[i])
}

async function navigate(event) {
  try {
    const preload = await event.preloadResponse
    if (preload) return preload
    return await fetch(event.request)
  } catch {
    const shell = await caches.open(CACHES.shell)
    return (await shell.match('/')) || Response.error()
  }
}

async function fromShell(request) {
  const shell = await caches.open(CACHES.shell)
  const hit = await shell.match(request, { ignoreSearch: true })
  if (hit) return hit
  const res = await fetch(request)
  if (res.ok && res.type === 'basic') shell.put(request, res.clone()).catch(() => undefined)
  return res
}

async function staleWhileRevalidate(event) {
  const cache = await caches.open(CACHES.api)
  const hit = await cache.match(event.request)
  const network = fetch(event.request)
    .then(async (res) => {
      if (res.ok) {
        await cache.put(event.request, res.clone())
        trim(CACHES.api, LIMITS.api).catch(() => undefined)
      } else if (res.status === 401 || res.status === 403 || res.status === 404) {
        await cache.delete(event.request)
      }
      return res
    })
  if (hit) {
    event.waitUntil(network.catch(() => undefined))
    return hit
  }
  return network
}

async function networkFirst(event) {
  const cache = await caches.open(CACHES.api)
  try {
    const res = await fetch(event.request)
    if (res.ok) {
      event.waitUntil(cache.put(event.request, res.clone()).then(() => trim(CACHES.api, LIMITS.api)).catch(() => undefined))
    } else if (res.status === 401 || res.status === 403 || res.status === 404) {
      event.waitUntil(cache.delete(event.request))
    }
    return res
  } catch (err) {
    const hit = await cache.match(event.request)
    if (hit) return hit
    throw err
  }
}

async function cacheFirstImage(event) {
  const cache = await caches.open(CACHES.images)
  const hit = await cache.match(event.request)
  if (hit) return hit
  const res = await fetch(event.request)
  if (res.ok || res.type === 'opaque') {
    cache.put(event.request, res.clone()).then(() => trim(CACHES.images, LIMITS.images)).catch(() => undefined)
  }
  return res
}

self.addEventListener('fetch', (event) => {
  const req = event.request
  const kind = classify({ method: req.method, url: req.url, mode: req.mode, destination: req.destination }, self.location.origin, PRECACHED)
  switch (kind) {
    case 'navigate':
      event.respondWith(navigate(event))
      return
    case 'shell':
      event.respondWith(fromShell(req))
      return
    case 'api':
      event.respondWith(staleWhileRevalidate(event))
      return
    case 'live':
      event.respondWith(networkFirst(event))
      return
    case 'image':
      event.respondWith(cacheFirstImage(event))
      return
    default:
      // réseau seul (POST, authentification, temps réel…)
      return
  }
})

self.addEventListener('message', (event) => {
  const type = event.data && event.data.type
  if (type === 'CLEAR_USER_DATA') {
    // déconnexion : aucune donnée personnelle ne reste dans le cache
    event.waitUntil(caches.delete(CACHES.api))
  } else if (type === 'SKIP_WAITING') {
    self.skipWaiting()
  }
})

self.addEventListener('push', (event) => {
  const { title, options } = notificationFromPush(event.data ? event.data.text() : '')
  event.waitUntil(self.registration.showNotification(title, options))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = new URL((event.notification.data && event.notification.data.url) || '/', self.location.origin).href
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
      const same = windows.find((w) => w.url === target)
      if (same) return same.focus()
      const any = windows.find((w) => new URL(w.url).origin === self.location.origin)
      if (any) {
        await any.focus()
        // l'app écoute ce message et navigue sans recharger (voir src/pwa/register.ts)
        any.postMessage({ type: 'OPEN_URL', url: new URL(target).pathname + new URL(target).search })
        return undefined
      }
      return self.clients.openWindow(target)
    })(),
  )
})
