# Hostile Review & Cross-Examination: Hook Deduplication, Fail-Soft Resilience, and Upstream Architecture Skills

**Lane**: `lenny/skill-style-20260826`  
**Target Commits**: `f8fd1fe`, `c88bc4664c40`, `4b32d95e04fc`, `bf7cdd0`, `92d0067`, `54afa70`  
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

## 2. Style, Bloat & Architecture Cross-Examination

### A. Unnecessary `else` Branches
- **Audit**: Inspected conditional control flow across `setup.go`, `agentproto.go`, and test helpers.
- **Finding**: Functions strictly adhere to early returns:
  - `scopeConfigDir`: returns early on `!project`, keeping the happy path flat at root indentation.
  - `writeOrRemoveConfigFile`: handles `len(data) == 0` with an early return before `writeJSONFile`.
  - `maybePrintProjectTrustWarning`: guards on `target != targetCodex || !project` with an immediate return.
- **Map Mutation**: In `removeRawclawAntigravityHooks`, the `if len(filtered) == 0 { delete(...) } else { groupMap[...] = filtered }` branch is an explicit map mutation and represents the minimal clear Go idiom.

### B. Repeated Shell Fragments
- **Audit**: `rawclawPrimeScript` and `rawclawCodexPrimeScript` duplicate ~30 lines of POSIX shell logic (session ID regex extraction, catalog path expansion, `set -C` atomic claim, and temporary JSON creation).
- **Evaluation**: Inlining templates into `const` string blocks avoids runtime string templating dependencies, keeps binary startup instant, and makes each POSIX shell script self-contained and independently auditable with standard shell linters (`shellcheck`).

### C. Needless Helpers & Public Surface
- **Audit**: Unification helpers `installRawclawHookWith` and `ejectRawclawHookWith` use higher-order functions (`hasHooks`, `removeHooks`) to parameterize format differences.
- **Encapsulation**: All helper functions remain strictly unexported in `package cli`. No new public API surface is introduced.

### D. Comments: Invariant Defense vs. Obvious Code
- **Audit**: Block comments in `setup.go` document critical failure modes and non-obvious platform constraints:
  - Explains why `rawclawMarker = "hooks/rawclaw/"` uses path segments rather than the bare word "rawclaw" to avoid clobbering sibling tools.
  - Explains why `true >` is used over `: >` (preventing fatal exits under POSIX `dash` when special builtins hit redirection errors).
  - Documents non-interactive `PATH` isolation and `os.Executable` absolute path baking.
- Trivial standard library calls have zero comment clutter.

### E. Functional-Option & Interface Temptation
- **Audit**: The implementation resisted introducing abstract `type HookTarget interface` or `type InstallOption func(*InstallConfig)`.
- **Right-Sizing**: Procedural dispatch across 3 known agent targets via unexported functions satisfies the two-adapter / right-sizing rule with zero heap allocations or interface indirection.

### F. Modernizations (Shrink Without Changing POSIX Bytes)
- **Applicable Go Modernizations**: `cmp.Or` for environment fallbacks, `slices.Contains` for hook filtering, and `t.Context()` in test setups.
- **POSIX Invariant**: Go-side modernizations strictly preserve the exact literal bytes of the POSIX `sh` templates (preserving `(set -C; : > "$entry")` and single-quote escaping).

---

## 3. Proof of Correctness (Correctness Beats Line Count)

### 1. Concurrent Dedup Proof
```sh
claimed=0
if (set -C; : > "$entry") 2>/dev/null; then
    claimed=1
elif [ -e "$entry" ]; then
    exit 0
fi
```
- **Atomicity**: Shell `set -C` (noclobber) maps to kernel `open(..., O_WRONLY|O_CREAT|O_EXCL)`.
- **Proof**: If $N$ processes launch concurrently for the same `session_id`, exactly one subshell creates `$entry` and sets `claimed=1`. The other $N-1$ processes fail the `set -C` check, hit `elif [ -e "$entry" ]`, and terminate immediately with exit code `0`.
- Only the winning process writes the metadata JSON and launches `nohup "$RAWCLAW" ingest "$session_id"`.
- Verified by `TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest`.

### 2. Fail-Soft Permission and I/O Behavior
- **Unwritable Directory / Permission Denied**:
  - If `mkdir -p` fails or `$catalog_dir` is unwritable, `claimed` stays `0`.
  - Fallback: `true > "$entry" 2>/dev/null || true` prevents shell termination (using `true >` instead of `: >` to avoid POSIX `dash` fatal exit on special builtin redirection failure).
  - The hook bypasses the catalog write, outputs the discovery banner to stdout, and exits `0` without breaking the agent's SessionStart event (`TestPrimeScript_CatalogWriteFailure_NeverFailsHook`).
- **Missing Dependencies**:
  - Codex hook: `command -v python3 >/dev/null 2>&1 || exit 0` gracefully suppresses the banner if Python 3 is absent.
  - Antigravity hook: Injects pre-marshaled JSON (`@@RAWCLAW_INJECT_JSON@@`), requiring zero runtime JSON binaries.

### 3. JSON Envelopes
- **Session Catalog Entry**: `{"session_id":"...", "transcript_path":"...", "cwd":"...", "source":"claude"|"codex"}` written atomically via temporary file rename (`mv -f "$tmp_entry" "$entry"`).
- **Claude Code**: Emits bare banner text to stdout.
- **Codex**: Emits `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"..."}}`.
- **Antigravity**: Emits `{"injectSteps":[{"ephemeralMessage":"..."}]}`.

### 4. Exact Ejection
- **File Removal**: Deletes `hooks/rawclaw/prime.sh` and legacy `tagqueue.sh`.
- **Directory Cascade**: `removeIfEmpty` recursively removes `hooks/rawclaw`, `hooks`, and config directories **only if completely empty**. Sibling tools or user files are never deleted.
- **Config Surgical Editing**: Removes only keys matching `hooks/rawclaw/` or `"rawclaw"`. If the resulting config is empty `{}`, the file is deleted (`writeOrRemoveConfigFile`); otherwise, non-rawclaw keys are preserved byte-for-byte.

---

## 4. Upstream Architecture & Bloat Skills Scorecard

| Skill | Grade | Actionable Deletion Signal | Correctness Awareness | Noise Level | Assessment for this Target |
|---|:---:|:---:|:---:|:---:|---|
| **`modular-refactor` + `right-sizing`** | **A** | High | High | Low | **Best-in-class brake**: Its explicit stopping criterion ("a CLI tool does not need layers of abstraction; don't add a port without 2+ implementations in sight") justifies keeping hook setup flat and procedural. |
| **`golang-code-style`** | **A** | High | High | Low | **High value**: Enforces early returns, unexported-by-default encapsulation, and eliminates unnecessary `else` branches. |
| **`codebase-design`** | **A-** | High | High | Low | **Strong framework**: The "deep module" and "deletion test" concepts validate that `setup.go` provides a compact interface (`RunSetup`, `RunEject`) hiding complex OS/shell interactions. |
| **`ponytail` / `ponytail-review`** | **B+** | High | Medium | Medium | **Good for pruning, risky on shell**: Prunes dead returns and unneeded helpers effectively, but risks stripping critical defensive POSIX fallbacks (`true >`, `2>/dev/null`, permission branches) if applied blindly. |
| **`golang-structs-interfaces`** | **C+** | Low | High | High | **Over-abstraction hazard**: Encourages interface creation, compile-time assertions, and embedding that tempt developers to convert 3 static file templates into an object hierarchy. |
| **`golang-design-patterns`** | **D** | Low | Medium | High | **Severe bloat hazard**: Recommends functional options (`NewInstaller(WithClaude(), WithCodex())`), builders, and lifecycle hooks that would inflate 150 lines of lean Go into 400+ lines of ceremony. |

### Skills That Would Make This Patch Worse
1. **`golang-design-patterns`**: Would attempt to wrap procedural hook installation in functional options or factory structs, adding indirection without any architectural benefit.
2. **`golang-structs-interfaces`**: Would tempt introducing a `HookTarget` interface and separate struct types for each runtime, scattering simple string templates across multiple files.
3. **Unconstrained `ponytail` (Ultra mode)**: Might flag POSIX shell fail-soft fallback branches (`true > "$entry" || true`, `|| true` on `mkdir`, and multi-step `set -C` subshells) as "speculative error handling" or attempt to delete characterization tests, degrading runtime resilience in restricted user environments.

---

## 5. Metric & Conclusion
- **Net Production Lines Possible**: Net -42 lines across hook installation/ejection deduplication without compromising POSIX bytes or test coverage.
- **Go Source Integrity**: Zero Go source edits required; `gofmt` clean across `internal/`.
