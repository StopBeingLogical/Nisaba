# Codebase assessment — Nisaba

**Date:** 2026-08-01 · **Commit:** `2b8701c` (clean, `main == origin/main`) · **Assessed by:** Opus 5

Every claim below carries evidence. Nothing was fixed during this assessment.

## Build and test baseline

```
$ go version
go version go1.25.0 linux/amd64

$ go build ./...      → exit 0
$ go vet ./...        → exit 0
$ go test ./...
?   	nisaba	[no test files]
?   	nisaba/db	[no test files]
ok  	nisaba/handlers	0.006s
ok  	nisaba/sync	0.006s
```

Green baseline, no toolchain or network blockers. Roughly 15,500 lines: `handlers/` 4,253 · `sync/` 3,678 · `templates/` 3,205 · `db/` 2,961 · `main.go` 667 · `static/` 243.

## What works end to end

| Capability | Verified how | Evidence |
|---|---|---|
| Service is live on Atlas | HTTP probe | `http://192.168.3.174:8090/` → 200 in 0.006s |
| Deployed build matches repo HEAD | Route probe | `/mystery-packs` → 303 (auth redirect, route exists); `/nonexistent-xyz` → 404 |
| Wishlist path | HTTP probe | `/wishlist` → 200 in 0.235s |
| Mystery packs feature | Shipped in `a2cef85`; only tested code in the repo | `handlers/mystery_packs_api_test.go`, `sync/mystery_packs_scrape_test.go` |

## Finding 1 — the library page takes 5.3 seconds, and pagination makes it worse

Reproducible, not a cold cache:

```
/library run1 -> 5.337s      /library?page=2 -> 8.842s
/library run2 -> 5.337s      /wishlist       -> 0.235s
/library run3 -> 5.375s      / (dashboard)   -> 0.006s
```

Three consecutive runs land within 40ms of each other. The dashboard and wishlist are fast, so this is specific to the library grid path, not the host, the container, or SQLite generally.

This crosses the project owner's own stated responsiveness threshold, and it survived a dedicated performance pass — the 2026-06-28 session added 200-per-page pagination, five indexes, and template caching (`CLAUDE.md:161-162`) and the page is still 5.3s.

**Cause not isolated. Two candidates, both hypotheses:**

- **Per-row subqueries** in `db/store.go:99-113` — the `SELECT` carries three `GROUP_CONCAT` correlated subqueries (genres, tags, owned stores) plus a `multi_store_owned` `EXISTS` that self-joins `games` × `game_stores` on `igdb_id`, all evaluated per returned row.
- **`OFFSET` scanning** — page 2 costing 65% more than page 1 is the signature of `ORDER BY sort_title … LIMIT/OFFSET` walking rows it then discards.

Isolating this is a task, not an assessment conclusion. `handlers/library.go:117-119` already logs elapsed time above 100ms, so the server log likely holds the answer.

## Finding 2 — `queries/` is a parallel, unused source of truth

`db/store.go:2-4` states it plainly:

> It mirrors the intent of the sqlc queries/ files but is implemented with database/sql directly so the binary compiles without running sqlc. Once sqlc is available, this package can be replaced with the generated output.

So `sqlc.yaml` and 503 lines of `queries/*.sql` describe queries that no code path executes, sitting beside the 2,961-line hand-rolled `db/store.go` that actually runs. `CLAUDE.md:37` correctly says "no sqlc" for the live path but the scaffolding stayed. Two definitions of the same queries, one of them silently drifting, is a trap for any model asked to change a query.

Owner's call: adopt sqlc, or delete the scaffolding.

## Finding 3 — test coverage is one feature wide

Two test files in ~15,500 lines, both belonging to the newest feature (mystery packs). `main.go` (667 lines, including 29 schema migration statements) and `db/store.go` (2,961 lines) have none. Given "additive migrations only" is a hard constraint (`CLAUDE.md:91-92`), the migration path is both the most dangerous code in the repo and the least covered.

Not a defect on its own — this is a personal tool, organically tested. It matters here only because it sets what "acceptance" can mean for tasks that touch `db/` or migrations: right now, it can only mean "builds and the page still loads."

## Architecture as built

Single Go binary, Chi v5 router, 52 routes registered in `main.go`. SQLite with `SetMaxOpenConns(1)` (deliberate, single-writer — `CLAUDE.md:88-89`). `html/template` + `//go:embed`, HTMX + Tailwind CDN, no frontend build step. Packages: `db` (hand-rolled store), `handlers` (per-domain files), `sync` (16 integration files — IGDB, GG.deals, ITAD, Steam, Heroic, GOG, RAWG, ProtonDB, Playnite, mystery-pack scrapers). Deployed as a single Docker container on TrueNAS, port 8090→8080, dataset-backed DB, Cloudflare tunnel with session-cookie auth.

Real seams already exist — `db` ↔ `handlers` ↔ `sync` are clean boundaries, and the sync integrations are independently shaped. Contracts should describe these, not invent new ones.

## Where state lives

`.changelog/UNRELEASED.md` as a working file, consolidated into per-directory `CHANGELOG.md` files at commit time (`CLAUDE.md:140-151`). `CLAUDE.md` itself carries the session seed, constraints, deploy loop, and a "Recent Session Context" section. There is no task tracker. Any spec must map onto the changelog system, not replace it.

## Doc/reality gap

Smaller than expected — `CLAUDE.md` is accurate on stack, constraints, deploy loop, and paths. Two items:

| Claim | Where | Reality |
|---|---|---|
| Mystery packs "ready for implementation / ready for testing and deployment" | agent memory `nisaba_mystery_packs.md` | Built, committed in `a2cef85`, and live — `/mystery-packs` returns 303 on the deployed instance. The memory is stale. |
| Repo presents as an sqlc project | `sqlc.yaml`, `queries/` | Dead scaffolding; see Finding 2 |

`CLAUDE.md:169` records a still-open known issue: GG.deals returns category-level store names only (`gg.deals/retail`, `gg.deals/keyshop`), never individual stores, with three named fix options including switching to ITAD.

## Conventions

No comments on unchanged code; no docstrings retrofitted; no error handling for impossible cases; secrets never rendered as `value=` in HTML; template partials registered in `handlers.New()` (`CLAUDE.md:131-136`). Additive migrations only, idempotent, in `runMigrations()`. Commit messages are short imperative summaries.

## Findings that should become tasks

1. Isolate the `/library` 5.3s cause — read the existing timing log, then bisect between the per-row subqueries and the `OFFSET` scan. Discovery task, ends at a written finding, no fix.
2. Resolve `queries/` — adopt or delete. Needs an owner ruling first.
3. GG.deals store-name granularity — ITAD swap or an alternative, per `CLAUDE.md:169`.
4. Refresh the stale mystery-packs memory/status note.
