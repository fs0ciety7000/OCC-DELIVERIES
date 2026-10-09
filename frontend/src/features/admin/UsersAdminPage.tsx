import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Search, ShieldCheck, ShieldOff } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Avatar, Badge, Button, Card, EmptyState, Input, Segmented, Skeleton } from '@/components/ui'
import { adminApi } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'
import { formatRelativeTime, plural } from '@/lib/format'
import { useDebounced } from '@/lib/hooks'
import { qk } from '@/lib/queryKeys'
import type { AdminUser, UserRole } from '@/lib/types'
import { AdminHeader } from './AdminLayout'

type RoleFilter = 'all' | UserRole

export function UsersAdminPage() {
  const { user: me } = useAuth()
  const [q, setQ] = useState('')
  const [role, setRole] = useState<RoleFilter>('all')
  const query = useDebounced(q.trim(), 250)
  const qc = useQueryClient()
  const list = useQuery({
    queryKey: qk.admin.users(query, role),
    queryFn: () => adminApi.users(query, 1, role === 'all' ? undefined : role),
    placeholderData: keepPreviousData,
  })
  const setUserRole = useMutation({
    mutationFn: ({ u, next }: { u: AdminUser; next: UserRole }) => adminApi.setRole(u.id, next),
    onSuccess: ({ user }) => {
      toast.success(user.role === 'admin' ? `${user.name || user.email} est maintenant admin` : `${user.name || user.email} n'est plus admin`)
      void qc.invalidateQueries({ queryKey: qk.admin.all })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <div>
      <AdminHeader title="Utilisateurs" description={list.data ? plural(list.data.totalItems, 'compte') : undefined} />
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="min-w-0 flex-1">
          <Input type="search" aria-label="Rechercher un utilisateur" placeholder="Nom ou e-mail…" value={q} onChange={(e) => setQ(e.target.value)} leftIcon={<Search className="size-4" />} />
        </div>
        <Segmented<RoleFilter>
          label="Filtrer par rôle"
          value={role}
          onChange={setRole}
          options={[
            { value: 'all', label: 'Tous' },
            { value: 'admin', label: 'Admins' },
            { value: 'user', label: 'Membres' },
          ]}
        />
      </div>
      {list.isPending && <Skeleton className="h-64 rounded-lg" />}
      {list.isError && <EmptyState tone="danger" emoji="⚠️" title="Chargement impossible" description={errorMessage(list.error)} />}
      {list.data?.items.length === 0 && <EmptyState title="Personne" description="Aucun compte ne correspond." />}
      <ul className="space-y-2">
        {list.data?.items.map((u) => {
          const isMe = u.id === me?.id
          const admin = u.role === 'admin'
          return (
            <li key={u.id}>
              <Card className="flex flex-wrap items-center gap-3 p-3 sm:flex-nowrap sm:p-4">
                <Avatar user={{ id: u.id, name: u.name, color: u.color, avatar: u.avatar }} size={40} decorative />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="truncate font-semibold">{u.name || 'Sans nom'}</span>
                    {admin && <Badge variant="brand">Admin</Badge>}
                    {isMe && <Badge>Toi</Badge>}
                  </div>
                  <p className="truncate text-xs text-subtle">
                    {u.email} · {plural(u.parties, 'commande')} · inscrit {formatRelativeTime(u.created)}
                  </p>
                </div>
                <Button
                  variant="secondary"
                  size="sm"
                  className="ml-auto"
                  disabled={isMe}
                  title={isMe ? 'Tu ne peux pas retirer tes propres droits.' : undefined}
                  loading={setUserRole.isPending && setUserRole.variables?.u.id === u.id}
                  leftIcon={admin ? <ShieldOff className="size-4" /> : <ShieldCheck className="size-4" />}
                  onClick={() => setUserRole.mutate({ u, next: admin ? 'user' : 'admin' })}
                >
                  {admin ? 'Retirer admin' : 'Promouvoir admin'}
                </Button>
              </Card>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
