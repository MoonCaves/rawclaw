# Decision references

Source: created here as a catalog of verified upstream material; the linked immutable files own the patterns.

## Temporary HOME and XDG fixture isolation

- Repository: `cisco-ai-defense/defenseclaw`.
- Immutable revision: `6b6888d75e09c1134e0dfd3a874bec9b318cac42`.
- Pattern: [`internal/testenv/home.go`, lines 27–34](https://github.com/cisco-ai-defense/defenseclaw/blob/6b6888d75e09c1134e0dfd3a874bec9b318cac42/internal/testenv/home.go#L27-L34).
- Temporary caller: [`internal/config/bundled_mcp_test.go`, lines 32–34](https://github.com/cisco-ai-defense/defenseclaw/blob/6b6888d75e09c1134e0dfd3a874bec9b318cac42/internal/config/bundled_mcp_test.go#L32-L34).
- License: [Apache-2.0](https://github.com/cisco-ai-defense/defenseclaw/blob/6b6888d75e09c1134e0dfd3a874bec9b318cac42/LICENSE); source header credits Cisco Systems, Inc. and its affiliates, 2026.

The upstream helper redirects HOME and durable XDG data to the same fixture home:

```go
t.Setenv("HOME", home)
t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
```

The caller supplies `home := t.TempDir()`. The same helper bytes are present at introduction commit `1060b05d3c0d37fcf4e9c98435ee2ac5a4f5007c` (2026-07-16) and the revision above. Source and license were fetched and independently checked on 2026-11-05.

This source was discovered after the initial local fixture correction. The subsequent provenance annotation records adoption of the verified upstream pattern now; it does not claim the original correction was copied from this source.

## Archive and scopes fixture subset

The 2026-11-05 extraction applies only the HOME-relative XDG data assignment and its temporary HOME binding to existing fixtures in `internal/archive/archive_test.go`, `internal/archive/archivetest/archivetest.go`, and `internal/scopes/scopes_test.go`. The copied assignment is literal from `internal/testenv/home.go` line 34; the caller's `home := t.TempDir()` binding is literal from `internal/config/bundled_mcp_test.go` line 33. Each destination records its own immutable copy-site citation and `Prior-Art` trailer. Supervisor ACKed this exact nine-statement scope after independently matching the pinned sources and license.

This is a narrow archive/scopes fixture adoption. It does not cover the 47 direct `XDG_DATA_HOME` `t.TempDir()` assignments in other fixture files, or the contract files and contract edits excluded from the PR114 extraction. It does not establish whole-suite XDG isolation. The upstream source was discovered after the initial local fixture diagnosis; the citations document this later adoption and do not change that chronology.

## Direct temporary XDG data fixtures

- Exact statement: `t.Setenv("XDG_DATA_HOME", t.TempDir())`.
- Immutable source: [`dundee/gdu` `pkg/remove/trash_error_test.go`, lines 116–116](https://github.com/dundee/gdu/blob/4361b09ccc5850413bfec35bb2b904d96d2e5cb7/pkg/remove/trash_error_test.go#L116-L116), in a concrete `*testing.T` test.
- License: [MIT](https://github.com/dundee/gdu/blob/4361b09ccc5850413bfec35bb2b904d96d2e5cb7/LICENSE.md), Copyright 2020-2021 Daniel Milde.
- Source SHA256: `5364b655973b1cc64ba6c890c35358a84810315d31682309cf658badbb13fc1b`; license SHA256: `613e9144d1403bba9553d64daf6a5e28d4c6bd9fabed81da80efaf91eb573dad`.

The source and license were fetched and independently checked on 2026-11-05. This records prospective adoption now, not provenance of the earlier held correction. Scope is only 46 direct fixture statements in 11 existing test files under `internal/agentproto` and `internal/view`, each immediately after an existing HOME assignment. Contract files, contract rows, assertions, imports, system messages, and the separate Cisco archive/scopes fixtures are excluded. Each copied statement carries an immutable copy-site citation and each target file its own `Prior-Art` trailer.

The isolated statement uses standard-library APIs available at RawClaw's Go 1.24 floor; no gdu dependency or its module's Go 1.26 floor is adopted. Existing fixtures are nonparallel in the inspected source. This narrow adoption does not establish whole-suite XDG isolation.

## Claude resume executable selection and shell argument quotation

Source: `git/git` at immutable revision `b8242b093d9e941a34460d715e3ce616a34ac3fe`; the linked upstream files own these patterns.

- [`connect.c` lines 1406–1408](https://github.com/git/git/blob/b8242b093d9e941a34460d715e3ce616a34ac3fe/connect.c#L1406-L1408): select an environment-provided executable, defaulting only when the variable is absent.
- [`quote.c` lines 11–48](https://github.com/git/git/blob/b8242b093d9e941a34460d715e3ce616a34ac3fe/quote.c#L11-L48): single-quote a shell word and escape embedded single quotes and exclamation points.
- [`quote.c` lines 85–95](https://github.com/git/git/blob/b8242b093d9e941a34460d715e3ce616a34ac3fe/quote.c#L85-L95): quote every argument before composing the command.
- License: [GPL-2.0, `COPYING`](https://github.com/git/git/blob/b8242b093d9e941a34460d715e3ce616a34ac3fe/COPYING). Source and license fetched on 2026-11-05.

Applicability is limited to PR108's `claudeExecutable`, `shellQuoteExecutable`, and whole-vector quotation in `ResumeCommand`. The Go adaptation uses `os.LookupEnv` with `CLAUDE_CLI_NAME`/`claude`, immutable strings and `strings.NewReplacer`, and a slice loop plus `strings.Join`; it does not reproduce Git's C buffer/alias handling. This is a prospective source record for extraction onto current main; it does not establish new provenance for the earlier commits, cover unrelated parser or claim logic, or grant general permission to reuse Git. License recording establishes neither incompatibility nor legal clearance.

## Hermes native resume CLI

Source: `NousResearch/hermes-agent` at immutable revision `d8c79e4b6b8c63e6c40167ff95ec28e7b1fee189`; the linked upstream files own this CLI contract.

- [`hermes_cli/_parser.py` lines 179–181](https://github.com/NousResearch/hermes-agent/blob/d8c79e4b6b8c63e6c40167ff95ec28e7b1fee189/hermes_cli/_parser.py#L179-L181): defines `--resume` / `-r` with a session argument.
- [`pyproject.toml` lines 586–587](https://github.com/NousResearch/hermes-agent/blob/d8c79e4b6b8c63e6c40167ff95ec28e7b1fee189/pyproject.toml#L586-L587): declares the `hermes` executable at `hermes_cli.main:main`.
- [`hermes_cli/main.py` lines 3021–3025](https://github.com/NousResearch/hermes-agent/blob/d8c79e4b6b8c63e6c40167ff95ec28e7b1fee189/hermes_cli/main.py#L3021-L3025): promotes top-level resume/continue without a subcommand to chat.
- License: [MIT, `LICENSE`](https://github.com/NousResearch/hermes-agent/blob/d8c79e4b6b8c63e6c40167ff95ec28e7b1fee189/LICENSE), Copyright (c) 2025 Nous Research. Source and license fetched on 2026-11-05.

This records the native `hermes --resume SESSION` interface for prospective adoption in RawClaw's existing resume command table. It does not cover session discovery, ID/prefix resolution, archive source support, shell quotation, executable overrides, or the separate `hermes resume` alias. It does not claim that earlier RawClaw changes were copied from this source, retroactively approve the held candidate, or grant implementation or provenance-gate approval.

## Existing RawClaw resume assertion sources

Source: `MoonCaves/rawclaw` at immutable revision `2e5b6573555d3fc01aa844fe5ecc88e4b90cf334`; the linked immutable files own these existing historical test patterns.

- [`internal/cli/tagrefresh_test.go` lines 121–128](https://github.com/MoonCaves/rawclaw/blob/2e5b6573555d3fc01aa844fe5ecc88e4b90cf334/internal/cli/tagrefresh_test.go#L121-L128) and [138–141](https://github.com/MoonCaves/rawclaw/blob/2e5b6573555d3fc01aa844fe5ecc88e4b90cf334/internal/cli/tagrefresh_test.go#L138-L141): authoritative metadata fixture and resume invocation; [145–147](https://github.com/MoonCaves/rawclaw/blob/2e5b6573555d3fc01aa844fe5ecc88e4b90cf334/internal/cli/tagrefresh_test.go#L145-L147): command-substring assertion.
- [`internal/cli/cmd_root_ux_test.go` lines 117–129](https://github.com/MoonCaves/rawclaw/blob/2e5b6573555d3fc01aa844fe5ecc88e4b90cf334/internal/cli/cmd_root_ux_test.go#L117-L129): anonymous JSON result, decode and scalar string-field equality.
- [`internal/source/resume_executable_test.go` lines 35–59](https://github.com/MoonCaves/rawclaw/blob/2e5b6573555d3fc01aa844fe5ecc88e4b90cf334/internal/source/resume_executable_test.go#L35-L59): hostile argv fixtures and command equality; [61–79](https://github.com/MoonCaves/rawclaw/blob/2e5b6573555d3fc01aa844fe5ecc88e4b90cf334/internal/source/resume_executable_test.go#L61-L79): per-source argv equality.
- [`internal/source/source_test.go` lines 18–42](https://github.com/MoonCaves/rawclaw/blob/2e5b6573555d3fc01aa844fe5ecc88e4b90cf334/internal/source/source_test.go#L18-L42) and [47–59](https://github.com/MoonCaves/rawclaw/blob/2e5b6573555d3fc01aa844fe5ecc88e4b90cf334/internal/source/source_test.go#L47-L59): hostile CWD fixtures and per-source command equality.
- License: [MIT, `LICENSE` lines 1–21](https://github.com/MoonCaves/rawclaw/blob/2e5b6573555d3fc01aa844fe5ecc88e4b90cf334/LICENSE#L1-L21), Copyright (c) 2026 MoonCaves. Complete source and license bytes fetched on 2026-11-05.

This records historical assertion patterns for prospective reuse, not independent evidence of native Hermes semantics. A prospective adaptation must identify exact copied blocks and explain source/session/CWD fixture substitutions, JSON options and the decoded `Scope` to `Command` field substitution at each copy site. Use separate text and JSON cases, a complete literal command expectation and a command-substring check without adding text-frame expectations. Do not import the unrelated adapter-Lookup assertion or its registration.

RawClaw's existing generic `ResumeCommand` quotes each source's argv without a Hermes-specific quoting branch. The Hermes native CLI source above covers its executable/option contract, not quotation; the existing shell-quotation source retains its stated scope. Any adoption is limited to fixture/field wiring through existing generic behavior; it must not introduce quoting or runtime rules. These sources establish no native arbitrary-ID acceptance. Exact adaptations still require source-before-edit ACK, three-way review and the ordinary provenance gate. This entry neither establishes provenance for PR131 or its held donor, reverses their HOLD, authorizes implementation, nor grants general same-repository source eligibility.

## Native hook source boundaries: Gentle AI and Engram (2026-11-05)

These pinned native hook chains provide useful comparisons for session metadata and publication semantics. Neither inspected chain establishes the full RawClaw sequence of hook payload → complete session metadata JSON → no-clobber catalog claim → ownership/error-dependent ingest or fallback.

### Gentle AI native Claude review hook

- Repository: `Gentleman-Programming/gentle-ai`, immutable revision [`3ea08cdfbdce671053c82377a4365915799abe57`](https://github.com/Gentleman-Programming/gentle-ai/tree/3ea08cdfbdce671053c82377a4365915799abe57).
- [`internal/cli/review_stop_hook.go`, lines 33–48, 117–140, 148–160, 331–350, 358–384](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/internal/cli/review_stop_hook.go) parses typed Claude hook input and persists review-baseline/reminder state. That state is mutable: the writer reads and updates an existing record, treats corrupt JSON as missing, and calls `filemerge.WriteFileAtomic`.
- [`internal/components/filemerge/writer.go`, lines 29–33, 252–304](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/internal/components/filemerge/writer.go) publishes changed bytes by `os.Rename` replacement. This does not establish no-clobber ownership. The inspected chain does not project complete session catalog metadata or connect publication ownership/errors to ingest or fallback.
- Source SHA256: `review_stop_hook.go` `d15fd767b349b5683acfe767fb66348f306b4c497c8ce0219ad2185cb9bd7ac3`; `writer.go` `7940af9374b8a2bd9ea0d1eca519acafe98fe302cc4f5dbf54e7a8a75d7b02bd`.
- License: [MIT, `LICENSE`](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/LICENSE), Copyright (c) 2025 Gentleman Programming. License SHA256: `2806e215dd306b614fe3211e24186652c7d56c1a57340537fc8f2f6822bc74f5`.
- `go.mod` declares Go 1.25.10. That module declaration does not establish that a separable standard-library snippet is incompatible with Go 1.24.

### Engram native Claude host-session binding

- Repository: `Gentleman-Programming/engram`, immutable revision [`2c71ca11bc198eb5cf52a12d6a44032c616d3b54`](https://github.com/Gentleman-Programming/engram/tree/2c71ca11bc198eb5cf52a12d6a44032c616d3b54).
- [`plugin/claude-code/hooks/hooks.json`, lines 29–38](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/plugin/claude-code/hooks/hooks.json) registers the native SessionStart binding hook. [`cmd/engram/hook_bind.go`, lines 54–70](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/cmd/engram/hook_bind.go) reads hook JSON and publishes a host binding; [`cmd/engram/hook_claude.go`, lines 583–592](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/cmd/engram/hook_claude.go) validates the Claude session ID.
- [`internal/hostbind/hostbind.go`, lines 33–43, 65–81](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/internal/hostbind/hostbind.go) stores `{pid,start,session_id,updated_at}` in a PID-keyed file and publishes by rename replacement. [`internal/hostbind/hostbind_test.go`, lines 34–45](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/internal/hostbind/hostbind_test.go) explicitly expects a later `sess-b` binding to replace `sess-a` for the same PID. The Claude caller discards publication errors. This is a mutable host-session locator; it does not provide complete catalog metadata or ownership/error-dependent ingest/fallback.
- Source SHA256: `hostbind.go` `eb37f4d7858c9bfa468c1c9227145d9bcb378e6b8f6123f3d6779b372c639afd`; `hostbind_test.go` `b5f898116022cabfe2a88ca29ed3b99db79fa4bf12f08a1dd9adcce40847890f`; `hook_bind.go` `02d552d7206fc61f856ca156b0ce2948de7fdc43255e09904617cc2453ce31d4`; `hook_claude.go` `0f94f8dbe0c808f21f983d4ead11f23f5ab5b017b1c97e1f94f0bdd059ae7b88`; `hooks.json` `4dd00b7d9f6f841dc0b05ad3411fdb12f7c005a1cc343ce08dc92e2aefdaeb04`.
- License: [MIT, `LICENSE`](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/LICENSE), Copyright (c) 2026 Alan Buscaglia. License SHA256: `09608597ddda4e5f9033ac407a0d401986d96376c47f6d46789ca38db672dc15`.
- `go.mod` declares Go 1.26.10. That module declaration does not establish that a separable standard-library snippet is incompatible with Go 1.24.

The generic `paths.WriteCatalogEntry` helper also uses rename; the relevant RawClaw no-clobber comparison is the shell hook's first-writer publication and ownership-driven ingest/fallback, not every catalog helper. Codex-wide source suitability remains OPEN: the inspected first-prompt `O_EXCL` marker and HTTP lifecycle snippets do not establish a complete local catalog chain for every Codex path. These bounded comparisons do not establish that no suitable donor exists, a runtime fix or absence, approval, or closure.
