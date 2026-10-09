import PocketBase from 'pocketbase'

/** Client PocketBase : même origine (le binaire Go sert l'API et la SPA). */
export const pb = new PocketBase(typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8090')
pb.autoCancellation(false)

/** URL publique d'un fichier PocketBase (avatar, cover…), sans dépendre de `collectionId`. */
export function fileUrl(collection: string, recordId: string, filename?: string | null, thumb?: string): string | undefined {
  if (!filename) return undefined
  const base = pb.buildURL(`/api/files/${encodeURIComponent(collection)}/${encodeURIComponent(recordId)}/${encodeURIComponent(filename)}`)
  return thumb ? `${base}?thumb=${encodeURIComponent(thumb)}` : base
}
