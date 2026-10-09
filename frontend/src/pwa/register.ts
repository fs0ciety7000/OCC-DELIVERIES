/**
 * Enregistrement du service worker (build de production uniquement : dist/sw.js est
 * généré par pwa/plugin.ts) et capture de l'invite d'installation.
 */
type Listener = () => void

interface BeforeInstallPromptEvent extends Event {
  prompt(): Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

let deferredPrompt: BeforeInstallPromptEvent | null = null
let installed = false
const installListeners = new Set<Listener>()
const emitInstall = () => installListeners.forEach((l) => l())

let openUrlHandler: ((url: string) => void) | null = null

/** Navigation demandée par le service worker (clic sur une notification, onglet déjà ouvert). */
export function onOpenUrl(handler: ((url: string) => void) | null) {
  openUrlHandler = handler
}

export function registerServiceWorker(): void {
  if (typeof window === 'undefined') return
  window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault()
    deferredPrompt = e as BeforeInstallPromptEvent
    emitInstall()
  })
  window.addEventListener('appinstalled', () => {
    installed = true
    deferredPrompt = null
    emitInstall()
  })
  if (!('serviceWorker' in navigator) || !import.meta.env.PROD) return
  navigator.serviceWorker.addEventListener('message', (event: MessageEvent<{ type?: string; url?: string }>) => {
    if (event.data?.type === 'OPEN_URL' && typeof event.data.url === 'string' && event.data.url.startsWith('/')) {
      if (openUrlHandler) openUrlHandler(event.data.url)
      else window.location.assign(event.data.url)
    }
  })
  const register = () => {
    navigator.serviceWorker.register('/sw.js', { scope: '/' }).catch(() => undefined)
  }
  if (document.readyState === 'complete') register()
  else window.addEventListener('load', register, { once: true })
}

/** Déconnexion : vide le cache des lectures personnelles du service worker. */
export function clearUserCache(): void {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return
  navigator.serviceWorker.controller?.postMessage({ type: 'CLEAR_USER_DATA' })
}

export function canPromptInstall(): boolean {
  return deferredPrompt !== null
}

export function wasInstalled(): boolean {
  return installed
}

export function subscribeInstall(cb: Listener): () => void {
  installListeners.add(cb)
  return () => void installListeners.delete(cb)
}

/** Affiche l'invite native (Chrome / Edge / Android). `true` si acceptée. */
export async function promptInstall(): Promise<boolean> {
  const e = deferredPrompt
  if (!e) return false
  deferredPrompt = null
  emitInstall()
  await e.prompt()
  const choice = await e.userChoice
  return choice.outcome === 'accepted'
}
