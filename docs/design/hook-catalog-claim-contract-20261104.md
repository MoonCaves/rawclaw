# Hook catalog claim behavior — 2026-11-04

Source: existing code at `0afecdfab2321abb49733746f7250e6572d5ec5e`, inspected for issue #89. This report describes that snapshot; it does not introduce a new operational contract or certify installed hooks. Production code is unchanged.

## Finding

**VERIFIED:** the shared Claude/Codex/Antigravity startup catalog mechanism records a durable claim **before** starting detached ingest. It does not represent successful ingestion, a running-process lock, or a lease. A successful hard-link claim launches ingest; an existing entry (including a symlink) suppresses another startup launch; inability to claim with no existing entry falls back to launching ingest.[1]

**VERIFIED:** no expiry or success-dependent release exists in this mechanism. The temporary file/link name is removed after the claim attempt; the final catalog entry remains. The existing test explicitly covers a second startup with a persistent catalog, including its simulated-reboot interpretation.[1][2] The freshness reader explicitly allows a catalog entry to outlive a deleted transcript.[3]

**VERIFIED, bounded cleanup finding:** the inspected Claude/Codex eject function and separate Antigravity eject function remove hook scripts, hook registrations, and empty configuration directories; they do not delete catalog entries.[4][5] A repository search of `CatalogDir` consumers and catalog cleanup/removal terms found no production catalog expiry or purge path in this snapshot. This does not exclude external deletion, a different version, or an externally configured catalog directory. Removing an entry externally permits a later startup claim again.

**VERIFIED:** a nonempty unsafe filename ID skips catalog creation and launches fail-soft ingest with the original session ID. Repeating that startup fallback can repeat the launch. This is separate from deliberate refresh cadence: Antigravity nonzero invocations ingest for **both** valid and invalid IDs before the startup catalog block; Claude `UserPromptSubmit` also ingests independently.[6][7]

## Failure counterexample

The extracted shared claim code below runs in a temporary catalog. A mocked `nohup` records arguments and fails; no RawClaw ingest runs. The first startup still leaves a claim, the second is suppressed, and deleting only the temporary final marker permits a new launch. This is a shell-fragment probe, not an installed-hook integration test or evidence that real ingest failed.

Command, from the inspected repository root:

```python
from pathlib import Path
import re, subprocess, tempfile, os
s = Path('internal/cli/setup.go').read_text()
head = re.search(r'const rawclawSessionCatalogHead = `([^`]+)`', s).group(1)
tail = re.search(r'const rawclawSessionCatalogTail = `([^`]+)`', s).group(1)
script = '''exec 3>&1
nohup() { printf 'INGEST_FAIL:%s|%s|%s|%s\\n' "$1" "$2" "$3" "$4" >&3; return 1; }
RAWCLAW=rawclaw
session_id=valid-id
catalog_session_id=$session_id
input='{"transcript_path":"/missing-transcript","cwd":"/workspace"}'
''' + head + '''printf '  "source": "claude"\\n'
''' + tail + '\nwait\n'
with tempfile.TemporaryDirectory() as d:
    env = os.environ | {'RAWCLAW_CATALOG_DIR': d}
    for n in (1, 2):
        r = subprocess.run(['/bin/sh', '-c', script], env=env,
                           capture_output=True, text=True)
        print(f'run={n} exit={r.returncode} marker={(Path(d)/"valid-id").is_file()} output={r.stdout.strip()!r}')
    (Path(d)/'valid-id').unlink()
    r = subprocess.run(['/bin/sh', '-c', script], env=env,
                       capture_output=True, text=True)
    print(f'after-temp-marker-deletion exit={r.returncode} marker={(Path(d)/"valid-id").is_file()} output={r.stdout.strip()!r}')
```

Output:

```text
run=1 exit=0 marker=True output='INGEST_FAIL:rawclaw|ingest|--|valid-id'
run=2 exit=0 marker=True output=''
after-temp-marker-deletion exit=0 marker=True output='INGEST_FAIL:rawclaw|ingest|--|valid-id'
```

The hook's zero status is not a successful-ingest receipt. Detached child completion is not observed by the claim mechanism. A later per-turn refresh or explicit ingest may still ingest the transcript; this probe does not claim the session is permanently unindexable.

## Implications for a narrowly scoped repair

**RECOMMENDATION:** evaluate invalid-ID startup dedup against the existing valid-ID claim behavior first. Matching durable claim-before-launch behavior does not require inventing a success-dependent retry policy. A requirement to expire or release a claim after ingest failure would add behavior that the existing valid-ID startup mechanism lacks. That may be useful separately, but it is a broader change rather than a prerequisite established by valid-ID parity.

**VERIFIED limitation:** startup claim identity is currently a flat native-session filename shared by these sources, not a source-qualified opaque key. Catalog resolution checks the exact filename, then filters directory names by the requested native-ID prefix **before** parsing JSON.[8] Ingest also tries the native session catalog path before other discovery routes.[9] A hash in the current flat catalog cannot be assumed to preserve those lookups. Source-qualified marker isolation would itself change current cross-source claim identity, even if desirable.

**OPEN:** the storage/encoding and discovery consequences of supporting unsafe IDs still need a concrete implementation and review. A private marker namespace could avoid confusing catalog discovery, but this report neither chooses one nor supplies implementation provenance. No claim is made that a cache-hashing primitive alone supplies an upstream hook-dedup implementation. Deliberate nonzero-invocation/UserPromptSubmit refresh and original ingest arguments must remain visible in any proposed repair.

No repository tests or benchmarks were run: the change is a source report, and the only execution was the bounded shell-fragment probe above. Owning documentation was checked; the root `AGENTS.md` Index already covers `docs/design/`, and no product contract or structure moved.

## Immutable sources

[1] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/setup.go#L26-L63 — shared claim, temporary cleanup, launch and existing-entry suppression

[2] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/catalog_hook_test.go#L320-L336 — persistent second-startup expectation; inspected, not executed for this report

[3] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/index/consolidated.go#L1558-L1566 — missing transcript does not remove retained catalog history

[4] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/setup.go#L1065-L1124 — Claude/Codex eject scope

[5] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/setup.go#L1237-L1280 — Antigravity eject scope

[6] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/setup.go#L90-L115 — Claude validation, UserPromptSubmit and unsafe-ID startup fallback

[7] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/setup.go#L224-L265 — Antigravity validation, deliberate later-turn refresh and unsafe-ID startup fallback

[8] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/paths/paths.go#L414-L453 — native filename/prefix catalog resolution

[9] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/cmd_ingest.go#L153-L190 — native session ingest discovery
