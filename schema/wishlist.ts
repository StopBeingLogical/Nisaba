// ============================================================
// Wishlist Schema
// ============================================================

import type { ArtAsset, ArtSource, EnrichmentStatus, SteamDeckStatus, ProtonRating } from './game_library';

// ── Enums ────────────────────────────────────────────────────

export type WishlistStoreKey = 'steam' | 'gog';

export type PurchaseStoreKey =
  | 'steam'
  | 'gog'
  | 'epic'
  | 'fanatical'
  | 'humble'
  | 'g2a'
  | 'instant_gaming'
  | 'loaded'
  | 'other';

export type BundleStore =
  | 'humble'
  | 'fanatical'
  | 'indiegala'
  | 'other';

export type ResellerStore =
  | 'g2a'
  | 'instant_gaming'
  | 'loaded'
  | 'other';

export type WishlistPriority = 'high' | 'medium' | 'low';

// ── Sub-types ────────────────────────────────────────────────

export interface WishlistStoreEntry {
  id: string;
  store_url: string;
  date_added: string;           // ISO 8601 date — when added to this store's wishlist
  priority: number | null;      // Steam only — user's sort order in store wishlist
  available: boolean;           // Currently purchasable
  coming_soon: boolean;         // Pre-release, not yet available
  in_development: boolean;      // Early access / in development
  native_client: boolean | null; // GOG only — Galaxy compatible
}

export interface StorePriceEntry {
  price: number;
  regular_price: number;
  discount_percent: number | null;
  store_url: string;
}

export interface BestCurrentPrice {
  price: number;
  store: string;
  store_url: string;
  discount_percent: number | null;
  expires: string | null;       // ISO 8601 datetime — null if not a limited sale
  voucher: string | null;       // Promo code if applicable
}

export interface HistoricalLow {
  price: number;
  store: string;
  date: string;                 // ISO 8601 date
}

export interface BundleEntry {
  store: BundleStore;
  name: string;
  url: string;
  tier_price: number | null;    // Price to unlock this tier — null for pay-what-you-want
  expires: string | null;       // ISO 8601 datetime
  discovered: string;           // ISO 8601 datetime — when first detected
}

export interface PriceSnapshot {
  date: string;                 // ISO 8601 date — when snapshot was recorded
  price: number;                // Lowest price seen that day across all ITAD stores
  store: string;                // Store that had the lowest price
}

export interface ResellerEntry {
  store: ResellerStore;
  url: string;
  last_price: number | null;
  last_checked: string;         // ISO 8601 datetime
}

// ── Core Schema ──────────────────────────────────────────────

export interface WishlistEntry {

  // ── Identity ──────────────────────────────────────────────
  id: string;                           // UUID — internal wishlist entry identifier
  library_id: string | null;            // Links to GameRecord if already owned — non-null triggers owned flag
  igdb_id: number | null;
  title: string;
  sort_title: string;                   // Auto-derived, overridable
  enrichment_status: EnrichmentStatus;
  last_enriched: string | null;             // ISO 8601 datetime — when metadata was last populated or rehydrated

  // ── Artwork ───────────────────────────────────────────────
  artwork: {
    cover: ArtAsset;                    // Required
    square: ArtAsset;                   // Required
    background: ArtAsset | null;
    logo: ArtAsset | null;
    icon: ArtAsset | null;
  };

  // ── Platform ──────────────────────────────────────────────
  platforms: {
    steam_deck_verified: SteamDeckStatus | null;
    proton_rating: ProtonRating | null; // Omitted when steam_deck_verified = 'verified'
  };

  // ── Store Entries ─────────────────────────────────────────
  // At least one of steam or gog will always be present.
  stores: Partial<Record<WishlistStoreKey, WishlistStoreEntry>>;

  // ── Pricing ───────────────────────────────────────────────
  pricing: {
    currency: string;                   // e.g. 'USD' — set at app config level

    // ITAD aggregated — auto-populated on sync
    itad_id: string | null;             // ITAD game ID for API lookups
    best_current: BestCurrentPrice | null;
    historical_low: HistoricalLow | null;
    store_prices: Record<string, StorePriceEntry>; // keyed by ITAD store identifier
    last_synced: string | null;         // ISO 8601 datetime

    // Active bundles — auto-populated from ITAD
    bundles: BundleEntry[];

    // Reseller links — manually entered
    resellers: ResellerEntry[];

    // Per-entry price target — null means global thresholds apply ($2 instant, $5 consider)
    target_price: number | null;

    // Price history — one snapshot per sync where price changed, used for timeline chart
    price_history: PriceSnapshot[];
  };

  // ── User Data ─────────────────────────────────────────────
  user: {
    preferred_store: PurchaseStoreKey | null;  // Target storefront for purchase
    override_owned_warning: boolean;           // Default false — suppresses owned flag when
                                               // intentionally targeting another store
    priority: WishlistPriority | null;         // Personal ranking, independent of store priority
    tags: string[] | null;
    notes: string | null;
    date_added: string;                        // ISO 8601 date — when added to our DB
  };
}
