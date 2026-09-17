# DATA-002 — merge and delete the duplicate game rows

**Date:** 2026-09-17 · **Environment:** atlas (Bobby confirmed) · **Ruling:**
`spec/PRODUCT-5.md`

## Acceptance

```bash
$ grep -q before spec/evidence/DATA-002.md && grep -q after spec/evidence/DATA-002.md && echo ok
ok
```

## Guards, read on live before anything was written

All four returned **0**:

| guard | result |
|---|---:|
| doomed rows carrying user data (favourite, rating, notes, play_status, installed, hidden) | 0 |
| doomed store links that would conflict with the survivor's | 0 |
| wishlist entries whose `library_id` points at a doomed row | 0 |
| games whose `parent_id` points at a doomed row | 0 |

The first is the one that matters most: it is the measured confirmation, rather
than the assumption, that **no user-authored field is lost**. The last two mean
nothing outside the game tables needed repointing.

## Before and after

| | before | after |
|---|---:|---:|
| `games` | 4101 | **3390** |
| duplicate identities | 718 | **7** |
| games with no store link | 549 | **0** |
| `game_stores` | 4402 | 4305 |
| `game_genres` | 11454 | 9637 |
| `game_contents` | 1264 | **1264** |
| distinct `(store, store_id)` pairs | **4294** | **4294** |
| games with playtime > 0 | 477 | 418 |
| games with `play_time_minutes` = 0 | 0 | **0** |

711 rows were merged and deleted — 718 flagged, minus the 7 groups excluded
below. `PRAGMA quick_check` is `ok` and `PRAGMA foreign_key_check` returns
nothing after the run.

**Nothing was lost.** All 4294 distinct `(store, store_id)` pairs survive the
merge, every one of the 1264 DLC/content rows moved intact, and no game is left
without a store link. The `game_genres` and `game_stores` row counts fall
because duplicate rows collapsed, not because data went away.

`games` with playtime falls 477 → 418 because 62 doomed rows held playtime and
their values were carried onto the survivors; the arithmetic is 477 − 62 plus
gains, and the zero column staying at zero is the check that no row was given a
spurious value.

## A bug the dry run caught

The first version merged the scalars with a correlated subquery against `games`
*inside an `UPDATE` of `games`*. SQLite does not bind that subquery to the row
being updated. A minimal reproduction showed a survivor inheriting **an
unrelated group's playtime**, and on the full copy it inflated games-with-playtime
from 477 to **645**.

The rewrite precomputes a `merge_vals` table per survivor and reads only from
that. The same run then gave 418. On live the earlier version would have written
wrong playtimes across the library — which is why the dry run is not optional
for an operation like this.

## The 7 groups deliberately left merged

`game_stores` allows one link per store per game, so a group holding two
different `store_id`s for the same store cannot be merged without discarding an
identifier. Those rows were excluded rather than merged:

| group | the two identifiers |
|---|---|
| Mass Effect 2 (2010) | `ea app/Origin.OFR.50.0005262` · `ea app/OFB-EAST:56694` |
| Dragon Age: Origins | `ea app/Origin.OFR.50.0001535` · `ea app/DR:138994900` |
| Grand Theft Auto: San Andreas | `steam/12250` · `steam/12120` |
| Grand Theft Auto III | `steam/12230` · `steam/12100` |
| Heretic: Shadow of the Serpent Riders | `steam/2390` · `steam/3286930` |
| Grand Theft Auto: Vice City | `steam/12240` · `steam/12110` |
| Wasteland 2: Director's Cut | `steam/404730` · `steam/240760` |

Both GTA entries report the same Steam name, so those pairs are genuinely one
game under two app IDs. **`steam/3286930` is not**: it resolves to
**"Heretic + Hexen"**, a different product stored here under Heretic's title, so
merging would have erased a real ownership record. All 7 remain visible as two
tiles; merging them on the survivor's identifier is a one-statement change if
Bobby would rather have the tidier library.

## Visible result

Spot-checked against the audit's worst groups, on live:

| group | rows before | rows now |
|---|---:|---:|
| `Fallout 2` | 5 | **1** (gog + family sharing) |
| `Record of Lodoss War-Deedlit in Wonder Labyrinth-` | 8 | **1** (xbox + family sharing) |
| `Baldur's Gate II: Enhanced Edition` | 6 | **1** (gog + amazon + family sharing) |
| `Jade Empire: Special Edition` | — | **1** (steam + ea app) |
| `Dragon Age: Origins` | 8 | **2** (excluded above) |

The rendered library now reports **3390 games**, exactly the distinct-identity
count, against 4101 before.

## Backups

On Ergaster at `/tmp/nisaba-backup-2026-09-17/`: full `.dump` files for `games`,
`game_stores`, `game_genres`, `game_contents`, `game_tags`,
`game_install_sources`, `wishlist_entries`, `wishlist_stores`,
`wishlist_price_history` and `sync_errors`, plus `doomed-711.json` — every
deleted row with its id, its survivor, and its title.

The merge ran in a single `BEGIN IMMEDIATE` transaction with foreign keys on; the
scratch `dup_map` and `merge_vals` tables were dropped afterwards and confirmed
absent.
