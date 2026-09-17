# AUDIT-002 — second audit pass, post-cleanup

**Date:** 2026-09-17 · **Environment:** local + atlas (read-only on production) ·
**Ruling:** `spec/PRODUCT-6.md`

## Acceptance

```bash
$ grep -q before spec/evidence/AUDIT-002.md && grep -q after spec/evidence/AUDIT-002.md && echo ok
ok
```

## Method

Read-only. Every query ran against the live file via `sqlite3` over SSH; the one
copy taken (for the matcher dry run) was a `.backup`, and it was removed
afterwards. Nothing in this task wrote to production.

## What is verified clean

| check | result |
|---|---:|
| `PRAGMA integrity_check` | **ok** |
| `PRAGMA foreign_key_check` | **no rows** |
| orphan `game_stores` / `game_genres` / `game_contents` / `wishlist_stores` | **0 / 0 / 0 / 0** |
| `wishlist_entries.library_id` pointing at a missing game | **0** |
| `games.parent_id` pointing at a missing game | **0** |
| games with no owned store link | **0** |
| negative playtime · rating out of 1–5 | **0 · 0** |
| malformed `release_date` · sentinel dates (≥ 9000-01-01) · future dates | **0 · 0 · 0** |
| unparseable `artwork` JSON, games · wishlist | **0 · 0** |
| non-`http` store URLs | **0** |
| duplicate wishlist identities · duplicate store ids | **0 · 0** |
| `sync_errors` rows | **0** |
| games text: mojibake · untrimmed · doubled space · HTML entity | **0 · 0 · 0 · 0** |

This confirms the `PRODUCT-5` cleanup held: the earlier repairs were not undone
by any later sync, and **`game_stores.owned = 0` is still 0 rows**.

## Live shape after the cleanup

| | count |
|---|---:|
| `games` | **3390** |
| `game_stores` owned links | **3652** |
| `wishlist_entries` | 676 |
| distinct store identifiers | 3652 |

Store breakdown, all `owned = 1`:

| store | links |
|---|---:|
| steam | 1974 |
| gog | 1040 |
| epic | 216 |
| amazon | 138 |
| **steam family sharing** | **124** |
| xbox | 76 |
| playstation | 40 |
| ea app | 29 |
| battle.net | 8 |
| nintendo | 7 |

**The 124 family-sharing games are intact.** They carry `owned = 1` and are not
part of the `owned = 0` reference set that `DATA-003` deleted, so `DATA-003`'s
"still present" claim is confirmed rather than assumed.

## The gaps, all measured

| | count | share |
|---|---:|---:|
| games `needs_review` (never matched) | **428** | 12.6% |
| — of those, no `igdb_id` | **428** | all of them |
| games with no artwork | 66 | 1.9% |
| games with no release date | 409 | 12.1% |
| games with no description | 214 | 6.3% |
| games with no developer | 2089 | 61.6% |
| games with no publisher | **3390** | **100%** |
| wishlist no `igdb_id` · no artwork | 53 · 53 | the WISH-001 stragglers |

**`publisher` is empty on every row, and that is a code fact, not a sync
failure.** `InsertGame` (`db/store.go:1930`) does not list the column and
`EnrichGame` does not set it, so no path ever writes it. `IGDBGame`
(`sync/igdb.go:67`) does not request companies at all, so `developer` — filled
only by the Playnite post or the manual form — could never be backfilled by
enrichment either. Both are ruled on in `PRODUCT-6` §2.

## Duplicates

**7 groups remain, down from 718.** Five hold two genuine **owned** identifiers
for one store and cannot merge without discarding one, because `game_stores` is
keyed on `(game_id, store)`:

| group | the two owned identifiers |
|---|---|
| Grand Theft Auto III | `steam/12100` · `steam/12230` |
| Grand Theft Auto: Vice City | `steam/12110` · `steam/12240` |
| Grand Theft Auto: San Andreas | `steam/12120` · `steam/12250` |
| Dragon Age: Origins | `ea app/Origin.OFR.50.0001535` · `ea app/DR:138994900` |
| Mass Effect 2 (2010) | `ea app/Origin.OFR.50.0005262` · `ea app/OFB-EAST:56694` |

**The other 2 were blocked by an `owned = 0` reference link, not an owned one —
a miss in the `DATA-002` guard.** Replaying the dedupe's own grouping against the
pre-cleanup `.backup` shows why:

| group | survivor's owned link | the blocker the guard matched | on which row |
|---|---|---|---|
| Wasteland 2: Director's Cut | `steam/404730` | `steam/240760` | the **doomed** row's `owned = 0` reference |
| Heretic: Shadow of the Serpent Riders | `steam/2390` | `steam/3286930` | the **doomed** row's `owned = 0` reference |

The guard's `NOT EXISTS` joined `game_stores` without filtering `owned = 1`,
while the survivor ranking counted only `owned = 1` links. `steam/240760` and
`steam/3286930` each live on a **different** game, so those pairs held no real
conflict. `DATA-003` then deleted every `owned = 0` row, so both groups are
mergeable now. Both identifiers survive on their own games — nothing was lost, as
`DATA-002`'s 4294 → 4294 identifier guard had already established.

## Coherence and text

| finding | count |
|---|---:|
| wishlist entries with `best_current_price` < `historical_low_price` | **12** |
| wishlist titles carrying HTML entities | **3** |

The 3 entity titles (`Deck &amp; Conn`, `Dungeons &amp; Degenerate Gamblers`,
`Aether &amp; Iron`) are guaranteed IGDB misses because the stored title contains
the literal `amp`, so they cannot match on any normalisation. They are decoded
and re-enriched in the same pass.

## The matcher, measured against live IGDB

An enrichment backfill was dry-run against a `.backup` copy with real IGDB: it
matched **3 of 428**. Probing 14 of the unmatched titles directly showed **0
exact normalised matches** and **8 present under a variant name**:

| stored title | IGDB name |
|---|---|
| BioShock Infinite Complete Edition | BioShock Infinite: The Complete Edition |
| Alone in the Dark 1 | Alone in the Dark |
| Might and Magic 6 - The Mandate of Heaven | Might and Magic VI: The Mandate of Heaven |
| Warcraft: Orcs and Humans | Warcraft: Orcs & Humans |
| Modern Warfare 3 | Call of Duty: Modern Warfare 3 |
| GTA IV | Grand Theft Auto IV |

and 4 returned **no results at all** — `RollerCoaster Tycoon® Deluxe` among them,
the `®` breaking the search string itself.

`ENRICH-001` applies three spelling equivalences and re-measures **3/428 →
100/428, 0 errors**. Because the three changes only equate different *spellings*
of one name, none can match a different game. The remaining 328 need containment
or edition-stripping matching, which can pick a wrong entry, and are left unruled.

## Not touched

The 124 family-sharing games, the 5 unmergeable groups, the dormant
`steam_deck_verified` / `proton_rating` fetchers, and anything in `OPEN.md`.
