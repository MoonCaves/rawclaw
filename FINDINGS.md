# Furiosa Tick 28 post-231415Z patch-identity referee

## Scope and verdict

Cutoff: `20260826T231415Z` (2026-08-27 07:14:15 WITA). The requested
post-cutoff census found no Han or Ozzy branch ref movement. The newest Han/Ozzy
ref timestamps visible in this checkout are earlier than the cutoff:

- Han: `origin/han/luna-ozzy-claim-spy-20260827` at `04:18:04 WITA`,
  `daaf973dd30746bf8bfa65bf615c182fe25b1cd5`.
- Ozzy: `origin/ozzy/composite-instant-tagwrite-20260827` at `06:28:19 WITA`,
  `bc8af914d7d5736a8155929e0d81c998a4be5efc`.

The post-cutoff refs are audit workers, not Han/Ozzy product tips:
`worker/furiosa-t25-mechanisms-20260827@ab885674699e235cb6e9c9eaa5209e0b4ce0775b`
at 07:23:44, `worker/furiosa-t26-speed-audit-20260827@c6ad369ce16bf75f48a717a6fa41b3d12e1fe833`
at 07:34:27, `worker/furiosa-t26-filter-mutation-20260827@ee86f33dd6646dee80fe28544cd066cde7830a3b`
at 07:34:29, and `worker/furiosa-t26-selector-mutation-20260827@10c1b13c5650e5d20f532f32ae9316b7d2b1676d`
at 07:36:49. Their payloads are `FINDINGS.md` only.

**Strongest verdict: NO SCORE CLAIM.** No post-cutoff Han/Ozzy ref contains a
new score-eligible production payload, and no external adoption/rebuttal receipt
for a new post-cutoff payload was established.

## Claimed or messaged SHAs after the cutoff

The complete post-cutoff commit log in this checkout is:

| SHA | time WITA | payload | ruling |
|---|---:|---|---|
| `9aabff6996ae60207d806b6fb5aec6e29eb9fd24` | 07:23:05 | `FINDINGS.md` external-mechanism report | NO SCORE CLAIM |
| `ab885674699e235cb6e9c9eaa5209e0b4ce0775b` | 07:23:44 | `FINDINGS.md` fingerprint clarification | NO SCORE CLAIM |
| `2a0f386223e3401ee32245e20b1d7705b638bf46` | 07:24:12 | `FINDINGS.md` census report | NO SCORE CLAIM |
| `6eb9eef8a94af1d0d8fd90f3574e65b47ef4bcda` | 07:24:28 | `FINDINGS.md` prior-art regrade | NO SCORE CLAIM |
| `f4b709bfbad86ba431570334c77766aca0e89298` | 07:26:59 | `FINDINGS.md` census refinement | NO SCORE CLAIM |
| `eb4e9e926e508415ee6ab52b4076e40d16253eec` | 07:31:49 | `FINDINGS.md` speed-audit start | NO SCORE CLAIM |
| `7cce076e886e4e8a1c3bdef99f04a3601dd390f6` | 07:32:06 | `FINDINGS.md` mutation-audit start | NO SCORE CLAIM |
| `c6ad369ce16bf75f48a717a6fa41b3d12e1fe833` | 07:34:27 | `FINDINGS.md` hold on claimed Ozzy `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` | UNCERTAIN claim, no score |
| `ee86f33dd6646dee80fe28544cd066cde7830a3b` | 07:34:29 | `FINDINGS.md` focused-filter audit | NO SCORE CLAIM |
| `10c1b13c5650e5d20f532f32ae9316b7d2b1676d` | 07:36:49 | `FINDINGS.md` selector mutation audit | NO SCORE CLAIM |

`386ec9d` is not a post-cutoff branch movement: the audit identifies it as the
older Ozzy speed-prune implementation, parent `0d1da19c`, with whole stable
patch-id `356c1cb3878d142f910494843358b2737554dace` and
`internal/index/consolidated.go` path patch-id
`6b42e87e9d75eccc8a5527faa6c001653c15be82`. Its later report supplies no fair
baseline or raw comparison, so it remains **UNCERTAIN**, not score eligible.

## Locked candidate identity

The specified Direction Lock objects remain unchanged and predate the cutoff:

- base `878f631b74e68aa76302f382e28096dc3d60b545`, whole patch-id
  `b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc`;
- source winner `c38f79acf9c9ae43ebd091a95f36837f43c0e423`, whole/path IDs
  `6a62ff59b1b20a5873006b17ce72cd64229f65a6` /
  `41b270da6a33147a5e89f959cf14cb2441128ddb`;
- current-base adaptation `0cd00e44c7eb87e30fcf72f8ae790e7060635b09`,
  direct parent `878f631`, whole/path IDs
  `57bdcd672364438b3b898f35d6f60c7cc178f5ca` /
  `ab5ee7d69f18a12786a85166f6dec53c32caedd6`;
- loser `a78b39b3d87c82a4f83878359afc98e2b8fde2d4`, whole/path IDs
  `b47c5d83ef7a9a57b42f8a20f47c19f9ec4eb821` /
  `ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab`.

The post-cutoff commits have no `internal/index` payload and cannot alter these
patch identities, ancestry, applicability, or test-list evidence. Therefore
there is no post-cutoff novelty to score.

No product files were changed by this referee.
