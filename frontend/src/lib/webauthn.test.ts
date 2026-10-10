import { afterEach, describe, expect, it, vi } from 'vitest'
import { PrefetchedOptions } from './passkeys'
import {
  base64urlToBuffer,
  bufferToBase64url,
  credentialToJSON,
  parseCreationOptions,
  parseRequestOptions,
  passkeysSupported,
  webAuthnErrorMessage,
} from './webauthn'

const bytes = (b: ArrayBuffer) => Array.from(new Uint8Array(b))

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('base64url', () => {
  it.each([[[]], [[0]], [[0xfb, 0xff]], [[1, 2, 3]], [[0xff, 0xfe, 0xfd, 0xfc]]])('aller-retour %j', (input) => {
    const enc = bufferToBase64url(new Uint8Array(input))
    expect(enc).not.toMatch(/[+/=]/)
    expect(bytes(base64urlToBuffer(enc))).toEqual(input)
  })

  it('encode comme le serveur (RawURLEncoding)', () => {
    expect(bufferToBase64url(new Uint8Array([0xfb, 0xff, 0xbf]))).toBe('-_-_')
    expect(bufferToBase64url(new TextEncoder().encode('u1'))).toBe('dTE')
  })
})

describe('options', () => {
  it('convertit les options de création sans parseCreationOptionsFromJSON', () => {
    vi.stubGlobal('PublicKeyCredential', function PublicKeyCredential() {})
    const o = parseCreationOptions({
      challenge: 'AQID',
      rp: { id: 'localhost', name: 'OCC Deliveries' },
      user: { id: 'dTE', name: 'ana@occ.be', displayName: 'Ana' },
      excludeCredentials: [{ id: 'BAU', type: 'public-key', transports: ['internal'] }],
    })
    expect(bytes(o.challenge as ArrayBuffer)).toEqual([1, 2, 3])
    expect(bytes(o.user.id as ArrayBuffer)).toEqual([0x75, 0x31])
    expect(o.user.name).toBe('ana@occ.be')
    expect(o.rp.id).toBe('localhost')
    expect(bytes(o.excludeCredentials![0]!.id as ArrayBuffer)).toEqual([4, 5])
  })

  it('utilise la méthode native quand elle existe', () => {
    const native = vi.fn(() => ({ native: true }))
    vi.stubGlobal('PublicKeyCredential', Object.assign(function PublicKeyCredential() {}, { parseRequestOptionsFromJSON: native }))
    expect(parseRequestOptions({ challenge: 'AQID' })).toEqual({ native: true })
    expect(native).toHaveBeenCalledWith({ challenge: 'AQID' })
  })

  it('options de connexion sans allowCredentials (usernameless)', () => {
    vi.stubGlobal('PublicKeyCredential', function PublicKeyCredential() {})
    const o = parseRequestOptions({ challenge: 'AQID', rpId: 'localhost', userVerification: 'required' })
    expect(bytes(o.challenge as ArrayBuffer)).toEqual([1, 2, 3])
    expect(o.allowCredentials).toBeUndefined()
    expect(o.userVerification).toBe('required')
  })
})

describe('credentialToJSON', () => {
  const buf = (...b: number[]) => new Uint8Array(b).buffer

  it('enregistrement (sans toJSON natif)', () => {
    const cred = {
      id: 'AQ',
      rawId: buf(1),
      type: 'public-key',
      authenticatorAttachment: 'platform',
      response: { clientDataJSON: buf(2), attestationObject: buf(3), getTransports: () => ['internal', 'hybrid'] },
      getClientExtensionResults: () => ({ credProps: { rk: true } }),
    } as unknown as PublicKeyCredential
    expect(credentialToJSON(cred)).toEqual({
      id: 'AQ',
      rawId: 'AQ',
      type: 'public-key',
      authenticatorAttachment: 'platform',
      response: { clientDataJSON: 'Ag', attestationObject: 'Aw', transports: ['internal', 'hybrid'] },
      clientExtensionResults: { credProps: { rk: true } },
    })
  })

  it('connexion (sans toJSON natif)', () => {
    const cred = {
      id: 'AQ',
      rawId: buf(1),
      type: 'public-key',
      authenticatorAttachment: null,
      response: { clientDataJSON: buf(2), authenticatorData: buf(3), signature: buf(4), userHandle: buf(0x75, 0x31) },
      getClientExtensionResults: () => ({}),
    } as unknown as PublicKeyCredential
    expect(credentialToJSON(cred)).toMatchObject({
      rawId: 'AQ',
      response: { clientDataJSON: 'Ag', authenticatorData: 'Aw', signature: 'BA', userHandle: 'dTE' },
    })
  })

  it('préfère toJSON natif', () => {
    const cred = { toJSON: () => ({ id: 'native' }) } as unknown as PublicKeyCredential
    expect(credentialToJSON(cred)).toEqual({ id: 'native' })
  })
})

describe('détection et erreurs', () => {
  it('non pris en charge sans PublicKeyCredential', () => {
    vi.stubGlobal('PublicKeyCredential', undefined)
    expect(passkeysSupported()).toBe(false)
  })

  it('messages français, rien pour une annulation', () => {
    expect(webAuthnErrorMessage(new DOMException('x', 'NotAllowedError'))).toBeNull()
    expect(webAuthnErrorMessage(new DOMException('x', 'AbortError'))).toBeNull()
    expect(webAuthnErrorMessage(new DOMException('x', 'InvalidStateError'))).toMatch(/déjà enregistrée/)
    expect(webAuthnErrorMessage(new Error('x'))).toBeNull()
  })
})

describe('PrefetchedOptions', () => {
  it('donne les options prêtes de façon synchrone, une seule fois, et recharge les options périmées', async () => {
    vi.useFakeTimers()
    let n = 0
    const p = new PrefetchedOptions(async () => ++n, 1000)
    p.warm()
    await vi.waitFor(() => expect(n).toBe(1))
    await Promise.resolve()
    expect(p.take()).toBe(1) // synchrone : pas de promesse
    const next = p.take() // consommées → nouvel appel
    expect(next).toBeInstanceOf(Promise)
    expect(await next).toBe(2)
    p.warm()
    await vi.waitFor(() => expect(n).toBe(3))
    await Promise.resolve()
    vi.advanceTimersByTime(1500) // périmées
    expect(await p.take()).toBe(4)
  })
})
