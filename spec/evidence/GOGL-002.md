# GOGL-002 — refresh the GOG access token server-side

**Date:** 2026-09-16 · **Commit:** this commit · **Environment:** local

## Acceptance

```bash
$ go build ./...
build ok

$ go vet ./...
vet ok

$ go test ./...
?   	nisaba	[no test files]
?   	nisaba/db	[no test files]
ok  	nisaba/handlers	0.004s
ok  	nisaba/sync	0.004s
```

Functional check, run against the local `nisaba-perf.db` copy through a
throwaway test in the `sync` package (deleted afterwards; the tree is clean —
`git status` shows only the task's own files):

```bash
$ GOG_SECRET=<galaxy client secret> go test ./sync/ -run TestManualGogRefresh -v
=== RUN   TestManualGogRefresh
    token returned: true (length 192), err: <nil>
    access token rewritten: true
    refresh token rewritten (rotated): true
    refresh token present: true
    expiry rewritten: true
--- PASS: TestManualGogRefresh (0.29s)
```

No token or client-secret value appears in this file or in the repository.

## Result

`gogGetAccessToken` (`sync/gog_auth.go`) returns a usable token: it uses the
stored one while the recorded expiry holds, and otherwise exchanges the stored
refresh token at `auth.gog.com/token` for a new pair, writing the access token,
the rotated refresh token and a fresh expiry back to `app_config`.

- **The old expiry gate is gone.** It rejected a token GOG still accepts
  (`GOGL-001`), so a lapsed stamp now causes a refresh rather than a failure.
- **A failed refresh does not discard a stored token.** Per the `GOGL-001`
  finding, the stored token is returned and the refresh error is logged, so an
  unreachable token endpoint cannot break a sync that would otherwise work.
- **The client secret is a credential, not a constant.** It is read from
  `app_config` as `gog.client_secret` and set once through Settings, so no
  secret enters the repository (`ACCEPTANCE.md`). Bobby chose this placement
  after confirming the token is not read-only (`GOGL-001`, note on the client
  secret).
- `sync/gog_wishlist.go` now calls the shared helper and no longer owns the
  client ID; the wishlist pass itself is still present and untouched — `GOGL-004`
  retires it.

What this does not prove: that a refresh succeeds when the refresh token has
been revoked (password change or logout-all-sessions), and nothing was deployed.
The live instance still holds the pre-refresh token pair and will refresh on its
first GOG call after the deploy.

## Note for GOGL-003

The helper is the only auth entry point the library sync needs; it should call
`gogGetAccessToken` and treat a `302` from `embed.gog.com` as "refresh and
retry", since both GOG endpoints redirect rather than returning 401
(`GOGL-001`).
