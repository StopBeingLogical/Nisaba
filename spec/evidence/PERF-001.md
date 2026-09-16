# PERF-001 — Copy the live database locally
**Date:** 2026-09-16 · **Commit:** `a40cd38` · **Environment:** atlas, local

## Acceptance
```bash
$ ssh truenas_admin@192.168.3.174 'sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db ".backup /tmp/nisaba-perf.db" && ls -la /tmp/nisaba-perf.db'
-rw-r--r-- 1 truenas_admin truenas_admin 12779520 Sep 16 16:31 /tmp/nisaba-perf.db

$ scp -q truenas_admin@192.168.3.174:/tmp/nisaba-perf.db ~/code/nisaba/nisaba-perf.db
$ test -s nisaba-perf.db
exit=0

$ sqlite3 nisaba-perf.db "SELECT COUNT(*) FROM games;"
4033
```

## Row counts (COUNT)

```
games|4033
wishlist_entries|667
wishlist_price_history|18335
game_stores|4334
```

## Result
A consistent copy of the production database is at `~/code/nisaba/nisaba-perf.db`
(12,779,520 bytes), taken with `sqlite3 ".backup"` rather than a raw file copy so
a WAL-adjacent write cannot be captured half-applied. The source was only read;
the temporary file on Atlas was removed after transfer
(`rm -f /tmp/nisaba-perf.db`).

The copy is gitignored — `git check-ignore -v nisaba-perf.db` →
`.gitignore:5:*.db` — so it cannot be committed by accident.

Drift since the 2026-06-28 session note in `CLAUDE.md`: games 3675 → 4033,
wishlist entries 558 → 667, price history 15,566 → 18,335.

What this does not prove: that the copy still matches the live database at any
later moment. It is a snapshot taken 2026-09-16, and the live service keeps
writing.
