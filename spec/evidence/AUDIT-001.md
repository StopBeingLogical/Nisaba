# AUDIT-001 — full data audit

**Date:** 2026-09-17 · **Environment:** local, against a `.backup` copy of the live
database (676 wishlist entries, 4101 games — the post-`DEPLOY-006` state)

## Acceptance

```bash
$ sqlite3 /tmp/nisaba-audit.db "PRAGMA integrity_check;"
ok
$ sqlite3 /tmp/nisaba-audit.db "PRAGMA foreign_key_check;"
(no rows)
```

## Method

A read-only copy was taken with `sqlite3 .backup` on Atlas, pulled to Ergaster,
and queried there; the remote scratch copy was deleted immediately. **No write
of any kind was made to the live database for this audit.** Nothing was deleted,
merged or repaired.

## What is clean

Worth stating, because it narrows the cleanup sharply:

- `PRAGMA integrity_check` → `ok`. `PRAGMA foreign_key_check` → no rows, against
  a schema with foreign keys enabled.
- No orphaned child rows in any table.
- **No bad values**: zero non-positive prices (wishlist or history), zero
  negative playtime, zero malformed `release_date`, zero non-`http` `store_url`,
  zero games whose only store links are reference-only.
- `wishlist_entries` has **no duplicates** by `igdb_id` or by title.

So the damage is not corruption. It is duplication and debris.

## F1 — 718 duplicate game rows (the headline)

```
rows 4101 · distinct identities 3383 · excess 718
```

Identity is `igdb_id` where set, else the exact title. 1015 rows sit inside a
duplicate group: **549 of them have no store link at all**, 466 have one.

**Cause, dated precisely.** All 549 no-store rows were created on **2026-04-27**,
in the window of the Playnite sync rollout (`2a060c9` 2026-04-26 "Add Playnite
automated library sync", `af6f843` "Add title-based deduplication fallback",
`688dded` "Add makeSortTitle helper", `dda5af6` 2026-04-27 "Exclude Steam Family
Sharing"). The sync inserted a game row per posted entry before the title-based
dedup fallback worked, and those rows never received a store link.

**Every one is redundant**: all 549 have a same-titled sibling that *does* have a
store link, and 483 share an `igdb_id` with it. 483 carry full IGDB metadata
(art, description, release date) — they were inserted and then enriched as if
they were real games.

Worst groups: `Dragon Age: Origins` ×8, `Mass Effect 2 (2010)` ×8, `Record of
Lodoss War-Deedlit in Wonder Labyrinth-` ×8, `Baldur's Gate II: Enhanced
Edition` ×6, `Fallout 2` ×5.

**Visible?** Yes. `ListGames` joins `game_stores` only when a store *filter* is
applied (`db/store.go:74`), so a row with no store link is still listed — 4101
tiles for 3383 games, each duplicate rendered with no store badge.

**A second, independent duplication mode:** the same Steam app id on more than
one game row — 100 store ids, 208 rows, **108 excess**. These rows *do* have
links, so they are not part of the 549.

## F2 — `multi_store_owned` is computed from the duplicates, not from the links

This is the finding that gates the cleanup. `db/store.go:108-111`:

```sql
CASE WHEN g.igdb_id IS NOT NULL AND EXISTS (
    SELECT 1 FROM games g2
    WHERE g2.igdb_id = g.igdb_id AND g2.id != g.id
      AND EXISTS (SELECT 1 FROM game_stores gs2 WHERE gs2.game_id = g2.id AND gs2.owned = 1)
) THEN 1 ELSE 0 END AS multi_store_owned
```

It asks *"does another game row share my IGDB id and have an owned link?"* —
not *"am I owned on more than one store?"*. Measured both ways:

| Definition | Games |
|---|---:|
| ≥2 owned store links (correct) | **102** |
| sibling row with the same `igdb_id` (as shipped) | **796** |

**786 false positives** — games flagged as owned on multiple stores that are
owned on exactly one — and **92 false negatives** — games genuinely owned on 2+
stores that are not flagged.

Correcting it is not optional if the duplicates are removed: it is the only
reason the flag currently returns the right answer for some games. **Deleting
the 644 redundant rows while leaving this query alone would collapse the flag to
near-meaningless.** Fix the query first, then dedupe, or the indicator changes
meaning twice.

## F3 — 124 Steam Family Sharing games he does not own

124 games whose only store link is `store = 'steam family sharing'`, 124 distinct
games, every one rendering a `steam family sharing` badge in the library. The
Playnite script now filters this category (commit `dda5af6`, the day the rows
appeared), and `README.md` states only personal ownership is tracked — but the
rows predate the filter and were never removed. These are games he has **not**
bought.

## F4 — 748 `owned = 0` store links, read by nothing

748 rows with `owned = 0`, all of them on a game row that also has an `owned = 1`
link. **Every one of the 12 `game_stores` reads in the codebase filters
`owned = 1`** (`db/store.go:74,106,110,181,290,363,431,1127,1133,1769,2007,2459`).
They are the residue of `SyncSteamCrossRefs`, which has had no caller since
commit `9557160` and is one of the three dormant fetchers in `OPEN.md`. Inert,
but they are 748 rows of dead weight whose only distinguishing feature is a flag
nothing consults.

## F5 — text integrity

- **3 mojibake titles**, with a literal U+FFFD replacement character where a
  non-ASCII letter belongs: `Pok\xEF\xBF\xBDmon Sword` → `Pokémon Sword`,
  `Br\xEF\xBF\xBDtal Legend` → `Brütal Legend`, `Pok\xEF\xBF\xBDmon HOME` →
  `Pokémon HOME`.
- **13 titles with whitespace damage** — trailing spaces
  (`Warhammer 40,000: Dawn of War - Anniversary Edition `), doubled spaces
  (`Amnesia:  The Dark Descent`, `Heroes Of  Loot: Gauntlet Of Power`),
  and a leading space (` Wanba Warriors`). These sort wrongly and are why some
  of them miss IGDB matching.
- **3 wishlist titles stored HTML-escaped** (`Deck &amp; Conn`,
  `Dungeons &amp; Degenerate Gamblers`, `Aether &amp; Iron`) — recorded in
  `WISH-001`. Zero `games` rows are affected.

## F6 — 3 sentinel release dates

`Retro Classics` ×3, `release_date = '9998-12-30'`, `igdb_id` 0, created
2026-04-27 in the same batch as F1. A placeholder, not a date.

## F7 — 131 stale `sync_errors` rows

130 `proton` + 1 `pricing`, spanning **2026-03-12 to 2026-03-19** — six months
old, all from the ProtonDB scraper, which has had no caller since `9557160`. The
ProtonDB columns they refer to are themselves stale (1779 games carry
`steam_deck_verified`, 826 carry `proton_rating`, all from before that commit).

## F8 — missing artwork, already covered

54 wishlist entries and 148 library games without a cover. Both were audited and
ruled in their own rounds (`WISH-001`, `PRODUCT-4.md`) and are not re-litigated
here.

## F9 — nine tables with zero rows

`devices`, `storage_volumes`, `game_install_sources`, `game_tags`,
`enrichment_queue`, `wishlist_bundles`, `wishlist_resellers`, `wishlist_tags`,
`mystery_pack_price_history`. Not defects — but worth knowing that **Install
state tracking is advertised in `README.md` and has never stored a row.**

## What cleanup would require — decisions, not work

This audit is complete. Nothing below has been done, and none of it is a task.

1. **Fix `multi_store_owned` before any dedup** (F2). Small, code, required first.
2. **Dedupe the 718 rows** (F1) — merge store links, and the 68 rows carrying
   playtime plus 67 carrying `last_played` onto the survivor, then delete.
   `is_favorite`, `rating`, `notes`, `play_status` and `is_installed` are unset
   on every duplicate, so no user-authored data is at risk. No migration can do
   this: migrations are additive only.
3. **Remove the 124 family-sharing games** (F3).
4. **Delete the 748 `owned = 0` links** (F4) — or keep them and say why.
5. **Repair text** (F5, F6): 3 mojibake titles, 13 whitespace, 3 sentinel dates.
6. **Clear the 131 stale `sync_errors`** (F7).

Items 1 and 2 are coupled and must be one change. Everything else is
independent.
