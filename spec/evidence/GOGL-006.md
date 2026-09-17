# GOGL-006 — stop the Playnite script sending GOG games

**Date:** 2026-09-16 · **Commit:** this commit · **Environment:** local

## Acceptance

```bash
$ go build ./...
build ok
$ go test ./...
?       nisaba  [no test files]
?       nisaba/db       [no test files]
ok      nisaba/handlers (cached)
ok      nisaba/sync     (cached)

$ grep -q 'GOG is synced by the server' templates/sync.html && echo pass
pass
```

## Result

The embedded Playnite script now drops GOG before it builds a payload, one line
after the store is derived:

```powershell
$source = if ($g.Source) { $g.Source.Name.ToLower() } else { "playnite" }
# GOG is synced by the server; sending it from here would fight that sync.
if ($source -eq "gog") { continue }
```

The skip is placed on `$source`, not on the game id or title, because that is the
value the payload's `source` field carries — so "GOG games" means exactly what the
endpoint would have received as `source: "gog"`, and nothing else is filtered.
Every other store still goes out unchanged: Steam, Epic, Amazon, Xbox,
PlayStation, Battle.net and anything else Playnite reports keep arriving through
this path, which is the point of the ruling ("it can still be useful for all the
other libraries").

Why the skip matters even though both writers key on the GOG product id: with the
server sync live, the script would have posted the same games with Playnite's own
store URL over the canonical `www.gog.com/en/game/<slug>` link the server writes.
No duplicates — but two writers of one link, and no way to tell which last won.

Copy corrected in the same file, because both statements it carried are now false:

- **Full Sync card** — said "Sync ownership from Steam, GOG, Epic, and Amazon via
  Playnite. Refresh Steam and GOG wishlists…". Now names Playnite's stores and
  says GOG comes from the server, and the wishlist it refreshes is Steam's.
- **Playnite card** — said the sync "replaces Heroic and GOG syncs". Now lists the
  stores it does carry and states that GOG is synced by the server once a day.

What this does not prove: the skip is not live. The script reaches a browser only
through the embedded template in the binary, so until `DEPLOY-005` the deployed
instance still hands out the old script and a Playnite run still posts GOG rows.
No server-side code reads `source: "gog"` differently as a result of this change —
the receiver is unchanged.
