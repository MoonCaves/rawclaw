# Han hostile rival parity census

Date: 2026-08-27 WITA
Audit base: `fb99037cda7c4ca80b6f5294631e5e5c0acc71b6`
Working HEAD after the required report checkpoint: `ee65257`
Mode: report-only; no production-code transplant or merge authorization.

## Verdict

`fb99037` is a real, current-base, integrated deletion with an independently green focused
and full race gate. Its `-25` total-line claim is exact for the commit payload (`+3/-28`), but
that does not prove instantaneous all-scope tag-write. The TDir path still refreshes the complete
tree through `EnsureIndexedTreeSource` and recursive `ContainedJSONL`.

`571f128` is a valid sibling test receipt, not an ancestor of the audit base. Its latency claim is
not personally verified here: the named test is absent from this checkout and the exact filter
returned `PASS ... [no tests to run]`. The 19-second evidence test is therefore report-only.

`75d1656` is a functionally useful but over-large first sidecar deletion candidate. Its guarded
cleanup is incomplete when the source has no topic/verdict tables. `c38f79a`, a newer descendant
of the current base, closes that specific gap with a smaller production delta than the original
candidate but still needs an independent current-base gate before adoption.

The detached-publication gap is independently evidenced by `987c6a3`, `0b39b82`, and `1ddf6ba`.
Those are documentation/reproduction receipts, not a durable publication fix. `2b5416d` and
`be086d9` are stale or report-only documentation. No claim earns score: no external adoption or
Direction Lock was observed.

## Exact rival parity matrix

| SHA | Resolution, ancestry, payload | Patch identity / net lines | Gates personally observed | Status and ruling |
|---|---|---|---|---|
| `857dc62414426b540b57a497609122721982a367` | Resolves; parent `cd6efe3`; ancestor of `fb99037`; two test files await detached publication. | patch-id `0077c8eff6037e6a208e6343586284d12116dbc3`; `+25/-15`, net `+10` test lines. | Current-base full race includes its tests and passed; no standalone historical gate claimed. | Integrated ancestor; test-harness support, not a new product mechanism; no score. |
| `fb99037cda7c4ca80b6f5294631e5e5c0acc71b6` | Resolves; parent `857dc624`; exact audit-base tip; removes `overlayAuthoritativeTopics` and two unit tests. | patch-id `172b017850112fba5c5a4d9d1a8e735c964789a2`; production `+3/-12`, test `-16`; total `+3/-28`, net `-25`. | Focused CLI race: `TestRunTagPrepCmdReadsCommittedTagBeforeConsolidatedFold`, `TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication`, `TestRunTagWriteDefaultCatalogFastPathBeforeFence`, `TestTagWriteTDirFastPathAuthorsBeforeConsolidatedFence`, `TestTagWriteQueuesDerivedPublication` passed. Full `CGO_ENABLED=0 go test -race -count=1 ./...` passed. | Integrated/current-base ready. Correct deletion of redundant overlay bookkeeping; not proof of all-scope instantaneous behavior. |
| `571f128eedc4b1760a9b139fd1732d31180a97f2` | Resolves; parent `9054dee`; sibling of `fb99037` (merge-base `857dc624`), not in audit-base ancestry; test-only latency instrumentation. | patch-id `4c6d70c17de20634a0a9c084809c8a138892812f`; `+27/-12`, net `+15` test lines. | Exact filter on audit base: `TestTagPrepLatencyAndHeldConsolidatedFence` matched no tests and returned pass/no-tests. No claimed 0.2–0.7s or 19s result was reproduced. | Report-only/unintegrated; latency claim `UNCERTAIN`, not score eligible. |
| `2b5416d332c51d5fa733378bec86ccf3db22bc65` | Resolves; parent `418bfa7`; Han harvest branch; not an ancestor of the audit base. Removes one EOF blank line from `HAN_OZZY_HARVEST.md`. | patch-id `db7280913bd7eb7a1b7c2036e995fed344f280c2`; docs `-1`, net `-1`. | `git diff --check` clean for the payload; no product gate applicable. | Stale/docs-only hygiene; no mechanism, no adoption, no score. |
| `75d16566290a6932223540abd70379fd359c1cf5` | Resolves; parent `857dc624`; sibling candidate, not audit-base ancestry; adds whole-session sidecar cleanup and test. | patch-id `88512d6e9bddea3f848b235a72ea9dc823c0197f`; production `+48`, test `+51`; total `+99`, net `+99`. | Equivalent current-base deletion, co-contributor, and removed-session tests passed, but the candidate-specific new test was not run from this checkout. | Functionally useful but bloated and incomplete: `hasTopics`/`hasVerdicts` guards skip cleanup when source sidecar tables are absent. Superseded by `96aa522`/`c38f79a`; no score. |
| `8c8216e25e22496b2e3e919fce836be49d692e25` | Resolves; parent `2ca75b9`; independent Furiosa/Han comparator, not audit-base ancestry. Narrows topic deletion to sole-source ownership. | patch-id `3a409032463981bbdcf625eeeac1ff9424973a14`; production `+15/-12`, net `+3`. | No candidate checkout gate run here; current-base equivalent deletion and co-contributor tests passed, which is not candidate adoption proof. | Semantically useful comparator; current-base readiness unproven; independently superseded in the later sidecar line by `96aa522`/`c38f79a`; no score. |
| `987c6a31186bb15615175c5198389aa0d31846f6` | Resolves; parent `0d1da19`; report-only detached-publication finding; not audit-base ancestry. | patch-id `33c107108b0ddd123cddaea88f5119e157c392a2`; docs `+47`, net `+47`. | Receipt records a focused gate, but this desk did not rerun the candidate checkout or the external kill harness. | Valid red evidence: `Start`/`Release` does not guarantee a terminal child receipt. Report-only; no fix or score. |
| `0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f` | Resolves; parent `987c6a3`; independent referee reproduction; not audit-base ancestry. | patch-id `59e62cb90a5b5eddd0b7163478148a5eb69b52dc`; docs `+28`, net `+28`. | The receipt reports 20 immediate-kill runs, but this desk did not rerun the temporary harness. | Corroborating valid gap evidence, not adoption and not a durable fix; no score. |
| `be086d9468e1b3c779932993598c9ae1853aee8e` | Resolves; parent `ead0faa`; nil-scope disposition report; not audit-base ancestry. | patch-id `07a4c37af1dcb7bb0cb273569786ceaa23c3fe2e`; docs `+10`, net `+10`. | Current-base default-catalog and TDir tests passed; the specific disposable mutation was not rerun. | Report-only/no-bug assertion. It does not establish all-scope speed; no score. |
| `1ddf6badf91fa75451cc3a58d585b73e7667788b` | Resolves; parent `8c8216e`; terminal-receipt ruling; not audit-base ancestry. | patch-id `6f80337c2b920ce188e00b32b33844880493c39d`; docs `+15/-8`, net `+7`. | No candidate-specific test; current full race is green but cannot validate this report’s historical mutation. | Valid report-only ruling: queued means launch accepted, not published. No score. |

## Newer relevant immutable receipts

These were discovered read-only after the named set and are included to prevent stale parity:

| SHA | Relationship and payload | Ruling |
|---|---|---|
| `96aa522611fdcb78e281db31634144e40222de91` | Descends directly from `fb99037`; patch-id `d54fa75907a2cb2b5bb823d101fe3d385ac6c775`; `+20` production lines and `+51` test lines. | First sidecar-pruning follow-up; preserves co-contributor guard but is superseded/incomplete for source DBs lacking sidecar tables. |
| `c38f79acf9c9ae43ebd091a95f36837f43c0e423` | Descends from `96aa522`; patch-id `6a62ff59b1b20a5873006b17ce72cd64229f65a6`; `+20/-20` production and `+48` test lines, net `+48`. Moves removed-session topic/verdict cleanup outside sidecar-presence guards and adds the missing regression. | Newest current-base sidecar candidate observed; plausible adoption candidate, but candidate-specific focused/full gates and mutation proof were not run here. |
| `e43127edc1d35e111c8b0fa5bcb19a8cb59b26ce` | Descends directly from `fb99037`; patch-id `ce9dc82e0c222cc51d3424581dc811c6b6814ea6`; `+42` production and `+48` test lines, net `+90`. Replaces whole-TDir refresh with resolved-session container refresh. | Directly attacks the all-scope latency challenge, but is not in this audit branch and was not independently gated here. Candidate only; no score. |
| `69d8d0b59e8f89dde3c06ae8f72773ccea4aca00` | Furiosa integration branch from `138ec8c`, not current-base ancestry; patch-id `89cfa75a722d206c5e94a7beda1f52c73ceffa35`; large detached-publication transplant (`+666/-864` versus `fb99037`). | Overlap-heavy composite comparator; no current-base adoption credit. Its own descendant receipts clarify best-effort semantics but do not close durable terminal completion. |
| `d91870634ff42b11165811111442acab26244d39` | Furiosa nil-scope follow-up from `ea2b28a`; patch-id `5133121d630d549c255f82606b13c1012c6c748f`; `+8/-32`; removes the consolidated fence from default tag-write. | Conflicts with the safety contract recorded by `be086d9`: treating consolidated-only writes as nonblocking risks lost writes during snapshot replacement. Rejected pending a fence-held mutation that proves safety. |
| `3b641ce9582541a60a7b37c8456bedaa9d86d29c` / `cb154d939612a7b1be078f1eede385553858a5de` | Best-effort wording follow-ups; patch-ids `b2d5b3e2afbf8ef404e95e356b38f5d8d35bcc` and `773fd509dfdc771f0659656221a1fe459d614d41`; docs/help/test only. | Honest contract clarification, not terminal publication reliability and not a product correctness adoption. |

## Deduplicated claim census

1. **Authoritative topic read / overlay removal.** `fb99037` deletes a redundant merge helper and
   two unit tests. The remaining function returns the complete authoritative source set. This is
   distinct from consolidated source-scoped deletion and should not be double-counted against
   `75d1656`, `8c8216e`, `96aa522`, or `c38f79a`.
2. **Foreground latency versus detached completion.** `571f128` measures two quantities, but its
   test is absent from the audit base. It cannot substantiate a current-base latency claim. The
   detached receipt gap (`987c6a3` → `0b39b82` → `1ddf6ba`) is a separate process-lifetime claim.
3. **Source-scoped topic deletion.** `8c8216e` and `75d1656` address related deletion boundaries
   but are not patch-identical. `96aa522` is the direct current-base follow-up to `75d1656`; the
   newer `c38f79a` is the no-sidecar-table correction. Count one correctness family, not four wins.
4. **TDir/all-scope speed.** `e43127e` is the only newly observed payload that removes the
   `EnsureIndexedTreeSource` whole-tree refresh from the TDir fast path. Its claim remains pending
   because no independent current-base gate was run here.
5. **Nil-scope consolidated writes.** `be086d9` reports the fence as intentional; `d918706`
   removes that fence. These are contradictory claims, not additive mechanisms. The current source
   still acquires `ConsolidatedFence` for `dbp == ConsolidatedPath()` at `cmd_tag.go:483-493`.
6. **Documentation/harness hygiene.** `2b5416d`, `987c6a3`, `0b39b82`, `be086d9`, and `1ddf6ba`
   have no production payload. They can preserve evidence, but cannot score product adoption.

## Evidence-backed challenges

### Challenge A: “instantaneous all-scope tag-write” is disproven by the current call path

At `internal/cli/tagresolve_fast.go:15-30`, nil scope resolves a unique catalog hit, then calls
`index.EnsureIndexedTreeSource(db, tdir)`. That function at `internal/index/index.go:1204-1245`
calls `ensureIndexedTree`, which updates the index from the tree. The tree update reaches
`paths.ContainedJSONL` at `internal/index/index.go:888`; `ContainedJSONL` recursively enumerates
`*.jsonl` under the transcript directory (`internal/paths/paths.go:192-215`). Therefore the TDir
path is targeted by session lookup but not O(1) with respect to the project transcript tree.

Personally observed evidence: `TestTagWriteTDirFastPathAuthorsBeforeConsolidatedFence` passed,
but took 0.88s including setup and a refresh/fold cycle; that test proves ordering and authoring,
not an all-scope latency bound. The focused `-race` gate and the full race gate do not change this
semantic fact.

### Challenge B: `75d1656` cannot be treated as complete sidecar deletion

Its new topic cleanup is inside `if hasTopics` and verdict cleanup inside `if hasVerdicts`. The
newer `c38f79a` diff removes those guards around the affected-session cleanup and adds
`TestConsolidate_PrunesExistingSidecarsWhenSourceHasNoSidecarTables`. This is direct source
evidence that `75d1656` left a reachable legacy/no-sidecar boundary unhandled. `c38f79a` is a
distinct follow-up, not proof that the original candidate was complete.

### Challenge C: queued detached publication is not terminal success

`987c6a3` identifies `cmd_tag.go` printing queued after `Start`, while `tagpublish.go` releases
the process and emits terminal output only inside the child. `0b39b82` records an independent
20-run immediate-kill reproduction with zero child receipt bytes. `1ddf6ba` correctly narrows the
contract to launch accepted / best effort. These receipts establish a gap and a contract ruling;
they do not establish a durable queue, retry owner, or completed publication.

## Personally observed gates and limitations

- `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunTagPrepCmdReadsCommittedTagBeforeConsolidatedFold|TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication|TestRunTagWriteDefaultCatalogFastPathBeforeFence|TestTagWriteTDirFastPathAuthorsBeforeConsolidatedFence|TestTagWriteQueuesDerivedPublication' -v`: pass.
- `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'TestConsolidate_DeletesTopicsRemovedFromSource|TestConsolidate_PreservesTopicsWhenCoContributorRemains|TestConsolidate_DeletesSessionRemovedFromSource' -v`: pass.
- `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestTagPrepLatencyAndHeldConsolidatedFence' -v`: pass with `[no tests to run]`; this is a failed evidence attempt, not latency proof.
- `CGO_ENABLED=0 go test -race -count=1 ./...`: pass; all packages completed, including `internal/cli` 364.465s and `internal/index` 394.269s.
- `/Users/jay-m4/go/bin/dupl -threshold 100 internal/cli internal/index`: `Found total 0 clone groups.`
- `/Users/jay-m4/go/bin/golangci-lint run ./internal/cli ./internal/index`: existing five findings in `internal/cli/tagpublish.go` (`errcheck` x3, `staticcheck` QF1003 x2); not introduced by this report.
- `gofmt -l internal/`: empty; `git diff --check`: clean; final tree is clean after the report commit.
- Graphify reflection was run and `graphify-out/reflections/LESSONS.md` read. The checkout has no
  `graphify-out/graph.json`, so literal `query`, `explain`, and `path` attempts could not produce
  graph evidence; no graph artifact was generated because the lane permits only `FINDINGS.md` and
  `RIVAL_CENSUS.md` edits.
- Required harness documents were read from the read-only session-recovery harness copy. Its valid
  prior-art watermark is `20260826T202709Z`; no mailbox cursor, helper, or `.agent-mailbox` state
  was touched.

## Adoption and score ruling

No score. No immutable external adopter receipt, same-base transplant receipt, or complete
Direction Lock was observed. The full race gate passing on the audit base is verification of the
checkout, not adoption of any sibling candidate. Recommended next action is a fresh current-base
comparison of `c38f79a` and `e43127e`, with mutation tests for no-sidecar cleanup, TDir transcript
growth, and consolidated-fence loss before any integration decision.
