# GOGL-004 — retire the GOG wishlist pass

**Date:** 2026-09-16 · **Commit:** this commit · **Environment:** local

## Acceptance

```bash
$ go build ./...
build ok
$ go vet ./...
vet ok
$ go test ./...
?   	nisaba	[no test files]
?   	nisaba/db	[no test files]
ok  	nisaba/handlers	(cached)
ok  	nisaba/sync	0.005s

$ ! grep -q 'SyncGOGWishlist' handlers/sync.go && echo pass
pass
$ test ! -e sync/gog_wishlist.go && echo pass
pass
```

## Result

The GOG wishlist pass is gone from the full sync (`handlers/sync.go`, the step
and its `wlResult.Added` accumulation), and `sync/gog_wishlist.go` is deleted.
The file's only caller was that step, so it became dead code the moment the step
came out — `grep SyncGOGWishlist` matched nothing else.

What was deliberately kept:

- **The GOG auth path.** `/auth/gog/push` and `/auth/gog/exchange`, the
  `gog.access_token` / `gog.refresh_token` / `gog.client_secret` config keys and
  the settings card all stay: they are how `GOGL-002`/`GOGL-003` authenticate the
  library sync. Only the *wishlist reader* was retired, not the credentials.
- **`db/store.go`.** The methods the deleted file used
  (`UpsertWishlistEntry`, `UpsertWishlistStoreLink`,
  `DeleteStaleWishlistEntries`) are all still used by the Steam wishlist
  (`sync/wishlist.go:83`, `:88`, `:95`), so nothing there changed.
- **The 58 `gog-wish-` entries.** They are still in `wishlist_entries` and still
  show in the wishlist view. Deleting them is `GOGL-005`, which is Bobby's to run
  because it is a deletion against the live database.
- **Manual non-Steam wishlist entries.** Adding one by hand still works;
  only the automated GOG source is retired (`PRODUCT-3.md`, Not in scope).

Observable effects on the next full sync: no "GOG wishlist skipped: token
expired" line, one fewer step in the sync status text, and the reported wishlist
count is Steam's alone rather than Steam plus GOG.

What this does not prove: nothing about the deployed instance — the live service
still runs the old binary and still attempts the wishlist pass. That changes at
`DEPLOY-004`.
