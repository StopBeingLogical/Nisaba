# DEPLOY-004 — deploy the GOG round

**Date:** 2026-09-16 · **Commit:** `8e33a46` · **Environment:** atlas

Bobby confirmed the deploy (2026-09-16) — server-side GOG library sync, the
token refresh, and the retired wishlist pass.

## Acceptance

```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
/
200
```

## Steps

```bash
$ go build ./... && go vet ./...
build + vet ok

$ rsync -av --exclude='.git' --exclude='*.db' --exclude='imgcache' --exclude='._*' \
    ~/code/nisaba/ truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/
```

**One extra step, and it was load-bearing.** `rsync` without `--delete` left the
deleted `sync/gog_wishlist.go` on the server next to the new `sync/gog_auth.go`,
where its own `GOGClientID` constant and `gogGetAccessToken` function duplicated
the new file's — a build failure. That matters because `deploy.sh` runs
`docker rm -f nisaba` *before* `docker compose up --build`, so a failed build
leaves the site down. The file was removed explicitly, and the duplicate
declaration was confirmed gone before deploying:

```bash
$ ssh truenas_admin@192.168.3.174 'rm -f /mnt/MemoryAlpha/nisaba/source/sync/gog_wishlist.go'
stale file removed
$ ssh truenas_admin@192.168.3.174 'grep -rn "func gogGetAccessToken" .../source/sync/ | wc -l'
1
```

```bash
$ ssh truenas_admin@192.168.3.174 "cd /mnt/MemoryAlpha/nisaba/source && bash deploy.sh"
#13 [builder 6/6] RUN ... go build -ldflags="-s -w" -o nisaba .  DONE 12.5s
#17 writing image sha256:d0c5bc6d... done
nisaba  | 2026/09/17 02:42:16 price sync: next run 2026-09-17 11:00 (in 8h18m0s)
nisaba  | 2026/09/17 02:42:16 gog library sync: next run 2026-09-17 11:00 (in 8h18m0s)
nisaba  | 2026/09/17 02:42:16 NISABA running on :8080
```

The second line is the new code running: no previous build had a `gog library
sync` scheduler. Both jobs read 11:00 because the container clock is UTC and the
deploy landed at 02:42.

## Verification

```bash
$ for p in / /library /wishlist; do curl -s -o /dev/null -w '%{http_code} %{time_total}\n' http://192.168.3.174:8090$p; done
200 0.011526
200 0.081915
200 0.277785

$ sudo -n sqlite3 <db> "SELECT (COUNT of wishlist_entries), (gog-wish- left), (gog library rows), (running syncs)"
672|0|1037|0
```

The wishlist is still at 672 with no GOG entries after the deploy, and nothing
is mid-sync. `gog_library_rows` is still 1037 because that sync has not run yet
— its window is 11:00 UTC.

## What this does not prove

- **The first scheduled runs.** Both jobs are armed for 2026-09-17 11:00 UTC; the
  GOG library sync has never executed on the deployed instance. Watch for a
  `sync_log` row with `type='ownership'`, and expect `games_added` 3.
- **That the round's sync works without the client secret.** The live config has
  no `gog.client_secret` yet. `gogGetAccessToken` will try to refresh, fail with
  "GOG client secret is not set", log it, and fall back to the stored token —
  which GOG still accepts (`GOGL-001`), so the first run should succeed anyway.
  That fallback is a reprieve, not a design: the moment GOG rejects the stored
  token, GOG stops syncing until the secret is set. Set it in Settings.
- **A full sync's behaviour.** The retired wishlist pass is provably gone from
  the binary (`sync/gog_wishlist.go` no longer exists and no other caller
  existed), but no full sync has been run since the deploy to observe its log.

## Follow-up this exposed

The deployed tree is not in parity with the repository. Because deploys never
use `--delete`, `queries/`, `sqlc.yaml` (both deleted by `BASE-002` on
2026-09-16) and an `enrichment/` directory that the repository does not have are
still sitting in `/mnt/MemoryAlpha/nisaba/source/`. None are referenced — only
`templates/*`, `static/*` and `schema.sql` are embedded (`main.go:24-30`) — so
they are inert, but any *deleted* `.go` file under them would break the build
the same way `gog_wishlist.go` nearly did. Recorded in `OPEN.md`.
