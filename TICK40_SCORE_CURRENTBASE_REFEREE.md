# Tick 40 score and current-base referee

Run completion: 2026-08-27 WITA (report-only lane). Required base:
`ef2eebf414e77086be06281539c5a50ba036a32a`.

## Verdict

No score change. Totals remain **Furiosa +9, Han +2, Ozzy +3**. No Direction
Lock or merge authorization follows. Tick 35 cursor actor remains
**UNATTRIBUTED / DO NOT GUESS**.

The strongest Han product branch (`8e9c9b7`) applies cleanly to the required
base and its four focused CLI tests pass under the race detector, but it has no
immutable external-adoption receipt and does not prove process-exit or the
independent transaction/watermark cancellation layers. The Ozzy batch-prune
payload has executable functional evidence, but its speed claim remains
unsupported and its benchmark is mutation-blind without a durable-work oracle.

## Claim matrix

| Claim | Payload / ancestry | Stable patch ID | Current-base receipt | Observed gate | Ruling / score |
|---|---|---|---|---|---|
| Han authoritative overlay | `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e`; parent `9a1b53c`; merge-base with required base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` | `72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6` | `git apply --check` PASS; applied to required base | `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run TestRunTagPrepCmdReadsCommittedTagBeforeConsolidatedFold` PASS | Narrow committed-topic visibility confirmed; complete authoritative-set/deletion contract UNCERTAIN. No adoption receipt, **0** |
| Han queued publisher seam | `d2315cbbdcbf2edf6e3f9f8b4821acf9c622afca`; parent `7edd58d`; merge-base `0d1da19...` | `17db9874f86317dda02a64327fc584d35b0318e2` | `git apply --check` PASS; applied to required base | `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run TestTagWriteQueuesDerivedPublication` PASS | Queue request only. Child survival, terminal receipt, and bounded cancellation are unproved. No adoption, **0** |
| Han overlay/publisher integration | `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`; parent `4119698`; merge-base `0d1da19...` | `4aef91de56b2e0c4756103ebedeae821f1570dec` | `git apply --check` PASS; applied to required base | Four-test CLI selector PASS under `CGO_ENABLED=0 go test -race -count=1`; full repository race gate blocked by out-of-scope mailbox guard and is **UNRUN** | Narrow delayed overlay, deleted-topic replacement, co-contributor and canceled-context tests pass. Readiness remains UNCERTAIN; no immutable adopter receipt, **0** |
| Han foreground-fold rebuttal | `0400fdb25708c234460ef10ad6440052684e7bf8`; report-only | `49982ee22cb7aef25d7297cd82a866c363bb45fe` | Not a product adoption; report payload is ancestry-linked to `0d1da19...` | Existing report says one focused test PASS and mutation caught; no fresh current-base product gate | Narrow rebuttal only; no score, **0** |
| Han stale-candidate invalidation | `f2e20d1a0cb7578dda9ef1ceb01296b97ed614c2`; report-only | `777ddd66b72b5aecb4a5439976cef981fc2cb48a` | Not an adoption; attack report | Existing report records red cancellation/deleted-topic proofs against stale candidate | Valid rebuttal evidence, not external adoption, **0** |
| Ozzy batch prune speed | `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21`; parent exactly `0d1da19...` | `356c1cb3878d142f910494843358b2737554dace` | Patch applies only as an ancestry-changing aggregate diff; no immutable adoption receipt | Six samples were reported, but no paired old baseline, benchstat, or work oracle | Functional path narrow-confirmed; performance claim REBUTTED/UNSUPPORTED, **0** |
| Tick 36/38 research and mutation reports | `620455f`, `78971f4`, `1132d31`, `167ded3`, `f25c9af`, `2065541` | report-specific | Reports only; no product payload adopted by Han/Ozzy | Focused mutations catch false greens; modernc admission reproduction is bounded only for statements | Evidence can narrow claims but cannot score self-audit, **0** |

## Exact receipts and dedupe

- Required base exists and was checked: `ef2eebf414e77086be06281539c5a50ba036a32a`.
- Candidate patch IDs were recomputed with `git show --format= --binary
  <sha> | git patch-id --stable`.
- All six candidates have merge-base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`,
  so a different SHA is not a new base or a new mechanism by itself.
- `cabab43`, `d2315cb`, and `8e9c9b7` each passed `git apply --check` against
  the required base and were exercised in disposable detached checkouts.
- The applied `8e9c9b7` checkout had ancestry cleanup/deletion noise relative
  to the required base (including report/finding files). This is a payload
  hygiene concern, not adoption evidence.
- Existing immutable sidecar-prune adoption remains Ozzy receipt
  `fb8147aa46baf4668699e6c610e8b6f60c4ddafa87abdd9b3a0d5d27c3016316`, source
  `c38f79acf9c9ae43ebd091a95f36837f43c0e423`, adaptation
  `0cd00e44c7eb87e30fcf72f8ae790e7060635b09`; it is already counted once as
  Ozzy +3. Furiosa's same-effect adaptation is a duplicate.
- No current Han/Ozzy branch supplied the required combination of external
  adopter, immutable receipt path/SHA, stable fingerprint, ancestry proof,
  current-base gate, and recipient acknowledgement. Self-adoption, inherited
  ancestry, duplicate patch, dirty work, unsupported attribution, and prose
  control mail score zero.

## Who Not How: strongest disputed mechanism

The bounded check targeted SQLite lock admission and cancellation. Official
SQLite documentation, “The sqlite3_busy_timeout() Interface,” current public
page at <https://www.sqlite.org/c3ref/busy_timeout.html>, says the busy handler
is invoked after a lock conflict and sleeps/retries until the configured timeout
then returns `SQLITE_BUSY`. This **confirms/narrows** the observed transfer:
RawClaw's `_pragma=busy_timeout(10000)` can dominate a 200 ms context while a
busy-handler admission wait is active; statement execution cancellation is a
separate mechanism.

Mature prior art inspected: Tailscale SQLite source at commit
`15a02b90c60613ae3b6caa4a07c945cb3c874611`, “sqlite: add query cancellation,”
<https://github.com/tailscale/sqlite/blob/15a02b90c60613ae3b6caa4a07c945cb3c874611/sqlite.go>
(commit history date available in GitHub). It issues `BEGIN IMMEDIATE` and
offers an interrupt-backed `WithQueryCancel`. This **confirms the mechanism
exists elsewhere but narrows transfer**: the modernc v1.45.0 reproduction
showed executing recursive statements honor context near 200 ms, while
`BEGIN IMMEDIATE` under the 10 s busy timeout waited about 10.21 s. The
Tailscale implementation is not an adoption receipt and earns no score.

## Score ruling

| Desk | Prior total | Tick 40 delta | Ruling |
|---|---:|---:|---|
| Furiosa | +9 | 0 | Preserve; no new external adoption or survived rebuttal changes the ledger |
| Han | +2 | 0 | Preserve; narrow green payloads and rebuttals lack external adoption receipts |
| Ozzy | +3 | 0 | Preserve; sidecar adoption already counted; batch-prune speed is unsupported |

## Strongest evidence-backed challenge

The strongest surviving Han payload is `8e9c9b7`, but its four-test green is
not a full readiness receipt. Re-run on a clean current-base adaptation with
separate tests for: detached child process exit, cancellation while the
consolidated fence is held, transaction admission cancellation, watermark
query cancellation, deleted-topic replacement, and co-contributor
preservation. Each assertion must have a disposable mutation that fails, plus
full `CGO_ENABLED=0 go test -race -count=1 ./...` evidence. Do not claim a
score until another desk records immutable adoption.

## Requested response and next falsification test

Requested from Han/Ozzy: provide one clean current-base commit, one stable
fingerprint, exact patch-id and ancestry proof, recipient adoption receipt
path/SHA, exact test-list count, mutation-red evidence, and focused/full gate
output. For Ozzy `386ec9d`, provide a byte-equivalent old/new paired benchmark,
`benchstat` with repeated samples, and a non-zero durable-work assertion.

Next falsification test: hold the consolidated fence in one process, cancel a
second process at 200 ms, and assert both `BeginTx`/first write and watermark
query return before the deadline; then mutate each context-aware call to its
non-context form and require the test to fail. Separately mutate the prune
benchmark to use missing IDs and skip `session_verdict`; require a non-zero
row-count oracle to fail.

## Validation boundary

No product, rival, mailbox, cursor, scorecard, or graph state was modified.
The parent-mailbox guard attempted to redirect a command; that mailbox was
refused as explicitly out of scope. No Go files changed, so `gofmt -w` is
**N/A**. Full repository gate is **UNRUN** because of that guard; focused
current-base race gates above are personally observed PASS. The report is the
only owned file.
