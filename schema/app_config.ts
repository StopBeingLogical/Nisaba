// ============================================================
// App Config Schema
// ============================================================

// ── Pricing ──────────────────────────────────────────────────

export interface PricingConfig {
  currency: string;                 // e.g. 'USD' — applied to all pricing display
  instant_buy_threshold: number;    // Default 2.00 — price at or below this is an instant buy
  consider_threshold: number;       // Default 5.00 — price at or below this warrants checking history
}

// ── Store Credentials ─────────────────────────────────────────

export interface SteamConfig {
  api_key: string;
  steam_id: string;                 // 64-bit SteamID
}

export interface GOGOAuthConfig {
  client_id: string;                // Public GOG Galaxy client_id
  client_secret: string;            // Public GOG Galaxy client_secret
  access_token: string;
  refresh_token: string;
  expires_at: string;               // ISO 8601 datetime — auto-refreshed on expiry
  user_id: string;
}

// ── Enrichment Credentials ────────────────────────────────────
// Enrichment order: IGDB → RAWG → store scrape → flag needs_review

export interface IGDBConfig {
  client_id: string;                // Twitch developer app client ID
  client_secret: string;            // Twitch developer app client secret
  access_token: string;             // OAuth token — auto-refreshed (expires every ~60 days)
  token_expires_at: string;         // ISO 8601 datetime
}

export interface RAWGConfig {
  api_key: string;                  // Free registration at rawg.io
}

// ── Price Tracking ────────────────────────────────────────────

export interface ITADConfig {
  api_key: string;                  // Free registration at isthereanydeal.com
}

// ── Core Schema ──────────────────────────────────────────────

export interface AppConfig {
  pricing: PricingConfig;
  steam: SteamConfig;
  gog: GOGOAuthConfig;
  igdb: IGDBConfig;
  rawg: RAWGConfig;
  itad: ITADConfig;
}

// ── Defaults ─────────────────────────────────────────────────

export const DEFAULT_PRICING: PricingConfig = {
  currency: 'USD',
  instant_buy_threshold: 2.00,
  consider_threshold: 5.00,
};
