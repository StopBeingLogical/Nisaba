# Locked requirements — Nisaba, 2026-08-01 round

Every line below is a ruling Bobby made, either in the 2026-08-01 interview or
in `CLAUDE.md` (cited). Nothing here is inferred. Unratified items are in
`OPEN.md` and no task may be derived from them.

Track: **feature change**. Nisaba is in production on Atlas and stays working
throughout.

## Scope of this round

Two changes, and only these two:

1. **Library page performance.** `/library` currently serves in 5.3s and
   `/library?page=2` in 8.8s (`evidence/ASSESSMENT.md`, Finding 1). **Target:
   under 1 second for page 1 and for any later page**, measured against the
   deployed instance the way the assessment measured it
   (`curl -s -o /dev/null -w '%{time_total}'`). Ratified 2026-08-01.
   **Approach (ruled 2026-09-16): whatever serves responsiveness.** That is
   `PERF-004`'s nested-`EXISTS` rewrite of the `multi_store_owned` arm in
   `ListGames` — measured 2.53s → 0.011s for page 1, row output identical over
   every row. Executed by `PERF-005`.
2. **GG.deals store-name granularity.** GG.deals returns only category-level
   names (`gg.deals/retail`, `gg.deals/keyshop`), never individual stores, so
   `best_current_store` and `wishlist_price_history.store` are always
   categories (`CLAUDE.md:169`). Real store names must reach the UI. **Which
   provider path gets used is deliberately not ruled yet** — a discovery task
   reports first, then Bobby rules. Ratified 2026-08-01.
   **Pre-cutover history (ruled 2026-09-16): scorched earth.** When real store
   names arrive, existing rows holding `gg.deals/retail` or `gg.deals/keyshop`
   are deleted — not left mixed, not backfilled — so new price data starts
   clean. `GG-002` executes it.
   **Provider (ruled 2026-09-16): ITAD.** GG.deals represents more stores, but
   without real shop names that breadth is irrelevant, so ITAD is adopted on the
   strength of `shop.name` (`GG-001`). A different provider may displace it only
   if it is measured to supply real storefront names. Executed by `GG-002`.

Mystery-packs follow-through is **out of scope** for this round.

## Ordering rule

No fix is written against a hypothesis. For both halves of this round, a
discovery task isolates and records the facts first, and the implementation
task depends on that finding. This is a ruling, not a preference: the
2026-06-28 performance pass optimized on a hypothesis and the page is still
5.3s.

## Deployment

Deployment is **always its own task**, labeled `environment: [atlas]`, and it
stops for Bobby's explicit confirmation. It is never bundled into an
implementation task. Ratified 2026-08-01.

## Removals

`sqlc.yaml` and `queries/` are deleted. `db/store.go` is and remains the single
source of truth for queries. Nisaba is not adopting a code generator.

## Acceptance standard

- Every task's acceptance is a runnable command plus, where behavior is
  user-visible, a named URL that must return 200 and show a stated thing.
- `go build ./...` and `go vet ./...` pass on every task.
- Tasks are not required to add tests. Existing tests must not break.

## Execution model

- Tasks must be dispatchable to **either** a local 14B-class model **or** a
  frontier model, whichever is available. Every task therefore carries an
  honest `capability` label and enough specification that a bounded model can
  execute it without making architectural choices.
- Task granularity follows the stricter of the two audiences: one outcome, a
  narrow file set, no ambiguity left for the executor to resolve.

## Constraints that must hold (existing rulings)

- `sqlDB.SetMaxOpenConns(1)` is intentional and is never removed
  (`CLAUDE.md:88-89`).
- Schema changes are additive and idempotent, in `runMigrations()` in
  `main.go`. Never DROP or rename a column; never modify existing rows in a
  migration (`CLAUDE.md:91-92`).
- The deployed instance stays working. Library filters, sorting, search, and
  pagination keep their current behavior unless a ruling changes them.
- No comments added to unchanged code; no docstrings retrofitted; no error
  handling for impossible cases (`CLAUDE.md:131-136`).
- Secrets are never rendered as `value=` in HTML (`CLAUDE.md:135`).
- Changelog one-liners go to `.changelog/UNRELEASED.md` as work happens
  (`CLAUDE.md:140-151`).

## Not in scope

- Mystery-packs follow-through (min-value pack automation, batch delete, bulk
  price update, list pagination).
- Adopting sqlc or any ORM.
- Any change to the auth model, the deployment topology, or the Cloudflare
  tunnel.
- Reworking the changelog system.
