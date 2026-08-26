# 37ec96b invalid session-ID advisory

Audit date: 2026-08-26. Scope is only the `37ec96b` path-safe catalog claim
change in `internal/cli/setup.go`. No production or integration branch was
modified.

## Verdict: ACCEPT WITH ADVISORY; no transplant-blocking counterexample

The invalid-ID bypass is reachable from manually injected or malformed hook
stdin, but I found no evidence that supported Claude or Codex hook payloads
normally generate such IDs. Claude and Codex runtime session identifiers are
tool-generated scalar identifiers; Codex's official hook documentation
explicitly says subagent hooks use the parent session ID. RawClaw's own
slash-containing Claude IDs are internal transcript lineage IDs, derived by
`provenance.SessionIDFor` for subagent files, not values emitted by the
SessionStart hook payload.

For an invalid ID, both templates skip catalog naming and run the quoted
best-effort command `nohup "$RAWCLAW" ingest "$session_id" ... &` instead.
That preserves the safety boundary: no user-controlled value becomes a path
component, no shell metacharacter is executed, and the hook does not silently
drop the ingest attempt. If the manually supplied ID cannot resolve to a
source, the detached ingest may fail with its output suppressed; that is the
existing hook fail-soft policy for an invalid request, not loss of a supported
session. Repeated manual invocations can repeat that best-effort attempt, an
operational-noise/DoS advisory rather than a data-integrity or transplant
blocker.

## Exact implementation evidence

The Claude template classifies the parsed value at `setup.go:47–53`:

    catalog_session_id=$session_id
    case "$catalog_session_id" in
        ''|.*|*[!ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-]*) catalog_session_id= ;;
    esac

The Codex template has the identical classifier at `setup.go:152–158`.
Allowed catalog basenames are flat non-empty values containing only ASCII
letters, digits, `.`, `_`, and `-`. This includes the documented Codex-style
`thr_123` shape and UUID-like values. Slash, `..`/dot-leading values, spaces,
quotes, semicolons, pipes, backticks, and other metacharacters are rejected.

The invalid branch is explicit in both templates:

- Claude `setup.go:62–66`: invalid non-empty IDs launch quoted `ingest` and do
  not enter the catalog block.
- Codex `setup.go:167–169`: same behavior.

Valid IDs use a private temporary directory and same-directory hard-link claim:

- Claude `setup.go:67–100`.
- Codex `setup.go:170–203`.

The claim path uses only `catalog_session_id` (`entry` and `tmp_entry`), while
the original session ID remains a single quoted argument to `ingest`. The
invalid path therefore cannot traverse the catalog directory even when the
original string contains `/` or `..`.

## Reachability from supported sources

Claude's adapter derives top-level IDs from the transcript filename stem at
`internal/source/claude/claude.go:53–63`, via
`internal/provenance/provenance.go:105–124`. A Claude subagent file is assigned
the internal lineage ID `<parent>/<stem>` by `SessionIDFor`, but this is an
index/container identity derived from the filesystem. It is not read from or
fed back into the Claude hook's `session_id` field. The adapter tests pin this
split: `internal/source/claude/claude_test.go:73–105` expects a subagent
container ID `top/sub`, while the hook tests supply scalar session IDs.

Codex reads the session's own scalar `session_meta.payload.id` at
`internal/source/codex/meta.go:37–75`; it stores parent linkage separately in
`parent_thread_id` / `forked_from_id`. `internal/source/codex/codex.go:95–112`
passes that scalar `m.id` as the container ID. The official Codex hooks page
also states that subagent hooks use the parent session ID and shows
`thr_123`, which the allowlist accepts:

    https://developers.openai.com/codex/hooks

Claude's official hooks page describes `session_id` as the current session
identifier (without documenting a user-controlled path grammar):

    https://docs.anthropic.com/en/docs/claude-code/hooks

Thus a supported runtime-generated payload has no demonstrated route to the
slash-containing internal Claude lineage form. A caller that manually pipes
`{"session_id":"x/../../outside"}` into the installed script is outside the
normal source/runtime boundary and is exactly the hostile case the patch
contains.

## POSIX and regression evidence

The target's existing matrix passed under both available POSIX shells and both
templates, three shuffled race runs:

    CGO_ENABLED=0 go test -race -count=3 -shuffle=on ./internal/cli -run 'TestPrimeScripts_(SessionStartCatalogClaimIsPathSafe|SessionStartDeduplicatesConcurrentIngest|StopLaunchDetachedPrewarm)' -v
    PASS; ok github.com/MoonCaves/rawclaw/internal/cli 7.416s

That matrix covers `sh` and `dash`, Claude and Codex, existing FIFO and
directory entries, traversal `x/../../outside`, concurrent deduplication, and
detached Stop prewarm. The traversal case asserts no catalog escape and an
ingest call with the original ID; FIFO/directory cases assert the existing
object is unchanged and no nested artifact is created.

The direct POSIX classifier probe was run as:

    for shx in sh dash; do ...; done

Observed for both shells:

    sh:   abc123=>abc123, thr_123=>thr_123, UUID=>UUID,
          x/../../outside=>INVALID, x;touch SHOULD_NOT_RUN=>INVALID,
          .hidden=>INVALID, empty=>INVALID
    dash: same classifications
    shell metacharacter remained data; no command executed

Source-parser and lineage tests also passed:

    CGO_ENABLED=0 go test -race -count=1 ./internal/source/claude ./internal/source/codex ./internal/provenance
    ok claude 1.565s; ok codex 1.821s; ok provenance 2.282s

No full repository gate was run. Graphify was unavailable in this worktree
because it has no `graphify-out/graph.json`.

## Final ruling

Accept `37ec96b` for independent transplant and gate. The invalid-ID bypass is
an intentional path-safety and fail-soft behavior for hostile/manual input. It
does not weaken supported Claude/Codex session ingestion, create catalog path
escape, execute shell syntax, or violate the practical no-silent-failure
contract for valid hook payloads. Track only the low-severity advisory that a
caller can deliberately repeat detached ingest attempts with an invalid ID;
no transplant-blocking counterexample was reproduced.
