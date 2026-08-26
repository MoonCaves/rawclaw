# Foreground fold latency audit

Base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.

`runTagWriteCmd` writes the authoritative topic rows to the resolved database,
closes that connection, and then calls `index.SyncConsolidatedFrom` inline. The
fold is advisory in error handling, but its latency is not detached: it acquires
`consolidated.lock` synchronously before returning to the caller.

`TestRunTagWriteForegroundFoldLatency` uses the existing lock as a deterministic
fence. It proves, in separate phases:

- the topic row is readable from the project database before publication;
- the consolidated reader has no new topic while the fence is held;
- the fold remains blocked for more than the 100 ms observation window;
- releasing the fence allows the fold to publish and the consolidated reader to
  see the topic.

Observed WITA from the race test: source durable 44.6 ms; fence release 155.3
ms; fold return 238.2 ms; release-to-return 82.9 ms. The exact values are
machine-dependent; the ordering is the contract evidence.

The requested stronger assertion about `runTagWriteCmd` itself cannot be made
deterministically with the current seams. `runTagWriteCmd` performs guarded
lookup before authoring. When the consolidated row is absent, that lookup falls
back through catalog scope sweeping, which can itself call indexing and acquire
the same fence. Holding the fence before invoking the command therefore blocks
lookup/write, not the post-write fold. There is no injected lookup or fold
callback to pause after the authoritative commit. A production seam (or a
command-level hook immediately before `SyncConsolidatedFrom`) is required to
prove command return latency directly.

`TestRunTagWriteFoldsIntoTheOneStore` and its routine equivalent encode a real
immediate consolidated-visibility promise, but they also couple the promise to
foreground fold completion. The current implementation confirms that the
coupling exists; it does not prove that the coupling is desirable product
behavior. If closeout latency is the priority, publication must be detached or
the seam must make that policy explicit.

REBUTTED
