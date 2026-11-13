# Session Takeover Path Measurements (2026-09-09)

Measurements supporting session takeover and bounded read path hardening:

## 1. D1: All-digit hex uuid8 refs
- In `internal/agentproto/types.go`, the check `reNumericRef = regexp.MustCompile("^[0-9]+$")` was rejecting valid hex UUID prefixes.
- Measurement across corpus: 17,649 / 756,602 messages (2.33%) have an all-digit uuid8.
- Dropping `reNumericRef` eliminates this 2.33% false rejection rate (Doctrine Rule 4: Losers get deleted).

## 2. D2: System prompt anchor capping
- In `internal/view/view.go`, reading an anchor whole (`cap = -1`) causes massive context dumps when the anchor is a `system` message.
- Measurement across corpus: 134 topic segments anchor on a system runtime injection.
- Largest observed system prompt anchor is 41,669 characters in session 01a07ab9.
- Policy: cap non-conversational `system` anchors to `dispCap` (defined in `internal/view/view.go`) unless `--with tools` explicitly requests raw tool/system data.

## 3. D3: Single-session topic spine retrieval
- In `internal/cli/cmd_topics.go`, adding `--session <id>` enables reading a single session's chronological topic spine with start refs and summaries.
- Avoids multi-turn search roundtrips when taking over a session.

## Verification on 2026-11-13: outline and direct system reads

Source: created here from an isolated native CLI replay of the recovered real
Codex session `01a07ab9`, followed by an independent build and default-outline
and direct-read check. This dated addition leaves the September measurements
above unchanged. **The outline flood was not reproduced for this one session.**
It is not a claim about every session or a closure of issue #101.

### Recorded inputs and binary

- Source revision: `eb0529059c150cd439842b99d6624f7acbfedce9`; tree: `5bee7f12d08186336d762c20b4fe879306fdf04a`.
- Recovered transcript artifact SHA256: `46348b3d500e47027d559782c16277ebd27017c9c302999473d345f4b675f4eb`.
- Go 1.24.0, Darwin arm64, `CGO_ENABLED=0`; recorded binary SHA256: `c3d41f95a92dd5510561a15621c8ffa454041a92750a7105386a43a25fd389be`.
- The independent build produced the same binary hash. That hash identifies this build artifact, not a promise of identical binaries across platforms or build environments.

The copied historical artifact matched its existing recovered source copy byte
for byte. This does not verify that today's original live transcript is
unchanged. Native ingestion into fresh isolated stores produced 1,849 messages,
including 79 `system` rows; the largest system row was 41,669 runes. Its native
UUID was `7a65e6d5431648d5`.

### Observed behavior

All six native commands below exited 0 with empty stderr. JSON observations
were reduced to metadata; transcript and rendered message contents are not
included here.

| Command after ingestion | Output bytes | Relevant metadata |
| --- | ---: | --- |
| `outline <session> --json --timeout 60s` | 2,572 | 4 start / 4 end records; 0 system anchors; maximum preview 300 runes |
| `outline <session> --with tools,thinking --json --timeout 60s` | 2,697 | 4 start / 4 end records; 0 system anchors; maximum preview 300 runes |
| `outline <session> --timeout 60s` | 1,942 | Default human rendering |
| `read <system-ref> --json --timeout 60s` | 1,761 | System anchor 200 runes; maximum neighbor text 200 runes |
| `read <system-ref> --with tools,thinking --json --timeout 60s` | 45,072 | Same system anchor 41,669 runes; maximum neighbor text 200 runes |

The sixth command was `ingest <session> --json`: 1,849 messages, source `codex`,
144 output bytes. Neither read supplied `--budget`; both returned
`truncated=false` and `char_budget=null`. Those global-budget fields did not
report the per-message display cap in this observation; this note makes no new
metadata-policy or defect ruling.

At the pinned revision, [`BookendMessages`](../../internal/store/messages.go)
selects non-empty `user`/`assistant` rows. [`Outline`](../../internal/agentproto/outline.go)
renders those bookends with `outlineDispCap=300`, including when tools and
thinking are requested. Direct [`BuildAnchoredView`](../../internal/view/view.go)
uses the separate `dispCap=200` for a system anchor by default and removes that
anchor cap when `IncludeTools` is true. The existing read help describes
`--with tools,thinking` without `--budget` as the full-system-anchor override.
The bookend regression fixture and these runtime observations test different
boundaries.

### Isolated replay command shapes

These are portable command shapes, not verbatim execution receipts. In a clean
checkout of the pinned revision, build with Go 1.24.0 and `CGO_ENABLED=0`, setting
`binary` to the resulting binary's absolute path. Set `source_jsonl` to the
recovered artifact, verify its SHA256 above, and keep raw outputs local. The
artifact is not distributed with this repository.

```sh
scratch=$(mktemp -d)
session=01a07ab9-1dc9-7510-a288-5d651a3be665
mkdir -p "$scratch/home/.codex/sessions" "$scratch/config/claude" \
  "$scratch/cache" "$scratch/data" "$scratch/catalog"
cp "$source_jsonl" "$scratch/home/.codex/sessions/$(basename "$source_jsonl")"
env -i PATH=/usr/bin:/bin HOME="$scratch/home" \
  CODEX_HOME="$scratch/home/.codex" CLAUDE_CONFIG_DIR="$scratch/config/claude" \
  XDG_CACHE_HOME="$scratch/cache" XDG_DATA_HOME="$scratch/data" \
  XDG_CONFIG_HOME="$scratch/config" RAWCLAW_CATALOG_DIR="$scratch/catalog" \
  BINARY="$binary" SESSION="$session" sh <<'REPLAY'
set -e
"$BINARY" ingest "$SESSION" --json > "$HOME/ingest.json"
"$BINARY" outline "$SESSION" --json --timeout 60s > "$HOME/outline.json"
"$BINARY" outline "$SESSION" --with tools,thinking --json --timeout 60s > "$HOME/outline-with.json"
"$BINARY" outline "$SESSION" --timeout 60s > "$HOME/outline.txt"
"$BINARY" read "$SESSION:7a65e6d5" --json --timeout 60s > "$HOME/read.json"
"$BINARY" read "$SESSION:7a65e6d5" --with tools,thinking --json --timeout 60s > "$HOME/read-with.json"
REPLAY
jq '{start_count:(.start|length),end_count:(.end|length),system_anchors:([.start[],.end[]]|map(select(.role=="system"))|length),max_text_runes:([.start[],.end[]]|map(.text|length)|max)}' "$scratch/home/outline.json" "$scratch/home/outline-with.json"
jq '{anchor:([.window[]|select(.anchor)|{role,text_runes:(.text|length)}]),truncated,char_budget}' "$scratch/home/read.json" "$scratch/home/read-with.json"
```

### Limits still open

The historical 134 figure counts system-anchored topic segments, not 134 sampled
sessions; 41,669 was the largest observed anchor, not an average. This replay
covers one recovered Codex transcript, not the complete historical corpus.
The original SQL measurement and original bulk-read invocation, output and
revision remain **OPEN**. This evidence selects no new display cap, changes no
CLI semantics, and supplies no runtime fix.
