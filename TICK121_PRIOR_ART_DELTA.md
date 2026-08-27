# Tick 121 prior-art delta

## Run, base, and predecessor scoring

- `run_timestamp_utc`: `2026-08-27T22:02:00Z`; `run_completion_utc`: `2026-08-27T22:03:00Z` (captured with `date -u`).
- `report_base`: `758aa4417794c7a000e90f67c19e51f03817bdfd`.
- Canonical ledger SHA-256: `1e517b5cc1061c40a2a94f1dda04385a72661518a78c7b55bfe71535f988f513` (matched).
- Predecessor `98ae85385a328f58640ec262e7c189c1d821083d`; file SHA-256 `dd1f0d4aa2a10bd22123fb6e847b59c887247b73710f02701f3352726847b974` (matched); watermark `20260827T032519Z`.
- T109 at `caa77e52e4fcf2bd6b00b3a6080f5befb1f59078` scored first. Its file hash matches `33fe50bb192cc7ca3704412a613efcad0513828d97401698b22d76434e48e1cf`, but base `ae8703bf` was stale, so its claims are rejected for current-base adoption. No score event.

## Regrade and live problems

- Issue 55 semantic regression remains unresolved: focused green tests and mutant RED evidence are not a public-main semantic adoption receipt.
- Explicit GitHub merge authority/audit receipt is absent. T120 preserves current-main `758aa44`, focused race PASS, and mutant RED, and rules HOLD.
- Stale-result base binding remains required: worker result must name exact base/head/current-main SHA and be rejected when stale.
- Every recommendation since the watermark was regraded; no independently adopted fix, immutable public-main receipt, or authorized merge event was found. Score delta `0`; totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`.

## Three exact external mechanisms

### 1. Semantic regression oracle

- Source/title/date: `https://www.sqlite.org/sqllogictest/doc/trunk/about.wiki` — `sqllogictest About`; date unavailable; inspected `2026-08-27T22:02:10Z`.
- Mechanism: declarative SQL statements carry expected row output; replay mismatches fail, making semantics an explicit oracle rather than timing/exit-code evidence.
- Relevance: adapt as a compact Go table asserting Issue 55 durable result/state invariants across normal and mutation cases, with no runtime dependency.
- Recommendation `PA-SEMANTIC-RESULT-ORACLE-001`; normalized-text SHA-256 `4d5ec7d12dd0ff69b1b9b3cbd0c1a8bb0e0d0e8fcb4a4a1d388d53f0f3b83a7a`; `partial comparator`, score `0`; enriches existing `PA-SEMANTIC-BENCH-COUNTER-001`.

### 2. Explicit merge authority and audit receipt

- Source/title/date: `https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue` — `Managing a merge queue`; date unavailable; inspected `2026-08-27T22:02:25Z`.
- Mechanism: merge queue tests a temporary merge-group ref; required checks bind to that exact SHA, while audit logs record actor/action. Green checks on another head do not authorize merge.
- Relevance: require an immutable receipt naming PR, head SHA, current-base SHA, actor, decision, and required checks before public mutation; directly addresses T120 HOLD.
- Recommendation `PA-GITHUB-MERGE-AUTHORITY-RECEIPT-001`; normalized-text SHA-256 `b4bb8fd4e06f38eb7d8485bba3f7ce7be4ab9e52a67244aaf9f5f5dc7ef7a4b5`; `pending`, score `0`; documentation is precedent, not adoption.

### 3. Stale-result binding

- Source/title/date: `https://docs.github.com/en/rest/checks/runs` — `REST API endpoints for check runs`; date unavailable; inspected `2026-08-27T22:02:40Z`.
- Mechanism: every check run has immutable `head_sha`; consumers compare it to the accepted PR head/current base, so old status cannot transfer to a new head.
- Relevance: bind reports/adoption receipts to report base, candidate head, and observed public-main SHA; reject any mismatch. This catches T109’s stale `ae8703bf` anchor.
- Recommendation `PA-STALE-RESULT-HEAD-BINDING-001`; normalized-text SHA-256 `9cb91f8ae8e524e26962305aac146cc6aa5ed9e93e45ee68f1a5e1ccf1d663ba`; `pending`, score `0`.

## Dedupe and direction lock

- Duplicates rejected: T109/T120 reports, control receipts, focused greens, mutant evidence, stale-base branches, and documentation as adoption. No score-eligible events.
- `direction_lock`: `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically `LOCKED`; `NO MERGE AUTHORIZATION`.

## Proposed append bytes

The exact append candidate is between the markers below; the supervisor must append it without rewriting prior bytes.

BEGIN_APPEND
## Tick 121 prior-art delta — 2026-08-27T22:02:00Z
- prior_watermark: 20260827T032519Z
- report_base: 758aa4417794c7a000e90f67c19e51f03817bdfd
- recommendations: PA-SEMANTIC-RESULT-ORACLE-001 partial comparator (fingerprint 4d5ec7d12dd0ff69b1b9b3cbd0c1a8bb0e0d0e8fcb4a4a1d388d53f0f3b83a7a); PA-GITHUB-MERGE-AUTHORITY-RECEIPT-001 pending (fingerprint b4bb8fd4e06f38eb7d8485bba3f7ce7be4ab9e52a67244aaf9f5f5dc7ef7a4b5); PA-STALE-RESULT-HEAD-BINDING-001 pending (fingerprint 9cb91f8ae8e524e26962305aac146cc6aa5ed9e93e45ee68f1a5e1ccf1d663ba)
- changed_statuses: no existing recommendation changed; T109 rejected for stale base; no adoption
- adoption_evidence: none; T120 authority absence remains HOLD
- score_eligible_events: none; score delta 0; totals Furiosa +9, Han +2, Ozzy +3
- duplicates_rejected: T109/T120 reports, controls, focused greens, mutant evidence, stale-base branches, docs as adoption
- new_watermark: 20260827T032519Z
- next_leads: permanent Issue55 semantic oracle; immutable authority receipt; reject stale results before scoring/publication
- direction_lock: PA-CONSOLIDATED-SIDECAR-PRUNE-001 technically LOCKED; NO MERGE AUTHORIZATION
- delta_timestamp_utc: 2026-08-27T22:02:00Z
- run_completion_utc: 2026-08-27T22:03:00Z
END_APPEND

- `proposed_delta_sha256`: `147186f5455352b5d74e43f6c58b7725ffef7a207daa1f14fe6b8340315d88fc`.
- `expected_ledger_sha256`: `56f7f2d0941ea03d12b3a58e6f487b2f96ff71c72608af1018d5c91789913fca` (current ledger bytes followed by exact append bytes).

## Verification receipt

- Graphify MCP orientation was attempted first against this checkout but blocked by the repository mailbox pre-tool guard; local fallback was used and the limitation is recorded.
- No mailbox/cursor, shared ledger, product, test, scorecard, or harness state was modified.
- `PATCH`: report-only delta; no merge authorization. Worktree is exactly based on `758aa4417794c7a000e90f67c19e51f03817bdfd`.
