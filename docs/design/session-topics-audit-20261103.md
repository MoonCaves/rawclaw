# Issue 103 audit: `topics --session`

Audit date: 2026-11-03
Audited checkout: `0afecdfab2321abb49733746f7250e6572d5ec5e`
Issue: [MoonCaves/rawclaw#103](https://github.com/MoonCaves/rawclaw/issues/103)

## Findings

**VERIFIED — the missing direct per-session store path claim is stale.** At the audited revision, `Topics` routes non-empty `TopicsOpts.Session` directly to `topicsFromSession`. That function normalizes the supplied ID/ref, resolves one session through `locateSession`, opens its resolved store read-only, and calls `store.TopicsForSession`. The store query selects segments for that session ordered by row ID. The result retains non-empty labels, start refs, summaries, and store order. See [`topics.go` at the audited revision](https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/topics.go#L44-L99) and [`store/topics.go`](https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/store/topics.go#L158-L192).

The direct path is present in ancestor commit [`3fddfa3`](https://github.com/MoonCaves/rawclaw/commit/3fddfa3e485a1d0efe5b17dcd70954144cfd890f), which introduced `TopicsOpts.Session` and `topicsFromSession`; its parent has no session branch. This is the same core path proposed in issue 103’s remediation item 1. The current implementation does not split topics into strings or query every store to list a located session.

**VERIFIED — the current CLI help says `--session` can “list or filter,” but the direct path does not filter on the positional query.** In the audited implementation, `q` is only copied into `TopicsResult.Query`; `topicsFromSession` does not match against it. A temporary Go overlay fixture with three ordered segments produced:

```text
query=beta limit=1 -> hits=3 topics=[alpha beta gamma]
include-path=^never-matches$ -> hits=3
```

The CLI overlay executed the actual command `rawclaw topics --session sessmulti --limit 1 --include-path '^never-matches$' beta`. It printed all three segments, including `alpha` and `gamma`, while rendering the header `3 topic(s) matching 'beta':`. A second actual command with `--this-project --dir <fixture-project>` also returned all three segments. The fixture additionally indexed an unrelated second project and asserted that a `--this-project` lookup for `sessmulti` there returns `not found in scope`. Together these checks demonstrate that the flag scopes session resolution while query, limit, and include-path are ignored by direct session listing. See [`cmd_topics.go`](https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/cmd_topics.go#L26-L59).

**History boundary:** `d9d94c1` contains a prototype that filtered by topic and summary, but `git merge-base --is-ancestor d9d94c1e4ccd5916b4b7fff7f19e8d68a2977ab3 HEAD` returned exit 1. It is divergent history, not a landed implementation and not proof of a regression. Conversely, `3fddfa3` is an ancestor (merge-base check exit 0) and introduces the direct, unfiltered session branch. The current help/behavior mismatch is verified; why the help says “filter,” and whether filtering should be implemented, remain open contract questions.

**VERIFIED — `--limit` is not applied to session listing.** `Topics` computes a defaulted limit, but the session branch passes it to `topicsFromSession`, where it is unused. The overlay probe above shows that `Limit: 1` still returns the three-segment spine. The flag help describes a maximum for ranked hits “across all projects”; the session mode and its historical feature description promise the whole spine. Therefore the ignored limit is observable, but whether that is a defect is **OPEN**: the all-segments contract reasonably takes precedence in session-list mode unless the CLI documents a separate session cap.

**VERIFIED — session scoping uses the supplied project scope; `--include-path` is bypassed in session mode.** `locateSession` receives the scope and limits candidate projects from that scope. The CLI makes `--this-project` scope explicit before calling `Topics`, so session lookup is project-scoped in that mode. By contrast, `--include-path` is stored in `TopicsOpts` but only applied in `topicsByFanOut` and consolidated global-search filtering; `topicsFromSession` does not consult it. The synthetic fixture passed a scope containing the target project and a regex matching no project, yet still got the session’s three segments. Whether `--include-path` is intended to constrain `--session` is **OPEN** because its help describes projects searched, while the `--session` help describes one-session mode.

**VERIFIED — global topic search has a distinct store/fallback path.** When `Session` is empty, `Topics` first queries the consolidated store. `topicsFromStore` returns “not handled” for an empty result as well as for absent rows or query errors; `Topics` then calls `topicsByFanOut`, which resolves the scope and opens each project index. This may fan out after a valid consolidated-store query found no matching topics. That behavior is outside `topicsFromSession`; the audit did not benchmark its cost or establish that it is unnecessary. Keep it as a separate performance/compatibility question rather than evidence that session listing lacks a direct path. See [`topics.go`](https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/topics.go#L44-L60), [`topicsFromStore`](https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/topics.go#L101-L138), and [`topicsByFanOut`](https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/topics.go#L141-L215).

Session resolution itself first checks the consolidated store, then falls back to sweeping the supplied project scopes if it cannot find a candidate there. That lookup fallback is separate from both the one-store topic listing and global topic-search fan-out. It is a recovery path in `locateSession`, not a loop in the segment read. See [`locate.go`](https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/locate.go#L59-L84).

**VERIFIED — the cited GitHub CLI lines do not support the proposed removal.** At commit [`707514fdaea01d51886a99d2941bd8ad62d84234`](https://github.com/cli/cli/blob/707514fdaea01d51886a99d2941bd8ad62d84234/pkg/cmd/issue/list/list.go#L48-L95), `cli/cli/pkg/cmd/issue/list/list.go` lines 48–95 construct the issue-list command, set `opts.BaseRepo = f.BaseRepo`, validate positive result limit, and reject simultaneous `--author` and `--app`. The cited range contains no `--session` flag, no per-session store lookup, and no fallback policy. `--repo` is an ambient repository selection concept, not evidence for removing RawClaw’s session lookup or fallback behavior. The cited code supports only the generic idea that CLI scope can be supplied by flags; it does not demonstrate the implementation proposed in issue 103.

## Verification receipts

Commands run from the audited worktree:

```text
$ go test ./internal/agentproto -run '^TestTopicsSessionFlag$' -count=1
ok  	github.com/MoonCaves/rawclaw/internal/agentproto	0.859s
```

Temporary overlay command (the overlay test was written under `/tmp`; no checkout files were changed):

```text
$ go test -overlay=/tmp/issue103_topics_overlay.json ./internal/agentproto -run '^TestIssue103SessionOptionsOverlay$' -v
=== RUN   TestIssue103SessionOptionsOverlay
    issue103_topics_overlay_test.go:36: query=beta limit=1 -> hits=3 topics=[alpha beta gamma]
    issue103_topics_overlay_test.go:39: include-path=^never-matches$ -> hits=3
--- PASS: TestIssue103SessionOptionsOverlay (0.04s)
PASS
ok  	github.com/MoonCaves/rawclaw/internal/agentproto	0.858s
```

An API overlay used the same three-message fixture and asserted that `Topics("beta", scope, TopicsOpts{Session: "sessmulti", Limit: 1})` returns the ordered labels `alpha`, `beta`, and `gamma`; it also asserted that a nonmatching `IncludePath` leaves all three hits. The CLI overlay below repeats that fixture through `NewRootCmd` and checks the actual rendered output. It uses only a temporary synthetic transcript, not existing transcript data. `newArchiveHome` and `runCmd` are existing helpers in `internal/cli/cmd_archive_test.go` and `internal/cli/cmd_lifecycle_test.go`.

Complete CLI overlay test source:

```go
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestIssue103TopicsCLIOverlay(t *testing.T) {
	newArchiveHome(t)
	project := filepath.Join(t.TempDir(), "fixture-project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	messages := []struct{ id, body string }{
		{"9f3e1c20-aaaa-bbbb-cccc-000000000001", "one"},
		{"9f3e1c20-aaaa-bbbb-cccc-000000000002", "two"},
		{"9f3e1c20-aaaa-bbbb-cccc-000000000003", "three"},
	}
	var transcript string
	for i, m := range messages {
		transcript += `{"type":"user","uuid":"` + m.id + `","timestamp":"2026-06-01T10:0` + string(rune('0'+i)) + `:00Z","message":{"role":"user","content":"` + m.body + `"}}` + "\n"
	}
	if err := os.WriteFile(filepath.Join(project, "sessmulti.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}
	dbp, _, _, err := index.EnsureIndexed(project, false)
	if err != nil {
		t.Fatal(err)
	}
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatal(err)
	}
	for i, row := range []struct{ id, topic, summary string }{
		{messages[0].id, "alpha", "first"}, {messages[1].id, "beta", "second"}, {messages[2].id, "gamma", "third"},
	} {
		if err := store.UpsertTopicSegment(con, "sessmulti", row.id, "", row.topic, row.summary, float64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := index.SyncConsolidatedFrom(dbp); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other-project")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	otherTranscript := `{"type":"user","uuid":"9f3e1c20-bbbb-bbbb-cccc-000000000001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"other"}}` + "\n"
	if err := os.WriteFile(filepath.Join(other, "other-session.jsonl"), []byte(otherTranscript), 0o644); err != nil {
		t.Fatal(err)
	}
	otherDB, _, _, err := index.EnsureIndexed(other, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.SyncConsolidatedFrom(otherDB); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "topics", "--session", "sessmulti", "--limit", "1", "--include-path", "^never-matches$", "beta")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cli output:\n%s", out)
	if !strings.Contains(out, "3 topic(s) matching 'beta':") {
		t.Errorf("unexpected count/query header: %s", out)
	}
	for _, topic := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(out, "  "+topic+"  ·") {
			t.Errorf("missing %s from CLI output: %s", topic, out)
		}
	}
	scoped, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "topics", "--this-project", "--dir", project, "--session", "sessmulti", "beta", "--limit", "1")
	if err != nil {
		t.Fatal(err)
	}
	for _, topic := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(scoped, "  "+topic+"  ·") {
			t.Errorf("--this-project lost %s: %s", topic, scoped)
		}
	}
	t.Logf("this-project cli output:\n%s", scoped)
	excluded, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "topics", "--this-project", "--dir", other, "--session", "sessmulti")
	if err == nil || !strings.Contains(err.Error(), "not found in scope") {
		t.Errorf("foreign session under --this-project error = %v, output=%s", err, excluded)
	}
}
```

Map it without adding a tracked test file, then run it:

```sh
printf '{"Replace":{"%s/internal/cli/issue103_topics_overlay_test.go":"/tmp/issue103_cli_overlay_test.go"}}\n' "$PWD" > /tmp/issue103_cli_overlay.json
go test -count=1 -overlay=/tmp/issue103_cli_overlay.json ./internal/cli -run '^TestIssue103TopicsCLIOverlay$' -v
```

The source file above was saved at `/tmp/issue103_cli_overlay_test.go` for the run. Its exact output was:

```text
=== RUN   TestIssue103TopicsCLIOverlay
    issue103_topics_overlay_test.go:72: cli output:
        3 topic(s) matching 'beta':

          alpha  ·  fixture-project  ·  read ref=sessmult:9f3e1c20 — first
          beta  ·  fixture-project  ·  read ref=sessmult:9f3e1c20 — second
          gamma  ·  fixture-project  ·  read ref=sessmult:9f3e1c20 — third
    issue103_topics_overlay_test.go:90: this-project cli output:
        3 topic(s) matching 'beta':

          alpha  ·  fixture-project  ·  read ref=sessmult:9f3e1c20 — first
          beta  ·  fixture-project  ·  read ref=sessmult:9f3e1c20 — second
          gamma  ·  fixture-project  ·  read ref=sessmult:9f3e1c20 — third
--- PASS: TestIssue103TopicsCLIOverlay (0.05s)
PASS
ok  	github.com/MoonCaves/rawclaw/internal/cli	0.913s
```

CLI help excerpt from `go run ./cmd/rawclaw topics --help`:

```text
Search ONLY the topic layer — the concept labels a tagging subagent attached to past sessions — and print, per hit, `<topic> · <project> · read ref=<sess8>:<uuid8>` pointing at where that topic begins. Topics are NOT in the default search ranking; reach for this when a normal `rawclaw "query"` is ambiguous. Searches every project by default; --this-project (with --dir) to narrow. Use --session <id> to list or filter topics for a single session.
      --limit int             max topic hits (across all projects, ranked together) (default 8)
      --session string        list or filter topic segments for a single session ID/ref
```

Ancestry check for the history boundary:

```text
$ for c in d9d94c1e4ccd5916b4b7fff7f19e8d68a2977ab3 3fddfa3e485a1d0efe5b17dcd70954144cfd890f 3fddfa3e485a1d0efe5b17dcd70954144cfd890f^; do if git merge-base --is-ancestor "$c" HEAD; then printf '%s ancestor\n' "$c"; else printf '%s not-ancestor\n' "$c"; fi; done
d9d94c1e4ccd5916b4b7fff7f19e8d68a2977ab3 not-ancestor
3fddfa3e485a1d0efe5b17dcd70954144cfd890f ancestor
3fddfa3e485a1d0efe5b17dcd70954144cfd890f^ ancestor
```

No private transcript scan, full suite, code edit, or production test edit was performed. The checkout remained clean while gathering evidence. The only changes in this worktree are this report file.
