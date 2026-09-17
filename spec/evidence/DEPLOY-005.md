# DEPLOY-005 — deploy the Playnite corrections

**Date:** 2026-09-17 · **Commit:** `0e3286c` · **Environment:** atlas (Bobby confirmed)

## Acceptance

```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
200
```

## Result

Bobby confirmed, then rsync and `deploy.sh` put `GOGL-006` and `GOGL-007` on
Atlas. Image `sha256:76bd4b22c8bb03479cf1015bf700ef28e937f4e80d717972d60b66f33eade271`,
container up, the schedules intact at the end of the build:

```
price sync: next run 2026-09-17 11:00 (in 7h55m0s)
 gog library sync: next run 2026-09-17 11:00 (in 7h55m0s)
NISABA running on :8080
```

**The template is embedded, so the binary is the proof it is served.** Both new
strings are in the running container's binary:

```bash
$ sudo docker exec nisaba grep -ac 'GOG is synced by the server' /app/nisaba
1
$ sudo docker exec nisaba grep -ac 'if ($source -eq "gog") { continue }' /app/nisaba
1
```

`/sync` returns 303, the auth redirect — the page is behind login, so it cannot be
fetched from here to read the script. That is why the check above is against the
binary rather than the rendered page.

## Tree checked before the container was killed

`deploy.sh` removes the container *before* it builds, so a stale file is a dead
site rather than a failed deploy (`DEPLOY-004`). The server tree was compared to
the local one first:

- `handlers/sync.go`: one `StartSync("ownership")` at line 310, one
  `AppendSyncErrors("ownership", …)` at 375. No `playnite` value left.
- `templates/sync.html`: both new strings present.
- `sync/gog_wishlist.go`: absent, so the `GOGL-004` deletion still holds.
- Duplicate-symbol check: `GOGClientID` and `gogGetAccessToken` each defined once,
  in `sync/gog_auth.go`.

**Every Go file on the server that the repository does not have is an AppleDouble
`._*` resource fork** — `<dir>/._<name>.go` for most sources, including
`sync/._gog_wishlist.go`. Go ignores files whose names begin with `.` or `_`, so
these cannot break a build; they are noise from a macOS rsync without
`--exclude='._*'`, nothing more. The `gog_wishlist.go` failure last round was a
real file, which is the distinction that matters: the drift is only dangerous for
*deleted `*.go`* that rsync never removes, and there is none outstanding.

## Live state after the deploy

| | value |
|---|---:|
| `wishlist_entries` | 672 |
| `wishlist_entries` `gog-wish-%` | 0 |
| `game_stores` `store='gog'` | 1037 |
| `games` | 4097 |
| `sync_log` rows `running` | 0 |

Routes `/`, `/library`, `/wishlist` all 200. The newest `sync_log` row is id 86, a
`full` run from `2026-09-17 00:33:11` to `00:47:47` reporting 736 added — it ran
on the pre-`DEPLOY-004` binary, so it *attempted* the GOG wishlist step. It could
not have recreated the 58 entries: that build's `gogGetAccessToken` refused a
token whose recorded expiry had passed (`sync/gog_wishlist.go` at `914966c`),
which is the gate `GOGL-002` removed, and the step logged a skip when it got that
error. So the deletion's result survived the run, and 0 GOG entries is the
expected state rather than a coincidence of ordering.

## What is now observable that was not

A Playnite run after this deploy writes a `sync_log` row of type `ownership`
instead of failing the CHECK and silently discarding the run and its errors. The
first such run is Bobby's to trigger — the endpoint is `POST /api/sync/playnite`,
and the script is on the Sync page.

Still scheduled rather than observed: the first GOG library sync and first
price-only run, both at `2026-09-17 11:00` UTC.

## Observed 2026-09-17 21:1x UTC — the first scheduled runs

Both jobs fired at the first window after the deploy, and both are the only
`sync_log` rows of their type at that time:

| id | type | status | added | updated | started | finished |
|---:|---|---|---:|---:|---|---|
| 87 | `pricing` | done | 0 | 539 | 11:00:00 | 11:00:53 |
| 88 | `ownership` | done | **3** | 0 | 11:00:00 | 11:01:02 |

No `error_message` on either. Row 88 is the prediction in `GOGL-001` and
`GOGL-003` landing exactly: **3 games added**, the three the library view held
and the database did not, in 62 seconds.

Data afterward, against what `GOGL-003` predicted from the scratch database:

| | before | after | `GOGL-003` prediction |
|---|---:|---:|---:|
| `game_stores` `store='gog'` | 1037 | **1040** | 1040 |
| …with a non-empty `store_url` | 0 | **992** | 992 |
| `wishlist_entries` `id LIKE 'gog-wish-%'` | 0 | **0** | 0 |

So GOG's own canonical deal links now populate a store that had never carried
one, exactly the 992 of 1040 GOG products whose payload has a `url`.

### The retired path stayed retired under a Full sync

A **Full sync ran at 20:47:24 on this binary** (id 89, done, 677 added, 543
updated, no error) and the GOG wishlist count is still **0** and the GOG store
rows are still 1040. This replaces the earlier reasoning-by-construction: the
writer is not merely absent from the new binary, its replacement has now been
exercised against the live database without recreating a single `gog-wish-` row.
Wishlist moved 672 → 676 across the round, all Steam.

### `gog.client_secret` is still unset, and the fallback is what ran

`app_config` holds `gog.access_token`, `gog.access_token_expires`,
`gog.refresh_token`, `sync.api_secret` and `sync.price_hour` — **no
`gog.client_secret`**. Row 88 therefore ran on the documented fallback: the
refresh could not be attempted, and GOG accepted the stored access token whose
recorded expiry passed in March. That is the reprieve `GOGL-002` records, and it
is still open — the day GOG stops honouring that token the GOG sync fails until
the secret is set in Settings. The secret is the public Galaxy constant in
Heroic's `gogdl`; nothing on Praxis holds it, so this is a paste, not a machine.

### One action the deploy did not perform for Bobby

`GOGL-006` is in the served template, so the Sync page now hands out a script
that skips GOG — but only someone who **re-copies the script** gets it. Whatever
copy is configured on the Windows side is the old one and will keep posting GOG
until it is replaced. No Playnite run has happened since the deploy, so this is
untested in the live system rather than broken.
