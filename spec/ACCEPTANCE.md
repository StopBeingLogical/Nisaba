# Round completion gate — 2026-08-01

This round is complete only when `REL-001` records all of the following against
one commit. Every line is objectively checkable by whoever runs it.

- `scripts/spec-validate.sh` passes.
- `go build ./...`, `go vet ./...`, and `go test ./...` pass.
- `/library` serves in under 1 second on the deployed instance, three
  consecutive runs.
- `/library?page=2` serves in under 1 second on the deployed instance, three
  consecutive runs.
- Every filter, sort, and rendered field named in `contracts/library-view.md`
  still works after the fix.
- `wishlist_entries.best_current_store` holds real storefront names for
  entries priced after the cutover, not `gg.deals/retail` or
  `gg.deals/keyshop`.
- The wishlist UI renders those names, and an unmapped identifier degrades to
  the raw value rather than blank.
- `sqlc.yaml` and `queries/` are gone and nothing references them.
- `CLAUDE.md` and `README.md` route new sessions to `SESSION_SEED.md`.
- No API key or secret appears in the repository, in task evidence, or in a
  changelog entry.
- The deployed instance is healthy and no user data was deleted.

Anything Bobby dislikes about the result afterward creates corrective tasks. It
does not retroactively let an unattended model claim quality it could not
measure.
