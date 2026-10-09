import { describe, expect, it } from 'vitest'
import { parseEuros } from '@/lib/euros'
import { CSV_COLUMNS, CSV_OPTIONAL_COLUMNS, csvTemplate, detectImportKind, JSON_EXAMPLE, parseImportJson } from './importFormat'

describe('csvTemplate', () => {
  const csv = csvTemplate()
  const lines = csv.replace(/^\ufeff/, '').trim().split('\r\n')

  it('commence par le BOM et l’en-tête attendu (séparateur ;)', () => {
    expect(csv.startsWith('\ufeff')).toBe(true)
    expect(lines[0]).toBe([...CSV_COLUMNS, ...CSV_OPTIONAL_COLUMNS].join(';'))
  })

  it('contient des lignes d’exemple avec des décimales françaises valides', () => {
    expect(lines).toHaveLength(3)
    const header = lines[0]!.split(';')
    const row = lines[1]!.match(/("([^"]|"")*"|[^;]*)(;|$)/g)!.map((c) => c.replace(/;$/, '').replace(/^"|"$/g, ''))
    const price = row[header.indexOf('price_eur')]!
    expect(price).toBe('12,50')
    expect(parseEuros(price)).toBe(1250)
    // la description contient « ; » : elle est donc citée
    expect(lines[1]).toContain('"Tomate; mozzarella; basilic"')
    expect(row[header.indexOf('lat')]).toBe('50,4542')
  })

  it('accepte un autre séparateur', () => {
    expect(csvTemplate(',').split('\r\n')[0]).toContain('restaurant_slug,restaurant_name')
  })
})

describe('détection et lecture', () => {
  it('reconnaît le format', () => {
    expect(detectImportKind('menu.CSV', '')).toBe('csv')
    expect(detectImportKind('x.json', '')).toBe('json')
    expect(detectImportKind('sans-extension', '  [{"slug":"a"}]')).toBe('json')
    expect(detectImportKind('sans-extension', '\ufeffrestaurant_slug;category')).toBe('csv')
    expect(detectImportKind('image.png', '\u0089PNG')).toBeNull()
  })

  it('lit un objet, un tableau ou { restaurants }', () => {
    const one = JSON.stringify(JSON_EXAMPLE[0])
    expect(Array.isArray(parseImportJson(one))).toBe(false)
    expect(parseImportJson(JSON.stringify(JSON_EXAMPLE))).toHaveLength(1)
    expect(parseImportJson(JSON.stringify({ restaurants: JSON_EXAMPLE }))).toHaveLength(1)
    expect(() => parseImportJson('42')).toThrow()
    expect(() => parseImportJson('{oops')).toThrow()
  })
})
