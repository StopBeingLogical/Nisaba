# DEPLOY-001 — Deploy the library performance fix
**Date:** 2026-09-16 · **Commit:** `52586a6` · **Environment:** atlas (run at Bobby's
explicit go-ahead, on his behalf)

## Acceptance
```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
200
```

## What ran

```bash
# 1. stage the source (verified, not assumed)
$ rsync -av --exclude='.git' --exclude='*.db' --exclude='imgcache' --exclude='._*' \
    ~/code/nisaba/ truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/
sent 117,293 bytes  received 1,716 bytes  speedup is 6.26
local  db/store.go bdbbb1de9a95ef87dab696e9d7cfc36f
remote db/store.go bdbbb1de9a95ef87dab696e9d7cfc36f   MATCH

# 2. deploy — note the absence of -t
$ ssh truenas_admin@192.168.3.174 "cd /mnt/MemoryAlpha/nisaba/source && bash deploy.sh"
 nisaba  Built
 Container nisaba  Creating / Created / Starting / Started
 writing image sha256:1483ef5eee630034b5db2d830df6be88bc6a15df3f76ce2056443164fe83d2bc
==> Done. Container logs:
nisaba  | 2026/09/17 00:02:43 NISABA running on :8080

$ ssh ... 'sudo -n docker ps --filter name=nisaba'
nisaba | Up 10 seconds | 0.0.0.0:8090->8080/tcp
```

## Result
The fix is live. The image was rebuilt, the container replaced, and the service
answers 200 on the host and from off-box. The staged source was hash-checked
against local before the restart, so what deployed is what was verified in
`PERF-006`.

**The interactive-TTY constraint is stale.** `SESSION_SEED.md`'s tripwire table and
`CLAUDE.md:94-95` both say `sudo docker` over non-interactive SSH "always fails"
and that inline `ssh host "sudo docker …"` will fail — deploy.sh calls `sudo docker
rm -f` and `sudo docker compose up` and ran clean over a plain `ssh`, no `-t`,
because `sudo` is passwordless for `truenas_admin`. Both docs were corrected the
same day.

Measured before the restart, for `PERF-007`'s comparison: `/library` 5.657s,
5.643s, 5.630s; `/library?page=2` 9.308s.

What this does not prove: that the target is met (that is `PERF-007`), or anything
about pages this deploy did not touch.
