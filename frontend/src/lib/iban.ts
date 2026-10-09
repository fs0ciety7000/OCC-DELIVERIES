/** Normalise un IBAN : sans espaces, majuscules. */
export function normalizeIban(input: string): string {
  return input.replace(/[\s-]/g, '').toUpperCase()
}

/** Groupe par 4 pour l'affichage : BE71 0961 2345 6769. */
export function formatIban(input: string): string {
  return normalizeIban(input).replace(/(.{4})/g, '$1 ').trim()
}

/** Validation mod-97 (ISO 13616). Vide → considéré valide (champ optionnel). */
export function isValidIban(input: string): boolean {
  const iban = normalizeIban(input)
  if (!/^[A-Z]{2}\d{2}[A-Z0-9]{10,30}$/.test(iban)) return false
  const rearranged = iban.slice(4) + iban.slice(0, 4)
  let remainder = 0
  for (const ch of rearranged) {
    const code = ch.charCodeAt(0)
    const digits = code >= 65 ? String(code - 55) : ch
    for (const d of digits) remainder = (remainder * 10 + Number(d)) % 97
  }
  return remainder === 1
}

export function isValidBic(input: string): boolean {
  const bic = input.replace(/\s/g, '').toUpperCase()
  return bic === '' || /^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$/.test(bic)
}
