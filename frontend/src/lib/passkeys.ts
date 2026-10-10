import type { RecordModel } from 'pocketbase'
import { pb } from './pb'
import type { PasskeyView, User } from './types'
import { credentialToJSON, parseCreationOptions, parseRequestOptions, type CreationOptionsJSON, type RequestOptionsJSON } from './webauthn'

/**
 * Passkeys : `/api/occ/passkeys*` (docs/ARCHITECTURE.md §5). Les cérémonies se font
 * en deux temps : *begin* (options du serveur) puis `navigator.credentials.*` puis
 * *finish*. Les options peuvent être chargées **avant** le clic (`prefetch…`) pour que
 * `navigator.credentials.create/get` soit appelé directement dans le geste
 * (Safari refuse sinon) ; un défi ne sert qu'une fois.
 */

const json = (body: unknown) => ({ method: 'POST', body: JSON.stringify(body), headers: { 'Content-Type': 'application/json' } })

export type CreationBegin = { publicKey: CreationOptionsJSON }
export type RequestBegin = { publicKey: RequestOptionsJSON; mediation?: string }

export const passkeysApi = {
  list: () => pb.send<PasskeyView[]>('/api/occ/passkeys', { method: 'GET' }),
  rename: (id: string, name: string) =>
    pb.send<PasskeyView>(`/api/occ/passkeys/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ name }), headers: { 'Content-Type': 'application/json' } }),
  remove: (id: string) => pb.send<void>(`/api/occ/passkeys/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  registerBegin: (name?: string) => pb.send<CreationBegin>('/api/occ/passkeys/register/begin', json(name ? { name } : {})),
  registerFinish: (credential: unknown, name?: string) => pb.send<PasskeyView>('/api/occ/passkeys/register/finish', json(name ? { credential, name } : { credential })),
  loginBegin: (conditional = false) => pb.send<RequestBegin>('/api/occ/passkeys/login/begin', json(conditional ? { conditional } : {})),
  loginFinish: (credential: unknown) => pb.send<{ token: string; record: User }>('/api/occ/passkeys/login/finish', json({ credential })),
}

/**
 * Ajoute une passkey. `begin` = options déjà chargées (recommandé : appel de
 * `navigator.credentials.create` sans attente réseau dans le clic) ou une promesse.
 */
export async function registerPasskey(begin: CreationBegin | Promise<CreationBegin>, name?: string): Promise<PasskeyView> {
  const opts = begin instanceof Promise ? await begin : begin
  const cred = (await navigator.credentials.create({ publicKey: parseCreationOptions(opts.publicKey) })) as PublicKeyCredential | null
  if (!cred) throw new DOMException('Aucune passkey créée.', 'NotAllowedError')
  return passkeysApi.registerFinish(credentialToJSON(cred), name)
}

/**
 * Se connecte avec une passkey (fenêtre du navigateur, ou autoremplissage avec
 * `conditional`). En cas de succès, la session PocketBase est enregistrée.
 */
export async function loginWithPasskey(begin: RequestBegin | Promise<RequestBegin>, opts: { conditional?: boolean; signal?: AbortSignal } = {}): Promise<User> {
  const b = begin instanceof Promise ? await begin : begin
  const cred = (await navigator.credentials.get({
    publicKey: parseRequestOptions(b.publicKey),
    ...(opts.conditional ? { mediation: 'conditional' as CredentialMediationRequirement } : {}),
    signal: opts.signal,
  })) as PublicKeyCredential | null
  if (!cred) throw new DOMException('Aucune passkey choisie.', 'NotAllowedError')
  const res = await passkeysApi.loginFinish(credentialToJSON(cred))
  pb.authStore.save(res.token, res.record as unknown as RecordModel)
  return res.record
}

/**
 * Options *begin* chargées à l'avance et renouvelées avant l'expiration du défi
 * (10 min côté serveur) : `take()` les consomme (usage unique) ; rappeler `warm()` ensuite.
 */
export class PrefetchedOptions<T> {
  private current: { value: T; at: number } | null = null
  private pending: Promise<T> | null = null
  private readonly fetcher: () => Promise<T>
  private readonly maxAgeMs: number

  constructor(fetcher: () => Promise<T>, maxAgeMs = 8 * 60_000) {
    this.fetcher = fetcher
    this.maxAgeMs = maxAgeMs
  }

  /** Charge en arrière-plan (erreurs ignorées : `take()` réessaiera). */
  warm(): void {
    if (this.fresh() || this.pending) return
    const p = this.fetcher()
    this.pending = p
    p.then(
      (value) => {
        // pas déjà remise à un `take()` pendant le chargement
        if (this.pending === p) {
          this.current = { value, at: Date.now() }
          this.pending = null
        }
      },
      () => {
        if (this.pending === p) this.pending = null
      },
    )
  }

  /** Options prêtes (synchrone, pour le geste) ou, à défaut, une promesse. Chaque valeur n'est donnée qu'une fois. */
  take(): T | Promise<T> {
    const c = this.fresh() ? this.current : null
    this.current = null
    if (c) return c.value
    const p = this.pending ?? this.fetcher()
    this.pending = null
    return p
  }

  private fresh(): boolean {
    return !!this.current && Date.now() - this.current.at < this.maxAgeMs
  }
}
