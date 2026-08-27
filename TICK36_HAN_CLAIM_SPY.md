# Tick 36 Han claim spy

## Scope and verdict

Audit window: post-Tick-32 claim activity after the Han claim-spy cutoff
`2026-08-27T00:31:43Z`, using fixed RawClaw base
`0d1da19c4c21961b86cb3ca84ed047d941c83ed3`. Furiosa’s supervisor mailbox was
not read or modified. Han supervisor and Han-owned worktrees were inspected
read-only.

**Score impact: 0.** No post-Tick-32 Han item has an independently acknowledged
adoption or score-eligible rebuttal. Scheduler/control mail, silence, and our
outbound challenges are not score claims.

## Inventory and state

Han supervisor branch `supervisor/han-mechanism-20260827` remains at
`0d1da19c4c21961b86cb3ca84ed047d941c83ed3`, upstream equal. Its mailbox after
the cutoff contains scheduler directives and Furiosa challenges only; no Han
worker reply establishes adoption. The following Han-owned worktrees were
clean and upstream-equal at inspection unless noted:

| Worktree / branch | HEAD | merge-base with `0d1da19` | state |
|---|---|---|---|
| `rawclaw-han-luna-tag-overlay` / `han/luna-tag-overlay-20260827` | `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e` | `0d1da19` | clean, 0/0 |
| `rawclaw-han-luna-detached-tag` / `han/luna-detached-tag-publisher-20260827` | `d2315cbbdcbf2edf6e3f9f8b4821acf9c622afca` | `0d1da19` | clean, 0/0 |
| `rawclaw-han-luna-overlay-publisher-integration` / `han/luna-overlay-publisher-integration-20260827` | `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` | `0d1da19` | clean, 0/0 |
| `rawclaw-han-luna-attack-furiosa-fold` / `han/luna-attack-furiosa-fold-20260827` | `0400fdb25708c234460ef10ad6440052684e7bf8` | `0d1da19` | clean, 0/0 |
| `rawclaw-han-luna-invalidation-attack` / `han/luna-invalidation-attack-20260827` | `f2e20d1a0cb7578dda9ef1ceb01296b97ed614c2` | `0d1da19` | clean, 0/0 |
| `rawclaw-han-luna-skill-adoption-blitz` / `han/luna-skill-adoption-blitz-20260827` | `ef2fdb7c666bcb4e317cf7965bf138e9176f63ee` | `0d1da19` | clean, 0/0 |
| `rawclaw-han-luna-candidate-stomp` / `han/luna-candidate-stomp-20260827` | `11bb89443f8dbfbf915a22bc22cc0af88f0bba18` | `0d1da19` | no upstream configured |
| `rawclaw-han-luna-ozzy-harvest` / `han/luna-ozzy-harvest-20260827` | `2b5416d332c51d5fa733378bec86ccf3db22bc65` | `0d1da19` | clean, 0/0 |
| `rawclaw-han-luna-harness-audit` / `han/luna-harness-audit-20260827` | `34c801606662faafab17ff0ac3c26665ef9625a0` | `0d1da19` | clean, 0/0 |
| `rawclaw-han-luna-prior-art` / `han/luna-path-prior-art-20260827` | `dd5457194f718dc2eb6ed14f46a3b8c00c2b9f69` | includes base history | no upstream configured |

The untracked `.cursor` and one acknowledgement file in the Han supervisor
worktree are pre-existing local mailbox plumbing, not claim evidence.

## Material claim rulings

### `cabab43` authoritative overlay: UNCERTAIN, narrowly supported

Commit `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e`, parent `9a1b53c`, stable
patch ID `72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6`. It changes
`internal/cli/tagrefresh.go` (+39/-1) and adds a 41-line test. The branch is a
clean descendant of the fixed base with 3 files changed, +87/-1 overall.

Exact gate:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli \
  -run '^TestRunTagPrepCmdReadsCommittedTagBeforeConsolidatedFold$'
ok ... 2.389s
```

The exact `-list` filter matched one test. The test proves a committed
authoritative topic is visible when the derived fold is absent. The implementation
overlays/replaces matching start UUIDs but appends authoritative rows to all
derived rows; it does not prove deletion of derived rows absent from the source.
Therefore the narrow visibility behavior is **CONFIRMED**, while the implied
complete authoritative-set contract/product readiness is **UNCERTAIN**. No
adoption or score event is evidenced.

### `d2315cb` detached publisher seam: UNCERTAIN

Commit `d2315cbbdcbf2edf6e3f9f8b4821acf9c622afca`, parent `7edd58d`, stable
patch ID `17db9874f86317dda02a64327fc584d35b0318e2`. The branch is clean,
fixed-base descendant, 5 files changed (+183/-11 overall). It adds a detached
`tag-publish` child and a test that replaces the spawn seam.

Exact gate:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli \
  -run '^TestTagWriteQueuesDerivedPublication$'
ok ... 3.542s
```

The exact `-list` filter matched one test. This proves the foreground command
requests one publisher and reports “publication queued” under a test seam. It
does not prove child survival after parent exit, bounded cancellation, terminal
receipt semantics, or eventual publication. **UNCERTAIN**; no score.

### `8e9c9b7` overlay/publisher integration: CONFIRMED narrowly; readiness UNCERTAIN

Commit `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`, parent `4119698`, stable
patch ID `4aef91de56b2e0c4756103ebedeae821f1570dec`. Clean/upstream 0/0,
fixed-base descendant; range is 12 files, +742/-17. The production change
prunes only sole-source topic rows absent from the incoming source and the
branch carries context-aware publisher/overlay tests.

Exact filter list matched four tests:

```text
TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication
TestOverlayAuthoritativeTopicsReplacesSessionSet
TestOverlayAuthoritativeTopicsRemovesDeletedTopics
TestRunTagPublishChildHonorsCanceledContext
```

Exact gate:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli \
  -run 'TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication|TestOverlayAuthoritativeTopics(ReplacesSessionSet|RemovesDeletedTopics)|TestRunTagPublishChildHonorsCanceledContext'
ok ... 3.192s
```

The focused tests personally pass and cover delayed authoritative visibility,
deleted overlay rows, cancellation of the context-aware child, and
co-contributor-preserving source pruning. This is **CONFIRMED** for those
specific contracts. It is not proof of full-repository readiness, process-exit
survival, or all cancellation layers; broader readiness remains **UNCERTAIN**.
No recipient adoption or score event is present.

### `0400fdb` foreground-fold attack: CONFIRMED narrow rebuttal; no product score

Commit `0400fdb25708c234460ef10ad6440052684e7bf8`, parent `ba4213f`, stable
patch ID `49982ee22cb7aef25d7297cd82a866c363bb45fe`. Report hash:
`76354b68380739ce428435e691c0f1c2a8688375a6c14ffdf2178357d811b1e7`.

The report’s imported candidate was `8aab2cb...`, cherry-pick result
`ba4213f...` on exact base. Its exact filter matched one test. Personally
reported gates were `CGO_ENABLED=0 go test -race -count=1` PASS in 2.138s and
`-count=5` PASS in 7.207s. The disposable assertion mutation still passed,
showing test semantic blindness. The test calls `runTagWrite` plus
`SyncConsolidatedFrom`, not `runTagWriteCmd`; thus the narrow claim that the
consolidated fold blocks on the held fence is **CONFIRMED**, while a
command-level foreground latency conclusion is **UNCERTAIN**. This is a
report-only rebuttal, not an adoption event.

### `f2e20d1` invalidation attack: CONFIRMED against stale candidate; no score

Commit `f2e20d1a0cb7578dda9ef1ceb01296b97ed614c2`, parent `fab3c3d`, stable
patch ID `777ddd66b72b5aecb4a5439976cef981fc2cb48a`. Report hash:
`bb6add61de9c927bffd6f43d162dbc30f8d3763eded0293f19743321528c7fd9`.

The report reproduces two red proofs on the exact stale candidate: canceled
publisher waits beyond the 300 ms bound because the fold loses context, and
deleted authoritative topics remain in the overlay. Its focused command reports
both intended tests failing; the broader package gate fails only on those
intentional red proofs. The assertion and cancellation-failure mutations were
reported as false greens and restored. The attack verdict is **CONFIRMED** for
the candidate defects; the later `8e9c9b7` branch is a separate, narrowly green
correction and is not automatically adopted.

### `11bb894` candidate-stomp adjudication: CONFIRMED as metadata ruling

Commit `11bb89443f8dbfbf915a22bc22cc0af88f0bba18`, parent exact base, stable
patch ID `88256ab4b9971d85a959905e2056b37f0fda5325`. Report hash:
`f0e1998d894631f894daf0bce4fda121d7e63da1db3c224668b8eea5ba2fd468`.

It correctly rejects the candidate set as a clean adoption set: `bd8346c` is
contaminated by `61b7957`, `37ec96b` is older-lineage, and `61b7957` is a
separate test-only benchmark refactor. Hostile execution and benchmark
behavior were explicitly **UNCERTAIN/UNRUN**. The ancestry/patch-identity
ruling is **CONFIRMED**; no product adoption or score.

### `ef2fdb7` adoption packet: NO SCORE CLAIM

Commit `ef2fdb7c666bcb4e317cf7965bf138e9176f63ee`, parent `72bf935`, stable
patch ID `7544eee22a6a84ab1a4562696fa9f19295e17711`; report hash
`5d7bb2cee847c1de43d7f7afce8fbb6b4e5de97ff2823fe9392176db45291161`.

The packet explicitly says it does not claim score, merge authorization, or
readiness. It records `00e587d` as partial/blocked by the deleted-topic ghost
and records mailbox-clock hardening gates as research input. **NO SCORE CLAIM**;
the packet’s own “adoption” language is a proposal ledger, not recipient
acknowledgement.

### Han harness, ticker, Graphify, cadence, and Ozzy-harvest reports: NO SCORE CLAIM

These are process or research reports, not product-adoption claims:

- `34c8016` (`HAN_HARNESS_AUDIT.md`, stable patch ID
  `496aa5298aeecae75aba1c131f20b822e67688aa`, report hash
  `f61a62e37a0b9d0cbc2e5f54283ddc90790b637d141406765f0a99a101447a9b`) concludes the Han launch
  contract FAIL/UNCERTAIN because no ten-minute scheduler/watchdog was
  observed, despite live mailbox blocking. **CONFIRMED** as an audit finding;
  no score.
- `9cc6099` (`HAN_TICKER_ACTIVATION.md`, patch ID
  `1e8c09771d03365c519209768fa5b81f265dcc0b`, report hash
  `81cf1344cb1fd26e6105132a3b272b178a7590bbed2576f4b152cba64d0ce73f`)
  reports one one-shot ticker execution PASS but persistent activation
  UNCERTAIN. **NO SCORE CLAIM**.
- `4e6a84e` (`HAN_GRAPH_MECHANISM.md`, patch ID
  `9753641cf7461bbcc00ac7cff5871c9b7f68ae4a`, report hash
  `b98604ef43a61c2c901bd3ca4b460247c49473caf2ca4296bfa9224f2f465a8e`) is Graphify-only orientation;
  it explicitly says graph evidence cannot prove candidate behavior. **NO SCORE CLAIM**.
- `7f5217c` (`HAN_PERIODIC_SKILL_CADENCE.md`, patch ID
  `6094df8d09a7f1e8df5f39a1c881bab523af1c0e`, report hash
  `f2d2a1b7f6ac1f760fa5a5b0165c87ccf934375ffc8f91b2bd1c3bf0a4c858c9`) defines process cadence and
  separately notes Furiosa launch-mechanism adoption credit. It does not prove
  a new Han product claim. **NO SCORE CLAIM**.
- `2b5416d` (`HAN_OZZY_HARVEST.md`, patch ID
  `db7280913bd7eb7a1b7c2036e995fed344f280c2`, report hash
  `2bc2c637ea2a11eb60038848411995e96046f33b0a30131ce90b5600f9c96323`) rejects Ozzy product transplants
  as duplicate/stale and adopts only a research ledger. Its report says no
  fresh rival test was personally run. **NO SCORE CLAIM**.
- `dd54571` (`HAN_PATH_CLAIM_PRIOR_ART.md`, report hash
  `5aa0e7d60a584a1a6c06c6e848d6cfe75ceef7c46ffcbd02ccc64e22370b998e`) records POSIX prior art only;
  external mechanism presence is not RawClaw adoption. **NO SCORE CLAIM**.

## Scheduler/control mail and outbound challenges

Han mailbox messages `20260827T004443Z` (Tick 34 claim spy),
`20260827T005444Z` (Tick 35 prior art), and `20260827T010444Z` (Tick 36
mutation review) are scheduler directives. Furiosa messages
`20260827T005120Z` and `20260827T010146Z` are outbound challenges asking Han
to prove modernc progress-handler applicability and context-bounded
`BEGIN IMMEDIATE`. They are not Han claims, and no silence or missing reply is
converted into a score.

## Sharp next challenge

Challenge the strongest remaining Han product candidate, `8e9c9b7`, on a fresh
fixed-base reproduction that combines: detached child process exit, cancellation
while the consolidated fence is held, transaction and watermark-query
cancellation, deleted-topic replacement, and co-contributor preservation. The
focused four-test green is useful but does not establish those layers together.
Require an exact test-list count, a disposable mutation that turns each
assertion false, full race evidence, report hash, and recipient adoption before
any score or merge credit.

## Final acceptance

This report is the only intended file change. No RawClaw product files or rival
worktrees were edited. All unrun experiments remain **UNCERTAIN**. The final
report commit must be clean and upstream-equal; `gofmt` is not applicable to
this Markdown-only lane.
