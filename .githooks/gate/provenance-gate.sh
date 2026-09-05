#!/bin/sh
# provenance-gate.sh — decision gate. Run as pre-push (blocks the push) and before a merge (--range). Not per commit.
# Stage 1 (deterministic): find staged logic under internal/, parse Prior-Art trailers, fetch the cited upstream
# lines by SHA. Stage 2 (model): hand doctrine + fetched upstream lines + diff to a fast model with a fixed prompt
# and a JSON schema; REJECT blocks the commit and prints the findings.
#
# Shape lifted from git's own hooks/commit-msg.sample (trailer parsing via `git interpret-trailers`) and
# kubernetes hack/verify-*.sh (read-only presubmit, non-zero on violation). Fail-CLOSED for code under internal/:
# a gate that cannot reach its model or its sources rejects, and says why. Fail-open for everything else.
#
# Usage:
#   by hand:   .githooks/gate/provenance-gate.sh --range <base>..<head>
#   as hook:   invoked by .githooks/pre-push; never from a branch copy
# Env:
#   PROVENANCE_MODEL   agy model name (default: gemini-3.8-flash-low, fastest tier in `agy models`)
#   PROVENANCE_TIMEOUT seconds for the model call (default 90)
set -u
# GATE_DIR is where THIS script lives (the tracked hooks dir); prompt and schema are read from here, never from
# the branch under test, so a branch cannot weaken its own gate (found 2026-09-05: a worktree ran a stale copy).
GATE_DIR=$(cd "$(dirname "$0")" && pwd)
ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$ROOT" || exit 0
MODEL="${PROVENANCE_MODEL:-gemini-3.8-flash-low}"
TIMEOUT="${PROVENANCE_TIMEOUT:-90}"
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT

if [ "${1:-}" = "--range" ]; then
  RANGE="$2"; MSG_TEXT=$(git log --format=%B "$RANGE"); DIFF_CMD="git diff $RANGE"
else
  MSG_FILE="${1:-}"; [ -r "$MSG_FILE" ] || exit 0
  MSG_TEXT=$(cat "$MSG_FILE"); DIFF_CMD="git diff --cached"
fi

# ---- Stage 0: is there any staged logic under internal/ at all? -------------------------------------------
FILES=$($DIFF_CMD --name-only -- 'internal/**/*.go' ':!internal/**/*_test.go' 2>/dev/null)
[ -n "$FILES" ] || exit 0
# Any added line that is not a comment, blank, import, or bare brace counts (RubyHeron 346 Attack A:
# a keyword regex let `x = evaluateCustomHeuristic(a, b)` through). Lifted from the shape of
# kubernetes hack/verify-boilerplate.sh: every file in scope is checked, no keyword whitelist.
# Added OR removed lines count (WhiteGorge 357 #6: deleting an upstream safeguard is a decision too).
LOGIC=0
for f in $FILES; do
  if $DIFF_CMD -U0 -- "$f" | grep -E '^[-+][^-+]' \
     | grep -Ev '^[-+][[:space:]]*(//|$|import |package |"[^"]*"$|\)$|\}$|\{$)' | grep -q .; then LOGIC=1; fi
done
if [ "$LOGIC" = 0 ]; then exit 0; fi

# No skip variable. (RubyHeron 346 Attack C: an untracked skip log is invisible to git.) The only escape is
# `Prior-Art: wiring-only`, which the model still checks, and which CI sees in the commit message.

# ---- Stage 1: trailers -> upstream bytes ----------------------------------------------------------------------
# Trailer forms accepted:  Prior-Art: owner/repo path L10-L20 (MIT) @<sha>     |  path#L10-L20  |  wiring-only
printf '%s\n' "$MSG_TEXT" | grep -E '^Prior-Art:' > "$TMP/trailers" || true
if grep -q 'wiring-only' "$TMP/trailers"; then
  echo "[provenance-gate] wiring-only declared; model still checks that it is wiring." >&2
fi
: > "$TMP/upstream.txt"
FETCH_FAIL=0
while IFS= read -r t; do
  case "$t" in *wiring-only*) continue;; esac
  repo=$(printf '%s' "$t" | awk '{print $2}')
  path=$(printf '%s' "$t" | awk '{print $3}' | sed 's/#L.*$//')
  lines=$(printf '%s' "$t" | grep -oE 'L?[0-9]+-L?[0-9]+' | head -1 | tr -d L)
  if ! grep -qF -- "$repo" docs/design/decision-references.md docs/design/prior-art-steal-list.md 2>/dev/null; then
    echo "[provenance-gate] REJECT (deterministic): $repo is not in docs/design/decision-references.md or prior-art-steal-list.md. Add it there first, in its own commit, with the file and lines (357 #12: no vanity forks)." >&2; exit 1
  fi
  sha=$(printf '%s' "$t" | grep -oE '@[0-9a-f]{40}' | tr -d @)
  a=${lines%-*}; b=${lines#*-}
  if [ -z "$sha" ]; then
    echo "[provenance-gate] REJECT (deterministic): trailer has no 40-hex commit SHA: $t" >&2; exit 1
  fi
  ref=$sha
  url="https://raw.githubusercontent.com/$repo/$ref/$path"
  if curl -fsSL --max-time 20 "$url" > "$TMP/src" 2>/dev/null; then
    { printf '\n===== %s %s L%s-L%s @%s =====\n' "$repo" "$path" "$a" "$b" "$ref"; sed -n "${a},${b}p" "$TMP/src"; } >> "$TMP/upstream.txt"
    if [ "$(sed -n "${a},${b}p" "$TMP/src" | wc -l | tr -d ' ')" = 0 ]; then
      echo "[provenance-gate] REJECT (deterministic): $repo $path has no lines $a-$b at $sha" >&2; exit 1
    fi
  else
    echo "[provenance-gate] REJECT (deterministic): cannot fetch $url" >&2; exit 1
  fi
done < "$TMP/trailers"

# 357 #22: every added `// policy:` must point at a measurement file under docs/design that exists.
for pf in $($DIFF_CMD -U0 -- $FILES | grep -E '^\+.*// policy:' | grep -oE 'docs/design/[A-Za-z0-9._/-]+' | sort -u); do
  [ -f "$pf" ] || { echo "[provenance-gate] REJECT (deterministic): policy label cites $pf which does not exist." >&2; exit 1; }
done
if $DIFF_CMD -U0 -- $FILES | grep -E '^\+.*// policy:' | grep -vqE 'docs/design/'; then
  echo "[provenance-gate] REJECT (deterministic): a // policy: label must cite its measurement as docs/design/<file>[#anchor]. Format: // policy: <date> <what> see docs/design/<file>" >&2; exit 1
fi

# ---- Stage 2: model verdict ------------------------------------------------------------------------------------
$DIFF_CMD -- $FILES > "$TMP/diff.patch"
{
  echo "DOCTRINE:"; cat docs/agents/doctrine.md 2>/dev/null || echo "(doctrine.md missing: treat every logic line as DRIFT)"
  echo; echo "TRAILERS AND FETCHED UPSTREAM LINES:"; cat "$TMP/trailers"; cat "$TMP/upstream.txt"
  echo; echo "DIFF:"; cat "$TMP/diff.patch"
  echo; cat "$GATE_DIR/provenance-gate-prompt.md"
} > "$TMP/prompt.txt"

if ! command -v agy >/dev/null 2>&1; then
  echo "[provenance-gate] REJECT: agy not found; the gate cannot run. Install it or set PROVENANCE_SKIP=1 (logged)." >&2; exit 1
fi
TO=timeout; command -v timeout >/dev/null 2>&1 || { command -v gtimeout >/dev/null 2>&1 && TO=gtimeout || TO=""; }
# agy reads the prompt from the -p argument, not stdin (verified 2026-09-05: stdin form errors out).
if ! $TO ${TO:+"$TIMEOUT"} agy --model "$MODEL" --output-format json --json-schema "$GATE_DIR/provenance-gate.schema.json" \
      -p "$(cat "$TMP/prompt.txt")" > "$TMP/verdict.json" 2> "$TMP/agy.err"; then
  echo "[provenance-gate] REJECT: model call failed or timed out ($TIMEOUT s). stderr:" >&2; tail -5 "$TMP/agy.err" >&2; exit 1
fi
[ -n "${PROVENANCE_DEBUG:-}" ] && cp "$TMP/verdict.json" /tmp/pg-verdict.json && cp "$TMP/agy.err" /tmp/pg-agy.err && cp "$TMP/prompt.txt" /tmp/pg-prompt.txt
VERDICT=$(python3 - "$TMP/verdict.json" "$TMP/upstream.txt" <<'EOF'
import json,sys
raw=open(sys.argv[1]).read()
import re
def norm(t): return re.sub(r"\s+"," ",t).strip()
upstream=norm(open(sys.argv[2]).read()) if len(sys.argv)>2 else ""
try:
    d=json.loads(raw)
except Exception:
    # agy json output may wrap the result; find the first object containing "verdict"
    import re
    m=re.search(r'\{[^{}]*"verdict"[^{}]*\}', raw, re.S); d=json.loads(m.group(0)) if m else {}
# agy --output-format json wraps the model text under "response" (observed 2026-09-05); older builds used "result".
for key in ("response","result"):
    if isinstance(d,dict) and key in d and isinstance(d[key],str):
        try: d=json.loads(d[key]); break
        except Exception: pass
verdict=d.get("verdict","REJECT")
# Deterministic backstop: a FOUND must quote upstream bytes that actually exist. Otherwise it is MADE.
for f in d.get("findings",[]):
    if f.get("class")=="FOUND":
        q=norm(f.get("upstream_quote") or "")
        if len(q)<12 or q not in upstream:
            f["class"]="MADE"; f["reason"]="FOUND without a verifiable upstream quote -> MADE. "+str(f.get("reason",""))
            verdict="REJECT"
print(verdict)
for f in d.get("findings",[]):
    if f.get("class")=="MADE":
        print(f"  MADE {f.get('file')}:{f.get('line')} {f.get('reason')}", file=sys.stderr)
EOF
)
if [ "$FETCH_FAIL" = 1 ]; then echo "[provenance-gate] one or more cited sources could not be fetched (see above)." >&2; fi
# Verdict ledger (357 #21) as a git note on HEAD: a tracked git object, pushed with refs/notes/*, no worktree
# mutation, readable from any clone. Lifted from git's own notes mechanism (git/git Documentation/git-notes.txt);
# suggested by RubyHeron 366. A REJECT followed by an APPROVE on the same diff hash is visible in `git notes
# --ref=decision-gate show <sha>` history.
DIFF_HASH=$(shasum -a 256 "$TMP/diff.patch" | cut -c1-12)
NOTE=$(printf '%s range=%s diff=%s model=%s verdict=%s' "$(date -u +%FT%TZ)" "${RANGE:-staged}" "$DIFF_HASH" "$MODEL" "$VERDICT")
git notes --ref=decision-gate append -m "$NOTE" HEAD >/dev/null 2>&1 || true
printf '%s\n' "$NOTE" >> "$(git rev-parse --git-common-dir)/decision-gate.log"
case "$VERDICT" in
  APPROVE) echo "[provenance-gate] APPROVE ($MODEL)" >&2; exit 0;;
  *) echo "[provenance-gate] REJECT ($MODEL): a decision above was MADE by an agent instead of FOUND on the internet. Cite the source line in a Prior-Art trailer, or label a measured constant // policy:." >&2; exit 1;;
esac
