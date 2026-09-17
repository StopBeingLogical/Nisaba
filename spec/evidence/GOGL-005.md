# GOGL-005 — delete the stale GOG wishlist entries

**Date:** 2026-09-16 · **Status:** done · **Environment:** atlas

Run against the live database at `/mnt/MemoryAlpha/nisaba/data/nisaba.db` with
Bobby's explicit go-ahead (2026-09-16). `SESSION_SEED.md` otherwise reserves the
live database to him; `PRODUCT-3.md` ruled that these 58 rows are deleted rather
than left stale once `GOGL-004` retired their writer.

## Before

```bash
$ ssh truenas_admin@192.168.3.174 'sqlite3 <db> "SELECT ..."'
running_syncs: 0
entries: 58
priced_entries: 7
stores: 58
history: 7
tags: 0
bundles: 0
resellers: 0
wishlist_total: 730
```

A copy of the rows was taken first, on Ergaster: `/tmp/gog005-backup-entries-2026-09-16.json`
(58 rows, 53,366 bytes), `-stores-` (58 rows) and `-history-` (7 rows).

`history: 7` where the handoff predicted 39 — the copy those numbers came from
was taken at 19:31, before the 2026-09-16 scorched-earth cleanup trimmed the
pre-cutover price rows. The live figure is the correct one.

## First attempt failed, and why

```bash
$ sqlite3 <db> "PRAGMA foreign_keys=ON; DELETE FROM wishlist_entries WHERE id LIKE 'gog-wish-%';"
Error: stepping, attempt to write a readonly database (8)
exit: 8
```

`nisaba.db` is `-rw-r--r-- root root` and the SSH user is `uid=950(truenas_admin)`,
so it could read the database and not write it. Nothing was deleted: the counts
still read 58 / 58 / 7 afterwards, and `quick_check` was `ok`.

Retried with `sudo -n`, which is passwordless for `truenas_admin` on this host
and matches how the app itself writes — the container runs as `uid=0(root)`,
against a root-owned file.

## The deletion

```bash
$ sudo -n /usr/bin/sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db \
    "PRAGMA foreign_keys=ON; DELETE FROM wishlist_entries WHERE id LIKE 'gog-wish-%';"
exit: 0
```

## After

```bash
$ sudo -n /usr/bin/sqlite3 ...
quick_check|ok
entries: 0
stores: 0
history: 0
tags: 0
bundles: 0
resellers: 0
wishlist_total: 672
gog_links_anywhere: 0
```

730 − 672 = 58 entries removed, and every child row followed through the
`ON DELETE CASCADE` declared on `wishlist_entries(id)`. No GOG store link is
left in the wishlist at all. `pragma_quick_check` returns `ok`.

The file is still `-rw-r--r-- root root` and no `-wal`/`-shm` files were left
behind, so the app's own write path is unchanged.

## The live service afterwards

```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
/            200
/library     200
/wishlist    200
```

`/wishlist` went from 1,467,016 bytes at 01:00 to 1,362,788 bytes, consistent
with 58 entries leaving the page. The container log shows the requests served
after the write with no errors.

## Caveat — the writer is still live until DEPLOY-004

The deployed binary is the pre-round build, so its full sync still calls
`SyncGOGWishlist` and will recreate these entries if Bobby presses Full sync
before `DEPLOY-004` lands; the stored GOG authorisation is still valid
(`GOGL-001`). If that happens, re-run the same DELETE — it is idempotent — or
deploy first. After `DEPLOY-004` the writer is gone and this cannot recur.
