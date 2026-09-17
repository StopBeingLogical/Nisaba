# GOGL-005 — delete the stale GOG wishlist entries

**Status:** prepared, not run · **Date:** 2026-09-16 · **Environment:** atlas + human

Not run here on purpose: this deletes rows from the live database, which is
never an executing model's to touch (`SESSION_SEED.md`, "Never yours"). `GOGL-004`
removed the pass that maintained these rows, so they are now orphaned data with
no writer. This task removes the rows themselves. Everything was prepared and
dry-run on a scratch copy so the real run is one command.

## The rows

Measured on `nisaba-perf.db`, the 2026-09-16 copy of the live database:

| Table | Rows carrying a `gog-wish-` id |
|:--|--:|
| `wishlist_entries` | 58 |
| `wishlist_stores` | 58 |
| `wishlist_price_history` | 39 |
| `wishlist_tags` | 0 |
| `wishlist_bundles` | 0 |
| `wishlist_resellers` | 0 |

7 of the 58 carry a current price. Removing them leaves **609** entries, which
is exactly the count of non-GOG entries in the file.

## The command

Every child table declares `REFERENCES wishlist_entries(id) ON DELETE CASCADE`
(`schema.sql:200-247`), so a single DELETE suffices — **but the pragma is
required**. The `sqlite3` CLI opens with `foreign_keys=OFF`, while the app's
connection opens with it on (`main.go:44`); without the pragma the delete
succeeds and leaves orphan child rows sitting in the database.

```bash
ssh truenas_admin@192.168.3.174 "sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db \"PRAGMA foreign_keys=ON; DELETE FROM wishlist_entries WHERE id LIKE 'gog-wish-%';\""
```

Then verify, expecting `0|0|0|609`:

```bash
ssh truenas_admin@192.168.3.174 "sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db \"SELECT (SELECT COUNT(*) FROM wishlist_entries WHERE id LIKE 'gog-wish-%'), (SELECT COUNT(*) FROM wishlist_stores WHERE wishlist_id LIKE 'gog-wish-%'), (SELECT COUNT(*) FROM wishlist_price_history WHERE wishlist_id LIKE 'gog-wish-%'), (SELECT COUNT(*) FROM wishlist_entries);\""
```

## Dry run (2026-09-16, throwaway copy)

```bash
$ cp nisaba-perf.db /tmp/del-check.db
$ sqlite3 /tmp/del-check.db "PRAGMA foreign_keys=ON; DELETE FROM wishlist_entries WHERE id LIKE 'gog-wish-%';"
$ sqlite3 /tmp/del-check.db "SELECT (SELECT COUNT(*) ... entries_left, stores_left, history_left, wishlist_total"
0|0|0|609
```

All 58 entries, all 58 store links and all 39 history rows went; no other table
moved.

## Handoff

- **What changed:** nothing yet. `GOGL-004` is committed (`7bda30f`) and the
  wishlist pass is gone, so these 58 rows are now stale by construction.
- **Last passing check:** `go build ./...`, `go vet ./...`, `go test ./...` pass
  at `7bda30f`; `scripts/spec-validate.sh` passes.
- **Blocker:** a deletion against
  `/mnt/MemoryAlpha/nisaba/data/nisaba.db`, which is Bobby's to run.
- **Safest next action:** run the command above, then record its two count lines
  in this file, labelled the way the task entry names them (the earlier counts,
  then the later ones). Those labels are the run's own output and are
  deliberately absent from this handoff, so the acceptance check cannot pass on
  a deletion that has not happened — this paragraph avoids printing them.
- **Rollback:** none once it has run. The pass that created these rows is deleted
  (`GOGL-004`), so they cannot be regenerated. Copy the 58 rows first if that
  matters.
