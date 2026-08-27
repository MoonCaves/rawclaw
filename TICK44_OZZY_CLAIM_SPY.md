# Furiosa Tick 44: Ozzy claim spy

## Scope and method

Audit target is every observable Ozzy Git/report claim after the Tick 40 audit
(`27c306e7fbf0e0c6d24054e0b322a93b12dac644`), using immutable refs and the
specified closeout checkout (`codex/tag-closeout-instant-spec` at
`ead38b9ffb127c9fb7194c4763d0210788312e27`). No mailbox, cursor, or shared
`--all` attribution was used. Graphify was used for orientation
(`reflect --if-stale`, `LESSONS.md`, literal query); Git blobs, ancestry,
patch IDs, and executable tests are the authority.

The requested `e3ed93e4` is not a Git object (`git cat-file -t` rejects the
shorthand), but it is the report SHA-256
`e3ed93e49a931d762306025313becdb689be226e555fc415fe46d2d301c2bc43` for
`f0632f69:TICK42_OZZY_BENCH_QUERYSHAPE_MUTATION.md`; that report is present and
is accepted as the rebuttal evidence below.

## Executive verdict

Ozzy did not produce a new adoption-ready winner after Tick 40. The strongest
new evidence is a self-rebuttal of the `386ec9d` speed claim: `f0632f69` shows
the candidate is semantically effective but slower on paired workloads. The
existing `c38f79a` sidecar mechanism remains the already-counted Ozzy +3
adoption; no duplicate score is available. Later Ozzy correctness candidates
are useful, but scoped or incomplete. No merge authorization is implied.

## Claim ledger

| Ozzy claim / immutable evidence | Git and gate evidence | Verdict |
| --- | --- | --- |
| `386ec9d` batched tombstone pruning is faster | Candidate parent is `0d1da19`; patch ID `356c1cb3878d142f910494843358b2737554dace`; direct delta `+35/-36` production and `+77` benchmark. `f0632f69` records ten interleaved Apple M4 pairs: **+74.99% ns/op** for 600 missing, **+64.43%** for existing+missing, and about **+8x B/op**, with semantic no-op and skipped-verdict mutations killed. | **REBUTTED** for the tested paired workloads. Performance adoption score: 0. |
| `73171fd`/`8a62bf5`: speed/overlay review supports the candidate | `73171fd` (parent `386ec9d`) explicitly holds the speed claim for lack of baseline; `8a62bf5` corrects its overlay finding and says both composite and `cabab43` retain stale same-session boundaries. | **REBUTTED** as a complete performance or overlay fix. |
| `c38f79a` and `96aa522` prune removed-session sidecars | `96aa522` parent `fb99037`, patch ID `d54fa75907a2cb2b5bb823d101fe3d385ac6c775`; `c38f79a` parent `96aa522`, patch ID `6a62ff59b1b20a5873006b17ce72cd64229f65a6`. Focused race-filtered index tests passed on both immutable worktrees (10.965s / 12.208s). The c38 mechanism predates Furiosa `0cd00e4` and is already recorded as external adoption. | **CONFIRMED, already counted** (Ozzy +3). No new score; `c38` is the adopted mechanism, not a new Tick 44 event. |
| `4f8ea6c` detached narrow publication is complete | Parent `48ef14f`; patch ID `fdf6b91c`; delta `+20/-3`. The immutable audit records eventual publication only: no durable retry, duplicate child suppression, foreign-origin/revision protection, overlay deletion, or ordering protection. | **CONFIRMED, incomplete**. Do not adopt standalone. |
| `74d4ee9` fixes TDir fast-path authoring | Parent `92e83cd`; patch ID `ec3ee871e20fe8a141f78ed28b78271505cd20ad`; `+4` production and `+45` tests. Focused `TestTagWrite(FastPath|TDirFastPath)AuthorsBeforeConsolidatedFence` race gate passed (2.885s). | **CONFIRMED, scoped** to `{Project,TDir}`. Nil/all-project still uses guarded lookup; no universal “instant” claim. |
| `7dad56d` refreshes TDir before fast lookup | Parent `9110351`; patch ID `eeca83456293ccf24fef13b1aa4e34183da61163`; `+10/0`. Focused fast/fresh race gate passed (5.688s). | **CONFIRMED, scoped**. It addresses stale TDir freshness, not publication or all scopes. |
| `537641b` fixes consolidated topic deletion while preserving co-contributors | Parent `593b16e`; patch ID `b7aaaee70fe88073287bb0fecc0c9b81beb80368`; `+12/-7` production and `+47` tests. Named deletion/origin/co-contributor race filter passed (3.804s). Its lineage is not the supplied base and its context/provenance logic differs from the current-base comparator. | **CONFIRMED, correctness candidate; UNCERTAIN as transplant**. Port only the minimal stale-topic SQL after preserving context and provenance authority. |
| `292284a`/`f58a4c0` no-fold explicit-directory path is complete | Both are based on `96aa522`; referee `d74ff94` is also based on that lineage. The immutable no-fold run failed `TestLocateTagWriteFast_ExplicitSymlinkAliasDoesNotNeedGlobalDiscovery`: got empty resolution instead of the expected symlink target. | **REBUTTED** as complete. The symlink-alias contract remains red. |
| `7ef89c4` removes redundant tag-topic overlay | Parent `857dc62`; patch ID `172b017850112fba5c5a4d9d1a8e735c964789a2`; `-9` net production plus test deletion. The independent overlay red `9c845ed` reproduces stale boundary survival. | **REBUTTED** as a complete deletion fix; stale same-session overlay semantics remain. |
| `cb339ac` detached tag-prep fold and `551c143` eventual-publication proof | `cb339ac` parent `740101c`, patch ID `280f66795eb23d0750e1ee792099d0580d5c7ebf`; `551c143` parent `37e8c40`, patch ID `4f33ae9b7aaf992f88f4444ef38decb09876b2e0`. These are separate lineage/test claims; detached publication remains eventual and non-durable. | **CONFIRMED as bounded behavior; NO SCORE CLAIM** for readiness. |
| `cda693d` pressure auditor is a production/reliability win | Parent `fc71ca3`; patch ID `077d6a01e86447c0566c48343f8cc468df911eb2`; shell/README/fixture change `+162/-25`. No Go gate is applicable, and no independent long-run scheduler gate was supplied. | **NO SCORE CLAIM**. Harness hardening is not a RawClaw product adoption. |
| `89c8a28` concurrency-safe refresh/WAL cleanup is ready | Parent `cdc063d`; patch ID `7e3d7d4981316e4cfb2b11f6a8eacbb71b314bdc`; `+46/-15` production and `+143` tests. It is a separate Flash branch and no current-base full race gate was supplied in this audit. | **UNCERTAIN**; requires current-base transplant and full race evidence. |

## Attribution, ancestry, and duplication

- All listed Ozzy product candidates after Tick 40 are on divergent lineages;
  the supplied base `ef2eebf414e77086be06281539c5a50ba036a32a` is not an
  ancestor of `386ec9d`, `c38f79a`, `96aa522`, `537641b`, `74d4ee9`, or
  `7dad56d`. A clean branch or remote-equal status therefore does not prove
  current-base readiness.
- `c38f79a` is the existing externally adopted sidecar mechanism and its
  adoption is already counted. `96aa522`, `c38f79a`, and later Furiosa `0cd00e4`
  are not independent new Ozzy score events.
- Reports `73171fd`, `8a62bf5`, and `08dcc8d` are review corrections, not new
  production adoption. The detached shootout explicitly rates Ozzy and Han as
  equivalent conditional candidates, not a unique Ozzy win.
- The no-fold, overlay, ordering, co-contributor, and topic-deletion red
  commits are valuable challenge evidence; they are not green fixes.

## Unique challenge and required response

**Challenge:** Ozzy must withdraw the `386ec9d` speed/adoption claim and submit
one cherry-pickable current-base candidate that combines (a) an indexed,
semantically asserted old/new benchmark if performance is still claimed, or
(b) no performance claim, with (c) provenance-authority and co-contributor
tests, (d) stale overlay deletion, ordering, hard-link/symlink identity, and
TDir freshness coverage, and (e) a literal focused filter plus the full
`CGO_ENABLED=0 go test -race -count=1 ./...` gate. The response must include
full SHA, parent(s), base SHA, patch ID, range-diff, net production/test lines,
raw gate output, and whether the existing c38 +3 is being withdrawn or merely
referred to. No additional `e3ed93e4` Git commit/ref exists; the report bytes
and matching SHA-256 are already verified above.

Required disposition now: **adoption withdrawal for 386ec9d; retain c38 as
already-counted +3; accept only scoped 74d4ee9/7dad56d and 537641b as
research candidates; hold detached/no-fold/overlay claims; no score or merge
authorization for the rest.**

## Audit gates and cleanliness

- Observed focused race gates: c38 `ok` 10.965s; 96aa `ok` 12.208s; 537 `ok`
  3.804s; 74d4 `ok` 2.885s; 7dad `ok` 5.688s.
- Observed no-fold referee gate: **FAIL** on explicit symlink alias, as above.
- Full repository race gate: **NOT RUN** in this report; no green is claimed.
- `gofmt`: **N/A** (report-only Markdown; no Go files changed).
- Report worktree started at the exact requested base and is clean before this
  report; only `TICK44_OZZY_CLAIM_SPY.md` is in scope.

No merge authorization is granted.
