# BASE-003 — Correct stale claims in the operational reference
**Date:** 2026-09-16 · **Commit:** `a40cd38` (parent; this file lands with the
CLAUDE.md correction) · **Environment:** local

## Acceptance
```bash
$ grep -q SESSION_SEED.md CLAUDE.md
exit=0

$ ! grep -q 'no sqlc' CLAUDE.md
exit=0

$ bash scripts/spec-validate.sh
task graph validation passed: 19 task(s)
exit=0
```

## Result
`CLAUDE.md:46` now reads `- **DB layer:** Hand-rolled \`db/store.go\` (no ORM, no
code generator)`. The sqlc mention is gone now that `BASE-002` deleted
`sqlc.yaml` and `queries/`; `grep -n 'sqlc\|queries/' CLAUDE.md` returns nothing.
Routing survives: `CLAUDE.md:3` and `README.md:9` both send a new session to
`SESSION_SEED.md`, which is the round gate's routing requirement.

Spot-checks of the remaining claims against the tree (output, not inspection):

| Claim | Evidence |
|---|---|
| SQLite single-writer, `SetMaxOpenConns(1)` is deliberate | `main.go:49: sqlDB.SetMaxOpenConns(1)`; corroborated by `db/CHANGELOG.md:31` |
| Migrations run from `runMigrations()`, additive and idempotent | `main.go:196 func runMigrations(sqlDB *sql.DB) error`; `main.go:193` documents it as applying `schema.sql` then running migrations |
| Deploy loop | `deploy.sh` exists and uses `sudo docker rm -f` + `docker compose up --build -d`, matching the documented `ssh -t` requirement |

Not verified by this task: the auth route list, the image-proxy/CDN claim, and
the IGDB notes — those need live probes or a full read of `handlers/` and
`sync/`, outside this task's 60-minute shape. The "Recent Session Context"
section is explicitly labelled history rather than status, so it was left alone.
