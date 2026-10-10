import { useCallback, useEffect, useRef, useState } from 'react'
import { useConfig } from '@/lib/geo-context'
import { errorMessage } from '@/lib/errors'
import { loginWithPasskey, passkeysApi, PrefetchedOptions, type RequestBegin } from '@/lib/passkeys'
import { conditionalMediationAvailable, isWebAuthnCancel, passkeysSupported, webAuthnErrorMessage } from '@/lib/webauthn'

export interface PasskeyLoginState {
  /** Bouton « Se connecter avec une passkey » affiché. */
  available: boolean
  /** Autoremplissage actif : le champ e-mail doit porter `autocomplete="username webauthn"`. */
  conditional: boolean
  busy: boolean
  error: string | null
  /** À appeler **dans** le clic (la fenêtre du navigateur s'ouvre sans attente réseau). */
  start: () => void
}

/**
 * Connexion par passkey sur la page de connexion : bouton (fenêtre du navigateur)
 * et autoremplissage du champ e-mail (`mediation: 'conditional'`) quand le
 * navigateur le permet. `onSuccess` est appelé une fois la session enregistrée.
 */
export function usePasskeyLogin(onSuccess: () => void, active = true): PasskeyLoginState {
  const supported = passkeysSupported()
  const config = useConfig()
  const enabled = active && supported && config.data?.passkeys === true
  const [conditional, setConditional] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [prefetch] = useState(() => new PrefetchedOptions<RequestBegin>(() => passkeysApi.loginBegin()))
  const conditionalAbort = useRef<AbortController | null>(null)
  const mounted = useRef(true)
  const successRef = useRef(onSuccess)
  useEffect(() => {
    successRef.current = onSuccess
  }, [onSuccess])

  const fail = useCallback((e: unknown) => {
    if (!mounted.current || isWebAuthnCancel(e)) return
    setError(webAuthnErrorMessage(e) ?? errorMessage(e, 'La connexion avec la passkey a échoué.'))
  }, [])

  // Autoremplissage : une demande « conditionnelle » attend que l'utilisateur choisisse une passkey dans le champ.
  const startConditional = useCallback(() => {
    if (!mounted.current) return
    conditionalAbort.current?.abort()
    const ctrl = new AbortController()
    conditionalAbort.current = ctrl
    loginWithPasskey(passkeysApi.loginBegin(true), { conditional: true, signal: ctrl.signal })
      .then(() => {
        if (mounted.current) successRef.current()
      })
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) fail(e)
      })
  }, [fail])

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  useEffect(() => {
    if (!enabled) return
    prefetch.warm()
    let cancelled = false
    void conditionalMediationAvailable().then((ok) => {
      if (cancelled || !ok) return
      setConditional(true)
      startConditional()
    })
    return () => {
      cancelled = true
      conditionalAbort.current?.abort()
      conditionalAbort.current = null
    }
  }, [enabled, prefetch, startConditional])

  const start = () => {
    setError(null)
    setBusy(true)
    // une seule demande WebAuthn à la fois : on suspend l'autoremplissage
    const hadConditional = !!conditionalAbort.current
    conditionalAbort.current?.abort()
    conditionalAbort.current = null
    loginWithPasskey(prefetch.take())
      .then(() => {
        if (mounted.current) successRef.current()
      })
      .catch((e: unknown) => {
        fail(e)
        prefetch.warm()
        if (hadConditional) startConditional()
      })
      .finally(() => {
        if (mounted.current) setBusy(false)
      })
  }

  return { available: enabled, conditional: enabled && conditional, busy, error, start }
}
