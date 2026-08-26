# Catalog-claim mutation audit

Independent base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`, branch
`worker/furiosa-audit-mutation-20260827`. Graphify was the first codebase action:
`renderHookScript` and `TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath`.
Required skills, root `AGENTS.md`, `CONTRIBUTING.md`, and the requested Mnemon recall
were read; recall was treated as hypothesis only. The graph is the main checkout
snapshot and cannot see detached-branch edits.

The Claude and Codex POSIX-sh templates create a same-directory temporary entry,
publish with `ln "$tmp_entry" "$catalog_dir"`, and recognize existing paths with
`[ -e "$entry" ] || [ -L "$entry" ]`.

## Baseline

```text
time CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath|TestClaudePrimeScript_CreatesSessionCatalogEntry|TestCodexPrimeScript_Creates'
```

`PASS`; package test `5.362s`, wall `7.886s`.

The hostile matrix is 3 shells (`/bin/sh`, `dash`, `bash`) × Claude/Codex × 9 states:
new, regular, FIFO, directory, symlink, dangling symlink, symlink-directory,
symlink-FIFO, and Unix socket. Existing cases have a 2-second context timeout; new
cases have 15 seconds. It checks no special-path mutation/open, valid new JSON,
no temporary-directory leak, and one detached ingest for a new claim. No FIFO probe
is unbounded.

```text
time env CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest|TestPrimeScripts_SessionStartIngestsWhenCatalogUnavailable|TestPrimeScripts_StopLaunchDetachedPrewarm|TestPrimeScript_CatalogWriteFailure_NeverFailsHook'
```

`PASS`; package test `3.739s`, wall `4.493s`. This covers concurrent deduplication,
unavailable-parent fail-soft behavior, detached Stop prewarm, and exit-zero write
failure.

## Mutants

Both scratch variants were fresh detached worktrees at the exact base SHA above.

### M1: unsafe opening redirection (FIFO)

Scratch `/private/tmp/rawclaw-mut-fifo`, `HEAD=0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
Both `ln` claims were replaced with `(set -C; : > "$entry" 2>/dev/null)`, restoring
open-before-check behavior. Scratch diff: 2 insertions/2 deletions, net 0 lines.

```text
timeout 12s env CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath
```

`MUTATION_EXIT=124`, wall about `12.6s`, with no completion. **KILLED**: opening the
existing FIFO blocks. The timeout protected the probe.

### M2: omit dangling-symlink check

Scratch `/private/tmp/rawclaw-mut-symlink`, `HEAD=0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
Both `[ -e ] || [ -L ]` checks became `[ -e ]`; scratch diff: 2 insertions/2 deletions,
net 0 lines.

```text
timeout 30s env CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath
```

**KILLED** in `8.335s` wall: all six Claude/Codex × `/bin/sh`, `dash`, `bash`
dangling-symlink cases launched `ingest fifo-claim-test`, but expected zero.

## Traversal / invalid ID

The supported-hook matrix uses scalar tool-generated IDs; it does not claim arbitrary
manual IDs safe. In fresh detached `/private/tmp/rawclaw-mut-traversal` at the same SHA,
the fixed test ID became `../escape/invalid-session`. With `TMPDIR=/private/tmp/mut-tmp`
and its `escape` parent pre-created:

```text
TMPDIR=/private/tmp/mut-tmp timeout 30s env CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath
```

`TRAVERSAL_EXIT=1`, wall `1.302s`; it created
`/private/tmp/mut-tmp/escape/invalid-session` outside each random catalog directory,
then failed in-catalog assertions. This is advisory evidence that slash-bearing manual
IDs can escape the catalog namespace, not a supported Claude/Codex payload regression.

## Verdict

Supported Claude/Codex claims are mutation-backed for non-opening/non-blocking
special-path handling and dangling-symlink deduplication. Concurrency, fail-soft,
detached Stop, and timeout protections passed. Arbitrary invalid/traversal IDs remain
an explicit boundary/advisory.
