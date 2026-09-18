# Round 2026-09-17 (fourth pass) — making the match queue workable

Locked owner requirements. Supersedes `PRODUCT-7.md` where they disagree; that
file stays as the record of what was ruled then.

## The request

Bobby, 2026-09-17, after working part of the queue: *"I went through several
pages. A lot of these I would need to inspect one by one, it may take awhile."*

That is the whole brief, and it is a report of friction rather than a feature
request. Measured against live before ruling on anything:

| confidence | total | undecided | yes | no |
|---|---:|---:|---:|---:|
| **no candidate** | **143** | **143** | — | — |
| prefix | 74 | 48 | 14 | 12 |
| weak | 64 | 28 | 15 | 21 |
| tokens | 35 | 16 | 18 | 1 |
| contains | 12 | 7 | 3 | 2 |

**143 of the 242 undecided rows had nothing on the page to compare against.**
The right column said `No candidate found`, so ruling on them meant leaving the
page to look elsewhere — and none of them had been started. That, not the queue
length, is what made it slow. The other signal: `tokens` was running 18 yes to 1
no (nearly all safe), while `weak` was a genuine coin flip at 15/21, which is
where the real inspection goes.

## Ruled, 2026-09-17 (fourth pass)

Bobby chose **two** of four offered options: **inline IGDB search** and **more
candidate evidence**. Explicitly **not** chosen, and therefore not built:
keyboard shortcuts and a bulk-accept for the high-confidence tiers.

### 1. Evidence is stored with the candidate, not fetched per page view

`match_review` gains `summary`, `genres`, `platforms` and `igdb_url`, filled when
the candidate is found. A page shows 25 rows, so fetching evidence at render time
would mean 25 IGDB calls per page view — stored once, the page stays a single
database read. Display is bounded so one verbose entry cannot stretch a row into
a wall of text: the summary is cut at **280 characters** on a word boundary, and
platform lists show **4** names plus a `+N more` count.

### 2. Picking a match *is* the verdict, and saves immediately

The search box sits on every row — open by default where there is nothing to
compare, collapsed behind *"Wrong game? Search IGDB"* where a candidate exists.
Picking a result stores it and marks the row **yes** straight away, labelled
`you picked this`. Two reasons: the click is already an unambiguous instruction,
and a row resolved this way should not depend on the page being saved before it
counts. The page's **Save** continues to govern the yes/no radios, so the two
paths do not fight: Save only ever writes `decision` and `decided_at`, never a
candidate.

This deliberately overrides `MATCH-001`'s rule that a re-run must never disturb a
decided row — a manual pick uses an unconditional write, because the owner
clicking is exactly the case where the new value outranks the old.

### 3. Re-seeding must not move an existing pairing

Re-running the search is expected and safe: rows already ruled on are skipped, so
their verdicts and candidates are untouched. What is newly required is that a
re-seed **cannot change a candidate under an undecided row either** — the owner
may have looked at a row and not yet answered. This was verified rather than
assumed: all **242** undecided pairings were byte-identical before and after the
live re-seed, because adding `platforms.name` to the fetched fields does not
reach the scoring function.

### 4. Already-decided rows keep no evidence, deliberately

The re-seed skips them, so the 86 rows already ruled on show no summary or IGDB
link on the Yes/No tabs. Left as-is: evidence exists to help make a decision, and
those decisions are made. If they are revisited during the true-up, the ruling
above (a manual pick overrides unconditionally) still lets a row be corrected.

### 5. What is still not wired

Unchanged from `PRODUCT-7` and still awaiting a ruling: **nothing is applied to
`games`**. A yes stores a verdict; it does not set `igdb_id`, fetch artwork, or
link the game. The true-up pass needs Bobby's ruling first — whether a yes links
only or also re-enriches, and whether a no leaves the game unmatched or hides it.
