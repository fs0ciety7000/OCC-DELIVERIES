/**
 * Types TS miroirs du contrat `docs/ARCHITECTURE.md`.
 * Tous les montants sont en centimes (entiers).
 */

export type Cents = number
export type ISODate = string

export interface BaseRecord {
  id: string
  collectionId?: string
  collectionName?: string
  created?: ISODate
  updated?: ISODate
}

/* ------------------------------------------------------------------ users */

export type UserRole = 'user' | 'admin'

export interface User extends BaseRecord {
  email?: string
  name: string
  avatar?: string
  color?: string
  verified?: boolean
  /** `admin` donne accès au panneau /admin (OCC_ADMIN_EMAIL / OCC_ADMINS ou promotion). */
  role?: UserRole | ''
}

/** Utilisateur « allégé » tel que renvoyé par `Summary`. */
export interface UserLite {
  id: string
  name: string
  avatar?: string
  color?: string
}

export interface PayoutProfile extends BaseRecord {
  user: string
  holder_name: string
  iban: string
  bic: string
  payment_link: string
  /** Mobile (E.164) ou e-mail enregistré sur Wero. */
  wero_id: string
  /** Mobile (E.164) lié à Bancontact Pay. */
  bancontact_phone: string
  /** Image du QR « recevoir » (fichier protégé). */
  wero_qr: string
  bancontact_qr: string
}

/* ------------------------------------------------------------ restaurants */

export type ProviderId = 'ubereats' | 'takeaway' | 'deliveroo' | 'weloveat'

export interface RestaurantProviderLink {
  id: ProviderId
  url: string
}

export interface Restaurant extends BaseRecord {
  name: string
  slug: string
  description: string
  emoji: string
  cover?: string
  cover_url?: string
  cuisines: string[] | null
  address: string
  lat: number
  lng: number
  phone: string
  rating: number
  rating_count: number
  price_level: number
  eta_min: number
  eta_max: number
  delivery_fee: Cents
  min_order: Cents
  providers: RestaurantProviderLink[] | null
  active: boolean
  /** Synchronisation : identifiant stable de la source (`fournisseur:url`). */
  source_key?: string
  /** Synchronisation : sources qui proposent ce restaurant. */
  sources?: SyncSourceRef[] | null
  /** Verrouillé : la synchronisation ne le modifie plus (posé à toute modification admin). */
  locked?: boolean
  /** Obsolète : plus proposé par aucune source depuis cette date ("" sinon). */
  stale_since?: ISODate
  /** Carte partielle : seuls quelques plats sont connus (instantané Uber Eats). */
  partial_menu?: boolean
  /** Position approximative (lieu par défaut) : distance non fiable. */
  geo_approx?: boolean
  /** Serveur : nombre de plats disponibles (cartes incomplètes masquées sous `minMenuItems`). */
  items_count?: number
  /** Serveur : champs complétés depuis OpenStreetMap (attribution ODbL), `null` sinon. */
  enriched_from?: EnrichedFrom | null
}

/** Provenance des coordonnées complétées automatiquement (`restaurants.enriched_from`). */
export interface EnrichedFrom {
  provider: 'osm' | (string & {})
  /** Page OpenStreetMap de l'établissement reconnu. */
  url: string
  fields: ('phone' | 'address' | 'geo' | (string & {}))[]
  checked_at: string
}

export type NearbyRestaurant = Restaurant & { distanceKm: number }

export interface MenuCategory extends BaseRecord {
  restaurant: string
  name: string
  position: number
}

export interface OptionChoice {
  id: string
  name: string
  price: Cents
}

export interface OptionGroup {
  id: string
  name: string
  min: number
  max: number
  choices: OptionChoice[]
}

export type MenuTag = 'veggie' | 'vegan' | 'spicy' | 'gluten_free' | 'new' | (string & {})

export interface MenuItem extends BaseRecord {
  restaurant: string
  category: string
  name: string
  description: string
  price: Cents
  emoji: string
  image?: string
  tags: MenuTag[] | null
  option_groups: OptionGroup[] | null
  popular: boolean
  available: boolean
  position: number
  source_key?: string
  sources?: SyncSourceRef[] | null
  /** Verrouillé : la synchronisation ne le modifie plus. */
  locked?: boolean
}

/* -------------------------------------------------------- synchronisation */

export type SyncProvider = 'deliveroo' | 'weloveat' | 'takeaway-site' | 'jsonld' | 'ubereats-snapshot'
export type SyncRunStatus = 'running' | 'success' | 'partial' | 'failed' | 'blocked'
export type SyncTrigger = 'cron' | 'manual' | 'startup'
export type SyncSourceStatus = 'ok' | 'blocked' | 'failed'

export interface SyncSourceRef {
  provider: SyncProvider
  url: string
  checked_at: string
}

/** Collection `sync_sources` (admin). */
export interface SyncSource extends BaseRecord {
  provider: SyncProvider
  label: string
  /** Page liste (Deliveroo), racine de l'API (weloveat) ou site (takeaway-site / jsonld). */
  url: string
  city: string
  /** Plus petit = préféré pour le menu, lu en premier. */
  priority: number
  enabled: boolean
  /** Requêtes supplémentaires pour les options (suppléments weloveat). */
  options: boolean
  last_run_at: ISODate
  /** « ok: 40 restaurants », « blocked: … », « failed: … » (serveur). */
  last_status: string
}

export interface SyncStats {
  restaurants_created: number
  restaurants_updated: number
  restaurants_stale: number
  items_created: number
  items_updated: number
  items_price_changed: number
  items_unavailable: number
  /** Restaurants complétés depuis OpenStreetMap (téléphone, adresse, position). */
  restaurants_enriched?: number
}

export interface SyncSourceResult {
  id: string
  label: string
  provider: SyncProvider
  url: string
  status: SyncSourceStatus
  message: string
  restaurants: number
  network: number
  cached: number
  durationMs: number
}

export interface SyncRun {
  id: string
  started_at: ISODate
  finished_at: ISODate
  status: SyncRunStatus
  trigger: SyncTrigger
  stats: SyncStats
  sources: SyncSourceResult[]
  changesCount: number
  /** Détail uniquement (`GET /sync/runs/{id}`). */
  changes?: string[]
  log?: string
  /** Dernières lignes du journal (exécution en cours, `GET /sync/status`). */
  logTail?: string
  error: string
}

export interface SyncStatus {
  enabled: boolean
  cron: string
  timezone: string
  running: SyncRun | null
  lastRun: SyncRun | null
  nextRunAt: ISODate | null
}

/** Statut d'un hôte vérifié par « Découvrir ». */
export type SyncDiscoverStatus = 'found' | 'absent' | 'unreachable' | 'http' | 'robots' | 'blocked' | 'other' | 'invalid' | 'skipped'

export interface SyncDiscoverTry {
  host: string
  status: SyncDiscoverStatus
  message?: string
}

/** Site satellite Takeaway trouvé par `POST /api/occ/admin/sync/discover`. */
export interface SyncDiscoverFound {
  url: string
  host: string
  name: string
  address: string
  items: number
  categories: number
  takeawayUrl?: string
  lat?: number
  lng?: number
  /** Distance au lieu par défaut (km, une décimale) ; absente sans coordonnées. */
  distanceKm?: number
  /** Une source `takeaway-site` / `jsonld` lit déjà ce site. */
  alreadySource: boolean
  sourceId?: string
}

export interface SyncDiscoverResult {
  query: string
  kind: 'takeaway' | 'name' | 'site'
  slug: string
  tried: SyncDiscoverTry[]
  found: SyncDiscoverFound[]
  network: number
  durationMs: number
  interrupted: boolean
}

export interface SyncRunList {
  page: number
  perPage: number
  totalItems: number
  items: SyncRun[]
}

/* ---------------------------------------------------------------- parties */

export type PartyStatus = 'lobby' | 'voting' | 'ordering' | 'review' | 'paying' | 'closed' | 'cancelled'
export type PartyProvider = 'ubereats' | 'takeaway' | 'deliveroo' | 'weloveat' | 'manual'
export type SplitMode = 'equal' | 'proportional'
export type DispatchMethod = 'ubereats' | 'takeaway' | 'deliveroo' | 'weloveat' | 'export' | 'phone'

export interface PartyDispatch {
  method: DispatchMethod
  at: ISODate
  url?: string
}

export interface Party extends BaseRecord {
  code: string
  title: string
  host: string
  members: string[]
  status: PartyStatus
  candidates: string[]
  restaurant: string
  provider: PartyProvider | ''
  delivery_address: string
  notes: string
  voting_ends_at: ISODate
  ordering_ends_at: ISODate
  split_mode: SplitMode | ''
  delivery_fee: Cents
  service_fee: Cents
  tip: Cents
  payer: string
  dispatch: PartyDispatch | null
  closed_at: ISODate
  expand?: {
    host?: User
    members?: User[]
    candidates?: Restaurant[]
    restaurant?: Restaurant
    payer?: User
  }
}

export type PartyRole = 'host' | 'member'

export interface PartyMember extends BaseRecord {
  party: string
  user: string
  role: PartyRole
  ready: boolean
  expand?: { user?: User }
}

export interface Vote extends BaseRecord {
  party: string
  user: string
  restaurant: string
}

export interface SelectedOption {
  group: string
  choices: string[]
}

export interface OrderItem extends BaseRecord {
  party: string
  user: string
  menu_item: string
  quantity: number
  selected_options: SelectedOption[] | null
  note: string
  name: string
  options_label: string
  unit_price: Cents
  total: Cents
}

/** Ce que le client envoie : uniquement des intentions (le serveur calcule les prix). */
export interface OrderItemInput {
  party: string
  user: string
  menu_item: string
  quantity: number
  selected_options: SelectedOption[]
  note: string
}

export type PaymentMethod = 'qr' | 'wero' | 'bancontact' | 'link' | 'cash' | 'later' | 'self'
export type PaymentStatus = 'pending' | 'declared' | 'confirmed'

export interface Payment extends BaseRecord {
  party: string
  debtor: string
  creditor: string
  amount: Cents
  method: PaymentMethod | ''
  status: PaymentStatus
  reference: string
  declared_at: ISODate
  confirmed_at: ISODate
  expand?: { debtor?: User; creditor?: User }
}

/* ------------------------------------------------------------- endpoints */

export interface HealthResponse {
  status: 'ok'
  version: string
}

export interface ProviderConfig {
  id: string
  name: string
  color: string
  enabled: boolean
}

export interface AppConfig {
  currency: string
  defaultLocation: { lat: number; lng: number; label: string }
  providers: ProviderConfig[]
  /** Les restaurants ayant moins de plats disponibles sont absents des listes (0 = aucun filtre). */
  minMenuItems: number
}

/** `GET / PATCH /api/occ/admin/settings`. */
export interface AdminSettings {
  /** Seuil « cartes incomplètes » (0 = désactivé, 1–100). */
  minMenuItems: number
  /** Restaurants actifs masqués des listes publiques par ce seuil. */
  hiddenRestaurants: number
  activeRestaurants: number
}

export interface NearbyQuery {
  lat: number
  lng: number
  radiusKm?: number
  q?: string
  cuisine?: string
}

export interface SummaryItem {
  id: string
  name: string
  optionsLabel: string
  note: string
  quantity: number
  unitPrice: Cents
  total: Cents
}

export interface SummaryParticipant {
  user: UserLite
  ready: boolean
  items: SummaryItem[]
  subtotal: Cents
  sharedFees: Cents
  total: Cents
}

export interface SummaryConsolidatedLine {
  menuItem: string
  name: string
  optionsLabel: string
  quantity: number
  total: Cents
  notes: string[]
}

export interface Summary {
  partyId: string
  currency: string
  status: PartyStatus
  restaurant: { id: string; name: string; minOrder: Cents; deliveryFee: Cents } | null
  participants: SummaryParticipant[]
  consolidated: SummaryConsolidatedLine[]
  itemsSubtotal: Cents
  deliveryFee: Cents
  serviceFee: Cents
  tip: Cents
  sharedFees: Cents
  grandTotal: Cents
  minOrderReached: boolean
  allReady: boolean
  splitMode: SplitMode
}

export interface Dispatch {
  method: DispatchMethod
  url: string | null
  cartText: string
  instructions: string[]
}

export interface PaymentQR {
  amount: Cents
  reference: string
  beneficiary: string
  epc: string | null
  link: string | null
  wero: { id: string; hasQr: boolean } | null
  bancontact: { phone: string; hasQr: boolean } | null
  /** Moyens réellement proposés, dans l'ordre d'affichage recommandé. */
  methods: DeclareMethod[]
}

export type WalletKind = 'wero' | 'bancontact'

export type ExportFormat = 'csv' | 'txt' | 'json'
export type PaymentAction = 'declare' | 'confirm' | 'reset'
export type DeclareMethod = 'qr' | 'wero' | 'bancontact' | 'link' | 'cash' | 'later'

export interface TransitionBody {
  to: PartyStatus
  restaurant?: string
}

/* ------------------------------------------------------------------ admin */

/** Article au format d'import (`POST /api/occ/admin/import`). */
export interface ItemImport {
  name: string
  description?: string
  price: Cents
  emoji?: string
  tags?: string[]
  option_groups?: OptionGroup[]
  popular?: boolean
  available?: boolean
}

export interface CategoryImport {
  name: string
  items: ItemImport[]
}

/** Restaurant + menu au format d'import / export (les clés inconnues sont ignorées). */
export interface RestaurantImport {
  slug: string
  name: string
  description?: string
  emoji?: string
  cover_url?: string
  cuisines?: string[]
  address?: string
  lat?: number
  lng?: number
  phone?: string
  rating?: number
  rating_count?: number
  price_level?: number
  eta_min?: number
  eta_max?: number
  delivery_fee?: Cents
  min_order?: Cents
  providers?: RestaurantProviderLink[]
  active?: boolean
  partial_menu?: boolean
  geo_approx?: boolean
  categories: CategoryImport[]
  [key: string]: unknown
}

export interface ImportPreviewItem {
  category: string
  name: string
  price: Cents
  tags: string[]
  popular: boolean
  available: boolean
  options: number
}

export interface ImportRestaurantReport {
  slug: string
  name: string
  exists: boolean
  active: boolean
  categories: number
  items: number
  errors: string[]
  warnings: string[]
  menu: ImportPreviewItem[]
  restaurant?: string
}

export interface ImportReport {
  dryRun: boolean
  valid: boolean
  errors: string[]
  restaurants: ImportRestaurantReport[]
  items: number
}

export interface AdminStats {
  users: number
  admins: number
  restaurants: { total: number; active: number }
  menuItems: number
  parties: { total: number; byStatus: Record<PartyStatus, number> }
  partiesPerDay: { date: string; count: number }[]
  orderedTotal: Cents
  orderedLines: number
  topRestaurants: { id: string; name: string; slug: string; emoji: string; parties: number; amount: Cents }[]
}

export interface AdminUser {
  id: string
  name: string
  email: string
  role: UserRole
  color: string
  avatar: string
  verified: boolean
  created: ISODate
  parties: number
}

export interface AdminUserList {
  page: number
  perPage: number
  totalItems: number
  items: AdminUser[]
}
