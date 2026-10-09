import { describe, expect, it } from 'vitest'
import { fold, highlightRanges, searchTerms } from './fold'

const marked = (text: string, terms: string[]) =>
  highlightRanges(text, terms)
    .map((r) => text.slice(r.start, r.end))
    .join('|')

describe('fold', () => {
  it('plie casse, accents et ligatures comme le serveur', () => {
    expect(fold('Râmen')).toBe('ramen')
    expect(fold('Bœuf BOURGUIGNON')).toBe('boeuf bourguignon')
    expect(fold('Poké ÉPICÉ')).toBe('poke epice')
  })

  it('découpe une saisie en termes sans doublon', () => {
    expect(searchTerms('  Poké-bowl  poke ')).toEqual(['poke', 'bowl'])
    expect(searchTerms('!!')).toEqual([])
  })
})

describe('highlightRanges', () => {
  it('surligne le début des mots, accents d’origine conservés', () => {
    expect(marked('Râmen miso', ['ram'])).toBe('Râm')
    expect(marked('Tomo Râmen', ['ramen', 'tomo'])).toBe('Tomo|Râmen')
  })

  it('ne surligne pas au milieu d’un mot', () => {
    expect(marked('Caramel', ['ram'])).toBe('')
  })

  it('gère les ligatures et fusionne les plages qui se chevauchent', () => {
    expect(marked('Œuf mollet', ['oeuf'])).toBe('Œuf')
    expect(marked('pizza', ['pi', 'pizz'])).toBe('pizz')
  })

  it('renvoie une liste vide sans terme', () => {
    expect(highlightRanges('Pizza', [])).toEqual([])
    expect(highlightRanges('', ['a'])).toEqual([])
  })
})
