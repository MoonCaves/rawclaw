# Doctrine: we copy, we do not decide (2026-09-05)

Doctrine-Version: 1

This file is the fixed text. It is pasted verbatim into every builder, referee and auditor prompt.
It is never summarized, never paraphrased, never "adapted". If it needs to change, Jay changes it here.

## The rule

1. **No line of search, ranking, parsing, indexing or freshness logic is written from our own judgment.**
   Every such line is copied from a named open-source file, with repo, path and line range, or it does not land.
2. **No constant without an origin.** A number in code is either the source project's default, cited, or it is
   labeled `// policy: <date> <what> see docs/design/<file>` where that file records the measurement that chose it.
   The gate checks the file exists. An unlabeled number is a defect. `// policy:` covers numbers, timeouts and
   thresholds only; it never covers control flow, comparators or fallbacks.
3. **A measurement beats an argument, and a fresh measurement beats an old one.** Any ranking change is judged by
   the eight recall queries in `docs/design/exact-tier-notes.md`, run with `--before 2026-09-03`, on a `.backup`
   copy of the store, both runs timed. No numbers, no merge.
4. **Losers get deleted, not disabled.** Code that lost a measurement is removed in the same lane, with the numbers
   recorded in the notes so nobody re-adds it without new evidence.
5. **Disagreement is required, agreement is suspect.** A builder must name one thing in its brief it believes is
   wrong before coding. A referee must report a break, or the exact steps that failed to break it. "Looks good"
   and "you are right" are rejected as rulings.
6. **Fresh eyes, fixed prompt.** An auditor with no prior context, given only this file and the diff, runs after
   every merge. Its verdict is posted to mail thread `doctrine-audit`. Three consecutive PASS verdicts do not
   loosen anything.
7. **Mail before action.** `fetch_inbox` before every edit, commit, benchmark, ruling and message.
8. **Unique identity, always.** Register via `macro_start_session`, sign every message. Never "Antigravity",
   "Claude", or a default name.

## Added from WhiteGorge's "5 Mechanical Guards" (mail 325, 2026-09-05)

9. **Fetch the upstream bytes in-session before writing.** No code from memory. The verbatim upstream block is posted to the
   thread before the port is written. No bytes, no change.
10. **Three-way diff at lane end.** Referee diffs upstream source, our port, and the existing RawClaw interface. Any branch,
    condition or helper that exists in the port but not upstream is an invention and is deleted before merge, unless it is
    plain wiring or a type conversion.
11. **Handshake before the first edit under `internal/`.** Every agent always works in its own worktree, no exception; the ACK gates code, not the worktree. A builder posts its intended citations and gets an ACK from supervisor or referee before
    writing a line of logic. This is also the duplicate-work guard (see mail 316/318).

## The stop-guard (rule 12)

12. **No commit of logic under `internal/` lands without the provenance gate's APPROVE.** `scripts/provenance-gate.sh` fetches the
    cited upstream lines by SHA and asks a fast model, with a fixed prompt and a JSON schema, whether every added logic line is LIFTED,
    WIRING or POLICY. REJECT blocks the commit and names the lines. The same script runs in CI on the PR range; branch protection makes
    CI binding. `PROVENANCE_SKIP=1` is logged and is itself an audit finding.

## What "copied" means

- The commit message carries a trailer per copied unit: `Prior-Art: <repo> <path> L<start>-L<end> (<license>)`.
- The code carries a comment at the copy site: `// Lifted from <repo> <path> L<start>-L<end>`. Edits to the
  copied text are marked `// rawclaw:` with the reason.
- The source's tests come along when they exist.

## What is allowed to be ours

- Wiring: calling the copied function, plumbing a result into an existing struct.
- Product shape: refs, goal and resolution bookends, the help text, the note wording.
- Operational constants labeled `// policy:` with date and measurement.
- CLI flag definitions, help text, error wrapping (`fmt.Errorf(... %w)`), log lines. (WhiteGorge 328.)

## What "wiring" means, strictly (WhiteGorge 328)

Struct field assignment, function-call pass-through, error propagation (`if err != nil { return }`), type conversion.
Any other `if`/`else`, arithmetic, loop, sort comparison or string manipulation is LOGIC: it traces to a cited line or it is a HOLD.

## Phrases that void a ruling (WhiteGorge 330)

"100% right", "you are right", "looks good", "LGTM", "agreed" with nothing after it. A ruling containing these and no break, no break attempt and no command output is rejected as a non-ruling.

## Citations pin a commit

`Prior-Art:` records an immutable SHA or tag, never `main`/`master` HEAD (WhiteGorge 328). Line references rot otherwise.

## What the auditor checks, and only this

- Every non-comment line added to `internal/` in the diff traces to a `Prior-Art:` trailer or a `// policy:` label.
- Every numeric literal added is cited or labeled.
- No new mode, flag, table, env var or file that the brief did not name, unless the thread shows it was argued and measured.
- Deleted losers stayed deleted.
- The builder's disagreement message exists. The referee's ruling contains a break or a break attempt.
- Verdict is PASS or HOLD with file:line for every HOLD item. No advice, no praise.
