# DEPLOY-003 — Deploy the follow-up round
**Date:** 2026-09-16 · **Commit:** `7d3591c` · **Environment:** atlas

## Acceptance
```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
200
```

## Deploy

Source hashes checked before the rebuild — `sync/schedule.go`, `db/store.go`,
`main.go`, `templates/wishlist_detail.html` — all MATCH, then:

```
 Container nisaba  Started
==> Done. Container logs:
nisaba  | 2026/09/17 00:56:02 price sync: next run 2026-09-17 11:00 (in 10h4m0s)
nisaba  | 2026/09/17 00:56:02 NISABA running on :8080
```

The container's own log naming its next window is the scheduler reporting from
production: it read the hour, found today's window still ahead, and went to
sleep for it. Container clock is UTC, so 11:00 UTC is 07:00 US Eastern.

## Configuration

```bash
$ sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db \
    "INSERT INTO app_config(key,value) VALUES('sync.price_hour','11')
     ON CONFLICT(key) DO UPDATE SET value=excluded.value;"
sync.price_hour|11
```

Written explicitly rather than left to the code default, so the value is
inspectable and changeable on the box. Changing it takes effect on the next loop
iteration — i.e. after the currently-sleeping window — not immediately.

## Health

```
  /                200
  /library         200
  /wishlist        200
  /nonexistent-xyz 404
```

## PRICE-002 live

Five entries have a GG.deals price and no ITAD price in production (one more than
the local copy showed — the full sync added `Nivalis Nights`):

```
steam-wish-2184080|Echo Weaver|84.99
steam-wish-1327080|Tower and Sword of Succubus|12.99
steam-wish-3393110|AION 2|24.99
steam-wish-2326200|NEO BERLIN 2087|109.16
steam-wish-1488490|Nivalis Nights|84.99
```

`http://192.168.3.174:8090/wishlist/steam-wish-2184080` serves the fallback:

```html
<div class="sync-card-title">Pricing</div>
<div class="flex items-baseline justify-between">
    <span class="text-xl font-bold price-normal">$84.99</span>
    <span class="text-xs text-gray-500">GG.deals</span>
</div>
<div class="text-xs text-gray-600 mt-2">No storefront is currently listing this — GG.deals price shown</div>
<a href="https://gg.deals/game/echo-weaver/" target="_blank" rel="noopener"
   class="text-xs text-amber-500 hover:text-amber-400 mt-2 inline-block">GG.deals ↗</a>
```

## What is not yet observed

**The scheduled run has not fired in production.** At deploy time the pricing row
count was unchanged (15 rows, newest 2026-06-19) and the newest `sync_log` row was
still run 86, the button-driven full sync — the scheduler correctly declined to
catch up, because today's window had not arrived and prices were fresh from that
sync minutes earlier. The first real run is **2026-09-17 11:00 UTC**.

The mechanism itself is proven where it can be proven without waiting:
`evidence/PRICE-001.md` shows the same binary performing the catch-up run and
then declining to repeat it after a restart. What is unverified is only that this
container, at that hour, does the same thing. The place to look is
`sync_log` — a row with `type='pricing'`, `status='done'`, and ~500 in
`games_updated`.
