# Round 2026-09-17 (seventh pass) — the search box was asking IGDB the wrong question

Locked owner requirements. Supersedes `PRODUCT-10.md` where they disagree; that
file stays as the record of what was ruled then.

## The request

Bobby: *"I just did a manual search for the first game "Against the Storm" with the
little search box, and it pulled the way wrong game. I went to the actual IGDB site
and did the search and found the exact match by exact title match."*

## The finding: the site and the API are not the same search

IGDB's website found the game. Our box, asking IGDB's API for the same words, did
not. Probed against the live API, the game is there and our query was simply not
reaching it:

| query against `api.igdb.com/v4/games` | result |
|---|---|
| `search "Against the Storm"; limit 5;` | `The Demon Crystal 3: Dark Storm`, `Metal Storm`, three *Life is Strange: Before the Storm* episodes |
| the same, `limit 30` | the correct entry, **`147519 Against the Storm`, at position 14** |
| `search "…"; where platforms = (6); limit 40;` | the correct entry at **position 10** |
| `where name ~ *"Against the Storm"*` | `Against the Storm` — first |
| `where name = "Against the Storm"` | `Against the Storm` |
| `where name = "Against the storm"` | **empty** |

Three separate defects fell out of that, and all three are in the *query layer*
rather than the scorer:

1. **`limit 5` truncated the answer away.** The correct entry sits at position 10 of
   the platform-filtered ranking and 14 unfiltered. The scorer never saw it.
2. **IGDB's `search` is a conjunction over every term, matched against the summary
   as well as the name.** So `search "Fallout 2: A Post Nuclear Role Playing Game"`
   returns an **empty set** — no entry matches all eight terms — and
   `search "Against the Storm"` returns *Life is Strange* episodes because their
   blurb contains the word "against". A store title carrying a subtitle therefore
   either matches nothing or matches noise.
3. **The platform filter reshaped the ranking.** `where platforms = (6)` did not
   merely drop console rows: it returned a different result set, and pushed the
   correct entry further down. It is not a safe narrowing.

Also measured, and worth recording because they are counter-intuitive:
`where name ~ *"…"*` is **case-insensitive** while `where name = "…"` is
**case-sensitive** and returns an empty set for the same name in different case; and
a wildcard lookup returns its matches in **no useful order**, so the exact entry can
fall outside a small limit even when it is one of the matches at all (the base entry
for `The Falconeer` sat outside the first 10 of 18 and inside the first 50).

## Ruled, 2026-09-17 (seventh pass)

### 1. Build the search from two sources, not one

A **name-anchored** wildcard lookup is merged **ahead** of IGDB's ranked search for
every title. The anchored form cannot return summary noise and cannot be outranked,
which is the property the whole fix rests on; the ranked form is kept because it
reaches variants a name lookup cannot (`Battle Isle 2` → `Battle Isle 2200`). Both
are asked for 25 rows instead of 5.

### 2. Cut the title at both ends, and try the narrower head first

A subtitle is retried on its **head** when the full form reaches nothing confident.
Two heads are generated, because store titles decorate front and back and the two
ends need opposite treatment:

- cut at the **last** separator — drops a trailing decoration while keeping the
  game's own name: `Warhammer 40,000: Dawn of War II - Anniversary Edition` →
  `Warhammer 40,000: Dawn of War II`
- cut at the **first** separator — drops a descriptive tail:
  `Fallout 2: A Post Nuclear Role Playing Game` → `Fallout 2`

The order is not cosmetic. The first version of this fix cut at the first separator
only, which turned the Dawn of War II title into `Warhammer 40,000` and matched
**`Dawn of War` — the wrong game** at the confident prefix tier. The measured sample
caught it before it shipped, which is the only reason it is documented here as a
rule rather than discovered later.

A head must be at least two words. `Batman: Arkham Asylum` is not shortened to
`Batman`, which would anchor against every Batman entry IGDB has.

### 3. Do not filter by platform

The filter is removed from the search rather than widened. `bestMatch` already
prefers a PC entry among exact names, so the intent it was serving ("avoid matching
a mobile/console variant") is kept without narrowing the result set.

### 4. Pacing moves into the client

A search is no longer one request — it is two, or up to six when the fallback forms
run. Every caller paced itself at 250ms per *game*, so on its own this change would
have put two to four times that rate on the wire and started collecting 429s.
`IGDBClient.query` now enforces the floor itself, since it is the single funnel every
IGDB call passes through. **This regression was introduced and caught in the same
pass**, before deploy.

### 5. Stop the loop on a *confident* score, not any score

The fallback forms are only skipped once a form has scored at the `contains` tier
(0.7) or above. A weak hit is exactly what a title with a real match underneath it
scores — the old behaviour stored `Metal Storm` at 0.05 for `Against the Storm` —
so stopping on "any score above zero" would keep the wrong candidate and skip the
query that finds the right one.

### 6. The search box shows its hits best-first

`RankSearchResults` orders the handler's output by the same scorer the queue uses,
so the entry that *is* the title being searched is the first thing on screen rather
than whatever IGDB happened to rank first. The list is capped at 15 for the box.

### 7. A concatenated word is an exact match

Stores concatenate words IGDB spaces out, and no token test can see through it: the
tokens are disjoint, so `Dragonview` scored **zero** against `Dragon View` and the
correct entry was discarded. Spacing is now ignored for the exact tier.

### 8. Re-seed the queue

The widened search reaches further, so the queue is re-seeded to take the better
candidates. Rows already ruled on are skipped by design and were verified untouched.

## Measured

On the 110 undecided rows that had no candidate, plus three controls (113 rows):

| | exact match | any usable match |
|---|---:|---:|
| before (`search`, limit 5) | 1 | 7 |
| after | **10** | **48** |

No row regressed: the widened ranked query is a superset of the old one, and that is
asserted rather than assumed. Live after re-seeding: **168 → 211** undecided rows
with a candidate, 0 errors, the 50 decided rows byte-identical, `integrity_check ok`.

## The cost, recorded rather than hidden

A wider search finds more entries, and the scorer's **prefix tier is generous**:
when the stored title extends an IGDB name, the extension is not required to look
like a subtitle. So the re-seed replaced some weak-but-correct candidates with
confident-but-wrong ones — `Call of Duty: WaW` → `Call of Duty`,
`STAR WARS™: Rebel Assault 1` → `Star Wars`, `M.A.X.` → `Max` (exact, via punctuation
stripping). Measured over the whole queue: 47 replacements scored **higher**, 0
scored lower, 23 kept the same score on a different entry, and **0** confident
matches were lost. The wrong ones are the cost of the wider net, and they are the
owner's to reject — a rejection is remembered, so each is paid once.

This is a **scoring** limitation, not a search one, and it is left for its own round
rather than tweaked on the way past. Recorded in `spec/OPEN.md`.
