# Furiosa Tick 11 Phase 3 — nil-scope hostile lane

Candidate under test: `857dc62414426b540b57a497609122721982a367` (`test: await detached tag publication`), based on Ozzy composite branch. Prior fast-path change: `74d4ee9` (`fix: fast-path TDir tag authoring`). Furiosa comparison winner: `8c8216e`.

## Hypothesis

The generated command `rawclaw tag-write <session-prefix>` passes `scope == nil`. The new nil-scope `locateTagWriteFast` path only succeeds when the catalog has exactly one usable hit and its project index is already fresh. If the catalog is absent/stale, or a session prefix is ambiguous across projects, the command falls through to `LocateSessionGuarded`; that lookup can invoke the all-project `ScopeFn` or resolve the derived consolidated store. Under a held consolidated fence, a mistaken consolidated resolution can block authoring even though a live per-project source is available.

## Red contract

With a nonzero filter (a concrete session prefix), generated nil-scope authoring must either resolve the unique live source and write its authoritative per-project database promptly while the consolidated fence is held, or return an explicit ambiguity/not-found result. It must not call guarded lookup or wait on the consolidated fence when the source-only path is deterministically available. Publication may remain deferred, but the source write and its eventual publication request must be correct. Duplicate-session prefixes and no-catalog fallback must remain conservative: never silently choose one project.

## Attack plan

Use disposable deterministic tests that hold `consolidated.lock`, poison the guarded `ScopeFn`, and invoke `runTagWriteCmd` with `scope == nil`. Exercise (1) the generated/default catalog path, (2) no-catalog fallback, and (3) duplicate-session ambiguity. Verify prompt return, authoritative source topics, and publication behavior. Compare the candidate against `74d4ee9` and `8c8216e` using patch-id/range-diff and net-line accounting before deciding whether a product correction is warranted.
