# Round 2026-09-17 (second pass) — enrichment gaps and the matcher

Locked owner requirements. Supersedes `PRODUCT-5.md` where they disagree; that
file stays as the record of what was ruled then.

## The report

Bobby, 2026-09-17: *"Let's do a full audit and clean up of the data."* — a second
audit pass after the `PRODUCT-5` cleanup landed.

The pass is `evidence/AUDIT-002.md`. It re-measures every category from
`AUDIT-001` against the **post-cleanup** live database and confirms the earlier
work held: `integrity_check` is `ok`, `foreign_key_check` returns nothing, there
are zero orphans in any table, zero negative or out-of-range values, zero
malformed or sentinel dates, zero unparseable artwork JSON, zero non-`http`
store URLs, zero duplicate wishlist identities, and `sync_errors` is empty. The
games text is fully clean — 0 mojibake, 0 untrimmed, 0 doubled spaces, 0 HTML
entities. The **124 Steam Family Sharing games are intact**, still carrying
`owned = 1`: they were never part of the `owned = 0` reference set that the
debris step removed, so `DATA-003`'s claim holds.

## The finding that changed a ruling

Ruling 2 below asks for the 428 unmatched library games to be enriched. The
backfill was written, dry-run against a copy of the live database with real
IGDB, and **it moved 3 of 428.** That is not a data gap: `bestMatch`
(`sync/igdb.go`) required the IGDB name to *normalise to exactly the same
string* as the stored title, and store titles are decorated — `™`/`®`, edition
suffixes, year-in-parens, and arabic numerals where IGDB uses roman.

Probed against live IGDB, 0 of 14 sampled titles matched exactly, while 8 of the
14 were present in the results under a variant name:
`BioShock Infinite Complete Edition` → IGDB `BioShock Infinite: The Complete
Edition`; `Might and Magic 6` → `Might and Magic VI`; `Warcraft: Orcs and
Humans` → `Warcraft: Orcs & Humans`. A further 4 returned **no results at all**,
which pointed at the search string itself: `RollerCoaster Tycoon® Deluxe` carries
a `®` that IGDB's name does not.

So the matcher, not the library, was the bottleneck. `ENRICH-001` extends it
with three **spelling equivalences** — none of which can match a *different*
game:

1. The IGDB **search string** is stripped of `™`, `®`, `©` before it is sent.
2. `&` expands to `and` during normalisation, so `Orcs & Humans` equals
   `Orcs and Humans`.
3. Canonical roman-numeral tokens normalise to arabic, so `VI` equals `6`.
   A token is only converted if its canonical roman spelling round-trips, which
   leaves words like `mix` and `did` alone.

Measured on a copy of the live database with real IGDB: **3/428 → 100/428
matched, 0 errors**, `publisher` filled on 92 and `developer` on +26 (the rest
already carried a developer from a store sync, which `COALESCE` preserves). The
identical backfill run twice changes nothing the second time.

**Still unruled:** the remaining 328, of which roughly half sit in IGDB under a
decorated or abbreviated name (`Halcyon 6` → `Halcyon 6: Starbase Commander`,
`GTA IV` → `Grand Theft Auto IV`). Catching those needs containment or
edition-stripping matching, which *can* pick the wrong entry, so it is left as a
question rather than assumed.

## Ruled, 2026-09-17 (second pass)

### 1. The 2 duplicate groups blocked only by reference links are merged

The dedupe's conflict guard (`DATA-002`) counted **all** store links, including
the `owned = 0` reference links that no query reads. Two of the 7 groups it
excluded — **Wasteland 2: Director's Cut** and **Heretic: Shadow of the Serpent
Riders** — were blocked only by a reference link that lives on a *different*
game (`steam/240760`, `steam/3286930`). The `DATA-003` debris step then deleted
every `owned = 0` link, so those two groups are mergeable and stayed duplicated
for no remaining reason. **Merge them**, keeping each survivor's identifier.

The other 5 groups each hold two genuine **owned** identifiers for one store
(`steam/12100` and `steam/12230` are both "Grand Theft Auto III"), and
`game_stores` allows one link per store per game, so merging them means
discarding a real ownership record. They stay.

### 2. The unmatched library games are enriched, and developer/publisher with them

`EnrichGame` writes `igdb_id`, `artwork`, `description` and `release_date` — and
nothing else. Two consequences, both measured: **`publisher` is empty on all
3390 games** (`InsertGame` omits the column and no path ever sets it), and
**2089 games have no `developer`**. The IGDB response type did not even ask for
companies, so enrichment could never fill either.

**Request the companies and persist both.** `ENRICH-001` adds
`involved_companies.company.name`, `involved_companies.developer` and
`involved_companies.publisher` to the fetch, and `EnrichGame` writes `developer`
and `publisher` through `COALESCE` so a value already supplied by a sync or by
hand is never overwritten.

### 3. The multiple-stores indicator keeps counting family sharing

It flags **243** games, up from the 102 measured pre-merge, because merging
unified split-store ownership — the intended effect of `DATA-001`. 36 of the 243
count `steam family sharing` as the second store. Excluding it would give 216.
**Keep counting it** — no code change.

### 4. Three smaller repairs go in the same pass

- **The 3 wishlist titles carrying HTML entities** (`Deck &amp; Conn`,
  `Dungeons &amp; Degenerate Gamblers`, `Aether &amp; Iron`) are decoded, because
  the entity guarantees an IGDB miss, and then re-enriched.
- **The 12 wishlist entries whose `best_current_price` sits below their own
  `historical_low_price`** are made coherent: a price below the recorded low *is*
  a new low, so the recorded low takes the current price and store.
- **The company backfill** above runs against the live database.

## What stays untouched

The 124 family-sharing games; the 5 genuinely unmergeable duplicate groups; the
`steam_deck_verified` and `proton_rating` fetchers that lost their callers in
June; and the duplicate groups' visible two-tile appearance, which is the honest
display of two real store identifiers.
