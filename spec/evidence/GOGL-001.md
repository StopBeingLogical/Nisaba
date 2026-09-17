# GOGL-001 — prove the stored GOG refresh token still refreshes

**Date:** 2026-09-16 · **Commit:** `914966c` · **Environment:** local + network

Run from `~/code/nisaba` on Ergaster. The credential comes from the local
`nisaba-perf.db`, the read-only copy of the live database taken by `PERF-001`
on 2026-09-16 19:31 — the live database was not touched. No token value, and no
client secret value, appears anywhere in this file. The refresh call uses the
public GOG Galaxy client secret published in Heroic's `gogdl` client
(`gogdl/auth.py`, `CLIENT_ID`/`CLIENT_SECRET`); the client ID is already a
constant in this repository at `sync/gog_wishlist.go:16`.

Bobby authorised the credential call (2026-09-16), understanding that a rotated
token might make a re-push from Praxis necessary.

## Acceptance

```bash
$ sqlite3 nisaba-perf.db "SELECT key, LENGTH(value) FROM app_config WHERE key LIKE 'gog.%';"
gog.access_token|192
gog.access_token_expires|10
gog.refresh_token|64

$ sqlite3 nisaba-perf.db "SELECT value FROM app_config WHERE key='gog.access_token_expires';" && date +%s
1773294825
1789610539

$ curl -s "https://auth.gog.com/token?client_id=<galaxy client id>&client_secret=<galaxy client secret>&grant_type=refresh_token&refresh_token=<stored refresh token>" | jq -r '...'
Refresh: ok
expires_in: 3600
rotated refresh_token present: true
scope:
error: none
error_description: none

$ curl -s -o /dev/null -w 'stored token -> HTTP %{http_code}\n' -H "Authorization: Bearer <stored access token>" https://embed.gog.com/user/data/games
stored token -> HTTP 200
$ curl -s -o /dev/null -w 'no token    -> HTTP %{http_code}\n' https://embed.gog.com/user/data/games
no token    -> HTTP 302

$ curl -s -H "Authorization: Bearer <stored access token>" https://embed.gog.com/user/data/games | jq '{owned_count: (.owned|length)}'
{"owned_count": 1397}

$ curl -s "https://auth.gog.com/token?...&refresh_token=<stored refresh token>" | jq -r '...'
old refresh token: still valid

$ curl -s https://api.gog.com/products/1207660773 | jq '{id, title, slug}'
{"id": 1207660773, "title": "7th Legion", "slug": "7th_legion"}

$ curl -s -H "Authorization: Bearer <stored access token>" "https://embed.gog.com/account/getFilteredProducts?mediaType=1&page=1" | jq '{totalProducts, page, totalPages, perPage: (.products|length)}'
{"totalProducts": 1040, "page": 1, "totalPages": 11, "perPage": 100}

$ curl -s -H "Authorization: Bearer <stored access token>" "https://embed.gog.com/account/getFilteredProducts?mediaType=2&page=1" | jq '{totalProducts, perPage: (.products|length)}'
{"totalProducts": 0, "perPage": 0}

$ curl -s -o /dev/null -w 'HTTP %{http_code}\n' "https://embed.gog.com/account/getFilteredProducts?mediaType=1&page=1"
HTTP 302
```

Reconciliation against the local copy of the live database:

```bash
$ sqlite3 nisaba-perf.db "SELECT COUNT(*) FROM game_stores WHERE store='gog';"
1037

$ # 11 pages of getFilteredProducts, ids collected
library-view ids: 1040
=== in library view, NOT in db (3) ===
1443424837 1663581818 1872608764
=== in db, NOT in library view (0) ===
=== of the 361 owned-not-in-db, how many are in the library view? ===
3

$ # the three
1443424837	Jazz Jackrabbit 2 Plus	true	false	false	true	/en/game/jazz_jackrabbit_2_plus
1663581818	State of Mind	true	false	false	true	/en/game/state_of_mind
1872608764	Pyramids and Aliens: Escape Room	true	false	false	true	/en/game/pyramids_and_aliens_escape_room
```

## Result

**The refresh flow works, and it does not cost anything to run.**

- `auth.gog.com/token` with `grant_type=refresh_token` returns a usable access
  token and a **rotated refresh token**, expiry 3600s. Refreshing is available
  to the app server-side with no new credential beyond one first push.
- **The stored access token still works**, despite its recorded expiry
  (`1773294825` = 2026-03-09) being ~6 months past. The recorded
  `expires_in`-based expiry is therefore not a reliable gate; `gogGetAccessToken`
  (`sync/gog_wishlist.go:72-84`) rejects a token that GOG would still accept.
- **Rotation does not invalidate the old refresh token** — the stored one
  refreshed again successfully afterwards. Nothing in the live config was
  broken by this proof, and no re-push from Praxis is needed.
- **The owned list is real:** `embed.gog.com/user/data/games` returns 1397 ids.
- **`getFilteredProducts` is the right source, not per-id lookups.** With
  `mediaType=1` it returns 1040 products over **11 requests** (100/page) —
  id, title, relative url/slug, image, `worksOn` Windows/Mac/Linux, category,
  `isGame`/`isMovie`/`isHidden`, `dlcCount` and release date. Resolving the 1397
  owned ids individually would be 1397 requests; this is the same bulk-over-
  per-entry finding `GG-002` recorded for ITAD.
- **The library is not actually missing 361 games.** The database's 1037 `gog`
  rows reconcile exactly against the 1040-product library view: 3 missing, 0
  stale. Of the 361 owned-but-absent ids only those same 3 are library
  products; the remaining 358 are entitlements the library view does not show
  (packs, Prime Gaming / Amazon Luna rewards, and delisted products — e.g.
  `Sid Meier's Civilization IV: The Complete Edition - Amazon Prime`,
  `Quake II - Amazon Prime`, `Brigador: Deluxe DLC Upgrade`). A sync that
  trusted `owned` blindly would add ~358 non-library rows.
- `mediaType=2` (movies) is empty, so `mediaType=1` is the whole account; no
  extra filtering is needed.
- **Both GOG endpoints answer 302, not 401, without a token** — an
  implementation must treat a redirect as "authenticate/refresh", not only 401.

What this does **not** prove: that a refresh survives the token being revoked
(password change, `POST /account/logout_all_sessions`), or that GOG will not
invalidate a rotated refresh token in future; and it did not exercise a single
write, so no statement is made here about how the sync upserts.

## Note on the client secret

The refresh call requires the Galaxy client secret. It is public (it is a
constant in Heroic's open-source `gogdl`), but committing it here would violate
`ACCEPTANCE.md`'s "no API key or secret appears in the repository". It
therefore belongs in `app_config` as `gog.client_secret`, set once by hand,
with a set/unset boolean in the settings view per the existing convention. This
is carried into `GOGL-002`.

The task's acceptance commands, run after this file was written:

```bash
$ grep -q 'Refresh' spec/evidence/GOGL-001.md && echo pass
pass
$ grep -q 'owned' spec/evidence/GOGL-001.md && echo pass
pass
$ git diff --quiet -- sync handlers db main.go templates && echo pass
exit 0
```
