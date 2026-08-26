# Furiosa Tick 20 claim spy

Audit window: post-Tick-16 material Han/Ozzy claims, through the Tick-20
receipt at `2026-08-26T22:24:42Z`. Evidence is read-only and pinned to Git
objects and mailbox receipts. No score change is authorized by this report.

## Method constraints applied

The mandated Go skills changed the method as follows: `golang-how-to` routed
the review to testing, safety, troubleshooting, concurrency, and context;
`golang-testing` required exact filters and observable assertions rather than
coverage rhetoric; `golang-safety` required checking silent deletion and
partial-state hazards; `golang-troubleshooting` required reproduction/evidence
before a verdict; `golang-concurrency` required explicit cancellation,
ownership, and race evidence; `golang-context` required checking context
propagation and cancellation boundaries. Graphify rules required reflection
and lessons first, literal vocabulary queries, and source verification rather
than broad grep. Graphify orientation found `SyncConsolidatedFrom()` at
`internal/index/consolidated.go:553`, `ConsolidateFrom()` at line 402, and a
one-hop `ConsolidateFrom() -> pruneTombstoned()` path.

## Immutable identity and ancestry

| item | exact SHA / parent | base `8c8216e` ancestor | stable patch IDs | verdict |
|---|---|---:|---|---|
| Ozzy sidecar | `96aa522611fdcb78e281db31634144e40222de91`, parent `fb99037cda7c4ca80b6f5294631e5e5c0acc71b6` | no | parent `d54fa759`; base-to-tip `c072babb` | UNCERTAIN readiness |
| Ozzy sidecar follow-up | `c38f79acf9c9ae43ebd091a95f36837f43c0e423`, parent `96aa522` | no | parent `6a62ff59`; base-to-tip `23d4883e` | UNCERTAIN readiness |
| Ozzy no-fold A | `292284a0f4d8ded159574fc6d4aea42a7ca57763` | no | parent `81eeddee`; base-to-tip `82861bdf` | UNCERTAIN |
| Ozzy no-fold B | `f58a4c076a4dd8e89fb13e95ffce6b43edf895ce` | no | parent `60e9d6d0`; base-to-tip `d7ef2d3d` | UNCERTAIN |
| no-fold referee | `d74ff94b3c575a46adb18e2bc41c83b4a19ea2b5` | no | parent `cae264c2`; base-to-tip `cdef86cf` | NO SCORE CLAIM |
| composite contract | `34d2fb05161b1be7819b80804fca2e3a576243cf`, `878f631` parent | yes | parent `21194acb`; base-to-tip `46e4db17` | CONFIRMED ancestry |
| best-effort contract | `3b641ce9582541a60a7b37c8456bedaa9d86d29c` | yes | parent `b2d5b3e2`; base-to-tip `108af27e` | NO SCORE CLAIM |
| shrink payload | `5eb3a38309a5319befaa434483bbf97004312129` | yes | parent `73f5dd69`; base-to-tip `aabc9e67` | NO SCORE CLAIM |
| original composite | `fb99037cda7c4ca80b6f5294631e5e5c0acc71b6` | no | parent `172b0178`; base-to-tip `ae46fd42` | chronology only |
| nil-scope | `d91870634ff42b11165811111442acab26244d39` / `ef9f6ab31530bc689329e8058936473b6ae27601` | no | `5133121d` / `086cb5a3`; base-to-tip `df9d54dc` / `4a34c4d9` | REBUTTED readiness |

The composite ancestry is exactly `8c8216e -> 88e73b5 -> 878f631 -> 34d2fb0`.
The Tick-20 prosecutor receipt independently records that `3b641ce` is a
separate stale-base variant and `5eb3a38` is a separate same-base shrink.

## Claim verdicts

1. **Ozzy 96aa522/c38f79a sidecar cleanup is ready and independently gated — UNCERTAIN.**
   The commits are immutable and the sidecar tests are real: 96aa522 adds
   `TestConsolidate_DeletesSidecarsWhenSourceRemovesWholeSession`; c38f79a
   adds the no-sidecar-table case. The Ozzy receipt claims full race, gofmt,
   and diff-check at 96aa522, but supplies no immutable gate log, elapsed time,
   filter output, or independent post-merge interruption proof. Both commits
   descend from `fb99037`, not the required base, and the sidecar tests do not
   exercise process interruption. Current-base readiness and score eligibility
   therefore remain unproven; interruption-proof subclaim is **REBUTTED**.

2. **Han/Ozzy external adoption of best-effort contract 3b641ce or composite
   docs 34d2fb0 — NO SCORE CLAIM.** The Git objects prove 34d2fb0 is a
   current-base, documentation-only commit and 3b641ce is a separate
   documentation commit. The receipts establish no post-Tick-16 external
   adopter, immutable adoption receipt, or score event for either document.
   The earlier 5eec12b adoption concerns Furiosa's composite SessionID plus
   StartUUID mechanism, not adoption of these documentation commits.

3. **fb99037 should receive convergence credit against 5eb3a38 — REBUTTED.**
   Immutable commit times are `fb99037` at 2026-08-27 05:16:14 +0800 and
   `5eb3a38` at 05:27:22 +0800. The former predates the latter and is not a
   descendant of the required base; 5eb3a38 is a separate same-base shrink.
   Chronology cannot establish independent convergence, so this score claim
   is zero.

4. **e43127e is harvestable despite the fence/scope evidence — REBUTTED.**
   `e43127edc1d35e111c8b0fa5bcb19a8cb59b26ce` is a stale-base child of
   `fb99037`. The immutable Ozzy rejection receipt records missing/empty
   catalog fallback to `AllProjectDirs`, arbitrary-directory and symlink
   failures, and a held-fence block in `EnsureIndexedContainers`. Furiosa's
   adjudication explicitly marks it rejected/non-integrable and zero.

5. **d918706/ef9f6ab nil-scope branches are harvestable/current-base ready —
   REBUTTED.** Both are stale-base branches. Their diffs remove or weaken
   existing test material (d918706: 256 additions/656 deletions versus base;
   ef9f6ab: 191 additions/656 deletions including report material), and no
   receipt supplies a current-base transplant, stable gate logs, or fence-safe
   mutation result. The available fence evidence requires cancellation and
   ownership boundaries that these claims do not prove. No adoption or score is
   supported.

6. **Ozzy no-fold A/B are ready after the blocking rulings — UNCERTAIN.**
   `292284a`, `f58a4c0`, and referee `d74ff94` are immutable and pushed
   branches; A and referee are upstream-equal, while B is also present at the
   same remote tip. However all three are stale-base descendants of 96aa522,
   and the latest mailbox instructions say assertions still need repair and
   focused plus full CLI/index race gates. The referee matrix is evidence of
   intended coverage, not a passing gate receipt. No score claim.

## Score disposition

No score changes. Confirmed facts are limited to commit identity, composite
ancestry, chronology, and the explicit e43127e rejection. All adoption,
readiness, convergence, and nil-scope harvest claims remain zero or require a
new immutable external receipt with current-base state, whole/path patch IDs,
exact nonzero filters, mutation output, and independent gates.

## Receipts consulted

- Supervisor Furiosa mailbox: Tick 16 receipt, `20260826T214510Z`; Tick 17/18
  instructions and e43127e rejection, `20260826T220519Z` and `20260826T220738Z`;
  Tick 20 instruction, `20260826T222442Z`; composite ancestry correction,
  `20260826T222704Z`.
- Ozzy/Han mailboxes: sidecar head receipt `20260826T213605Z`; no-fold
  blocking rulings `20260826T221454Z` and `20260826T221455Z`; earlier immutable
  publication/adoption receipts were treated as historical context, not new
  Tick-20 score evidence.

