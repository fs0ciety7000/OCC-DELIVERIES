import { pb } from './pb'
import type {
  AdminSettings,
  AdminStats,
  AdminUserList,
  AdminUser,
  AppConfig,
  ImportReport,
  PartyStatus,
  UserRole,
  DeclareMethod,
  Dispatch,
  DispatchMethod,
  ExportFormat,
  HealthResponse,
  MenuCategory,
  MenuItem,
  NearbyQuery,
  NearbyRestaurant,
  OrderItem,
  OrderItemInput,
  Party,
  PartyMember,
  Payment,
  PaymentAction,
  PaymentQR,
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
  WalletKind,
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

  join: (code: string) => pb.send<{ party: Party }>('/api/occ/parties/join', json({ code: code.toUpperCase() })),

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

  /** Image QR « recevoir » du payeur (Wero / Bancontact Pay) → blob URL (à révoquer). */
  walletQrObjectUrl: async (paymentId: string, kind: WalletKind): Promise<string | null> => {
    const res = await fetch(pb.buildURL(`/api/occ/payments/${paymentId}/wallet-qr/${kind}`), {
      headers: { Authorization: pb.authStore.token },
    })
    if (res.status === 404) return null
    if (!res.ok) throw new Error('Impossible de charger le QR du payeur.')
    return URL.createObjectURL(await res.blob())
  },

  paymentQR: (paymentId: string) => pb.send<PaymentQR>(`/api/occ/payments/${paymentId}/qr`, { method: 'GET' }),

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

  users: (q: string, page = 1, role?: UserRole) =>
    pb.send<AdminUserList>('/api/occ/admin/users', { method: 'GET', query: { q, page, perPage: 50, ...(role ? { role } : {}) } }),

  setRole: (userId: string, role: UserRole) =>
    pb.send<{ user: AdminUser }>(`/api/occ/admin/users/${userId}/role`, { method: 'PATCH', body: JSON.stringify({ role }), headers: { 'Content-Type': 'application/json' } }),

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
    pb.collection('parties').create<Party>({ ...input, host: hostId, split_mode: 'equal' }, { expand: PARTY_EXPAND }),

  mineActive: (userId: string) =>
    pb.collection('parties').getFullList<Party>({
      filter: pb.filter('members ?= {:u} && status != "closed" && status != "cancelled"', { u: userId }),
      sort: '-updated',
      expand: 'restaurant,members',
    }),

  setCandidates: (id: string, candidates: string[]) =>
    pb.collection('parties').update<Party>(id, { candidates }, { expand: PARTY_EXPAND }),

  update: (id: string, data: Partial<Pick<Party, 'title' | 'delivery_address' | 'notes' | 'voting_ends_at' | 'ordering_ends_at'>> & PartyFeesInput) =>
    pb.collection('parties').update<Party>(id, data, { expand: PARTY_EXPAND }),

  members: (partyId: string) =>
    pb.collection('party_members').getFullList<PartyMember>({
      filter: pb.filter('party = {:p}', { p: partyId }),
      sort: 'created',
      expand: 'user',
    }),

  votes: (partyId: string) =>
    pb.collection('votes').getFullList<Vote>({ filter: pb.filter('party = {:p}', { p: partyId }), sort: 'created' }),

  vote: (partyId: string, userId: string, restaurantId: string) =>
    pb.collection('votes').create<Vote>({ party: partyId, user: userId, restaurant: restaurantId }),

  unvote: (voteId: string) => pb.collection('votes').delete(voteId),

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
  oauth: (provider: string) => pb.collection('users').authWithOAuth2<User>({ provider }),
  refresh: () => pb.collection('users').authRefresh<User>(),
}

export type PayoutProfileInput = Pick<PayoutProfile, 'holder_name' | 'iban' | 'bic' | 'payment_link' | 'wero_id' | 'bancontact_phone'>

export const payoutApi = {
  /** `null` si l'utilisateur n'a pas encore de profil. */
  mine: async (userId: string): Promise<PayoutProfile | null> => {
    const list = await pb.collection('payout_profiles').getList<PayoutProfile>(1, 1, {
      filter: pb.filter('user = {:u}', { u: userId }),
    })
    return list.items[0] ?? null
  },
  /**
   * Création / mise à jour en FormData (fichiers QR Wero / Bancontact).
   * `files[kind] = File` téléverse, `null` supprime, `undefined` ne change rien.
   */
  save: (
    userId: string,
    existingId: string | null,
    data: PayoutProfileInput,
    files: Partial<Record<'wero_qr' | 'bancontact_qr', File | null>> = {},
  ) => {
    const fd = new FormData()
    for (const [k, v] of Object.entries(data)) fd.append(k, v)
    for (const [k, v] of Object.entries(files)) {
      if (v === null) fd.append(k, '')
      else if (v) fd.append(k, v)
    }
    if (existingId) return pb.collection('payout_profiles').update<PayoutProfile>(existingId, fd)
    fd.append('user', userId)
    return pb.collection('payout_profiles').create<PayoutProfile>(fd)
  },

  /** URL d'un fichier protégé de son propre profil (jeton de fichier court). */
  fileUrl: async (profile: PayoutProfile, field: 'wero_qr' | 'bancontact_qr') => {
    const filename = profile[field]
    if (!filename) return null
    const token = await pb.files.getToken()
    return pb.files.getURL(profile as unknown as { id: string; collectionId: string; collectionName: string }, filename, { token })
  },
}
