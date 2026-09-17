# DATA-005 — repair the wishlist entity titles and the incoherent price pairs

**Date:** 2026-09-17 · **Environment:** atlas (Bobby confirmed) · **Ruling:**
`spec/PRODUCT-6.md`

## Acceptance

```bash
$ grep -q before spec/evidence/DATA-005.md && grep -q after spec/evidence/DATA-005.md && echo ok
ok
```

## (a) The 3 titles carrying HTML entities

An entity in the stored title guarantees an IGDB miss — the matcher sees the
literal `amp` as a word, so no normalisation can reach the real name. Decoded,
and `sort_title` takes the same substitution (`makeSortTitle` only strips a
leading article, and none of the three has one).

| id | before | after |
|---|---|---|
| `steam-wish-2400510` | `Dungeons &amp; Degenerate Gamblers` | `Dungeons & Degenerate Gamblers` |
| `steam-wish-3828500` | `Deck &amp; Conn` | `Deck & Conn` |
| `steam-wish-1836560` | `Aether &amp; Iron` | `Aether & Iron` |

| | before | after |
|---|---:|---:|
| wishlist titles containing an HTML entity | **3** | **0** |

Re-enrichment ran in the same pass, and **all three then matched**, which is the
proof that the entity was the only obstacle:

| id | after | `igdb_id` |
|---|---|---:|
| `steam-wish-1836560` | `matched` | 335238 |
| `steam-wish-2400510` | `matched` | 248673 |
| `steam-wish-3828500` | `matched` | 361654 |

## (b) The 12 incoherent price pairs

`best_current_price` below the entry's own `historical_low_price`. A current
price below the recorded low **is** a new low, so the recorded low takes the
current price and its store. Example rows:

| id | title | before low | store | after low | store |
|---|---|---|---:|---|---:|---|
| `steam-wish-1524550` | Madshot | 4.95 | GameBillet | **4.81** | GameBillet |
| `steam-wish-2717880` | The Rogue Prince of Persia | 8.99 | Steam | **7.20** | Ubisoft Store |
| `steam-wish-2514330` | The Rabbit Haul | 14.99 | Fanatical | **12.74** | Fanatical |
| `steam-wish-3372060` | Hell Maiden | 9.99 | Steam | **8.49** | Fanatical |
| `steam-wish-2842040` | Star Wars Outlaws | 15.75 | GreenManGaming | **14.00** | Ubisoft Store |

| | before | after |
|---|---:|---:|
| entries with `best_current_price` < `historical_low_price` | **12** | **0** |

Only `historical_low_price` and `historical_low_store` were written.

## Backups

`wishlist_entries` was dumped to `/tmp/nisaba-backup-2026-09-17b/
wishlist_entries.sql` before the repairs, along with the verbatim before-state of
all 15 affected rows.
