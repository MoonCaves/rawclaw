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

## Native no-replace publication source delta

Date: 2026-11-05. Source-only record; no adoption, copied-unit eligibility, implementation or provenance-gate verdict.

`antibyte/AuraGo` at immutable revision `a2f36c6dacb41e77e68ac5788a8802115953881d` supplies [`internal/server/atomic_publish.go` L11-L37](https://github.com/antibyte/AuraGo/blob/a2f36c6dacb41e77e68ac5788a8802115953881d/internal/server/atomic_publish.go#L11-L37): `publishFileNoReplace(tempPath, targetPath string) error` publishes an already-written source with `os.Link`. It returns `nil` on link success, a wrapped `errAtomicPublishTargetExists` sentinel when the target exists, and a wrapped error for other link failures. The signature returns an error, not a created boolean. The unit imports only standard-library packages and contains no stale-PID, TTL, target-deletion or rename fallback policy.

The helper first requires both absolute cleaned parent-directory strings to match. It does not source RawClaw's current private-subdirectory staging unchanged. Its [upstream caller, `internal/server/agodesk_knowledge_upload.go` L727-L743](https://github.com/antibyte/AuraGo/blob/a2f36c6dacb41e77e68ac5788a8802115953881d/internal/server/agodesk_knowledge_upload.go#L727-L743), creates a temp file in the target directory and writes it; [L777-L803](https://github.com/antibyte/AuraGo/blob/a2f36c6dacb41e77e68ac5788a8802115953881d/internal/server/agodesk_knowledge_upload.go#L777-L803) closes/validates it before publication and handles the exists sentinel. The helper does not write or fsync the source, validate that it is a regular file, or prevent later inode mutation. Complete-bytes publication assumes the caller has finished writing; temp removal and directory open/sync/close errors are ignored. File-content syncing remains caller responsibility; this establishes no crash-durability guarantee. Native hardlink availability and filesystem errors remain platform dependent.

The fetched [tests, `internal/server/atomic_publish_test.go` L11-L33, L35-L87 and L89-L101](https://github.com/antibyte/AuraGo/blob/a2f36c6dacb41e77e68ac5788a8802115953881d/internal/server/atomic_publish_test.go#L11-L101), cover existing-target preservation, a competing O_EXCL writer, and different-directory rejection. They were not executed for this source record; the competing writer test does not establish absence of transient partial content for that separate O_EXCL writer. The primitive differs from RawClaw's existing `WriteCatalogEntry`, which replaces its target with rename and returns no first-claim result. Connecting a decoder, serializer and no-replace publication still leaves staging, ID, claim/fallback and editor invocation relationships to establish.

Complete helper, tests, caller, [MIT `LICENSE` L1-L21](https://github.com/antibyte/AuraGo/blob/a2f36c6dacb41e77e68ac5788a8802115953881d/LICENSE#L1-L21), Copyright (c) 2025 AuraGo contributors, and [go.mod L1-L3](https://github.com/antibyte/AuraGo/blob/a2f36c6dacb41e77e68ac5788a8802115953881d/go.mod#L1-L3) were fetched on 2026-11-05. The application module requires Go 1.27.3, above RawClaw's Go 1.24 floor; it is not a drop-in module dependency. Any prospective copied unit needs exact source/import/sentinel/test boundaries, source-before-edit ACK, three-way review and the ordinary gate. This record does not source the complete RawClaw hook protocol, retroactively source a held candidate, reverse a HOLD, change gate scope, or authorize implementation.

## Generated Claude/Codex cwd preservation probe

Date: 2026-11-05. Source: existing generated hooks at immutable [RawClaw `3e988fa0e2d77864ae5e3b75f434f4a401de14be`, `internal/cli/setup.go`](https://github.com/MoonCaves/rawclaw/blob/3e988fa0e2d77864ae5e3b75f434f4a401de14be/internal/cli/setup.go#L26-L166).

The following probe builds that checkout and runs its actual `setup --yes` generator inside temporary configuration roots. It then replaces only the temporary executable with an argv-logging stub, leaving the generated scripts unchanged. Each runtime receives valid JSON and an ordinary session ID. Both actual cwd directories exist. Plain cwd survives; an escaped quote produces a truncated decoded cwd ending in a backslash; a newline between the `cwd` key and its value produces empty cwd. All six hooks exit 0, print the banner and request exactly one stub `ingest -- SESSION`; all six published catalog records parse as JSON.

This is valid-but-wrong catalog data from the current extraction path. It differs from the earlier conditional writer probe, which feeds already-decoded quote/newline values directly to serialization and observes invalid JSON. Correct decoding alone does not establish correct serialization. This synthetic probe establishes neither editor-produced payloads nor successful real ingestion, and grants no copied-unit eligibility or implementation approval.

Run from the pinned checkout with Go, Python 3 and `/bin/sh`. Only CASE/CATALOG output normalizes the generated temporary root to `<temp>`; setup also prints its temporary generated paths. The excerpts below are literal normalized CASE lines, not fabricated full output.

```sh
python3 - <<'PY'
import hashlib,json,os,pathlib,subprocess,tempfile,time
repo=pathlib.Path.cwd()
with tempfile.TemporaryDirectory(prefix='rawclaw-hook89-tick222-') as scratch:
 root=pathlib.Path(scratch); binary=root/'rawclaw'; home=root/'home'; home.mkdir(); (root/'tmp').mkdir()
 b=subprocess.run(['go','build','-o',str(binary),'./cmd/rawclaw'],env=dict(os.environ,CGO_ENABLED='0'),capture_output=True,text=True)
 print('BUILD CGO_ENABLED=0 go build -o <temp>/rawclaw ./cmd/rawclaw exit='+str(b.returncode)+' stdout='+repr(b.stdout)+' stderr='+repr(b.stderr)); assert b.returncode==0
 claude=root/'claude'; codex=root/'codex'; codex.mkdir()
 env={'PATH':os.environ['PATH'],'HOME':str(home),'TMPDIR':str(root/'tmp'),'XDG_CONFIG_HOME':str(root/'config'),'XDG_DATA_HOME':str(root/'data'),'XDG_CACHE_HOME':str(root/'cache'),'CLAUDE_CONFIG_DIR':str(claude),'CODEX_HOME':str(codex),'ANTIGRAVITY_HOME':str(root/'absent-agy'),'RAWCLAW_CATALOG_DIR':str(root/'setup-catalog')}
 s=subprocess.run([str(binary),'setup','--yes'],env=env,cwd=root,capture_output=True,text=True,timeout=20)
 print('SETUP actual temporary-config generator exit='+str(s.returncode));print(s.stdout,end='');print('SETUP stderr='+repr(s.stderr));assert s.returncode==0
 hooks={name:base/'hooks/rawclaw/prime.sh' for name,base in [('claude',claude),('codex',codex)]}
 before={name:hashlib.sha256(p.read_bytes()).hexdigest() for name,p in hooks.items()}
 binary.write_text('#!/bin/sh\nprintf \'%s\\0\' "$@" >> "$RAWCLAW_TEST_CALLS"\n: > "$RAWCLAW_TEST_DONE"\nexit 0\n');binary.chmod(0o755)
 work=root/'work';work.mkdir(); plain=work/'plain';plain.mkdir(); quoted=work/'a"b';quoted.mkdir(); transcript=root/'transcript.jsonl';transcript.write_text('')
 for runtime,hook in hooks.items():
  for case,cwd in [('plain',plain),('escaped-quote',quoted),('multiline-value',plain)]:
   sid='rc89-'+runtime+'-'+case
   event={'session_id':sid,'hook_event_name':'SessionStart','transcript_path':str(transcript),'cwd':str(cwd)}
   payload=json.dumps(event,separators=(',',':'))
   if case=='multiline-value':payload=payload.replace('"cwd":','"cwd":\n')
   assert json.loads(payload)==event
   catalog=root/(runtime+'-'+case+'-catalog');log=root/(runtime+'-'+case+'.calls');done=root/(runtime+'-'+case+'.done')
   caseenv=dict(env,RAWCLAW_CATALOG_DIR=str(catalog),RAWCLAW_TEST_CALLS=str(log),RAWCLAW_TEST_DONE=str(done))
   p=subprocess.run(['/bin/sh',str(hook)],input=payload.encode(),cwd=cwd,env=caseenv,capture_output=True,timeout=10)
   for _ in range(100):
    if done.exists():break
    time.sleep(.02)
   assert done.exists(),'detached stub did not complete'
   raw=(catalog/sid).read_bytes();record=json.loads(raw);calls=log.read_bytes().split(b'\0')[:-1]
   assert calls==[b'ingest',b'--',sid.encode()],calls
   def normalized(value):return value.replace(str(root),'<temp>')
   result={'runtime':runtime,'case':case,'payload_valid_json':True,'cwd_exists':cwd.is_dir(),'expected_cwd':normalized(str(cwd)),'actual_decoded_cwd':normalized(record['cwd']),'cwd_preserved':record['cwd']==str(cwd),'catalog_valid_json':True,'hook_exit':p.returncode,'banner':b'[rawclaw]' in p.stdout,'stub_argv':[x.decode() for x in calls],'stderr':p.stderr.decode()}
   print('CASE '+json.dumps(result,sort_keys=True))
   print('CATALOG '+normalized(raw.decode()).replace('\n','\\n'))
   assert p.returncode==0 and b'[rawclaw]' in p.stdout
   assert record['session_id']==sid and record['source']==runtime
   assert (record['cwd']==str(cwd))==(case=='plain')
   if runtime=='codex':assert json.loads(p.stdout)['hookSpecificOutput']['hookEventName']=='SessionStart'
 assert before=={name:hashlib.sha256(p.read_bytes()).hexdigest() for name,p in hooks.items()}
 print('generated_hooks_unchanged=true')
 print('side_effects=temporary setup config/scripts, fake executable/call logs, six catalog entries; no real ingest or live editor')
print('temporary_fixture_removed=true')
PY
```

Observed on Darwin with `/bin/sh`; the independent rerun produced the same six CASE and six CATALOG lines. Both complete probe processes exited 0. Selected output:

```text
BUILD CGO_ENABLED=0 go build -o <temp>/rawclaw ./cmd/rawclaw exit=0 stdout='' stderr=''
CASE {"actual_decoded_cwd": "<temp>/work/plain", "banner": true, "case": "plain", "catalog_valid_json": true, "cwd_exists": true, "cwd_preserved": true, "expected_cwd": "<temp>/work/plain", "hook_exit": 0, "payload_valid_json": true, "runtime": "claude", "stderr": "", "stub_argv": ["ingest", "--", "rc89-claude-plain"]}
CASE {"actual_decoded_cwd": "<temp>/work/a\\", "banner": true, "case": "escaped-quote", "catalog_valid_json": true, "cwd_exists": true, "cwd_preserved": false, "expected_cwd": "<temp>/work/a\"b", "hook_exit": 0, "payload_valid_json": true, "runtime": "claude", "stderr": "", "stub_argv": ["ingest", "--", "rc89-claude-escaped-quote"]}
CASE {"actual_decoded_cwd": "", "banner": true, "case": "multiline-value", "catalog_valid_json": true, "cwd_exists": true, "cwd_preserved": false, "expected_cwd": "<temp>/work/plain", "hook_exit": 0, "payload_valid_json": true, "runtime": "claude", "stderr": "", "stub_argv": ["ingest", "--", "rc89-claude-multiline-value"]}
CASE {"actual_decoded_cwd": "<temp>/work/plain", "banner": true, "case": "plain", "catalog_valid_json": true, "cwd_exists": true, "cwd_preserved": true, "expected_cwd": "<temp>/work/plain", "hook_exit": 0, "payload_valid_json": true, "runtime": "codex", "stderr": "", "stub_argv": ["ingest", "--", "rc89-codex-plain"]}
CASE {"actual_decoded_cwd": "<temp>/work/a\\", "banner": true, "case": "escaped-quote", "catalog_valid_json": true, "cwd_exists": true, "cwd_preserved": false, "expected_cwd": "<temp>/work/a\"b", "hook_exit": 0, "payload_valid_json": true, "runtime": "codex", "stderr": "", "stub_argv": ["ingest", "--", "rc89-codex-escaped-quote"]}
CASE {"actual_decoded_cwd": "", "banner": true, "case": "multiline-value", "catalog_valid_json": true, "cwd_exists": true, "cwd_preserved": false, "expected_cwd": "<temp>/work/plain", "hook_exit": 0, "payload_valid_json": true, "runtime": "codex", "stderr": "", "stub_argv": ["ingest", "--", "rc89-codex-multiline-value"]}
generated_hooks_unchanged=true
temporary_fixture_removed=true
```

The scripts, configuration files, stub logs and catalog entries exist only within `TemporaryDirectory` and are removed on return. This records reproduction evidence only; parsing, serialization, native hook protocol and adoption decisions remain open.

## Beads Codex typed decoder candidate

Date: 2026-11-06. Source-only delta; no adoption or implementation-readiness verdict.

Beads at immutable `c28ee2357ac808ed8e80bac29d7cc33441ceb5d4` supplies a typed Codex hook tuple: string `session_id`, `transcript_path`, `cwd`, `hook_event_name`, `model` and `trigger` in [`cmd/bd/codex_hook.go` L31-L38](https://github.com/gastownhall/beads/blob/c28ee2357ac808ed8e80bac29d7cc33441ceb5d4/cmd/bd/codex_hook.go#L31-L38). Its [decoder prefix, L65-L72](https://github.com/gastownhall/beads/blob/c28ee2357ac808ed8e80bac29d7cc33441ceb5d4/cmd/bd/codex_hook.go#L65-L72), calls `json.NewDecoder(stdin).Decode(&input)`, propagates errors except `io.EOF`, then lets a nonempty decoded event override the event argument. This establishes that specific schema and decoder/error relationship, not complete-input validation.

The isolated schema/decoder prefix ran on actual Go 1.24.0 with `CGO_ENABLED=0`. It preserved escaped quotes/newlines; accepted unknown fields, empty input, root/field `null`, duplicate keys (last value wins), trailing garbage and a second JSON value; and returned errors for a malformed first value, a number in a string field and an array root. A single `Decode` leaves trailing bytes unchecked. These are scratch probe observations, not upstream suite results. The fetched [upstream tests, `codex_hook_test.go` L23-L122](https://github.com/gastownhall/beads/blob/c28ee2357ac808ed8e80bac29d7cc33441ceb5d4/cmd/bd/codex_hook_test.go#L23-L122), were inspected, not executed; they cover context injection, a compaction warning and one-shot context refresh, not those decoder boundaries.

Reproduce the trailing-data boundary with Python 3, network access and Go; only the pinned schema and decoder prefix are extracted. The return expression is adapted to expose the decoded input; the wrapper, fixtures and printing are probe instrumentation. `TemporaryDirectory` removes the scratch module on return:

```sh
python3 - <<'PYTHON'
import os, pathlib, subprocess, tempfile, urllib.request
url = ('https://raw.githubusercontent.com/gastownhall/beads/'
       'c28ee2357ac808ed8e80bac29d7cc33441ceb5d4/cmd/bd/codex_hook.go')
lines = urllib.request.urlopen(url, timeout=30).read().decode().splitlines()
source = ('package main\n'
          'import("encoding/json";"fmt";"io";"strings")\n'
          + '\n'.join(lines[30:38])
          + '\nfunc decode(stdin io.Reader) (codexHookInput, error) {\n'
          + '\n'.join(lines[65:69]).replace('return err', 'return input, err')
          + '\nreturn input, nil\n}\n'
          + 'func main() { for _, s := range []string{'
          + '`{"cwd":"a"} garbage`, `{"cwd":"a"}{"cwd":"b"}`'
          + '} { v,e := decode(strings.NewReader(s));'
          + 'fmt.Printf("cwd=%q err=%v\\n",v.CWD,e) } }\n')
with tempfile.TemporaryDirectory(prefix='beads-hook-decoder-') as scratch:
    root = pathlib.Path(scratch)
    (root/'go.mod').write_text('module hookprobe\n\ngo 1.24.0\n')
    (root/'main.go').write_text(source)
    env = dict(os.environ, GOTOOLCHAIN='go1.24.0', CGO_ENABLED='0')
    subprocess.run(['gofmt', '-w', 'main.go'], cwd=root, check=True, env=env)
    subprocess.run(['go', 'version'], cwd=root, check=True, env=env)
    subprocess.run(['go', 'run', '.'], cwd=root, check=True, env=env)
PYTHON
```

Observed output:

```text
go version go1.24.0 darwin/arm64
cwd="a" err=<nil>
cwd="a" err=<nil>
```

The [MIT license](https://github.com/gastownhall/beads/blob/c28ee2357ac808ed8e80bac29d7cc33441ceb5d4/LICENSE#L1-L21) covers this source. The whole [module requires Go 1.27.3](https://github.com/gastownhall/beads/blob/c28ee2357ac808ed8e80bac29d7cc33441ceb5d4/go.mod#L1-L3), above RawClaw's Go 1.24 floor; the isolated prefix uses only `encoding/json` and `io`. The full handler also imports Cobra and other standard-library packages. Its [output writers, L134-L148](https://github.com/gastownhall/beads/blob/c28ee2357ac808ed8e80bac29d7cc33441ceb5d4/cmd/bd/codex_hook.go#L134-L148), serialize hook-response context, not catalog records or POSIX assignments. This does not establish RawClaw's shell transport, catalog serialization, claim/ingest timing or Antigravity mappings; combining it with separate primitives still needs source-backed relationships, a source-before-edit ACK and the ordinary provenance gate.
