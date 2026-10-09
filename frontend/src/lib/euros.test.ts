import { describe, expect, it } from 'vitest'
import { centsToEuros, parseDecimal, parseEuros } from './euros'

describe('parseEuros', () => {
  it.each([
    ['12,50', 1250],
    ['12.50', 1250],
    ['12,5', 1250],
    ['12', 1200],
    ['0,99', 99],
    [' 12,50 € ', 1250],
    ['€12.50', 1250],
    ['12€50', 1250],
    ['1 234,50', 123450],
    ['1.234,50', 123450],
    ['1,234.50', 123450],
    ['1\u00a0234,5', 123450],
  ])('%s → %i', (input, cents) => {
    expect(parseEuros(input)).toBe(cents)
  })

  it.each(['', 'abc', '-1', '12,505', '1,2,3', '999999'])('rejette « %s »', (input) => {
    expect(parseEuros(input)).toBeNull()
  })
})

describe('centsToEuros', () => {
  it('formate pour un champ de saisie', () => {
    expect(centsToEuros(1250)).toBe('12,50')
    expect(centsToEuros(5)).toBe('0,05')
    expect(centsToEuros(0)).toBe('0,00')
    expect(centsToEuros(undefined)).toBe('0,00')
  })
  it('aller-retour', () => {
    for (const c of [0, 1, 99, 100, 1250, 123456]) expect(parseEuros(centsToEuros(c))).toBe(c)
  })
})

describe('parseDecimal', () => {
  it('accepte virgule et point', () => {
    expect(parseDecimal('50,4542')).toBe(50.4542)
    expect(parseDecimal('3.9567')).toBe(3.9567)
    expect(parseDecimal('-0,5')).toBe(-0.5)
    expect(parseDecimal('nord')).toBeNull()
  })
})
