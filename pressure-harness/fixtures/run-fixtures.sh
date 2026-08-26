#!/usr/bin/env bash
set -Eeuo pipefail
fixture_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
auditor="${fixture_dir}/../pressure-auditor.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/pressure-fixtures.XXXXXX")"
trap 'rm -rf -- "${tmp}"' EXIT
run() { local name="$1"; shift; local late=0; [[ "${name}" == late || "${name}" == three ]] && late=1; PRESSURE_HARNESS_ROOT="${tmp}/${name}" PRESSURE_NOW=1800 PRESSURE_LATE="${late}" "${auditor}" "$@"; }
printf '%s\n' 'fixture present'; run present --dry-run --once | grep -q 'misses=0'
printf '%s\n' 'fixture late'; mkdir -p "${tmp}/late/state"; printf '0\n' >"${tmp}/late/state/misses"; run late --once >/dev/null
grep -q 'misses=1 escalation=warn' "${tmp}/late/state/receipts.log"
printf '%s\n' 'fixture 3-miss'; mkdir -p "${tmp}/three/state"; printf '2\n' >"${tmp}/three/state/misses"; run three --once >/dev/null
grep -q 'stop-and-escalate' "${tmp}/three/state/receipts.log"
printf '%s\n' 'fixture duplicate'; run duplicate --once >/dev/null; run duplicate --once >/dev/null
[[ "$(wc -l <"${tmp}/duplicate/state/receipts.log")" -eq 2 ]]
printf '%s\n' 'fixture STOP'; run stop --dry-run stop | grep -q 'no sentinel written'; run stop stop >/dev/null
[[ -e "${tmp}/stop/state/STOP" ]]
printf '%s\n' 'all fixtures passed'
