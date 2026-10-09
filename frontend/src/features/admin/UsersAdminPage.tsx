import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Ban, KeyRound, LogOut, Mail, MoreHorizontal, Search, Send, ShieldCheck, ShieldOff, Trash2, UserCheck } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { Avatar, Badge, Button, Card, EmptyState, Field, Input, Segmented, Sheet, Skeleton, Textarea } from '@/components/ui'
import { providerLabel } from '@/features/auth/providers'
import { adminApi } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { formatRelativeTime, plural } from '@/lib/format'
import { useDebounced } from '@/lib/hooks'
import { qk } from '@/lib/queryKeys'
import type { AdminUser, UserRole } from '@/lib/types'
import { AdminHeader } from './AdminLayout'
import { filterQuery, USER_FILTERS, type UserFilter } from './userFilters'

/** Actions destructives confirmées dans une feuille. */
type Confirm = { kind: 'ban' | 'delete' | 'logout' | 'reset'; user: AdminUser }

const BAN_REASON_MAX = 300

export function UsersAdminPage() {
  const { user: me } = useAuth()
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<UserFilter>('all')
  const [menuFor, setMenuFor] = useState<AdminUser | null>(null)
  const [confirm, setConfirm] = useState<Confirm | null>(null)
  const [reason, setReason] = useState('')
  const query = useDebounced(q.trim(), 250)
  const qc = useQueryClient()
  const list = useQuery({
    queryKey: qk.admin.users(query, filter),
    queryFn: () => adminApi.users(query, 1, filterQuery(filter)),
    placeholderData: keepPreviousData,
  })
  const mail = useQuery({ queryKey: qk.admin.mail, queryFn: adminApi.mailStatus, staleTime: 60_000 })
  const mailOn = mail.data?.enabled ?? false

  const done = (msg: string) => {
    toast.success(msg)
    setConfirm(null)
    setMenuFor(null)
    setReason('')
    void qc.invalidateQueries({ queryKey: qk.admin.all })
  }
  const nameOf = (u: AdminUser) => u.name || u.email

  const setUserRole = useMutation({
    mutationFn: ({ u, next }: { u: AdminUser; next: UserRole }) => adminApi.setRole(u.id, next),
    onSuccess: ({ user }) => done(user.role === 'admin' ? `${nameOf(user)} est maintenant admin` : `${nameOf(user)} n'est plus admin`),
    onError: (e) => toast.error(errorMessage(e)),
  })
  const unban = useMutation({
    mutationFn: (u: AdminUser) => adminApi.unban(u.id),
    onSuccess: ({ user }) => done(`${nameOf(user)} est réactivé·e`),
    onError: (e) => toast.error(errorMessage(e)),
  })
  const run = useMutation({
    mutationFn: async (c: Confirm): Promise<string> => {
      switch (c.kind) {
        case 'ban':
          await adminApi.ban(c.user.id, reason.trim())
          return `${nameOf(c.user)} est suspendu·e et déconnecté·e`
        case 'delete':
          await adminApi.deleteUser(c.user.id)
          return 'Compte supprimé (historique conservé)'
        case 'logout':
          await adminApi.forceLogout(c.user.id)
          return `${nameOf(c.user)} est déconnecté·e de tous ses appareils`
        case 'reset': {
          const r = await adminApi.sendPasswordReset(c.user.id)
          return `Lien de réinitialisation envoyé à ${r.email}`
        }
      }
    },
    onSuccess: done,
    onError: (e) => toast.error(errorMessage(e)),
  })
  const testMail = useMutation({
    mutationFn: adminApi.mailTest,
    onSuccess: (r) => toast.success(`E-mail de test envoyé à ${r.to}`, { description: 'Vérifie aussi les indésirables.' }),
    onError: (e) => toast.error(errorMessage(e)),
  })

  const open = (kind: Confirm['kind'], user: AdminUser) => {
    setMenuFor(null)
    setReason('')
    setConfirm({ kind, user })
  }

  return (
    <div>
      <AdminHeader title="Utilisateurs" description={list.data ? plural(list.data.totalItems, 'compte') : undefined} />

      <Card className="mb-4 flex flex-wrap items-center gap-3 p-3 sm:p-4" aria-live="polite">
        <span className={cn('grid size-10 shrink-0 place-items-center rounded-full', mailOn ? 'bg-success/12 text-success' : 'bg-warning/12 text-warning')}>
          <Mail aria-hidden className="size-5" />
        </span>
        <div className="min-w-0 flex-1 text-sm">
          <p className="font-semibold">
            E-mails {mail.isPending ? '…' : mailOn ? <Badge variant="success">Actifs</Badge> : <Badge variant="warning">Désactivés</Badge>}
          </p>
          <p className="truncate text-xs text-subtle">
            {mailOn && mail.data
              ? `${mail.data.senderName} <${mail.data.senderAddress}> · ${mail.data.host}:${mail.data.port}${mail.data.fromEnv ? ' · OCC_SMTP_*' : ''}`
              : 'Renseigne OCC_SMTP_HOST, OCC_SMTP_USERNAME, OCC_SMTP_PASSWORD et OCC_MAIL_FROM puis redéploie : vérification, mot de passe oublié et liens de réinitialisation.'}
          </p>
        </div>
        <Button variant="secondary" size="sm" leftIcon={<Send className="size-4" />} disabled={!mailOn} loading={testMail.isPending} onClick={() => testMail.mutate()}>
          Envoyer un e-mail de test
        </Button>
      </Card>

      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="min-w-0 flex-1">
          <Input type="search" aria-label="Rechercher un utilisateur" placeholder="Nom ou e-mail…" value={q} onChange={(e) => setQ(e.target.value)} leftIcon={<Search className="size-4" />} />
        </div>
        <div className="relative overflow-x-auto">
          <Segmented<UserFilter> label="Filtrer les comptes" value={filter} onChange={setFilter} options={USER_FILTERS} />
        </div>
      </div>
      {list.isPending && <Skeleton className="h-64 rounded-lg" />}
      {list.isError && <EmptyState tone="danger" emoji="⚠️" title="Chargement impossible" description={errorMessage(list.error)} />}
      {list.data?.items.length === 0 && <EmptyState title="Personne" description="Aucun compte ne correspond." />}
      <ul className="space-y-2">
        {list.data?.items.map((u) => {
          const isMe = u.id === me?.id
          return (
            <li key={u.id}>
              <Card className={cn('flex items-center gap-3 p-3 sm:p-4', u.deleted && 'opacity-70')}>
                <Avatar user={{ id: u.id, name: u.name, color: u.color, avatar: u.avatar }} size={40} decorative />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="truncate font-semibold">{u.name || 'Sans nom'}</span>
                    {u.role === 'admin' && <Badge variant="brand">Admin</Badge>}
                    {isMe && <Badge>Toi</Badge>}
                    {u.deleted ? <Badge>Supprimé</Badge> : u.banned && <Badge variant="danger">Suspendu</Badge>}
                    {!u.deleted && !u.verified && <Badge variant="warning">Non vérifié</Badge>}
                    {u.providers.map((p) => (
                      <Badge key={p} variant="info">
                        {providerLabel(p)}
                      </Badge>
                    ))}
                  </div>
                  <p className="truncate text-xs text-subtle">
                    {u.deleted ? `supprimé ${formatRelativeTime(u.deletedAt)}` : u.email} · {plural(u.parties, 'commande')} · inscrit {formatRelativeTime(u.created)}
                    {u.lastLoginAt && !u.deleted ? ` · connecté ${formatRelativeTime(u.lastLoginAt)}` : ''}
                  </p>
                  {u.banned && !u.deleted && (
                    <p className="truncate text-xs text-danger">
                      Suspendu {formatRelativeTime(u.bannedAt)}
                      {u.bannedReason ? ` — ${u.bannedReason}` : ''}
                    </p>
                  )}
                </div>
                <Button variant="ghost" size="icon" aria-label={`Actions pour ${u.name || u.email}`} disabled={u.deleted} onClick={() => setMenuFor(u)}>
                  <MoreHorizontal className="size-5" />
                </Button>
              </Card>
            </li>
          )
        })}
      </ul>

      {/* Menu d'actions d'un compte */}
      <Sheet open={!!menuFor} onClose={() => setMenuFor(null)} title={menuFor ? menuFor.name || menuFor.email : ''} description={menuFor?.email}>
        {menuFor && (
          <ActionList>
            {menuFor.role === 'admin' ? (
              <Action icon={<ShieldOff />} disabled={menuFor.id === me?.id} hint={menuFor.id === me?.id ? 'Tu ne peux pas retirer tes propres droits.' : undefined} loading={setUserRole.isPending} onClick={() => setUserRole.mutate({ u: menuFor, next: 'user' })}>
                Retirer les droits admin
              </Action>
            ) : (
              <Action icon={<ShieldCheck />} disabled={menuFor.banned} loading={setUserRole.isPending} onClick={() => setUserRole.mutate({ u: menuFor, next: 'admin' })}>
                Promouvoir admin
              </Action>
            )}
            <Action icon={<KeyRound />} disabled={!mailOn} hint={mailOn ? 'Un e-mail avec un lien valable 30 minutes. Personne ne voit le mot de passe.' : "Indisponible : l'envoi d'e-mails n'est pas configuré."} onClick={() => open('reset', menuFor)}>
              Envoyer un lien de réinitialisation
            </Action>
            <Action icon={<LogOut />} disabled={menuFor.id === me?.id} hint="Ferme toutes ses sessions (tous appareils)." onClick={() => open('logout', menuFor)}>
              Forcer la déconnexion
            </Action>
            {menuFor.banned ? (
              <Action icon={<UserCheck />} loading={unban.isPending} onClick={() => unban.mutate(menuFor)}>
                Réactiver le compte
              </Action>
            ) : (
              <Action icon={<Ban />} tone="danger" disabled={menuFor.id === me?.id} hint={menuFor.id === me?.id ? 'Tu ne peux pas suspendre ton propre compte.' : undefined} onClick={() => open('ban', menuFor)}>
                Suspendre le compte
              </Action>
            )}
            <Action icon={<Trash2 />} tone="danger" disabled={menuFor.id === me?.id} hint={menuFor.id === me?.id ? 'Depuis ton profil uniquement.' : 'Anonymise le compte ; l’historique des commandes est conservé.'} onClick={() => open('delete', menuFor)}>
              Supprimer le compte
            </Action>
          </ActionList>
        )}
      </Sheet>

      {/* Confirmations */}
      <Sheet
        open={!!confirm}
        onClose={() => setConfirm(null)}
        title={confirm ? CONFIRM_TITLE[confirm.kind](nameOf(confirm.user)) : ''}
        footer={
          confirm && (
            <div className="flex gap-2">
              <Button variant="secondary" block onClick={() => setConfirm(null)}>
                Annuler
              </Button>
              <Button variant={confirm.kind === 'ban' || confirm.kind === 'delete' ? 'danger' : 'primary'} block loading={run.isPending} disabled={reason.length > BAN_REASON_MAX} onClick={() => run.mutate(confirm)}>
                {CONFIRM_CTA[confirm.kind]}
              </Button>
            </div>
          )
        }
      >
        {confirm?.kind === 'ban' && (
          <div className="space-y-3">
            <p className="text-sm text-muted">Toutes ses sessions sont fermées immédiatement et toute nouvelle connexion (mot de passe ou Google) est refusée avec le message « Compte suspendu ». Réversible à tout moment.</p>
            <Field label="Motif" optional hint="Visible des admins uniquement." error={reason.length > BAN_REASON_MAX ? `${BAN_REASON_MAX} caractères maximum.` : undefined}>
              {(p) => <Textarea {...p} data-autofocus value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Ex. comportement abusif, compte en double…" />}
            </Field>
          </div>
        )}
        {confirm?.kind === 'delete' && (
          <div className="space-y-2 text-sm text-muted">
            <p>Le nom, l'adresse e-mail, la photo, les coordonnées de remboursement et les connexions Google sont effacés ; le compte ne peut plus servir.</p>
            <p>Les commandes passées restent visibles sous « Compte supprimé », avec leurs montants. <strong className="text-fg">C'est irréversible.</strong></p>
          </div>
        )}
        {confirm?.kind === 'logout' && <p className="text-sm text-muted">Il ou elle devra se reconnecter sur chaque appareil. Le compte reste actif.</p>}
        {confirm?.kind === 'reset' && <p className="text-sm text-muted">Un lien pour choisir un nouveau mot de passe sera envoyé à {confirm.user.email}. Il est valable 30 minutes.</p>}
      </Sheet>
    </div>
  )
}

const CONFIRM_TITLE: Record<Confirm['kind'], (name: string) => string> = {
  ban: (n) => `Suspendre ${n} ?`,
  delete: (n) => `Supprimer le compte de ${n} ?`,
  logout: (n) => `Déconnecter ${n} partout ?`,
  reset: (n) => `Envoyer un lien à ${n} ?`,
}

const CONFIRM_CTA: Record<Confirm['kind'], string> = {
  ban: 'Suspendre',
  delete: 'Supprimer le compte',
  logout: 'Déconnecter',
  reset: 'Envoyer le lien',
}

function ActionList({ children }: { children: ReactNode }) {
  return <ul className="-mx-1 space-y-1">{children}</ul>
}

function Action({
  icon,
  children,
  hint,
  tone,
  disabled,
  loading,
  onClick,
}: {
  icon: ReactNode
  children: string
  hint?: string
  tone?: 'danger'
  disabled?: boolean
  loading?: boolean
  onClick: () => void
}) {
  return (
    <li>
      <button
        type="button"
        disabled={disabled || loading}
        onClick={onClick}
        className={cn(
          'flex min-h-12 w-full items-start gap-3 rounded-md px-3 py-2.5 text-left transition-colors hover:bg-fg/[0.06] disabled:cursor-not-allowed disabled:opacity-50 motion-reduce:transition-none [&_svg]:size-5',
          tone === 'danger' ? 'text-danger' : 'text-fg',
        )}
      >
        <span aria-hidden className="mt-0.5 shrink-0">
          {icon}
        </span>
        <span className="min-w-0">
          <span className="block font-semibold">{children}</span>
          {hint && <span className="block text-xs font-normal text-muted">{hint}</span>}
        </span>
      </button>
    </li>
  )
}
