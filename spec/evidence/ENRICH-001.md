# ENRICH-001 — fetch and persist developer and publisher, widen spelling equivalence

**Date:** 2026-09-17 · **Environment:** local (verification against a `.backup` of
the live database with real IGDB) · **Ruling:** `spec/PRODUCT-6.md`

## Acceptance

```bash
$ go build ./... && go vet ./... && go test ./...
?   nisaba          [no test files]
?   nisaba/db       [no test files]
ok  nisaba/handlers 0.003s
ok  nisaba/sync     0.006s
$ grep -c involved_companies sync/igdb.go
2
$ grep -q 'PUBLISHER' spec/evidence/ENRICH-001.md && echo ok
ok
```

## Change (a) — the company fetch and the two columns

`IGDBGame` decoded only `id,name,cover,genres,summary,first_release_date,url,
platforms`, so enrichment could never learn a company. Three additions:

- `IGDBGame` gains `InvolvedCompanies` (`company.name`, `developer`, `publisher`),
  and `igdbFields` requests
  `involved_companies.company.name,involved_companies.developer,involved_companies.publisher`.
- `DeveloperName()` / `PublisherName()` return the credited companies
  comma-joined, or `""` when IGDB credits none.
- `EnrichGameParams` gains `Developer` and `Publisher`, and `EnrichGame` writes
  them through **`COALESCE`** so a value already supplied by a sync or by hand is
  never overwritten.

`EnrichWishlistEntry` is deliberately unchanged — `wishlist_entries` has no such
columns.

**Proven against live IGDB**, not assumed, from the search path itself:

```
Hollow Knight    match="Hollow Knight"   dev="Team Cherry"    pub="Team Cherry"
Baldur's Gate 3  match="Baldur's Gate III" dev="Larian Studios" pub="Larian Studios"
Cyberpunk 2077   match="Cyberpunk 2077"  dev="CD Projekt RED" pub="CD Projekt"
```

## Change (b) — three spelling equivalences in title comparison

The matcher required the IGDB name to **normalise to exactly the same string** as
the stored title. Three additions, each of which can only equate different
*spellings* of one name:

1. **`searchTitle` strips `™` `®` `©` from the search string.** These are absent
   from IGDB names and make the search return nothing:
   `RollerCoaster Tycoon® Deluxe` returned **0 results**.
2. **`&` expands to `and`** in `normalizeTitle`, so `Orcs & Humans` equals
   `Orcs and Humans`.
3. **Canonical roman numerals normalise to arabic**, so `VI` equals `6`. A token
   is converted only if its canonical roman spelling round-trips
   (`romanToArabic` vs `toRoman`), which leaves words like `mix` and `did` alone.

`normalizeTitle` is shared with RAWG and the reseller scrapers; the widening
applies to all of them consistently. **No containment, prefix or
edition-stripping matching was added** — those can select the wrong entry and
remain unruled.

## Measured

An enrichment backfill was run against a `.backup` of the live database with real
IGDB, before and after the matcher change:

| | matches of 428 | developer filled | **PUBLISHER** filled |
|---|---:|---:|---:|
| before | **3** | 1301 | **0** |
| after | **100** | 1327 | **92** |
| second run | **0** (already matched) | 1327 | 92 |

`0 errors` on every run. The developer column moves less than the match count
because 74 of the 100 games already carried a developer from a store sync, which
`COALESCE` correctly preserves.

A direct probe of 14 unmatched titles showed **0/14 exact normalised matches
before** and 3/14 after, with 8/14 present under a variant name — so the blocker
was the comparison, not the library.

## What this does not fix

**328 games remain `needs_review`.** Roughly half of them sit in IGDB under a
decorated or abbreviated name — `Halcyon 6` → `Halcyon 6: Starbase Commander`,
`GTA IV` → `Grand Theft Auto IV`, `LostWinds 2` → `LostWinds`,
`BioShock Infinite Complete Edition` → `BioShock Infinite: The Complete
Edition`. Catching those needs containment or suffix-stripping matching, which
*can* pair a game with the wrong entry, so it is left as a question with its
measurements attached rather than assumed.
