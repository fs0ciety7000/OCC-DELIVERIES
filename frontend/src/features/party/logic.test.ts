import { describe, expect, it } from 'vitest'
import type { Restaurant, Vote } from '@/lib/types'
import { moveRanked, myRanking, ordinal, rankPoints, standingsOf, toggleRanked, withMyRanking } from './logic'

const vote = (user: string, restaurant: string, rank: number): Vote => ({ id: `${user}-${restaurant}`, party: 'p', user, restaurant, rank })

describe('vote par classement (logique client)', () => {
  it('points : même règle que le serveur (K − rang + 1, au moins 1)', () => {
    expect([1, 2, 3, 4].map((r) => rankPoints(4, r))).toEqual([4, 3, 2, 1])
    expect(rankPoints(3, 5)).toBe(1)
    expect(rankPoints(3, 0)).toBe(0)
  })

  it('mon bulletin trié par rang, ajout / retrait / déplacement bornés', () => {
    const votes = [vote('me', 'b', 2), vote('other', 'c', 1), vote('me', 'a', 1)]
    expect(myRanking(votes, 'me')).toEqual(['a', 'b'])
    expect(toggleRanked(['a', 'b'], 'c')).toEqual(['a', 'b', 'c'])
    expect(toggleRanked(['a', 'b', 'c'], 'a')).toEqual(['b', 'c'])
    expect(moveRanked(['a', 'b', 'c'], 'c', -1)).toEqual(['a', 'c', 'b'])
    expect(moveRanked(['a', 'b', 'c'], 'a', 1)).toEqual(['b', 'a', 'c'])
    const same = ['a', 'b']
    expect(moveRanked(same, 'a', -1)).toBe(same)
    expect(moveRanked(same, 'z', 1)).toBe(same)
  })

  it('votes optimistes : seul mon bulletin change', () => {
    const next = withMyRanking([vote('me', 'a', 1), vote('other', 'a', 1)], 'p', 'me', ['b', 'a'])
    expect(next.filter((v) => v.user === 'other')).toHaveLength(1)
    expect(myRanking(next, 'me')).toEqual(['b', 'a'])
  })

  it('classement du serveur joint aux restaurants, candidats manquants en fin', () => {
    const r = (id: string) => ({ id, name: id.toUpperCase() }) as Restaurant
    const out = standingsOf([r('a'), r('b'), r('c')], {
      candidates: 3,
      voters: 1,
      winner: 'b',
      standings: [
        { restaurant: 'b', points: 3, firstChoices: 1, voters: 1 },
        { restaurant: 'x', points: 2, firstChoices: 0, voters: 1 },
        { restaurant: 'a', points: 2, firstChoices: 0, voters: 1 },
      ],
    })
    expect(out.map((s) => [s.restaurant, s.points])).toEqual([['b', 3], ['a', 2], ['c', 0]])
    expect(standingsOf([r('a')], undefined)[0]!.points).toBe(0)
    expect([1, 2, 3].map(ordinal)).toEqual(['1er', '2e', '3e'])
  })
})
