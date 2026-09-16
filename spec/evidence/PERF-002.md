# PERF-002 — Capture the app's own library timing log
**Date:** 2026-09-16 · **Commit:** `dd5ffc9` · **Environment:** atlas

## Acceptance
```bash
$ ssh truenas_admin@192.168.3.174 'grep -c "Library page loaded" /mnt/MemoryAlpha/nisaba/data/nisaba.log'
34

$ ssh ... 'grep "Library page loaded" .../nisaba.log | tail -5'
2026/08/02 19:16:49 Library page loaded in 5.408585955s (3941 games, page 1/20)
2026/08/31 00:04:14 Library page loaded in 5.443662545s (3941 games, page 1/20)
2026/09/08 00:06:19 Library page loaded in 5.608344384s (4033 games, page 1/21)
2026/09/08 00:06:42 Library page loaded in 5.587744259s (4033 games, page 1/21)
2026/09/10 05:16:02 Library page loaded in 5.662826148s (4033 games, page 1/21)
```

## Distribution of the 34 entries

```
18  page 1/20        (3941 games era)
12  page 1/19        (3675 games era, from 2026-06-28)
 3  page 1/21        (4033 games era)
 1  page 2/20        (the only non-page-1 entry)
```

First entry: `2026/06/28 02:55:26 ... 4.937943458s (3675 games, page 1/19)`.
Worst entry: `2026/09/10 05:16:02 ... 5.662826148s`.

## Result
The app's own server-side measurement confirms the assessment's 5.3s and shows it
has not improved: 5.41–5.66s for page 1 across the 4033-game era. The container
has been up 2 months (`docker ps` → `nisaba Up 2 months`), so the log spans the
whole period since the 2026-06-28 performance pass.

The log is nearly all page-1 requests — only one page-2 entry exists — so the
8.84s page-2 figure quoted in the assessment and `PRODUCT.md` comes from its HTTP
probe, not from this log. Page 2 is measured directly in `PERF-003`.

What this does not prove: which component spends the time. The log line covers
the whole handler — eight store calls plus template execution — so it bounds the
problem without locating it.
