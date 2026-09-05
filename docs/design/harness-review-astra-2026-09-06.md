# Harness review — 2026-09-06

**Disposition: REJECT unattended production.** This is a review, not an implementation.

Reviewed base: `294009e6d5f135f6fc54a11ddd9a3ab6ee14c19f`, branch `review/astra-harness-20260906`.
All ordinary repository line references below refer to that base unless a different commit is stated.
The assigned output is this file only (`REVIEW-BRIEF.md:5`). No push, registration, or mail is part of this review.

## 1. Failure cases and local evidence

### Incident verification

The brief's three approvals followed by rejection is present in the shared Git directory's
`decision-gate.log:16-19`, for the recorded patch-hash prefix `71067b976e44`:

```text
2026-09-05T09:55:28Z range=main..ded2022 diff=71067b976e44 model=gemini-3.8-flash-low verdict=APPROVE
2026-09-05T09:56:07Z range=main..ded2022 diff=71067b976e44 model=gemini-3.8-flash-low verdict=APPROVE
2026-09-05T13:14:36Z range=98ade78..ded2022 diff=71067b976e44 model=gemini-3.8-flash-low verdict=APPROVE
2026-09-05T13:20:16Z range=da205c291b25db805b89f6726ab4dc6468de197d..HEAD diff=71067b976e44 model=gemini-3.8-flash-low verdict=REJECT
```

These runs occurred on **September 5, 2026, at 17:55:28, 17:56:07, 21:14:36, and
21:20:16 WITA**, not September 6. There is also an earlier rejection at 17:48:40 WITA
(`decision-gate.log:15`). The ledger is local administrative evidence, not a tracked source file.

This establishes verdict disagreement for one recorded patch hash, **not four identical complete
model inputs**: the ranges differ, the script hashes only `diff.patch`, and it does not retain the
prompt, source digests, model revision, or schema digest in its note
(`.githooks/gate/provenance-gate.sh:93-110,143-150`). Do not report a measured model error rate
from this evidence.

Recomputing the non-test internal diff for `98ade78..ded2022` produced the full SHA-256
`71067b976e449648d8916607ee767eea0e74e76b85f8d084cb75303e2b6966f5`.
This corroborates that candidate's recorded prefix, not the identity of all four prompts.

The Meilisearch citation is in `docs/design/decision-references.md:166`, but is absent from the
seven `Prior-Art:` lines in commit `ded2022b144726ec842896373ba19070b6d3613b`, read with
`git show -s --format=%B ded2022`. The gate reads those lines from commit messages, not the
reference document (`.githooks/gate/provenance-gate.sh:28,53,59-75`). The document is used only
as a repository-substring membership check (`.githooks/gate/provenance-gate.sh:64-65`).
The cited document cannot itself supply the missing fetched evidence. The preserved ledger
does not contain the rejecting model's finding text; its exact rationale must not be invented.

The rejection is independently warranted under the composition rule, without pretending to
recover the model's reasoning. At `ded2022`, `internal/retrieve/retrieve.go:553-577,850-876`
uses a newest/oldest condition to bypass fusion and combine two lists. The fetched ClickClack
range actually initializes a rank expression unconditionally and starts a cursor predicate;
it does not implement that two-list fusion bypass. Wacli's cited lines order by BM25 and row
ID, not timestamp. The other cited ranges supply fusion, not the conditional bypass.
Combining those ingredients is exactly what `.githooks/gate/provenance-gate-prompt.md:33-39`
forbids. Supplying the Meilisearch trailer would fix an evidence omission, not automatically
validate every deduplication, fallback, and ordering decision in the port.

The following incident sources were fetched with `curl -fsSL --max-time 25` from the
SHA-pinned raw URLs. These are incident receipts, not additional entries in the thirty-project
catalog in section 2.

- `openclaw/clickclack`, `apps/api/internal/store/sqlite/search_pages.go:56-75`, commit
  `fa52084a04bf72e926eb593db02775043b3271fc`. Verbatim **61-63**:

  ```go
	rankExpression := "bm25(messages_fts)"
	innerOrderBy := "rank ASC, created_at_key DESC, m.id DESC"
	outerOrderBy := "ranked.rank ASC, ranked.created_at_key DESC, m.id DESC"
  ```

  Raw: `https://raw.githubusercontent.com/openclaw/clickclack/fa52084a04bf72e926eb593db02775043b3271fc/apps/api/internal/store/sqlite/search_pages.go`.
- `openclaw/wacli`, `internal/store/search.go:99-105`, commit
  `030505c8cbfe0b48d3f6bac53eb35ae1095d5cc5`. Verbatim **102-104**:

  ```go
	args := []interface{}{sanitizeFTSQuery(p.Query)}
	query, args = applyMessageFilters(query, args, p)
	query += " ORDER BY bm25(messages_fts), m.rowid DESC LIMIT ?"
  ```

  Raw: `https://raw.githubusercontent.com/openclaw/wacli/030505c8cbfe0b48d3f6bac53eb35ae1095d5cc5/internal/store/search.go`.
- `paradedb/paradedb`, `pg_search/tests/pg_regress/sql/reciprocal_rank_fusion.sql:32-68`, commit
  `c9395676e2299d2b156ae827161efdd15f08486a`. Verbatim **53-55**:

  ```sql
rrf AS (
  SELECT order_id, 1.0 / (60 + rank) AS s FROM order_search
  UNION ALL
  ```

  Raw: `https://raw.githubusercontent.com/paradedb/paradedb/c9395676e2299d2b156ae827161efdd15f08486a/pg_search/tests/pg_regress/sql/reciprocal_rank_fusion.sql`.
- `yoanbernabeu/grepai`, `search/hybrid.go:57-89`, commit
  `b355512615aafbea65baeaa6a478fbd1c70f3e24`. Verbatim **80-82**:

  ```go
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
  ```

  Raw: `https://raw.githubusercontent.com/yoanbernabeu/grepai/b355512615aafbea65baeaa6a478fbd1c70f3e24/search/hybrid.go`.

### Thirty production failure cases

`OBSERVED` means a deterministic fixture or the named local receipt establishes the claim.
`CONDITIONAL` means a concrete execution path or threat scenario derived from the cited code;
it is not a claim that a real model or attacker performed the exploit.

1. **Verdict shopping, not stable admission — OBSERVED disagreement; CONDITIONAL exploitation.**
   A builder reruns the same candidate until it passes. The ledger has three approvals and a
   rejection for one patch-hash prefix, and the script neither checks prior rejections nor
   caches an input-bound result. Persistence is rewarded over compliance. The ledger alone
   cannot separate model randomness from changing inputs. Evidence: `decision-gate.log:16-19`;
   `.githooks/gate/provenance-gate.sh:104-110,143-153`.

2. **The checker accepts its own contradiction — OBSERVED.** A schema-valid response with
   `verdict=APPROVE` and a `MADE` finding exits 0 and prints both the MADE finding and APPROVE.
   An empty findings array also passes for uncited logic; even a missing findings field passes
   the local parser. Native provider schema enforcement would address only the last example.
   A model can omit a decision or contradict its aggregate verdict without the host detecting it.
   Evidence: `.githooks/gate/provenance-gate.schema.json:3-5,15-51`;
   `.githooks/gate/provenance-gate.sh:128-139,151-153`; probes below.

3. **Upstream prompt injection — CONDITIONAL.** A cited source comment contains counterfeit
   rubric text or an instruction to return an empty finding list. It is concatenated into the
   same prompt string as doctrine and patch. The later instruction to treat it as data is a
   request to the model, not a parser-enforced trust boundary. An adversarial source can target
   omission, not merely say "approve." No live injection success rate was measured.
   Evidence: `.githooks/gate/provenance-gate.sh:74-75,94-107`;
   `.githooks/gate/provenance-gate-prompt.md:13-14,49-50`.

4. **Citation laundering — CONDITIONAL.** An agent publishes its invented algorithm in an
   accepted repository, then cites it back. The checker establishes a fetched immediate donor,
   not who originated the algorithm, when it became independent evidence, or why that donor
   applies. "FOUND" becomes "I first wrote it elsewhere." The same problem exists without
   malice when the donor itself copied an undocumented heuristic.
   Evidence: `.githooks/gate/provenance-gate.sh:61-75,129-135`;
   `.githooks/gate/provenance-gate-prompt.md:24-26`.

5. **A fork SHA proves neither upstream endorsement nor independence — CONDITIONAL.** Once a
   fork name is present in either reference document, any fetchable 40-hex commit is eligible.
   There is no check for membership in a designated upstream branch, a release, or an approved
   maintainer identity. A full SHA freezes the bytes chosen by the attacker; it does not bless
   their choice. Evidence: `.githooks/gate/provenance-gate.sh:64-75`. This finding does not
   depend on assuming GitHub serves an arbitrary fork object through its parent's URL.

6. **The allowlist is writable prose and drifts with the worktree — OBSERVED implementation;
   CONDITIONAL exploitation.** Membership is fixed-string substring search, so even an incidental
   mention of `owner/repo` qualifies. A candidate can add that mention in its own commit, or a
   dirty worktree can add it without committing; the "own commit" requirement is only an error
   string. A deleted or renamed prose reference can conversely reject a previously valid port.
   Evidence: `.githooks/gate/provenance-gate.sh:64-65`; `docs/design/decision-references.md:3-5,149-158`.

7. **Body text and other commits supply counterfeit trailers — CONDITIONAL.** The script says
   it copied `git interpret-trailers`, but actually greps every line beginning `Prior-Art:` from
   concatenated commit bodies. A quotation in a message body, or an unrelated documentation
   commit in the range, becomes evidence for a code commit. Commit ownership is lost before
   the model sees the trailers. Evidence: `.githooks/gate/provenance-gate.sh:7,28,52-63`;
   compare real trailer parsing in source S10.

8. **File-scoped citations are enforced only by prose — OBSERVED parser bypass.** The host
   checks quotes against one normalized string containing every source and its generated
   headers. A quote from a trailer aimed at a different file passes. So does a quote consisting
   only of the generated `owner/repo path` header. Whitespace normalization also weakens the
   claimed character-for-character requirement. An agent can reuse any convenient match if the
   model supplies the wrong association. Evidence: `.githooks/gate/provenance-gate.sh:75,115-135`;
   `.githooks/gate/provenance-gate-prompt.md:8-10,27-36`; probes below.

9. **Rebase and range/HEAD confusion invalidate approval identity — OBSERVED binding defect;
   CONDITIONAL misuse.** A verdict for `base..other-commit` is appended to the caller's `HEAD`,
   not necessarily that candidate. After a rebase, patch identity can remain while parent,
   integration context, and trailers change; notes either remain on the old object or may be
   copied by a separately configured rewrite. There is no complete approval identity to
   invalidate. Evidence: `.githooks/gate/provenance-gate.sh:28,147-150`; the `da205c2` note
   contains ranges ending at other commits, as reproduced in the incident commands.

10. **Notes are not a reliably transported or tamper-resistant certificate — OBSERVED config;
    CONDITIONAL loss.** This installation has an explicit default notes push refspec; it would
    be false to claim notes are never configured for push. However its fetch refspec is only
    heads, and an explicit branch push need not include the notes ref. Notes can also be
    overwritten by the configured forced notes push; the script swallows note-write errors.
    A new reviewer can receive code without its evidence. Evidence: shared `.git/config:14-16`;
    `.githooks/gate/provenance-gate.sh:143-150`; Git transport context in S05.

11. **A new clone has files, not the enabling hook configuration — CONDITIONAL.** The tracked
    pre-push file asserts that `core.hooksPath` points to it. That is a local configuration
    dependency, not a property of the commit. A fresh clone or a different checkout can push
    without this gate unless some independent authority installs/enforces it. Source S05
    documents hook discovery; observed enabling state is shared `.git/config:8`.
    Evidence: `.githooks/pre-push:35-42`.

12. **Worktrees share a mutable external gate; they are not inherently hookless — OBSERVED
    configuration; CONDITIONAL failure.** This review worktree inherits an absolute hooks path
    into the main checkout, and the live gate bytes matched at inspection. Therefore the claim
    "all worktrees lack hooks" is wrong. But relocating/removing that checkout breaks the path;
    changing its gate changes every worker's judge without changing the worker's commit.
    Relative/per-worktree overrides are another configuration, not an automatic guarantee.
    Evidence: shared `.git/config:8`; `.githooks/pre-push:38-45`;
    `.githooks/gate/provenance-gate.sh:18-20`.

13. **An outage or quota window stops the whole autonomous line — CONDITIONAL.** Missing `agy`,
    source failures, timeouts, and model nonzero exits reject; there is no deferred queue or
    resumable uncertainty state. A fleet retries and amplifies rate limits. Source downloads
    are serial, each gets 20 seconds, and their count/byte volume is unbounded. Without either
    timeout executable the model call has no script-enforced deadline. Passing the complete
    prompt as one argument introduces OS argument-size and model-context ceilings. The Beads
    hook may add its own 300-second wait. Evidence: `.githooks/pre-push:4-31`;
    `.githooks/gate/provenance-gate.sh:59-81,101-108`.

14. **The gate's author can avoid review of the gate — OBSERVED scope; CONDITIONAL deployment.**
    A hook/prompt/schema-only change contains no eligible internal Go diff and exits 0.
    Co-locating the gate with hooks prevents one branch-local substitution in this installation,
    but does not protect the shared checkout or the credentials that update it. After such a
    change is adopted, the judge runs under its author's new rules. Nothing here asks an
    independent, protected policy authority to approve it.
    Evidence: `.githooks/pre-push:38-42`; `.githooks/gate/provenance-gate.sh:18-20,35-36`.

15. **`policy:` is a filename test, not measurement verification — OBSERVED.** An unlabeled
    measurement, fabricated date, or `docs/design/../../proof.txt` containing no numbers can
    satisfy the host's file-existence test. It does not read the measurement into the prompt,
    require units or a dataset digest, or verify the result selected the number. An agent
    labels an arbitrary timeout and asks the model to call it POLICY. Evidence:
    `.githooks/gate/provenance-gate.sh:84-99`;
    `docs/agents/doctrine.md:12-15,59`; policy fixture below.

16. **Deletion has a contradictory proof requirement — CONDITIONAL.** The prompt calls deletion
    of an old invention FOUND and says not to request a source, but all FOUND findings require
    a matching quote and the host rejects missing quotes. Honest deletion can be blocked. In
    the opposite direction, an agent can misdescribe a safeguard as an old invention; no
    historical ownership manifest is provided to settle the classification. Evidence:
    `.githooks/gate/provenance-gate-prompt.md:15-18,27-30`;
    `.githooks/gate/provenance-gate.sh:129-135`; `docs/agents/doctrine.md:19-20`.

17. **Comments, braces, and imports change behavior — OBSERVED.** A build-tag flip, a movement
    of a closing brace that makes a call conditional, and an import swap from `math/rand` to
    `math/rand/v2` all exit 0 before the model. The line regex classifies syntax by appearance,
    not effect. Embed directives and package initialization are further reasons that these
    classes are not intrinsically harmless. Evidence: `.githooks/gate/provenance-gate.sh:37-46`;
    concrete fixture inputs below.

18. **Generated-code provenance is neither modeled nor consistently covered — OBSERVED scope;
    CONDITIONAL blockage.** Generated Go in a nested internal directory reaches the judge;
    the same file under `generated/` does not. It is false that a generated marker exempts an
    internal file. But the rubric has no relation between generator version, input schema,
    options, and output digest: an honest generator emits uncitable lines, while a builder can
    relocate them outside the filter. Evidence: `.githooks/gate/provenance-gate.sh:35-46`;
    `.githooks/gate/provenance-gate-prompt.md:21-26`; generated-file probes below.

19. **Vendored and data-driven behavior bypasses the copy-site gate — OBSERVED vendor probe;
    CONDITIONAL other paths.** A change in `vendor/`, a module replacement, SQL/data under a
    non-Go path, or an embedded payload can change the shipped behavior without matching the
    sole source filter. A clean-looking Go adapter can call newly substituted dependencies.
    Evidence: `.githooks/gate/provenance-gate.sh:35-36`;
    `.githooks/gate/provenance-gate-prompt.md:11`; vendor fixture below.

20. **Tests can be weakened without the gate noticing — OBSERVED.** A builder deletes the
    donor's regression case or changes its expected result in `_test.go`; the filter skips it.
    This does not mean test code itself ordinarily ships in the product. It means the doctrine's
    requirement to bring donor tests has no admission check, and later claims based on those
    tests can be made vacuous. Evidence: `.githooks/gate/provenance-gate.sh:35`;
    `docs/agents/doctrine.md:53`; test-only probe below.

21. **Docs-only policy changes can become executable authority — OBSERVED scope; CONDITIONAL
    manipulation.** Doctrine and reference prose are read from the current worktree, not from a
    separately authorized policy commit. A documentation-only push skips the model, yet changes
    future classification; a dirty doctrine can affect the current run without entering the
    candidate at all. Existing texts also disagree about SHA versus tag acceptance, skip
    variables, gate location, CI, and commit versus push timing. A worker follows whichever
    instruction is convenient. Evidence: `docs/agents/doctrine.md:5-6,43-46,73`;
    `.githooks/gate/provenance-gate.sh:2,48-49,67-70,95`;
    `.githooks/gate/provenance-gate-prompt.md:46`.

22. **Force-push checks the wrong history and has no receiving-side authority — CONDITIONAL.**
    The hook compares against the merge base of local HEAD and locally recorded origin/main,
    not the actual old and proposed new remote object IDs. A rewritten branch, stale tracking
    ref, or `--force-with-lease` update is not bound to the reviewed range. Client hook bypass
    remains possible; remote enforcement was not verified and must not be inferred from a
    doctrine sentence. Evidence: `.githooks/pre-push:37-45`;
    `docs/agents/doctrine.md:43-46`; S05's actual pre-push protocol.

23. **Merges are promises in prose, not a binding merge admission point — OBSERVED wiring;
    CONDITIONAL conflict drift.** `post-merge` contains only the Beads shim, while the gate's
    header says to run it before a merge by hand. A local merge can therefore advance without
    the decision gate; a conflict resolution or a changed target branch also needs a newly
    identified candidate, not a reused source-branch approval. The net range can contain the
    resolution, but that is not a guarantee it was checked before integration. Evidence:
    `.githooks/post-merge:1-33`; `.githooks/gate/provenance-gate.sh:2,11-13,28`.

24. **Multi-commit ranges erase both bad history and evidence ownership — CONDITIONAL.** Add
    uncited code in commit A and revert it in B: the endpoint diff is empty, but both commits
    are published. Add code in A and an unrelated donor trailer in B: the model sees the net
    code and pooled citations without their ownership. Squashing changes which of these
    contracts is meaningful; no such contract is specified by the implementation. Evidence:
    `.githooks/gate/provenance-gate.sh:28,35-36,53,93-97`;
    `docs/agents/doctrine.md:50,77`.

25. **Research permission silently becomes release permission — CONDITIONAL.** The owner may
    permit researching all licenses while still having a distribution policy that disallows a
    particular combination. There is no release-stage license decision or inventory in this
    gate: the trailer's parenthesized license is not checked. If the later owner review is
    removed to achieve no-human operation, copied code reaches release without that deferred
    decision ever happening. This is a missing policy boundary, not a legal conclusion about
    this repository. Evidence: `REVIEW-BRIEF.md:10-12`;
    `docs/design/decision-references.md:5,32-34`;
    `.githooks/gate/provenance-gate.sh:61-75,128-153`; S06-S09 and S28 supply separate inventory/policy mechanisms.

26. **Even the nominal source scope has holes — OBSERVED.** Uncited code in
    `internal/check.go` and in `cmd/review/main.go` exits 0 without a model call. The literal
    `internal/**/*.go` pathspec matched the nested fixture but not a Go file immediately
    under `internal/`. The doctrine's prose says every internal non-comment line; the code
    says something narrower. An agent relocates a helper and keeps calling it. Evidence:
    `.githooks/gate/provenance-gate.sh:35-36`; `docs/agents/doctrine.md:77`; scope probes below.

27. **An invalid range is a successful gate run — OBSERVED.** `--range does-not-exist..HEAD`
    emits a fatal Git diagnostic and exits 0, without calling the model. The script uses
    `set -u`, ignores the failed log command, suppresses the failing file-list command's stderr,
    and treats its empty output as no work. A shallow clone, typo, or missing base becomes an
    approval-shaped success. Evidence: `.githooks/gate/provenance-gate.sh:17,21-36`;
    invalid-range probe below.

28. **A push of another ref is judged as a push of HEAD — OBSERVED implementation;
    CONDITIONAL bypass.** The hook never reads Git's `<local ref, local OID, remote ref,
    remote OID>` input for the decision gate. Pushing `other:main`, multiple branches, a tag,
    a new branch, or a ref deletion runs one HEAD/origin-main check, independent of those
    update tuples and even of the named remote. This also interacts badly with the observed
    all-branches default push refspec. Evidence: `.githooks/pre-push:35-45`;
    shared `.git/config:15-16`; compare S05's loop.

29. **A space in a filename erases its logic from scrutiny — OBSERVED.** The path
    `internal/p/a b.go` enters the newline file list, then `for f in $FILES` splits it into
    nonexistent paths. Both individual diffs are empty and the gate exits 0. The later
    unquoted `$FILES` expansion has the same structural defect. An agent can choose a valid
    filename rather than falsifying a citation. Evidence:
    `.githooks/gate/provenance-gate.sh:35,42-46,85,93`; filename probe below.

30. **A copied regression passes the only gate while defeating the product — CONDITIONAL.**
    Correctly copied slow code or a donor constant inappropriate for this workload meets the
    provenance rubric. That rubric explicitly excludes code quality and tests, while the
    separate audit still demands benchmarks and a test suite. There is no enforced relationship
    between source identity and first-hit relevance, warm/cold latency, or answer-first freshness.
    An agent optimizes for citations instead of the one-command outcome. Eight repeatedly used
    queries are not evidence of a universal guarantee. Evidence:
    `.githooks/gate/provenance-gate-prompt.md:1-3,44-47`;
    `docs/agents/northstar-audit-prompt.md:15-19,40-48`;
    `docs/agents/doctrine.md:16-18`; `docs/design/exact-tier-notes.md:93-114`.

### Deterministic adversarial fixtures

Executed against a byte-for-byte copy of the reviewed script in a **separate temporary Git
repository**, with fixture-local hooks disabled. The model executable was replaced by a stub
returning the indicated JSON; the two quote-association probes also used a stub `curl` returning
one source line. These establish shell/parser behavior, **not real-model compliance rates,
GitHub acceptance of fabricated SHAs, or a product test-suite result**. The product worktree,
shared hooks, and real decision-gate notes were not mutated by the probes.

Gate SHA-256: `80075f7ecf053bdf659ce36587d8a0b92d6ff94e710b3f71e3d740dc1499e6ad`.
Prompt SHA-256: `503acf5dd2ea59fd606c5484044c4a04f1ecf4c9e5c8a1e5ce283362a162aaf4`.
Schema SHA-256: `9e5984e3cca83b46eda5484c460698ea9496cebeca4ca1a49ee3d703ff5043dd`.

| Fixture | Exit | Model called | Observation |
| --- | --- | --- | --- |
| Uncited nested internal logic; empty findings | 0 | Yes | APPROVE accepted. |
| Uncited logic; no findings field | 0 | Yes | No local schema validation. |
| APPROVE plus a MADE finding | 0 | Yes | Prints MADE and APPROVE. |
| Invalid range | 0 | No | Git failure becomes no work. |
| `cmd/review/main.go` | 0 | No | Outside filter. |
| `internal/p/check_test.go` | 0 | No | Explicitly excluded. |
| `generated/check.go` | 0 | No | Outside filter. |
| `vendor/example.invalid/p/check.go` | 0 | No | Outside filter. |
| Gate-prompt-only change | 0 | No | Policy change skipped. |
| Build constraint flip | 0 | No | Comment-shaped behavior skipped. |
| Closing-brace movement | 0 | No | Changed conditional scope skipped. |
| `math/rand` → `math/rand/v2` import | 0 | No | Dependency change skipped. |
| `internal/p/a b.go` | 0 | No | Word splitting loses path. |
| Fake policy text with `docs/design/../../proof.txt` | 0 | Yes | Existing non-measurement file suffices for host. |
| Matching quote from wrong target file | 0 | Yes | Target binding not checked by host. |
| Quote consisting of generated source header | 0 | Yes | Header treated as upstream evidence. |
| Generated nested internal Go, stub REJECT | 1 | Yes | Generated marker does not exempt it. |
| `internal/check.go`, stub REJECT | 0 | No | Immediate-child scope hole. |

For the brace case, the before/after difference moves only `}`. The second `Sink()` changes
from unconditional to conditional. No formatting or compilation result is claimed:

```diff
 func Check(flag bool) {
  if flag {
   Sink()
- }
  Sink()
+ }
 }
```

Minimal reproduction of the contradictory-verdict and invalid-range defects follows.
Run from the reviewed checkout; all mutations stay in the temporary fixture. The `agy` here
is deliberately fake and never calls a provider.

```sh
review_root=$PWD
fixture=$(mktemp -d)
mkdir -p "$fixture/.githooks/gate" "$fixture/docs/agents" "$fixture/internal/p" "$fixture/bin"
cp "$review_root"/.githooks/gate/* "$fixture/.githooks/gate/"
cp "$review_root/docs/agents/doctrine.md" "$fixture/docs/agents/"
cd "$fixture"
git init -q
git config core.hooksPath /dev/null
git config user.name review-fixture
git config user.email fixture@example.invalid
git add .
git commit -qm base
base=$(git rev-parse HEAD)
printf 'package p\nvar Choice = 42\n' > internal/p/check.go
git add internal/p/check.go
git commit -qm 'uncited choice'
cat > bin/agy <<'STUB'
#!/bin/sh
printf '%s\n' '{"verdict":"APPROVE","findings":[{"file":"internal/p/check.go","line":2,"class":"MADE","reason":"No source."}]}'
STUB
chmod +x bin/agy
PATH="$fixture/bin:$PATH" sh .githooks/gate/provenance-gate.sh --range "$base..HEAD"
printf 'contradictory_verdict_exit=%s\n' "$?"
PATH="$fixture/bin:$PATH" sh .githooks/gate/provenance-gate.sh --range does-not-exist..HEAD
printf 'invalid_range_exit=%s\n' "$?"
```

The documented reproduction was also executed verbatim from this report, with shell exit 0:
`contradictory_verdict_exit=0`, `invalid_range_exit=0`. The full fixture
set additionally used `{"verdict":"APPROVE","findings":[]}` and `{"verdict":"APPROVE"}`.
For the scoped-quote probes, the trailer pointed to `internal/not-changed.go`, while the
finding named `internal/p/check.go`; the stub source was `one source line with approval`.
Both that string and the generated header substring `openclaw/wacli ignored.go` passed the
host backstop. The fabricated source revision in that isolated test was forty zeroes.

### Local receipts and limits

- `git rev-parse HEAD` before report edits returned the reviewed base above; the initial
  `git status --short` was empty. The supplied directory already was the review worktree;
  no clone was required.
- `git config --show-origin --get core.hooksPath` returned an absolute path into the main
  checkout; its line is shared `.git/config:8`. `cmp` between that live gate and the reviewed
  gate returned 0. No claim is made about its later state.
- `git notes --ref=decision-gate show ded2022` and `git notes --ref=decision-gate show da205c2`
  reproduce the mismatched attachment of verdicts. The first two incident approvals are in
  the latter note, while the third is in the former. `git log` of the notes ref shows separate
  append commits. These notes are local evidence, not proof that a remote received them.
- `rg -n 'provenance-gate|decision-gate' .github .githooks/post-merge .githooks/pre-commit scripts/harness-gate.sh`
  produced no matching gate invocation. This is a local wiring check, **not** a remote
  branch-protection audit or a claim that every possible merge path was examined.
- No `graphify-out/` directory existed in this worktree; there was no local graph to query.
  No graph was generated or borrowed from another branch. No production model calls, pushes,
  registrations, mail, source edits, dependency builds, product tests, or policy changes were
  performed. The nearest owning handbook was read and deliberately left unchanged because
  this assignment fences edits to the report (`REVIEW-BRIEF.md:5`; `AGENTS.md:71`).

Report verification: exactly **30 numbered failure cases**, **30 source projects**, and **37
verbatim source excerpts** were checked mechanically against the downloaded files. All source
excerpts are 3-10 lines and at most 25 whitespace-delimited words; all thirty catalog records
carry full commit SHAs and fetched raw URLs. There are **18 recorded isolated gate probes**;
the documented two-case reproducer was separately executed. `git diff --check` returned 0.
The only repository path changed from the reviewed base is this report. These are artifact
and targeted-harness checks, not a claim that the product suite or remote protections passed.

## 2. Thirty source projects

These are thirty research candidates, not thirty proposed dependencies or approvals to copy.
All files below were actually fetched with `curl -fsSL --max-time 35` from
`raw.githubusercontent.com` at the listed full commit SHA. Initial missing paths were corrected
from repository trees before citation; no failed fetch is presented as evidence. Nix 2.24.0
and OpenHands 0.60.0 are deliberately historical selections, not claims about their current
defaults. License compatibility did not filter research; release authorization remains separate.

Each S-number identifies one project. The quoted block is contiguous, verbatim, and 3-10 lines;
additional fetched context is cited where needed. A mechanism being present in a repository
is not a claim about deployment maturity, security certification, or universal correctness.
To reproduce a source record, substitute its Raw URL and excerpt bounds:

```sh
curl -fsSL --max-time 35 "$RAW_URL" -o source.txt
sed -n "${START},${END}p" source.txt
```

### 1. SLSA — S01

**Source:** `slsa-framework/slsa`, `spec/build-provenance.md`, commit
`54b88b009fd45acb331c7e6578a526e0f36e0430`. Fetched context: L159-L219 and L239-L254.
Verbatim excerpt: **L251-L254**.

```json
"resolvedDependencies": [{
    "uri": "git+https://github.com/octocat/hello-world@refs/heads/main",
    "digest": {"gitCommit": "7fd1a60b01f91b314f59955a4e4d4e80d8edf11d"}
}]
```

**Lift and placement:** Lift the separation between externally requested parameters and resolved dependency identities. Store the requested donor ref alongside its resolved commit/blob digest in the admission input bundle, before any model call. A provenance record is a statement about inputs, not proof that those inputs were the right ones.

Raw: `https://raw.githubusercontent.com/slsa-framework/slsa/54b88b009fd45acb331c7e6578a526e0f36e0430/spec/build-provenance.md`.

### 2. in-toto attestation — S02

**Source:** `in-toto/attestation`, `spec/v1/statement.md`, commit
`2dcd055e9f72e746687c306e35f4e59720ff45be`. Fetched context: L9-L21 and L34-L55.
Verbatim excerpt: **L12-L16**.

```json
  "subject": [
    {
      "name": "<NAME>",
      "digest": {"<ALGORITHM>": "<HEX_VALUE>"}
    },
```

**Lift and placement:** Lift the Statement envelope and digest-bound subject. Make the exact candidate the subject and use a versioned decision-gate predicate for policy, sources, and verdict. Do not merely sign a free-form note mentioning HEAD; define what the predicate claims and who is authorized to assert it.

Raw: `https://raw.githubusercontent.com/in-toto/attestation/2dcd055e9f72e746687c306e35f4e59720ff45be/spec/v1/statement.md`.

### 3. Sigstore cosign — S03

**Source:** `sigstore/cosign`, `pkg/cosign/verify.go`, commit
`dd9ef8589841c8c4b8fb6460b93b21165c356382`. Fetched context: L82-L89 and L150-L152.
Verbatim excerpt: **L85-L89**.

```go
type Identity struct {
	Issuer        string
	Subject       string
	IssuerRegExp  string
	SubjectRegExp string
```

**Lift and placement:** Lift explicit issuer/subject identity constraints for verification of publisher attestations. The signature verifier belongs at the protected publication boundary, not in builder-owned configuration. These fields identify a signer; they do not prove that the copied code is correct or independently originated.

Raw: `https://raw.githubusercontent.com/sigstore/cosign/dd9ef8589841c8c4b8fb6460b93b21165c356382/pkg/cosign/verify.go`.

### 4. Sigstore gitsign — S04

**Source:** `sigstore/gitsign`, `internal/git/git.go`, commit
`6f3d985de8f41b2534010bb015ede094e2e6f9d6`. Fetched context: L29-L37.
Verbatim excerpt: **L35-L37**.

```go
	opts.Rekor = rekor
	return signature.Sign(ctx, ident, data, opts)
}
```

**Lift and placement:** Lift the commit-signing integration that passes a transparency-log writer into the signature path. Use it to attribute publisher commits or evidence commits. The accompanying function comment describes the embedded log entry and offline verification; neither a signed commit nor a log entry substitutes for an input-bound decision record.

Raw: `https://raw.githubusercontent.com/sigstore/gitsign/6f3d985de8f41b2534010bb015ede094e2e6f9d6/internal/git/git.go`.

### 5. Git — S05

**Source:** `git/git`, `templates/hooks/pre-push.sample`, commit
`3cb9185f65410273787f74333cc027d2ea5daada`. Fetched context: L15-L18 and L25-L44.
Verbatim excerpt: **L27-L30**.

```sh
while read local_ref local_oid remote_ref remote_oid
do
	if test "$local_oid" = "$zero"
	then
```

**Lift and placement:** Lift the actual pre-push input loop and its distinct deletion/new-ref/update handling. Check every proposed update, not HEAD against an assumed remote main. For an untrusted builder, put the authoritative check in a protected publisher or receiving boundary; a local hook remains bypassable. The supplemental Git documentation below covers discovery, refspec precedence, and notes rewriting.

Raw: `https://raw.githubusercontent.com/git/git/3cb9185f65410273787f74333cc027d2ea5daada/templates/hooks/pre-push.sample`.

**Supplemental files from the same Git commit, also fetched with curl:**

`git/git`, `Documentation/githooks.adoc`, commit
`3cb9185f65410273787f74333cc027d2ea5daada`; context L16-L26; excerpt
**L20-L22**. Hook lookup and executable-bit behavior are installation properties, not properties conferred by tracking a file.

```text
By default the hooks directory is `$GIT_DIR/hooks`, but that can be
changed via the `core.hooksPath` configuration variable (see
linkgit:git-config[1]).
```

Raw: `https://raw.githubusercontent.com/git/git/3cb9185f65410273787f74333cc027d2ea5daada/Documentation/githooks.adoc`.

`git/git`, `Documentation/git-notes.adoc`, commit
`3cb9185f65410273787f74333cc027d2ea5daada`; context L203-L208 and L381-L393; excerpt
**L203-L205**. Notes occupy a selected ref, and rewrite-copy behavior is separately configured. Treat transport and rewrite policy as explicit parts of an evidence workflow.

```text
`--ref=<ref>`::
	Manipulate the notes tree in _<ref>_.  This overrides
	`GIT_NOTES_REF` and the `core.notesRef` configuration.  The ref
```

Raw: `https://raw.githubusercontent.com/git/git/3cb9185f65410273787f74333cc027d2ea5daada/Documentation/git-notes.adoc`.

`git/git`, `Documentation/git-push.adoc`, commit
`3cb9185f65410273787f74333cc027d2ea5daada`; context L44-L52 and L430-L434; excerpt
**L430-L432**. Explicit refspecs take precedence over remote push configuration; the documented no-verify option bypasses pre-push. The local notes refspec is therefore not universal enforcement or guaranteed transport.

```text
`--verify`::
`--no-verify`::
	Toggle the pre-push hook (see linkgit:githooks[5]).  The
```

Raw: `https://raw.githubusercontent.com/git/git/3cb9185f65410273787f74333cc027d2ea5daada/Documentation/git-push.adoc`.

### 6. SPDX specification — S06

**Source:** `spdx/spdx-spec`, `schemas/spdx-schema-2-3.json`, commit
`c07600f239fbd9377be2195f4660887cfee72b78`. Fetched context: L624-L707.
Verbatim excerpt: **L705-L707**.

```json
        },
        "required" : [ "SPDXID", "ranges", "snippetFromFile" ],
        "additionalProperties" : false
```

**Lift and placement:** Lift the snippet record's required source-file and range identifiers from the explicitly selected SPDX 2.3 schema. Associate copied target ranges with source ranges and their license/copyright inventory. This is a structured representation to replace vague per-commit prose; it is not an automated compatibility ruling.

Raw: `https://raw.githubusercontent.com/spdx/spdx-spec/c07600f239fbd9377be2195f4660887cfee72b78/schemas/spdx-schema-2-3.json`.

### 7. REUSE tool — S07

**Source:** `fsfe/reuse-tool`, `src/reuse/header.py`, commit
`5795575acad2c53db1fb4998bb613db5ef6845d6`. Fetched context: L142-L159.
Verbatim excerpt: **L151-L153**.

```python
            spdx_copyrights = reuse_info.copyright_notices.union(
                existing_spdx.copyright_notices
            )
```

**Lift and placement:** Lift the preservation/union of existing copyright notices while writing headers. Put that operation in the copy/import step so attribution is not discarded during a port. The shown branch preserves notices; do not blindly copy the surrounding replace-license option or interpret header presence as permission to release.

Raw: `https://raw.githubusercontent.com/fsfe/reuse-tool/5795575acad2c53db1fb4998bb613db5ef6845d6/src/reuse/header.py`.

### 8. ScanCode Toolkit — S08

**Source:** `aboutcode-org/scancode-toolkit`, `src/scancode/api.py`, commit
`058f4396269476bdf2abf4a6b679a76f501b7e00`. Fetched context: L150-L179 and L224-L235.
Verbatim excerpt: **L224-L231**.

```python
    if detected_expressions:
        licensing = get_cache().licensing
        detected_license_expression = combine_expressions(
            expressions=detected_expressions,
            relation='AND',
            unique=True,
            licensing=licensing
        )
```

**Lift and placement:** Lift license-detection output and combination of detected expressions into the source inventory step, before applying an owner-defined release policy. Run the scanner outside the static product binary. Detection evidence and an AND-combined expression are inputs for a policy decision, not proof of ultimate code origin.

Raw: `https://raw.githubusercontent.com/aboutcode-org/scancode-toolkit/058f4396269476bdf2abf4a6b679a76f501b7e00/src/scancode/api.py`.

### 9. OSS Review Toolkit — S09

**Source:** `oss-review-toolkit/ort`, `evaluator/src/main/kotlin/Evaluator.kt`, commit
`d66e204892325d0d412fd9847cef01546986a5d9`. Fetched context: L41-L67.
Verbatim excerpt: **L60-L63**.

```kotlin
        val violations = scripts.flatMapTo(mutableListOf()) {
            val scriptInstance = run(it).scriptInstance as RulesScriptTemplate
            scriptInstance.ruleViolations
        }
```

**Lift and placement:** Lift the explicit policy evaluator/result pattern: evaluate authorized rules over recorded inputs and aggregate rule violations. Plug this into the admission command's policy stage without importing the entire ORT application. Rule scripts are executable policy and must come from the protected authority, never from fetched donor comments.

Raw: `https://raw.githubusercontent.com/oss-review-toolkit/ort/d66e204892325d0d412fd9847cef01546986a5d9/evaluator/src/main/kotlin/Evaluator.kt`.

### 10. Gerrit Change-Id hook — S10

**Source:** `GerritCodeReview/gerrit`, `resources/com/google/gerrit/server/tools/root/hooks/commit-msg`, commit
`48f79e2c974b35dfea9abed3583bb134a96af707`. Fetched context: L69-L89.
Verbatim excerpt: **L80-L82**.

```sh
if git -c 'trailer.separators=:=' interpret-trailers --no-divider --parse < "$1" | grep -q "^$token: $pattern$" ; then
  exit 0
fi
```

**Lift and placement:** Lift Git's structured trailer parsing rather than a body-line grep. Adapt the parsed representation to commit-local Prior-Art fields with required target ranges. A Change-Id is useful for review continuity across amendments, but it must not be used as a reusable approval for changed candidate bytes.

Raw: `https://raw.githubusercontent.com/GerritCodeReview/gerrit/48f79e2c974b35dfea9abed3583bb134a96af707/resources/com/google/gerrit/server/tools/root/hooks/commit-msg`.

### 11. Homu — S11

**Source:** `servo/homu`, `homu/main.py`, commit
`d6b918f45b4aadfb43ae4cba5b9b80a4f9c00638`. Fetched context: L141-L151.
Verbatim excerpt: **L141-L146**.

```python
    def head_advanced(self, head_sha, *, use_db=True):
        self.head_sha = head_sha
        self.approved_by = ''
        self.status = ''
        self.merge_sha = ''
        self.build_res = {}
```

**Lift and placement:** Lift the invalidation transition when a pull-request head advances: clear approval, status, merge identity, and build results. Apply the same state transition when candidate or base identity changes. Reuse this small state-machine idea, not the assumption that an old review remains valid because a branch name stayed constant.

Raw: `https://raw.githubusercontent.com/servo/homu/d6b918f45b4aadfb43ae4cba5b9b80a4f9c00638/homu/main.py`.

### 12. Prow/Tide — S12

**Source:** `kubernetes-sigs/prow`, `pkg/tide/tide.go`, commit
`1a000594c40919068dd43a5704f279f273135d18`. Fetched context: L2184-L2202.
Verbatim excerpt: **L2188-L2191**.

```go
	keys := []string{jobName, refs.Org, refs.Repo, refs.BaseRef, refs.BaseSHA}
	for _, pull := range refs.Pulls {
		keys = append(keys, strconv.Itoa(pull.Number), pull.SHA)
	}
```

**Lift and placement:** Lift the batch identity key containing job name, repository, base ref/base SHA, and every pull SHA. Use the corresponding complete candidate-and-base identity to scope a decision cache and a merge request. Borrow the keying mechanism without installing a new presubmit fleet, which would exceed the owner's gate constraint.

Raw: `https://raw.githubusercontent.com/kubernetes-sigs/prow/1a000594c40919068dd43a5704f279f273135d18/pkg/tide/tide.go`.

### 13. Bors-ng — S13

**Source:** `bors-ng/bors-ng`, `lib/worker/batcher.ex`, commit
`ca725797e53a88e954998de0bbb14a8a5acb13ab`. Fetched context: L419-L434.
Verbatim excerpt: **L427-L431**.

```elixir
        _ when pr.head_sha != patch.commit ->
          :race

        _ ->
          GitHub.merge_branch!(
```

**Lift and placement:** Lift the explicit head-versus-reviewed-patch comparison and its race outcome immediately before merge. Place that check in the protected publisher, together with checking the expected target ref. A queue is not mandatory; rejecting a stale candidate at the final write is the transferable part.

Raw: `https://raw.githubusercontent.com/bors-ng/bors-ng/ca725797e53a88e954998de0bbb14a8a5acb13ab/lib/worker/batcher.ex`.

### 14. commitlint — S14

**Source:** `conventional-changelog/commitlint`, `@commitlint/lint/src/lint.ts`, commit
`39635bdaf1eec7805ac1ff7339b1f122d91c6fe7`. Fetched context: L169-L188.
Verbatim excerpt: **L173-L175**.

```typescript
	const errors = results.filter(
		(result) => result.level === RuleConfigSeverity.Error && !result.valid,
	);
```

**Lift and placement:** Lift deterministic aggregation: error-severity failed rules become errors, and validity is computed from the error list. Use this inside the existing decision gate to reject any MADE finding, missing unit, or schema failure regardless of the model's top-level verdict. This is not a request to add a style linter as another gate.

Raw: `https://raw.githubusercontent.com/conventional-changelog/commitlint/39635bdaf1eec7805ac1ff7339b1f122d91c6fe7/@commitlint/lint/src/lint.ts`.

### 15. gitlint — S15

**Source:** `jorisroovers/gitlint`, `gitlint-core/gitlint/rules.py`, commit
`4d9119760056492eabc201bfad5de2f9e660b85f`. Fetched context: L66-L99.
Verbatim excerpt: **L67-L70**.

```python
    rule_id: str
    message: str
    content: Optional[str] = None
    line_nr: Optional[int] = None
```

**Lift and placement:** Lift structured rule violations with rule ID, message, content, and line number, plus the distinction between commit-wide and line-targeted rules. Make malformed trailers and invalid policy references local deterministic diagnostics. Preserve commit identity in the surrounding result instead of flattening all commit bodies together.

Raw: `https://raw.githubusercontent.com/jorisroovers/gitlint/4d9119760056492eabc201bfad5de2f9e660b85f/gitlint-core/gitlint/rules.py`.

### 16. promptfoo — S16

**Source:** `promptfoo/promptfoo`, `src/assertions/llmRubric.ts`, commit
`3e1710f282e1963664b162cc81114f41a917fb0c`. Fetched context: L28-L50.
Verbatim excerpt: **L38-L40**.

```typescript
  if (isGraderFailure(resp)) {
    return { ...resp, assertion };
  }
```

**Lift and placement:** Lift grader-failure propagation before normal score handling. In the gate adapter, an unavailable or malformed grader result should remain an infrastructure/unknown state, never become a pass through an inverted score or a permissive default. This excerpt is about failure separation, not a claim that LLM rubrics are deterministic.

Raw: `https://raw.githubusercontent.com/promptfoo/promptfoo/3e1710f282e1963664b162cc81114f41a917fb0c/src/assertions/llmRubric.ts`.

### 17. DeepEval G-Eval — S17

**Source:** `confident-ai/deepeval`, `deepeval/metrics/g_eval/g_eval.py`, commit
`077c81b1bffc7eabc3733f950f21b87c09d695ad`. Fetched context: L76-L89.
Verbatim excerpt: **L81-L85**.

```python
        self.evaluation_steps = (
            evaluation_steps
            if evaluation_steps and len(evaluation_steps) > 0
            else None
        )
```

**Lift and placement:** Lift the explicit evaluation_steps input as part of a versioned rubric record. Supply and hash those steps rather than allowing the evaluator to invent a rubric during an admission run. Its threshold and strict-mode settings are additional policy inputs; copying their defaults would not make them justified for RawClaw.

Raw: `https://raw.githubusercontent.com/confident-ai/deepeval/077c81b1bffc7eabc3733f950f21b87c09d695ad/deepeval/metrics/g_eval/g_eval.py`.

### 18. Ragas AspectCritic — S18

**Source:** `explodinggradients/ragas`, `src/ragas/metrics/_aspect_critic.py`, commit
`298b68274234c060deacab3cf5fb52aa3a20e885`. Fetched context: L134-L138 and L154-L165.
Verbatim excerpt: **L158-L161**.

```python
        if self.strictness > 1:
            score = Counter(
                [item.verdict for item in safe_loaded_responses]
            ).most_common(1)[0][0]
```

**Lift and placement:** Lift the explicit aggregation of collected verdicts if performing an offline consistency study of the gate. The nearby code makes an even strictness value odd. Keep the whole panel and a fixed sampling plan; do not put repeated voting on every push or describe majority vote as a proof of determinism.

Raw: `https://raw.githubusercontent.com/explodinggradients/ragas/298b68274234c060deacab3cf5fb52aa3a20e885/src/ragas/metrics/_aspect_critic.py`.

### 19. Inspect AI — S19

**Source:** `UKGovernmentBEIS/inspect_ai`, `src/inspect_ai/scorer/_reducer/reducer.py`, commit
`e3988a7d0e60f8222fbfa0ddee8d5086d2ed6e87`. Fetched context: L41-L80.
Verbatim excerpt: **L67-L69**.

```python
        ) -> str | int | float | bool:
            value, count = counts.most_common(1)[0]
            return value if count * 2 > panel_size else float("nan")
```

**Lift and placement:** Lift the strict-majority reducer's unscored outcome and panel metadata for an optional judge-calibration lane. The context explicitly retains missing votes in the panel size instead of lowering the bar. If a panel is ever used, map its unscored result to UNKNOWN, not APPROVE; the routine publisher still need not run multiple models.

Raw: `https://raw.githubusercontent.com/UKGovernmentBEIS/inspect_ai/e3988a7d0e60f8222fbfa0ddee8d5086d2ed6e87/src/inspect_ai/scorer/_reducer/reducer.py`.

### 20. LangSmith SDK — S20

**Source:** `langchain-ai/langsmith-sdk`, `python/langsmith/evaluation/evaluator.py`, commit
`9d1dff7ed3c1c34d314f14016473aebe772a17ea`. Fetched context: L71-L100.
Verbatim excerpt: **L90-L92**.

```python
    source_run_id: Optional[Union[uuid.UUID, str]] = None
    """The ID of the trace of the evaluator itself."""
    target_run_id: Optional[Union[uuid.UUID, str]] = None
```

**Lift and placement:** Lift separate evaluator-run and target-run identities, attached to structured evaluation results. Store these fields in the local evidence bundle so a reviewer can distinguish the model execution from the candidate evaluated. Reusing the data shape does not require sending the repository or donor bytes to a hosted tracing service.

Raw: `https://raw.githubusercontent.com/langchain-ai/langsmith-sdk/9d1dff7ed3c1c34d314f14016473aebe772a17ea/python/langsmith/evaluation/evaluator.py`.

### 21. Braintrust SDK — S21

**Source:** `braintrustdata/braintrust-sdk`, `js/src/framework.ts`, commit
`5353f61e71732f7a6d4e3310ab2f93a7f3400e0f`. Fetched context: L1544-L1561 and L1622-L1623.
Verbatim excerpt: **L1545-L1547**.

```typescript
          } catch (e) {
            logSpanError(rootSpan, e);
            error = e;
```

**Lift and placement:** Lift explicit evaluation error capture, and keep any configured trials distinct from their aggregate scores. Put raw error/trace data beside each gate attempt rather than retaining only a verdict string. The surrounding errorScoreHandler is itself policy; do not use it to silently assign a passing score to an outage.

Raw: `https://raw.githubusercontent.com/braintrustdata/braintrust-sdk/5353f61e71732f7a6d4e3310ab2f93a7f3400e0f/js/src/framework.ts`.

### 22. Nix — S22

**Source:** `NixOS/nix`, `src/libstore/derivations.cc`, commit
`206e32e2d7c72c940a4348648f5de46122c495c9`. Fetched context: L786-L800 and L871-L892.
Verbatim excerpt: **L884-L886**.

```cpp
    auto hash = hashString(HashAlgorithm::SHA256, drv.unparse(store, maskOutputs, &inputs2));

    std::map<std::string, Hash> outputHashes;
```

**Lift and placement:** Lift derivation identity built from serialized inputs, with dependency identities incorporated before hashing and memoization. Use that design for a complete gate-input cache rather than hashing only the patch. This source is intentionally pinned to release 2.24.0; adopting the idea does not require making Nix a RawClaw runtime dependency.

Raw: `https://raw.githubusercontent.com/NixOS/nix/206e32e2d7c72c940a4348648f5de46122c495c9/src/libstore/derivations.cc`.

### 23. Bazel remote execution/cache protocol — S23

**Source:** `bazelbuild/remote-apis`, `build/bazel/remote/execution/v2/remote_execution.proto`, commit
`77ec630134abbf9aa525f921eee4e5d11dc20f7e`. Fetched context: L608-L653.
Verbatim excerpt: **L621-L623**.

```protobuf
  Digest input_root_digest = 2;

  reserved 3 to 5; // Used for fields moved to [Command][build.bazel.remote.execution.v2.Command].
```

**Lift and placement:** Lift the Action separation between command digest and input-root digest, plus explicit cacheability. Treat checker/prompt/configuration and candidate/source input tree as separate hashed inputs. A cached judgment is reusable only for the same action; the wire format does not prove a nondeterministic judge would independently return the same answer.

Raw: `https://raw.githubusercontent.com/bazelbuild/remote-apis/77ec630134abbf9aa525f921eee4e5d11dc20f7e/build/bazel/remote/execution/v2/remote_execution.proto`.

### 24. OpenHands — S24

**Source:** `OpenHands/OpenHands`, `openhands/runtime/impl/docker/docker_runtime.py`, commit
`f0f452d1962046907dee27f30ef007973eec2b9d`. Fetched context: L421-L424 and L518-L532.
Verbatim excerpt: **L522-L524**.

```python
                entrypoint=[],
                network_mode=network_mode,
                ports=port_mapping,
```

**Lift and placement:** Lift the runtime lifecycle seam that explicitly passes entrypoint, network, ports, environment, and mounts. Put builders or graders behind a constrained runtime instead of handing them the publisher's environment. This historical 0.60.0 source permits host networking and arbitrary runtime kwargs: do not copy those permissions as secure defaults.

Raw: `https://raw.githubusercontent.com/OpenHands/OpenHands/f0f452d1962046907dee27f30ef007973eec2b9d/openhands/runtime/impl/docker/docker_runtime.py`.

### 25. SWE-agent — S25

**Source:** `SWE-agent/SWE-agent`, `sweagent/environment/swe_env.py`, commit
`3ea751c087f32b16e039a2233dd6eefecef325d5`. Fetched context: L24-L42.
Verbatim excerpt: **L27-L30**.

```python
    deployment: DeploymentConfig = Field(
        default_factory=lambda: DockerDeploymentConfig(image="python:3.11", python_standalone_dir="/root"),
        description="Deployment options.",
    )
```

**Lift and placement:** Lift a typed deployment configuration and separately bounded setup-command execution. Make the harness's execution environment an explicit input, outside product code. Do not inherit the mutable python:3.11 image tag or its root path as a security policy; select an authorized digest and privilege boundary separately.

Raw: `https://raw.githubusercontent.com/SWE-agent/SWE-agent/3ea751c087f32b16e039a2233dd6eefecef325d5/sweagent/environment/swe_env.py`.

### 26. Aider repo-map — S26

**Source:** `Aider-AI/aider`, `aider/repomap.py`, commit
`5dc9490bb35f9729ef2c95d00a19ccd30c26339c`. Fetched context: L684-L703.
Verbatim excerpt: **L698-L702**.

```python
            if num_tokens < max_map_tokens:
                lower_bound = middle + 1
            else:
                upper_bound = middle - 1

```

**Lift and placement:** Lift token-budget-aware repository navigation for an agent's preliminary source discovery. It belongs before review-input construction, not inside authoritative diff coverage: trimming a repo map is acceptable navigation, while trimming away unreviewed changes is not. The nearby approximation tolerance is not an exact audit-budget guarantee.

Raw: `https://raw.githubusercontent.com/Aider-AI/aider/5dc9490bb35f9729ef2c95d00a19ccd30c26339c/aider/repomap.py`.

### 27. Software Heritage SWHID model — S27

**Source:** `SoftwareHeritage/swh-model`, `swh/model/swhids.py`, commit
`ae97e5e854bd316bb9b505932eed205d73d6380c`. Fetched context: L290-L308 and L323-L356.
Verbatim excerpt: **L302-L306**.

```python
    >>> swhid = QualifiedSWHID(
    ...     object_type=ObjectType.CONTENT,
    ...     object_id=bytes.fromhex('8ff44f081d43176474b267de5451f2c2e88089d0'),
    ...     lines=(5, 10),
    ... )
```

**Lift and placement:** Lift qualified content identifiers with line ranges and separately represented origin/anchor/path context. Store them as durable donor references alongside fetched blob digests and target mappings. A qualified identifier is a locator and content/context claim, not independent proof that the named repository authored the algorithm.

Raw: `https://raw.githubusercontent.com/SoftwareHeritage/swh-model/ae97e5e854bd316bb9b505932eed205d73d6380c/swh/model/swhids.py`.

### 28. GitHub dependency-review action — S28

**Source:** `actions/dependency-review-action`, `src/main.ts`, commit
`284c089a1c4d8e8673b7398cdb8df358a9de47ad`. Fetched context: L169-L177.
Verbatim excerpt: **L173-L176**.

```typescript
        allow: config.allow_licenses,
        deny: config.deny_licenses,
        licenseExclusions: config.allow_dependencies_licenses
      }
```

**Lift and placement:** Lift explicit allow/deny/license-exception policy inputs for the release eligibility portion of the same admission command. The dependency-change comparison does not detect all pasted source code, so supplement it with snippet inventory rather than claiming it solves code-copy origin. No additional GitHub Actions workflow is proposed.

Raw: `https://raw.githubusercontent.com/actions/dependency-review-action/284c089a1c4d8e8673b7398cdb8df358a9de47ad/src/main.ts`.

### 29. JPlag — S29

**Source:** `jplag/JPlag`, `core/src/main/java/de/jplag/JPlag.java`, commit
`610afa1437339686ac174379811fff8a2f488fc5`. Fetched context: L80-L98.
Verbatim excerpt: **L86-L88**.

```java
        if (options.normalize() && options.language().supportsNormalization() && options.language().requiresCoreNormalization()) {
            submissionSet.normalizeSubmissions();
        }
```

**Lift and placement:** Lift language-aware normalization followed by submission comparison to find likely donor/port correspondences. Use it as an evidence-finding side tool on candidate blocks, never as automatic semantic equivalence or release permission. The shown source conditions normalization on language capabilities; it is not a universal cross-language matcher.

Raw: `https://raw.githubusercontent.com/jplag/JPlag/610afa1437339686ac174379811fff8a2f488fc5/core/src/main/java/de/jplag/JPlag.java`.

### 30. Dolos — S30

**Source:** `dodona-edu/dolos`, `core/src/hashing/winnowFilter.ts`, commit
`9aa312d6cde7b9befb1da35613c21ef9bda8e000`. Fetched context: L33-L75.
Verbatim excerpt: **L64-L66**.

```typescript
          if (buffer[i] <= buffer[minPos]) {
            minPos = i;
          }
```

**Lift and placement:** Lift token fingerprinting with rightmost-minimum selection for bounded donor lookup and duplicate-block discovery. Plug it into the source-evidence locator before the judge. Winnowing can suggest overlap despite superficial edits; overlap cannot establish who copied whom, preserve a changed condition's meaning, or settle licensing.

Raw: `https://raw.githubusercontent.com/dodona-edu/dolos/9aa312d6cde7b9befb1da35613c21ef9bda8e000/core/src/hashing/winnowFilter.ts`.


## 3. Architectural critique

### The premise is incoherent as written

"Never decide" is incompatible with choosing a source, a version, a subset of its code, an
adapter, a target representation, and a rule for conflicts between donors. Moving these
choices into an allowlist or a supervisor does not eliminate them. It changes who makes them
and whether the decision is visible. The allowed type conversions and wiring already create
exceptions to the literal rule (`docs/agents/doctrine.md:10-15,35-37,55-65`). The model prompt
adds an ordered slice beside a map and an FTS5 subquery as supposedly forced scaffolding,
despite otherwise forbidding uncited data structures and relationships
(`.githooks/gate/provenance-gate-prompt.md:21-23,37-42`). The boundary is judgment, not copying.

The defensible objective is **no unrecorded discretionary change**, not no decisions. Record
which decisions are delegated, the evidence required, and the conditions under which the
automation must stop. A source's popularity or age can inform a trust decision; neither is a
proof of suitability. A SHA is an address. A signature attributes bytes to an identity. An
attestation states something about identified bytes. None of those alone proves the statement
true or the code useful; see the deliberately separate data structures in S01-S04 and S27.

### The authors confuse evidence retrieval with proof

A quote's existence answers "did these characters occur?" It does not answer "does this
operator have the same role, with the same preconditions, across both programs?" The host's
substring test cannot establish the composition rule that the prompt asks it to defend
(`.githooks/gate/provenance-gate.sh:115-135`;
`.githooks/gate/provenance-gate-prompt.md:33-39`). The incident's stale ClickClack range is a
concrete example: it is a real file and a real SHA, yet it does not say what the local copy-site
comment claims (`internal/retrieve/retrieve.go:553` at `ded2022`; incident excerpts above).

Three-way comparisons, token overlap, and SWHIDs answer useful but narrower questions.
An exact-match check can prove identity of a specified block. A normalized comparison can
suggest ancestry. A content identifier can anchor retrieval. None decides whether an omitted
lock, inverted predicate, or changed default preserves a contract. Use S27, S29, and S30 as
evidence-location tools, never as a semantic or licensing oracle. Requiring a donor for every
relationship can also prevent combining two sound components into a working application;
the current composition rule literally creates that constraint
(`.githooks/gate/provenance-gate-prompt.md:37-39`).

### A model must not define both the obligation set and its own grade

The schema permits zero findings, and the program delegates enumerating decisions to the same
model whose verdict admits the change (`.githooks/gate/provenance-gate.schema.json:15-51`;
`.githooks/gate/provenance-gate-prompt.md:20-26`). This is self-certification with a fresh
conversation, not independent coverage. Fresh context removes conversational carry-over; it
does not manufacture ground truth or prevent the same blind spot recurring.

Give a judge an externally enumerated change-unit list and require one validated disposition
per unit. For an uncertain semantic relationship, allow **UNKNOWN**. The current schema's
APPROVE/REJECT and FOUND/POLICY/MADE enums cannot represent unavailable evidence separately
from an identified invention (`.githooks/gate/provenance-gate.schema.json:8-12,32-37`). The
status should be computed by the host from coverage and findings, not copied from a model's
top-level string. S14, S16, S19, S20, and S21 show reusable error/result separation mechanisms.

Voting is not determinism. Repetition can quantify variation only when all inputs and the
sampling plan are preserved; voting can still reinforce correlated mistakes. A fixed prompt,
a low randomness setting, or an odd number of votes is not a semantic correctness proof.
S17-S19 make rubric steps and aggregation explicit, but they do not justify retrying until
approval. Never turn an unknown or failed judge into a passing vote.

### Cache complete judgments, not convenient patch hashes

The present 12-hex patch hash is neither a cache nor an authenticated subject. It omits the
base tree, commit messages, policy, donor content, and judge configuration, and is only
recorded after the call (`.githooks/gate/provenance-gate.sh:147-150`). Reducing randomness by
reusing the first result would make the first mistake sticky if the cache key or authority
were wrong. A reject lock keyed only by this hash would also prevent correcting a missing
trailer while keeping the code unchanged.

Use an input manifest analogous to S01/S02, with a content-addressed action key analogous to
S22/S23. Include the exact proposed ref update, base and candidate tree/commit IDs, complete
trailer manifest, each upstream blob digest and source range, the policy version, the checker
binary/schema/prompt digests, and the model identifier plus exposed generation settings.
Record an opaque provider revision when available; explicitly mark it unavailable otherwise.
Retain the raw response and stderr separately from the computed disposition. An approval
cache hit requires all authorized inputs to match. Changed evidence creates a new key; it is
not a free reroll of an unchanged question. No fixed hit-rate or latency claim is made here.

### One gate is compatible with deterministic checks; zero authority is not

The owner rejected extra CI, linters, and tests as admission gates (`REVIEW-BRIEF.md:11-12`).
That does not logically require asking a model whether a SHA exists or a quote matches. Those
are small deterministic operations inside the existing decision gate. Copy a real Git input
loop from S05, real trailer parsing from S10, result aggregation from S14, and candidate
identity handling from S11-S13. Do not install all thirty projects, transplant their CI stacks,
or add runtime dependencies to the RawClaw binary (`AGENTS.md:9-12`).

But a client hook cannot be the sole security authority against an agent that owns the shell,
hook configuration, and push credentials. The current "never from the branch" comment does
not change who can edit the installed gate (`.githooks/pre-push:38-42`;
`.githooks/gate/provenance-gate.sh:18-20`). If removing humans also means refusing a protected
writer or receiving-side policy boundary, robust autonomous enforcement is not an available
outcome. Say that explicitly rather than calling a local convention fail-closed.

### What I would build instead

This is a proposed design, not an implemented patch or a claim that the owner already
authorized these choices. It retains **one admission command**, not a new matrix of gates.

1. **Separate proposing from publishing.** Builders can create commits but cannot write the
   protected branch, mutate admission policy, or access the publisher's credentials. One
   narrow publisher checks and applies the exact proposed ref update. A compare-and-swap on
   the expected old ref prevents approving one base and integrating on another. Use the
   candidate-binding portions of S05 and S11-S13; do not require their full CI environments.
2. **Make a closed input bundle.** Resolve the candidate and policy from Git objects, not
   dirty worktree files. Parse commit-local provenance structurally; require explicit target
   ranges, donor blob digests, and a separately authorized source identity catalog. Store
   generated outputs as derivations of generator/input/options digests, not fabricated
   copy-site citations. S01, S02, S06, S22, S23, and S27 provide the constituent formats.
3. **Perform mechanical admission first.** Check complete changed-path coverage, syntax,
   source retrieval, range bounds, checksums, target binding, and policy authorization.
   Classify comments, data, deletions, dependencies, and generator changes explicitly rather
   than silently excluding them. Use exact matching for declared unmodified copies and a
   small set of versioned authorized transformations. Stop on everything outside those
   transformations; do not claim that fuzzy matching proves semantics.
4. **Spend one bounded model call only on unresolved semantics.** Give it a fixed unit list,
   untrusted excerpts separated from instructions, no write credentials, and no authority to
   fetch or execute more material. Validate every response locally. Derive disposition from
   its findings and coverage. Persist UNKNOWN and infrastructure failures as distinct states;
   an outage suspends publication instead of triggering an uncontrolled retry loop.
5. **Cache and attest the complete result.** Sign or otherwise authenticate the input-bound
   disposition with an identity builders cannot impersonate. Preserve failures as well as
   approvals. Bind publication to that exact candidate, and retain the evidence somewhere
   receiving clones actually obtain it. S02-S05 and S20-S23 give reusable components; a Git
   note may be an index into the evidence, not the evidence's sole authority.
6. **Keep research unrestricted; preauthorize release policy separately.** Collect source
   notices and SPDX-style inventories during copying. Use the same admission command's
   protected policy data to decide whether a candidate is eligible for the configured
   release destination. Unclassified combinations stop without pretending the code is
   legally forbidden. If no human can ever answer such a case, the preauthorized domain
   must remain finite. S06-S09 and S28 are starting points, not legal determinations.

These choices are decisions. Hiding them in a copied script does not make them disappear.
The owner must authorize the trust boundary and the unresolved-case policy once; automation
can then operate inside that boundary without seeking permission on every routine port.

### The one-command promise is an objective, not a universal guarantee

"Exactly what they want" is not directly observable from a keyword query in every case.
Ambiguity, missing transcripts, and genuinely conflicting history cannot be eliminated by
choosing an older algorithm. What can be specified is latency under a stated workload,
first-hit relevance against a stated judgment set, and explicit incomplete-result behavior.
The product already names partial-result disclosure as an invariant (`AGENTS.md:55-57`),
while the north-star audit names first-hit precision and wall time
(`docs/agents/northstar-audit-prompt.md:15-22`). Those are different properties from origin.

If the owner forbids collecting any new behavioral evidence, neither an LLM nor a deterministic
origin gate can honestly guarantee that copied changes improve those properties. The existing
eight-query record can support conclusions about those recorded runs, not all future users
or workloads (`docs/design/exact-tier-notes.md:93-114`). The reasonable disagreement is not
"add a huge test suite anyway." It is: **accept the narrower assurance you chose, or authorize
the evidence necessary for the stronger claim.**

The authors are fooling themselves when they equate a fixed prompt with a fixed result,
a matched quote with semantic equivalence, an immediate donor with independent origin,
a local note with transported attestation, and a tracked hook with protected enforcement.
The cited implementation and the deterministic probes distinguish each pair. Renaming the
judge a "decision gate" does not close those gaps.
