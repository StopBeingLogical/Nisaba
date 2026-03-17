// ============================================================
// Game Library Schema
// ============================================================

// ── Enums ────────────────────────────────────────────────────

export type EnrichmentStatus =
  | 'matched'      // IGDB match confirmed and applied
  | 'scraped'      // Populated from store page, no IGDB match
  | 'needs_review' // Neither source found — flagged for manual selection
  | 'manual';      // User manually selected or edited the match

export type ArtSource =
  | 'store'  // Official store artwork (preferred)
  | 'igdb'   // IGDB fallback
  | 'manual' // User-provided override

export type StoreKey =
  | 'epic'
  | 'gog'
  | 'amazon'
  | 'steam'
  | 'ea'
  | 'ubisoft';

export type ContentType =
  | 'dlc'
  | 'expansion'
  | 'soundtrack'
  | 'artbook'
  | 'skin'
  | 'pack'
  | 'other';

export type InstallationType =
  | 'separate'  // Must be installed independently — shows as clickable entity
  | 'automatic' // Installed with base game — shows as badge/indicator only

export type Platform = 'windows' | 'linux' | 'mac';

export type SteamDeckStatus =
  | 'verified'
  | 'playable'
  | 'unsupported'
  | 'unknown';

export type ProtonRating =
  | 'platinum'
  | 'gold'
  | 'silver'
  | 'bronze'
  | 'borked'
  | 'unknown';

export type PlayStatus =
  | 'unplayed'
  | 'playing'
  | 'completed'
  | 'dropped'
  | 'on_hold';

// ── Sub-types ────────────────────────────────────────────────

export interface ArtAsset {
  url: string;
  source: ArtSource;
}

export interface StoreLink {
  id: string;
  store_url: string;
  owned: boolean;           // false = reference only (e.g. Steam cross-ref for non-Steam games)
  date_acquired: string | null; // ISO 8601 date — when added to this store's library
  play_time: number | null;     // minutes — from store API if available
  namespace?: string;           // Epic only
}

export interface InstallSource {
  store: StoreKey;
  volume_id: string;            // UUID — references StorageVolume.id in device schema
  install_path: string;
  install_size: number | null;  // bytes
  version: string | null;
  executable: string | null;    // Entry point — Epic provides this, others may not
  platform: Platform;
  save_path: string | null;
}

export interface ContentItem {
  store: StoreKey;
  store_id: string;
  title: string;
  type: ContentType;
  installation_type: InstallationType;
  record_id: string | null;     // UUID ref to standalone GameRecord if installation_type = 'separate'
}

// ── Core Schema ──────────────────────────────────────────────

export interface GameRecord {

  // ── Identity ──────────────────────────────────────────────
  id: string;                       // UUID — internal identifier
  igdb_id: number | null;           // IGDB match ID — primary enrichment source
  title: string;
  sort_title: string;               // Stripped of leading articles, auto-derived, overridable
  developer: string | null;
  publisher: string | null;
  description: string | null;       // From IGDB, fallback to store scrape
  short_description: string | null;
  genres: string[] | null;          // From IGDB, fallback to store scrape
  release_date: string | null;      // ISO 8601 date — from IGDB, fallback to store data
  enrichment_status: EnrichmentStatus;
  last_enriched: string | null;     // ISO 8601 datetime — when metadata was last populated or rehydrated
  is_complete_edition: boolean;     // true = GOTY/Definitive/Complete — suppresses contents indicator
  parent_id: string | null;         // UUID ref to base game — populated for standalone DLC records

  // ── Artwork ───────────────────────────────────────────────
  artwork: {
    cover: ArtAsset;                // Wide/landscape — required
    square: ArtAsset;               // Square or portrait — required
    background: ArtAsset | null;    // Hero/banner image
    logo: ArtAsset | null;          // Transparent logo
    icon: ArtAsset | null;          // Small icon
  };

  // ── Store Links ───────────────────────────────────────────
  // Only stores where the game exists are present.
  // owned: false = reference only (used for Steam Deck cross-referencing).
  stores: Partial<Record<StoreKey, StoreLink>>;

  // ── Platform Support ──────────────────────────────────────
  platforms: {
    windows: boolean;               // Default true
    mac: boolean;
    linux: boolean;
    steam_deck_verified: SteamDeckStatus | null; // All games — via Steam cross-reference
    proton_rating: ProtonRating | null;          // Omitted when steam_deck_verified = 'verified'
  };

  // ── Install State ─────────────────────────────────────────
  install: {
    is_installed: boolean;
    sources: InstallSource[];       // One entry per store/device combination
  };

  // ── Contents ──────────────────────────────────────────────
  // All associated DLC/content for non-complete editions.
  // installation_type drives display: 'separate' = clickable, 'automatic' = badge only.
  contents: ContentItem[];

  // ── User Data ─────────────────────────────────────────────
  user: {
    play_status: PlayStatus | null;
    rating: 1 | 2 | 3 | 4 | 5 | null;
    review: string | null;          // Short review — paired with rating, not mandatory
    is_favorite: boolean;           // Default false
    is_hidden: boolean;             // Default false — hides from main library view
    tags: string[] | null;
    notes: string | null;           // Private free text
    date_added: string;             // ISO 8601 date — earliest date_acquired across all stores
    last_played: string | null;     // ISO 8601 datetime
    play_time: number | null;       // Aggregate minutes across all stores
  };
}
