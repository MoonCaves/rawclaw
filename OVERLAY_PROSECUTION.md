# Tag overlay prosecution

Verdict: retain `e6f22f1`; reject `cabab43` as the standalone candidate.

## Identity and size

- e6 whole and relevant stable patch ID: `99523502a6ce02afa6116c3efffbc72e1f44e03c`.
- cabab43 whole and relevant stable patch ID: `72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6`.
- `internal/cli/tagrefresh.go`: e6 `+36/-2`; cabab43 `+38/-1`.
- e6 is three production net lines smaller. Its 29-line pure overlay test is separate; cabab43 adds a 41-line end-to-end test and 8-line findings file.

## Decisive mutation

A disposable two-session case used consolidated `(SessionID=other, StartUUID=same)` and authoritative `(SessionID=target, StartUUID=same)`. e6's `SessionID + NUL + StartUUID` key passed, retaining both rows. cabab43's `StartUUID`-only map replaces the other-session row; the equivalent pure-seam test cannot compile because cabab43 exposes no overlay helper. This is a real cross-session identity defect, not a style difference.

Removing either candidate's overlay call kills the committed-tag-before-fold visibility proof (the pre-implementation red proof `9a1b53` fails the exact filter). e6's stable-order mutation passes: existing consolidated rows retain positions, authoritative replacements stay in place, and new rows append in source order. Consolidated-history preservation passes: derived-only rows remain visible.

## Failure behavior

`TopicsForSession` already treats missing/read query errors as an empty non-fatal view. cabab43 adds a second authoritative connection and a returned error path around that API; this is extra production code and can convert an otherwise successful refresh into a command error on connection failure. e6 reuses the existing fail-soft reader and keeps the command path unchanged.

## Gates

- e6: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli` PASS (`70.084s`).
- cabab43 exact worktree: same gate PASS (`98.553s`).
- e6 collision mutation: PASS, filter matched one test (`1.773s`).
- cabab43 collision mutation: FAIL at compile time because no pure overlay seam exists, exposing the candidate's weaker testability.
- `git diff --check` clean for both candidate commits.

External adoption: Han's integration receipt `5eec12b` adopted the composite `(SessionID, StartUUID)` identity after the collision mutation. This validates the material finding, but does not change the standalone winner: e6 is smaller and semantically stronger.
