# Tick 45 live technical-problem census

Run boundary: `2026-08-27` WITA, report-only, exact audit base
`ef2eebf414e77086be06281539c5a50ba036a32a`. This is an independent census;
earlier rankings are not reused. No mailbox, cursor, scorecard, or ledger was
read or changed. No score change or merge authorization follows.

## Immutable branch movement

The newest visible product/referee tips are:

| desk/ref | tip | observation |
| --- | --- | --- |
| Furiosa exact-base sidecar adaptation | `0cd00e44c7eb87e30fcf72f8ae790e7060635b09` | sidecar cleanup moved outside source-sidecar guards; parent `878f631b74e68aa76302f382e28096dc3d60b545`; merge-base with audit base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` |
| Ozzy sidecar line | `c38f79acf9c9ae43ebd091a95f36837f43c0e423` | same ownership effect, parent `96aa522611fdcb78e281db31634144e40222de91`; merge-base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` |
| Han integrated overlay/publication | `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` | parent `4119698525e806025ec36d00e0c85a5b1b3574a7`; merge-base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` |
| Han overlay support | `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e` | parent `9a1b53c710eb409c6f346b5cd95bbdd7212dccf6`; CLI-only overlay |
| Furiosa detached publisher comparator | `8c8216e25e22496b2e3e919fce836be49d692e25` | parent `2ca75b9`; receipt gap remains open; no current-base adoption |
| integration ref | `a33ab023eae0ca324956a66cf17b7ffa5b16c39d` | exact-base descendant; only deletes report/test material and changes a benchmark; not an adoption receipt |

Evidence files and immutable SHA-256 receipts used: Tick44 Han
`4b9be9b6` / `e1820d69da9ef1a43446d5230b1e5cb167c5cb15f9fa4f7a7e4f733dd91c78b4`,
Tick44 Ozzy `650c043c` / `4a622aef8cbddc17196cd74cdddbcc08aed177bde32433e593520fc58dd65ad8`,
Tick44 score `28d4f964` / `cebe6b44f5db2d118a948ffa29b819d62924bd8e32dc4d0cc1d8fd9813a60a59`,
and Tick45 research `77a167b3` / `8ff11117604b7e3e726f69e71623b04c3cdb827f26b4b19bc2c4f58436ca76e1`.

## Live decisions (maximum five)

### 1. Sidecar pruning and the Direction Lock

**Base/candidates:** locked base `878f631b74e68aa76302f382e28096dc3d60b545`,
Ozzy `c38f79acf9c9ae43ebd091a95f36837f43c0e423`, Furiosa `0cd00e44c7eb87e30fcf72f8ae790e7060635b09`,
rejected siblings `a78b39b3d87c82a4f83878359afc98e2b8fde2d4`,
`96aa522611fdcb78e281db31634144e40222de91`, and
`a62ab05c0c23e63e7ed401c0b85aa52011b2e71a`.

**Evidence and verdict:** Tick31 lock referee (`0cc1c100`, report SHA
`73f8d4a7684e7fe051211de9ebf056d20964d7f696f824fa52b3e7438a6c2b4d`) and
Tick45 official Kubernetes owner-reference research (`77a167b3`, SHA above)
support ownership-scoped deletion and co-contributor preservation. The
technical direction remains **LOCKED, technical-only**. `c38f79a` is the
existing externally adopted event; `0cd00e4` is a same-effect adaptation.
No new score and no merge authorization are proven.

**Missing decisive evidence:** an independent adopter receipt on the exact
current base, with the locked selector
`TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor`,
full race output, and one-fault mutation reds. The Tick45 Kubernetes source is
corroboration, not SQLite predicate or writer-fence proof.

**Smallest prior-art question:** does an ownership-scoped, retryable cleanup
controller require a durable owner beyond RawClaw's transaction, or is the
existing transaction-plus-fence contract sufficient for every no-sidecar and
co-contributor case?

**Direction-Lock invalidation triggers, rechecked:** (1) recorded base or
candidate production payload changes; (2) a newer production patch supersedes
the locked SQL effect; (3) the locked selector has a reproducible mutation red;
(4) the recorded focused/full gate regresses; or (5) the immutable adopter
receipt is invalidated or superseded. None is proven by the Tick45 evidence.

### 2. Fencing, cancellation, watermark, and terminal publication state

**Base/candidates:** cancellation candidate `047a6def4f21c3279563a7aef2cc331b4ecb6b6d`
(parent exactly `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`), Han integrated candidate
`8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`,
and external nil-scope alternatives `d91870634ff42b11165811111442acab26244d39` and
`ef9f6ab31530bc689329e8058936473b6ae27601`.

**Evidence and verdict:** the immutable cancellation audit
`cc3e08848aeb58d5360c631e516d5ede0c89f831` (report SHA
`63574bd2e781f05c8af8f0b8266e187be3658e79c1fbb78279862f0810857cba`) killed
the `AcquireConsolidatedFence(ctx)` to `Background()` mutation, but
`BeginTx` and watermark `QueryRowContext` mutations survived because the test
cancelled while the fence was held. Han's Tick42 mutation report records the
same boundary. The current verdict is **fence cancellation narrowly confirmed;
transaction/watermark cancellation incomplete; no adoption**.

Tick45 SQLite atomic-commit research (`77a167b3`) sharpens the rule: canceled
work cannot publish a success watermark or terminal receipt before a successful
commit. It does not prove a context-bounded modernc busy wait.

**Missing decisive evidence:** phase-controlled tests that cancel during
transaction admission, maintenance SQL, and watermark publication; a
terminal record written only after the same successful commit; and a current-
base full race gate. The nil-scope no-fence alternatives are not current-base
and conflict with the observed snapshot/rename lost-write race.

**Smallest prior-art question:** can the actual modernc driver interrupt a
busy admission/maintenance phase and atomically suppress watermark/receipt
publication, without a new daemon or driver fork?

### 3. Detached publication: terminal receipt versus honest best effort

**Base/candidates:** receipt red proof `987c6a31186bb15615175c5198389aa0d31846f6`,
referee reproduction `0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f`, best-effort wording
`1e61f732fc3f9234dd9b94a55407d7419fe71b47`, and `3b641ce9582541a60a7b37c8456bedaa9d86d29c`.
The underlying detached publisher is `8c8216e25e22496b2e3e919fce836be49d692e25`.

**Evidence and verdict:** `Start` returning followed by `Process.Release` does
not establish child entry or completion. The immutable referee report
`FINDINGS_DETACHED_PUBLICATION_RECEIPT.md` has report SHA
`1ac2d6465221d444ae51daa95fee495c84642a33a2c384b14e4fcc1683a85540` and
records 20/20 immediate-kill runs with no child terminal bytes. The wording
commits are **honest contract clarifications, not a durable fix**. A durable
queue/retry owner or synchronous wait is still missing; no score or merge
authorization follows.

**Missing decisive evidence:** a durable owner whose pending/completed/failed
record survives the foreground and child, or an explicit product decision that
best-effort absence of a terminal line is acceptable. A startup log line alone
does not close the gap.

**Smallest prior-art question:** under the single-static-binary constraint, is
there a standard-library-only durable owner for eventual publication, or must
the interface remain explicitly best effort?

### 4. Authoritative overlay and candidate composition

**Base/candidates:** Han overlay `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e`,
fixture `d2315cbbdcbf2edf6e3f9f8b4821acf9c622afca`, integrated Han
`8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`, Furiosa sidecar `0cd00e44c7eb87e30fcf72f8ae790e7060635b09`,
and overlay deletion `fb99037cda7c4ca80b6f5294631e5e5c0acc71b6`.

**Evidence and verdict:** Han's overlay gives narrow committed-topic
visibility; `d2315cb` is fixture-only. Han's integrated candidate and Furiosa
sidecar adaptation conflict in `internal/index/consolidated.go` and its tests
under both application orders (immutable composite report
`1dd45346dc72a5abd0aab8871c4289f3dd1b2202`). `fb99037` removes redundant
overlay bookkeeping but does not prove all-scope instantaneous tag-write.
Verdict: **composition unresolved; alternatives are not an additive winner**.

**Missing decisive evidence:** one manually resolved current-base candidate,
with provenance/co-contributor/ordering tests, detached survival, cancellation
and watermark proofs, and focused plus full race gates. Do not sum line counts
from conflicting payloads.

**Smallest prior-art question:** what is the smallest deep interface that
preserves source ownership, authoritative reads, and snapshot publication
without layering two competing overlay mechanisms?

### 5. Candidate liveness and integration branch movement

**Base/candidates:** Ozzy TDir candidates `74d4ee9b1bfcece8a37d17eecb91dfe4ac71f300`
and `7dad56df1cdf235fe72e14618cb15a81ed965611`; Furiosa nil-scope
`d91870634ff42b11165811111442acab26244d39` / `ef9f6ab31530bc689329e8058936473b6ae27601`;
integration ref `a33ab023eae0ca324956a66cf17b7ffa5b16c39d`.

**Evidence and verdict:** Tick44 Ozzy report (`650c043c`, SHA above) confirms
the TDir author-before-lookup and source-refresh fixes only for their scoped
paths. The nil-scope no-fence alternatives have merge-base
`2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce`, not the audit base, and directly
reopen the proven lost-write race. The integration ref is an exact-base
benchmark/report cleanup (`+0` production) rather than a product adoption.
Verdict: **scoped liveness remains a research candidate; no universal instant
tag-write claim and no integration receipt**.

**Missing decisive evidence:** current-base transplant with non-zero latency
and semantic work oracles, plus fence-held mutation proof for retained-history
consolidated writes. A no-tests-to-run pass is not evidence.

**Smallest prior-art question:** can source refresh be bounded by a durable
watermark/index without skipping required transcript ownership or weakening the
consolidated fence?

## Stable patch identity and duplicate ruling

All IDs below were recomputed from each commit's direct parent with
`git patch-id --stable`; path IDs are per changed product file. `range-diff`
was run on direct one-commit ranges where ancestry could fabricate novelty.

| commit (parent) | whole stable ID | path stable IDs | ruling |
| --- | --- | --- | --- |
| `c38f79a` (`96aa522`) | `6a62ff59b1b20a5873006b17ce72cd64229f65a6` | `consolidated.go` `41b270da6a33147a5e89f959cf14cb2441128ddb`; test `29b08e7f467ff6dd147771bdfe5d0240e18cd8ca` | distinct bytes, canonical no-sidecar correction; same sidecar mechanism |
| `0cd00e4` (`878f631`) | `57bdcd672364438b3b898f35d6f60c7cc178f5ca` | Go `ab5ee7d69f18a12786a85166f6dec53c32caedd6`; test `ac5ee690834ba263b5e03dcfdf17473f8fae07f4` | distinct adaptation, same effect family; no second score |
| `a78b39b` (`9068aff`) | `b47c5d83ef7a9a57b42f8a20f47c19f9ec4eb821` | Go `ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab`; test `084fa2c3e8add730074b0ee74296e2545d12fb75` | rejected duplicate |
| `96aa522` (`fb99037`) | `d54fa75907a2cb2b5bb823d101fe3d385ac6c775` | Go `ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab`; test `2922ec9900d50805aed3a79750170794ca890aca` | rejected duplicate |
| `a62ab05` (`9068aff`) | `b0c65fc6cfa0b3938d4a27b80108e609ba24fab5` | Go `ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab`; test `0499a406c9d3f06ce065ab7e5021af652ae83c0a` | rejected duplicate |
| `8e9c9b7` (`4119698`) | `4aef91de56b2e0c4756103ebedeae821f1570dec` | Go `044d7551d753396dd5709300988181b40dd20d0c`; test `46a21c8d7f5ea757a0f06fea5569676de385ffdf` | Han integrated family; conflicts with `0cd00e4` |
| `cabab43` (`9a1b53c`) | `72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6` | `tagrefresh.go` same | narrow overlay support, not sidecar duplicate |
| `d2315cb` (`7edd58d`) | `17db9874f86317dda02a64327fc584d35b0318e2` | `tagrefresh_test.go` same | fixture-only, no product claim |
| `fb99037` (`857dc62`) | `172b017850112fba5c5a4d9d1a8e735c964789a2` | `tagrefresh.go` `a2159cae0b8552e74211c5b1258d07d1e150cd9e`; test `751565c9448cdcd2194988ebf4006c8bb4f6bb03` | overlay bookkeeping deletion; not all-scope liveness |
| `5eb3a38` (`b0126f2`) | `73f5dd69a25ee9f6e39bcd2036397b46661d741b` | `tagrefresh.go` same | ancestry/semantic overlap with overlay deletion; not additive |
| `1e61f73` (`8c8216e`) | `773fd509dfdc771f0659656221a1fe459d614d41` | cmd `69033245a257e890789ff6d80cf39a17fe1e3c60`; test `0e3ca3c43c858f8e8db96e8914ae632e9f4211e1` | best-effort wording predecessor |
| `3b641ce` (`b2280fc`) | `b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc` | cmd `f50ccbe88fa29f9e0d88ce98c42e8944e4a9a545`; test `bdc8e8f91f1d50f0e67c1ac6703cafb0f429b686` | same wording family, adds help detail; no durable fix |
| `d918706` (`ea2b28a`) | `5133121d630d549c255f82606b13c1012c6c748f` | cmd `320432f703ca4d17b832904734857a15a9f85c4a`; test `1f2fe0255f4306856d2cf218378e46ec82731745` | no-fence liveness alternative; stale base and safety conflict |
| `ef9f6ab` (`f80ccd0`) | `086cb5a32567201d650ddee0405b3a60d7372803` | cmd `89e33d5538630a4a13c537c46c915a4dd8541e97`; test `e8fc39b5b45d5c4e84b2be9787272a6b1c288c87` | same nil-scope family plus report; not current-base |
| `a33ab02` (`0d1da19`) | `bf55b763e3d78f03ca1b0c1b50d029a535d685e4` | `connect_bench_test.go` `82e142f3630e29de6ffcf0182f05eba2050357ea` | integration benchmark cleanup, no product adoption |

Range-diff results: `96aa522^..c38f79a` versus `878f631^..0cd00e4` has no
patch mapping; direct payloads are different but semantically one family.
`fb99037^..fb99037` versus `b0126f2^..5eb3a38` has no mapping and both delete
overlay bookkeeping in different ancestry; count one conceptual family.
`8c8216e^..1e61f73` versus `b2280fc^..3b641ce` maps only the wording commit
with a changed help sentence; it is not a second receipt fix. The `96aa522`
commit maps exactly as an ancestor in the `fb99037^..c38f79a` comparison;
ancestry is not new novelty.

## Final disposition

The live problem set is five unresolved decisions above. Sidecar pruning is a
technical Direction Lock only. Official Go, SQLite, and Kubernetes sources
sharpen existing invariants but create no new stable ID. Current visible totals
remain unchanged by this report; no candidate is authorized for merge.

Report-only completion contract: no Go files changed; `gofmt -w` is N/A;
`git diff --check` is the report gate. This file is the only authorized edit.
