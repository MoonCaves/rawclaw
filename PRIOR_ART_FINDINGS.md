# Tick 29 prior-art adoption regrade

Run completion: `2026-08-26T23:58:09Z` (UTC). Exact base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
Prior valid watermark: `20260826T232815Z`. The watermark is unchanged: no later receipt was
processed in this bounded audit, and the prohibited Furiosa supervisor mailbox was not read.

## Mandatory regrade

All three recommendations introduced in the Tick 25 prior-art delta remain **pending, score 0**.
No independent adopter, immutable adoption receipt, current-base implementation, or score-eligible
event was found after the corrected watermark.

| Recommendation | Fingerprint | Status | Adoption evidence / score |
|---|---|---|---|
| `PA-SQLITE-BEGIN-IMMEDIATE-001` | `7e0c7cf321c3b845baf8518173f636fdf86eda2d81b69cd8b00b635cff80b214` | `pending` | None; score `0` |
| `PA-FTS5-DELETEMERGE-001` | `21ae4bb81dff3c0531f08b62290258442aaea11b75949aeb2e7bca2996e240a2` | `pending` | None; score `0` |
| `PA-GO-SINGLEFLIGHT-FALLBACK-001` | `1532e53cf1b582d958f6fec89bcb723cf2da7681bc696a5b7cfbc0fe4bf3465a` | `pending` | None; score `0` |

The prior-art worker report `origin/worker/furiosa-t25-mechanisms-20260827@ab885674699e235cb6e9c9eaa5209e0b4ce0775b`
is research evidence, not independent adoption. Its own report says all three had no immutable
external RawClaw adoption evidence. No later Git commit contains any of the three recommendation
IDs (`git log --all --since='2026-08-26T23:29:21Z' -S<recommendation-id>` returned no matches).

## Dedupe and locked direction

No duplicate adoption event exists to reject beyond the ledger’s existing rulings. The three
mechanisms remain distinct and complementary:

- `BEGIN IMMEDIATE` is SQLite writer admission, not crash-durable atomic-commit credit or the
  external file-fence mechanism.
- FTS5 `deletemerge` budgets search-index tombstone compaction, not source-scoped sidecar deletion
  eligibility.
- Go `singleflight.Group.Do` coalesces in-process fallback triggers, not durable CAS, outbox,
  reconciliation ownership, or completion receipts.

`PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically `LOCKED` on the recorded base/candidates
(`878f631b...`, `a78b39b`, `c38f79a`, adapted `0cd00e4`). No explicit invalidation trigger was
observed: no base change to the locked comparison, no listed production-patch change, no new
failing mutation, no gate regression, and no adoption-receipt invalidation. This is not merge
authorization.

## Evidence and commands

Ledger lines 719–726 record the three exact normalized recommendations, fingerprints, pending
statuses, zero scores, duplicate boundaries, and the prior-art worker receipt. The corrected ledger
watermark at lines 733–736 is `20260826T232815Z`.

Commands run from the read-only evidence checkout:

```sh
sed -n '680,760p' /Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/PRIOR_ART_LOG.md
git log --all --since='2026-08-26T23:29:21Z' -S'PA-SQLITE-BEGIN-IMMEDIATE-001' --format='%H %ad %D %s' --date=iso-strict
git log --all --since='2026-08-26T23:29:21Z' -S'PA-FTS5-DELETEMERGE-001' --format='%H %ad %D %s' --date=iso-strict
git log --all --since='2026-08-26T23:29:21Z' -S'PA-GO-SINGLEFLIGHT-FALLBACK-001' --format='%H %ad %D %s' --date=iso-strict
rg -n -i 'PA-SQLITE-BEGIN-IMMEDIATE-001|PA-FTS5-DELETEMERGE-001|PA-GO-SINGLEFLIGHT-FALLBACK-001|7e0c7cf3|21ae4bb8|1532e53c' /Users/jay-m4/code/rawclaw-* --glob '*.md'
git for-each-ref --format='%(refname:short)|%(objectname)|%(committerdate:iso-strict)|%(subject)' refs/remotes/origin
date -u '+%Y-%m-%dT%H:%M:%SZ'
```

The search found only the ledger’s pending entries and the original Furiosa mechanisms report;
there was no independent adopter receipt or implementation claim to re-grade. Therefore:

- changed statuses: none;
- adoption evidence: none new;
- score-eligible events: none; totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`;
- duplicates rejected: none new;
- valid new watermark: unchanged `20260826T232815Z`;
- next leads: obtain an independently adopted current-base implementation with red/green gates,
  stable whole/path patch IDs, and an immutable adopter receipt; separately test each mechanism’s
  stated boundary before considering adoption.

No Go files were edited and no Go gate was claimed. Only this report is in the editable fence.
