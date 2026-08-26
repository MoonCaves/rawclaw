# POSIX-sh session-catalog claim prior art

## Scope and evidence boundary

This is a prior-art report, not a claim that either RawClaw candidate is safe or
novel. The comparison is against the individual commits `37ec96bebb2a8317617544836ef9730149e1f0d4`
and `bd8346c5468435ba8636042c4846032e26460dba`, each inspected directly with
`git show` from the fixed base. No ancestry-contaminated branch range was used.

Graphify orientation preceded source inspection:

- MCP `query_graph(project_path=RawClaw, question="claim catalog session path", mode=bfs,
  depth=2)` found `renderHookScript`, `CatalogDir`, `WriteCatalogEntry`,
  `sessionHitFromCatalog`, the catalog hook tests, and the existing question
  “Can ln precheck reliably claim a session start hook?”.
- MCP `get_node("renderHookScript")` located `internal/cli/setup.go` and showed
  degree 16.
- MCP `shortest_path("renderHookScript", "WriteCatalogEntry", max_hops=5)`
  returned a three-hop path through hook installation and removal. This was
  useful for keeping the review at the hook/catalog seam, although the inferred
  middle edges are not treated as proof.
- MCP `get_neighbors("catalog_hook_test.go", relation_filter="references")`
  returned no neighbors, so targeted `git show` was used for exact candidate
  details.
- An initial MCP query against the research worktree failed because that
  worktree has no graph JSON; the same literal query against the fixed RawClaw
  checkout succeeded. This is recorded as a graph dead end, not source evidence.

## Exact mechanisms

### 1. Same-directory hard-link publication — adopt

**Mechanism.** Create a complete temporary regular file, then run the POSIX
`link()` operation through the `ln` utility with the temporary file as source
and the catalog directory as destination. A successful link creates the final
directory entry; an existing final entry or symlink makes the operation fail.
Only the winner launches ingest. Remove the temporary name after the link.

**Invariant.** The final directory entry is created atomically, and the source
and destination are on the same filesystem. The temporary file is complete
before publication. `link()` is specified to atomically create a new link and to
fail when the destination already exists or is a symbolic link.

**Primary sources.**

- POSIX `link()` specification, Issue 7 (2018):
  https://pubs.opengroup.org/onlinepubs/9699919799/functions/link.html
  (immutable specification URL; see the atomic-create and existing-entry
  failure requirements).
- Mature shell implementation: openSUSE `transactional-update.in`,
  `bashlock()`, repository commit
  `ef8af9f43526c7e482a4a290439a24fd1cb22ffc`:
  https://github.com/openSUSE/transactional-update/blob/ef8af9f43526c7e482a4a290439a24fd1cb22ffc/sbin/transactional-update.in#L254-L274
  It writes a PID to a same-directory temporary name, attempts `ln` as the
  winner test, retries only after stale-lock inspection, then removes the
  temporary name.

**Smallest RawClaw adaptation.** Keep the existing quoted `ln` publication,
but derive every temporary pathname from a validated flat catalog key, never
from the raw session identifier. The candidate `37ec96b` does this by allowing
only ASCII letters, digits, dot, underscore, and hyphen, rejecting empty and
dot-leading keys, and using `catalog_dir/.tmp.$$` as the temporary directory.
Keep the final lookup and the `-e || -L` check quoted. Do not add a hash tool,
lock daemon, `flock`, shell other than POSIX `sh`, or runtime dependency.

**Comparison.** `37ec96b` is the closer adaptation: its per-process temporary
directory no longer contains the untrusted ID, and its `ln` destination is the
catalog directory. `bd8346c` validates the final key but still builds
`tmp_dir="$catalog_dir/.tmp.$session_id.$$"` from the raw ID; a slash can escape
the intended temporary namespace. Thus `bd8346c` is not safe for its stated
untrusted-input boundary. Neither candidate gets novelty credit: the exact
same-directory `ln` pattern is older public practice, and the POSIX primitive is
standardized.

**Rejection reason for variants.** A raw-ID temporary directory, unquoted
pathname, or `ln` destination that is itself the untrusted path component is
rejected for traversal and special-file exposure. A cross-filesystem temporary
file is rejected because `link()` then fails with `EXDEV`, turning a valid claim
into an avoidable fallback race.

### 2. Atomic `mkdir` directory claim — useful external target, not the catalog record

**Mechanism.** Attempt `mkdir "$claim_dir" 2>/dev/null`; exactly one concurrent
caller can create a previously absent directory. Treat success as ownership and
existing-directory failure as “already claimed”. A trap removes the directory
when the short-lived operation exits.

**Invariant.** The claim name must be a safe, flat derived key. The directory is
the claim marker; payload must be written only below the directory after claim.
Never recursively remove an untrusted path. Stale-owner recovery is a separate
policy and must not silently delete a live owner.

**Primary sources.**

- POSIX `mkdir` utility, Issue 7 (2018):
  https://pubs.opengroup.org/onlinepubs/9699919799/utilities/mkdir.html
- Mature shell implementation: Quarkus `docs/docs-preview.sh`, repository
  commit `17154c2e7756096166c61b4f1a84301aa4dc5bc7`:
  https://github.com/quarkusio/quarkus/blob/17154c2e7756096166c61b4f1a84301aa4dc5bc7/docs/docs-preview.sh#L34-L52
  It uses quoted `mkdir` as the concurrency gate, records a PID below the
  owned directory, and cleans up with an exit trap.

**Smallest RawClaw adaptation.** Use `catalog_dir/.claim.<validated-key>` as a
  directory only if the catalog format can change from one flat JSON file to a
  claim directory. That is a materially larger on-disk compatibility change
  than hard-link publication. If retained as a future external pattern, write
  the JSON payload inside the owned directory and publish a separate completion
  marker; do not use `rm -rf` on any path containing raw input.

**Comparison.** Neither candidate uses `mkdir` as the final once-per-session
claim. Both use `mkdir` to create a temporary workspace, which is a sound
supporting step only after the pathname is safe. `37ec96b` improves this by
making the temporary directory independent of the raw ID. `bd8346c` does not.
`mkdir` alone is therefore not an adoption replacement for the existing flat
catalog record without changing readers and migration behavior.

**Rejection reason for RawClaw adoption.** A directory marker cannot directly
carry the current JSON catalog entry, and stale-directory cleanup introduces a
new crash-recovery policy. That is unnecessary format and lifecycle complexity
for the static-binary core. Reject as the primary catalog mechanism; retain as
prior art for a separate lock/claim directory if a future format explicitly
needs one.

### 3. POSIX `set -C` / noclobber redirection — reject as the final claim

**Mechanism.** In a subshell, enable noclobber (`set -C`) and run a plain output
redirection such as `: > "$entry"`. POSIX specifies that `>` redirection fails
when noclobber is set; the successful open/create is the claim attempt.

**Invariant.** The pathname must already be a safe flat key. The operation must
not be followed by an existence precheck that converts an ambiguous failure
into success. The created file is only an empty marker unless a separate safe
publication step replaces it.

**Primary source.** POSIX Shell Command Language, Issue 7 (2018), redirection
and `noclobber`:
https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html#tag_18_07
The specification states that `>` fails when noclobber is set and that a failure
to open or create a file fails the redirection.

**Smallest RawClaw adaptation.** Validate the flat key before constructing the
path, then use noclobber only as an empty claim marker. If rich JSON is required,
write it to a same-directory temporary file and use hard-link publication; do
not replace the hard-link invariant with `mv` after a separate precheck.

**Comparison.** Both candidate commits are descendants of the prior noclobber
shape. `37ec96b` removes the final noclobber claim in favor of same-directory
`ln`, while preserving fail-soft ingest for invalid IDs. `bd8346c` also uses the
`ln` shape but leaves the raw-ID temporary directory defect. The old candidate
pattern's `|| [ ! -e "$entry" ]` fallback is rejected: existence tests are not
an atomic claim, and special files or races can turn failure into a false win.
Neither candidate's switch to `ln` is novel; it is a sounder use of the
standardized primitive when implemented with safe temporary names.

## Adoption verdict

Adopt mechanism 1 only: validated flat key plus a same-directory temporary
regular file and POSIX `ln` hard-link publication, with `37ec96b`'s temporary
directory derivation (or an equivalent raw-ID-independent name). Treat
`37ec96b` as the usable candidate shape after independent hostile testing, but
do not accept its novelty claim. Reject `bd8346c` as written because its raw
session identifier still enters `tmp_dir`. Reject mechanisms 2 and 3 as the
primary catalog format because `mkdir` changes the on-disk shape and noclobber
does not provide the required rich-record publication without reintroducing a
race. This recommendation stays within RawClaw's static binary, zero-runtime-
dependency, POSIX-sh hook constraints.

## Accounting and gates

- Production lines changed: 0.
- Test lines changed: 0.
- Documentation lines added: 176 (this report only; count verified with
  `wc -l`).
- `git diff --check`: pass.
- Branch is based at `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` and contains this
  report commit only.
