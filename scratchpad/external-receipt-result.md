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
- historical independent mutation reproduction `0b39b82a...`: 20/20 immediate
  kills after successful `Start` yielded zero child receipt bytes, with focused
  candidate race gate PASS in 3.022s;
- current focused race-gate result (`real 169.63s`, unrelated pre-existing
  `TestTagWriteDefaultScopeConsolidatedOnlyDoesNotBlock` failure);
- Graphify refresh/query failures due to the absent graph file; and
- first report commit `6f5667e` pushed successfully to
  `origin/worker/furiosa-external-receipt-contract-20260827`; final closeout
  commit and push receipt are added below.

Disposition: keep publication best-effort and change the foreground wording to
say authoritative write complete plus best-effort detached publication, with
possible stale consolidated reads and absent terminal receipt. A durable pending
record plus retry owner is required for a stronger eventual-publication
contract; it is not a tiny safe correction.

Final report-content commit: `c7c51068147bb6068d7f95e91859d1766fb806cc`,
pushed successfully to
`origin/worker/furiosa-external-receipt-contract-20260827`.
