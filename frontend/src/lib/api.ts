import type { RecordModel } from 'pocketbase'
import { pb } from './pb'
import type {
  AdminSettings,
  AdminStats,
  AdminUserList,
  AdminUser,
  AdminUserStatus,
  AccountInfo,
  MailStatus,
  AppConfig,
  ImportReport,
  PartyStatus,
  UserRole,
  DeclareMethod,
  Dispatch,
  DispatchMethod,
  ExportFormat,
  HealthResponse,
  HistoryPage,
  MyStats,
  ReorderPreview,
  ReorderResult,
  MenuCategory,
  MenuItem,
  NearbyQuery,
  NearbyRestaurant,
  SearchQuery,
  SearchResult,
  OrderItem,
  OrderItemInput,
  Party,
  PartyMember,
  Payment,
  PaymentAction,
  PaymentQR,
  CollectQR,
  PayoutProfile,
  Restaurant,
  RestaurantImport,
  SelectedOption,
  SplitMode,
  SyncDiscoverResult,
  SyncRun,
  SyncRunList,
  SyncSource,
  SyncStatus,
  Summary,
  TransitionBody,
  User,
  Vote,
  BallotResponse,
  Tally,
  PushPrefsResponse,
  PushPublicKey,
  NotifyPrefs,
  GuestAuthResponse,
  InvitePreview,
  PartyTeamInfo,
  Team,
  TeamHistoryPage,
  TeamRecord,
} from './types'

const json = (body: unknown) => ({ method: 'POST', body: JSON.stringify(body), headers: { 'Content-Type': 'application/json' } })

export const PARTY_EXPAND = 'host,members,candidates,restaurant,payer'

/* ------------------------------------------------------- /api/occ métier */

export const occ = {
  health: () => pb.send<HealthResponse>('/api/occ/health', { method: 'GET' }),

  config: () => pb.send<AppConfig>('/api/occ/config', { method: 'GET' }),

  nearby: async (q: NearbyQuery) => {
    const query: Record<string, string | number> = { lat: q.lat, lng: q.lng, radiusKm: q.radiusKm ?? 5 }
    if (q.q) query.q = q.q
    if (q.cuisine) query.cuisine = q.cuisine
    const res = await pb.send<{ items: NearbyRestaurant[] | null }>('/api/occ/restaurants/nearby', { method: 'GET', query })
    return res.items ?? []
  },

  /** Recherche globale (restos, plats, collègues, raccourcis). `signal` : requête annulée si la saisie change. */
  search: (q: SearchQuery, signal?: AbortSignal) => {
    const query: Record<string, string | number> = { q: q.q }
    if (q.limit) query.limit = q.limit
    if (q.lat !== undefined && q.lng !== undefined) {
      query.lat = q.lat
      query.lng = q.lng
    }
    return pb.send<SearchResult>('/api/occ/search', { method: 'GET', query, signal })
  },

  /** Idempotent : `alreadyMember` si on faisait déjà partie de la commande (quel que soit son statut). */
  join: (code: string) => pb.send<{ party: Party; alreadyMember?: boolean }>('/api/occ/parties/join', json({ code: code.toUpperCase() })),

  leave: (partyId: string) => pb.send<{ ok: true }>(`/api/occ/parties/${partyId}/leave`, json({})),

  transition: (partyId: string, body: TransitionBody) =>
    pb.send<{ party: Party }>(`/api/occ/parties/${partyId}/transition`, json(body)),

  ready: (partyId: string, ready: boolean) =>
    pb.send<{ member: PartyMember }>(`/api/occ/parties/${partyId}/ready`, json({ ready })),

  summary: (partyId: string) => pb.send<Summary>(`/api/occ/parties/${partyId}/summary`, { method: 'GET' }),

  dispatch: (partyId: string, method: DispatchMethod) =>
    pb.send<{ party: Party; dispatch: Dispatch }>(`/api/occ/parties/${partyId}/dispatch`, json({ method })),

  setPayer: (partyId: string, payer: string) =>
    pb.send<{ party: Party; payments: Payment[] }>(`/api/occ/parties/${partyId}/payer`, json({ payer })),

  exportUrl: (partyId: string, format: ExportFormat) => pb.buildURL(`/api/occ/parties/${partyId}/export?format=${format}`),

  /** Télécharge l'export (CSV/TXT/JSON) avec le jeton d'auth puis déclenche l'enregistrement. */
  downloadExport: async (partyId: string, format: ExportFormat, fallbackName = 'commande') => {
    const res = await fetch(occ.exportUrl(partyId, format), { headers: { Authorization: pb.authStore.token } })
    if (!res.ok) {
      let message = "L'export a échoué."
      try {
        const body = (await res.json()) as { message?: string }
        if (body.message) message = body.message
      } catch {
        /* corps non JSON */
      }
      throw new Error(message)
    }
    const blob = await res.blob()
    const disposition = res.headers.get('Content-Disposition') ?? ''
    const match = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(disposition)
    const filename = match?.[1] ? decodeURIComponent(match[1]) : `${fallbackName}.${format}`
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    setTimeout(() => URL.revokeObjectURL(url), 2000)
  },

  paymentAction: (paymentId: string, action: PaymentAction, method?: DeclareMethod) =>
    pb.send<{ payment: Payment }>(`/api/occ/payments/${paymentId}/action`, json(method ? { action, method } : { action })),

  paymentQR: (paymentId: string) => pb.send<PaymentQR>(`/api/occ/payments/${paymentId}/qr`, { method: 'GET' }),

  /** Payeur seulement : QR (EPC + liens avec montant) de chaque part à encaisser. */
  collectQR: (partyId: string) => pb.send<CollectQR>(`/api/occ/parties/${partyId}/payments/qr`, { method: 'GET' }),

  /** Mes commandes (membre), plus récentes d'abord, 10 par page. */
  history: (page = 1, perPage = 10) => pb.send<HistoryPage>('/api/occ/me/history', { method: 'GET', query: { page, perPage } }),

  myStats: () => pb.send<MyStats>('/api/occ/me/stats', { method: 'GET' }),

  /** Ma dernière commande dans le restaurant de la party (revalidée par le serveur). */
  reorderPreview: (partyId: string) => pb.send<ReorderPreview>(`/api/occ/parties/${partyId}/reorder`, { method: 'GET' }),

  /** Ajoute ma dernière commande à mon panier : le serveur recalcule les prix et saute l'indisponible. */
  reorder: (partyId: string) => pb.send<ReorderResult>(`/api/occ/parties/${partyId}/reorder`, json({})),

}

/* ------------------------------------------------------------ /api/occ/admin */

/** Télécharge une réponse authentifiée en fichier (export JSON…). */
async function downloadAuthed(path: string, fallbackName: string) {
  const res = await fetch(pb.buildURL(path), { headers: { Authorization: pb.authStore.token } })
  if (!res.ok) throw new Error("Le téléchargement a échoué.")
  const disposition = res.headers.get('Content-Disposition') ?? ''
  const match = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(disposition)
  saveBlob(await res.blob(), match?.[1] ? decodeURIComponent(match[1]) : fallbackName)
}

/** Déclenche l'enregistrement d'un blob côté navigateur. */
export function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 2000)
}

export interface ImportResponse {
  report: ImportReport
  restaurant?: string
  items?: number
}

export const adminApi = {
  stats: () => pb.send<AdminStats>('/api/occ/admin/stats', { method: 'GET' }),

  /** Import JSON (un objet ou un tableau). `dryRun` valide sans rien écrire. */
  importJson: (body: RestaurantImport | RestaurantImport[], dryRun: boolean) =>
    pb.send<ImportResponse>(`/api/occ/admin/import${dryRun ? '?dryRun=1' : ''}`, json(body)),

  importCsv: (file: File, dryRun: boolean) => {
    const fd = new FormData()
    fd.append('file', file)
    return pb.send<ImportResponse>(`/api/occ/admin/import/csv${dryRun ? '?dryRun=1' : ''}`, { method: 'POST', body: fd })
  },

  exportAll: () => pb.send<RestaurantImport[]>('/api/occ/admin/export', { method: 'GET' }),

  downloadExport: () => downloadAuthed('/api/occ/admin/export', 'occ-restaurants.json'),

  users: (q: string, page = 1, filter: { role?: UserRole; status?: AdminUserStatus } = {}) =>
    pb.send<AdminUserList>('/api/occ/admin/users', {
      method: 'GET',
      query: { q, page, perPage: 50, ...(filter.role ? { role: filter.role } : {}), ...(filter.status ? { status: filter.status } : {}) },
    }),

  setRole: (userId: string, role: UserRole) =>
    pb.send<{ user: AdminUser }>(`/api/occ/admin/users/${userId}/role`, { method: 'PATCH', body: JSON.stringify({ role }), headers: { 'Content-Type': 'application/json' } }),

  ban: (userId: string, reason: string) => pb.send<{ user: AdminUser }>(`/api/occ/admin/users/${userId}/ban`, json({ reason })),
  unban: (userId: string) => pb.send<{ user: AdminUser }>(`/api/occ/admin/users/${userId}/unban`, json({})),
  forceLogout: (userId: string) => pb.send<{ user: AdminUser }>(`/api/occ/admin/users/${userId}/logout`, json({})),
  sendPasswordReset: (userId: string) => pb.send<{ sent: boolean; email: string }>(`/api/occ/admin/users/${userId}/password-reset`, json({})),
  /** « Suppression » = anonymisation (historique conservé). */
  deleteUser: (userId: string) => pb.send<{ user: AdminUser }>(`/api/occ/admin/users/${userId}`, { method: 'DELETE' }),

  mailStatus: () => pb.send<MailStatus>('/api/occ/admin/mail', { method: 'GET' }),
  /** Envoie un e-mail de test à l'admin connecté. */
  mailTest: () => pb.send<{ sent: boolean; to: string }>('/api/occ/admin/mail/test', json({})),

  cancelParty: (partyId: string) => pb.send<{ party: Party }>(`/api/occ/admin/parties/${partyId}/cancel`, json({})),

  /* --- synchronisation automatique des menus --- */

  syncStatus: () => pb.send<SyncStatus>('/api/occ/admin/sync/status', { method: 'GET' }),
  syncRuns: (page = 1) => pb.send<SyncRunList>('/api/occ/admin/sync/runs', { method: 'GET', query: { page, perPage: 20 } }),
  syncRun: (id: string) => pb.send<{ run: SyncRun }>(`/api/occ/admin/sync/runs/${id}`, { method: 'GET' }),
  /** Sans argument : toutes les sources activées ; avec `sourceId` : cette source seule. */
  startSync: (sourceId?: string) => pb.send<{ run: SyncRun }>('/api/occ/admin/sync/run', json(sourceId ? { sourceId } : {})),
  /** Cherche le site satellite Takeaway d'un restaurant (10 à 20 s). */
  discoverSync: (query: string) => pb.send<SyncDiscoverResult>('/api/occ/admin/sync/discover', json({ query })),
  /** Ajoute un site Takeaway comme source (idempotent : `created=false` si déjà suivi). */
  addSyncSite: (url: string, label?: string) => pb.send<{ source: SyncSource; created: boolean }>('/api/occ/admin/sync/sources', json({ url, label })),
  syncSources: () => pb.collection('sync_sources').getFullList<SyncSource>({ sort: 'priority,label' }),
  saveSyncSource: (id: string | null, data: Partial<SyncSource>) =>
    id ? pb.collection('sync_sources').update<SyncSource>(id, data) : pb.collection('sync_sources').create<SyncSource>(data),
  deleteSyncSource: (id: string) => pb.collection('sync_sources').delete(id),

  /* --- réglages (cartes incomplètes) --- */

  settings: () => pb.send<AdminSettings>('/api/occ/admin/settings', { method: 'GET' }),
  saveSettings: (minMenuItems: number) =>
    pb.send<AdminSettings>('/api/occ/admin/settings', {
      method: 'PATCH',
      body: JSON.stringify({ minMenuItems }),
      headers: { 'Content-Type': 'application/json' },
    }),

  /* --- catalogue (collections, rules « role = admin ») --- */

  restaurants: () => pb.collection('restaurants').getFullList<Restaurant>({ sort: 'name' }),
  restaurant: (id: string) => pb.collection('restaurants').getOne<Restaurant>(id),
  saveRestaurant: (id: string | null, data: Partial<Restaurant>) =>
    id ? pb.collection('restaurants').update<Restaurant>(id, data) : pb.collection('restaurants').create<Restaurant>(data),
  deleteRestaurant: (id: string) => pb.collection('restaurants').delete(id),

  saveCategory: (id: string | null, data: Partial<MenuCategory>) =>
    id ? pb.collection('menu_categories').update<MenuCategory>(id, data) : pb.collection('menu_categories').create<MenuCategory>(data),
  deleteCategory: (id: string) => pb.collection('menu_categories').delete(id),

  saveItem: (id: string | null, data: Partial<MenuItem>) =>
    id ? pb.collection('menu_items').update<MenuItem>(id, data) : pb.collection('menu_items').create<MenuItem>(data),
  deleteItem: (id: string) => pb.collection('menu_items').delete(id),

  /* --- commandes (lecture admin via les rules) --- */

  parties: (status: PartyStatus | '', page = 1) =>
    pb.collection('parties').getList<Party>(page, 50, {
      sort: '-created',
      expand: 'host,restaurant',
      ...(status ? { filter: pb.filter('status = {:s}', { s: status }) } : {}),
    }),
}

/* ------------------------------------------------- collections (CRUD SDK) */

export const restaurantsApi = {
  get: (id: string) => pb.collection('restaurants').getOne<Restaurant>(id),
  categories: (restaurantId: string) =>
    pb.collection('menu_categories').getFullList<MenuCategory>({
      filter: pb.filter('restaurant = {:r}', { r: restaurantId }),
      sort: 'position,name',
    }),
  items: (restaurantId: string) =>
    pb.collection('menu_items').getFullList<MenuItem>({
      filter: pb.filter('restaurant = {:r}', { r: restaurantId }),
      sort: 'position,name',
    }),
}

export interface CreatePartyInput {
  title: string
  delivery_address?: string
  notes?: string
  candidates?: string[]
  /** Commande « Pour l'équipe … » : adresse, candidats et partage par défaut de l'équipe si vides. */
  team?: string
}

export interface PartyFeesInput {
  delivery_fee?: number
  service_fee?: number
  tip?: number
  split_mode?: SplitMode
}

export const partiesApi = {
  get: (id: string) => pb.collection('parties').getOne<Party>(id, { expand: PARTY_EXPAND }),

  /** Le hook serveur force host, members, code et status. */
  create: (input: CreatePartyInput, hostId: string) =>
    pb.collection('parties').create<Party>({ ...input, host: hostId, ...(input.team ? {} : { split_mode: 'equal' }) }, { expand: PARTY_EXPAND }),

  mineActive: (userId: string) =>
    pb.collection('parties').getFullList<Party>({
      // ⚠ relation multiple : `members.id ?=` (et non `members ?=`, qui ne matche jamais — CLAUDE.md).
      filter: pb.filter('members.id ?= {:u} && status != "closed" && status != "cancelled"', { u: userId }),
      sort: '-updated',
      expand: 'restaurant,members',
    }),

  setCandidates: (id: string, candidates: string[]) =>
    pb.collection('parties').update<Party>(id, { candidates }, { expand: PARTY_EXPAND }),

  update: (id: string, data: Partial<Pick<Party, 'title' | 'delivery_address' | 'notes' | 'voting_ends_at' | 'ordering_ends_at' | 'auto_close_disabled'>> & PartyFeesInput) =>
    pb.collection('parties').update<Party>(id, data, { expand: PARTY_EXPAND }),

  members: (partyId: string) =>
    pb.collection('party_members').getFullList<PartyMember>({
      filter: pb.filter('party = {:p}', { p: partyId }),
      sort: 'created',
      expand: 'user',
    }),

  votes: (partyId: string) =>
    pb.collection('votes').getFullList<Vote>({ filter: pb.filter('party = {:p}', { p: partyId }), sort: 'created' }),

  /** Remplace tout mon bulletin (1er choix d'abord, `[]` = retirer). `clientKey` : idempotence (file hors ligne). */
  ballot: (partyId: string, ranking: string[], clientKey?: string) =>
    pb.send<BallotResponse>(`/api/occ/parties/${partyId}/ballot`, { ...json({ ranking, ...(clientKey ? { clientKey } : {}) }), method: 'PUT' }),

  tally: (partyId: string) => pb.send<Tally>(`/api/occ/parties/${partyId}/tally`, { method: 'GET' }),

  orderItems: (partyId: string) =>
    pb.collection('order_items').getFullList<OrderItem>({ filter: pb.filter('party = {:p}', { p: partyId }), sort: 'created' }),

  addItem: (input: OrderItemInput) => pb.collection('order_items').create<OrderItem>(input),

  updateItem: (id: string, data: Partial<{ quantity: number; note: string; selected_options: SelectedOption[] }>) =>
    pb.collection('order_items').update<OrderItem>(id, data),

  removeItem: (id: string) => pb.collection('order_items').delete(id),

  payments: (partyId: string) =>
    pb.collection('payments').getFullList<Payment>({
      filter: pb.filter('party = {:p}', { p: partyId }),
      sort: 'created',
      expand: 'debtor,creditor',
    }),
}

export const usersApi = {
  update: (id: string, data: Partial<Pick<User, 'name' | 'color'>>) => pb.collection('users').update<User>(id, data),
  authMethods: () => pb.collection('users').listAuthMethods(),
  login: (email: string, password: string) => pb.collection('users').authWithPassword<User>(email, password),
  register: async (name: string, email: string, password: string) => {
    await pb.collection('users').create<User>({ name, email, password, passwordConfirm: password, emailVisibility: false })
    return pb.collection('users').authWithPassword<User>(email, password)
  },
  /**
   * Connexion / inscription OAuth2 (Google) par fenêtre surgissante. La fenêtre est
   * ouverte **dans le clic** (sinon les bloqueurs la refusent) : `PopupBlockedError` si
   * le navigateur l'empêche.
   */
  oauth: (provider: string) => {
    const popup = openAuthPopup()
    if (!popup) return Promise.reject(new PopupBlockedError())
    return pb
      .collection('users')
      .authWithOAuth2<User>({
        provider,
        urlCallback: (url) => {
          popup.location.href = url
        },
      })
      .finally(() => {
        try {
          popup.close()
        } catch {
          /* fenêtre déjà fermée */
        }
      })
  },
  refresh: () => pb.collection('users').authRefresh<User>(),

  /* --- e-mails : vérification, mot de passe oublié, changement d'adresse --- */
  requestVerification: (email: string) => pb.collection('users').requestVerification(email),
  confirmVerification: (token: string) => pb.collection('users').confirmVerification(token),
  requestPasswordReset: (email: string) => pb.collection('users').requestPasswordReset(email),
  confirmPasswordReset: (token: string, password: string) => pb.collection('users').confirmPasswordReset(token, password, password),
  requestEmailChange: (newEmail: string) => pb.collection('users').requestEmailChange(newEmail),
  /** Demande le mot de passe actuel ; toutes les sessions sont ensuite fermées. */
  confirmEmailChange: (token: string, password: string) => pb.collection('users').confirmEmailChange(token, password),
  /** Change le mot de passe puis se reconnecte (le serveur invalide les anciens jetons). */
  changePassword: async (user: Pick<User, 'id' | 'email'>, oldPassword: string, password: string) => {
    await pb.collection('users').update<User>(user.id, { oldPassword, password, passwordConfirm: password })
    if (user.email) await pb.collection('users').authWithPassword<User>(user.email, password)
  },

  /* --- sécurité du compte (/api/occ/me/*) --- */
  account: () => pb.send<AccountInfo>('/api/occ/me/account', { method: 'GET' }),
  unlinkProvider: (provider: string) => pb.send<{ ok: boolean }>(`/api/occ/me/providers/${encodeURIComponent(provider)}`, { method: 'DELETE' }),
  /** Suppression de son compte (anonymisation) : `confirm` = « SUPPRIMER ». */
  deleteMe: (confirm: string) => pb.send<void>('/api/occ/me/delete', json({ confirm })),
}

/** Le navigateur a bloqué la fenêtre de connexion Google. */
export class PopupBlockedError extends Error {
  constructor() {
    super('Ton navigateur a bloqué la fenêtre de connexion. Autorise les fenêtres surgissantes pour ce site, puis réessaie.')
    this.name = 'PopupBlockedError'
  }
}

function openAuthPopup(): Window | null {
  if (typeof window === 'undefined' || !window.open) return null
  const w = Math.min(520, window.innerWidth)
  const h = Math.min(680, window.innerHeight)
  const left = window.screenX + (window.outerWidth - w) / 2
  const top = window.screenY + (window.outerHeight - h) / 2
  return window.open('', 'occ_oauth2', `width=${w},height=${h},left=${left},top=${top},resizable,menubar=no`)
}

export type PayoutProfileInput = Pick<PayoutProfile, 'holder_name' | 'iban' | 'bic' | 'revolut_tag' | 'paypal_me' | 'payment_link'>

export const payoutApi = {
  /** `null` si l'utilisateur n'a pas encore de profil. */
  mine: async (userId: string): Promise<PayoutProfile | null> => {
    const list = await pb.collection('payout_profiles').getList<PayoutProfile>(1, 1, {
      filter: pb.filter('user = {:u}', { u: userId }),
    })
    return list.items[0] ?? null
  },
  /** Création / mise à jour (le serveur normalise IBAN, BIC, revtag, PayPal.me et lien). */
  save: (userId: string, existingId: string | null, data: PayoutProfileInput) => {
    if (existingId) return pb.collection('payout_profiles').update<PayoutProfile>(existingId, data)
    return pb.collection('payout_profiles').create<PayoutProfile>({ ...data, user: userId })
  },
}

/* ------------------------------------------- équipes & invités (1760000016) */

export type TeamSettingsInput = Partial<
  Pick<TeamRecord, 'name' | 'emoji' | 'color' | 'address' | 'lat' | 'lng' | 'usual_time' | 'usual_days' | 'default_candidates' | 'default_split' | 'archived' | 'admins'>
>

export const teamsApi = {
  mine: async () => (await pb.send<{ items: Team[] }>('/api/occ/me/teams', { method: 'GET' })).items ?? [],
  get: async (id: string) => (await pb.send<{ team: Team }>(`/api/occ/teams/${id}`, { method: 'GET' })).team,
  /** Création par la collection : le serveur force propriétaire, membres et code. */
  create: (data: TeamSettingsInput & { name: string }) => pb.collection('teams').create<TeamRecord>(data),
  update: (id: string, data: TeamSettingsInput) => pb.collection('teams').update<TeamRecord>(id, data),
  /** Lien fixe /e/:code — idempotent (`alreadyMember`). */
  join: (code: string) => pb.send<{ team: Team; alreadyMember: boolean }>('/api/occ/teams/join', json({ code: code.toUpperCase() })),
  leave: (id: string) => pb.send<{ ok: true }>(`/api/occ/teams/${id}/leave`, json({})),
  removeMember: (id: string, userId: string) => pb.send<{ team: Team }>(`/api/occ/teams/${id}/members/${userId}`, { method: 'DELETE' }),
  newCode: (id: string) => pb.send<{ code: string }>(`/api/occ/teams/${id}/code`, json({})),
  /** « Lancer la commande du jour » — renvoie la commande en cours si elle existe (`created: false`). */
  launch: (id: string, title?: string) => pb.send<{ party: Party; created: boolean }>(`/api/occ/teams/${id}/launch`, json(title ? { title } : {})),
  history: (id: string, page = 1, perPage = 10) =>
    pb.send<TeamHistoryPage>(`/api/occ/teams/${id}/parties`, { method: 'GET', query: { page, perPage } }),
  /** Rejoindre en un geste la commande de son équipe (sans code). */
  joinParty: (partyId: string) => pb.send<{ party: Party; alreadyMember: boolean }>(`/api/occ/parties/${partyId}/join`, json({})),
  partyTeam: (partyId: string) => pb.send<PartyTeamInfo>(`/api/occ/parties/${partyId}/team`, { method: 'GET' }),
}

export const guestApi = {
  /** Aperçu public d'un lien /j/:code (6 caractères) ou /e/:code (8). */
  preview: (code: string) => pb.send<InvitePreview>(`/api/occ/invites/${encodeURIComponent(code.toUpperCase())}`, { method: 'GET' }),
  /** Crée l'invité·e, le connecte (jeton enregistré) et le fait rejoindre la commande / l'équipe. */
  join: async (input: { name: string; color?: string; partyCode?: string; teamCode?: string }) => {
    const res = await pb.send<GuestAuthResponse>('/api/occ/guest', json(input))
    pb.authStore.save(res.token, res.record as unknown as RecordModel)
    return res
  },
  /** Transforme le compte invité en compte complet (même enregistrement : historique conservé). */
  upgrade: async (input: { email: string; password: string; name?: string }) => {
    const res = await pb.send<{ token: string; record: User }>('/api/occ/me/upgrade', json({ ...input, passwordConfirm: input.password }))
    pb.authStore.save(res.token, res.record as unknown as RecordModel)
    return res
  },
}

/* --------------------------------------------- notifications push (1760000015) */

export interface PushSubscriptionInput {
  endpoint: string
  keys: { p256dh: string; auth: string }
}

export const pushApi = {
  publicKey: () => pb.send<PushPublicKey>('/api/occ/push/public-key', { method: 'GET' }),
  subscribe: (sub: PushSubscriptionInput) => pb.send<{ ok: boolean; id: string }>('/api/occ/push/subscribe', json(sub)),
  unsubscribe: (endpoint: string) =>
    pb.send<{ ok: boolean; deleted: number }>('/api/occ/push/subscribe', {
      method: 'DELETE',
      body: JSON.stringify({ endpoint }),
      headers: { 'Content-Type': 'application/json' },
    }),
  test: () => pb.send<{ queued: boolean; devices: number }>('/api/occ/push/test', json({})),
  prefs: () => pb.send<PushPrefsResponse>('/api/occ/push/prefs', { method: 'GET' }),
  setPrefs: (prefs: Partial<NotifyPrefs>) =>
    pb.send<{ prefs: NotifyPrefs }>('/api/occ/push/prefs', { method: 'PATCH', body: JSON.stringify(prefs), headers: { 'Content-Type': 'application/json' } }),
}
