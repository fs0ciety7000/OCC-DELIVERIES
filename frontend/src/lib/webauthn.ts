/**
 * Passkeys (WebAuthn) côté navigateur : détection, base64url et conversion
 * JSON ⇄ options / credentials. Utilise les méthodes natives
 * (`parseCreationOptionsFromJSON`, `toJSON`) quand le navigateur les a, sinon
 * de petits convertisseurs maison (aucune dépendance).
 */

/* ---------------------------------------------------------------- base64url */

export function bufferToBase64url(buf: ArrayBuffer | ArrayBufferView): string {
  const bytes = buf instanceof ArrayBuffer ? new Uint8Array(buf) : new Uint8Array(buf.buffer, buf.byteOffset, buf.byteLength)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function base64urlToBuffer(s: string): ArrayBuffer {
  const b64 = s.replace(/-/g, '+').replace(/_/g, '/')
  const bin = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4))
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out.buffer
}

/* ---------------------------------------------------------------- détection */

type PKC = typeof PublicKeyCredential & {
  parseCreationOptionsFromJSON?: (o: unknown) => PublicKeyCredentialCreationOptions
  parseRequestOptionsFromJSON?: (o: unknown) => PublicKeyCredentialRequestOptions
  isConditionalMediationAvailable?: () => Promise<boolean>
}

function pkc(): PKC | undefined {
  return typeof window !== 'undefined' && typeof window.PublicKeyCredential === 'function' ? (window.PublicKeyCredential as PKC) : undefined
}

/** Le navigateur sait créer / utiliser des passkeys (contexte sécurisé + API WebAuthn). */
export function passkeysSupported(): boolean {
  return !!pkc() && typeof navigator !== 'undefined' && !!navigator.credentials && typeof navigator.credentials.create === 'function'
}

/** Autoremplissage des passkeys dans le champ e-mail (`mediation: 'conditional'`). */
export async function conditionalMediationAvailable(): Promise<boolean> {
  const P = pkc()
  if (!P?.isConditionalMediationAvailable) return false
  try {
    return await P.isConditionalMediationAvailable()
  } catch {
    return false
  }
}

/* ------------------------------------------------------------------ options */

/** Options JSON telles que renvoyées par le serveur (`publicKey`). */
export type CreationOptionsJSON = {
  challenge: string
  user: { id: string; name: string; displayName: string }
  excludeCredentials?: { id: string; type: string; transports?: string[] }[]
  [key: string]: unknown
}

export type RequestOptionsJSON = {
  challenge: string
  allowCredentials?: { id: string; type: string; transports?: string[] }[]
  [key: string]: unknown
}

const descriptors = (list?: { id: string; type: string; transports?: string[] }[]) =>
  list?.map((c) => ({ ...c, id: base64urlToBuffer(c.id), type: 'public-key' as const, transports: c.transports as AuthenticatorTransport[] | undefined }))

export function parseCreationOptions(json: CreationOptionsJSON): PublicKeyCredentialCreationOptions {
  const P = pkc()
  if (P?.parseCreationOptionsFromJSON) return P.parseCreationOptionsFromJSON(json)
  return {
    ...(json as unknown as PublicKeyCredentialCreationOptions),
    challenge: base64urlToBuffer(json.challenge),
    user: { ...json.user, id: base64urlToBuffer(json.user.id) },
    excludeCredentials: descriptors(json.excludeCredentials),
  }
}

export function parseRequestOptions(json: RequestOptionsJSON): PublicKeyCredentialRequestOptions {
  const P = pkc()
  if (P?.parseRequestOptionsFromJSON) return P.parseRequestOptionsFromJSON(json)
  return {
    ...(json as unknown as PublicKeyCredentialRequestOptions),
    challenge: base64urlToBuffer(json.challenge),
    allowCredentials: descriptors(json.allowCredentials),
  }
}

/* -------------------------------------------------------------- credentials */

type MaybeJSON = PublicKeyCredential & { toJSON?: () => unknown }

/** `PublicKeyCredential` → JSON (format `toJSON()` du W3C) envoyé au serveur. */
export function credentialToJSON(cred: PublicKeyCredential): unknown {
  const c = cred as MaybeJSON
  if (typeof c.toJSON === 'function') {
    try {
      return c.toJSON()
    } catch {
      /* certains gestionnaires de mots de passe cassent toJSON : conversion maison */
    }
  }
  const r = cred.response as AuthenticatorResponse & Partial<AuthenticatorAttestationResponse & AuthenticatorAssertionResponse>
  const response: Record<string, unknown> = { clientDataJSON: bufferToBase64url(r.clientDataJSON) }
  if (r.attestationObject) {
    response.attestationObject = bufferToBase64url(r.attestationObject)
    if (typeof r.getTransports === 'function') response.transports = r.getTransports()
  }
  if (r.authenticatorData) response.authenticatorData = bufferToBase64url(r.authenticatorData)
  if (r.signature) response.signature = bufferToBase64url(r.signature)
  if (r.userHandle) response.userHandle = bufferToBase64url(r.userHandle)
  return {
    id: cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
    response,
    clientExtensionResults: cred.getClientExtensionResults?.() ?? {},
  }
}

/* ------------------------------------------------------------------ erreurs */

/** L'utilisateur a fermé la fenêtre / annulé (ou la demande a été interrompue) : pas un vrai échec. */
export function isWebAuthnCancel(err: unknown): boolean {
  return err instanceof DOMException && (err.name === 'NotAllowedError' || err.name === 'AbortError')
}

/** Message français pour une erreur du navigateur (null pour une annulation). */
export function webAuthnErrorMessage(err: unknown): string | null {
  if (isWebAuthnCancel(err)) return null
  if (err instanceof DOMException) {
    if (err.name === 'InvalidStateError') return 'Cette passkey est déjà enregistrée sur cet appareil.'
    if (err.name === 'SecurityError') return 'Les passkeys ne fonctionnent que sur l’adresse officielle du site (https).'
    if (err.name === 'NotSupportedError') return 'Cet appareil ne sait pas créer de passkey.'
  }
  return null
}
