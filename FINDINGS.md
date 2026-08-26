# Hostile Review & Cross-Examination: Hook Deduplication, Fail-Soft Resilience, Special-File Matrix & Prior Art

**Lane**: `lenny/skill-style-20260826`  
**Target Commits**: `f8fd1fe`, `c88bc4664c40`, `4b32d95e04fc`, `bf7cdd0`, `92d0067`, `54afa70`, `13966cf`, `9b1169a`, `2cc11d6`  
**Inspected Files**:
- `internal/cli/setup.go`
- `internal/cli/catalog_hook_test.go`
- `internal/cli/cmd_ingest_test.go`
- `internal/cli/cmd_tag_onestore_test.go`
- `internal/agentproto/agentproto.go`

---

## 1. Executive Summary & Cross-Examination

### `f8fd1fe` — *SessionStart Ingest Dedup & Hook Helper Unification*
- **Ingest Dedup**: Relocated `nohup "$RAWCLAW" ingest "$session_id"` to execute **after** the catalog claim/creation in both Claude Code (`rawclawPrimeScript`) and Codex (`rawclawCodexPrimeScript`) templates. Previously, detached ingest was launched before checking the per-session catalog marker, spawning duplicate background ingests on repeated starts despite documented dedup contracts.
- **Helper Unification**: Introduced `installRawclawHookWith` and `ejectRawclawHookWith`, consolidating 42 lines of duplicated file stat, directory cascading (`removeIfEmpty`), and config rewrite logic across Claude, Codex, and Antigravity.
- **Dead Return Pruned**: Removed dead `error` return from `addRawclawAntigravityHooks` (`error` -> `void`), simplifying call sites.

### `c88bc4664c40` — *PR35 Hooks Continuation & Test Normalization*
- Restored `TestPrimeScripts_StopLaunchDetachedPrewarm` polling and deadline logic to baseline `c14e806`.
- Maintained `TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest` regression coverage.
- Normalized raw script indentation in `internal/cli/setup.go`, ensuring diffs contain only the atomic ingest relocations and test fixtures.

### `4b32d95e04fc` / `bf7cdd0` — *Guarded Codex Catalog Fallback Resolution*
- Added `TestGuardedSessionLookupDoesNotTreatForeignCatalogPathAsClaude` and `TestGuardedSessionLookupUsesForeignPreResolvedScope` in `internal/cli/cmd_tag_onestore_test.go`.
- Proved that foreign (e.g. Codex) session catalog entries aren't accidentally narrowed into Claude-only scope lookups in `LocateSessionGuarded`, preserving source-aware fallback resolution across heterogeneous runtimes.

---

## 2. Special-File Matrix Attack on `(set -C; : > "$entry")`

### The Special-File Vulnerability Matrix
The naive noclobber redirection claim `(set -C; : > "$entry") 2>/dev/null` was evaluated against a matrix of non-regular filesystem entries across `/bin/sh` and `/bin/dash`:

| Node Type | Behavior under `(set -C; : > "$entry")` | Severity | Root Cause |
|---|---|:---:|---|
| **FIFO (named pipe)** | **HANGS INDEFINITELY** | **FATAL (P0)** | Shell redirection executes `open(path, O_WRONLY)` without `O_NONBLOCK`. In POSIX, opening a FIFO for write blocks in the kernel until a reader connects. Hook hangs on SessionStart, deadlocking agent launch. |
| **UNIX Socket** | Fails with `ENXIO` / `EOPNOTSUPP` | Medium | Shell fails to open socket for writing. Falls through to `elif [ -e "$entry" ]`, exiting 0. |
| **Directory** | Fails with `EISDIR` | Medium | Redirection fails; subshell exits non-zero; falls through to `elif [ -e "$entry" ]`. |
| **Dangling Symlink** | Claims/overwrites target | High | `set -C` sees link target is missing and creates target regular file, mutating foreign paths. |
| **Symlink to FIFO** | **HANGS INDEFINITELY** | **FATAL (P0)** | Follows symlink and blocks on FIFO `open()`. |
| **Missing Parent** | Fails with `ENOENT` | Low | `mkdir -p` failure handled fail-soft. |

### Prior Art & Safe Hard-Link Mechanism (Git `tempfile.c`)
- **Upstream Pattern**: Git `tempfile.c:create_tempfile_mode` (`c44beea485f0f2feaf460e2ac87fdd5608d63cf0`) writes to a PID-isolated sibling opened with `O_CREAT|O_EXCL`, then performs a final atomic link step.
- **RawClaw Adaptation**: Replace open-by-redirection with atomic `ln "$tmp_entry" "$entry" 2>/dev/null`.
- **Inviolable Invariant**: The `link(2)` system call operates strictly on directory entries and inode links; it **never opens the target file for I/O**. Consequently, `ln` cannot hang on FIFOs, sockets, or special devices.

---

## 3. Patch-ID & Range-Diff Attack on Rival Hook Branches

Cross-examination of rival branches (`conor/fix-hook-fifo-claim` vs `norm/flash-hooks` vs `lenny/raid-hooks-20260826`):

```bash
git patch-id --stable:
  9b75b6006c56603528f7ef8e3b2626a638f2c98c  13966cf (Conor: link claim)
  4fd42e86fccc178626d19bd0353aba3a029a93fa  2cc11d6 (Norm: flash hooks)
  0165e364e7b5c8819775f3a1734d3f4b25d6e7a3  9b1169a (Conor: pre-link existence check + test matrix)
```

- **Novelty Deduction**:
  - `2cc11d6` (Norm) directly replicates Conor's `13966cf` template restructuring (`tmp_entry > ln "$tmp_entry" "$entry"`), confirming zero independent mechanism novelty.
  - Conor's `9b1169a` improves on `13966cf` by adding the pre-link existence check `if [ -e "$entry" ] || [ -L "$entry" ]; then exit 0; fi`, preventing unnecessary temporary file creation when the catalog entry already exists.
  - `9b1169a` adds comprehensive test coverage across regular/FIFO/directory/symlink/socket matrix under both `/bin/sh` and `/bin/dash`.

---

## 4. Confirmed Deduction & Smallest Safe Transplant

### Confirmed Deduction
The noclobber redirection `(set -C; : > "$entry")` is provably broken against pre-existing FIFOs and must be permanently discarded in favor of `link(2)`-based atomic claiming.

### Smallest Safe Transplant (Net -4 lines in `setup.go`)
Replace the two-phase empty claim + `mv` in `rawclawPrimeScript` and `rawclawCodexPrimeScript` with direct hard-link claiming:

```sh
tmp_entry="$catalog_dir/.tmp.$session_id.$$"
if {
    printf '{\n'
    printf '  "session_id": "%s",\n' "$esc_session_id"
    printf '  "transcript_path": "%s",\n' "$esc_transcript_path"
    printf '  "cwd": "%s",\n' "$esc_cwd"
    printf '  "source": "claude"\n'
    printf '}\n'
} > "$tmp_entry" 2>/dev/null && ln "$tmp_entry" "$entry" 2>/dev/null; then
    rm -f "$tmp_entry" 2>/dev/null || true
    nohup "$RAWCLAW" ingest "$session_id" </dev/null >/dev/null 2>&1 &
elif [ -e "$entry" ] || [ -L "$entry" ]; then
    rm -f "$tmp_entry" 2>/dev/null || true
    exit 0
else
    rm -f "$tmp_entry" 2>/dev/null || true
    # Fail-soft: if catalog persistence failed due to permissions or I/O, ingest still launches.
    nohup "$RAWCLAW" ingest "$session_id" </dev/null >/dev/null 2>&1 &
fi
```

**Benefits**:
1. Eliminates FIFO hang entirely (zero open on `$entry`).
2. Atomically claims and writes the rich JSON in one step (no separate empty marker file).
3. Preserves fail-soft single-ingest launch on permission errors.
4. Cleans up temporary link files reliably.

---

## 5. Upstream Architecture & Bloat Skills Scorecard

| Skill | Grade | Actionable Deletion Signal | Correctness Awareness | Noise Level | Assessment for this Target |
|---|:---:|:---:|:---:|:---:|---|
| **`modular-refactor` + `right-sizing`** | **A** | High | High | Low | **Best-in-class brake**: Its explicit stopping criterion ("a CLI tool does not need layers of abstraction; don't add a port without 2+ implementations in sight") justifies keeping hook setup flat and procedural. |
| **`golang-code-style`** | **A** | High | High | Low | **High value**: Enforces early returns, unexported-by-default encapsulation, and eliminates unnecessary `else` branches. |
| **`codebase-design`** | **A-** | High | High | Low | **Strong framework**: The "deep module" and "deletion test" concepts validate that `setup.go` provides a compact interface (`RunSetup`, `RunEject`) hiding complex OS/shell interactions. |
| **`ponytail` / `ponytail-review`** | **B+** | High | Medium | Medium | **Good for pruning, risky on shell**: Prunes dead returns and unneeded helpers effectively, but risks stripping critical defensive POSIX fallbacks (`true >`, `2>/dev/null`, permission branches) if applied blindly. |
| **`golang-structs-interfaces`** | **C+** | Low | High | High | **Over-abstraction hazard**: Encourages interface creation, compile-time assertions, and embedding that tempt developers to convert 3 static file templates into an object hierarchy. |
| **`golang-design-patterns`** | **D** | Low | Medium | High | **Severe bloat hazard**: Recommends functional options (`NewInstaller(WithClaude(), WithCodex())`), builders, and lifecycle hooks that would inflate 150 lines of lean Go into 400+ lines of ceremony. |

---

## 6. Metric & Conclusion
- **Net Production Lines Possible**: Net -42 lines across hook installation/ejection deduplication plus -4 lines from the hard-link claim transplant.
- **Go Source Integrity**: Zero Go source edits required; `gofmt` clean across `internal/`.
