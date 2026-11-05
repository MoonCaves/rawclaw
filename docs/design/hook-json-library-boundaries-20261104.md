# Issue 89: JSON library source boundaries

Date: 2026-11-04. Source note only; no adoption or implementation verdict.

## Parser and transport boundaries

GJSON is pinned to `690362d6edf4bcbde1e9f54d552d3814f1cd5bcb`, under the MIT license.[3]
Its `Result` separates type, decoded `Str`, and raw JSON; `String()` returns decoded strings, converts numbers and booleans, and returns raw JSON for objects/arrays (gjson.go L65–112).[1]
`Get` explicitly does not validate JSON; malformed input can yield unexpected results (gjson.go L2172–2177).[1]
Consequently, calling `Get(...).String()` does not establish string-only field handling or malformed-input policy.
The pinned module declares Go 1.23 and requires `tidwall/match` and `tidwall/pretty` (go.mod L1–8).[2]
Using it would add build dependencies without requiring an external runtime executable; this note does not propose adding it.

The existing [Infisical source ledger](hook-assignment-protocol-sources-20261104.md) records typed key/value → quoted POSIX export serialization and its non-NUL transport probe.
That ledger remains the source for its immutable pin, license, and limitations; they are not duplicated here.
JSON lookup and POSIX export primitives do not source the complete hook transport composition: field selection, trusted output names, type/default decisions, malformed-input/NUL handling, invocation/eval behavior, and editor mappings remain separate relationships to establish.

## Catalog serialization constraint

At baseline `754cf4ab3045f2196be8b32c3c7b69d48332af52`, `rawclawSessionCatalogHead` doubles backslashes in `cwd`, then places it inside JSON quotes with `printf` (setup.go L26–44).[4]
The following probe feeds **already decoded** quote/newline/backslash fixture values into those exact existing writer lines.
It does not show that the current `sed` extractor returns those values; it isolates the downstream writer after a hypothetical correct decoder.
Run from the baseline repository with Python 3 and POSIX `/bin/sh`:

```sh
python3 - <<'PYTHON'
from pathlib import Path
import json, os, subprocess
head = Path('internal/cli/setup.go').read_text().split('const rawclawSessionCatalogHead = `', 1)[1].split('`', 1)[0]
lines = [line.strip() for line in head.splitlines()]
script = next(x for x in lines if x.startswith('esc_cwd=')) + '\n'
script += next(x for x in lines if x.startswith("printf '  \"cwd\"")) + '\n'
for name, value in [('quote', '/tmp/a"b'), ('newline', '/tmp/a\nb'), ('backslash', r'/tmp/a\b')]:
    p = subprocess.run(['/bin/sh', '-c', script], env=dict(os.environ, cwd=value), capture_output=True)
    candidate = b'{' + p.stdout.rstrip().removesuffix(b',') + b'}'
    try:
        result = 'valid=' + str(json.loads(candidate)['cwd'] == value)
    except json.JSONDecodeError:
        result = 'invalid-json'
    print(f'{name}: exit={p.returncode} {result} emitted={candidate!r}')
PYTHON
```

Observed output:

```text
quote: exit=0 invalid-json emitted=b'{  "cwd": "/tmp/a"b"}'
newline: exit=0 invalid-json emitted=b'{  "cwd": "/tmp/a\nb"}'
backslash: exit=0 valid=True emitted=b'{  "cwd": "/tmp/a\\\\b"}'
```

Parser replacement alone therefore cannot promise valid catalog JSON for decoded strings; the existing writer also needs JSON literal serialization or an explicitly narrower scope that leaves this defect open.

## Current Goose/Hermes extraction and template probe

This separate probe pins public main `0afecdfab2321abb49733746f7250e6572d5ec5e`, extracts the current Goose and Hermes raw Go templates, and runs each whole template with JSON hook input, isolated temporary state, and a stub `rawclaw` ingest command. The synthetic `cwd` values use `/tmp` paths. Plain paths survive; quote, backslash, and newline paths produce an empty extracted `cwd` for both templates. The resulting catalog entries are valid JSON with `cwd` set to the empty string. In all eight cases the shell exits 0, prints the banner, and calls the ingest stub.

For these inputs the extracted value is empty, not a truncated path prefix. This measures current extraction before catalog serialization. It does not contradict the preceding hypothetical probe, which feeds already-decoded special characters directly into the catalog writer and observes invalid JSON for quote/newline values. The two probes exercise different values at the writer boundary.

Run from the pinned checkout with Python 3 and `/bin/sh`:

```python
import hashlib, json, os, platform, re, shlex, subprocess, tempfile
from pathlib import Path

REV = "0afecdfab2321abb49733746f7250e6572d5ec5e"
src = subprocess.check_output(["git", "show", f"{REV}:internal/cli/setup.go"])
text = src.decode()

def raw(name):
    m = re.search(rf"const\s+{name}\s*=\s*`(.*?)`", text, re.S)
    if not m:
        raise RuntimeError(f"missing raw Go constant {name}")
    return m.group(1)

banner = raw("rawclawBanner")
templates = {"goose": raw("rawclawGooseCatalogScript"),
             "hermes": raw("rawclawHermesCatalogScript")}
print(f"source_revision={REV}")
print(f"setup_go_sha256={hashlib.sha256(src).hexdigest()}")
print(f"platform={platform.system()} shell=/bin/sh")

for runtime, template in templates.items():
    outcomes, exits, banners, calls, valid = [], 0, 0, 0, 0
    for label, tail in [("plain", "plain"), ("quote", 'a"b'),
                        ("backslash", r"a\b"), ("newline", "a\nb")]:
        with tempfile.TemporaryDirectory(prefix=f"rc89-{runtime}-{label}-") as temp:
            root = Path(temp)
            sid = f"rc89-{runtime}-{label}-session"
            cwd = f"/tmp/{sid}/{tail}"
            event = {"session_id": sid}
            event.update({"hook_event_name": "SessionStart", "working_dir": cwd}
                         if runtime == "goose" else {"cwd": cwd})
            payload = json.dumps(event, separators=(",", ":"))
            stub, log = root / "rawclaw-stub", root / "calls.log"
            stub.write_text('#!/bin/sh\nprintf \'%s\\n\' "$*" >> "$RAWCLAW_TEST_CALLS"\n')
            stub.chmod(0o755)
            script = template.replace("@@RAWCLAW@@", shlex.quote(str(stub)))
            script = script.replace("@@BANNER_RAW@@", banner)
            script = script.replace("set -eu\n", "set -eu\ntrap 'wait' 0\n", 1)
            hook = root / "hook.sh"
            hook.write_text(script)
            hook.chmod(0o755)
            env = dict(os.environ, HOME=str(root / "home"),
                       XDG_DATA_HOME=str(root / "data"),
                       XDG_CACHE_HOME=str(root / "cache"),
                       RAWCLAW_CATALOG_DIR=str(root / "catalog"),
                       RAWCLAW_TEST_CALLS=str(log))
            Path(env["HOME"]).mkdir()
            p = subprocess.run(["/bin/sh", str(hook)], input=payload.encode(),
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               env=env, timeout=10, check=False)
            exits += p.returncode == 0
            banners += banner.encode() in p.stdout
            calls += log.exists() and log.read_text().splitlines() == [f"ingest -- {sid}"]
            record = json.loads((root / "catalog" / sid).read_bytes())
            valid += 1
            outcomes.append("preserved" if record.get("cwd") == cwd else
                            "empty" if record.get("cwd") == "" else "different")
    print(f"{runtime}: cwd={','.join(outcomes)} exit={exits}/4 "
          f"banner={banners}/4 stub_calls={calls}/4 valid_catalog_json={valid}/4")
```

Observed on Darwin with `/bin/sh`:

```text
source_revision=0afecdfab2321abb49733746f7250e6572d5ec5e
setup_go_sha256=88057095f725f6511837122a955b00af4b49657e1cf77e3953fe73750e3a9a8a
platform=Darwin shell=/bin/sh
goose: cwd=preserved,empty,empty,empty exit=4/4 banner=4/4 stub_calls=4/4 valid_catalog_json=4/4
hermes: cwd=preserved,empty,empty,empty exit=4/4 banner=4/4 stub_calls=4/4 valid_catalog_json=4/4
```

The stub call proves only that the template invoked the command with the tested session ID; it does not establish ingestion success. The fixture does not emulate an editor/provider accepting the payload, or establish live hook behavior on other shells or platforms. It records source-only behavior at this revision; it proposes no runtime fix or JSON-library adoption. [5]

## Sources

[1] https://github.com/tidwall/gjson/blob/690362d6edf4bcbde1e9f54d552d3814f1cd5bcb/gjson.go
[2] https://github.com/tidwall/gjson/blob/690362d6edf4bcbde1e9f54d552d3814f1cd5bcb/go.mod
[3] https://github.com/tidwall/gjson/blob/690362d6edf4bcbde1e9f54d552d3814f1cd5bcb/LICENSE
[4] https://github.com/MoonCaves/rawclaw/blob/754cf4ab3045f2196be8b32c3c7b69d48332af52/internal/cli/setup.go
[5] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/setup.go

## Native hook adapter source delta

Date: 2026-11-05. Source-only record; no adoption, copied-block eligibility, implementation or provenance-gate verdict.

`entireio/cli` at immutable revision `4188e9b89555579eb74a8bab9ab6ee054833042c` contains a native-hook stdin reader and source-specific typed event adapters. Its [shared reader, `cmd/entire/cli/agent/event.go` L283-L348](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/cmd/entire/cli/agent/event.go#L283-L348) decodes one complete JSON value without waiting for EOF and then unmarshals it into a typed payload. Empty input and decoder/unmarshal errors are returned; the ordinary reader is unlimited, while a separate variant accepts a byte limit. The terminal check uses `golang.org/x/term`. These blocks do not supply a whole-stream trailing-data check, required-field validation or explicit NUL rejection.

The Codex adapter supplies [typed session fields including string `cwd` and nullable `transcript_path`, `cmd/entire/cli/agent/codex/types.go` L34-L51](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/cmd/entire/cli/agent/codex/types.go#L34-L51), a [nullable-string helper at L97-L103](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/cmd/entire/cli/agent/codex/types.go#L97-L103), and [typed parsing/event mapping, `cmd/entire/cli/agent/codex/lifecycle.go` L188-L204](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/cmd/entire/cli/agent/codex/lifecycle.go#L188-L204). Its [SessionStart dispatch is at L114-L121](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/cmd/entire/cli/agent/codex/lifecycle.go#L114-L121). The Claude [session type, `cmd/entire/cli/agent/claudecode/types.go` L151-L158](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/cmd/entire/cli/agent/claudecode/types.go#L151-L158), omits `cwd`; its [parser at `claudecode/lifecycle.go` L136-L149](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/cmd/entire/cli/agent/claudecode/lifecycle.go#L136-L149) therefore does not source RawClaw's complete Claude field tuple.

The Antigravity [types, `cmd/entire/cli/agent/antigravity/types.go` L49-L93](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/cmd/entire/cli/agent/antigravity/types.go#L49-L93), explicitly omit `workspacePaths` and model `invocationNum` as an integer. They do not source first-workspace selection or RawClaw catalog/dedup behavior. The reader and event adapters do not emit POSIX assignments or serialize RawClaw catalog records; combining a parser with a separate exporter still leaves the transport composition to establish.

Complete source files, the reader's `event_test.go`, and [MIT `LICENSE` L1-L21](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/LICENSE#L1-L21), Copyright (c) 2026 Entire Inc., were fetched on 2026-11-05. The pinned [go.mod L1-L3](https://github.com/entireio/cli/blob/4188e9b89555579eb74a8bab9ab6ee054833042c/go.mod#L1-L3) requires Go 1.27.3, above RawClaw's documented Go 1.24 floor; the full application module is not a drop-in compatible dependency. Any prospective copied unit needs its exact source/import/helper/test boundaries, source-before-edit ACK, three-way review and the ordinary provenance gate. This record does not retroactively source a held candidate, reverse a HOLD, change gate scope, or authorize implementation.
