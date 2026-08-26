# Furiosa external nil-scope referee

## Verdict

**NO BUG. Reject the proposed nonblocking correction.**

## Invariant

When generated `tag-write <session-prefix>` resolution finds a retained session
with no surviving source or catalog row, `LocateSessionGuarded` resolves the
authoritative target to `consolidated.db`. A direct write to that target must
hold `consolidated.lock`, the same fence used by snapshot-and-replace rebuilds.
The write may wait; bypassing the fence can silently lose a committed tag when
the rebuild renames its replacement over the live database.

## Evidence

The base reproduction held `consolidated.lock` and expected nil-scope
`runTagWriteCmd` to return within 300 ms. It stayed blocked, which is expected
serialization, not a publication failure. The regression test was corrected to
require blocking while the fence is held and completion after release.

The hostile production mutation deleted only the consolidated-fence block. The
corrected test then failed immediately because tag-write returned with
`err=<nil>` while the lock was held. The mutation therefore distinguishes the
safe implementation from a superficial fence bypass.

No production code was changed.

## Receipt

- Base: `2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce`
- Worktree: `/Users/jay-m4/code/rawclaw-furiosa-external-nilscope-referee`
- Branch: `worker/furiosa-external-nilscope-referee-20260827`
- Evidence commit: `8e52d8d6cea1d6f0c1831b3d28ad3b0d59155ea4`
- Stable patch-id: `3c24d3ff98ae50d513f776e8a6d4cdea83e8cba4`
- Files changed: `internal/cli/hostile_default_scope_test.go`, this report,
  and `scratchpad/external-nilscope-result.md`; production implementation
  unchanged.
