# Issue #85 result

**Worker:** Luna  
**Branch:** `khan/fix-issue85-catalog-hooks`  
**Base:** `98b6b2bc4341fe6eb72ea6399beb3731e9d3731d`  
**Implementation commit:** `22e5dd2`

## Result

Implemented session-birth catalog registration for detected Pi, Goose, and
OpenCode installations through their native extension/plugin seams.

- Pi: installs `~/.pi/agent/extensions/rawclaw-catalog.ts` and handles
  `session_start`.
- Goose: installs an Open Plugins plugin under `~/.agents/plugins/rawclaw/` and
  handles `SessionStart`.
- OpenCode: installs `~/.config/opencode/plugins/rawclaw.js` and handles
  `session.created`.
- `rawclaw setup --eject` removes these artifacts; existing Claude, Codex, and
  Antigravity wiring remains unchanged.
- Each hook writes a catalog claim and starts detached ingest only after a new
  claim is created, advancing the catalog directory mtime at birth.

## Transplant-first gate

The existing Claude/Codex catalog lifecycle in `internal/cli/setup.go` was the
transplant candidate. Its contract was reused: validate flat session IDs,
write a durable catalog claim, deduplicate an existing claim, and launch
best-effort detached ingest. The exact Claude/Codex shell template could not be
copied byte-for-byte because Pi and OpenCode load TypeScript/JavaScript modules
and expose lifecycle data through module APIs rather than JSON stdin. Goose
uses the Open Plugins nested `hooks/hooks.json` schema. Those are the specific
runtime API reasons for the small adapters.

Goose's SessionStart payload does not expose a transcript filename. Its hook
records the stable `sessions.db#<session_id>` URI instead; `session_id`, `cwd`,
and `source` are still recorded.

## Verification receipts

- `CGO_ENABLED=0 go build ./...` — exit 0.
- `go vet ./...` — exit 0.
- `CGO_ENABLED=0 go test -race -count=1 ./...` — exit 0.
  - CLI: 130.381s; index: 112.222s; all packages passed.
- `golangci-lint run` — not run: executable unavailable in this worktree.
- `graphify update .` — completed; 3718 nodes and 11206 edges rebuilt.

The worktree also contained pre-existing harness files `.launch.log` and
`PROMPT.md`; they were not staged or modified.
