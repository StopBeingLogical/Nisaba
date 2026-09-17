# DATA-004 — merge the 2 duplicate groups the reference-link guard blocked

**Date:** 2026-09-17 · **Environment:** atlas (Bobby confirmed) · **Ruling:**
`spec/PRODUCT-6.md`

## Acceptance

```bash
$ grep -q before spec/evidence/DATA-004.md && grep -q after spec/evidence/DATA-004.md && echo ok
ok
```

## Why these two

`DATA-002`'s conflict guard joined `game_stores` **without filtering
`owned = 1`**, while the survivor ranking counted only owned links. Two groups
were therefore excluded by an `owned = 0` **reference** link — one that lives on a
*different* game and that no query reads. `DATA-003` then deleted every
`owned = 0` row, leaving those two groups duplicated with no conflict at all.

Replayed against the pre-cleanup `.backup`, the guard's match:

| group | survivor's owned link | blocker the guard matched | where it actually lives |
|---|---|---|---|
| Wasteland 2: Director's Cut | `steam/404730` | `steam/240760` | game `c4c8d504` — the *doomed* row's reference link |
| Heretic: Shadow of the Serpent Riders | `steam/2390` | `steam/3286930` | game `44fb9d09` — the *doomed* row's reference link |

The 5 other groups hold two genuine **owned** identifiers for one store, which
`game_stores`' `(game_id, store)` key cannot hold at once, so they stay.

## Guards, read on live before anything was written

All **0**:

| guard | result |
|---|---:|
| doomed rows carrying user data (favourite, rating, notes, play_status, installed, hidden) | 0 |
| wishlist entries whose `library_id` points at a doomed row | 0 |
| games whose `parent_id` points at a doomed row | 0 |

Exactly **2** rows were doomed, and the map named them as intended:

| doomed | title | survivor | title |
|---|---|---|---|
| `517cb9c6…` | Wasteland 2 Director's Cut | `4d949957…` | Wasteland 2: Director's Cut |
| `aacfacd9…` | Heretic: Shadow of the Serpent Riders | `a2aed31c…` | Heretic: Shadow of the Serpent Riders |

## Before and after

| | before | after |
|---|---:|---:|
| `games` | 3390 | **3388** |
| `game_stores` owned links | 3652 | **3652** |
| distinct `(store, store_id)` pairs | 3652 | **3652** |
| games with no owned link | 0 | **0** |
| duplicate identity groups | 7 | **5** |
| excess duplicate rows | 7 | **5** |
| `multi_store_owned` flagged | 243 | **245** |

**Nothing was lost.** The owned link count and the distinct identifier count are
both unchanged, and the merged games now carry both stores:

```
Wasteland 2: Director's Cut            steam/404730 | gog/1444386007
Heretic: Shadow of the Serpent Riders  steam/2390   | gog/1290366318
```

The two identifiers that caused the earlier exclusion still exist on their own
games — `steam/240760` on `c4c8d504`, `steam/3286930` on `44fb9d09` — so the
`DATA-002` "no identifier lost" guarantee holds here too.

The indicator rises 243 → **245** because two games that were previously split
across two rows are now one row owned on two stores. That is the intended effect
of the merge, not a regression.

`PRAGMA quick_check` is `ok` and `PRAGMA foreign_key_check` returns nothing.

## Remaining

The 5 groups that cannot merge without discarding an owned identifier, all
verified still present with one store each: Grand Theft Auto III, Vice City and
San Andreas (`steam` ×2 each), Dragon Age: Origins and Mass Effect 2 (2010)
(`ea app` ×2 each).

## Backups

On Ergaster at `/tmp/nisaba-backup-2026-09-17b/`: `.dump` files for `games`,
`game_stores`, `game_genres`, `game_contents`, `game_tags`,
`game_install_sources` and `wishlist_entries`, taken immediately before the
merge.
