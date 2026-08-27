# Codex Rescue: RawClaw rolling 12-hour operational review

Audit capture boundary: **2026-08-26 16:15:26 UTC through 2026-08-27 04:15:26 UTC**, exactly **2026-08-27 00:15:26 through 12:15:26 WITA (UTC+8)**. Current-state checks after that boundary are explicitly marked and are not folded into the 12-hour counts.

## Verdict

**INFERENCE — The suspected failure is verified.** RawClaw did not suffer from builders or hostile reviewers being intrinsically too slow: 57 distinct production patch families appeared, 46 received exact-SHA review, median critique arrival was 17m03s, and 33 of 42 test-only commits were followed by an overlapping production fix with a 6m33s median. The system failed below that layer. It generated 311 commits and 1,688 mailbox deliveries, yet only one documentation commit and **zero production commits** reached `origin/main` during the window. PR #35 sat green for about six hours without a named decision owner; PR #40's assembled source sat about 5h18m before even becoming a PR. **OPINION — Keep the cheap parallel builders and adversarial attacks, but remove review work that is detached from a live candidate, make every review terminate in PATCH, REJECT, or SHIP, and give one named integrator authority and a 15-minute decision deadline.** The hard cancellation gap found at the real SQLite writer-wait layer deserves one final gate; score disputes, provenance repetitions, direction locks, and consensus waits do not.

## Compact evidence table

| Measure | Observed value | Classification |
| --- | ---: | --- |
| Audit window | 12h: `2026-08-26T16:15:26Z..2026-08-27T04:15:26Z` | OBSERVED FACT |
| Unique commits across every ref | **311** | OBSERVED FACT |
| Commit classes | **79 production-touching; 42 test-only; 178 docs/report-only; 12 other** | OBSERVED FACT |
| Stable patch families | **57 production; 28 test; 175 docs/report** | OBSERVED FACT |
| Commits reaching `origin/main` in-window | **1 docs-only (`c818ea1`); 0 production** | OBSERVED FACT |
| Remote refs whose tip was updated in-window | **160 refs**: 119 docs/report, 24 production, 13 test, 4 other. This includes the `origin/HEAD` alias; there were 159 actual branch refs. | OBSERVED FACT |
| Production candidate inventory | **57 distinct production patch families** | OBSERVED FACT |
| Independently reviewed production inventory | **46/57 families**; 11 had no conservative exact-SHA review match | OBSERVED FACT |
| Review packets | **99** review/audit/referee packets cited in-window production SHAs | OBSERVED FACT |
| Critique latency | median **17m03s**; p90 **1h08m43s** | OBSERVED FACT |
| Red-test to overlapping production fix | **33/42 test-only commits**; median **6m33s** | OBSERVED FACT |
| Production ship bundles at boundary | **2**, PR #35 and PR #40 | OBSERVED FACT |
| Production PRs opened in-window | **1**, PR #40. PR #35 predated the window; #37 and #38 were older production PRs closed during it. | OBSERVED FACT |
| Production PRs merged in-window | **0** | OBSERVED FACT |
| GitHub issues changed in-window | **#19, #21, and #22 closed as stale bookkeeping for fixes verified on existing `main`; #39 opened and remained open.** | OBSERVED FACT |
| PR #35 wait | CI green by about **06:15 WITA**; still open at 12:15:26, approximately **6h00m** | OBSERVED FACT |
| PR #40 assembly delay | composite source complete about **06:28 WITA**; PR opened 11:46:59, approximately **5h18m40s** | OBSERVED FACT |
| Registered worktrees at boundary snapshot | **431** | OBSERVED FACT |
| RawClaw mailbox snapshot | **192** mailbox directories; **1,688** deliveries; **1,644** unique bodies; about **1.99 MB** | OBSERVED FACT |
| Supervisor mailbox load | **565** deliveries to the two supervisor inboxes | OBSERVED FACT |
| Scheduler load | ticker state `55`, meaning **56 ticks (0..55)** and **112 supervisor tick messages** | OBSERVED FACT |
| Rotation ledger | **999 lines; 52 headings; 40 Direction Lock mentions; 25 `NO MERGE AUTHORIZATION` mentions** | OBSERVED FACT |
| Duplicate/stale/ancestry work | at least **12** in-window commit subjects explicitly named regrade, duplicate, ancestry, containment, or current-base re-litigation | OBSERVED FACT |
| Live service snapshot | both supervisors, ticker, and watchdog live; no implementation worker remained live at the later boundary check | OBSERVED FACT |

The commit classification is path-based. A commit touching non-test Go is production; only `_test.go` is test-only; only Markdown/docs/Graphify output is docs/report; merge commits, mixed report-plus-test commits, and shell-only pressure-harness commits are other. Patch families use stable patch IDs, so cherry-picks and rebases do not inflate candidate counts.

## The actual throughput pipeline

| Stage | Arrival/work observed | Service/output observed | Queue or stall |
| --- | --- | --- | --- |
| 1. Fast builders | 57 production patch families | Many atomic branches and pushed tips | Healthy arrival rate |
| 2. Hostile review | 46/57 production families reviewed; median 17m03s | Real red tests, mutations, patch-ID comparisons, and rival challenges | 11-family review backlog, but not the main bottleneck |
| 3. Corrective implementation | 33/42 test-only commits got a later overlapping production fix; median 6m33s | Builders responded quickly to concrete failures | Healthy when the critique named a live file and invariant |
| 4. Integration assembly | Two live production ship bundles at boundary | PR #40 source finished around 06:28 WITA but was not opened until 11:46:59 | **Primary queue: about 5h18m before PR creation** |
| 5. Merge decision | #35 and #40 were green, mergeable, and had no formal review or named decision owner at the boundary | No production merge decision | **Terminal queue: green work waited instead of closing** |
| 6. `origin/main` | 311 commits existed across refs | One docs-only merge; zero production merges | **Shipping throughput was zero** |

**INFERENCE — Builders, test writers, and reviewers were servicing their queues faster than the integrator/merge stage.** A 6m33s implementation response and 17m03s review response feeding a 5h18m integration delay and a six-hour green-PR wait is not a review-quality shortage. It is an integration-owner and closure shortage.

**OBSERVED FACT — Reviews often changed code but rarely closed shipping state.** At the code layer, 33 of 42 test-only commits, 79%, were followed before the boundary by a production commit touching the same files. At the PR layer, three PR state decisions happened during the window: one documentation merge (#36) and two production-PR closures (#37 and #38). Neither live production ship bundle merged. **INFERENCE — The critique engine was effective as a patch generator and ineffective as a terminal decision engine.**

The actionable backlog at the boundary was not “57 things ready to merge.” Those 57 families include rivals, rejected variants, and superseded work. The defensible backlog was: two live production integration bundles awaiting a decision, 11 production families without a conservative exact-SHA review match, and a large set of reviewed variants that needed an integrator to select or reject them permanently.

## Ten highest-impact failures, ordered by lost shipping time

### 1. Green production work had no named merge decision owner

**OBSERVED FACT —** PR #35 was green for about six hours at the boundary; PR #40 was green and mergeable too. Neither had a formal review, review decision, or named owner responsible for merge/close/rework by a deadline.

**INFERENCE —** The final queue was unowned. Every upstream optimization was wasted while the terminal server had no service guarantee.

**OPINION —** Name the integrator in the launch receipt and PR body. Once CI and the one named hostile gate are green, that person must merge, reject, or order one exact patch within 15 minutes.

### 2. Integration assembly took hours after implementation was available

**OBSERVED FACT —** PR #40's composite source was complete around 06:28 WITA; the PR opened at 11:46:59 WITA, about 5h18m40s later.

**INFERENCE —** The system had implementation inventory but no fast path from candidate branch to a reviewable ship branch.

**OPINION —** The integration owner should continuously cherry-pick the current winner. Do not wait for a score ceremony or full rival consensus before making a PR-shaped artifact.

### 3. Report production consumed more branch output than code production

**OBSERVED FACT —** 178 of 311 commits were docs/report-only. Of 160 updated remote refs, 119 ended in docs/report work, versus 24 production and 13 test. The remote-tip proxy is 119 report outputs to 37 code/test outputs, more than 3:1.

**INFERENCE —** Worker capacity was assigned to audit artifacts faster than integration could consume their findings.

**OPINION —** Report-only work gets at most one active lane per supervisor and only while a named live candidate needs it. The default worker assignment is implementation or a hostile test that can directly change that candidate.

### 4. The ticker arrival rate exceeded supervisor and integrator service rates

**OBSERVED FACT —** 56 ticks created 112 supervisor messages. Rotation closeouts stopped around tick 49 while the ticker continued through tick 55. At the boundary the scheduler was still creating work while ship-ready work waited.

**INFERENCE —** The scheduler created a growing control queue and encouraged launching another audit instead of draining integration.

**OPINION —** A tick must not create new lanes when either supervisor has an unread prior tick, when a green PR has waited 15 minutes, or when the integrator has an undecided candidate. Ticks should be completion-driven, not unconditional arrivals.

### 5. PR #35 versus PR #40 and immutable provenance were re-litigated

**OBSERVED FACT —** Stable patch IDs showed every one of PR #35's 41 patch IDs in PR #40; #35 had zero patch IDs absent from #40. The branches were not ancestors and #40 substituted asynchronous publication for #35's synchronous behavior, but that is semantic supersession, not a reason to keep both terminal decisions open. At least 12 in-window commit subjects explicitly re-opened duplicate, ancestry, containment, current-base, or prior-art regrade questions.

**INFERENCE —** The system confused “not identical ancestry” with “both still deserve an open ship path.” Repeated containment debate delayed closure.

**OPINION —** Once an integrator declares one semantic successor, the older PR closes unless it contains a named, wanted behavior absent from the successor. A verified stable patch ID, ancestry result, or reproduced mutation is immutable for that exact head. Re-open it only after a relevant head change or a concrete contradictory reproduction.

### 6. Direction locks and `NO MERGE AUTHORIZATION` became closure barriers

**OBSERVED FACT —** The rotation ledger contained 40 Direction Lock mentions and 25 `NO MERGE AUTHORIZATION` mentions. Workers correctly lacked authority, but there was no equally explicit positive merge authority attached to a designated integrator.

**INFERENCE —** Safety language propagated more reliably than decision ownership. The result was consensus waiting.

**OPINION —** Workers remain unable to merge. One named integrator receives explicit authority to merge after the fixed gates. Direction locks are advisory, scoped to one risk, and expire after 20 minutes or a candidate-head change.

### 7. Mailbox volume obscured decisions

**OBSERVED FACT —** The startup inventory found 1,688 deliveries, 1,644 unique bodies, about 1.99 MB, including 565 messages in the two supervisor inboxes. The scheduler alone supplied 112 supervisor messages.

**INFERENCE —** The high unique-body ratio means this was not just harmless duplicate copying; supervisors had to parse substantial distinct prose.

**OPINION —** Mail should be an eight-line decision record, not a report transport. Put evidence in the branch report and send only candidate, head, invariant, result, requested decision, and deadline.

### 8. Immutable evidence kept receiving new provenance and score work

**OBSERVED FACT —** At least 12 commit subjects explicitly re-opened duplicate, ancestry, containment, current-base, or prior-art regrade questions. `ROTATION_LOG.md` reached 999 lines.

**INFERENCE —** Provenance and score maintenance continued after it could no longer change the candidate or decision.

**OPINION —** A verified stable patch ID, ancestry result, or reproduced mutation is immutable for that exact head. Re-open only on a new commit that touches the relevant files or a concrete contradictory reproduction.

### 9. Too many critiques ended as reports instead of PATCH, REJECT, or SHIP

**OBSERVED FACT —** There were 99 exact-SHA review packets and 175 docs/report patch families, but zero production merges in-window. Concrete red tests did much better: 33/42 received overlapping production follow-up.

**INFERENCE —** Reviews tied to executable failures drove implementation; prose-only verdicts accumulated.

**OPINION —** A reviewer has 15 minutes and one mutation pass. It must provide the smallest patch, decisively reject with a concrete trigger, or say ship. “More review” is not an allowed terminal status.

### 10. The watchdog proved process presence, not throughput

**OBSERVED FACT —** The 24-hour watchdog checked tmux presence and `tick.count` age. It did not prove supervisor turns, worker receipts, integration commits, PR decisions, or merges.

**INFERENCE —** A green watchdog could coexist with a dead integration queue, which is exactly what the window showed.

**OPINION —** Watch the oldest undecided candidate, last integration commit, last PR decision, and tick backlog. Tmux presence is diagnostic metadata, not success.

## What should remain unchanged

- **OBSERVED FACT — Fast inexpensive builders worked.** They produced 57 production patch families and responded quickly when a red test named a concrete invariant. **OPINION — Keep Luna/Gemini Flash-class builders in parallel isolated worktrees.**
- **OBSERVED FACT — Hostile mutation work found a real hard-gate bug.** The existing green cancellation test exercised only the process-local fence. Independent probes held a real SQLite writer transaction and reproduced a wait beyond 250 ms at the first non-context `tx.Exec`. **OPINION — Keep this adversarial pressure.**
- **OBSERVED FACT — Cross-review found silent-data-loss and false-green classes that ordinary green suites missed.** **OPINION — Preserve independent gates for data loss, corruption, races, destructive behavior, cancellation, and false-green tests.**
- **OBSERVED FACT — Isolated worktrees, stable patch IDs, atomic commits, clean pushes, and exact reproductions made rival comparison possible.** **OPINION — Do not relax these mechanics.**
- **OPINION — Keep prior-art search and rival challenges when they are attached to a live candidate and can cause an immediate adoption, rejection, or patch.** Stop only the repeated score/provenance layer after the fact is fixed.

## Proposed rapid operating loop

1. **Integrator first.** A ticket starts only after naming one integration owner, one ship branch, the acceptance tests, and the hard-risk classes.
2. **Builder burst.** Launch four to six fast builders for at most 25 minutes in isolated worktrees. Each owns files, commits atomic units, pushes, and reports one candidate SHA.
3. **Immediate hostile attachment.** Attach at most two reviewers to the leading live candidate: one behavioral/mutation reviewer and one integration/rival reviewer. Review begins as soon as the first candidate commits, not after every builder finishes.
4. **Three legal review outputs.** Within 15 minutes the reviewer returns `PATCH <sha>`, `REJECT <concrete trigger>`, or `SHIP <named gates green>`. A patchable local defect should come with the smallest patch.
5. **Continuous integration.** The integrator selects the smallest correct candidate, cherry-picks it immediately, and keeps a PR or ship branch green while reviews run. Losing variants are closed in the ledger once their useful patch is adopted.
6. **One hard-gate pass.** Run only gates tied to concrete data-loss, corruption, race, cancellation, destructive, or false-green risks. Reversible uncertainty becomes a follow-up issue.
7. **Decision SLA.** Within 15 minutes of green CI plus the named hostile gate, the integrator merges, rejects, or requests one exact corrective patch. No consensus wait.
8. **Then refill.** The scheduler creates new work only after the terminal queue is empty or the current candidate is actively blocked by an external dependency.

## Exact operating-rule changes

| Area | Replace with this exact rule |
| --- | --- |
| Ticker | A tick may create work only if both supervisors have acknowledged the prior tick, no green PR has waited 15 minutes, and the integrator has no undecided candidate. One tick creates at most one builder lane and one hostile-review lane per supervisor. Otherwise it emits one `DRAIN_QUEUE` digest and creates no worker. |
| Ticker backlog | If `current_tick - last_closed_tick > 1`, suspend new assignments automatically until closeout catches up. Do not keep sending two messages every ten minutes into an existing backlog. |
| Scoring | Award points only for: a production patch adopted into the ship branch; a red test/mutation that changes the candidate; a decisive rejection that closes a branch; or a merge. No points for report length, repeated hashes, repeated ancestry, timestamp ledgers, or regrading an immutable fact. |
| Direction locks | Only the named integrator may issue one. It must name one candidate head, one risk, and one falsifier. It expires after 20 minutes or any relevant head change. It cannot require consensus. |
| Worker allocation | While no green PR waits: minimum 60% builder/patch lanes, maximum 25% hostile-review lanes, maximum 15% integration/recovery/report work. While a green PR waits 15 minutes: 100% of new lanes attach to that PR or stop launching. Maximum one report-only worker per supervisor. |
| Mailbox | Maximum eight lines: `TYPE`, `CANDIDATE`, `HEAD`, `INVARIANT`, `OBSERVED`, `ACTION`, `OWNER`, `DEADLINE`. Evidence lives on the branch. Do not resend unchanged ancestry, patch IDs, score, or timestamps. ACKs are not rebroadcast. |
| Review limit | Maximum two independent reviews per candidate and one mutation pass per named invariant. A third review requires a new concrete trigger in changed code. Every review ends in PATCH, REJECT, or SHIP within 15 minutes. |
| Stale-base work | Rebase/ancestry/containment is checked once when a candidate enters integration and again only after its head or base changes. Same-head re-litigation is prohibited. |
| Merge authority | Worker prompts continue to say workers cannot merge. The launch and PR body must additionally name the single integrator who can merge after CI and the named hard gate. That integrator's decision does not require supervisor consensus or score parity. |
| Watchdog | Fail if the oldest green undecided PR exceeds 15 minutes, no integration commit occurs for 30 minutes while candidate inventory exists, tick backlog exceeds one, or a completed worker receipt is unconsumed for 15 minutes. Tmux presence alone never passes the watchdog. |

## Rescue actions

### Next 15 minutes

**OPINION —** Stop launching new report-only lanes. Name the integration owner for PR #40. Record PR #35's post-window merge as the new base rather than re-opening its debate. Rebase or reconstruct #40 on that base, list only its remaining semantic delta, and publish the single SQLite cancellation gate below. The only acceptable terminal results are merge after green, smallest cancellation patch after red, or explicit rejection of the affected delta.

### Next 2 hours

**OPINION —** Resolve #40's post-#35 conflict, run CI and the one real-writer cancellation gate, and merge if green. If red, apply the smallest context-aware statement patch and rerun only that gate plus CI. Change the ticker to queue-draining mode, delete score credit for report/provenance work, and require the eight-line mailbox contract. Target: one production merge or one decisive production rejection, not another ledger.

### Next 24 hours

**OPINION —** Run the rapid loop on the next two live tickets. Measure: median green-to-decision below 15 minutes; at least two production terminal decisions; zero tick backlog above one; at least 60% of active worker lanes building or patching; report-only remote tips below 30%; and at least 80% of hostile reviews ending in PATCH, REJECT, or SHIP. Preserve the data-loss/race/cancellation gates and remove no adversarial capacity that is attached to live code.

## PR #35 and PR #40

### Fixed-window state at 12:15:26 WITA

- **OBSERVED FACT — PR #35:** head `a33ab02`, open, green, mergeable/clean, no formal reviews, and about six hours past green CI.
- **OBSERVED FACT — PR #40:** then-head `bc16820`, open, green, mergeable/clean, no formal reviews. All 41 stable patch IDs from #35 occurred in #40; #40 added 44 more patch IDs and semantically replaced synchronous publication behavior.
- **OPINION AT THE BOUNDARY —** Close #35 as superseded and put the decision on #40. Keeping both open had no remaining throughput value.

### Current post-window state

- **OBSERVED FACT — PR #35 merged after the audit boundary** at `2026-08-27T04:33:50Z`, **12:33:50 WITA**, producing `origin/main` commit `9fd82d3`. This does not change the in-window count of zero production merges.
- **OBSERVED FACT — PR #40** is still open at head `0152683`. Its latest CI is green, but after #35 merged GitHub reports `CONFLICTING` / `DIRTY` against `main`. The head also contains a post-window orphan-sidecar fix.

**Current recommendation:**

- **PR #35 — MERGED; no rescue action.** Do not revert it merely because #40 was the broader successor. Treat `9fd82d3` as the base and stop the containment debate.
- **PR #40 — REWORK ONCE, THEN RUN ONE FINAL NAMED GATE.** Rebase or reconstruct it on `9fd82d3`, retain only #40's wanted semantic delta, and resolve conflicts. Do not launch another broad review.

The single remaining hard gate is:

1. On the reconciled #40 head, seed a successful consolidated fold and then mutate the source so a second fold must write.
2. From a separate `store.ConnectRW` pool, hold a real SQLite writer transaction on `consolidated.db`. Do **not** acquire or block on the process-local writer gate and do not use `consolidated.lock` as the blocker.
3. Start `SyncConsolidatedFromContext` on the changed source, prove it has passed process-local admission and reached the SQLite first-write wait, then cancel at 250 ms.
4. Require bounded return with `context.Canceled` or `context.DeadlineExceeded`, no new session/message/topic/verdict rows, no watermark advancement, no partial publication, and no goroutine leak.
5. Release the holder and require one retry to publish exactly once and advance the watermark.

Independent Tick 46/47 probes already showed the existing fence-only test can stay green while the real first write remains blocked beyond 250 ms because the fold uses non-context `tx.Exec`. That is a concrete false-green and cancellation risk, so it is an appropriate hard gate. If the reconciled #40 passes, **SHIP**. If it fails, apply the smallest `ExecContext`/context-publication correction and rerun this gate plus CI; do not restart general review.

## Evidence corrections

- **OBSERVED FACT —** `HAN_HARNESS_AUDIT.md` correctly reported missing ticker/watchdog activation when written, but that claim became stale after `HAN_TICKER_ACTIVATION.md` and live tmux/process evidence. The current harness has a ticker and watchdog; the remaining problem is that they measure arrivals and process presence better than decisions.
- **OBSERVED FACT —** `PR35_VS_PR40_CONTAINMENT_AUDIT.md` correctly rejected ancestry identity and documented behavioral substitution. Its recommendation to preserve both independent review paths did not follow from those facts. Stable patch IDs established patch containment; one integrator still needed to select the desired behavior and close the other path.
- **OBSERVED FACT —** The canonical Graphify graph was useful for locating `runTagWriteCmd`, `SyncConsolidatedFrom`, `AcquireConsolidatedFence`, and `StampIngestWatermark`, but it predated newer `origin/main`. All conclusions above were corroborated against live Git, source, GitHub, processes, tmux, worktrees, mailboxes, ticker state, and reports.

## Commands and primary evidence used

The following command groups produced the observed evidence. Long classification loops applied the path rules stated above to each SHA; stable patch IDs deduplicated cherry-picks and rebases.

```sh
# Mandatory orientation
mnemon --store rawclaw recall
cd /Users/jay-m4/code/rawclaw
graphify reflect --if-stale
sed -n '1,240p' graphify-out/reflections/LESSONS.md
# Canonical graph queried at /Users/jay-m4/code/rawclaw/graphify-out/graph.json;
# live Git/source controlled when the graph was stale.

# Fixed-window commit inventory and main throughput
git log --all --since='2026-08-26T16:15:26Z' --until='2026-08-27T04:15:26Z' --format='%H' | sort -u
git diff-tree --root --no-commit-id --name-only -r <sha>
git show --pretty=format: --no-ext-diff <sha> | git patch-id --stable
git log origin/main --since='2026-08-26T16:15:26Z' --until='2026-08-27T04:15:26Z' --format='%H%x09%cI%x09%s'
git show --name-only --format='' c818ea1212bb1f1110cefa65472f658b844840ef
git for-each-ref --format='%(refname) %(objectname)' refs/remotes/origin

# PR inventory, readiness, and current state
gh pr list --repo MoonCaves/rawclaw --state all --limit 100 --json number,title,state,isDraft,createdAt,updatedAt,closedAt,mergedAt,headRefName,headRefOid,baseRefName,mergeable,mergeStateStatus,url
gh pr view 35 --repo MoonCaves/rawclaw --json number,state,title,headRefName,headRefOid,baseRefName,mergeable,mergeStateStatus,isDraft,reviewDecision,reviews,statusCheckRollup,createdAt,updatedAt,mergedAt,closedAt,url
gh pr view 40 --repo MoonCaves/rawclaw --json number,state,title,headRefName,headRefOid,baseRefName,mergeable,mergeStateStatus,isDraft,reviewDecision,reviews,statusCheckRollup,createdAt,updatedAt,mergedAt,closedAt,url
gh issue list --repo MoonCaves/rawclaw --state all --limit 200 --json number,title,state,createdAt,updatedAt,closedAt,labels,url
gh issue view 21 --repo MoonCaves/rawclaw --json number,state,title,createdAt,updatedAt,closedAt,closedByPullRequestsReferences,url
gh api repos/MoonCaves/rawclaw/issues/21/events --paginate
git fetch origin main ozzy/composite-instant-tagwrite-pr-20260827 integrate/tagwrite-closeout-wave1
git rev-list --left-right --count origin/main...origin/ozzy/composite-instant-tagwrite-pr-20260827
git diff --stat origin/main...origin/ozzy/composite-instant-tagwrite-pr-20260827
git log --oneline --reverse bc16820..origin/ozzy/composite-instant-tagwrite-pr-20260827

# Patch containment and candidate behavior
git branch -a --contains bc16820
git branch -a --contains a33ab02
git show origin/ozzy/composite-instant-tagwrite-pr-20260827:internal/cli/tagpublish.go
git show origin/ozzy/composite-instant-tagwrite-pr-20260827:internal/index/consolidated.go
sed -n '1,240p' /Users/jay-m4/code/rawclaw-wt-pr35-vs-pr40-audit/PR35_VS_PR40_CONTAINMENT_AUDIT.md

# Cancellation evidence
sed -n '1,320p' /Users/jay-m4/code/rawclaw-t46-db-cancel-mutation/TICK46_DB_CANCEL_MUTATION.md
sed -n '1,360p' /Users/jay-m4/code/rawclaw-furiosa-t47-db-cancel-referee/TICK47_DB_CANCEL_REFEREE.md
sed -n '1,380p' /Users/jay-m4/code/rawclaw-furiosa-t53-current-base-cancel/TICK53_CURRENT_BASE_CANCELLATION.md
git show --no-ext-diff --unified=60 0cd0b9ce77e362b5bd4e973f948eb9981cdbf452 -- internal/index/consolidated.go internal/index/consolidated_test.go

# Harness, mailbox, and process evidence
git -C /Users/jay-m4/code/rawclaw worktree list --porcelain
ps -axo pid,lstart,command
tmux list-sessions
sed -n '1p' /Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/tick.count
wc -l /Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/ROTATION_LOG.md
rg -n 'Direction Lock|NO MERGE AUTHORIZATION' /Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/ROTATION_LOG.md
sed -n '1,260p' /Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/bin/supervisor-tick.sh
sed -n '1,260p' /Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/bin/supervisor-24h-watchdog.sh
find /Users/jay-m4/code -path '/Users/jay-m4/code/rawclaw*/.agent-mailbox' -type d
find /Users/jay-m4/code -path '/Users/jay-m4/code/rawclaw*/.agent-mailbox/*.md' -type f
shasum -a 256 <mail-files>
stat -f '%z' <mail-files>

# Report claims checked against live state
sed -n '1,320p' /Users/jay-m4/code/rawclaw-han-luna-harness-audit/HAN_HARNESS_AUDIT.md
sed -n '1,220p' /Users/jay-m4/code/rawclaw-han-luna-ticker/HAN_TICKER_ACTIVATION.md
find /Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness -maxdepth 3 -type f -print
```

No production code, PR, issue, mailbox, process, ticker, or harness state was changed by this audit. Only this report is intended to be committed.
