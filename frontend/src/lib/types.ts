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

export interface User extends BaseRecord {
  email?: string
  name: string
  avatar?: string
  color?: string
  verified?: boolean
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

export interface RestaurantImport {
  slug: string
  name: string
  categories: { name: string; items: Array<Partial<MenuItem> & { name: string; price: Cents }> }[]
  [key: string]: unknown
}
