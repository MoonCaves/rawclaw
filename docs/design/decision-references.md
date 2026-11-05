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
