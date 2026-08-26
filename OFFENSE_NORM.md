# Norm integration hostile evidence audit

Date: 2026-08-26

Scope: immutable base `5b9756b2200ff6bd670f07407407d84d9f42d84b`; advertised integration `origin/norm/integration-wave1` at `f026d6aed1918fb2c158c71df976eaf0dbf278c8`; active heads `cc7619ec1dd0`, `6ddd17a37311`, `2cc11d683761`, `7478bfd96581`, `3530005b6143`; exact commits reachable from the integration head; Norm prewarm worker `7d5a6a550dc0` where needed to test the advertised prewarm claim.

This is report-only. No product or test source was edited. The only file added is this report.

## Verdict

### CONFIRMED DEDUCTION — integration prewarm claim is duplicate attribution, not new movement

The advertised `+2/-5` product and `+3/-9` test counts are exactly the two-file diff in `f026d6a`:

```
$ git diff --numstat f026d6a^ f026d6a -- internal/cli/setup.go internal/cli/setup_test.go
2       5       internal/cli/setup.go
3       9       internal/cli/setup_test.go
```

The same product/test patch is already committed as Norm’s prewarm worker `7d5a6a550dc018519cca8f106b86786597d66540`. The patch-id receipt is identical:

```
$ for c in 7d5a6a5 f026d6a; do git diff "$c^" "$c" -- internal/cli/setup.go internal/cli/setup_test.go | git patch-id --stable; done
e6322da4ca5faaa5b3b596fdbb33409bf376a4e5 0000000000000000000000000000000000000000
e6322da4ca5faaa5b3b596fdbb33409bf376a4e5 0000000000000000000000000000000000000000
$ git diff 7d5a6a5^ 7d5a6a5 -- internal/cli/setup.go internal/cli/setup_test.go > /tmp/a
$ git diff f026d6a^ f026d6a -- internal/cli/setup.go internal/cli/setup_test.go > /tmp/b
$ diff -u /tmp/a /tmp/b
<no output>
```

`f026d6a` does not touch `internal/cli/cmd_prewarm.go`; its only product change is removal of the impossible Antigravity hook error return. Calling this “prewarm” is review context, not a new prewarm implementation. The exact product/test movement is one patch attributed to both `norm/prewarm-ponytail` and `norm/integration-wave1`.

### CONFIRMED DEDUCTION — integration ancestry contains code also advertised through the Ozzy spy head

`f026d6a` contains `9d6564d` and `fd01a92`. `3530005b6143a41954d78be9fcd87653500b864c` has the same two commits as ancestors. The stable patch IDs are equal:

```
$ git log --reverse --format='%H %P %s' 5b9756b..f026d6a
9d6564d... 2ee9950... refactor(index): deduplicate rebuild and vault guards
fd01a92... 9d6564d... test(index): make issue 32 retry same-store and meaningful
f8b9595... fd01a92... refactor(index): consolidate phase timing
f026d6a... f8b9595... refactor(cli): remove impossible antigravity hook error

$ git patch-id --stable 9d6564d...; git patch-id --stable fd01a92...
$ git patch-id --stable over 5b9756b..f026d6a and 5b9756b..3530005b
c50a957c74c2bb06ef3865b7efdb3cd7b4cc8fb3 9d6564d...  (both ranges)
1ccc2b83923c1b93b130bce2fec51b5f410b8ecf fd01a92...  (both ranges)
```

`3530005` itself is documentation-only (`OZZY_SPY_FINDINGS.md`, `+66/-0`), so no new product movement is proved by that head. The shared code ancestry must not be counted as a second implementation.

### CONFIRMED DEDUCTION — two active worker trees contain uncommitted payloads

The advertised worker heads do not describe the complete on-disk state:

```
$ git -C /Users/jay-m4/code/rawclaw-norm-flash-catalog status --short --branch
## norm/flash-catalog
 M internal/agentproto/agentproto.go
$ git -C /Users/jay-m4/code/rawclaw-norm-flash-catalog diff --numstat
1       7       internal/agentproto/agentproto.go

$ git -C /Users/jay-m4/code/rawclaw-norm-flash-ingest status --short --branch
## norm/flash-ingest
 M internal/cli/cmd_ingest_test.go
$ git -C /Users/jay-m4/code/rawclaw-norm-flash-ingest diff --numstat
50      238     internal/cli/cmd_ingest_test.go
```

The catalog worker’s `-7/+1` inline-filter cleanup and the ingest worker’s `-238/+50` test-suite cleanup are not in `cc7619e` or `7478bfd`. They are therefore dirty, uncommitted payloads, not receipts attributable to those heads. The other checked Norm trees were clean at their named heads.

### NARROWED — hook and fence gates pass the tested contract, but are not evidence of integration

Focused gates were independently run on the worker trees:

```
$ CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestPrimeScripts_ExistingSpecialCatalogPathDoesNotBlock|TestPrimeScripts_SessionStartDeduplicatesDetachedIngest'
ok   github.com/MoonCaves/rawclaw/internal/cli  5.393s

$ CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'TestConsolidatedFence_LogsAcquireTimeoutDuration|TestConsolidate_LogsPhaseStartsAndDurations'
ok   github.com/MoonCaves/rawclaw/internal/index  2.689s
```

These tests establish only the covered special-path and log-order contracts. They do not turn the uncommitted catalog/ingest payloads into commits, and they do not establish that `2cc11d6` or `6ddd17a` was integrated into `f026d6a`.

### NO DEDUCTION — no unsafe mechanism proved in the inspected hook/fence changes

The inspected `2cc11d6` hook change uses a temporary file plus hard-link claim and has a focused FIFO/directory/symlink/socket non-blocking test. The inspected `6ddd17a` fence change adds timeout duration and ordering assertions. I did not find a source-backed failing interleaving from these exact heads; classify mechanism safety beyond those gates as unproven, not as a confirmed bug.

## Range and accounting receipts

`git range-diff --no-dual-color 5b9756b..f026d6a 5b9756b..7d5a6a5` maps the same setup change to different commit identities and shows no `cmd_prewarm.go` movement. The integration aggregate from base is `12 files, +538/-562`; its final integration commit is only `2 files, +5/-14` (`setup.go` and `setup_test.go`). The prewarm worker’s final commit is `3 files, +7/-16` only because it also edits its findings report; production/test lines remain the exact `+5/-14` patch above.

`git diff --check` was clean for the two dirty worker payloads. Clean whitespace does not make them committed or attributable.

