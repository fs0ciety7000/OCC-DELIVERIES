/** Libellé affiché pour un fournisseur OAuth2 (« Google »). */
export function providerLabel(name: string, displayName?: string): string {
  if (name === 'google') return 'Google'
  return displayName || name
}
