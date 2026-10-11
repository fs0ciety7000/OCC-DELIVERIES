import { ClientResponseError } from 'pocketbase'

/** Message d'erreur lisible (français) à partir d'une erreur PocketBase ou réseau. */
export function errorMessage(err: unknown, fallback = 'Oups, quelque chose a coincé. Réessaie dans un instant.'): string {
  if (err instanceof ClientResponseError) {
    if (err.isAbort) return 'Requête annulée.'
    if (err.status === 0) return 'Connexion impossible au serveur. Vérifie ta connexion.'
    const data = err.response?.data as Record<string, { message?: string }> | undefined
    const fieldMsg = data && Object.values(data).find((v) => v && typeof v.message === 'string')?.message
    const msg = (err.response?.message as string | undefined) || fieldMsg
    if (err.status === 401) return msg || 'Connecte-toi pour continuer.'
    if (err.status === 403) return msg || "Tu n'as pas les droits pour faire ça."
    if (err.status === 404) return msg || 'Introuvable.'
    if (err.status === 429) return msg || 'Trop de tentatives, réessaie dans quelques minutes.'
    return msg || fallback
  }
  if (err instanceof Error && err.message) return err.message
  return fallback
}

/** Limite de tentatives atteinte (429) : le serveur dit dans combien de minutes réessayer. */
export function isRateLimited(err: unknown): boolean {
  return err instanceof ClientResponseError && err.status === 429
}

export function isNotFound(err: unknown): boolean {
  return err instanceof ClientResponseError && err.status === 404
}

/** Le serveur a refusé ce champ (erreur de validation PocketBase `data.<field>`). */
export function hasFieldError(err: unknown, field: string): boolean {
  if (!(err instanceof ClientResponseError)) return false
  const data = err.response?.data as Record<string, unknown> | undefined
  return !!data && field in data
}
