# Enrichment Pipeline

All phases run in a single pass on manual sync trigger. Progress is shown per phase.

---

## Phase 1 — Identity Enrichment

### Step 1: Targeted store match
Search IGDB using the storefront-specific external ID where available
(IGDB maintains external ID mappings for Steam, GOG, and others).

```
Has storefront external ID in IGDB?
    ├─ Yes → exact match → set enrichment_status: matched → go to Phase 2
    └─ No  → fall through to title search
```

### Step 2: Title search fallback
```
Search IGDB by title
    ├─ 1 result  → auto-match → enrichment_status: matched → go to Phase 2
    ├─ 2+ results → present candidates to user (ranked by relevance)
    │               ├─ User selects one → enrichment_status: manual → go to Phase 2
    │               └─ User selects none → enrichment_status: needs_review
    │                                      fields remain null, skips to Phase 4
    └─ 0 results → try RAWG (Step 3)
```

### Step 3: RAWG fallback
```
Search RAWG by title
    ├─ 1 result  → auto-match → enrichment_status: matched → go to Phase 2
    ├─ 2+ results → present candidates to user
    │               ├─ User selects one → enrichment_status: manual → go to Phase 2
    │               └─ User selects none → enrichment_status: needs_review
    │                                      fields remain null, skips to Phase 4
    └─ 0 results → scrape store page (Step 4)
```

### Step 4: Store page scrape
```
Scrape store page for basic metadata
    ├─ Data found → enrichment_status: scraped → go to Phase 2
    └─ Nothing   → enrichment_status: needs_review
                   fields remain null, skips to Phase 4
```

**needs_review entries** are surfaced in a review queue for manual correction at any time.
User can revisit and either select a match or permanently mark as unmatched.

---

## Phase 2 — Field Population

Fields populated from the matched source. Store artwork is always preferred —
enrichment sources only fill gaps where store did not provide the field.

| Field              | IGDB | RAWG | Store scrape | Source priority       |
|--------------------|------|------|-------------|----------------------|
| `description`      | yes  | yes  | yes         | IGDB → RAWG → scrape |
| `short_description`| yes  | yes  | partial     | IGDB → RAWG → scrape |
| `genres`           | yes  | yes  | partial     | IGDB → RAWG → scrape |
| `release_date`     | yes  | yes  | yes         | IGDB → RAWG → scrape |
| `developer`        | yes  | yes  | partial     | IGDB → RAWG → scrape |
| `publisher`        | yes  | yes  | partial     | IGDB → RAWG → scrape |
| `artwork.cover`    | yes  | yes  | yes         | store → IGDB → RAWG  |
| `artwork.square`   | yes  | yes  | yes         | store → IGDB → RAWG  |
| `artwork.background`| yes | yes  | partial     | store → IGDB → RAWG  |
| `artwork.logo`     | no   | no   | yes         | store only           |
| `artwork.icon`     | no   | no   | yes         | store only           |

---

## Phase 3 — Cross-Reference Lookups

Runs for all games after Phase 2. Steam cross-reference runs for all non-Steam games.

### Steam cross-reference (non-Steam games only)
```
Search Steam store API by title
    ├─ Match found
    │   → add/update stores.steam with owned: false
    │   → fetch steam_deck_verified status from Steam API
    │   → if steam_deck_verified != 'verified'
    │       → fetch proton_rating from ProtonDB
    └─ No match
        → steam_deck_verified: null
        → proton_rating: null
```

### ITAD cross-reference (all games)
```
Look up ITAD ID via Steam App ID (if available) or title
    ├─ Match found
    │   → store itad_id in pricing.itad_id
    │   → fetch best_current, historical_low, store_prices
    │   → fetch active bundles → populate pricing.bundles[]
    │   → set pricing.last_synced
    └─ No match
        → pricing.itad_id: null
        → all pricing fields: null
```

---

## Phase 4 — DLC Population

Runs for all games after Phase 3.

```
For each game with DLC data from store import
    (Epic: dlcList entries, GOG: is_dlc flagged entries)

    For each DLC item:
        → check IGDB DLC relationships for parent game
        ├─ Found in IGDB
        │   → classify installation_type: separate | automatic
        │   → add to parent contents[] with record_id if separate
        │   → create standalone GameRecord for separate DLC
        └─ Not in IGDB
            → add to parent contents[] with installation_type from store data
            → mark DLC enrichment_status: needs_review
```

---

## Rehydration Mode

Triggered manually per-game or for the full library. Re-runs phases 1–4 but skips
ITAD pricing (handled by incremental sync). Respects user decisions — never silently
overrides a manual match.

| `enrichment_status` | Rehydration behaviour |
|---|---|
| `matched` | Re-fetch fields from same source using stored `igdb_id` — skip matching |
| `scraped` | Re-scrape store page, also retry IGDB/RAWG — may upgrade to `matched` |
| `manual` | Keep user's match — only re-fetch fields from stored `igdb_id`, never re-run matching |
| `needs_review` | Re-attempt full match, present new candidates if found |

Updates `last_enriched` on completion regardless of outcome.

---

## Review Queue

Games with `enrichment_status: needs_review` are surfaced in a dedicated review queue.
For each entry the user can:
- Search again with a corrected title
- Browse and select from presented candidates
- Mark as permanently unmatched (fields stay null, status stays needs_review but suppressed)
- Manually enter metadata directly

---

## Sync Modes

### Full enrichment
Runs once on initial setup, or triggered manually via "Re-enrich all".
Runs all 4 phases for every game. Expected time for ~2000 games: 20–30 min.
IGDB queries are batched (10 games per request) to reduce Phase 1 to under 1 minute.

### Incremental sync (default "Sync All")
Runs on demand. Only processes:
- New games not yet in the DB → full pipeline
- All games → ITAD pricing refresh (Phase 3 pricing only)
- Wishlist → fetch Steam + GOG wishlist updates

Skips metadata re-enrichment for already-enriched games.
Uses `last_enriched` to determine what is new.

---

## Rate Limits Reference

| Service  | Limit                    | Notes                          |
|----------|--------------------------|--------------------------------|
| IGDB     | 4 requests/sec           | ~2–3 min for 500 games         |
| RAWG     | 20,000 requests/month    | Only called for IGDB misses    |
| Steam    | 100,000 requests/24hr    | Cross-reference only           |
| ProtonDB | No official API          | Scraped — rate limit unknown   |
| ITAD     | Free tier limits apply   | 1 call per game for pricing    |
