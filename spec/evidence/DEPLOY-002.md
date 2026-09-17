# DEPLOY-002 — Deploy the price-source change
**Date:** 2026-09-16 · **Commit:** `94b9fd9` · **Environment:** atlas

Authorized by Bobby in-session ("You can do the restart"). `human` because it
restarts the production container, not because a TTY is needed — `sudo -n docker`
works over non-interactive SSH for `truenas_admin`, which the run below confirms
again (no `-t`, no password prompt).

## Acceptance
```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
200
```

## Deploy

```bash
$ rsync -a --exclude='.git' --exclude='*.db' --exclude='imgcache' ./ \
    truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/

$ ssh truenas_admin@192.168.3.174 "cd /mnt/MemoryAlpha/nisaba/source && bash deploy.sh"
…
#17 writing image sha256:ba0792cf16c1cb54a5015271b099176819b3474589b20237f971603c4db52173 done
 Container nisaba  Started
==> Done. Container logs:
nisaba  | 2026/09/17 00:25:33 NISABA running on :8080
```

Source hashes checked before the rebuild (local vs Atlas): `db/store.go`,
`sync/itad.go`, `sync/ggdeals.go`, `handlers/sync.go`, `main.go`,
`templates/wishlist_detail.html` — all MATCH.

## Order matters: migrate before scorching

The scorched-earth statements reference `gg_deals_price`, which only exists once
the new binary has run `runMigrations()`. The delete was run **after** the
restart, and the columns were confirmed present first:

```bash
$ sqlite3 -readonly /mnt/MemoryAlpha/nisaba/data/nisaba.db \
    "SELECT name FROM pragma_table_info('wishlist_entries') WHERE name LIKE 'gg_deals%';"
gg_deals_price
gg_deals_url
```

Run against the pre-migration copy it fails with `no such column:
gg_deals_price` (`GG-002`), which is why this deploy is sequenced this way.

## Scorched earth (ruled 2026-09-16, "yes, we are starting with clean data")

```bash
$ sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db "
    DELETE FROM wishlist_price_history;
    UPDATE wishlist_entries SET best_current_price=NULL, best_current_store=NULL,
      best_price_url=NULL, historical_low_price=NULL, historical_low_store=NULL,
      last_price_sync=NULL, gg_deals_price=NULL, gg_deals_url=NULL;"
history_rows|0
priced_entries|0
wishlist_entries_intact|667
games_intact|4033
```

The wishlist and library rows are untouched: 667 wishlist entries and 4033 games
before and after. What was deleted is the price data the round exists to replace.

## Key

`itad.api_key` written into the live `app_config` (verified present, prefix
checked only). It lives nowhere else in the deploy: not in the repository, not in
this evidence, not in the changelog, not in the source staged on Atlas.

## Health after

```bash
dashboard      200
wishlist       200
/library       200   (0.078s / 0.079s / 0.078s)
mystery-packs  303 → /?login=1     (protected route, unauthenticated request —
                                    expected; follows to 200)
nonexistent-xyz 404
```
