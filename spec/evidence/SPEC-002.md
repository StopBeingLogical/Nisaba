# SPEC-002 — Add compatible next-task selector
**Date:** 2026-09-16 · **Commit:** `f0c9a08` · **Environment:** local

## Acceptance
```bash
$ bash scripts/spec-next.sh local
SPEC-001
exit=0

$ bash scripts/spec-next.sh local,atlas
SPEC-001
exit=0
```

## Result
`scripts/spec-next.sh` exists, is tracked in `f0c9a08`, and both acceptance
commands exit 0. Both printed `SPEC-001` because it was the only task with
status `ready` at that commit; the two runs differ only in the environment
labels supplied, and `SPEC-001` requires `[local]`.

The script was written and committed as part of the spec authoring in
`f0c9a08`, not by this session. This entry records the acceptance re-run on that
commit and closes the status row.

What this does not prove: that environment filtering excludes anything. No task
was `ready` with an environment outside the supplied set, so the exclusion path
was not exercised — the expectation that it withholds such a task comes from
reading `scripts/spec-next.sh` (`emit_current` returns without printing when a
required label is missing), not from an observed run.
