# BASE-002 — Delete the unused sqlc scaffolding
**Date:** 2026-09-16 · **Commit:** `3e0674b` (tree before the deletion; the
deletion lands in the same commit as this file) · **Environment:** local

## Acceptance
```bash
$ git rm sqlc.yaml && git rm -r queries/
rm 'sqlc.yaml'
rm 'queries/<...>.sql'      # 503 lines across the queries/ files

$ go build ./...
exit=0

$ go vet ./...
exit=0

$ go test ./...
?   	nisaba	[no test files]
?   	nisaba/db	[no test files]
ok  	nisaba/handlers	(cached)
ok  	nisaba/sync	(cached)
exit=0

$ test ! -e sqlc.yaml
exit=0
$ test ! -d queries
exit=0
```

## Result
`sqlc.yaml` and `queries/` are gone, the tree still builds, vets, and tests
clean, and both existence checks pass. `db/store.go` is now the only definition
of any query in the repository, which is what `PRODUCT.md` ruled on 2026-08-01.

Inbound references checked before deleting, and what is left after:

| Reference | State |
|---|---|
| `CLAUDE.md:46` | Stack line still read "no ORM, no sqlc" — corrected by `BASE-003` |
| `db/store.go:2-4` | Package header describes the sqlc mirror and promises "once sqlc is available" — now stale; **out of this task's scope**, flagged as a follow-up |
| `SESSION_SEED.md:95,190` | The tripwire row and the "parallel truth" precedent — they describe this deletion, so they stay |
| `schema/tech_stack.md:10,46-54,98-103` | Pre-implementation design doc that still presents sqlc as the DB layer; a plan doc, left in place per the seed's "deleting instead of demoting" rule — **flagged**, not in any task's scope |
| `spec/PRODUCT.md`, `tasks.yaml`, `ACCEPTANCE.md`, `evidence/ASSESSMENT.md` | The ruling, its task, the gate, and the history |

No Go file, Dockerfile, or script reads either path — the two existence checks
and the clean build are the evidence for that.
