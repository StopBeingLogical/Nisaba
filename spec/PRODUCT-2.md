# Locked requirements — Nisaba, 2026-09-16 follow-up round

Every line below is a ruling Bobby made on 2026-09-16, after the 2026-08-01 round
closed (`evidence/REL-001.md`). Nothing here is inferred. The 2026-08-01 round's
requirements stay in `PRODUCT.md`; this file governs only the work it names.

Track: **feature change** on a live service. Nisaba stays working throughout.

## Scope of this round

Two changes, and only these two:

1. **Price freshness.** Prices are only refreshed when someone presses Full sync,
   which takes ~15 minutes because it also re-imports ownership, wishlists and
   enrichment. Ruled: **the app refreshes prices once a day, on its own.** The
   scheduled run is *price-only* — the ITAD pass and the GG.deals comparison
   pass, nothing else — because the 2026-09-16 measurement puts those at 37
   seconds of that 15-minute sync (Steam wishlist 7.5 min, enrichment 3.5,
   resellers 2, ownership 1). Executed by `PRICE-001`.
   **Cadence rationale, recorded so it is not re-litigated:** Steam rotates
   deals weekly and runs seasonal sales, so a daily check catches every sale
   start within a day; more frequent runs add rows to the price history without
   adding signal, and the wishlist sparkline reads the last 90 history rows per
   entry — one run a day makes that a three-month chart, twice a day halves it.
   **If a denser cadence is ever wanted, the dedupe comes first:** append a
   history row only when the price or store changed.
2. **Coverage fallback.** 5 wishlist entries have a GG.deals price but no ITAD
   price, and the UI shows nothing for them, because the cheaper-on-GG.deals
   callout requires an ITAD price to compare against. Ruled: **where ITAD has no
   price and GG.deals does, show the GG.deals price.** Executed by `PRICE-002`.

## Constraints that carry over

Everything in `PRODUCT.md`'s constraints still holds — additive idempotent
migrations only, `SetMaxOpenConns(1)` never removed, the deployed instance stays
working, no comments added to unchanged code, secrets never in HTML, changelog
one-liners, deployment always its own `atlas` task that stops for confirmation.

## Not in scope

- **The GOG wishlist token is expired** (`re-paste auth.json in Settings`,
  logged on every full sync since at least run 86 on 2026-09-17). Bobby deferred
  it on 2026-09-16; the GOG wishlist is skipped silently until he re-pastes it.
  **Superseded by `PRODUCT-3.md` (2026-09-16):** the GOG wishlist is retired
  rather than re-authorised, its 58 rows are deleted, and the stored token now
  serves the server-side GOG library sync, which refreshes it itself. The expiry
  gate described here no longer exists (`PRODUCT-3.md`, `GOGL-002`).
- **The 133 unpriced Steam entries are not a defect.** Sampled 60 deep:
  25 not released yet, 31 with no release date in ITAD at all, 4 released with no
  deal in US, DE or GB, and an independent endpoint (`POST /games/prices/v3`)
  returns nothing for all 133. Nothing to fix; they price themselves on release.
- Reseller scraping for non-Steam entries, enrichment, and the sync UI.
- Any Settings UI for the schedule hour. The value lives in `app_config` and is
  set by hand.
