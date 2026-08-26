# Furiosa retroactive Tick 18 mutation / duplicate evidence

Date: 2026-08-27 WITA
Base: `34d2fb05161b1be7819b80804fca2e3a576243cf`
Branch: `worker/furiosa-mutation-t18-20260827`

## Method constraints applied

I read the required Go and Graphify skills completely before inspection. The
rules that changed this run were: use the smallest CLI-facing evidence and
preserve stdout/stderr semantics; make tests capture CLI output and assert the
observable contract; reason about child-process cancellation and context
lifetimes; avoid panics, silent failure, or invented evidence; troubleshoot by
reproducing the named failure before judging it; document only claims grounded
in commands; and use Graphify's literal-vocabulary query/explain/path flow
before source browsing. The Go orchestrator also routed this as CLI, testing,
concurrency/context, safety, troubleshooting, and documentation work.

Mnemon recall was run first for the Tick 18 identifiers. Graphify had no graph
in this worktree, so the canonical parent graph was passed explicitly. Its
literal query connected `runTagWriteCmd`, `tagrefresh_test.go`, `WriteCatalogEntry`,
`startWatchdog`, and receipt-related tests; `explain Start` resolved to the
watchdog symbol. No path from `Start` to `published` was found.

## Disposable wording mutation

Mutation touched only `internal/cli/cmd_tag.go` and was fully reversed:

```diff
- store is detached best effort: a queued receipt does not mean published, and a terminal child receipt may be absent if the process environment disappears before the publisher starts.
+ store is detached publication.
- tag-write: publication queued (best effort; queued does not mean published; read-after-write is eventual)
+ tag-write: publication queued
```

The exact `git diff --no-ext-diff --binary | shasum -a 256` hash while mutated
was `80f7d490de7ccbb9bb02a35c7aafe849e88cc5b93eff37fbec6986dd7890e5c3`.
The diff was 2 insertions / 3 deletions in one file.

Command:

```text
go test -v ./internal/cli -run 'TestTagWriteQueuesDerivedPublication|TestTagWriteHelpStatesDetachedPublicationIsBestEffort'
```

Result while mutated: exit 1, ~1.7s. Both named tests were red. The queue test
observed exactly `wrote 1 topic segments for 5f3e1c20\ntag-write: publication queued\n`
and failed because the explicit best-effort phrase was absent. The help test
failed because both detached-best-effort and queued-not-published wording were
absent.

After reversal, the same command exited 0 in ~1.7s; both tests passed.

## Duplicate identity check

Stable patch IDs:

```text
88e73b5  -> 73f5dd69a25ee9f6e39bcd2036397b46661d741b
5eb3a38  -> 73f5dd69a25ee9f6e39bcd2036397b46661d741b
878f631  -> b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc
3b641ce  -> b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc
```

`git range-diff 5eb3a38^..5eb3a38 88e73b5^..88e73b5` reported `1: 5eb3a38 = 1: 88e73b5`.
`git range-diff 3b641ce^..3b641ce 878f631^..878f631` reported `1: 3b641ce = 1: 878f631`.

Ruling: reject both as duplicates. No product change is warranted.

## Process-death harness check

Repository search found the adjacent existing harness
`internal/cli/watchdog_child_test.go`, `TestWatchdog_ChildDoesNotOutliveDeadline`.
It starts a context-bound child, lets the watchdog hard-exit, and checks the
child is not orphaned. It is not a harness specifically for death between
`Start` and child entry, and it does not emit a terminal publication receipt.
Therefore no narrower terminal-receipt-byte claim is made (`UNCERTAIN`, no
matching harness).

The existing harness was nevertheless rerun five times as requested for the
available process-death evidence:

```text
go test -v ./internal/cli -run '^TestWatchdog_ChildDoesNotOutliveDeadline$' -count=5
```

Result: exit 0; 5/5 PASS; test durations 1.21s, 1.22s, 1.22s, 1.22s, 1.22s;
~7.9s wall. Terminal receipt bytes: not produced by this harness (N/A).

## Cleanup and final ruling

The disposable mutation was reversed. `git diff --check` passed and
`git status --short --branch` showed only the worker branch line, with no
tracked or untracked changes before this report was added. No mailbox helpers
or cursors were intentionally part of the work; an environment guard forced
one unrelated supervisor-mail acknowledgement before edits could continue.

Final ruling: the best-effort wording is contract-pinned by focused tests; the
requested weakening is a valid red mutation, but no correction should persist.
The four candidate commits are exact duplicate pairs and must not receive
independent credit.
