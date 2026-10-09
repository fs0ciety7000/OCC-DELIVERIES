/** Longueur minimale imposée par PocketBase (champ `password` de `users`). */
export const MIN_PASSWORD = 8

export type StrengthLevel = 0 | 1 | 2 | 3

export interface Strength {
  level: StrengthLevel
  label: string
  /** Conseil pour progresser (vide quand c'est solide). */
  hint: string
}

const COMMON = ['password', 'motdepasse', '12345678', '123456789', 'azertyui', 'qwertyui', 'occdeliveries', 'iloveyou']

/**
 * Indication de robustesse (purement indicative : le serveur n'impose que la longueur).
 * 0 trop court · 1 faible · 2 correct · 3 solide.
 */
export function passwordStrength(pw: string): Strength {
  if (pw.length < MIN_PASSWORD) return { level: 0, label: 'Trop court', hint: `${MIN_PASSWORD} caractères minimum.` }
  const lower = pw.toLowerCase()
  if (COMMON.some((c) => lower.includes(c)) || /^(.)\1+$/.test(pw)) {
    return { level: 1, label: 'Faible', hint: 'Évite les mots de passe trop courants.' }
  }
  const kinds = [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((r) => r.test(pw)).length
  const score = (pw.length >= 12 ? 2 : pw.length >= 10 ? 1 : 0) + (kinds >= 3 ? 2 : kinds === 2 ? 1 : 0)
  if (score >= 3) return { level: 3, label: 'Solide', hint: '' }
  if (score >= 2) return { level: 2, label: 'Correct', hint: 'Un peu plus long, ou une phrase, et ce sera parfait.' }
  return { level: 1, label: 'Faible', hint: 'Allonge-le ou mélange lettres, chiffres et symboles.' }
}

export interface PasswordPair {
  password: string
  confirm: string
}

/** Erreur bloquante du couple « nouveau mot de passe / confirmation » (null si valide). */
export function pairError(v: PasswordPair): string | null {
  if (v.password.length < MIN_PASSWORD) return `${MIN_PASSWORD} caractères minimum.`
  if (v.confirm !== v.password) return 'Les deux mots de passe ne correspondent pas.'
  return null
}
