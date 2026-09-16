# Nisaba executable specification

This directory turns the 2026-08-01 round into sub-hour tasks that unrelated
models — local 14B or frontier — can execute safely across arbitrary
environments and context windows, on a service that stays in production
throughout.

## Authority

1. `../SESSION_SEED.md` — role, rights, invariants, rituals.
2. `PRODUCT.md` — locked owner requirements.
3. `contracts/` — stable implementation boundaries.
4. `tasks.yaml` — task graph and status authority.
5. `context-map.yaml` — required reading selected by task prefix.
6. `STATE.md` — human-readable projection of current state.
7. `evidence/` — task completion evidence.

`OPEN.md` holds questions Bobby has not ruled on. Nothing in it is a
requirement and **no task may be derived from it.**

`../CLAUDE.md` stays authoritative for how the machine works — stack, deploy
loop, SSH access, auth, image proxy, IGDB behavior, coding conventions. This
spec is authoritative for what gets built and who decides. `CHANGELOG.md`,
`.changelog/`, and the per-directory changelogs remain the project's history
and keep being updated; they are not a task tracker.

## Task-size rule

Every task must fit within 60 minutes for a prepared compatible environment. A
task that cannot meet that limit must end at a durable discovery, code, or
validation boundary and create follow-up tasks. Never continue merely because
context remains.

Each task must:

- produce one reviewable outcome;
- modify a narrow file set;
- name prerequisites and required context;
- have deterministic acceptance checks;
- avoid combining research, implementation, deployment, and live validation;
- leave the repository resumable if time expires.

## Selecting work

Run `scripts/spec-next.sh <environment[,environment...]>`. Select only `ready`
work whose `depends_on` tasks are `done` and whose environments you actually
have. After selecting an ID, read the matching prefix entry in
`context-map.yaml` — that is the task's named context.

Environment labels:

- `local`: filesystem, Go toolchain, and a local database copy;
- `network`: internet access, or HTTP access to the deployed instance;
- `atlas`: authorized SSH to TrueNAS and the production source/data;
- `human`: Bobby's judgment or confirmation.

Capability hints are scheduling advice, **not model self-identification**:

- `bounded-coder`: established contract, localized change;
- `strong-coder`: cross-package change or recovery logic;
- `frontier`: ambiguous research or a recommendation Bobby will rule on;
- `human`: Bobby's decision or a deployment.

A task labeled above your tier is not yours. Say so and stop.

## Task states

`blocked`, `ready`, `in_progress`, `done`, `superseded`.

**Statuses do not promote themselves** (Bobby's ruling, 2026-09-16). A task
moves `blocked` → `ready` only when an executing session promotes it after every
`depends_on` is `done`; the executor claims it `ready` → `in_progress`. A graph
with nothing `ready` is stalled, not finished — `scripts/spec-next.sh` printing
nothing means nothing was promoted.

Only one task may be `in_progress`. A model claims it by changing status before
implementation. If it cannot commit that claim, it states the claim in its
first update and rechecks the worktree.

## Discovery tasks end at findings

`PERF-003`, `PERF-004`, and `GG-001` produce written evidence and no source
changes — their acceptance checks include `git diff --quiet` on the relevant
paths for exactly that reason. Bobby rules on what they report before the
matching implementation task unblocks. This is a ruling in `PRODUCT.md`, not a
style preference: the 2026-06-28 pass optimized on a hypothesis and `/library`
still served in 5.34s afterward.

## Deployment

Deployment is always its own `atlas` task and always stops for Bobby's
confirmation. It is never bundled into an implementation task.

## Completion evidence

Evidence includes commands and their real output, not claims such as "looks
correct." Live tasks record what was run, against what, and what came back.
Never record secrets or API keys.

## Spec integrity

Missing contracts, duplicate IDs, unknown dependencies, dependency cycles, more
than one `in_progress` task, or a `ready` task with incomplete dependencies are
errors. `scripts/spec-validate.sh` enforces this mechanically. It needs bash
4+ — on macOS that means Homebrew bash, not `/bin/bash`.
