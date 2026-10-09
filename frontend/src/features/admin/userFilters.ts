import type { AdminUserStatus, UserRole } from '@/lib/types'

export type UserFilter = 'all' | 'admins' | AdminUserStatus

export const USER_FILTERS: { value: UserFilter; label: string }[] = [
  { value: 'all', label: 'Tous' },
  { value: 'admins', label: 'Admins' },
  { value: 'banned', label: 'Suspendus' },
  { value: 'unverified', label: 'Non vérifiés' },
]

/** Paramètres de `GET /api/occ/admin/users` pour un filtre de l'écran. */
export function filterQuery(f: UserFilter): { role?: UserRole; status?: AdminUserStatus } {
  if (f === 'all') return {}
  if (f === 'admins') return { role: 'admin' }
  return { status: f }
}
