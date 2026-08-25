#!/bin/sh
set -eu

# Measure root-query search latency while targeted ingest writers share the
# same isolated SQLite store. RawClaw's search syntax is `rawclaw [query...]`;
# there is no separate `search` subcommand.

usage() {
	cat <<EOF
Usage: bench-concurrency.sh --binary PATH [options]

Options:
  -b, --binary PATH       rawclaw binary to benchmark (required)
  -n, --searchers N       parallel search workers (default: ${N})
  -m, --writers N         parallel ingest writers (default: ${M})
  -d, --duration SEC      benchmark duration in seconds (default: ${D})
  -o, --output PATH       latency CSV path (default: ${OUTPUT})
  -q, --query TEXT        search query (default: ${QUERY})
  -h, --help              show this help
EOF
}

N=${BENCH_SEARCHERS:-20}
M=${BENCH_WRITERS:-3}
D=${BENCH_DURATION:-60}
OUTPUT=${BENCH_OUTPUT:-bench-concurrency.csv}
QUERY=${BENCH_QUERY:-concurrency benchmark beacon}
BINARY=${RAWCLAW_BINARY:-}

while [ "$#" -gt 0 ]; do
	case "$1" in
	-b|--binary)
		[ "$#" -ge 2 ] || { echo "missing value for $1" >&2; exit 2; }
		BINARY=$2
		shift 2
		;;
	-n|--searchers)
		[ "$#" -ge 2 ] || { echo "missing value for $1" >&2; exit 2; }
		N=$2
		shift 2
		;;
	-m|--writers)
		[ "$#" -ge 2 ] || { echo "missing value for $1" >&2; exit 2; }
		M=$2
		shift 2
		;;
	-d|--duration)
		[ "$#" -ge 2 ] || { echo "missing value for $1" >&2; exit 2; }
		D=$2
		shift 2
		;;
	-o|--output)
		[ "$#" -ge 2 ] || { echo "missing value for $1" >&2; exit 2; }
		OUTPUT=$2
		shift 2
		;;
	-q|--query)
		[ "$#" -ge 2 ] || { echo "missing value for $1" >&2; exit 2; }
		QUERY=$2
		shift 2
		;;
	-h|--help)
		usage
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

case "$N:$M:$D" in
	*[!0-9:]*|*::*|:*)
		echo "searchers, writers, and duration must be integers" >&2
		exit 2
		;;
esac
[ "$N" -gt 0 ] || { echo "searchers must be positive" >&2; exit 2; }
[ "$M" -gt 0 ] || { echo "writers must be positive" >&2; exit 2; }
[ "$D" -gt 0 ] || { echo "duration must be positive" >&2; exit 2; }
[ -n "$BINARY" ] || { echo "--binary is required" >&2; exit 2; }
[ -x "$BINARY" ] || { echo "binary is not executable: $BINARY" >&2; exit 2; }

case "$OUTPUT" in
	/*) output_dir=${OUTPUT%/*}; [ -n "$output_dir" ] || output_dir=/ ;;
	*) output_dir=.;;
esac
mkdir -p "$output_dir"

now_ns() {
	now=$(date +%s%N 2>/dev/null || true)
	case "$now" in
		*[!0-9]*|"")
			if command -v python3 >/dev/null 2>&1; then
				python3 -c 'import time; print(time.time_ns())'
			elif command -v perl >/dev/null 2>&1; then
				perl -MTime::HiRes=time -e 'printf "%.0f\n", time() * 1000000000'
			else
				echo "need date +%N, python3, or perl for nanosecond timing" >&2
				exit 1
			fi
			;;
		*) printf '%s\n' "$now" ;;
	esac
}

work=$(mktemp -d "${TMPDIR:-/tmp}/rawclaw-concurrency.XXXXXX")
cleanup() {
	status=$?
	trap - EXIT INT TERM
	[ "$status" -eq 0 ] || echo "benchmark failed; retained artifacts: $work" >&2
	if [ "$status" -eq 0 ]; then
		rm -rf "$work"
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# HOME controls the cache (`~/.cache/session-search`). The other overrides
# isolate durable transcript/catalog/config paths as well.
export HOME=$work/home
export CLAUDE_CONFIG_DIR=$HOME/.claude
export XDG_DATA_HOME=$HOME/.local/share
export XDG_CACHE_HOME=$HOME/.cache
export RAWCLAW_CATALOG_DIR=$XDG_DATA_HOME/rawclaw/catalog
export RAWCLAW_ARCHIVE=off
export RAWCLAW_BACKGROUND_INGEST=off
unset CLAUDE_CODE_SESSION_ID ANTIGRAVITY_CONVERSATION_ID RAWCLAW_EMBED_ENDPOINT RAWCLAW_EMBED_KEY

project_dir=$CLAUDE_CONFIG_DIR/projects/cbench-project
mkdir -p "$project_dir"

session_for_writer() {
	printf 'cbench-%02d-0000-0000-000000000000' "$1"
}

seed_session() {
	idx=$1
	sid=$(session_for_writer "$idx")
	transcript=$project_dir/$sid.jsonl
	cat >"$transcript" <<EOF
{"type":"user","message":{"role":"user","content":"$QUERY"},"uuid":"$sid-user","timestamp":"2026-08-25T10:00:00Z","cwd":"$work/project"}
{"type":"assistant","message":{"role":"assistant","content":"The concurrency benchmark beacon is indexed in the isolated harness corpus."},"uuid":"$sid-assistant","timestamp":"2026-08-25T10:00:01Z"}
EOF
}

i=1
while [ "$i" -le "$M" ]; do
	seed_session "$i"
	i=$((i + 1))
done

# Prime the index and fail before the timed phase if the real CLI cannot ingest.
prime_id=$(session_for_writer 1)
if ! "$BINARY" --timeout 0 ingest "$prime_id" >"$work/prime.out" 2>"$work/prime.err"; then
	echo "initial ingest failed:" >&2
	cat "$work/prime.err" >&2
	exit 1
fi

deadline=$(( $(date +%s) + D ))
search_pids=
writer_pids=

run_searcher() {
	worker=$1
	latencies=$work/search-$worker.csv
	errors=$work/search-$worker.err
	: >"$latencies"
	: >"$errors"
	seq=1
	while [ "$(date +%s)" -lt "$deadline" ]; do
		start=$(now_ns)
		if "$BINARY" --timeout 0 --no-vector --json "$QUERY" >"$work/search-$worker.out" 2>>"$errors"; then
			end=$(now_ns)
			printf '%s,%s,%s\n' "$worker" "$seq" $((end - start)) >>"$latencies"
		else
			printf 'search worker %s sequence %s failed\n' "$worker" "$seq" >>"$errors"
			exit 1
		fi
		seq=$((seq + 1))
	done
}

run_writer() {
	worker=$1
	sid=$(session_for_writer "$worker")
	errfile=$work/writer-$worker.err
	: >"$errfile"
	while [ "$(date +%s)" -lt "$deadline" ]; do
		touch "$project_dir/$sid.jsonl"
		if ! "$BINARY" --timeout 0 ingest "$sid" >/dev/null 2>>"$errfile"; then
			printf 'writer %s ingest failed\n' "$worker" >>"$errfile"
			return 1
		fi
	done
}

i=1
while [ "$i" -le "$N" ]; do
	run_searcher "$i" &
	search_pids="$search_pids $!"
	i=$((i + 1))
done
i=1
while [ "$i" -le "$M" ]; do
	run_writer "$i" &
	writer_pids="$writer_pids $!"
	i=$((i + 1))
done

failed=0
for pid in $search_pids $writer_pids; do
	if ! wait "$pid"; then
		failed=1
	fi
done

if [ "$failed" -ne 0 ]; then
	for file in "$work"/*.err; do
		[ -s "$file" ] || continue
		echo "--- $file ---" >&2
		sed -n '1,80p' "$file" >&2
	done
	exit 1
fi

{
	echo 'worker,sequence,latency_ns'
	for file in "$work"/search-*.csv; do
		cat "$file"
	done
} >"$OUTPUT"

count=$(awk 'NR > 1 { n++ } END { print n + 0 }' "$OUTPUT")
[ "$count" -gt 0 ] || { echo "no search samples recorded" >&2; exit 1; }
sort -t, -k3,3n "$OUTPUT" | awk -F, -v count="$count" '
	NR == 1 { next }
	{
		values[++n] = $3
		if ($3 > max) max = $3
	}
	END {
		p50 = int((count * 50 + 99) / 100)
		p95 = int((count * 95 + 99) / 100)
		printf "samples=%d p50_ms=%.3f p95_ms=%.3f max_ms=%.3f\n", count, values[p50] / 1000000, values[p95] / 1000000, max / 1000000
	}'
