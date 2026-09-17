# DATA-001 — correct the multiple-stores indicator

**Date:** 2026-09-17 · **Environment:** local · **Ruling:** `spec/PRODUCT-5.md`

## Acceptance

```bash
$ go build ./... && go vet ./... && go test ./...
ok      nisaba/handlers
ok      nisaba/sync

$ grep -q 'g2.id != g.id' db/store.go && echo PRESENT || echo removed
removed
```

## The change

`multi_store_owned` asked whether **another game row** shared this row's
`igdb_id` and had an owned link. It now asks whether the game is owned on more
than one store:

```sql
-- was
CASE WHEN g.igdb_id IS NOT NULL AND EXISTS (
    SELECT 1 FROM games g2
    WHERE g2.igdb_id = g.igdb_id AND g2.id != g.id
      AND EXISTS (SELECT 1 FROM game_stores gs2 WHERE gs2.game_id = g2.id AND gs2.owned = 1)
) THEN 1 ELSE 0 END AS multi_store_owned

-- now
CASE WHEN (SELECT COUNT(*) FROM game_stores gs WHERE gs.game_id = g.id AND gs.owned = 1) >= 2
     THEN 1 ELSE 0 END AS multi_store_owned
```

`db/models.go:55` described the old semantics ("true if another game with the
same IGDB ID is owned in a different store") and now reads "true if the game is
owned on more than one store".

This is the only place the flag is computed. `GetGameDLCs` selects a literal `0`
for it, and the other `Scan` of the field is `ListGames`' own.

## Behavioural proof

Measured on the `.backup` copy (`AUDIT-001`'s working copy), using the shipped
expression itself rather than a paraphrase of it:

| | flagged |
|---|---:|
| before the fix (sibling rows) | 796 |
| after the fix (owned links) | **102** |

102 is the audit's independently measured count of games genuinely owned on 2+
stores, and the 796 → 102 movement is the 786 false positives plus the 92 false
negatives being corrected.

**After the dedupe the same expression reports 243**, which is the point of the
ordering: merging each group's store links onto one row legitimately *creates*
multi-store games, so the truthful number rises once the duplicates are gone.
The flag is measured before and after rather than only at one end.

## Performance

The library page is the instrument this round's earlier work set to under a
second, and the flag sits in its query, so it was checked rather than assumed.
The replacement is served by the index the earlier performance round added:

```
SEARCH gs USING COVERING INDEX idx_game_stores_game_id_owned (game_id=? AND owned=?)
```

A covering-index `COUNT` versus the correlated `EXISTS` it replaces, on the same
200-row page: `0.002s` against `0.003s` on the copy. The live page is confirmed
separately in `DEPLOY-007`; the copy is not the production instrument.
