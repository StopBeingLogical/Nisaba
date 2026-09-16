# BASE-001 — Record the green baseline
**Date:** 2026-09-16 · **Commit:** `487b793` · **Environment:** local (Ergaster)

## Acceptance
```bash
$ go version
go version go1.25.0 linux/amd64

$ go build ./...
exit=0

$ go vet ./...
exit=0

$ go test ./...
?   	nisaba	[no test files]
?   	nisaba/db	[no test files]
ok  	nisaba/handlers	0.005s
ok  	nisaba/sync	0.005s
exit=0
```

## Result
Green baseline at `487b793`. Its Go tree is identical to the round's spec
commit `f0c9a08` — everything committed between them is under `spec/` plus
`SESSION_SEED.md` — so this is the round's starting code measured on the current
worktree. Matches `spec/evidence/ASSESSMENT.md`, which recorded the same three
exits at `2b8701c` on go1.25.0, so nothing has rotted between the assessment and
now. No source change was made by this task.

What this does not prove: runtime behaviour. The two passing test files are the
mystery-packs ones; `main.go` and `db/store.go` still have none, so for tasks
touching `db/` or migrations "acceptance" can still only mean "builds and the
page loads" (`ASSESSMENT.md`, Finding 3).
