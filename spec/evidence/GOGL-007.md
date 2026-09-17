# GOGL-007 — log Playnite runs as an allowed sync_log type

**Date:** 2026-09-16 · **Commit:** this commit · **Environment:** local

## Acceptance

```bash
$ go build ./...
build ok
$ go vet ./...
vet ok
$ go test ./...
?       nisaba  [no test files]
?       nisaba/db       [no test files]
ok      nisaba/handlers (cached)
ok      nisaba/sync     (cached)

$ ! grep -q 'StartSync("playnite")' handlers/sync.go && echo pass
pass
$ ! grep -q 'AppendSyncErrors("playnite"' handlers/sync.go && echo pass
pass
```

## Result

Two call sites in `handlers/sync.go` changed type, and nothing else:

| line | before | after |
|---|---|---|
| 310 | `StartSync("playnite")` | `StartSync("ownership")` |
| 375 | `AppendSyncErrors("playnite", runID, errors)` | `AppendSyncErrors("ownership", runID, errors)` |

`sync_log.type` allows only `('full', 'ownership', 'install', 'pricing',
'wishlist', 'rehydrate')` (`schema.sql:265`). The old `playnite` value failed that
CHECK, so the INSERT in `StartSync` returned an error, `logID` stayed **0**, and
both the `FinishSync` and `AppendSyncErrors` calls sit inside the same
`if logID > 0` guard — so **no Playnite run and no Playnite error was ever
recorded**, and nothing appeared in Recent Activity. `sync_errors` carries no
CHECK of its own; it was only ever starved because the guard that reached it was.

`ownership` is the truthful type: this endpoint imports ownership. The schema's
own `CHECK` list — and the pre-existing `ownership` rows already in the live
database — make it the intended label for that work, so no migration and no table
rebuild was needed. This executes the ruling recorded in `OPEN.md` (2026-09-16:
"no rebuild; log Playnite runs as type `ownership`").

Two consequences worth naming rather than discovering later:

- **The type no longer identifies the writer.** The scheduled GOG library sync
  also logs `ownership` (`sync/schedule.go:176`), so a GOG run and a Playnite run
  are indistinguishable by `type` alone. That is inherent to the ruling — the
  allowed list has no better slot for either — and they are told apart by their
  content: the GOG run reports `games_added` 3 with GOG links, the Playnite run
  reports whatever Bobby posted.
- **`mystery_packs` still cannot be logged.** `handlers/sync.go:457` keeps
  `StartSync("mystery_packs")`, which fails the same CHECK, and it is deliberately
  untouched — the ruling scoped the fix to Playnite and `OPEN.md` records that the
  mystery-pack call site keeps its original type. That run remains invisible in
  Recent Activity, and the same `logID > 0` guard still discards its errors.

What this does not prove: nothing about the deployed instance. The live binary is
still the pre-round build, so a Playnite sync today still logs nothing. The first
observable Playnite run is the one after `DEPLOY-005`.
