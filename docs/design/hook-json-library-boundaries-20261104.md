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

## Sources

[1] https://github.com/tidwall/gjson/blob/690362d6edf4bcbde1e9f54d552d3814f1cd5bcb/gjson.go
[2] https://github.com/tidwall/gjson/blob/690362d6edf4bcbde1e9f54d552d3814f1cd5bcb/go.mod
[3] https://github.com/tidwall/gjson/blob/690362d6edf4bcbde1e9f54d552d3814f1cd5bcb/LICENSE
[4] https://github.com/MoonCaves/rawclaw/blob/754cf4ab3045f2196be8b32c3c7b69d48332af52/internal/cli/setup.go
