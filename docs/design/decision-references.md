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

## Native hook publication boundaries (2026-11-06)

These bounded native hook sources show distinct state and dispatch contracts. None of the inspected chains establishes a complete session-catalog record followed by no-clobber ownership and ownership/error-dependent ingest or fallback.

### Gentle AI Claude review hook

At [`Gentleman-Programming/gentle-ai` revision `3ea08cdfbdce671053c82377a4365915799abe57`](https://github.com/Gentleman-Programming/gentle-ai/tree/3ea08cdfbdce671053c82377a4365915799abe57), [`internal/components/agenthooks/claude.go` lines 46–56](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/internal/components/agenthooks/claude.go) configures a Claude SessionStart/Stop command. The inspected handler at [`internal/cli/review_stop_hook.go` lines 33–48, 117–160, 331–350, 358–384](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/internal/cli/review_stop_hook.go) parses typed hook input but persists review-baseline/reminder identities. It reads and updates the existing reminder record, treats corrupt JSON as missing, and writes through [`internal/components/filemerge/writer.go` lines 29–33, 92–143, 252–304](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/internal/components/filemerge/writer.go), whose publication replaces the destination by `os.Rename`. This is mutable review state, not complete session metadata with no-clobber ownership or dependent ingest.

- [`go.mod` lines 1–3](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/go.mod) declares Go 1.25.10. License: [MIT, `LICENSE`](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/LICENSE), Copyright (c) 2025 Gentleman Programming.

### Engram Claude host-session binding

At [`Gentleman-Programming/engram` revision `2c71ca11bc198eb5cf52a12d6a44032c616d3b54`](https://github.com/Gentleman-Programming/engram/tree/2c71ca11bc198eb5cf52a12d6a44032c616d3b54), [`plugin/claude-code/hooks/hooks.json` lines 29–38](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/plugin/claude-code/hooks/hooks.json) registers SessionStart binding. The handler reads the hook session ID and calls `hostbind.Publish`; the caller discards publish errors ([`cmd/engram/hook_bind.go` lines 33–71](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/cmd/engram/hook_bind.go)). [`internal/hostbind/hostbind.go` lines 33–43, 45–81](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/internal/hostbind/hostbind.go) stores PID/start/session_id/updated_at in a PID-keyed JSON file and publishes by rename replacement. [`internal/hostbind/hostbind_test.go` lines 34–45](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/internal/hostbind/hostbind_test.go) explicitly expects sess-b to replace sess-a for the same PID. This is a mutable host-session locator, not complete catalog metadata with ownership-dependent ingest.

- [`go.mod` lines 1–3](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/go.mod) declares Go 1.26.10. License: [MIT, `LICENSE`](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/LICENSE), Copyright (c) 2026 Alan Buscaglia.

### Engram Codex Windows UserPromptSubmit

At the same Engram revision, [`plugin/codex/hooks/hooks.json` lines 41–49](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/plugin/codex/hooks/hooks.json) defines the UserPromptSubmit hook with a Windows PowerShell dispatcher. Its encoded command resolves to `scripts\run-native-hook.ps1`; [`plugin/codex/scripts/run-native-hook.ps1` lines 8–41](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/plugin/codex/scripts/run-native-hook.ps1) reads the first-line `engram-windows-hook-command-v1` marker from Codex `config.toml`, validates the rooted executable path, and invokes `hook codex-user-prompt-submit`. [`cmd/engram/main.go` lines 920–922](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/cmd/engram/main.go) and [`cmd/engram/hook_claude.go` lines 75–77](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/cmd/engram/hook_claude.go) dispatch to the handler.

In [`cmd/engram/hook_codex.go` lines 24–28, 53–87, 366–380](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/cmd/engram/hook_codex.go), the first-prompt helper creates and closes an empty `O_CREATE|O_EXCL` marker; the result selects ToolSearch fallback. With valid project authority, resolved runtime identity, and nonempty prompt/session ID, the handler POSTs to `/prompts` independently of whether the marker is new or already exists. The marker is not completed catalog JSON and its result does not dispatch catalog ingest. The source test at [`cmd/engram/hook_codex_test.go` lines 547–580](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/cmd/engram/hook_codex_test.go) asserts two prompt posts across two handler calls; this test was inspected, not run.

RawClaw's comparator is its shell hook publication and ownership dispatch in [`internal/cli/setup.go` lines 26–61](https://github.com/MoonCaves/rawclaw/blob/efb62d1687ab409002f14b463a327720735905f4/internal/cli/setup.go#L26-L61): a completed JSON entry is hard-linked into place and the claimed/existing/absent cases select ingest or exit. The generic [`paths.WriteCatalogEntry` in `internal/paths/paths.go` lines 122–162](https://github.com/MoonCaves/rawclaw/blob/efb62d1687ab409002f14b463a327720735905f4/internal/paths/paths.go#L122-L162) also publishes by rename, so the comparison is scoped to the shell-hook ownership contract, not every catalog helper. The Codex finding above is limited to the inspected Windows UserPromptSubmit route; it makes no claim about all Codex paths or the absence of another donor.

The two upstream module declarations do not establish incompatibility of separable standard-library code with Go 1.24. The source record makes no implementation, runtime, approval, or closure claim.

#### Full-file SHA256 ledger

All hashes below are for complete files fetched from the immutable revisions cited above. Retain the linked MIT license notice if a substantial portion is copied.

| Repository | File | SHA256 |
| --- | --- | --- |
| MoonCaves/rawclaw | `internal/cli/setup.go` | `88057095f725f6511837122a955b00af4b49657e1cf77e3953fe73750e3a9a8a` |
| MoonCaves/rawclaw | `internal/paths/paths.go` | `4ec65e2aed2311fa489b28fa3b89b6efb61d29e3a9ae0c2f77511657c772d133` |
| gentle-ai | `internal/components/agenthooks/claude.go` | `d6ac1ca2b667e2d353f85962f37c30da1b20678a9c419caac59f86b58eaf872d` |
| gentle-ai | `internal/cli/review_stop_hook.go` | `d15fd767b349b5683acfe767fb66348f306b4c497c8ce0219ad2185cb9bd7ac3` |
| gentle-ai | `internal/components/filemerge/writer.go` | `7940af9374b8a2bd9ea0d1eca519acafe98fe302cc4f5dbf54e7a8a75d7b02bd` |
| gentle-ai | [`go.mod`](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/go.mod) | `4ec66689d196bf08d2dbbee4c8dcc3b1367b9a96a668ec4b1cfd18099b813574` |
| gentle-ai | [`LICENSE`](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/LICENSE) | `2806e215dd306b614fe3211e24186652c7d56c1a57340537fc8f2f6822bc74f5` |
| engram | `plugin/claude-code/hooks/hooks.json` | `4dd00b7d9f6f841dc0b05ad3411fdb12f7c005a1cc343ce08dc92e2aefdaeb04` |
| engram | `cmd/engram/hook_bind.go` | `02d552d7206fc61f856ca156b0ce2948de7fdc43255e09904617cc2453ce31d4` |
| engram | `internal/hostbind/hostbind.go` | `eb37f4d7858c9bfa468c1c9227145d9bcb378e6b8f6123f3d6779b372c639afd` |
| engram | `internal/hostbind/hostbind_test.go` | `b5f898116022cabfe2a88ca29ed3b99db79fa4bf12f08a1dd9adcce40847890f` |
| engram | `plugin/codex/hooks/hooks.json` | `463cb77fc10111f6fc8ed7955750db3e407c2c26428aaa3daa504fdf5cb132c5` |
| engram | `plugin/codex/scripts/run-native-hook.ps1` | `7504826bae4037230e4c83fa1b25170fe5442adc4d83859c04d4b07cac82517f` |
| engram | `cmd/engram/main.go` | `16882536e73e146a80dae5bcd8f340e15c35300257ea1fccfa4d5d5953a1dc68` |
| engram | `cmd/engram/hook_claude.go` | `0f94f8dbe0c808f21f983d4ead11f23f5ab5b017b1c97e1f94f0bdd059ae7b88` |
| engram | `cmd/engram/hook_codex.go` | `2fe369cc1137f00176dda2424f9bc70918a8a172e9be3f8f8f1e9bfe1d86e5f2` |
| engram | `cmd/engram/hook_codex_test.go` | `2bd02eac0dc7ee4d827b6aa70ecfa5ce19be884d3b0898b9fb4e4fecfeed5c03` |
| engram | [`go.mod`](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/go.mod) | `4a3ad075945ecd6bc1caf89e79b0469eca11707865adb0e714cacbc193949f9c` |
| engram | [`LICENSE`](https://github.com/Gentleman-Programming/engram/blob/2c71ca11bc198eb5cf52a12d6a44032c616d3b54/LICENSE) | `09608597ddda4e5f9033ac407a0d401986d96376c47f6d46789ca38db672dc15` |

## Explicit native Claude session forks

Source: `openclaw/openclaw` at immutable revision `6d7347b2da3f3ea44a85ac6fd7f3c15d9ab2caf8`; the linked upstream files own the decisions below.

- [`extensions/anthropic/cli-backend.ts` lines 254–257](https://github.com/openclaw/openclaw/blob/6d7347b2da3f3ea44a85ac6fd7f3c15d9ab2caf8/extensions/anthropic/cli-backend.ts#L254-L257) configures Claude's resume arguments and `forkArg: "--fork-session"`.
- [`src/agents/cli-runner/helpers.ts` lines 421–425](https://github.com/openclaw/openclaw/blob/6d7347b2da3f3ea44a85ac6fd7f3c15d9ab2caf8/src/agents/cli-runner/helpers.ts#L421-L425) checks explicit resume plus fork intent, rejects a backend without a fork argument, and appends that argument.
- [`src/agents/cli-runner/helpers.system-prompt-resume.test.ts` lines 53–69](https://github.com/openclaw/openclaw/blob/6d7347b2da3f3ea44a85ac6fd7f3c15d9ab2caf8/src/agents/cli-runner/helpers.system-prompt-resume.test.ts#L53-L69) tests marked fork versus subsequent ordinary resume. The adaptation selects session-fork assertions and excludes checkpoint behavior.
- [The same test file, lines 75–77](https://github.com/openclaw/openclaw/blob/6d7347b2da3f3ea44a85ac6fd7f3c15d9ab2caf8/src/agents/cli-runner/helpers.system-prompt-resume.test.ts#L75-L77) asserts the unsupported-fork error; checkpoint-only lines 70–74 are excluded.
- License: [MIT, `LICENSE` lines 1–21](https://github.com/openclaw/openclaw/blob/6d7347b2da3f3ea44a85ac6fd7f3c15d9ab2caf8/LICENSE#L1-L21), Copyright (c) 2026 OpenClaw Foundation. The full notice is retained in `internal/source/NOTICE.openclaw`.

Production source and license were fetched and independently checked on 2026-11-06. Full-file SHA256: backend `82c6719a27aa8fddfbfdccfee4071ca0cdf61c783ee6bdfedc04647e05525f78`; helper `abcfe709bc13adb10aa8fce399c339ef8819ec654fabf566ae11a893ec7aca74`; license `9efd316ecf1c4c60f6fd5d26433142fff2b6794d7d328e9bc6179b29bf9c82a4`.

The Go port specializes `useResume=true` in a resume-only API and keeps RawClaw's existing argv builder and shell renderer. Only a known Claude source receives the configured fork capability; the ordinary unknown-source fallback does not confer it. CLI changes pass the explicit option and errors through existing selected local and foreign matches, preserving ambiguity, metadata guards, source-filter rejection and archive origin guidance. RawClaw renders a command; this source record does not establish native execution or creation of a new session ID. No OpenClaw package, checkpoint feature, transcript copying or new discovery policy is adopted.
