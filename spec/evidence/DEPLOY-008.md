# DEPLOY-008 — deploy the enrichment change and backfill the unmatched games

**Date:** 2026-09-17 · **Environment:** atlas (Bobby confirmed) · **Ruling:**
`spec/PRODUCT-6.md`

## Acceptance

```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
200
```

## Which tree is live — a correction

`/mnt/MemoryAlpha/nisaba/` holds **two** source directories. The running
container's compose project is `source`:

```
com.docker.compose.project          : source
com.docker.compose.project.working_dir: /mnt/MemoryAlpha/nisaba/source
com.docker.compose.project.config_files: /mnt/MemoryAlpha/nisaba/source/docker-compose.yml
```

`/mnt/MemoryAlpha/nisaba/app/` is a **stale copy from March** and is not built by
anything. It still holds a real `sync/gog_wishlist.go` that the GOG round deleted
from the repo, which makes it look like a dangerous rsync leftover; it is not —
it is simply an unbuilt directory. The live tree `source/` was checked and holds
exactly the repo's **36** real `.go` files with no strays (only `._*` AppleDouble
forks, which the Go tool ignores). Verified before deploying, because
`deploy.sh` removes the container *before* it builds.

## The deploy

Synced `/home/bobby/code/nisaba/` → `source/`, then `deploy.sh`.

| | |
|---|---|
| previous image | `sha256:107c7ed2546b…` |
| deployed image | `sha256:310c30aa044f…` |
| new code in the running binary | `involved_companies` ×2, `publisher         = COALESCE` ×1 |

Routes after restart: `/` `200`, `/library` `200`, `/wishlist` `200`, `/sync`
`303`, `/settings` `303`. Both daily schedules intact
(`price sync` and `gog library sync` next at 2026-09-18 11:00).

## The backfill, before and after

Run out-of-band against the live database, calling the same two steps the Full
sync calls — `EnrichLibrary` then `EnrichWishlist` — because `POST /sync/all`
sits behind session authentication with no secret-based path, exactly as
`DEPLOY-006` recorded.

| | before | after |
|---|---:|---:|
| games `needs_review` | 428 | **328** |
| games with a developer | 1301 | **1327** |
| games with a **publisher** | **0** | **92** |
| wishlist `needs_review` | 53 | **41** |
| wishlist with `igdb_id` | 623 | **635** |
| wishlist with artwork | 623 | **635** |
| games total | 3388 | 3388 |

`library 428/428 matched=100 errors=0` · `wishlist 53/53 matched=12 errors=0`.
The 100 library matches are the **same number** the scratch-copy dry run
predicted, and the 12 wishlist matches include all three entity titles from
`DATA-005`.

Rendered pages agree with the database: `/library` reports **3388 games**.

## What this proves, and what it does not

**Proven:** the deployed binary contains the company fetch and the
`developer`/`publisher` writes; the matcher widening is in it; and the enrichment
function fills the columns on live data — every count above is measured after the
run, not predicted.

**Not proven:** that the *Full sync's* step reaches the new writes, because no
Full sync has run on this build. The out-of-band run calls `EnrichLibrary`
directly, so an owner-pressed Full sync is still the only thing that exercises
the deployed wiring end to end. This is the same limit `DEPLOY-006` recorded for
the wishlist step, and it is a limit of the evidence rather than of the code.

## Cleanup

The one-off runner was removed from the container and the host, and its source
was never committed — `git status` is clean apart from the spec files. The
`.backup` copies used for verification were deleted. Backups from `DATA-004` and
`DATA-005` remain on Ergaster at `/tmp/nisaba-backup-2026-09-17b/`.
