# Prewarm / Antigravity adversarial review

## Verdict

**ACCEPT — 7d5a6a5 is safe to transplant for the named production and
white-box test paths.** Its claim is narrow and true: `addRawclawAntigravityHooks`
only mutates an in-memory map, and every operation in that helper has no error
result. Removing its dead `error` return does not make filesystem failures
“impossible”; it does not touch the filesystem path.

The broader claim would be nonsense. Installation still has fallible directory
creation, script writes, JSON reads/parsing, atomic JSON temp-write/rename, and
legacy-file removal, all still wrapped and returned.

## Findings

### P2 — existing, not introduced: setup is not failure-atomic

`internal/cli/setup.go:930-942` writes the hook script before reading and
writing `hooks.json`. A config failure leaves an orphaned executable script.
Observed probe: with `settings.json` made a directory, `setup --yes` returned
status 1 with the wrapped read error, while `hooks/rawclaw/prime.sh` existed.
Eject can clean this up, but failed setup still changed disk state. This
predates 7d5a6a5 and is outside the safe transplant delta.

### P2 — existing, not introduced: script replacement is not atomic

`internal/cli/setup.go:417-426` uses `os.WriteFile` directly for the generated
shell script. A mid-write failure can leave a truncated script, unlike JSON's
sibling-temp-plus-rename path at `setup.go:527-550`. This is a hardening
opportunity, not evidence against removing an impossible map-helper error.

No new finding is caused by 7d5a6a5. It changes only the helper signature,
return, call, and two test callers; generated JSON, POSIX shell text, paths,
write ordering, eject behavior, and error wrapping are unchanged.

## Falsification checklist

- Missing parent paths: **clean for this commit**. `writeHookScript` and
  `writeJSONFile` call `os.MkdirAll`; fresh trees are covered by install/journey
  tests.
- Permission / invalid-parent failures: **errors remain observable**. A
  regular-file `CLAUDE_CONFIG_DIR` returned status 1 with
  `install hook script: mkdir ...: not a directory`.
- Config read/write failures: **errors remain observable**. A directory at
  `settings.json` returned status 1 with the wrapped read error; the script
  orphan was confirmed.
- Partial writes / atomicity: **JSON atomicity remains; script atomicity does
  not**. Neither property changed here.
- Setup/eject symmetry: **normal round trips clean** in existing Antigravity
  solo, sibling, empty-file, empty-object, adoption, and command journey
  tests. Failure atomicity is the existing P2 caveat.
- Generated POSIX `sh`: **unchanged**. Existing contract tests cover the
  generated `injectSteps` payload and invocation gating; no separate `sh -n`
  gate was added by this commit.
- Error-reporting contract: **preserved**. The install function still returns
  rendering, script-write, JSON-read, JSON-write, and legacy-cleanup errors.
  Only the impossible in-memory helper branch disappeared.

## Safe transplant paths

- `internal/cli/setup.go` — only the `addRawclawAntigravityHooks` signature,
  removal of `return nil`, and its call site in
  `installRawclawAntigravityHook`.
- `internal/cli/setup_test.go` — only callers in
  `TestAddRawclawAntigravityHooksReplacesExistingAndKeepsSibling` and
  `TestAddRawclawAntigravityHooksIdempotent`.

Do **not** transplant `FINDINGS.md`; it is a review artifact. The integration
tip comparison also shows `internal/index/consolidated_test.go` as an unrelated
deletion in the 2ee9950..7d5a6a5 range; exclude it from this prewarm change.

## Commands and observed output

Working tree: `/Users/jay-m4/code/rawclaw-norm-prewarm-review`, branch
`norm/prewarm-adversarial-review`.

```text
mnemon --store rawclaw recall "prewarm antigravity setup hook error" --limit 5 --verbose
=> returned prior Antigravity hook/setup verification and review findings.
graphify reflect --if-stale
=> Reflected 0 memories; graphify-out/reflections/LESSONS.md read.
graphify update . --no-cluster
=> graph updated: 3439 nodes, 12006 edges.
graphify query "addRawclawAntigravityHooks installRawclawAntigravityHook writeJSONFile" --budget 4000
=> surfaced setup.go, setup_test.go, install/eject, writeJSONFile, related tests.
graphify explain "installRawclawAntigravityHook"
=> confirmed calls to writeHookScript, readJSONFile, add helper, writeJSONFile.
```

The requested package race gate was run five times:

```text
RUN 1: ok github.com/MoonCaves/rawclaw/internal/cli 128.786s
RUN 2: ok github.com/MoonCaves/rawclaw/internal/cli 90.243s
RUN 3: ok github.com/MoonCaves/rawclaw/internal/cli 180.639s
RUN 4: ok github.com/MoonCaves/rawclaw/internal/cli 173.458s
RUN 5: ok github.com/MoonCaves/rawclaw/internal/cli 146.834s
```

Additional requested gates:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli/...
=> ok github.com/MoonCaves/rawclaw/internal/cli 128.878s
gofmt -l internal/
=> no output; status 0
git diff --check
=> no output; status 0
~/go/bin/golangci-lint run ./internal/cli/...
=> 0 issues.
```

Failure probes in a temporary `/tmp` tree:

```text
CLAUDE_CONFIG_DIR=/tmp/.../not-a-dir ... rawclaw setup --yes
=> status 1; "install rawclaw hook: install hook script: mkdir ...: not a directory"

CLAUDE_CONFIG_DIR=/tmp/.../claude ... rawclaw setup --yes (settings.json is a directory)
=> status 1; "install rawclaw hook: read .../settings.json: ... is a directory"
=> hooks/rawclaw/prime.sh existed afterward.
```

No production or test Go files were edited in this review.
