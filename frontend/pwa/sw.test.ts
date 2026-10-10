import { describe, expect, it } from 'vitest'
// @ts-expect-error -- module JS sans types, concaténé dans dist/sw.js
import { cacheNames, classify, notificationFromPush, staleCaches } from './sw-routes.js'
import { buildServiceWorker, precacheList } from './plugin'

const origin = 'https://eat.fs0ciety.org'
const pre = new Set(['/', '/assets/index-abc.js', '/favicon.svg'])
const get = (path: string, extra: Record<string, string> = {}) => ({ method: 'GET', url: path.startsWith('http') ? path : origin + path, ...extra })

describe('règles de cache du service worker', () => {
  it.each([
    ['POST jamais en cache', { ...get('/api/occ/parties/x/ready'), method: 'POST' }, 'none'],
    ['authentification', get('/api/collections/users/auth-with-password'), 'none'],
    ['rafraîchissement du jeton', get('/api/collections/users/auth-refresh'), 'none'],
    ['temps réel', get('/api/realtime'), 'none'],
    ['admin', get('/api/occ/admin/stats'), 'none'],
    ['profil personnel', get('/api/occ/me/history'), 'none'],
    ['QR de paiement', get('/api/occ/payments/abc/qr'), 'none'],
    ['coordonnées de paiement', get('/api/collections/payout_profiles/records'), 'none'],
    ['restos proches', get('/api/occ/restaurants/nearby?lat=1&lng=2'), 'api'],
    ['configuration', get('/api/occ/config'), 'api'],
    ['menu', get('/api/collections/menu_items/records?filter=x'), 'api'],
    ['fiche resto', get('/api/collections/restaurants/records/abc123'), 'api'],
    ['récap de party (réseau d’abord)', get('/api/occ/parties/abc123/summary'), 'live'],
    ['classement du vote (réseau d’abord)', get('/api/occ/parties/abc123/tally'), 'live'],
    ['party (réseau d’abord)', get('/api/collections/parties/records/abc123?expand=host'), 'live'],
    ['paniers (réseau d’abord)', get('/api/collections/order_items/records?filter=x'), 'live'],
    ['export (téléchargement)', get('/api/occ/parties/abc/export?format=csv'), 'none'],
    ['image de resto', get('/api/files/restaurants/abc/cover.webp?thumb=100x100'), 'image'],
    ['fichier protégé', get('/api/files/restaurants/abc/x.png?token=t'), 'none'],
    ['image distante', get('https://cdn.example.com/a.jpg', { destination: 'image' }), 'image'],
    ['script distant', get('https://cdn.example.com/a.js', { destination: 'script' }), 'none'],
    ['navigation', get('/party/abc', { mode: 'navigate' }), 'navigate'],
    ['admin PocketBase', get('/_/', { mode: 'navigate' }), 'none'],
    ['coquille', get('/assets/index-abc.js'), 'shell'],
    ['chunk paresseux', get('/assets/AdminLayout-x.js'), 'shell'],
  ])('%s', (_name, req, want) => {
    expect(classify(req, origin, pre)).toBe(want)
  })

  it('supprime les anciennes coquilles à l’activation', () => {
    expect(cacheNames('v2').shell).toBe('occ-shell-v2')
    expect(staleCaches(['occ-shell-v1', 'occ-shell-v2', 'occ-api-v1', 'occ-images-v1', 'other'], 'v2')).toEqual(['occ-shell-v1'])
  })

  it('lit la charge utile push (lien interne uniquement)', () => {
    const n = notificationFromPush(JSON.stringify({ title: 'Le vote est ouvert', body: 'Vote !', url: '/party/p1', tag: 'party-p1', renotify: true, ts: 5 }))
    expect(n.title).toBe('Le vote est ouvert')
    expect(n.options).toMatchObject({ body: 'Vote !', tag: 'party-p1', renotify: true, timestamp: 5, data: { url: '/party/p1' } })
    expect(notificationFromPush(JSON.stringify({ title: 'x', url: 'https://evil.example/' })).options.data.url).toBe('/')
    expect(notificationFromPush('pas du json').options.body).toBe('pas du json')
    expect(notificationFromPush(null).title).toBe('OCC Deliveries')
  })
})

describe('plugin de build', () => {
  it('précache JS/CSS et polices latines seulement', () => {
    const list = precacheList(['index.html', 'assets/a-1.js', 'assets/a-1.js.map', 'assets/b.css', 'assets/f-latin-wght-normal-x.woff2', 'assets/f-vietnamese-wght-normal-y.woff2', 'outils/x.js'])
    expect(list).toEqual(['/', '/assets/a-1.js', '/assets/b.css', '/assets/f-latin-wght-normal-x.woff2', '/favicon.svg', '/manifest.webmanifest', '/icons/icon-192.png', '/icons/badge-72.png', '/brand/interactive.svg', '/brand/cardor-monogram.svg'])
  })

  it('assemble un service worker versionné', () => {
    const a = buildServiceWorker(['assets/a-1.js'])
    const b = buildServiceWorker(['assets/a-2.js'])
    expect(a).toContain('const PRECACHE = ["/","/assets/a-1.js"')
    expect(a).toContain("self.addEventListener('push'")
    expect(a).not.toMatch(/^export /m)
    const version = (s: string) => /const SW_VERSION = "([a-f0-9]+)"/.exec(s)?.[1]
    expect(version(a)).toBeTruthy()
    expect(version(a)).not.toBe(version(b))
    // le SW assemblé est du JavaScript valide
    expect(() => new Function(a)).not.toThrow()
  })
})
