# External receipt result

The detached tag publisher receipt guarantee is refuted. `cmd.Start()` success
means only that a `Process` exists; `Process.Release()` gives up parent wait
resources. The parent then reports queued, while the child alone writes terminal
success/failure after entering `runTagPublishChild`. A kill or equivalent child
failure after `Start` and before child entry therefore leaves queued as the only
user-visible result and no terminal log line.

Evidence is recorded in [FURIOSA_EXTERNAL_RECEIPT_CONTRACT.md](../FURIOSA_EXTERNAL_RECEIPT_CONTRACT.md), including:

- candidate source lines and exact SHAs/patch-ids;
- candidate test coverage and its missing post-`Start` mutation;
- current focused race-gate result (`real 169.63s`, unrelated pre-existing
  `TestTagWriteDefaultScopeConsolidatedOnlyDoesNotBlock` failure);
- Graphify refresh/query failures due to the absent graph file; and
- the final commit and push receipt, to be filled after commit/push.

Disposition: keep publication best-effort and change the foreground wording to
say authoritative write complete plus best-effort detached publication, with
possible stale consolidated reads and absent terminal receipt. A durable pending
record plus retry owner is required for a stronger eventual-publication
contract; it is not a tiny safe correction.
