# Ozzy cdc063d scoped catalog ambiguity reproduction — Wave 3

## Verdict

REPRODUCED. HOLD cdc063d for scoped lookup: the guarded catalog fast path can
choose a Claude database despite an explicitly pre-resolved Codex scope. The
result is a wrong-source resolution, not an honest ambiguity.

## Exact trigger

At `internal/agentproto/agentproto.go:1769-1790`,
`locateSessionWithCatalog` invokes `catalogCands` before the scope sweep when
`guarded` is true. At `internal/agentproto/agentproto.go:1798-1823`,
`catalogCands` narrows catalog hits only by `scopeProjects`' project labels
(`:1798-1803`), then reconstructs every accepted hit as a Claude `TDir`
(`:1810-1818`). It ignores `view.Scope.Source`, `DBP`, and `CWD`.

The hostile fixture used one identical session id in Claude, Codex, and
Antigravity transcript files. The Claude catalog entry recorded cwd
`/home/user/shared`; the lookup scope was explicitly pre-resolved Codex with
`Project: "shared"`, `Source: "codex"`, `CWD: "/home/other/codex"`, and `DBP`
set to the Codex index. The consolidated store was absent, forcing the
catalog/fallback path.

## Observed result

The unguarded control (`locateSession`) respected the pre-resolved Codex DB.
The guarded path (`locateSessionGuarded`) returned the Claude DB
(`.../-home-user-shared.db`) for the same 8-character prefix instead of the
pre-resolved Codex DB (`.../004.db`). It returned the same full session id, so
this is silent source misrouting rather than a not-found or ambiguity error.

Focused hostile test (temporary test file only; never committed):

```text
CGO_ENABLED=0 go test -race -count=3 ./internal/agentproto -run TestReproScopedCatalogIgnoresSource
ok   github.com/MoonCaves/rawclaw/internal/agentproto  2.329s
wall: 3.740s
```

The test was run in a disposable detached worktree at cdc063d. Ozzy's worktree
was not edited. The three-source transcript setup and the unguarded control
were repeated three times under the race detector.

## Why cdc063d does not cover this

The changed branch at `internal/agentproto/agentproto.go:1811-1817` returns
`nil` only when an accepted catalog hit lies outside Claude's projects tree.
Here the accepted hit is inside Claude's tree, and the foreign source is
already excluded by the label filter. Therefore the new `return nil` branch
never runs; the catalog remains a one-hit Claude scope and wins before the
explicit Codex scope can be used.

## Net lines

cdc063d's production hunk is 3 added / 2 removed lines: net +1 line. The
reproduction test was disposable and is not part of this report commit.
