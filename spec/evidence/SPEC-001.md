# SPEC-001 — Add task-graph validator
**Date:** 2026-09-16 · **Commit:** `f0c9a08` · **Environment:** local

## Acceptance
```bash
$ bash scripts/spec-validate.sh
task graph validation passed: 19 task(s)
exit=0
```

## Result
`scripts/spec-validate.sh` exists, is tracked in `f0c9a08`, and its acceptance
command runs clean against `spec/tasks.yaml` at that commit. It checks ID shape,
duplicate IDs, missing or invalid statuses, unknown dependencies, `ready` tasks
with incomplete dependencies, the single-`in_progress` rule, and dependency
cycles.

The script was written and committed as part of the spec authoring in
`f0c9a08`, not by this session. This entry records the acceptance re-run on that
commit and closes the status row.

What this does not prove: that the checks actually fire on a malformed graph. No
negative case was run.
