# Tick 53 rival movement and scoring spy

Audit cutoff: `2026-08-27T03:44:45Z` (2026-08-27 11:44:45 WITA). Evidence was
read from fetched public refs, GitHub PR/API state, immutable commit objects,
and local remote-tracking refs. No supervisor mailbox or cursor was accessed.
Rival worktrees were treated as read-only. This report grants no merge
authorization.

## Executive verdict

No score-bearing external adoption, withdrawal, stopped merge, or rebuttal was
found for the named claims. Recommended new score: **0**. Same-family,
self-authored, inherited-base, report-only, test-only, and unadopted movement
is excluded.

## Historical-ledger audit: Furiosa +9

This audit covers only the immutable receipts, reports, and commits cited by
`/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/SCORECARD.md`.
Graphify was queried first (`Furiosa TICK53`) and returned no matching nodes;
therefore it supplied no source claim. The +9 ledger narrows to **+8**:

| Original points | Event | Adopter or rebuttal | Exact cited evidence | Recommendation fingerprint | Dedupe ruling | Disposition |
|---:|---|---|---|---|---|---|
| +1 | Independent correction of `bd8346c` setup-blob identity | No cited immutable adopter receipt. The scorecard says “Confirmed by Han,” but the cited evidence contains only the independent audit report. | Audit commit `8f58e023554fb7d64b651db7a3f40ac8cea10fb8`; cited result blobs `7d4e1cca596d4db869441219417bc2f7a2875960` and `c577bdccfdeea97264caaa98ddd15b16a0de4fad`; cited `82/74` numstat. | N/A | The blobs are materially distinct, but the score event is report-only in the cited evidence; distinct payload does not create external adoption. | **WITHDRAW** — 0 |
| +5 | Composite authoritative tag overlay adopted by Han's integration desk; delayed tag publication remained visible and session identity used `(SessionID, StartUUID)` | Han integration desk, commit `5eec12be3fd9c30d7544cadd02e26e03260a15cb`; prosecution report records the external adoption and the two-session mutation result. | Prosecution commit/report `9886c4f02de14293cc94d438c599c15337fbcad8`; adopter commit `5eec12be3fd9c30d7544cadd02e26e03260a15cb`; cited e6 patch ID `99523502a6ce02afa6116c3efffbc72e1f44e03c`; adopter patch ID `9767304327f9ee282df41c7f5877ebc51e3f9f63`; cited race results `70.084s` and `98.553s`. | `SessionID + NUL + StartUUID` overlay key plus delayed-publication visibility | Explicit judge override makes this one event. No additive default `+3` or unique-transplant `+2`; the latter is a different credit rule and is not counted here. | **KEEP** — +5 |
| +3 | Owner-only future-cursor recovery recommendation adopted in Han's cadence document | Han Solo receipt `20260826T193838Z-7cac0a97-class-a-furiosa-plus3-cursor-r.md` explicitly identifies external adoption; receipt SHA-256 `b5f52fb624dcc5c7f2497a0b632ad1c4d6c64ac66374d56daa8d8d9eddae0cbb`. | Adoption commit `7f5217c2f8a3cd797b277fcf794250c989504461`, pushed clean/upstream-equal; cited receipt path above. | Preserve hashes; quarantine without deletion; owner-only cursor reset; normal-UTC unread self-test; retained hashes | Distinct recommendation-adoption event from the overlay and setup-identity audit. It is documentation adoption, not an unsupported helper-correctness claim. | **KEEP** — +3 |

### Historical ruling

The cited evidence supports **KEEP +5** for the overlay adoption and **KEEP +3**
for the cursor-recovery adoption. The setup-identity item is **WITHDRAWN**:
its cited object proves an audit correction, but no cited immutable external
adopter receipt survives the evidence boundary, and report-only work scores
zero. Corrected historical Furiosa total: **+8** (from +9). This is a ledger
audit only and grants no merge authorization.

## Immutable claim reconciliation

| Claim | Evidence | Verdict and score |
|---|---|---|
| Ozzy composite branch at `aab285d` | `aab285d894b4b9ab3abeb9eee96e72015d160177` resolves. Commit time is `2026-08-27T03:44:27Z`, 18 seconds before cutoff; parent is `edbb255`, whose merge includes `c818ea1`. The commit only deletes branch-local `FINDINGS.md` (`8` lines). The public branch subsequently moved to `bc1682071e3c9bb734c2783ee121f43002d814d0` (`2026-08-27T03:55:25Z` committer time). | **CONFIRMED ref, but the named object is report-artifact cleanup, not a new product event.** The later branch movement is still Ozzy self-publication with no independent adopter receipt. **0.** |
| PR #35 containment through `a33ab02` | Public PR #35 is **open**, head `integrate/tagwrite-closeout-wave1` at `a33ab023eae0ca324956a66cf17b7ffa5b16c39d`, base `main` at stale `86c5ce06b789e9e287154ba25acc699d04ff2c7b`. `a33ab02` is a one-file `internal/store/connect_bench_test.go` change (`8` deletions), patch ID `82e142f3630e29de6ffcf0182f05eba2050357ea`. PR #40's current `bc168207...` and PR #35's `a33ab02` have common merge-base `0d1da19...`; neither is an ancestor of the other. Their path delta is `16` files, `+1478/-97`, including behavior changes in `cmd_tag.go`, `tagrefresh.go`, and `consolidated.go`. | **REJECTED as literal containment.** PR #35 is open and unmerged; PR #40's body says it supersedes #35 in scope, but that is not byte/ancestry containment and is not external adoption. **0.** |
| Han fresh-base claim `48661f4...` versus actual `origin/main c818ea1` | `48661f403f880e2c1dac7615f39bbb8264eeafe7` resolves at `2026-08-27T03:50:21Z`, parent `aab285d`; `git merge-base --is-ancestor c818ea1 48661f4` succeeds. Its only direct change is `internal/cli/tagpublish.go` (`+9/-7`), patch ID `ad792367c7e171c12fddf2662c8b6bd48854a10b`. The object is on Ozzy's composite PR #40 branch, not a direct `origin/han/*` tip. Han's public T52 sidecar tip `77947bd769ac9cf219aaa68fc2f06b336dd9bea5` is based on `48661f4` and has merge-base `c818ea1`; its direct patch ID is `26091f9ee5a336a106c39463e16591750ce6245c`. | **Fresh-base ancestry is real, but `48661f4` is a base/inherited Ozzy object, not Han adoption. Han's `77947bd` is a public product/test candidate, not an independently adopted result.** **0.** |
| `d918706` rejection | `d91870634ff42b11165811111442acab26244d39` resolves at `2026-08-26T21:32:59Z`, parent `ea2b28a`; it removes the consolidated fence path in `internal/cli/cmd_tag.go` and changes `internal/cli/hostile_default_scope_test.go` (`+8/-32`), patch ID `5133121d630d549c255f82606b13c1012c6c748f`. It is not descended from `c818ea1` and no external adopter or later rebuttal ref exists. | **REJECTED.** Removing the fence leaves acknowledged snapshot-and-rename lost-write risk unresolved. **0.** |
| `4ac774a4` malformed-own-checkout versus absent-current-base test | `git show 4ac774a4` fails with unknown revision; no matching object or ref exists in the fetched object/ref census. The named default-scope test is absent on the relevant `bc8af914` line. | **Malformed historical object; its own-checkout failure is not current-base evidence.** **0.** |

## Public movement after cutoff

GitHub PushEvent evidence after the cutoff showed Han worker refs, but no
independent adoption:

- `worker/han-rival-sidecar-t52-20260827` pushed `77947bd...` at
  `2026-08-27T04:01:56Z`. It is a direct production/test candidate plus
  report material, based on fresh `c818ea1` ancestry, with no adopter receipt.
- `worker/han-integration-recovery-t52-20260827` pushed `0cd0b9c...` at
  `2026-08-27T04:11:11Z`. It is a direct production/test candidate plus report
  material, patch ID `0e42e5863f27b36d40ef718cb02ab7c0c7fd1729`, with no
  adopter receipt.
- No post-cutoff `refs/heads/han/*` producer tip was found. The only current
  Ozzy producer ref at or after the cutoff is PR #40's
  `ozzy/composite-instant-tagwrite-pr-20260827` at `bc168207...`; PR #40 is
  **open**, created `2026-08-27T03:46:59Z`, updated `2026-08-27T03:56:23Z`,
  and has no merge timestamp.

PR #35 is also **open**, with no merge timestamp. The deleted remote helper refs
`origin/pr35-head`, `origin/pr35-merge`, and `origin/pr40-head` were observed
during fetch; their deletion is ref housekeeping, not a product withdrawal or
merge stop.

## Duplicate and non-score rulings

- `a33ab02` is the PR #35 benchmark-test change, but PR #40 does not contain
  it by ancestry or whole-tree behavior; the two branches diverge from
  `0d1da19`. Do not count either open PR as adoption.
- `48661f4` is an inherited/base commit on Ozzy's composite line; do not award
  it to Han.
- `aab285d` deletes a review artifact; it is not product movement.
- `77947bd` and `0cd0b9c` are Han-authored public candidates, but publication
  and fresh ancestry are not external adoption.
- `d918706` is the rejected no-fence nil-scope family.
- `4ac774a4` has no immutable object identity and cannot support a score claim.
- No path or whole-patch identity establishes an independently adopted,
  score-eligible event in this window. No score change is recommended.

## Sharp challenge for the supervisor

Do not award PR #40 or the Han T52 refs points from fresh-base ancestry or a
public push. Require one named external adopter, an immutable adoption/green
receipt, exact whole/path patch IDs, and a clean current-base `c818ea1` gate;
otherwise the claimed movement remains **0**, regardless of how many rival refs
repeat the same mechanism.

## Verification receipt

- Base: `origin/main` = `c818ea1212bb1f1110cefa65472f658b844840ef`.
- Branch at completion was `worker/furiosa-t53-rival-points-spy-20260827`.
- Graphify was refreshed and used before source inspection (`query`, `explain`,
  and `path` over `ConsolidateFrom`, `runTagWrite`, and `TagWrite`).
- No product or test file was modified.
