# Lenny Spy Wave 2

Audit date: 2026-08-26. Base: 479d14c782a229d3348b290885028c5efa7a8740.
Scope: the six rawclaw-lenny-raid-* and four rawclaw-lenny-skill-* worktrees.
Prior spy head: 00ea62784a1cd533ff8a7081fcf3a36cb12cd4cc. Rival trees were read-only;
only this report is writable.

## Verdict

The current Lenny wave has stopped producing source changes. All ten worktrees are clean,
but their final commits are 3,196–5,047 seconds old and every tmux pane is parked at a
completed receipt/prompt. The strongest new evidence is process staleness plus weak gate
scope: the locate desk explicitly says its broad race suite was not observed. The prior
retry-suppression, duplicate-skill-patch, and logger-scope findings remain unchanged and
are not re-ranked here.

## Ranked current findings

### 1. CONFIRMED — ten clean trees are idle, not actively converging

Evidence: 20260826T101636Z-42996f87-lenny-heartbeat-39-receipts-or.md reports every desk as
STALL_CANDIDATE, with ahead=2..6 and dirty=0. Independent inspection at this audit:

| worktree | HEAD | ahead | dirty | age at inspection |
|---|---:|---:|---:|---:|
| rawclaw-lenny-raid-fence | 6ddd17a | 3 | 0 | 5,047s |
| rawclaw-lenny-raid-prewarm | 229f7e9 | 3 | 0 | 4,248s |
| rawclaw-lenny-raid-containers | be4ef6c | 4 | 0 | 4,262s |
| rawclaw-lenny-raid-hooks | 7a78884 | 4 | 0 | 4,172s |
| rawclaw-lenny-raid-locate | fc1a075 | 2 | 0 | 3,525s |
| rawclaw-lenny-raid-phase | dd57060 | 2 | 0 | 3,196s |
| rawclaw-lenny-skill-architecture | 65f3b8b | 6 | 0 | 3,298s |
| rawclaw-lenny-skill-modernize | 7bf86ec | 5 | 0 | 3,301s |
| rawclaw-lenny-skill-interfaces | 6209534 | 5 | 0 | 4,226s |
| rawclaw-lenny-skill-style | 37e4f70 | 5 | 0 | 3,983s |

The lenny-raid-* and lenny-skill-* tmux panes all end at a receipt or interactive
prompt; no pane showed a running gate. This is an operational stall signal, not evidence
of dirty source or a failed test. Ponytail tag: delete — stop parked workers and
select one clean transplant instead of keeping ten duplicate desks alive.

### 2. CONFIRMED — the strongest broad-gate claim is explicitly unobserved

rawclaw-lenny-raid-locate (fc1a0759d429c43bb5cf150f77ac79f10c18d3fc) reports focused
tests and static checks, then says: “The broad multi-package count=3 race suite ...
was unobserved due to machine-wide I/O contention.” Its claimed focused command was
CGO_ENABLED=0 go test -race -count=1 ./internal/agentproto (52.936s) plus
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run "TestTag|TestLocate|TestCatalog|TestRefresh"
(74.440s). Those claims do not establish the handbook gate
CGO_ENABLED=0 go test -race -count=1 ./....

Independent checks in the same read-only tree:

- CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'Test(Tag|Locate|Catalog|Refresh)'
  passed in 7.354s wall.
- No full-repository gate was run by this spy. It would be dishonest to report one green.

Ponytail tag: shrink — keep the narrow evidence attached to the narrow claim; remove
“green” implications for packages that were not exercised.

### 3. PLAUSIBLE — test/documentation mass is disproportionate to small production shrinks

Current diffs from the common base, separated by category:

| worktree / HEAD | production | tests | docs (FINDINGS.md) | safe reading |
|---|---:|---:|---:|---|
| raid-containers / be4ef6c | +8 / -19 = -11 | +99 / -0 = +99 | +85 / -0 = +85 | helper may be useful; contract test is large |
| raid-fence / 6ddd17a | +0 / -0 = 0 | +74 / -0 = +74 | +109 / -0 = +109 | test-only lane; no production transplant |
| raid-hooks / 7a78884 | +124 / -190 = -66 | +186 / -13 = +173 | +104 / -0 = +104 | behavior change needs targeted failure cases |
| raid-locate / fc1a075 | +54 / -84 = -30 | +0 / -0 = 0 | +25 / -0 = +25 | strongest small net-negative candidate |
| raid-phase / dd57060 | +32 / -42 = -10 | +0 / -0 = 0 | +61 / -0 = +61 | plausible helper transplant |
| raid-prewarm / 229f7e9 | +1 / -3 = -2 | +4 / -11 = -7 | +78 / -0 = +78 | tiny code cleanup; mostly review text |
| each skill lane | shared code payload | shared tests | 50–140 added | not independent implementations |

The line counts are from git diff --numstat 479d14c..HEAD, not self-reported net prose.
The skill rows intentionally avoid double-counting their shared payload; the exact overlap
is already a prior finding. Ponytail tags: yagni, shrink — retain a test only when
the chosen base lacks that contract, and do not treat review prose as shipped behavior.

## Worker scoreboard

| desk | SHA | source result | gate evidence | status |
|---|---|---|---|---|
| raid-locate | fc1a075 | -30 prod | focused claim; broad suite explicitly unobserved; spy rerun 7.354s | best candidate, bounded |
| raid-phase | dd57060 | -10 prod | claimed focused phase tests, 4.142s and 14.486s | plausible, not full-gated |
| raid-prewarm | 229f7e9 | -2 prod, -7 tests | claimed index 3x and focused hook/tag tests | tiny cleanup only |
| raid-containers | be4ef6c | -11 prod, +99 tests | claimed index race/shuffle 296.290s | test-heavy |
| raid-hooks | 7a78884 | -66 prod, +173 tests | claimed CLI race-shuffle 3x; no independent full gate | high-impact, prior defect unchanged |
| raid-fence | 6ddd17a | 0 prod, +74 tests | spy rerun phase test 2.332s | test-only; concurrency scope narrow |
| skill-architecture | 65f3b8b | shared code payload | docs/scorecard pane; no new code | duplicate lineage |
| skill-interfaces | 6209534 | shared code payload | docs/scorecard pane; no new code | duplicate lineage |
| skill-modernize | 7bf86ec | shared code payload | docs/scorecard pane; no new code | duplicate lineage |
| skill-style | 37e4f70 | shared code payload | docs/scorecard pane; no new code | duplicate lineage |

## Exact observed versus claimed gates

Observed by this spy:

    CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'Test(Tag|Locate|Catalog|Refresh)'
    ok github.com/MoonCaves/rawclaw/internal/cli 5.917s
    wall: 7.354s

    CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_LogsPhaseStartsAndDurations$'
    ok github.com/MoonCaves/rawclaw/internal/index 1.709s
    wall: 2.332s

Claimed in captured panes, not independently rerun here: raid-containers index race/shuffle
296.290s; raid-fence focused tests 4.142s/14.486s; raid-hooks CLI race-shuffle count=3;
raid-phase focused tests 4.142s/14.486s; raid-prewarm index 3x and focused hook/tag tests;
raid-locate agentproto 52.936s and CLI focused 74.440s. Raid-locate’s pane is explicit that
the broad multi-package gate was not observed.

## Changed evidence and safe transplants

- raid-locate is the cleanest net-negative source candidate at -30 production lines,
  but only its focused tests are substantiated; merge it only after the recipient runs the
  required full gate.
- raid-phase is a plausible -10 production-line helper transplant if the existing
  phase-start/duration/source attributes remain byte-for-byte covered.
- raid-prewarm is a -2 production-line cleanup, not a broad feature.
- raid-containers may save 11 production lines, but its +99-line test contract should be
  kept only if the chosen base lacks equivalent coverage.
- No safe transplant is identified from the skill desks: their code/test payload is shared,
  while the latest commits are review documents.

## Three public-wire zingers

1. “Ten clean branches, ten parked prompts: that is a scoreboard of receipts, not a race.”
2. “A focused green test is a flashlight; locate’s own pane says the building-wide alarm was
   never tested.”
3. “Four skill desks brought four scorecards to the ring, but the patch IDs say they wore
   the same code under different jerseys.”

## Documentation and memory boundary

The nearest AGENTS.md chain was read. It was intentionally left unchanged because the
requested fence permits only this report. No green full-suite claim is made. Before editing
the report, mnemon --store rawclaw recall rawclaw --limit 10 returned no rows after the
required no-argument form failed with the CLI’s “requires at least 1 arg(s)” usage message.
