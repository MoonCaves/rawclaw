# Tick 37 cumulative prior-art regrade

Run completion: `2026-08-27T01:17:42Z` (captured with `date -u`; WITA `09:17:42`). Audit base: `ef2eebf414e77086be06281539c5a50ba036a32a`. Authoritative prior watermark: `20260827T010052Z` from the shared append-only ledger. The forbidden Furiosa parent mailbox was not read, modified, acknowledged, or advanced.

## Evidence boundary

Mandatory ponytail, modular-refactor, codebase-design, and Go skills were read before inspection. Mnemon recall preceded Graphify. Graphify ran against canonical `/Users/jay-m4/code/rawclaw/graphify-out/graph.json`: `reflect --if-stale`, literal query `score referee claim receipt patch ancestry chronology`, `explain`, and `path`; source and immutable Git evidence were then checked. Shared ledger read: `/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/PRIOR_ART_LOG.md`, whose post-Tick-35 append records all three Tick-34 mutations and no adoption. Pre-append ledger SHA-256: `fa4941faf71dbc4dded6b4f9b544ab55417852e6368b6b27a3a5ef885e36bb51`.

The only post-watermark rival receipts found are Tick-36 Han/Ozzy claim-spy reports and corrections. They are report-only, control, or challenge evidence; none supplies an independent adopter, immutable adoption receipt, or score event. Their relevant commits are Han `620455f213d3848f31212dc4f4fbf487284fcb9a` plus correction `c095126a35be41d35c933af4b7d16aba0ec8cd39`, and Ozzy `b1ec0e3a6c5acefe3bbfb0d6567cb1d3a8660e7b` plus correction `78971f4627e79c297d86c276269ef9fbcd0ff0d7`; all are audit documents, not adopted product payloads.

## Recommendation regrade

| Recommendation | Fingerprint | Status at Tick 37 | Score event | Evidence / duplicate boundary |
|---|---|---|---:|---|
| `PA-CONSOLIDATED-SIDECAR-PRUNE-001` | `d07f69f8d056f9f145bd9a864e3fa11660afadf13af3aca9acad39ea722bcb72` | `externally_adopted`, locked | 0 new (`Ozzy +3` already counted) | Immutable adoption receipt SHA `fb8147aa46baf4668699e6c610e8b6f60c4ddafa87abdd9b3a0d5d27c3016316`; source `c38f79a...`, adaptation `0cd00e4...`. Tick-36 correction reaffirms the existing Ozzy +3. Furiosa’s same-effect `0cd` is not a second event. |
| `PA-SQLITE-BEGIN-IMMEDIATE-001` | `7e0c7cf321c3b845baf8518173f636fdf86eda2d81b69cd8b00b635cff80b214` | `narrowed`, unadopted | 0 | Mutation `47b1c2b9...` moved contention to Begin but 10s busy timeout still delayed a 200ms context about 10.207s. Not context-bounded admission; no adopter receipt. |
| `PA-FTS5-DELETEMERGE-001` | `21ae4bb81dff3c0531f08b62290258442aaea11b75949aeb2e7bca2996e240a2` | `pending`, unadopted | 0 | No product payload or immutable adopter after watermark. Distinct from sidecar deletion and WAL checkpoint scheduling. |
| `PA-GO-SINGLEFLIGHT-FALLBACK-001` | `1532e53cf1b582d958f6fec89bcb723cf2da7681bc696a5b7cfbc0fe4bf3465a` | `pending`, unadopted | 0 | No adoption. In-process result coalescing remains distinct from durable ownership/fencing. |
| `PA-SQLITE-WAL-IDLE-CHECKPOINT-001` (alias `PA-SQLITE-WAL-PASSIVE-CHECKPOINT-001`) | `efe829449d40df34663d26fa33920d98f25d65fac768b66bc2e1218d8c201e91` | `pending`, alias duplicate rejected | 0 | Tick-30 ledger preserves the first ID; alias fingerprint `86a2faf...` is duplicate. No Ozzy response or adoption to Tick-33 challenge. |
| `PA-GO-WEIGHTED-SEMAPHORE-WRITER-001` | `3be536e7d5aa2e34267b8b0b334b81165311f124ce38d5bfd45ac57676593c40` | `rejected` | 0 | Dependency-specific weight-one machinery is unnecessary; stdlib token/select preserves the stated contract. No product change. |
| `PA-SQLITE-PROGRESS-BUDGET-001` | `6d296d6f799c6da5b26f79bd3ad51327a018d7bf11ca8324817b7b8c7753e42b` | `narrowed`, unadopted | 0 | modernc v1.45.0 has context interruption but no supported progress-handler API. Mutation report `bd653930...`, SHA `92f5d180...`; no custom fork or adopter. |
| `PA-SQLITE-BUSY-TIMEOUT-001` | `69634beea1d95e0696cb1f451e95a60df291d4487c6658ea0d231b25a9d5b841` | `duplicate` | 0 | RawClaw already uses `_pragma=busy_timeout(5000)` for RO and `(10000)` for RW. Not a new event; distinct from fence/context deadlines. |
| `PA-SEMANTIC-BENCH-COUNTER-001` | `c0bb59011b65af9866dccc35ba701b834ec6daebfaed3ecb95a1fcfc1d83d11c` | `confirmed validity rule`, unadopted | 0 | `B.ReportMetric` alone gives a green zero-work benchmark; explicit assertion is mandatory. Mutation report `bd653930...`; report-only, no adoption. |
| Ozzy `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` batch-prune speed claim | implementation family, no prior-art fingerprint | `rebutted/uncertain`, unadopted | 0 | Tick-36 correction confirms six samples but no fair old baseline, benchstat, semantic work guard, or independent adoption. Functional pruning is narrow-confirmed; performance claim remains unsupported. |

Other earlier terminal/publication recommendations remain at their prior `pending`/`partial`/`blocked` statuses; no post-watermark evidence changes them. In particular, report-only Han overlay/publisher branches (`cabab43`, `d2315cb`, `8e9c9b7`) and attack branches (`0400fdb`, `f2e20d1`, `11bb894`, `ef2fdb7`) have no adopter receipt and score zero. A different commit SHA, inherited ancestry, scheduler mail, self-adoption proposal, or outbound challenge does not create a novel score event.

## Chronology and duplicate rulings

- The only counted adoption remains Ozzy’s sidecar-prune receipt (`fb8147aa...`) and score `+3`; the shared totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`.
- Tick-36 Ozzy’s initial wording that c38 had no score was corrected by `78971f4...`; that correction restores the already-counted +3 and adds no Tick-36/Tick-37 score.
- Tick-36 Han’s `cabab43`, `d2315cb`, and `8e9c9b7` payloads are narrow behavior evidence, not external adoption. Their tests do not establish all process-exit, cancellation, deletion, or terminal-receipt layers.
- Tick-35 mutation reports are evidence-only: semantic `c0c8bcf...` (report SHA `5b4e4dbf...`), BEGIN `47b1c2b...` (report SHA `c0450124...`), and applicability `bd653930...` (report SHA `92f5d180...`). No score is awarded for self-audit.

## Watermark and next leads

`new_watermark` remains `20260827T010052Z`: no newer conforming processed Furiosa inbox receipt can be proven without reading the forbidden parent mailbox, and rival/outbound mail does not advance that cursor. No Direction Lock changes; the sidecar lock remains technical-only with no merge authorization.

Next challenge: require a fresh current-base adopter with one stable recommendation fingerprint, one immutable receipt SHA/path, exact patch-id and ancestry proof, a semantic mutation that fails the claimed invariant, and personally observed focused/full gates. For BEGIN IMMEDIATE, require a busy policy that actually bounds a live context; for 386, require paired old/new samples, benchstat, and a non-zero work assertion; for Han overlay/publisher work, combine process-death, cancellation, deletion, and co-contributor cases.

Validation: only this report is intended to be committed. No product, rival/shared harness, scorecard, rotation log, mailbox, cursor, or graph state was modified.
