# Data Relationships

## Entity Map

```
AppConfig (singleton)
│
│   Provides credentials and thresholds used by all sync
│   and enrichment operations. No direct FK relationships.
│
├─────────────────────────────────────────────────────────┐
│                                                         │
▼                                                         ▼
GameRecord ────────────────────────────► WishlistEntry
│  │  │                                  (library_id → GameRecord.id, optional)
│  │  │                                  "You own this on Epic"
│  │  │
│  │  └──[parent_id]──► GameRecord       Self-reference: DLC → base game
│  │                    (base game)       e.g. "Mortal Shell: Rotting Christ Pack"
│  │                         │                  └─► "Mortal Shell"
│  │                         │
│  │                    contents[]
│  │                    ContentItem
│  │                    └──[record_id]──► GameRecord
│  │                                     (standalone DLC record, optional)
│  │
│  └──[InstallSource]
│      └──[volume_id]──► StorageVolume ◄── Device
│                        (where installed)   └── StorageVolume[]
│                                                (composition)
│
└── stores{}
    └── StoreLink
        (owned: false entries = Steam cross-reference only,
         not a relationship to another entity)
```

---

## Relationships

### GameRecord → GameRecord (self)
- **Field:** `parent_id`
- **Cardinality:** many-to-one (many DLC records → one base game)
- **Optional:** yes — null for base games, set for standalone DLC records
- **Example:** Mortal Shell DLC record → Mortal Shell base record

### GameRecord → GameRecord (via ContentItem)
- **Field:** `contents[].record_id`
- **Cardinality:** many-to-one (many content items → one standalone DLC record)
- **Optional:** yes — null for `automatic` DLC, set for `separate` DLC
- **Purpose:** links a bundled content listing to its installable GameRecord

### GameRecord → StorageVolume (via InstallSource)
- **Field:** `install.sources[].volume_id`
- **Cardinality:** many-to-many (one game can be installed on multiple volumes;
  one volume holds many games)
- **Optional:** yes — only present when the game is installed
- **Example:** Cat Quest installed on Praxis → Stora (SD card volume)

### WishlistEntry → GameRecord
- **Field:** `library_id`
- **Cardinality:** one-to-one (one wishlist entry → at most one owned game record)
- **Optional:** yes — null when game is not owned anywhere
- **Purpose:** triggers owned flag in UI; used to display which store(s) it's owned on

### Device → StorageVolume
- **Field:** `storage_volumes[]`
- **Cardinality:** one-to-many (one device → many volumes)
- **Composition:** StorageVolume has no independent existence outside a Device
- **Example:** Praxis → [Internal SSD, Stora]

---

## Ownership vs Reference in StoreLink

A game's `stores` map can contain entries where `owned: false`. These are not
relationships to other entities — they are reference-only store entries added
during the Steam cross-reference step so the app can retrieve Steam Deck
verification status for non-Steam games.

```
GameRecord.stores.steam.owned = false   →  not owned on Steam
                                            but Steam App ID is known
                                            used for: steam_deck_verified, proton_rating
```

---

## AppConfig Dependencies (non-FK)

AppConfig is a singleton with no foreign key relationships, but it provides
credentials and settings consumed by every pipeline operation:

| AppConfig field     | Used by                              |
|---------------------|--------------------------------------|
| `steam.api_key`     | Ownership sync, wishlist sync        |
| `steam.steam_id`    | Ownership sync, wishlist sync        |
| `gog.*`             | Ownership sync, wishlist sync        |
| `igdb.*`            | Enrichment pipeline (Phase 1–2)      |
| `rawg.api_key`      | Enrichment pipeline (Phase 1–2)      |
| `itad.api_key`      | Enrichment pipeline (Phase 3), pricing sync |
| `pricing.*`         | Wishlist UI threshold indicators     |

---

## Schema File Index

| File                        | Contains                              |
|-----------------------------|---------------------------------------|
| `game_library.ts`           | GameRecord, all sub-types             |
| `wishlist.ts`               | WishlistEntry, all sub-types          |
| `device.ts`                 | Device, StorageVolume                 |
| `app_config.ts`             | AppConfig, credential configs         |
| `enrichment_pipeline.md`    | Enrichment flow, rehydration, sync modes |
| `sync_pipeline.md`          | Sync sources, change detection, Sync All sequence |
| `data_relationships.md`     | This file                             |
