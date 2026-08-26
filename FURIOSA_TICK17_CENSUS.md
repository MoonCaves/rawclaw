# Furiosa Tick 17 live-problem census

Date: 2026-08-27 WITA  
Base: `8c8216e25e22496b2e3e919fce836be49d692e25`  
Branch: `worker/furiosa-census-t17-20260827`  
Mode: report-only; the only worktree file added is this report.

## Method and evidence boundary

The required `golang-testing`, `golang-safety`, `golang-troubleshooting`, and
`graphify` skills were read completely before source or Git-blob inspection.
They changed this lane as follows: test claims require observable, named gates;
race/concurrency claims remain UNCERTAIN without an observed race gate; safety
review separates silent data-loss/corruption risks from harmless behavior; and
Graphify is orientation only, using literal vocabulary and freshness reflection.

`mnemon --store rawclaw recall` was run before this area. `graphify reflect
--if-stale` reported current lessons, and `graphify-out/reflections/LESSONS.md`
was read. Against the canonical graph at
`/Users/jay-m4/code/rawclaw/graphify-out/graph.json`, literal `explain`
(`ConsolidateFrom`), query (`overlay merge bookkeeping best effort sidecar
race`), and path (`ConsolidateFrom` -> `pruneTombstoned`) were used only to
orient the source seams. SHA, ancestry, patch identity, and gate status below
come from immutable Git evidence, not Graphify.

The score boundary is strict: another desk needs an immutable adopter receipt
and the same mechanism. Similar prose, self-adoption, inherited ancestry, a
dirty checkout, or an unobserved full race gate is not adoption proof.

## Desk heads and checkout state

| desk / head | immutable head and meaning |
| --- | --- |
| Han supervisor | `origin/supervisor/han-mechanism-20260827` -> `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`; no 5eb/3b product adoption in that head. |
| Ozzy latest observed | `origin/ozzy/luna-fresh-doc-referee-20260827` -> `12c02ae3deb4d7bdc9b53d9ed844244f37e37d03`; its ancestry includes the Ozzy sidecar line and contract docs, but no independent full-race receipt for 96aa522. |
| Furiosa supervisor evidence | `origin/supervisor/furiosa-evidence-20260827` -> `987c6a31186bb15615175c5198389aa0d31846f6`; the exact-base product candidates are recorded below on their own immutable refs. |
| This census | clean at `8c8216e25e22496b2e3e919fce836be49d692e25`; no upstream is configured for the worker branch. |

Remote refs are local tracking snapshots; no mailbox helper or cursor was
modified, and no product checkout was edited.

## Exact candidate identity

Patch IDs are stable IDs of each commit payload. Path-scoped IDs are shown when
the commit touches the corresponding path; an empty path has no payload there.
Numstat is direct-parent payload accounting, not a cumulative branch delta.

| candidate | parent / base relation | whole stable patch-id | path-scoped stable patch-id | numstat | ruling |
| --- | --- | --- | --- | --- | --- |
| `5eb3a38309a5319befaa434483bbf97004312129` | parent `b0126f2fe2427d638f0eed4e58114f5171920bf8`; `merge-base(base,candidate)=base` | `73f5dd69a25ee9f6e39bcd2036397b46661d741b` | `internal/cli`: `73f5dd69a25ee9f6e39bcd2036397b46661d741b` | `internal/cli/tagrefresh.go` `0/18` | Exact-base shrink: removes dead overlay bookkeeping. |
| `3b641ce9582541a60a7b37c8456bedaa9d86d29c` | parent `b2280fc0a2baaa89474730e0a9c22128dab10b4e`; `merge-base(base,candidate)=base` | `b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc` | `internal/cli`: same | `cmd_tag.go` `4/2`; `tagrefresh_test.go` `10/0` | Exact-base best-effort contract plus help and assertion. |
| `cb154d939612a7b1be078f1eede385553858a5de` | parent `69d8d0b59e8f89dde3c06ae8f72773ccea4aca00`; `merge-base(base,candidate)=0d1da19` | `773fd509dfdc771f0659656221a1fe459d614d41` | `internal/cli`: same | `cmd_tag.go` `2/1`; test `3/0` | Stale-base/subset wording follow-up; not a new exact-base candidate. |
| `96aa522611fdcb78e281db31634144e40222de91` | parent `fb99037cda7c4ca80b6f5294631e5e5c0acc71b6`; `merge-base(base,candidate)=0d1da19` | `d54fa75907a2cb2b5bb823d101fe3d385ac6c775` | `internal/index`: same | `consolidated.go` `20/0`; test `51/0` | Sidecar-pruning candidate; stale-base and no independent full-race receipt. |
| `d91870634ff42b11165811111442acab26244d39` | parent `ea2b28adae7c159e463b0472d1de4fe143515798`; `merge-base(base,candidate)=base` | `5133121d630d549c255f82606b13c1012c6c748f` | `internal/cli`: same | `cmd_tag.go` `0/15`; hostile test `8/25` | Removes the consolidated fence; conflicts with the recorded rebuild-safety invariant. Not integration-worthy. |
| `ef9f6ab31530bc689329e8058936473b6ae27601` | parent `f80ccd0b86a69a683092aecf1df32aa1f2d1b5ad`; `merge-base(base,candidate)=base` | `086cb5a32567201d650ddee0405b3a60d7372803` | `internal/cli`: same | report `6/7`; `cmd_tag.go` `4/17`; test `8/25` | Nil-scope v2; safety disposition rejects removing the fence without a mutation proof. |

The exact-base 5eb/3b stacks include Furiosa’s own preceding review/doc
commits. `git range-diff base^..5eb base^..3b` shows the two payloads are
unrelated: 5eb is the overlay deletion, while 3b is the best-effort wording
and test contract. `git range-diff` against Ozzy’s `fb99037` shows stale-lineage
divergence rather than a cherry-pick-equivalent patch.

## Live adoption and deduplication decisions

### (1) Has another desk independently adopted the `5eb3a38` shrink mechanism?

**NO SCORE CLAIM — convergence/prior implementation, not external adoption.**

Ozzy’s immutable `fb99037cda7c4ca80b6f5294631e5e5c0acc71b6` (on the Ozzy
lineage, parent `857dc62414426b540b57a497609122721982a367`) removes redundant
tag-topic overlay bookkeeping in `internal/cli/tagrefresh.go` and its obsolete
tests. It is the same shrink mechanism as 5eb3a38, but its whole/path patch-id
is `172b017850112fba5c5a4d9d1a8e735c964789a2`, not 5eb’s
`73f5dd69a25ee9f6e39bcd2036397b46661d741b`; its payload is `+3/-12` production
and `0/-16` test lines. Its merge-base is `0d1da19`, not the Tick 17 base, so
it is a prior implementation, not an adoption of Furiosa’s later recommendation.
The two changes must be deduplicated as one overlay-bookkeeping shrink family,
with zero external-adoption credit to Furiosa.

#### Chronology correction

The cited Ozzy receipt (`20260826T212425Z-03f95807-composite-fb99037-hard-boundar.md`)
is consistent with the immutable timestamps: Ozzy `fb99037` was authored at
`2026-08-27T05:07:29+08:00` and committed at `05:16:14+08:00`; Furiosa
`5eb3a38` was authored and committed at `05:27:22+08:00`. Ozzy therefore
predates Furiosa. Calling this “confirmed semantic adoption by Ozzy” was
incorrect and is withdrawn. The score-safe ruling is convergence/prior art,
same mechanism, deduplicate, zero score.

### (2) Has another desk independently adopted the `3b641ce` best-effort contract?

**NO SCORE CLAIM.**

`9068aff3c3a4b6cd1a51f0681ce7594e86018a07` and
`1e61f732fc3f9234dd9b94a55407d7419fe71b47` are immutable exact-base copies of
the wording subset, but both are Furiosa refs and therefore self-adoption.
`cb154d9` is also Furiosa and stale-base. Ozzy’s `967051f`/`12c02ae` are
design-contract documentation, not an independent product adoption of the
3b CLI help/output/test payload. Han’s current supervisor head does not carry
the mechanism. There is no adopter-plus-receipt pair from another desk.

### (3) Does `96aa522` now have sidecar-specific post-merge interruption proof
and an independent full race gate?

**UNCERTAIN for interruption proof; REBUTTED for the full-race claim.**

`96aa522` adds sidecar cleanup and a focused regression, but its direct payload
has no interruption/kill proof. The later Ozzy `c38f79acf9c9ae43ebd091a95f36837f43c0e423`
closes the no-sidecar-table boundary (`whole/path patch-id
6a62ff59b1b20a5873006b17ce72cd64229f65a6`, `+20/-20` production and `+48`
test), but no immutable receipt in the current census records an independent
full `CGO_ENABLED=0 go test -race -count=1 ./...` on 96aa522 or c38f79a.
The Ozzy `fresh-luna-adversarial` ref ends at c38f79a; its later docs are
contract text, not a race-gate receipt. A green full race on another checkout
would not prove this candidate’s adoption. Therefore no full-race credit is
available, and sidecar interruption coverage remains UNCERTAIN rather than
invented from prose.

### (4) Exact unique same-base candidates worth integration

**Only these exact-base product candidates remain worth integration review:**

1. `5eb3a38309a5319befaa434483bbf97004312129` — minimal overlay bookkeeping
   deletion, after its own review checkpoint. It is semantically duplicated by
   Ozzy’s stale-base `fb99037`, so integrate at most one mechanism.
2. `3b641ce9582541a60a7b37c8456bedaa9d86d29c` — best-effort queued-publication
   contract with help text and a direct output assertion. Its preceding exact-
   base doc/receipt stack is part of the range-diff context; the product
   payload itself is unique on this base.

The named `96aa522`, `cb154d9`, and the Ozzy `c38f79a`/`e43127e` sidecar/TDir
line are stale-base candidates and require a fresh transplant plus focused
red/green and race evidence before integration. A newer immutable hostile
receipt rejects `e43127e` specifically: missing/empty catalog fallback reaches
`AllProjectDirs`, arbitrary directories and symlink aliases fail source
detection, and `EnsureIndexedContainers` blocks on the held consolidated fence.
That receipt requires arbitrary-dir, symlink, missing-catalog, and held-fence
tests for any future no-fold candidate. `d918706` and `ef9f6ab` are
not worth integration because removing the consolidated fence risks writers
racing snapshot replacement; the nil-scope “responsive” claim cannot override
that invariant.

## Direction Lock

**NOT ELIGIBLE.**

The exact-base candidate set is known, but the required independent-adopter
proof is incomplete: 3b641ce has no other-desk adopter, and 96aa522 lacks an
independent full-race receipt plus sidecar interruption proof. Ozzy’s earlier
fb99037 is convergence/prior art for the 5eb shrink, not external adoption of
Furiosa’s recommendation; it must be deduplicated and scores zero. No full
race suite was run in this report-only census because these claims were
classifiable from immutable ancestry, chronology, patch IDs, direct diffs, and
receipt absence.

## Receipts and reproducibility commands

- Base identity: `git show -s --format='%H %P %s' 8c8216e25e22496b2e3e919fce836be49d692e25`.
- Candidate identity: `git show -s --format='%H %P %s' <candidate>`.
- Whole stable patch-id: `git show <candidate> --format= | git patch-id --stable`.
- Path stable patch-id: `git show <candidate> --format= -- <path> | git patch-id --stable`.
- Direct-parent numstat: `git diff-tree --no-commit-id --numstat -r <candidate>`.
- Ancestry: `git merge-base base <candidate>` and `git merge-base --is-ancestor base <candidate>`.
- Stack comparison: `git range-diff base^..<candidate-a> base^..<candidate-b>`.
- Exact adoption scan found no patch-id match for 5eb3a38 or 3b641ce outside
  their own Furiosa refs; Ozzy’s fb99037 is the separately identified semantic
  overlay-shrink receipt with a different patch-id.

No tests, product files, mailbox helpers, or cursors were modified by this
census. No full race suite was run.
