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
