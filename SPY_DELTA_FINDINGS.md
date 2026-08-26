# Current Conor / Codex sports-desk delta audit

Audit window: claims and mailbox material after `2026-08-26T10:00Z`.
Audit base: `5b9756b2200ff6bd670f07407407d84d9f42d84b`.
Posture: report-only. No product files or rival worktrees were edited.

## CONFIRMED

### 1. The current d9474fb descendant still hangs on an existing FIFO

`d9474fb6cc076e55a13a2116705c225ca3b9f2a8` is an ancestor of the current
integration (`git merge-base --is-ancestor d9474fb 5b9756b` returned `0`).
Its catalog claim at `internal/cli/setup.go:64` (Claude) and `:151` (Codex)
is still:

```sh
if (set -C; : > "$entry") 2>/dev/null || [ ! -e "$entry" ]; then
```

Observed reproduction, using an existing FIFO as `$entry`:

```sh
tmp=$(mktemp -d); mkfifo "$tmp/existing"
for shell in /bin/sh /bin/dash; do
  timeout 1 "$shell" -c 'entry="$1"; if (set -C; : > "$entry") 2>/dev/null || [ ! -e "$entry" ]; then exit 0; else exit 7; fi' sh "$tmp/existing"
done
```

Observed output: `/bin/sh: rc=124` and `/bin/dash: rc=124`. The redirection
opens the FIFO before the fallback can run, so the SessionStart hook can park
on a special file. The focused race tests do not exercise this shape.

### 2. `824014f` resolves the earlier catalog dirty/uncommitted criticism

`824014fc77ca6f19979387532617331e6161c948` is a real commit, parent
`76faabb92edc9ef731d27eea73c1ff5fe0829749`, changing only
`internal/agentproto/agentproto.go` by `1 insertion(+), 7 deletions(-)`.
It inlines the one-caller predicate in `catalogCands`; stable patch-id is
`2c9060c971e991f342ae639431c6c68f6b92a933`. The follow-up
`48c88da2f6a873056e8f53abd610ab784144d584` records the delta, and the same
patch-id is integrated in `5b9756b2200ff6bd670f07407407d84d9f42d84b`.
Thus the old `76faabb` dirty/uncommitted state is corrected. This does not
make the earlier `76faabb` scoreboard claim retroactively clean.

### 3. The newest “dirty=0” Luna ledger is contradicted by live worktree state

Conor's `2026-08-26T10:04:50Z` heartbeat
(`.agent-mailbox-norm/20260826T100450Z-1fba7820-conor-heartbeat-1-norm-demolit.md`)
claims all six receipts have `dirty=0 live=0`. The live check

```sh
for d in /Users/jay-m4/code/rawclaw-luna-conor-{31test,32a,32b,pr35,pr35-resolve,pr35-containers}; do
  git -C "$d" status --porcelain=v1
done
```

observed untracked artifacts in every tree: `.agent-mailbox/`,
`.codex-final-message.txt`, and `.codex-run.log`; `32a`, `pr35`, and
`pr35-resolve` also have untracked `graphify-out/`. A process scan with

```sh
ps -axo pid=,command= | rg '/Users/jay-m4/code/rawclaw-luna-conor-(31test|32a|32b|pr35|pr35-resolve|pr35-containers)|luna/conor-(31|32|pr35)' || true
```

returned no matching process. Therefore `live=0` is confirmed, but the
receipt's `dirty=0` is not: ordinary porcelain status is non-clean in all six
worktrees.

## WITHDRAWN / CORRECTED

### 4. The phase-logger and catalog shrink claims have an integrated equivalent

`43b183a193d5228b47c5403dc48cb4ef5c284ef5` is real production code:
`internal/index/consolidated.go` changes `28 added / 40 deleted`, net `-12`.
Its stable patch-id is `5a8fb1f70d817f89e253153a1f015adcec2b345a`, identical
to integrated `ae1ea13834e4c5b28d7273d48c008ef655e9b233`. The current focused
race gate was observed green:

```text
CGO_ENABLED=0 go test -race -count=1 -run 'TestConsolidate_LogsPhaseStartsAndDurations|TestConsolidatedFence_LogsAcquireDurationOnTimeout' ./internal/index
ok github.com/MoonCaves/rawclaw/internal/index 2.136s
```

The clean ruling is about the code cut, not independent-worker count: `a2761af`
itself is only `FINDINGS.md` (`22` added, no production lines).

`0193241b6ce317ec0c931e6160b8e82b21f48161` is test-only (`57` added in
`internal/index/containers_test.go`), stable patch-id
`2aecad9542d47c189738a422ebccedbe0736a920`, identical to `8824e256`. The
focused race test was observed green in `2.348s`; classify it as useful
coverage, not novel production takeover.

## UNVERIFIABLE

### 5. Six Luna SHAs are present in standalone clone heads, but none is integrated

The six heartbeat SHAs resolve in their live worktrees:

| receipt | immutable head | current integration |
|---|---|---|
| 31-log-contract | `d5d036b9dd94c59a9ee3da2da8fb8d1039cb671d` | not ancestor of `5b9756b` |
| 32-fault-repro-a | `ecf21a76ebe932915323f85e41105c6734fa9c22` | not ancestor |
| 32-fault-repro-b | `cece0a5956fd7692746415ffe67b1db25e093bff` | not ancestor |
| pr35-hooks-audit | `c88bc4664c4050082abfa635ee8b7600107b2e1f` | not ancestor |
| pr35-resolution-audit | `4b32d95e04fc8fc093d9ad1a1445e88a5a780727` | not ancestor |
| pr35-containers-audit | `54bf2b03d3b32bf639924ff0a1f8f6885772eb81` | not ancestor |

The main repository has exact refs for `d5d036b9...`, `cece0a595...`, and
`4b32d95e...`; it has no exact ref for `ecf21a76...`, `c88bc466...`, or
`54bf2b03...` (only separate `candidates/*` refs at different commits).
For the three exact main-repository refs, `git merge-base --is-ancestor
<sha> 5b9756b` returned `1`. The other three hashes are not objects in the
main repository, so their integration status is UNVERIFIABLE rather than a
negative ancestry result. The six standalone clones are checked out at the
receipt heads, but all have the untracked artifacts listed above and no
matching Luna process. This supports “receipts exist and are not integrated”;
it does not support “three live raids”.

### 6. `5610f95`, `c8618ff`, and exact novelty remain branch-local

`5610f95d7d8c9865ed6125e273c9f64989416a67` deletes only
`FINDINGS-OZZY-INTEGRATION.md` (`105` lines) and `FINDINGS-OZZY-REPRO.md`
(`118` lines): `0` production/test lines. It is not an ancestor of `5b9756b`.

`c8618ff0d7f765ce69edd39786a476f586d1fdbb` adds `35` and deletes `3` test
lines in `internal/cli/cmd_tag_onestore_test.go` (net `+32`), with stable
patch-id `3a5c3e70c06636b0b0bf50788662de562bb09e58`; it is also not an
ancestor of `5b9756b`. The current ambiguity-focused gate was green (`2.470s`),
but that proves the test, not integration or a production reduction.

## CLEAN WINS / GATES OBSERVED

- Current catalog-focused agentproto gate: `CGO_ENABLED=0 go test -race -count=1 -run 'Test.*Catalog|Test.*Locate' ./internal/agentproto` — green, `6.041s`.
- Current mixed-source ambiguity-focused CLI gate: `CGO_ENABLED=0 go test -race -count=1 -run 'Test.*Mixed.*Source|Test.*Ambigu' ./internal/cli` — green, `2.470s`.
- Current refresh-cache publish-failure gate: `CGO_ENABLED=0 go test -race -count=1 -run '^TestEnsureFreshContainer_PreservesRefreshDBOnPublishFailure$' ./internal/index` — green, `2.348s`.
- `5b9756b` itself is a committed `1 insertion(+), 7 deletions(-)` catalog shrink with the same stable patch-id as `824014f`.

## Strongest current public ammunition

1. “`d9474fb` is still an existing-FIFO SessionStart hang: the exact claim
   times out with status `124` under both `/bin/sh` and `/bin/dash`.”
2. “The six Luna receipts have `live=0`, but every live worktree is porcelain
   dirty with untracked worker artifacts; the heartbeat's `dirty=0` is false
   under the ordinary `git status --porcelain` check.”
3. “`824014f` is now a real, integrated `+1/-7` catalog shrink; the prior
   dirty/uncommitted criticism is corrected, while `a2761af` remains only a
   22-line Markdown report.”
4. “`5610f95` is `-223` Markdown review artifacts, not production deletion;
   `0193241` is useful but patch-id duplicate test coverage; `c8618ff` is a
   branch-local `+32` test change.”

No broad repository race gate was run for this report.
