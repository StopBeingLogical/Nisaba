# Sync Pipeline

The sync panel splits into three independent operations, each triggerable separately
or together via "Sync All".

---

## 1. Ownership Sync

Pulls from storefront APIs directly. Works from any device, any browser.
After completion, any new games not yet in the DB are passed to the enrichment pipeline.

### Steam
```
GET IPlayerService/GetOwnedGames/v1?steamid={id}&include_appinfo=1&include_played_free_games=1
    → returns: appid, playtime_forever, playtime_2weeks, name, img_icon_url

Batch GET store.steampowered.com/api/appdetails?appids={ids}
    → enriches: description, genres, release_date, art (fallback if IGDB misses)
```

### GOG
```
GET embed.gog.com/user/data/games          → returns list of owned product IDs
GET embed.gog.com/account/getFilteredProducts (paginated)
    → returns: id, title, developer, worksOn, image, url, releaseDate, genres, tags
Auth: Bearer token — auto-refreshed from stored refresh_token if expired
```

### Epic
```
OAuth token exchange (stored credentials, Legendary-style flow)
GET launcher-public-service-prod06.ol.epicgames.com/launcher/api/public/assets/
    → returns: appName, labelName, buildVersion, catalogItemId, namespace
```

### Amazon
```
OAuth token (stored Nile credentials)
GET nile library endpoint
    → returns: app_name, title, developer, genres, releaseDate, art
```

### EA / Ubisoft
```
EA:      No working API — ownership sync not available
Ubisoft: Deferred — Demux protocol complexity out of scope for initial release
```

### Change detection (all stores)
```
For each game in API response:
    ├─ Not in DB → create GameRecord, queue for enrichment pipeline
    ├─ In DB, no changes → skip
    └─ In DB, store data changed (playtime, etc.) → update store link fields only

For each game in DB not in API response:
    └─ Flag as potentially removed (do not auto-delete — prompt user to confirm)
```

---

## 2. Install State Sync

Reads local device files through the browser using the File System Access API.
The sync panel auto-detects the current device via browser user-agent, with manual
override available.

| Device | Detection signal | Storefront sources |
|--------|-----------------|-------------------|
| Praxis | SteamOS user-agent | Heroic files (Epic, GOG, Amazon), Steam local |
| Ergaster | Windows user-agent | Native manifests (Epic), GOG Galaxy DB, Steam local |

### Praxis — Heroic files
```
~/.var/app/com.heroicgameslauncher.hgl/config/heroic/

Epic (installed):
    legendaryConfig/legendary/installed.json
    → fields: app_name, install_path, install_size, version, executable, platform

GOG (installed):
    gogdlConfig/heroic_gogdl/manifests/{id}
    → fields: app_name, install_path, install_size, version, platform

Amazon (installed):
    nile_config/nile/installed.json
    → fields: app_name, install_path, install_size, version, platform
```

### Praxis — Steam (local)
```
~/.steam/steam/steamapps/libraryfolders.vdf     → discover all library paths
~/.steam/steam/steamapps/appmanifest_*.acf      → per-game install metadata
    → fields: appid, installdir, SizeOnDisk, buildid, LastUpdated
```

### Ergaster — Epic (native)
```
C:\ProgramData\Epic\EpicGamesLauncher\Data\Manifests\*.item
    → fields: AppName, InstallLocation, InstallSize, AppVersionString, LaunchExecutable
```

### Ergaster — GOG Galaxy (SQLite)
```
C:\ProgramData\GOG.com\Galaxy\storage\galaxy-2.0.db
    Table: InstalledExternalProducts / GamePieces
    → fields: productId, installationPath, version
```

### Ergaster — Steam (local)
```
C:\Program Files (x86)\Steam\steamapps\libraryfolders.vdf
C:\Program Files (x86)\Steam\steamapps\appmanifest_*.acf
    → same fields as Praxis Steam
```

### Install state change detection
```
For each installed game found in device files:
    ├─ GameRecord exists, not marked installed on this volume
    │   → add InstallSource entry for this volume_id
    ├─ GameRecord exists, already marked installed, version changed
    │   → update InstallSource.version
    └─ GameRecord does not exist
        → create GameRecord (ownership may not be synced yet), queue for enrichment

For each game in DB marked installed on this device's volumes but not found in files:
    └─ Update is_installed, remove InstallSource entry for this volume
```

---

## 3. Pricing Sync

Hits ITAD API directly. Works from any device, any browser.
Only runs for games that have a stored `itad_id`. Games without one are skipped
(ITAD cross-reference is handled by the enrichment pipeline).

```
Batch GET api.isthereanydeal.com/games/prices → current prices per store
Batch GET api.isthereanydeal.com/games/lowest → historical lows
Batch GET api.isthereanydeal.com/games/bundles → active bundle appearances

For each result:
    → update pricing.best_current
    → update pricing.historical_low
    → update pricing.store_prices
    → replace pricing.bundles[] with current active bundles
    → set pricing.last_synced

Wishlist entries with active bundles or price ≤ target_price
    → flagged in UI for user attention
```

---

## 4. Wishlist Sync

Pulls from Steam and GOG APIs. Works from any device, any browser.

### Steam
```
GET IWishlistService/GetWishlist/v1?steamid={id}
    → returns: appid, priority, date_added per item

Batch GET store.steampowered.com/api/appdetails?appids={ids}
    → enriches: available, coming_soon, in_development, store_url
```

### GOG
```
GET embed.gog.com/account/wishlist/search (paginated, sortBy=date_added)
    → returns full product objects including availability, pricing, native_client
Auth: Bearer token — auto-refreshed from stored refresh_token if expired
```

### Change detection
```
For each item in wishlist API response:
    ├─ Not in wishlist DB → create WishlistEntry, queue for enrichment pipeline
    │   Check GameRecord DB for matching library_id (owned flag)
    └─ Already in wishlist DB → update store entry fields (availability, priority)

For each item in wishlist DB not in API response:
    └─ Remove WishlistEntry (user removed it from their store wishlist)
```

---

## Sync All sequence
```
1. Ownership Sync     (all stores in parallel)
2. Wishlist Sync      (Steam + GOG in parallel)
3. Pricing Sync       (ITAD batch)
4. Install State Sync (current device only)
5. Enrichment Pipeline — incremental (new games only)
```

Steps 1–3 run in parallel. Step 4 runs concurrently with 1–3.
Step 5 starts after steps 1–4 complete.
