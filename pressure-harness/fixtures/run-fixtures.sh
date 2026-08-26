#!/usr/bin/env bash

set -Eeuo pipefail

fixture_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
auditor="${fixture_dir}/../pressure-auditor.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/pressure-fixtures.XXXXXX")"
trap 'rm -rf -- "${tmp}"' EXIT

run() {
  local name="$1"; shift
  local late=0
  [[ "${name}" == late || "${name}" == three || "${name}" == duplicate ]] && late=1
  PRESSURE_HARNESS_ROOT="${tmp}/${name}" PRESSURE_NOW=1800 PRESSURE_LATE="${late}" "$@"
}

printf '%s\n' 'fixture present'
run present "${auditor}" --dry-run --once | grep -q 'misses=0'

printf '%s\n' 'fixture late'
mkdir -p "${tmp}/late/state"
printf '0\n' >"${tmp}/late/state/misses"
run late "${auditor}" --once >/dev/null
grep -q 'misses=1 escalation=warn' "${tmp}/late/state/receipts.log"

printf '%s\n' 'fixture 3-miss STOP and loop exit'
mkdir -p "${tmp}/three/state"
printf '2\n' >"${tmp}/three/state/misses"
PRESSURE_HARNESS_ROOT="${tmp}/three" PRESSURE_NOW=1800 PRESSURE_LATE=1 PRESSURE_SLEEP_SECONDS=0 "${auditor}" start >/dev/null
[[ -e "${tmp}/three/state/STOP" ]]
[[ ! -e "${tmp}/three/state/pid" ]]
[[ "$(wc -l <"${tmp}/three/state/receipts.log")" -eq 1 ]]
! grep -q 'misses=4\|escalation=.*four' "${tmp}/three/state/receipts.log"

printf '%s\n' 'fixture duplicate same slot'
mkdir -p "${tmp}/duplicate/state"
printf '1\n' >"${tmp}/duplicate/state/misses"
run duplicate "${auditor}" --once >/dev/null
run duplicate "${auditor}" --once >"${tmp}/duplicate/second.out"
grep -q 'idempotent=1' "${tmp}/duplicate/second.out"
[[ "$(wc -l <"${tmp}/duplicate/state/receipts.log")" -eq 1 ]]
! grep -q 'misses=3\|misses=4' "${tmp}/duplicate/state/receipts.log"
[[ "$(find "${tmp}/duplicate/mailbox" -type f -name '*.receipt' -print | wc -l | tr -d ' ')" -eq 1 ]]

printf '%s\n' 'fixture send failure does not advance state'
mkdir -p "${tmp}/send-failure/state"
printf '1\n' >"${tmp}/send-failure/state/misses"
touch "${tmp}/send-failure/mailbox"
if PRESSURE_HARNESS_ROOT="${tmp}/send-failure" PRESSURE_NOW=1800 PRESSURE_LATE=1 "${auditor}" --once >/dev/null 2>&1; then
  printf '%s\n' 'send failure unexpectedly succeeded' >&2
  exit 1
fi
[[ -s "${tmp}/send-failure/state/send-failures.log" ]]
[[ ! -e "${tmp}/send-failure/state/state" ]]
[[ ! -e "${tmp}/send-failure/state/slot" ]]
[[ ! -e "${tmp}/send-failure/state/receipts.log" ]]
[[ "$(find "${tmp}/send-failure/mailbox" -type f -print | wc -l | tr -d ' ')" -eq 0 ]]

printf '%s\n' 'fixture PID reuse token'
mkdir -p "${tmp}/pid-reuse/state"
printf 'pid=%s\nstart=not-this-process\ncreated=0\n' "$$" >"${tmp}/pid-reuse/state/.lock-token-input"
mkdir "${tmp}/pid-reuse/state/.lock"
mv "${tmp}/pid-reuse/state/.lock-token-input" "${tmp}/pid-reuse/state/.lock/token"
run pid-reuse "${auditor}" --once >/dev/null
[[ -e "${tmp}/pid-reuse/state/state" ]]

printf '%s\n' 'fixture session identity mismatch'
mkdir -p "${tmp}/session/state"
printf '%s\n' 'wrong-session' >"${tmp}/session/state/session-id"
if run session "${auditor}" --once >/dev/null 2>&1; then
  printf '%s\n' 'session mismatch unexpectedly accepted' >&2
  exit 1
fi

printf '%s\n' 'fixture STOP command'
run stop "${auditor}" --dry-run stop | grep -q 'no sentinel written'
run stop "${auditor}" stop >/dev/null
[[ -e "${tmp}/stop/state/STOP" ]]

printf '%s\n' 'all fixtures passed'
