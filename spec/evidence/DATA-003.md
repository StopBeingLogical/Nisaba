# DATA-003 — repair the remaining data debris

**Date:** 2026-09-17 · **Environment:** atlas (Bobby confirmed) · **Ruling:**
`spec/PRODUCT-5.md`

## Acceptance

```bash
$ grep -q before spec/evidence/DATA-003.md && grep -q after spec/evidence/DATA-003.md && echo ok
ok
```

## Before and after

All six repairs, on live, in one transaction with foreign keys on:

| repair | before | after |
|---|---:|---:|
| mojibake titles | 3 | **0** |
| whitespace-damaged titles | 12 | **0** |
| `9998-12-30` sentinel dates | 1 | **0** |
| `owned = 0` store links | 653 | **0** |
| `sync_errors` older than April | 131 | **0** |
| scratch tables left behind | 0 | **0** |

Not 3 and 13 as the audit measured, and not 748 reference links: `DATA-002` ran
first and removed the copies that lived on duplicate rows. The audit's figures
were the pre-dedupe state. The live counts were re-measured before this ran
rather than carried over.

## The titles

Accents restored, written by character code so the statement carries no
non-ASCII of its own:

```
Pokémon Sword · Brütal Legend · Pokémon HOME
```

Whitespace trimmed and de-doubled, e.g.

```
Amnesia:  The Dark Descent        → Amnesia: The Dark Descent
Ultima™  Underworld I             → Ultima™ Underworld I
 Wanba Warriors                   → Wanba Warriors
Warhammer 40,000: Dawn of War - Anniversary Edition ␠ → …Anniversary Edition
```

**`sort_title` was rebuilt for exactly those rows**, using the same rule as
`makeSortTitle` in `db/store.go`: drop a leading `the`/`a`/`an`, otherwise the
title verbatim. A title change without its sort key would have left those rows
sorting under the old spelling. The set of affected ids was captured before the
updates rather than inferred after them.

## The reference links

653 `owned = 0` rows deleted, leaving `game_stores` at 3652 rows, all of them
`owned = 1`. Every one of the 12 `game_stores` reads in the codebase filters
`owned = 1`, so nothing reads them — they were the residue of `SyncSteamCrossRefs`,
which has had no caller since commit `9557160`.

**Worth stating plainly rather than burying:** 642 of those identifiers —
`(store, store_id)` pairs — appeared on *no* owned link, so they are now gone from
the database. The ruling was explicit and the effect is nil because nothing
consults `owned = 0`, but the numbers are recorded here rather than in a claim
that "nothing changed". Recoverable from `game_stores.sql` in the backup set.

## The stale errors

131 rows, 130 `proton` and 1 `pricing`, all spanning 2026-03-12 to 2026-03-19,
deleted. They came from the ProtonDB scraper, which lost its caller in commit
`9557160` along with the other dormant fetchers in `OPEN.md`. `sync_errors` is
now empty.

## Not touched

**The 124 Steam Family Sharing games are untouched**, confirmed still present
after the run — Bobby did not select them and they remain an unruled question in
`PRODUCT-5.md`. Also untouched: `wishlist_entries` (676, 622 with artwork) and
`sync_log` (89 rows).

## Backups

`/tmp/nisaba-backup-2026-09-17/` on Ergaster holds the pre-change `.dump` for
every table this touched, taken immediately before `DATA-002` ran and therefore
holding rows from both operations.

## Live state after the whole round

| | before the round | now |
|---|---:|---:|
| `games` | 4101 | **3390** |
| `game_stores` | 4402 | 3652 |
| duplicate identities | 718 | 7 (intentional) |
| games with no store link | 549 | **0** |
| games flagged multi-store | 796 (wrong) | **243** |
| `sync_errors` | 131 | **0** |
| rendered library | 4101 tiles | **3390 tiles** |
| `/library` page 1 | 0.078s | **0.070s** |

`PRAGMA quick_check` is `ok` and `PRAGMA foreign_key_check` returns nothing.
