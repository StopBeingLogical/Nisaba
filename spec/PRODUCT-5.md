# Round 2026-09-17 — data audit cleanup

Locked owner requirements. This file supersedes `PRODUCT-4.md` where they
disagree; the earlier file stays as the record of what was ruled then.

## The report

Bobby, 2026-09-17: *"Let's do a full audit and clean up of the data."*

The audit is `evidence/AUDIT-001.md` — read-only, from a `.backup` copy, with no
write to production. Its founding result is that **the database is not corrupt**:
`integrity_check` is `ok`, `foreign_key_check` returns nothing, and no value is
out of range (no non-positive prices or history rows, no negative playtime, no
malformed dates, no non-`http` store URLs, and no wishlist duplicates). The
damage is duplication and debris, and every category below is measured.

## Ruled, 2026-09-17

### 1. The multiple-stores indicator is corrected **before** the duplicates go

`multi_store_owned` (`db/store.go:108-111`) asks whether *another game row* shares
this row's `igdb_id` and has an owned link — not whether *this game* is owned on
more than one store. Measured both ways: **796 games flagged, 102 actually owned
on 2+ stores** — 786 false positives and 92 false negatives.

**The duplicate rows are load-bearing for that flag.** Removing them while leaving
the query alone would collapse it to near-meaningless, so this is one change in
two parts and the order is mandated: **correct the query, then dedupe.** The
correction is a `COUNT` over `game_stores` filtered on `owned = 1`, which the
existing `idx_game_stores_game_id_owned` index serves as a covering index. The
field's comment in `db/models.go:55` describes the old semantics and is corrected
with it.

### 2. The 718 duplicate game rows are merged, then deleted

4101 rows hold 3383 distinct games. Identity is `igdb_id` where set, else the
exact title. 549 of the duplicates have **no store link at all** and were created
on 2026-04-27, in the window of the Playnite rollout, before title deduplication
worked (`af6f843`, the previous day). A second, independent mode puts the same
Steam app id on 2–3 rows — 108 excess.

The merge keeps one row per identity, preferring a row that has owned store links
and then the earliest, and carries across the doomed rows' **266 store links, 1865
genres and 49 content rows**, plus the 68 rows holding playtime and 67 holding
`last_played`. Only then are the surplus rows deleted.

`is_favorite`, `rating`, `notes`, `play_status` and `is_installed` are unset on
every doomed row, so **no user-authored data is at risk** — measured, not assumed.
Zero wishlist entries and zero `parent_id` references point at a doomed row, so
nothing outside the game tables needs repointing.

Not a migration: migrations are additive only. A one-off, by hand, like the
2026-09-16 scorched earth and the `GOGL-005` deletion.

### 3. Text is repaired

- **3 mojibake titles** carrying a literal U+FFFD: `Pokémon Sword`, `Brütal
  Legend`, `Pokémon HOME` get their letters back.
- **13 whitespace-damaged titles** are trimmed and de-doubled. These sort wrongly
  and are part of why some never matched IGDB.
- **3 sentinel dates**: `Retro Classics` ×3 carry `release_date = '9998-12-30'`
  from the same April batch; they become NULL, because 9998 is not a date.

### 4. The 748 `owned = 0` store links are deleted

All 12 `game_stores` reads in the codebase filter `owned = 1`, so nothing reads
them. They are the residue of `SyncSteamCrossRefs`, which has had no caller since
commit `9557160` and sits in `OPEN.md` as one of three dormant fetchers. Removing
them takes `game_stores` from 4402 to 3654 rows with no behavioural change.

### 5. The 131 stale `sync_errors` are cleared

130 `proton` and 1 `pricing`, all between 2026-03-12 and 2026-03-19, from the
ProtonDB scraper that lost its caller in June. Six months of noise in Recent
Activity.

## Deliberately NOT cleaned

**The 124 Steam Family Sharing games stay.** Bobby did not select them. They are
games he does not own, each with only a `steam family sharing` link and a badge
in the library, and the audit's view is that they misrepresent ownership — but
that is recorded as an unruled question, not acted on. The Playnite script
already excludes the category, so nothing will re-add them.

This is the second open question the audit raised; the other is that **Install
state tracking is advertised in `README.md` and has never stored a row**
(`devices`, `storage_volumes` and `game_install_sources` are all empty). Neither
is derived into a task.

## Ordering

The indicator correction must reach Atlas **before** the dedupe runs, or the flag
sits at zero for every game in the window between. Everything else — text,
reference links, stale errors — is independent of both and can land in any order.
