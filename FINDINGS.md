# Ponytail Review Findings: internal/scopes and internal/lifecycle

## internal/lifecycle
internal/lifecycle/lifecycle.go:L221-247: stdlib: 26-line LoadTombstones scanner loop with manual buffering. os.ReadFile and strings.Split.
internal/lifecycle/lifecycle.go:L293: delete: redundant sort.Strings(files) after filepath.Glob. Nothing, filepath.Glob returns results lexically sorted.
internal/lifecycle/lifecycle.go:L372: delete: redundant sort.Strings(out) in projectDirs after os.ReadDir. Nothing, os.ReadDir already returns entries sorted by filename.
internal/lifecycle/lifecycle.go:L414-419: shrink: manual strings.Builder loop in appendTombstones. strings.Join(ids, "\n") + "\n".
internal/lifecycle/lifecycle.go:L457-463: shrink: 7-line sessionFileName with if/else. strings.TrimSuffix(filepath.Base(pathOrID), ".jsonl") + ".jsonl".
internal/lifecycle/floor.go:L34-42: shrink: 9-line multi-branch if statements in EvaluateMathFloor. Single boolean expression.

## internal/scopes
internal/scopes/scopes.go:L130: delete: redundant sort.Strings(entries) after filepath.Glob in orphanClaudeScopes. Nothing, filepath.Glob returns sorted results.
internal/scopes/antigravity.go:L20-100, goose.go:L20-100, scopes.go:L201-288: shrink: triplicated ContainerAdapter discover/group/index/orphan loop. Parameterized containerScopes helper in container.go.
internal/scopes/scopes.go:L234-254, antigravity.go:L58-76, goose.go:L58-76: shrink: triplicated Refresh<Source>CWD single-working-dir refresh logic. Shared refreshContainerCWD helper.
internal/scopes/scopes.go:L268-288, antigravity.go:L80-100, goose.go:L80-100: shrink: triplicated orphan<Source>Scopes glob/reconcile loop. Shared orphanContainerScopes helper.
internal/scopes/scopes.go:L371-383, antigravity.go:L102-106, goose.go:L102-106: shrink: triplicated <source>DBPath hash/encode key builders. Shared containerDBPath helper.
internal/scopes/scopes.go:L296-303, antigravity.go:L117-124, goose.go:L117-124: shrink: triplicated <source>OrphanLabel hex-suffix stripping logic. Shared containerOrphanLabel helper.

## Test Clones (Report Only - Retained As-Is per Supervisor Directive)
- internal/scopes/antigravity_test.go:L9-29 vs internal/scopes/scopes_test.go:L51-74: parallel injective/prefix test cases for container DB paths.
- internal/scopes/antigravity_test.go:L48-56 vs internal/scopes/scopes_test.go:L277-283: orphan label test assertions.

## Cross-Package Opportunities (Report Only - Beyond Fence)
- internal/lifecycle/lifecycle.go:L485-500: expandHome is duplicated from internal/paths/paths.go:L501. Left local intentionally to keep internal/lifecycle dependency-free.
- internal/scopes/scopes.go:L173-182: orphanLabel last-non-empty segment rule is duplicated in internal/index/retained.go:L165 to avoid import cycles.
- internal/scopes/scopes_test.go:L26-39: writeCodexRollout helper is duplicated in internal/source/codex and internal/agentproto test files.
- internal/scopes/scopes_test.go:L363-389: testLogRecorder slog handler is duplicated across multiple internal test packages.

net: -390 lines possible.
