# Cross-score hostile accounting audit

Date: 2026-08-26
Base: `5b9756b2200ff6bd670f07407407d84d9f42d84b`
Scope: current Conor, Norm, and Ozzy supervisor, integration, and worker
heads visible in the assigned checkout and their active worktrees. This is a
report-only audit. No product or test source was changed.

## Verdict

The current desks do not provide one independent code contribution per
advertised head. The accounting must collapse:

1. `norm/integration-wave1`, `norm/prewarm-ponytail`, and the `847426c`
   ancestor of `ozzy/harvest-wave1-20260826` for the same path-scoped setup
   patch (stable patch ID `e6322da4...`). Their whole-commit patch IDs differ
   because the worker and harvest commits also carry report or test files;
   this is shared product/test movement, not three implementation wins.
2. Ozzy's four catalog/hidden/prune/repro refs, all pointing at
   `cdc063d058cc...`, which is already an ancestor of the base.

The current Ozzy harvest tip `539de03` is also whole-commit patch-identical
to Norm `cfccbc6` (`7addd4ca...`). This is one nine-line test deletion and must
be counted once as adoption/cherry-pick evidence, not as two independent
implementations.

Three worker worktrees are dirty at audit time. Their uncommitted payloads are
not attributable to their advertised commits: Norm catalog (`+1/-7`), Norm
ingest (`+50/-238`), and Ozzy prune (`+29/-0`, with a whitespace error).

This report confirms duplicate attribution and stale/dirty receipt claims. It
does not infer an additional product bug from identity alone. The previously
reported Ozzy cleanup TOCTOU remains a source-backed finding at
`internal/index/containers.go:78-114`; it is not counted again as a new
cross-score movement.

## Method and exact commands

All comparisons used immutable object IDs, not branch names or report prose:

```sh
git rev-parse <ref>
git show <sha> --pretty=format: | git patch-id --stable
git diff <sha>^ <sha> -- internal/cli/setup.go internal/cli/setup_test.go \
  | git patch-id --stable
git diff --numstat <sha>^ <sha>
git merge-base --is-ancestor <sha> 5b9756b2200ff6bd670f07407407d84d9f42d84b
git range-diff 5b9756b..A 5b9756b..B
git -C <worktree> status --short --branch
git -C <worktree> diff --numstat
git -C <worktree> diff --check
```

Graphify orientation was attempted before source inspection:

```text
graphify reflect --if-stale
graphify query "supervisor integration worker" --graph /Users/jay-m4/code/rawclaw/graphify-out/graph.json --budget 4000
```

The assigned worktree has no `graphify-out/graph.json`; the current base
repository graph was used. Its query result was unrelated to this Git
accounting question, so no graph result is treated as evidence.

## Immutable inventory

The following are the relevant advertised tips and their commit-local stable
patch IDs. A tip patch ID identifies only that tip commit; ancestry and
range-diff are required for multi-commit claims.

| desk/ref | immutable tip | tip patch ID | receipt |
|---|---|---|---|
| Conor `bench-demolition` | `aece813d81362be3a19801b544eab2ff82b697a1` | `52f78fbdeb17bc2611f5aa3d32d151cf2749d5e2` | ref exists; its directory is currently checked out on another Conor branch |
| Conor `raid-lenny-modularity` | `db981351666f2e6029563f603ecbb899baeda045` | `1b816cccb39367bc899903e01ce1cc3f3cee865e` | active worktree clean |
| Conor `container-takeover` | `0193241b6ce317ec0c931e6160b8e82b21f48161` | `2aecad9542d47c189738a422ebccedbe0736a920` | active worktree clean |
| Conor `hook-failsoft-fix` | `ed1527ef8c8d7f8386b4908ef843fb9416535886` | `aac381e6408a359036ce248a73a04053b188fe64` | active worktree clean |
| Conor `six-skill-audit` | `34bef0ce688ea6c57b9d4a8fb8343ed10b635f6a` | `e76f5da403ab66fd2aa0d420f71e80600b3fd2af` | active worktree clean |
| Norm `integration-wave1` | `f026d6aed1918fb2c158c71df976eaf0dbf278c8` | `e6322da4ca5faaa5b3b596fdbb33409bf376a4e5` | clean; tracks origin |
| Norm `flash-catalog` | `cc7619ec1dd0ff6913fc142bfb7f3c4f084d7be4` | `a68f4adb56e536787aa543071779c46a60f2fd67` | **dirty** `internal/agentproto/agentproto.go` |
| Norm `flash-fence` | `6ac7f1a5d9e80eed9b14f0f92f8e3f3abf07d140` | `adfb05b7734c67a0bffb6782d56883ce9d037fe6` | clean |
| Norm `flash-hooks` | `2cc11d683761b702f26d1127efeb631a70ef348b` | `4fd42e86fccc178626d19bd0353aba3a029a93fa` | clean |
| Norm `flash-ingest` | `7478bfd965813d56b541586d1972df9225cae597` | `9c329d20220d8d6252e85d2aaccc68984ea7602f` | **dirty** `internal/cli/cmd_ingest_test.go` |
| Norm `prewarm-ponytail` | `7d5a6a550dc018519cca8f106b86786597d66540` | `fd7364068e7849f1c5b81da27a369f8a73a509ba` | clean; tracks origin |
| Ozzy catalog/hidden/prune/repro | `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` | `502a0b9b39d3b6121fdec252dc479256e6a6e271` | four refs, all clean except prune |
| Ozzy `flash-refresh-cleanup` | `89c8a284d20e4f6adba72accb3c0b34831a3b422` | `7e3d7d4981316e4cfb2b11f6a8eacbb71b314bdc` | clean |
| Ozzy `flash-spy-20260826` | `1f19f66f2a61c76c453cb1a2195e0df4b494823b` | `f1f21359fe7fb88dbc79a95dc2c686e8f04b1af9` | clean |
| Ozzy `harvest-wave1-20260826` | `539de03d46e4c3f251f123a261045d5ceea7eb0c` | `7addd4ca88dd31164e993883d4b57a4852e8e5b8` | clean, two commits ahead of origin; setup ancestor `847426c` is the scoped comparison |

The `conor/bench-demolition` worktree is checked out on the
`conor/raid-lenny-modularity` branch, so its directory name is not a reliable
branch identity. The ref inventory is authoritative.

## Confirmed deductions

### 1. Conor worktree directory is not the branch identity

```text
git -C /Users/jay-m4/code/rawclaw-conor-bench-demolition status --short --branch
## conor/raid-lenny-modularity...origin/conor/raid-lenny-modularity
git rev-parse conor/bench-demolition conor/raid-lenny-modularity
aece813d81362be3a19801b544eab2ff82b697a1
db981351666f2e6029563f603ecbb899baeda045
```

The directory name `rawclaw-conor-bench-demolition` does not identify the
checked-out `conor/bench-demolition` ref. Any status claim made from that path
without reading `git status --branch` would be stale. This is a receipt-boundary
deduction only; it does not accuse either commit of a product defect.

### 2. Norm integration, Norm prewarm, and Ozzy harvest ancestor share one path-scoped setup patch

Whole-commit stable patch IDs are not equal:

```text
for c in f026d6a 7d5a6a5 847426c 539de03; do
  git show "$c" --pretty=format: | git patch-id --stable
done
e6322da4ca5faaa5b3b596fdbb33409bf376a4e5 0000000000000000000000000000000000000000
fd7364068e7849f1c5b81da27a369f8a73a509ba 0000000000000000000000000000000000000000
e6322da4ca5faaa5b3b596fdbb33409bf376a4e5 0000000000000000000000000000000000000000
7addd4ca88dd31164e993883d4b57a4852e8e5b8 0000000000000000000000000000000000000000
```

The correctly scoped product/test comparison is:

```text
for c in f026d6a 7d5a6a5 847426c; do
  git diff "$c^" "$c" -- internal/cli/setup.go internal/cli/setup_test.go \
    | git patch-id --stable
done
e6322da4ca5faaa5b3b596fdbb33409bf376a4e5 0000000000000000000000000000000000000000
e6322da4ca5faaa5b3b596fdbb33409bf376a4e5 0000000000000000000000000000000000000000
e6322da4ca5faaa5b3b596fdbb33409bf376a4e5 0000000000000000000000000000000000000000
```

The integration tip's commit-local movement is the setup patch:

```text
git diff --numstat f026d6a^ f026d6a -- internal/cli/setup.go internal/cli/setup_test.go
2  5  internal/cli/setup.go
3  9  internal/cli/setup_test.go
```

The path-scoped patch proves one normalized setup change, not three. The
current Ozzy harvest tip is `539de03`, two commits ahead of origin; only its
`847426c` ancestor is included in this duplicate comparison. The exact
ancestry of the larger integration range must still be used for other commits;
the scoped patch alone cannot justify a second implementation credit.

The current harvest tip has a separate whole-commit duplicate with Norm's
`cfccbc6` test-slimming commit:

```text
git show cfccbc6 --pretty=format: | git patch-id --stable
git show 539de03 --pretty=format: | git patch-id --stable
7addd4ca88dd31164e993883d4b57a4852e8e5b8 0000000000000000000000000000000000000000
7addd4ca88dd31164e993883d4b57a4852e8e5b8 0000000000000000000000000000000000000000
git diff --numstat cfccbc6^ cfccbc6
0  9  internal/index/consolidated_test.go
git diff --numstat 539de03^ 539de03
0  9  internal/index/consolidated_test.go
```

The subject and author timestamp match; Ozzy's commit timestamp is later.
That supports adoption/cherry-pick attribution, not a second nine-line test
movement.

### 3. Four Ozzy names point at one base ancestor

```text
git rev-parse ozzy/flash-catalog-review ozzy/flash-hidden-pipelines \
  ozzy/flash-prune-benchmark ozzy/flash-repro-review
cdc063d058cc775ec2ee45a4231d8458ad3e9d43  (all four)
git merge-base --is-ancestor cdc063d058cc775ec2ee45a4231d8458ad3e9d43 \
  5b9756b2200ff6bd670f07407407d84d9f42d84b; echo $?
0
```

These refs add no post-base code. Their common tip is already integrated;
counting any of the four as current product movement is stale attribution.

### 4. Three dirty workers have payloads absent from their advertised heads

Observed status and line accounting:

```text
git -C /Users/jay-m4/code/rawclaw-norm-flash-catalog status --short --branch
 M internal/agentproto/agentproto.go
git -C /Users/jay-m4/code/rawclaw-norm-flash-catalog diff --numstat
1  7  internal/agentproto/agentproto.go

git -C /Users/jay-m4/code/rawclaw-norm-flash-ingest status --short --branch
 M internal/cli/cmd_ingest_test.go
git -C /Users/jay-m4/code/rawclaw-norm-flash-ingest diff --numstat
50  238  internal/cli/cmd_ingest_test.go

git -C /Users/jay-m4/code/rawclaw-ozzy-flash-prune status --short --branch
 M internal/index/consolidated_test.go
git -C /Users/jay-m4/code/rawclaw-ozzy-flash-prune diff --numstat
29  0  internal/index/consolidated_test.go
git -C /Users/jay-m4/code/rawclaw-ozzy-flash-prune diff --check
internal/index/consolidated_test.go:2217: new blank line at EOF.
```

These changes are not in `cc7619e`, `7478bfd`, or `cdc063d`. A clean report
or branch tip cannot advertise them as committed work. The Ozzy prune payload
also fails the required whitespace check.

### 5. The known Ozzy cleanup defect is one finding, not multiple desk wins

The immutable cleanup patch has the probe/release/remove sequence at
`internal/index/containers.go:78-114` (`89c8a284`). Its focused tests do not
insert an opener between `ROLLBACK` and `os.Remove`. This remains a confirmed
mechanism finding from the Ozzy audit, but this cross-score report gives it one
deduction regardless of how many supervisor reports mention it.

## Narrowed claims

- Report-only heads are evidence artifacts, not code movement. In particular,
  `conor/six-skill-audit`, `ozzy/flash-hook-review`,
  `ozzy/flash-integration-review`, and `ozzy/flash-ponytail-audit` have tip
  patches whose changed paths are reports. Their findings may be useful, but
  their lines must not be included in product/test net-line totals.
- Range-diff is ancestry-sensitive. The stable patch identity of a tip does
  not prove that the entire branch range is duplicate. The correct command for
  the integration/prewarm comparison is:

  ```text
  git range-diff --no-dual-color 5b9756b..f026d6a 5b9756b..7d5a6a5
  ```

  That comparison maps the setup movement and leaves any unrelated range
  commits to be judged separately.
- A branch that tracks `origin` is not thereby clean, and a branch without an
  upstream is not thereby dirty. Status and `diff --numstat` are the evidence
  used here.

## No deduction

- No additional source bug is inferred from equal SHAs or equal patch IDs.
  Duplicate accounting is not a correctness finding.
- The current-base hook behavior at `internal/cli/setup.go:299-314` retains
  the absolute-path and fail-soft contract. Historical unwritable-catalog
  claims are not re-filed as current defects.
- The focused fence and hook gates cited by the desk reports establish only
  their named assertions. They do not establish integration of dirty payloads,
  and their green status is not silently expanded into a full-suite claim.

## Net-line accounting

Count each normalized code/test patch once; exclude Markdown reports and
uncommitted payloads from committed attribution.

| bucket | additions | deletions | net | accounting |
|---|---:|---:|---:|---|
| Norm integration setup tip (`f026d6a`) | 5 | 14 | -9 | one patch; do not count again for Norm prewarm or Ozzy harvest |
| Ozzy catalog/hidden/prune/repro common tip (`cdc063d`) | 0 | 0 post-base | 0 | already an ancestor of base |
| Conor worktree/ref mismatch | 0 | 0 | 0 | directory label is not the checked-out branch |
| Norm catalog dirty payload | 1 | 7 | -6 | uncommitted; not attributable |
| Norm ingest dirty payload | 50 | 238 | -188 | uncommitted; not attributable |
| Ozzy prune dirty payload | 29 | 0 | +29 | uncommitted and `diff --check` fails |
| Norm `cfccbc6` / Ozzy `539de03` shared test deletion | 0 | 9 | -9 | one nine-line patch; adoption/cherry-pick counted once, not twice |
| **committed attributable movement** | **5** | **23** | **-18** | **deduplicated total: setup patch plus one shared nine-line test deletion** |

For completeness, the three dirty payloads together are `+80/-245` (net
`-165`) but remain outside the committed total. Adding them to the committed
score would turn unfinished work into a false receipt.

## Final classification

| subject | classification | reason |
|---|---|---|
| Conor bench worktree label | CONFIRMED DEDUCTION | `git status --branch` shows `raid-lenny-modularity`, not the directory label |
| Norm integration/prewarm/Ozzy harvest setup movement | CONFIRMED DEDUCTION | identical path-scoped stable patch ID; one movement, three attributions |
| Ozzy catalog/hidden/prune/repro refs | CONFIRMED DEDUCTION | common tip already ancestor of base |
| Norm catalog and ingest workers | CONFIRMED DEDUCTION | uncommitted payloads absent from advertised heads |
| Ozzy prune worker | CONFIRMED DEDUCTION | uncommitted payload plus `diff --check` failure |
| Ozzy refresh cleanup mechanism | CONFIRMED, single finding | source-backed TOCTOU at `containers.go:78-114`; not recounted per report |
| Ozzy harvest `539de03` vs Norm `cfccbc6` | CONFIRMED DEDUCTION | whole-commit patch-identical nine-line test deletion; count once |
| report-only heads | NARROWED | useful evidence, zero product/test lines |
| equal identity without semantic claim | NO DEDUCTION | no bug inferred from bookkeeping alone |
