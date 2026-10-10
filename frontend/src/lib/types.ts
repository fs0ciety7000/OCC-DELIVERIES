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
  /** Invité·e sans compte (prénom seulement, `POST /api/occ/guest`) — géré par le serveur. */
  is_guest?: boolean
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
  /** Revtag Revolut (sans @, minuscules) — lien revolut.me avec montant. */
  revolut_tag: string
  /** Nom PayPal.me — lien paypal.me avec montant. */
  paypal_me: string
  /** Autre lien de paiement (https, normalisé par le serveur). */
  payment_link: string
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
  /** L'hôte a désactivé la clôture automatique aux heures limites (migration 1760000015). */
  auto_close_disabled?: boolean
  /** Serveur : actions automatiques (rappels, clôtures) affichées dans la party. */
  auto_events?: AutoEvent[] | null
  /** Équipe pour laquelle la commande a été lancée (`''` sinon), immuable. */
  team?: string
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

/** Une ligne du bulletin d'un membre (vote par classement, ADR 0005) ; écrit par `PUT /ballot` uniquement. */
export interface Vote extends BaseRecord {
  party: string
  user: string
  restaurant: string
  /** 1 = 1er choix ; rangs contigus par membre. */
  rank: number
  /** `<clientKey>_<rank>` (file hors ligne). */
  client_key?: string
}

export interface TallyStanding {
  restaurant: string
  points: number
  firstChoices: number
  voters: number
}

/** Classement en direct calculé par le serveur (`GET /parties/{id}/tally`). */
export interface Tally {
  /** K : points d'un 1er choix (rang r → max(1, K − r + 1)). */
  candidates: number
  voters: number
  /** Triés par la règle du gagnant. */
  standings: TallyStanding[]
  /** Resto élu si le vote se fermait maintenant (`''` sans candidat). */
  winner: string
}

export interface BallotResponse {
  ballot: string[]
  tally: Tally
}

/** Entrée de `parties.auto_events` (planificateur des heures limites). */
export type AutoEventKind =
  | 'reminder_vote'
  | 'reminder_order'
  | 'vote_closed'
  | 'vote_extended'
  | 'vote_needs_host'
  | 'ordering_closed'
  | 'ordering_needs_host'
  | 'auto_failed'

export interface AutoEvent {
  kind: AutoEventKind | (string & {})
  /** RFC 3339 (UTC). */
  at: ISODate
  /** Texte français prêt à afficher (« Vote clôturé automatiquement à 11:45 — Pizza Nonna »). */
  text: string
}

/* ------------------------------------------------------ notifications push */

/** Préférences de notification (`users.notify_prefs`, champ caché). */
export interface NotifyPrefs {
  party: boolean
  payments: boolean
  reminders: boolean
  /** E-mail « bon de commande » à la validation de la commande (indépendant des appareils abonnés). */
  emails: boolean
}

/** `GET /api/occ/push/prefs`. */
export interface PushPrefsResponse {
  prefs: NotifyPrefs
  /** Appareils abonnés à ce compte. */
  devices: number
  enabled: boolean
  /** Envoi d'e-mails configuré sur le serveur (SMTP). */
  mail: boolean
}

/** `GET /api/occ/push/public-key`. */
export interface PushPublicKey {
  enabled: boolean
  publicKey: string
}

/** Charge utile d'une notification (push et toast temps réel `occ/notifications`). */
export interface NotificationPayload {
  kind: string
  title: string
  body: string
  /** Lien interne (ex. `/party/{id}`). */
  url: string
  tag: string
  partyId?: string
  renotify: boolean
  icon: string
  badge: string
  ts: number
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
  client_key?: string
}

/** Ce que le client envoie : uniquement des intentions (le serveur calcule les prix). */
export interface OrderItemInput {
  party: string
  user: string
  menu_item: string
  quantity: number
  selected_options: SelectedOption[]
  note: string
  /** Clé d'idempotence (file hors ligne) : un rejeu renvoie la ligne existante. */
  client_key?: string
}

/**
 * `wero` / `bancontact` : anciens moyens (retirés, ADR 0003 mise à jour 3),
 * conservés pour afficher les paiements déclarés avant leur retrait.
 */
export type PaymentMethod = 'qr' | 'revolut' | 'paypal' | 'link' | 'wero' | 'bancontact' | 'cash' | 'later' | 'self'
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
  /** Le serveur peut envoyer des e-mails (vérification, mot de passe oublié…). */
  mailEnabled: boolean
  /** Connexion par passkey configurée côté serveur (relying party WebAuthn valide). */
  passkeys?: boolean
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
  /** IBAN du payeur (sans espaces), présent avec `epc`. */
  iban: string | null
  /** Liens de paiement du payeur pour CE paiement (montant pré-rempli quand le format le permet). */
  links: PaymentLink[]
  /** Moyens réellement proposés, dans l'ordre d'affichage recommandé. */
  methods: DeclareMethod[]
}

export type PaymentLinkKind = 'revolut' | 'paypal' | 'link'

export interface PaymentLink {
  kind: PaymentLinkKind
  /** « Revolut », « PayPal » ou le domaine du lien libre. */
  label: string
  url: string
  /** `true` si le montant (et la communication quand c'est possible) est pré-rempli. */
  amountPrefilled: boolean
}

/** Part d'un·e collègue vue par le payeur (« Encaisser », `GET /parties/{id}/payments/qr`). */
export interface CollectItem {
  payment: string
  debtor: { id: string; name: string; avatar: string; color: string }
  amount: Cents
  status: PaymentStatus
  method: PaymentMethod | ''
  reference: string
  /** Payload EPC069-12 avec le montant exact de cette part (`null` sans IBAN). */
  epc: string | null
  /** Liens du payeur construits pour cette part (Revolut / PayPal.me avec montant…). */
  links: PaymentLink[]
}

export interface CollectQR {
  beneficiary: string
  iban: string | null
  items: CollectItem[]
}

export type ExportFormat = 'csv' | 'txt' | 'json'
export type PaymentAction = 'declare' | 'confirm' | 'reset'
export type DeclareMethod = 'qr' | 'revolut' | 'paypal' | 'link' | 'cash' | 'later'

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
  /** Compte suspendu (connexion refusée, sessions coupées). */
  banned: boolean
  bannedReason: string
  /** `''` si non suspendu. */
  bannedAt: ISODate | ''
  /** Compte supprimé (anonymisé : « Compte supprimé », historique conservé). */
  deleted: boolean
  deletedAt: ISODate | ''
  /** `false` pour un compte créé avec Google qui n'a jamais choisi de mot de passe. */
  passwordSet: boolean
  /** Fournisseurs OAuth2 liés (`google`). */
  providers: string[]
  /** Dernière connexion connue (`''` si aucune). */
  lastLoginAt: ISODate | ''
  /** Invité·e sans compte (badge « Invité »). */
  isGuest?: boolean
}

/** Filtre d'état de `GET /api/occ/admin/users` (`status`). */
export type AdminUserStatus = 'banned' | 'unverified' | 'deleted' | 'guest'

/** `GET /api/occ/admin/mail` — aucune donnée secrète. */
export interface MailStatus {
  enabled: boolean
  host: string
  port: number
  tls: boolean
  senderAddress: string
  senderName: string
  /** Configuration lue dans OCC_SMTP_* (sinon : admin PocketBase /_/). */
  fromEnv: boolean
}

/** `GET /api/occ/me/account` — sécurité du compte connecté. */
export interface AccountInfo {
  email: string
  verified: boolean
  /** `false` : compte Google sans mot de passe choisi (impossible de dissocier Google). */
  passwordSet: boolean
  providers: { id: string; provider: string; created: ISODate }[]
  mailEnabled: boolean
}

/** `GET /api/occ/passkeys` — une passkey de mon compte (Profil → Sécurité). */
export interface PasskeyView {
  id: string
  name: string
  created: ISODate
  /** `""` si jamais utilisée pour se connecter. */
  lastUsedAt: ISODate | ''
  transports: string[]
  /** Sauvegardée / synchronisée entre appareils (trousseau iCloud, Google…). */
  synced: boolean
}

export interface AdminUserList {
  page: number
  perPage: number
  totalItems: number
  items: AdminUser[]
}

/* ------------------------------------------- historique (/api/occ/me/*) */

/** Restaurant d'une entrée d'historique (mêmes noms de champs que la collection). */
export interface HistoryRestaurant {
  id: string
  name: string
  emoji: string
  cover: string
  cover_url: string
  active: boolean
}

export interface HistoryItem {
  menuItem: string
  name: string
  optionsLabel: string
  note: string
  quantity: number
  unitPrice: Cents
  total: Cents
}

export interface HistoryPayment {
  id: string
  method: PaymentMethod | ''
  status: PaymentStatus
  amount: Cents
}

/** Une party vue par l'utilisateur connecté (`GET /api/occ/me/history`). */
export interface HistoryEntry {
  id: string
  code: string
  title: string
  status: PartyStatus
  created: ISODate
  closedAt: ISODate
  restaurant: HistoryRestaurant | null
  provider: PartyProvider | ''
  host: UserLite
  isHost: boolean
  memberCount: number
  /** Mes articles uniquement. */
  items: HistoryItem[]
  subtotal: Cents
  sharedFees: Cents
  /** Ma part (articles + frais partagés), calculée par le serveur. */
  total: Cents
  grandTotal: Cents
  payer: UserLite | null
  /** Mon remboursement (débiteur), `null` avant le choix du payeur. */
  payment: HistoryPayment | null
}

export interface HistoryPage {
  page: number
  perPage: number
  totalItems: number
  totalPages: number
  items: HistoryEntry[]
}

/** `GET /api/occ/me/stats` : commandes passées (récap, remboursements, terminées). */
export interface MyStats {
  orders: number
  totalSpent: Cents
  favoriteRestaurant: { id: string; name: string; emoji: string; orders: number } | null
  favoriteDish: { name: string; quantity: number; orders: number } | null
}

export interface ReorderLine {
  menuItem: string
  name: string
  optionsLabel: string
  note: string
  quantity: number
  /** Prix unitaire actuel (serveur), 0 si indisponible. */
  unitPrice: Cents
  available: boolean
  reason?: string
}

/** `GET /api/occ/parties/{id}/reorder` : ma dernière commande dans ce restaurant. */
export interface ReorderPreview {
  source: { partyId: string; title: string; created: ISODate } | null
  items: ReorderLine[]
}

export interface ReorderResult {
  added: { name: string; quantity: number }[]
  skipped: { name: string; reason: string }[]
}

/* ------------------------------------------------- recherche globale (GET /api/occ/search) */

export interface SearchQuery {
  q: string
  /** Résultats max par groupe (1–20, défaut 6). */
  limit?: number
  lat?: number
  lng?: number
}

export interface SearchRestaurantHit {
  id: string
  name: string
  emoji: string
  cuisines: string[]
  itemsCount: number
  /** Absent sans position ou si la position du resto est approximative. */
  distanceKm?: number
}

export interface SearchDishHit {
  id: string
  name: string
  /** Centimes. */
  price: number
  emoji: string
  /** Extrait de la description autour du terme trouvé (texte brut, accents d'origine). */
  snippet: string
  restaurant: { id: string; name: string; emoji: string }
}

export interface SearchSharedParty {
  id: string
  code: string
  title: string
  status: PartyStatus
  created: string
}

/** Collègue : partage au moins une commande (ou une équipe) avec moi. */
export interface SearchPerson {
  id: string
  name: string
  avatar: string
  color: string
  sharedParties: number
  recentParties: SearchSharedParty[]
}

export type SearchActionId = 'new-party' | 'restaurants' | 'my-orders' | 'admin'

export interface SearchAction {
  id: SearchActionId
  label: string
  href: string
}

export interface SearchResult {
  query: string
  /** Termes réellement cherchés (pliés : minuscules, sans accents ; corrections comprises) — pour surligner. */
  terms: string[]
  /** Au moins un terme a été corrigé (faute de frappe). */
  fuzzy: boolean
  restaurants: SearchRestaurantHit[]
  dishes: SearchDishHit[]
  /** Toujours vide sans connexion. */
  people: SearchPerson[]
  actions: SearchAction[]
}

/* ------------------------------------------ équipes & invités (1760000016) */

export type Weekday = 'mon' | 'tue' | 'wed' | 'thu' | 'fri' | 'sat' | 'sun'
export type TeamRole = 'owner' | 'admin' | 'member' | ''

/** Collection `teams` (écriture des réglages par le propriétaire / les admins). */
export interface TeamRecord extends BaseRecord {
  name: string
  code: string
  owner: string
  admins: string[]
  members: string[]
  address: string
  lat: number
  lng: number
  usual_time: string
  usual_days: Weekday[]
  default_candidates: string[]
  default_split: SplitMode | ''
  emoji: string
  color: string
  archived: boolean
  last_party: string
  last_launch_at: ISODate
}

export interface TeamMember extends UserLite {
  isGuest: boolean
  role: TeamRole
}

/** Commande en cours d'une équipe (mise en avant sur la page d'équipe). */
export interface TeamParty {
  id: string
  code: string
  title: string
  status: PartyStatus
  created: ISODate
  host: UserLite
  memberCount: number
  isMember: boolean
  restaurant: HistoryRestaurant | null
}

/** `GET /api/occ/teams/{id}` (`team`) et `GET /api/occ/me/teams` (`items`, 6 membres max, sans candidats). */
export interface Team {
  id: string
  name: string
  code: string
  emoji: string
  color: string
  address: string
  lat: number
  lng: number
  usualTime: string
  usualDays: Weekday[]
  defaultCandidates: HistoryRestaurant[]
  defaultSplit: SplitMode
  archived: boolean
  created: ISODate
  myRole: TeamRole
  memberCount: number
  members: TeamMember[]
  activeParty: TeamParty | null
}

export interface TeamHistoryPage {
  page: number
  perPage: number
  totalItems: number
  totalPages: number
  items: { party: HistoryEntry; isMember: boolean }[]
}

/** `GET /api/occ/parties/{id}/team` — collègues de l'équipe pas encore dans la commande. */
export interface PartyTeamInfo {
  team: { id: string; name: string; emoji: string; color: string; isMember: boolean } | null
  missing: TeamMember[]
}

/** `GET /api/occ/invites/{code}` — aperçu public d'un lien d'invitation. */
export interface InvitePreview {
  kind: 'party' | 'team'
  code: string
  title: string
  joinable: boolean
  memberCount: number
  host?: string
  status?: PartyStatus
  emoji?: string
  color?: string
}

/** `POST /api/occ/guest`. */
export interface GuestAuthResponse {
  token: string
  record: User
  party: { id: string; title: string } | null
  team: { id: string; name: string } | null
}
