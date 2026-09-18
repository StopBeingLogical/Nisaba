# DEPLOY-013 — the search fix deployed, and the queue re-seeded

**Status:** done. Rulings: `spec/PRODUCT-11.md`, fix in `spec/evidence/MATCH-008.md`.

## Deploy

Server tree checked before rsyncing, per `CLAUDE.md` — `deploy.sh` removes the
container before it builds, so a build failure there takes the site down. The
diff touched `sync/igdb.go`, `handlers/match_review.go`, one new test file and
documentation; nothing deleted, `SearchGamePC` gone from both trees.

    $ ssh truenas_admin@192.168.3.174 'sudo docker inspect --format "{{.Image}}" nisaba'   # before
    sha256:f20da8a750917fd7c000e9acea899f148005319e79058f2d7c82443406be7d4d
    $ ssh truenas_admin@192.168.3.174 "cd /mnt/MemoryAlpha/nisaba/source && bash deploy.sh"
    Container nisaba  Started
    nisaba  | NISABA running on :8080
    $ ssh truenas_admin@192.168.3.174 'sudo docker inspect --format "{{.Image}}" nisaba'   # after
    sha256:0bc200e39e6305fc95b063ee8e61d9eea565dec64e3ba3a978b08f6963dc2b93

**Rollback target: `f20da8a75091`. Deployed: `0bc200e39e63`.**

    /                200
    /library         200
    /match-review    303   (session auth)

## Why a re-seed was ruled in

A wider search reaches further, so the queue is worth re-seeding. This was dry-run
on a copy **before** touching live, because a re-seed re-searches rows the owner has
already looked at. Over all 278 undecided rows:

```
  unchanged                  165
  gained a candidate          43
  candidate replaced          70
    scored higher (better)    47
    scored lower  (worse)      0
    same score, other entry   23
  candidate lost               0
  search errors                0
  a confident match lost       0
```

Zero losses and zero errors; 113 rows strictly better or newly workable. Rows already
ruled on are excluded by `ListGamesNeedingCandidate` (`m.decision IS NULL`) and
`UpsertMatchCandidate` refuses to overwrite a decided row.

## Backup

    /mnt/MemoryAlpha/nisaba/backups/2026-09-17-1927/nisaba.db   (12.7 MB)

## The re-seed

Runner built `CGO_ENABLED=0` — the first build was glibc-linked and died inside the
alpine container with `exec /tmp/nisaba-seed: no such file or directory`, which reads
like a missing file rather than a missing libc. Copied in, run, removed from the
container, the host and the tree; never committed.

```
  2026/09/18 02:28:14   ...  22/278 searched,  19 with a candidate, 0 errors
  2026/09/18 02:30:37   ... 149/278 searched, 112 with a candidate, 0 errors
  2026/09/18 02:33:05 re-seed complete
```

**4m51s for 278 rows**, 0 errors.

## Before and after, live

    integrity_check                                       ok
    games                                               3388
    games with an IGDB id                               3110   (unchanged — the seed writes only match_review)
    undecided                                            278
    undecided WITH a candidate                    168 -> 211
    undecided WITHOUT a candidate                 110 ->  67
    shortlist entries                             501 -> 753
    rows with alternatives                        117 -> 167
    rejections                                            36
    rejected entries still offered                         0
    decided                                               50
    undecided candidates at the confident tier            166   of 211
    undecided candidates that are an exact match           41

## The proof the owner's work was not disturbed

Every decided row's `(game_id, igdb_id, decision)` was dumped before and after and
diffed:

    before: 50 rows, after: 50 rows
    DECIDED_ROWS_IDENTICAL

## Corrected mid-flight

The first post-deploy route check reported `404` on `/library` and `/match-review`.
The responses carried `server: uvicorn` — port 8080 on Atlas is a different service.
Nisaba is on **8090**, and every number in this document is from there.
