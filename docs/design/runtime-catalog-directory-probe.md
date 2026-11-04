# Goose and Hermes catalog directory probe

**Result: reproduced in an isolated local fixture.** On Darwin `/bin/sh`, the pinned Goose and Hermes SessionStart templates treat a pre-existing directory at the intended session-claim path as a successful claim. Three overlapping hook processes each invoke the ingest stub and leave a nested `.tmp.<pid>.json` hard link. For a symlink to a directory, they do the same in the linked target, which is outside the fixture catalog but still inside the temporary fixture.

This establishes a path-selection defect in these emitted templates under injected local filesystem state. Live editor impact is unmeasured. The stub only logs arguments; no transcript is ingested. No repair or protocol approval is proposed here.

## Source and coverage

Tested snapshot: `0afecdfab2321abb49733746f7250e6572d5ec5e`, `internal/cli/setup.go` (SHA-256 `88057095f725f6511837122a955b00af4b49657e1cf77e3953fe73750e3a9a8a`). The Goose template at lines 362–392 has SHA-256 `cf4dbb62d4235ac4d998e079931cfcd47533edd82da9269e505b1b68d909e2ae`; Hermes at lines 395–417 has SHA-256 `4f16262884a819187ff4b9b43808859934996ee4705ca64d901dad7e9e7094b6`.

Both templates set `entry="$catalog_dir/$session_id"`, make a temporary file under the catalog, then run `ln "$tmp" "$entry"`; success removes the source temp file and launches `ingest` (Goose `setup.go:379–385`, Hermes `:404–410`). When `entry` already names a directory, `ln` creates the hard link inside it and returns success. The script then mistakes that for acquiring the intended session entry. A symlink to a directory takes the same path on the tested platform.

The existing `TestPrimeScripts_SessionStartCatalogClaimIsPathSafe` covers Claude, Codex, and Antigravity templates (`internal/cli/cmd_ingest_test.go:135–164`); its directory case expects no ingest (`:251–252`) and verifies the directory remains empty (`:255–268`). Goose and Hermes are absent from that test's template list.

## Reproduction

Run from the repository root. The Python harness extracts the pinned raw templates and banner, applies the same binary and banner substitutions as `renderRuntimeScript`, and runs the resulting scripts through `/bin/sh`. The stub logs its first three arguments and does not ingest. The harness asserts hook exit codes and per-case ingest counts before printing results.

```sh
python3 - <<'PY'
import hashlib, json, os, re, shlex, subprocess, tempfile, time
from pathlib import Path

rev = "0afecdfab2321abb49733746f7250e6572d5ec5e"
source = subprocess.check_output(["git", "show", rev + ":internal/cli/setup.go"], text=True)
def literal(name):
    pattern = r"const " + name + r" = " + chr(96) + r"(.*?)" + chr(96) + r"\n"
    return re.search(pattern, source, re.S).group(1)

banner = literal("rawclawBanner")
print("setup.go sha256=" + hashlib.sha256(source.encode()).hexdigest())
for runtime, const in [("goose", "rawclawGooseCatalogScript"),
                       ("hermes", "rawclawHermesCatalogScript")]:
    template = literal(const)
    print(runtime + " template sha256=" + hashlib.sha256(template.encode()).hexdigest())
    for case in ("directory", "symlink-directory", "regular-file", "new-file"):
        with tempfile.TemporaryDirectory(prefix="catalog-probe-") as td:
            root = Path(td); catalog = root / "catalog"; catalog.mkdir()
            sid = "probe-session-123"; entry = catalog / sid; target = root / "linked-target"
            if case == "directory": entry.mkdir()
            if case == "symlink-directory":
                target.mkdir(); entry.symlink_to(target, target_is_directory=True)
            if case == "regular-file": entry.write_text("existing\n")
            calls = root / "calls.tsv"; stub = root / "rawclaw-stub"
            stub.write_text('#!/bin/sh\nprintf "%s\\t%s\\t%s\\n" "$1" "$2" "$3" >> "$PROBE_CALLS"\n')
            stub.chmod(0o755)
            rendered = template.replace("@@RAWCLAW@@", shlex.quote(str(stub))).replace("@@BANNER_RAW@@", banner)
            assert "@@" not in rendered
            script = root / "hook.sh"; script.write_text(rendered); script.chmod(0o755)
            env = dict(os.environ, HOME=str(root / "home"), XDG_DATA_HOME=str(root / "data"),
                       XDG_CACHE_HOME=str(root / "cache"), RAWCLAW_CATALOG_DIR=str(catalog),
                       PROBE_CALLS=str(calls))
            Path(env["HOME"]).mkdir()
            payload = json.dumps({"session_id": sid, "hook_event_name": "SessionStart",
                                  "working_dir": "/fixture", "cwd": "/fixture"}).encode()
            procs = [subprocess.Popen(["/bin/sh", str(script)], stdin=subprocess.PIPE,
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
                     for _ in range(3)]
            for proc in procs:
                proc.stdin.write(payload); proc.stdin.close(); proc.stdin = None
            exits = []
            for proc in procs:
                proc.communicate(timeout=5); exits.append(proc.returncode)
            deadline = time.monotonic() + 2
            while time.monotonic() < deadline:
                if calls.exists() and len(calls.read_text().splitlines()) == 3: break
                time.sleep(0.02)
            argv = calls.read_text().splitlines() if calls.exists() else []
            assert exits == [0, 0, 0], (runtime, case, exits)
            expected_calls = {"directory": 3, "symlink-directory": 3, "regular-file": 0, "new-file": 1}[case]
            assert len(argv) == expected_calls, (runtime, case, argv)
            assert all(line == f"ingest\t--\t{sid}" for line in argv)
            nested = list(entry.glob(".tmp.*.json")) if entry.is_dir() and not entry.is_symlink() else []
            linked = list(target.glob(".tmp.*.json")) if target.exists() else []
            kind = "symlink" if entry.is_symlink() else "directory" if entry.is_dir() else "file" if entry.is_file() else "absent"
            print(f"{runtime} {case}: births=3 exits={exits} ingest={len(argv)} "
                  f"entry_nested_tmp={len(nested)} linked_target_tmp={len(linked)} entry={kind}")
PY
```

Observed output:

```text
setup.go sha256=88057095f725f6511837122a955b00af4b49657e1cf77e3953fe73750e3a9a8a
goose template sha256=cf4dbb62d4235ac4d998e079931cfcd47533edd82da9269e505b1b68d909e2ae
goose directory: births=3 exits=[0, 0, 0] ingest=3 entry_nested_tmp=3 linked_target_tmp=0 entry=directory
goose symlink-directory: births=3 exits=[0, 0, 0] ingest=3 entry_nested_tmp=0 linked_target_tmp=3 entry=symlink
goose regular-file: births=3 exits=[0, 0, 0] ingest=0 entry_nested_tmp=0 linked_target_tmp=0 entry=file
goose new-file: births=3 exits=[0, 0, 0] ingest=1 entry_nested_tmp=0 linked_target_tmp=0 entry=file
hermes template sha256=4f16262884a819187ff4b9b43808859934996ee4705ca64d901dad7e9e7094b6
hermes directory: births=3 exits=[0, 0, 0] ingest=3 entry_nested_tmp=3 linked_target_tmp=0 entry=directory
hermes symlink-directory: births=3 exits=[0, 0, 0] ingest=3 entry_nested_tmp=0 linked_target_tmp=3 entry=symlink
hermes regular-file: births=3 exits=[0, 0, 0] ingest=0 entry_nested_tmp=0 linked_target_tmp=0 entry=file
hermes new-file: births=3 exits=[0, 0, 0] ingest=1 entry_nested_tmp=0 linked_target_tmp=0 entry=file
```

All fixture paths are temporary and isolated. Three processes are started before their payloads are released; this demonstrates the observed local interleaving, not every possible schedule.
