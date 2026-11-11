# Session topic miss indexing probe

**Result: a missing session caused per-project indexes to be created in an isolated local fixture.** A global cold lookup created indexes for both fixture projects; an explicitly scoped lookup created only project-a's index. An explicit empty directory created no files in the monitored roots. All five CLI invocations returned exit 1 with `session "deadbeef" not found in scope`.

A separate one-project control also left an appended message unindexed after a consolidated session hit; a subsequent session miss indexed it. These are bounded measurements for issue #103, not a fix or an approved replacement algorithm. Whole-process timings below do not establish production severity or scaling.

## Source and coverage

Production baseline: `61fac14e674095678e47cad202e4a255c381047f`. The measured binary was built with `CGO_ENABLED=0 GOTOOLCHAIN=go1.24.0 go build -o "$BIN" ./cmd/rawclaw` (temporary binary path normalized to `$BIN`) from test-only head `8fd8e5c559c45fd106f7b3080c446cb3863e1eba`; its production and module diff against the baseline was empty. Build exit: 0. Binary SHA-256: `f1340e39163d41ca825648d37023fae09f918b15d88383969f564170c9f45a04`. Measurements ran on Darwin/macOS with `/usr/bin/time -p`.

The [topics command](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/cli/cmd_topics.go#L27-L60) resolves `--session` through [session lookup](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/agentproto/topics.go#L64-L80). [Scope selection](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/cli/cli.go#L400-L418) distinguishes global discovery from an explicit directory. [The locator](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/agentproto/locate.go#L67-L83) returns early for an explicit empty scope; after a consolidated miss it can sweep [candidate scopes](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/agentproto/locate.go#L185-L213). Resolving a [lazy Claude scope](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/scopes/scopes.go#L286-L297) calls `EnsureIndexed`; the [index write path](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/index/index.go#L1187-L1206) can update store state. The locator returns a matching consolidated candidate before resolving fallback scopes. This source trace supports the mechanism, not an observed call count.

Each project in the original two-project experiment contains the same unchanged [existing test record](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/index/index_test.go#L1335) as `sess1.jsonl`:

```json
{"type":"user","uuid":"u1","timestamp":"2026-08-25T00:00:00Z","message":{"role":"user","content":"first version"}}
```

Neither experiment starts with tags or catalog entries. The shared IDs mean consolidation can merge the fixtures; individual project DBs establish the two-project result.

## Reproduction

Run this combined adaptation of the executed recipes from a checkout of the production baseline above. It preserves the original cases and adds a separate one-project append case; the SQLite CLI observers below capture errors, whereas the executed one-project control used Python SQLite read-only observers. It builds a temporary binary; it does not install or configure RawClaw. Requirements: Go 1.24.0, Darwin `/usr/bin/time`, `/usr/bin/sqlite3`, and `shasum`. Run with Bash. Receipts remain under the printed temporary root. Timings and hashes will vary. This combined recipe was syntax-checked and executed once during review: the outer Bash process exited 0 with empty stderr, the original five CLI calls each exited 1, and the unique-source calls exited 1/0/1/0. Individual project SQL reads succeeded; some consolidated snapshot queries returned recorded SQLite exit 14 errors. This does not establish that every SQL observation succeeded.

```sh
probe_root=$(mktemp -d /tmp/rawclaw-topic-miss.XXXXXX)
rawclaw_probe_bin="$probe_root/rawclaw"
CGO_ENABLED=0 GOTOOLCHAIN=go1.24.0 go build -o "$rawclaw_probe_bin" ./cmd/rawclaw || exit
printf '%s\n' "$probe_root"
shasum -a 256 "$rawclaw_probe_bin"

for probe_case in global scoped empty-scope unique-source; do
  case_root="$probe_root/$probe_case"
  mkdir -p "$case_root/home/.cache/session-search" "$case_root/data" \
    "$case_root/config" "$case_root/cache" "$case_root/catalog" \
    "$case_root/claude/projects/project-a" "$case_root/claude/projects/project-b" \
    "$case_root/codex/sessions" "$case_root/antigravity" "$case_root/hermes" \
    "$case_root/opencode" "$case_root/pi" "$case_root/goose" \
    "$case_root/tmp" "$case_root/empty" "$case_root/receipts"
  printf '%s\n' '{"type":"user","uuid":"u1","timestamp":"2026-08-25T00:00:00Z","message":{"role":"user","content":"first version"}}' \
    > "$case_root/claude/projects/project-a/sess1.jsonl"
  if [ "$probe_case" != unique-source ]; then
    cp "$case_root/claude/projects/project-a/sess1.jsonl" \
      "$case_root/claude/projects/project-b/sess1.jsonl"
  fi
done

snapshot_probe() {
  local case_root="$1" snapshot_label="$2"
  find "$case_root/home/.cache/session-search" "$case_root/data" "$case_root/catalog" \
    -type f -exec shasum -a 256 {} \; | sort \
    > "$case_root/receipts/$snapshot_label.files"
  find "$case_root/home/.cache/session-search" -name '*.db' -type f -print |
  while IFS= read -r probe_db; do
    printf '%s\n' "$probe_db"
    /usr/bin/sqlite3 -init /dev/null -readonly "$probe_db" \
      'SELECT id, source_tool, source_path, message_count FROM sessions ORDER BY id; SELECT session_id, uuid, role, content FROM messages ORDER BY id; SELECT path, size, byte_offset FROM file_index ORDER BY path; SELECT key, value FROM meta ORDER BY key;'
    printf 'sqlite_exit=%s\n' "$?"
  done > "$case_root/receipts/$snapshot_label.rows" \
    2> "$case_root/receipts/$snapshot_label.sqlite.stderr"
}

run_probe() {
  local case_root="$1" run_label="$2" session_arg="$3"
  shift 3
  wc -c "$case_root/claude/projects/project-a/sess1.jsonl" \
    > "$case_root/receipts/$run_label.source-bytes"
  snapshot_probe "$case_root" "$run_label.before"
  (
    cd "$case_root" || exit
    set +e
    /usr/bin/time -p /usr/bin/env -i \
      PATH=/usr/bin:/bin LC_ALL=C \
      HOME="$case_root/home" TMPDIR="$case_root/tmp" \
      XDG_DATA_HOME="$case_root/data" XDG_CONFIG_HOME="$case_root/config" \
      XDG_CACHE_HOME="$case_root/cache" RAWCLAW_CATALOG_DIR="$case_root/catalog" \
      RAWCLAW_ARCHIVE=off \
      CLAUDE_CONFIG_DIR="$case_root/claude" CODEX_HOME="$case_root/codex" \
      ANTIGRAVITY_HOME="$case_root/antigravity" HERMES_HOME="$case_root/hermes" \
      OPENCODE_DATA_DIR="$case_root/opencode" PI_CODING_AGENT_DIR="$case_root/pi" \
      GOOSE_HOME="$case_root/goose" GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
      "$rawclaw_probe_bin" --timeout 30s topics --session "$session_arg" --json "$@" \
      > "$case_root/receipts/$run_label.stdout" \
      2> "$case_root/receipts/$run_label.stderr"
    printf '%s\n' "$?" > "$case_root/receipts/$run_label.exit"
  )
  snapshot_probe "$case_root" "$run_label.after"
}

run_probe "$probe_root/global" cold deadbeef
run_probe "$probe_root/global" warm deadbeef
run_probe "$probe_root/scoped" cold deadbeef --this-project --dir "$probe_root/scoped/claude/projects/project-a"
run_probe "$probe_root/scoped" warm deadbeef --this-project --dir "$probe_root/scoped/claude/projects/project-a"
run_probe "$probe_root/empty-scope" cold deadbeef --this-project --dir "$probe_root/empty-scope/empty"

run_probe "$probe_root/unique-source" seed deadbeef
printf '%s\n' '{"type":"user","uuid":"u2","timestamp":"2026-08-25T00:01:00Z","message":{"role":"user","content":"second version"}}' \
  >> "$probe_root/unique-source/claude/projects/project-a/sess1.jsonl"
run_probe "$probe_root/unique-source" hit-after-append sess1
run_probe "$probe_root/unique-source" miss-after-append deadbeef
run_probe "$probe_root/unique-source" hit-after-miss sess1
for probe_case in global scoped empty-scope unique-source; do
  case_root="$probe_root/$probe_case"
  find "$case_root/receipts" -type f -print -exec cat {} \;
done
```

## Observed results

The table summarizes the captured receipts; durations are the literal `real` values from the five timed processes. "Cold" starts with a fresh case home; "warm" repeats against that same home.

| Scope | Run | Exit | real (seconds) | Monitored index evidence |
| --- | --- | --- | --- | --- |
| Global | cold | 1 | 0.08 | Empty before; project-a.db and project-b.db created, fixture rows present |
| Global | warm | 1 | 0.07 | Both project DBs remain; their hashes change |
| Explicit project-a | cold | 1 | 0.05 | Empty before; only project-a.db created, fixture rows present |
| Explicit project-a | warm | 1 | 0.05 | Only project-a.db remains; its hash changes |
| Explicit empty directory | cold | 1 | 0.03 | No files or rows before or after in monitored roots |

All five stdout files were empty. All five stderr files contained this exact line:

```text
session "deadbeef" not found in scope
```

Both nonempty cold runs also emitted this warning. The timestamp is omitted and the temporary prefix is normalized to `$PROBE_ROOT`; `CASE` stands for `global` or `scoped`:

```text
WARN the consolidated store is missing or unreadable, so this lookup falls back to the per-project indexes; run `rawclaw consolidate` to build it path=$PROBE_ROOT/CASE/home/.cache/session-search/consolidated.db err="unable to open database file: out of memory (14)"
```

Initial snapshot queries of `consolidated.db` in the nonempty cases failed with:

```text
Error: in prepare, unable to open database file (14)
```

Individual project DB reads succeeded. Later read-only provenance checks also found each individual DB's `sessions.source_path` pointing at its respective fixture file, message content `first version`, and `file_index` size and byte offset both 115. Darwin resolved the temporary `/tmp` prefix to `/private/tmp`. This establishes fixture ingestion rather than schema-only creation. Later readback using the same `/usr/bin/sqlite3 -init /dev/null -readonly` query succeeded for all five DBs: global consolidated/a/b and scoped consolidated/a. Each returned exit 0, empty stderr, and exactly:

```text
sess1|claude
sess1|u1|user
```

The query, with its temporary database path normalized to `$DB`, was:

```sh
/usr/bin/sqlite3 -init /dev/null -readonly "$DB" \
  'SELECT id, source_tool FROM sessions ORDER BY id; SELECT session_id, uuid, role FROM messages ORDER BY id;'
```

The cause of the initial consolidated read errors remains **OPEN**. Failed reads do not establish empty data. Warm hashes establish changed bytes only; neither hashes nor SQLite sidecars establish re-ingestion or Resolve call counts. Hashes were captured before the SQL observers, whose read-only access can still affect shared-memory bookkeeping.

## Unique-source append control

This separate control used the same production baseline and binary, a fresh isolated home, and only project-a containing `sess1.jsonl`. After a seed miss indexed the first record, one complete unchanged [existing second record](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/index/index_test.go#L1371) was appended:

```json
{"type":"user","uuid":"u2","timestamp":"2026-08-25T00:01:00Z","message":{"role":"user","content":"second version"}}
```

The file grew from 115 to 231 bytes. Before/after snapshots opened both generated DBs with Python `sqlite3.connect(db.as_uri() + '?mode=ro', uri=True)` and queried session source/count, message content, file size/offset, and ingest watermarks. Every snapshot read succeeded; this different observer does not explain the original SQLite CLI errors.

| Call | Session | Exit | real (seconds) | Source bytes | Messages and indexed size/offset in both DBs |
| --- | --- | --- | --- | --- | --- |
| Seed miss | deadbeef | 1 | 0.42 | 115 | u1; 115/115 |
| Hit after append | sess1 | 0 | 0.03 | 231 | u1; 115/115 |
| Miss after append | deadbeef | 1 | 0.06 | 231 | u1,u2; 231/231 |
| Hit after miss | sess1 | 0 | 0.08 | 231 | u1,u2; 231/231 |

Both `consolidated.db` and `project-a.db` consistently attributed `sess1` to the unique project-a source. The message readback before refresh was `u1|first version`; after refresh it also contained `u2|second version`. `last_ingest_time` stayed unchanged across each hit; across the second miss it changed from `1794398689.411167` to `1794398689.61976` in consolidated and from `1794398689.418995` to `1794398689.6226401` in project-a. `last_ingest_catalog_mtime` stayed `1794398688.99563` throughout.

Both misses had empty stdout and the same ordinary not-found line shown above; the seed also emitted the missing/unreadable consolidated warning above with its own temporary path. Both hits emitted exactly:

```json
{
  "query": "",
  "hits": [],
  "note": "no topics tagged for session sess1"
}
```

The hit/miss distinction is established by message and offset readbacks, not that JSON. [TopicsForSession](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/store/topics.go#L161-L168) can treat a query error as no topics; empty-topic output is not proof of a successful topic-table read. No tags were added, and tagged-topic refresh semantics were not tested. This observes selected-source message refresh in one fixture; it does not choose the intended replacement policy or establish internal call counts.

## Initial incomplete-record characterization

A separate two-call characterization, repeated independently in a fresh disposable environment, used the same baseline and binary identified above. It is separate from the combined reproduction recipe and its hit/miss append control.

The sole source was the published synthetic `u1` record, 115 bytes including its newline, in project-a's `sess1.jsonl`. The first fixture contained only its first 60 bytes, a nonempty unterminated JSON fragment with no complete record. The remaining 55 bytes, including the original newline, were appended after the first observation.

Each stage ran the existing `--timeout 30s topics --session deadbeef --json` miss invocation with the reproduction's isolated environment. Python SQLite observers opened both generated DBs with `mode=ro` after each call and read `file_index` path/size/byte_offset, session source/count, and message UUID/content rows. Missing rows were not treated as numeric zero.

Both independent runs observed these values in **both** project-a's DB and `consolidated.db`:

| Stage | Source bytes | Indexed size / byte offset | Session message count | Message rows |
| --- | --- | --- | --- | --- |
| Initial unterminated fragment | 60 | 60 / 60 | 0 | None |
| Exact remainder and newline appended | 115 | 115 / 115 | 1 | `u1`, `first version`, once |

Both CLI calls in each run returned exit 1 with `session "deadbeef" not found in scope`. The initial call also warned `claude: skipped malformed jsonl lines count=1` and emitted the missing/unreadable consolidated-store warning described above. All Python SQL observations succeeded; this does not explain the earlier SQLite CLI observer errors.

The partial-stage offset 60 is an actual persisted row despite zero messages. Completion recovered the single message in both stores, so no data loss was observed in these runs. This characterizes initial indexing and completion; it establishes neither equivalence with later incremental appends nor internal call counts, a defect, or an intended checkpoint policy.

## Confinement and limits

The child environment is allowlisted with `env -i`. [Cache paths](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/store/store.go#L303-L315) use the isolated child `HOME/.cache`, ignoring `XDG_CACHE_HOME`; adapter and data roots are also isolated. `HOME` is supplied only to the child, never reassigned in the invoking shell. [Archive off](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/archive/archive.go#L50-L52) returns before archive config is read. No embedding endpoint is supplied, so [adapter construction](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/adapters/adapters.go#L322-L333) has no configured embedder and [semantic topup](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/semantic/topup.go#L118-L120) returns for a nil embedder. SQL initialization files are disabled. No real user transcripts, stores, or configuration are used.

This is environment confinement, not an OS network sandbox or syscall trace; absence of network syscalls was not measured. The watchdog is 30 seconds. Timings include process startup, lookup, indexing, consolidation, and shutdown, not indexing alone. No benchmark distribution, large-corpus slowdown, exact call counts, or full freshness, retained-session, subagent, archive, or collision coverage is established. The empty-scope result concerns only the monitored cache, data, and catalog roots.
