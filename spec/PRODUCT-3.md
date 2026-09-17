# Locked requirements — Nisaba, 2026-09-16 GOG round

Every line below is a ruling Bobby made on 2026-09-16, after the follow-up round
closed (`PRODUCT-2.md`, `evidence/DEPLOY-003.md`). Nothing here is inferred.
The earlier rounds' requirements stay in `PRODUCT.md` and `PRODUCT-2.md`; this
file governs only the work it names.

Track: **feature change** on a live service. Nisaba stays working throughout.

## Scope of this round

Two changes, and only these two:

1. **The GOG owned library syncs itself.** Ruled: *"design a better, more hands
   off way to sync the gog library list."* **Route (ruled 2026-09-16): server
   side, from GOG's API.** The app refreshes the stored GOG token itself and
   pulls the owned-product list; no Windows machine and no hand-pasted script in
   the loop. Executed by `GOGL-001` (proof), `GOGL-002` (token refresh),
   `GOGL-003` (the sync and its schedule).
2. **The GOG wishlist is retired, and its rows are deleted.** Ruled: *"I don't
   use the wishlist there anymore. The only wishlist would be coming from
   steam."* The GOG wishlist pass leaves the full sync, its settings card goes,
   and the 58 `gog-wish-` entries already in `wishlist_entries` are deleted
   rather than left stale. Executed by `GOGL-004` (code and UI) and `GOGL-005`
   (the deletion, which is Bobby's to run).

## Basis for the route (measured 2026-09-16, not assumed)

- **Only one reachable path puts GOG ownership in the database today, and it is
  manual.** `POST /api/sync/playnite` (`main.go:116`) accepts the library, but
  the invocation is a script Bobby pastes into Playnite's *Interactive SDK
  PowerShell* window by hand (`templates/sync.html:71-76`).
- **The Heroic path is dead code.** `UploadHeroicFiles` (`handlers/sync.go:520`)
  has no route in `main.go`, so `sync/heroic.go` is unreachable; the
  `heroic.library_path` setting (`handlers/settings.go:59`,
  `templates/settings.html:197`) is displayed and never used for an import.
- **The refresh token is already stored and never used.**
  `handlers/gog_auth.go:71` saves `gog.refresh_token`; nothing in the codebase
  reads it — `gogGetAccessToken` (`sync/gog_wishlist.go:72-84`) only checks the
  access token and its expiry, then errors.
- **The GOG wishlist token is expired**, so the wishlist pass returns early and
  `SyncAll` logs "skipped" (`handlers/sync.go:123-129`). Nothing in production
  depends on the stored token working, which is why the proof in `GOGL-001` can
  be run without risking a working feature.
- **The server-side flow is the one Heroic's `gogdl` already uses** (the client
  running on Praxis): refresh via `auth.gog.com/token` with
  `grant_type=refresh_token`, then `GET embed.gog.com/user/data/games` with a
  bearer token for the owned list, and `api.gog.com/products/{id}` for
  metadata — the last already used by `fetchGOGProductTitle`
  (`sync/gog_wishlist.go:134`). The client ID is already a constant
  (`sync/gog_wishlist.go:16`).
- **Checked and rejected: a public profile needs no credential.** GOG serves a
  public profile games page unauthenticated (`embed.gog.com/u/<username>/games`
  returns 200), but it is a JS shell with no JSON sibling (`/games.json` 404,
  `/games/ajax` 404, `?format=json` the same shell), and the documented owned
  list (`embed.gog.com/user/data/games`) sits in the authenticated account
  section beside `/userData.json`; the only unauthenticated user endpoint is
  `users/info/<id>`, which carries profile and wishlist-sharing status and no
  owned list. Using it would mean an undocumented endpoint that may be
  privacy-gated, and would make the library publicly visible. Bobby asked
  directly whether this existed (2026-09-16); it does not, in usable form.

## Ordering rule

`GOGL-001` runs before any sync code is written. It is a proof, not a
reconnaissance: if the stored refresh token no longer refreshes, the route is
still right but the round needs a stated first-run credential step, and the
tasks that follow change shape. This is the same rule the 2026-08-01 round
ratified — no fix is written against a hypothesis.

## Constraints that carry over

Everything in `PRODUCT.md`'s and `PRODUCT-2.md`'s constraints still holds —
additive idempotent migrations only, `SetMaxOpenConns(1)` never removed, the
deployed instance stays working, no comments added to unchanged code, secrets
never in HTML, changelog one-liners, deployment always its own `atlas` task that
stops for confirmation.

Two constraints are specific to this round:

- **The pasted credential is the only one.** The stored refresh token from
  `auth.json` remains the single source of GOG auth. Nothing in this round adds
  a login flow or asks Bobby for anything but a re-push of that file.
- **The GOG wishlist must not be replaced by a hidden dependency.** Retiring it
  removes a code path; it does not move wishlist work into the library sync.

## Not in scope

- **Reworking the Playnite path.** Steam, Epic, Amazon, Xbox, PlayStation and
  Battle.net ownership keep arriving exactly as they do now, script and all.
  Only GOG leaves that dependency.
  **Confirmed 2026-09-16 ("no need for playnite sync, at least for GOG. It can
  still be useful for all the other libraries"):** GOG is not Playnite's job any
  more, and Playnite remains the path for the other stores — so an observable
  Playnite run still matters, and the `sync_log` constraint defect in `OPEN.md`
  is a live problem rather than a GOG one. GOG itself is unaffected: its sync
  records to `sync_log` as type `ownership`, which the schema allows.
- **Deleting the dead Heroic code.** It is recorded as dead above; removing it
  is not part of this round.
- **Manual wishlist entries.** Users can still add a non-Steam wishlist entry by
  hand; only the automated GOG wishlist source is retired.
- **The `sync_log` constraint defect** (Playnite and mystery-pack runs cannot be
  logged, so they never appear in Recent Activity). It is recorded in `OPEN.md`
  and needs a ruling, because repairing it means rebuilding a table rather than
  an additive migration.
- **Any Settings UI for the schedule hour**, price cadence, or enrichment.
