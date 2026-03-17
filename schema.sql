-- ============================================================
-- NISABA — Database Schema
-- SQLite — updated to match db/store.go column names
-- ============================================================

PRAGMA foreign_keys = ON;

-- ============================================================
-- DEVICES
-- ============================================================

CREATE TABLE IF NOT EXISTS devices (
    id        TEXT PRIMARY KEY,
    name      TEXT NOT NULL,
    label     TEXT NOT NULL,
    platform  TEXT NOT NULL CHECK (platform IN ('steamos', 'windows', 'macos', 'linux', 'other')),
    last_seen TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS storage_volumes (
    id          TEXT PRIMARY KEY,
    device_id   TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    label       TEXT NOT NULL,
    path        TEXT NOT NULL,
    total_bytes INTEGER,
    free_bytes  INTEGER
);

-- ============================================================
-- GAMES
-- ============================================================

CREATE TABLE IF NOT EXISTS games (

    -- Identity
    id                  TEXT PRIMARY KEY,
    igdb_id             INTEGER,
    rawg_id             INTEGER,
    title               TEXT NOT NULL,
    sort_title          TEXT NOT NULL,
    developer           TEXT,
    publisher           TEXT,
    description         TEXT,
    short_description   TEXT,
    release_date        TEXT,
    enrichment_status   TEXT NOT NULL DEFAULT 'needs_review'
                            CHECK (enrichment_status IN ('matched', 'scraped', 'needs_review', 'manual')),
    last_enriched       TEXT,
    is_complete_edition INTEGER NOT NULL DEFAULT 0,
    parent_id           TEXT REFERENCES games(id),

    -- Artwork — JSON: { cover, square, background, logo, icon } each { url, source }
    artwork             TEXT NOT NULL DEFAULT '{}',

    -- Platform support
    windows             INTEGER NOT NULL DEFAULT 1,
    mac                 INTEGER NOT NULL DEFAULT 0,
    linux               INTEGER NOT NULL DEFAULT 0,
    steam_deck_verified TEXT CHECK (steam_deck_verified IN ('verified', 'playable', 'unsupported', 'unknown')),
    proton_rating       TEXT CHECK (proton_rating IN ('platinum', 'gold', 'silver', 'bronze', 'borked', 'unknown')),

    -- Install state (updated when install sources change)
    is_installed        INTEGER NOT NULL DEFAULT 0,

    -- User data
    play_status         TEXT CHECK (play_status IN ('unplayed', 'playing', 'completed', 'dropped', 'on_hold')),
    rating              INTEGER CHECK (rating BETWEEN 1 AND 5),
    short_review        TEXT,
    notes               TEXT,
    is_favorite         INTEGER NOT NULL DEFAULT 0,
    is_hidden           INTEGER NOT NULL DEFAULT 0,
    play_time_minutes   INTEGER,
    last_played         TEXT,
    date_added          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_games_sort_title          ON games(sort_title);
CREATE INDEX IF NOT EXISTS idx_games_enrichment_status   ON games(enrichment_status);
CREATE INDEX IF NOT EXISTS idx_games_play_status         ON games(play_status);
CREATE INDEX IF NOT EXISTS idx_games_is_installed        ON games(is_installed);
CREATE INDEX IF NOT EXISTS idx_games_is_favorite         ON games(is_favorite);
CREATE INDEX IF NOT EXISTS idx_games_is_hidden           ON games(is_hidden);
CREATE INDEX IF NOT EXISTS idx_games_parent_id           ON games(parent_id);
CREATE INDEX IF NOT EXISTS idx_games_steam_deck_verified ON games(steam_deck_verified);

-- Genres
CREATE TABLE IF NOT EXISTS game_genres (
    game_id TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    genre   TEXT NOT NULL,
    PRIMARY KEY (game_id, genre)
);

CREATE INDEX IF NOT EXISTS idx_game_genres_genre ON game_genres(genre);

-- User tags
CREATE TABLE IF NOT EXISTS game_tags (
    game_id TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    tag     TEXT NOT NULL,
    PRIMARY KEY (game_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_game_tags_tag ON game_tags(tag);

-- Store links
CREATE TABLE IF NOT EXISTS game_stores (
    game_id     TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    store       TEXT NOT NULL CHECK (store IN ('epic', 'gog', 'amazon', 'steam', 'ea', 'ubisoft')),
    store_id    TEXT,
    store_url   TEXT,
    owned       INTEGER NOT NULL DEFAULT 1,  -- 0 = reference only (Steam cross-ref)
    owned_since TEXT,
    PRIMARY KEY (game_id, store)
);

CREATE INDEX IF NOT EXISTS idx_game_stores_store    ON game_stores(store);
CREATE INDEX IF NOT EXISTS idx_game_stores_owned    ON game_stores(owned);
CREATE INDEX IF NOT EXISTS idx_game_stores_store_id ON game_stores(store_id);

-- Install sources — one row per game+device
CREATE TABLE IF NOT EXISTS game_install_sources (
    game_id            TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    device_id          TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    volume_id          TEXT REFERENCES storage_volumes(id),
    install_path       TEXT,
    install_size_bytes INTEGER,
    runner             TEXT CHECK (runner IN ('steam', 'legendary', 'gog', 'nile', 'ea', 'ubisoft', 'native')),
    last_seen          TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (game_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_install_sources_game_id   ON game_install_sources(game_id);
CREATE INDEX IF NOT EXISTS idx_install_sources_device_id ON game_install_sources(device_id);

-- Contents — bundled DLC items (non-standalone)
CREATE TABLE IF NOT EXISTS game_contents (
    game_id           TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    content_type      TEXT NOT NULL CHECK (content_type IN ('dlc', 'expansion', 'soundtrack', 'artbook', 'skin', 'pack', 'other')),
    title             TEXT NOT NULL,
    store_id          TEXT,
    is_installed      INTEGER NOT NULL DEFAULT 0,
    installation_type TEXT NOT NULL DEFAULT 'automatic' CHECK (installation_type IN ('separate', 'automatic')),
    sort_order        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (game_id, title)
);

CREATE INDEX IF NOT EXISTS idx_game_contents_game_id ON game_contents(game_id);

-- ============================================================
-- WISHLIST
-- ============================================================

CREATE TABLE IF NOT EXISTS wishlist_entries (

    -- Identity
    id                  TEXT PRIMARY KEY,
    library_id          TEXT REFERENCES games(id),
    igdb_id             INTEGER,
    rawg_id             INTEGER,
    title               TEXT NOT NULL,
    sort_title          TEXT NOT NULL,
    enrichment_status   TEXT NOT NULL DEFAULT 'needs_review'
                            CHECK (enrichment_status IN ('matched', 'scraped', 'needs_review', 'manual')),
    last_enriched       TEXT,

    -- Artwork — JSON
    artwork             TEXT NOT NULL DEFAULT '{}',

    -- Platform
    steam_deck_verified TEXT CHECK (steam_deck_verified IN ('verified', 'playable', 'unsupported', 'unknown')),
    proton_rating       TEXT CHECK (proton_rating IN ('platinum', 'gold', 'silver', 'bronze', 'borked', 'unknown')),

    -- Pricing (scalar columns for sorting/display)
    currency            TEXT NOT NULL DEFAULT 'USD',
    itad_id             TEXT,
    best_current_price  REAL,
    best_current_store  TEXT,
    historical_low_price REAL,
    historical_low_store TEXT,
    last_price_sync     TEXT,
    target_price        REAL,

    -- User data
    preferred_store     TEXT,
    priority            INTEGER NOT NULL DEFAULT 0,  -- 0=none, 1=low, 2=medium, 3=high
    notes               TEXT,
    date_added          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_wishlist_sort_title         ON wishlist_entries(sort_title);
CREATE INDEX IF NOT EXISTS idx_wishlist_library_id         ON wishlist_entries(library_id);
CREATE INDEX IF NOT EXISTS idx_wishlist_enrichment_status  ON wishlist_entries(enrichment_status);
CREATE INDEX IF NOT EXISTS idx_wishlist_priority           ON wishlist_entries(priority);
CREATE INDEX IF NOT EXISTS idx_wishlist_itad_id            ON wishlist_entries(itad_id);
CREATE INDEX IF NOT EXISTS idx_wishlist_best_price         ON wishlist_entries(best_current_price);

-- Wishlist tags
CREATE TABLE IF NOT EXISTS wishlist_tags (
    wishlist_id TEXT NOT NULL REFERENCES wishlist_entries(id) ON DELETE CASCADE,
    tag         TEXT NOT NULL,
    PRIMARY KEY (wishlist_id, tag)
);

-- Store availability per wishlist entry
CREATE TABLE IF NOT EXISTS wishlist_stores (
    wishlist_id TEXT NOT NULL REFERENCES wishlist_entries(id) ON DELETE CASCADE,
    store       TEXT NOT NULL CHECK (store IN ('steam', 'gog', 'epic', 'amazon', 'humble', 'fanatical')),
    store_id    TEXT,
    store_url   TEXT,
    PRIMARY KEY (wishlist_id, store)
);

-- Active bundles
CREATE TABLE IF NOT EXISTS wishlist_bundles (
    wishlist_id  TEXT NOT NULL REFERENCES wishlist_entries(id) ON DELETE CASCADE,
    bundle_name  TEXT NOT NULL,
    store        TEXT NOT NULL,
    url          TEXT,
    price        REAL,
    expires_at   TEXT,
    active       INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (wishlist_id, bundle_name)
);

CREATE INDEX IF NOT EXISTS idx_wishlist_bundles_wishlist_id ON wishlist_bundles(wishlist_id);

-- Manual reseller entries
CREATE TABLE IF NOT EXISTS wishlist_resellers (
    wishlist_id   TEXT NOT NULL REFERENCES wishlist_entries(id) ON DELETE CASCADE,
    reseller_name TEXT NOT NULL,
    url           TEXT,
    price         REAL NOT NULL,
    currency      TEXT NOT NULL DEFAULT 'USD',
    last_checked  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (wishlist_id, reseller_name)
);

-- Price history — pricecharting-style timeline
CREATE TABLE IF NOT EXISTS wishlist_price_history (
    wishlist_id TEXT NOT NULL REFERENCES wishlist_entries(id) ON DELETE CASCADE,
    price       REAL NOT NULL,
    store       TEXT NOT NULL,
    recorded_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_price_history_wishlist_id ON wishlist_price_history(wishlist_id);
CREATE INDEX IF NOT EXISTS idx_price_history_recorded_at ON wishlist_price_history(recorded_at);

-- ============================================================
-- APP CONFIG
-- ============================================================

CREATE TABLE IF NOT EXISTS app_config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- ============================================================
-- SYNC LOG
-- ============================================================

CREATE TABLE IF NOT EXISTS sync_log (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    type          TEXT NOT NULL CHECK (type IN ('full', 'ownership', 'install', 'pricing', 'wishlist', 'rehydrate')),
    status        TEXT NOT NULL DEFAULT 'running'
                      CHECK (status IN ('running', 'done', 'failed', 'partial')),
    started_at    TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at   TEXT,
    games_added   INTEGER,
    games_updated INTEGER,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_sync_log_started_at ON sync_log(started_at);
CREATE INDEX IF NOT EXISTS idx_sync_log_type       ON sync_log(type);

-- ============================================================
-- ENRICHMENT QUEUE
-- ============================================================

CREATE TABLE IF NOT EXISTS enrichment_queue (
    entity_type TEXT NOT NULL CHECK (entity_type IN ('game', 'wishlist')),
    entity_id   TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'running', 'done', 'failed')),
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT,
    PRIMARY KEY (entity_type, entity_id)
);

CREATE INDEX IF NOT EXISTS idx_enrichment_queue_status ON enrichment_queue(status);
