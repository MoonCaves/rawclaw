# Ozzy harvest raider report

## Verdict

ADOPT the unique prior-art ledger as research input, in chronological order after the
already harvested seed documents. REJECT every Ozzy product-code transplant from the
harvest tip: each is already adopted elsewhere, duplicate in mechanism, or unsafe to
carry from a stale lineage. The only integration-order change is to require the prior-art
ledger's composite catalog-key and same-volume staging checks before any corresponding
product change is accepted.

## Scope and immutable refs

- Fixed base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
- `ozzy/harvest-wave1-20260826`: `78b6a4fe5a90771d9de7a1e3e83e0c046ed834a8`.
- `ozzy/prior-art-20260826`: `a829b1b22aacc5ed64b5624d112e10eb27f67df3`.
- Harvest merge-base: `5b9756b2200ff6bd670f07407407d84d9f42d84b`.
- Prior-art merge-base: `86c5ce06b789e9e287154ba25acc699d04ff2c7b`.
- Both Ozzy tips were present locally and matched `origin` at inspection.

The prior-art tip is behind the fixed base in product history: its apparent `30-file`
code deletion (`+1668/-1883` versus the fixed base) is divergence from the fixed base,
not a new Ozzy payload. The post-merge-base commits are documentation only.

## Graphify-first evidence

The mandatory first source orientation was:

`mcp__graphify__query_graph({question:"harvest Ozzy", mode:"bfs", depth:2, token_budget:2000})`

Outcome: `No matching nodes found.`

Follow-up calls:

- `mcp__graphify__get_node({label:"resolveSegmentRange"})`: no matching node; the
  graph is stale for the newer range-resolver symbol.
- `mcp__graphify__get_node({label:"addRawclawAntigravityHooks"})`: found the setup
  seam and its source location.
- `mcp__graphify__get_neighbors({label:"addRawclawAntigravityHooks", relation_filter:"call"})`:
  confirmed removal and installation callers plus two idempotence tests.
- `mcp__graphify__query_graph({question:"catalog hook", mode:"bfs", depth:2, token_budget:3000})`:
  found the catalog/hook tests, `renderHookScript`, `CatalogDir`, and special-path
  safety coverage. This explains why the stale-lineage deletion of those tests is not
  transplantable.

Graphify framed the hook seam and stale-graph limitation; exact payload and novelty were
then checked with commit diffs, `range-diff`, and stable patch IDs.

## Product-code candidates

| commit | payload and evidence | ruling |
|---|---|---|
| `847426c` | Removes an impossible error return from Antigravity hook registration; `+5/-14`. Stable patch ID `e6322da4ca5faaa5b3b596fdbb33409bf376a4e5`, also present as Norm's `fa485c8`. | **REJECT — duplicate.** |
| `539de03` | Deletes nine informational assertions from the abrupt-exit retry test. Stable patch ID `7addd4ca88dd31164e993883d4b57a4852e8e5b8`; the wave ledger records this as already accepted from Norm. | **REJECT — duplicate cleanup.** |
| `b944d08` | Extracts shared `resolveSegmentRange`; `+50/-65`. Stable patch ID `0c8b28032a1f8baf7a6a076ac6205e47d753f476`, identical to Lenny's `b2ff61c`. | **REJECT — duplicate.** |
| `37ec96b` | Flat-ID validation, PID-only temporary namespace, and hostile shell/path tests; `+217/-28`. Its exact stable patch differs from later `bd8346c`, but the mechanism is the same path-safe catalog-claim implementation already landed by another desk. | **REJECT — inherited mechanism; no novelty credit.** |
| `78b6a4f` | Removes unreachable range clamps; `+1/-7`. Stable patch ID `cea8cc66c09632db4cd9980063e2e69a3646260c`, identical to Conor's `fb893ed` and Norm's `a317766`. | **REJECT — duplicate.** |
| prior-tip code diff | Deletes catalog, prewarm, tag-refresh, consolidation, and container tests/implementations because the branch diverged before the fixed base. | **REJECT — stale-lineage deletion; would erase current-base contracts.** |

## Unique prior-art candidates and ordered adoption

The first four prior-art seed commits (`c653543`, `2367b58`, `86e3d52`, `041a153`)
are patch-equivalent to the harvest branch's seed documents (`range-diff` marks them
`=`), so they score zero as new work. The unique post-seed chain is:

1. **ADOPT as research ledger:** `7726b9b`, `6f998d8`, `da4667c`, `a9a3ddd`, and
   `b2c630d` establish adoption-gated scoring, exact-SHA/payload separation, and live
   problem re-inventory.
2. **ADOPT before hook/catalog integration:** `23761fb` and `00d1783` record the
   defended flat-ID/PID temporary namespace and the narrowed `resolveSegmentRange`
   contract. They are research receipts, not additional code.
3. **ADOPT before any catalog implementation:** `37a2012`, `6208d11`, `b3c8c8c`,
   and `c3867f5` require deterministic child-reaping test wrappers, mutation gates for
   test deletions, and composite provenance matching `(Source, Project, SessionID, CWD)`.
4. **ADOPT last:** `8dfa677` and `a829b1b` add same-directory temporary staging for
   atomic rename, RFC 3986-style composite lookup specificity, generative path-property
   tests, idempotent unlink under the writer fence, and native SQLite WAL recovery.

The smallest useful transplant is the unique documentation chain above, ordered by
dependency. Do not transplant its stale product tree. Any later product implementation
must pass the stated composite-key, same-volume staging, mutation, and writer-fence gates.

## Gates and limitations

- Personally observed: fixed-base identity, both tip identities, clean stable patch-ID
  comparisons for all five harvest code commits, and `range-diff` duplicate/inherited
  classification.
- Not personally observed: a fresh test run on either rival tip. Worker summaries and
  ledger claims are not treated as test evidence.
- Decisive gate for a future product transplant: apply only the minimal patch to the
  fixed base, run `CGO_ENABLED=0 go test -race -count=1 ./...`, verify `gofmt -l
  internal/` is empty, then run hostile path/special-file and mutation checks for the
  affected contract. A zero-match test filter is a failure, not a green result.
- `git diff --check` is required on this report commit.

## Final ruling

**ADOPT** the unique prior-art research chain in the four ordered groups above.
**REJECT_ALL product-code transplants from Ozzy's two tips**: the five harvest code
commits are duplicate/inherited mechanisms, and the prior-art tip's code delta is stale
lineage rather than a candidate payload.

