# DEPLOY-010 — candidate evidence and inline search shipped, queue re-seeded

**Status:** done. Task: `MATCH-002`.

## Deploy

Three steps, as always, against the live tree `/mnt/MemoryAlpha/nisaba/source`
(the compose project is `source`; `app/` is a stale copy nothing builds).

- **Before syncing:** compared the two `.go` trees — **37 files on the server, 37
  locally, none only-on-server**, so the rsync without `--delete` had no stale
  file to leave behind.
- **Dry run** listed exactly the intended files plus three evidence documents
  from the previous round that had never been synced.
- **Synced and deployed.** New image **`85455eb667b3`**; previous
  **`afc4167e49b2`** remains on the host for rollback. Container `nisaba` came up
  clean and logged `NISABA running on :8080`.

Startup migration added the four evidence columns to the live database, confirmed
by `PRAGMA table_info(match_review)`:

    11|summary|TEXT|1|''|0
    12|genres|TEXT|1|''|0
    13|platforms|TEXT|1|''|0
    14|igdb_url|TEXT|1|''|0

## Routes

    /            -> 200  0.008s
    /library     -> 200  0.071s
    /wishlist    -> 200  0.345s
    /match-review -> 303  (session redirect, same as /review)

The running binary contains the new code (`strings` finds 12 occurrences of the
new template/column markers), and `go:embed templates/*` includes both new
partials.

**A false alarm worth recording.** The first route check reported `404` for
`/library`, `/match-review` and `/review`, which looked like a regression. The
responses carried `server: uvicorn` — port **8080** on the host is a different
service. Nisaba is published on **8090** (`8080:8080` → `8090:8080` in
`docker-compose.yml`); every check above is against the correct port.

## Re-seed

The candidate search was re-run so the evidence columns would be populated. It
cannot be triggered over HTTP without a browser session, so it was run by calling
the same function the **Find candidates** button calls, against the mounted live
database, with the binary built `CGO_ENABLED=0 GOOS=linux` for the alpine runtime:

    searched 242 / 242  —  99 with a candidate, 0 errors

Backup taken first at `/tmp/nisaba-backup-2026-09-17c/nisaba-pre-evidence.db`.
The runner was removed from the container, the host, and the working tree, and
was never committed.

**The re-seed moved nothing.** Snapshots of all 242 undecided pairings taken
before and after are identical, and `decision` counts are unchanged at
50 yes / 36 no / 242 undecided — the owner's answers and every candidate under
them are exactly as they were.

## Still unproven, and it is the same gap as `MATCH-001`

`/match-review` and its two new endpoints sit behind session auth with no
non-interactive trigger, so the code paths were exercised by calling the handlers
directly, not through chi's router plus the auth middleware. The search, the
manual pick, the row swap and the stored result are all measured; **pressing the
button in a browser is not.** Everything else about this deploy is verified.
