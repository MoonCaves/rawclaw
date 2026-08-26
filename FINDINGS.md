# Tick 22 c38/current-base score referee

Audit base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.

## Evidence identity

- The adoption receipt is present at `/Users/jay-m4/code/rawclaw/.agent-mailbox-ozzy/20260826T224735Z-5b061dd1-external-adoption-c38-current-.md` with SHA-256 `fb8147aa46baf4668699e6c610e8b6f60c4ddafa87abdd9b3a0d5d27c3016316`.
- It names `0cd00e44c7eb87e30fcf72f8ae790e7060635b09`, parent `878f631b74e68aa76302f382e28096dc3d60b545`, and adaptation patch ID `57bdcd672364438b3b898f35d6f60c7cc178f5ca`.
- The source candidate is `c38f79acf9c9ae43ebd091a95f36837f43c0e423`, parent `96aa522611fdcb78e281db31634144e40222de91`, patch ID `6a62ff59b1b20a5873006b17ce72cd64229f65a6`, on immutable branch `ozzy/fresh-luna-adversarial-20260827`.
- The c38 commit is timestamped 2026-08-27 05:38:03 WITA; the 0cd adaptation is timestamped 06:40:25 WITA. The earlier Ozzy branch candidate `96aa522611fdcb78e281db31634144e40222de91` is timestamped 05:29:06 WITA.

## 1. Attribution and chronology — CONFIRMED for Ozzy

The candidate branch alone would not prove a recommendation. The immutable prior receipt
`/Users/jay-m4/code/rawclaw/.agent-mailbox-ozzy/20260826T223543Z-5c485cbf-tick-21-c38-hostile-contract-c.md`
does: before 0cd existed, it names c38 as the existing solver, records the exact missing-sidecar-table
contract, and says the mechanism is being transplanted onto current-base `878f631`. The c38 object is
also on `ozzy/fresh-luna-adversarial-20260827`, while the later 0cd object is on
`worker/furiosa-final-current-base-20260827`. This is prior recommendation/implementation evidence,
not merely a candidate found in an unrelated directory.

## 2. Semantic adoption — CONFIRMED

The c38 hunk moves the `topic_segment` and `session_verdict` orphan deletes outside the
`hasTopics`/`hasVerdicts` guards, so pruning still runs when the source has no sidecar tables. The 0cd
hunk adds those same unconditional deletes to current-base `878f631`; its regression also checks an
orphan is deleted while a co-contributor's sidecar remains. Stable patch IDs differ (`6a62ff...` versus
`57bdcd...`) because the commits have different parents and test payloads, but the normalized
production behavior is the same. The changed test is a broader current-base adaptation, not unrelated
convergence.

## 3. No-double-count ruling — CONFIRMED duplicate

`TEN_MINUTE_ROTATION.md:90` says to update the scorecard “without double-counting the same patch or
recommendation.” The Ozzy +3 and Furiosa +2 are one causal event: Furiosa's 0cd transplant is the
implementation/adoption of Ozzy's c38 recommendation. The distinct authors and stable patch IDs do not
create a second recommendation. The scorecard's earlier overlay precedent likewise says one event does
not receive both the default +3 and unique-transplant +2.

Therefore the recorded pair `Ozzy +3` and `Furiosa +2` cannot both stand. Conservatively retain the
charter's externally adopted recommendation award: **Ozzy +3; Furiosa +0 for this event**. Do not add a
second +2. This is a score correction, not merge authorization.

## 4. Gates — personally observed versus inherited

Personally observed:

- On 0cd: `CGO_ENABLED=0 go test -race -count=1 -run TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor ./internal/index` — PASS, `1.793s`.
- On c38: `CGO_ENABLED=0 go test -race -count=1 -run TestConsolidate_PrunesExistingSidecarsWhenSourceHasNoSidecarTables ./internal/index` — PASS, `2.159s`.
- On a78: the same selector command — FAIL; both sidecar counts remained `1`, so the selector mutation is genuinely red on a78 and green on c38.

Inherited only (not claimed as my execution): the receipt's serialized
`CGO_ENABLED=0 go test -p 1 -race -count=1 ./internal/cli ./internal/index` PASS in 132s and its
reported clean/pushed state. The immutable commits and branches were inspected; I did not alter them.

## 5. Direction Lock schema — INCOMPLETE

The later rotation text contains a recommendation ID, fingerprint, base, winner, filter, adoption
receipt, and invalidation triggers. It is still incomplete under the schema: `candidate_shas_or_patch_ids`
does not enumerate all compared candidates (notably the a78 loser), and `gate_commands` does not give
both exact focused and full command strings. No Direction Lock should be treated as complete from this
record, and this referee grants no merge authorization.

## Final conservative delta

- Ozzy +3: **CONFIRMED**, externally adopted c38 recommendation.
- Furiosa +2: **REBUTTED as duplicate**, not an additional score event.
- Net Tick 22 delta for this mechanism: **Ozzy +3, Furiosa +0**.
- Missing evidence: a complete Direction Lock candidate list and exact focused/full gate-command field;
  the full-package gate remains inherited rather than referee-observed.
