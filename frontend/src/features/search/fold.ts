/*
 * Pliage identique au serveur (backend/internal/search/fold.go) : minuscules, accents
 * et ligatures retirés. Sert à surligner les termes renvoyés par `/api/occ/search`.
 */
const LIGATURES: Record<string, string> = { œ: 'oe', Œ: 'oe', æ: 'ae', Æ: 'ae', ß: 'ss', ﬁ: 'fi', ﬂ: 'fl' }

export function foldChar(ch: string): string {
  const lig = LIGATURES[ch]
  if (lig) return lig
  return ch.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase()
}

export function fold(s: string): string {
  let out = ''
  for (const ch of s) out += foldChar(ch)
  return out
}

const isWord = (ch: string) => /[\p{L}\p{N}]/u.test(ch)

/** Termes d'une saisie (lettres et chiffres, pliés, sans doublon). */
export function searchTerms(q: string): string[] {
  const out: string[] = []
  for (const w of fold(q).split(/[^\p{L}\p{N}]+/u)) if (w && !out.includes(w)) out.push(w)
  return out
}

export interface Range {
  start: number
  end: number
}

/**
 * Plages (indices UTF-16 dans `text`, accents d'origine) des mots qui commencent par un
 * des termes : « Râmen » est surligné pour le terme « ram ».
 */
export function highlightRanges(text: string, terms: string[]): Range[] {
  const clean = terms.filter(Boolean)
  if (!text || clean.length === 0) return []
  // texte plié + position d'origine de chaque unité pliée
  let folded = ''
  const origin: number[] = []
  let i = 0
  for (const ch of text) {
    const f = foldChar(ch)
    for (let k = 0; k < f.length; k++) origin.push(i)
    folded += f
    i += ch.length
  }
  origin.push(text.length)
  const ranges: Range[] = []
  for (const term of clean) {
    let from = 0
    for (;;) {
      const at = folded.indexOf(term, from)
      if (at < 0) break
      from = at + 1
      if (at > 0 && isWord(folded[at - 1] ?? ' ')) continue
      const start = origin[at] ?? 0
      // fin : prolonge jusqu'à la fin de l'unité d'origine couverte
      const endFolded = at + term.length
      let end = origin[endFolded] ?? text.length
      if (end === start) end = start + 1
      ranges.push({ start, end })
    }
  }
  ranges.sort((a, b) => a.start - b.start || b.end - a.end)
  const merged: Range[] = []
  for (const r of ranges) {
    const last = merged[merged.length - 1]
    if (last && r.start <= last.end) last.end = Math.max(last.end, r.end)
    else merged.push({ ...r })
  }
  return merged
}
