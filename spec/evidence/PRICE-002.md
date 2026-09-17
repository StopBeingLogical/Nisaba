# PRICE-002 — Show the GG.deals price where ITAD has none
**Date:** 2026-09-16 · **Commit:** this commit · **Environment:** local

## Acceptance
```bash
$ go build ./...
exit=0

$ go test ./...
ok  	nisaba/handlers	(cached)
ok  	nisaba/sync	0.004s
exit=0
```

## What changed

`templates/wishlist_detail.html`, `wishlist_grid_partial.html`,
`wishlist_cards_partial.html` — only the branch that used to say "no pricing
data" / "Not listed". Templates only; no handler, query, or schema change, and
nothing is written to the database. This is display of a value already stored.

## The entries this is for

Four wishlist entries have a GG.deals price and no ITAD price, so until now the
UI showed them as unpriced:

```
title                        gg_deals_price  url
Echo Weaver                  84.99           https://gg.deals/game/echo-weaver/
Tower and Sword of Succubus  12.99           https://gg.deals/game/tower-and-sword-of-suc…
AION 2                       24.99           https://gg.deals/game/aion-2/
NEO BERLIN 2087              109.16          https://gg.deals/game/neo-berlin-2087/
```

## Rendered

Detail page (`/wishlist/steam-wish-2184080`, `Echo Weaver`), the whole Pricing
card as served:

```html
<div class="sync-card">
    <div class="sync-card-title">Pricing</div>
    <div class="flex items-baseline justify-between">
        <span class="text-xl font-bold price-normal">$84.99</span>
        <span class="text-xs text-gray-500">GG.deals</span>
    </div>
    <div class="text-xs text-gray-600 mt-2">No storefront is currently listing this — GG.deals price shown</div>
    <a href="https://gg.deals/game/echo-weaver/" target="_blank" rel="noopener"
       class="text-xs text-amber-500 hover:text-amber-400 mt-2 inline-block">GG.deals ↗</a>
</div>
```

List view, same database:

```bash
$ curl -s http://127.0.0.1:8098/wishlist | grep -c 'GG.deals</div>'
288
```

The price is labelled `GG.deals`, not a storefront name, because GG.deals does
not say which shop is behind it — the same reason `GG-005` refuses to write a
shop for the comparison column. The line above it states the situation rather
than implying a listing exists.

Entries with no price anywhere still render "Not listed" / "No pricing data":
the new branch is guarded by `.GGDealsPrice.Valid`, so the old placeholder
survives for the 186 entries nothing covers.
