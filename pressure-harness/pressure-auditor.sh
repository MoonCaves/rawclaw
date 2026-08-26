#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
root_dir="${PRESSURE_HARNESS_ROOT:-${script_dir}}"
state_dir="${PRESSURE_STATE_DIR:-${root_dir}/state}"
mailbox_dir="${PRESSURE_MAILBOX_DIR:-${root_dir}/mailbox}"
lock_dir="${state_dir}/.lock"
pid_file="${state_dir}/pid"
stop_file="${state_dir}/STOP"
receipt_file="${state_dir}/receipts.log"
miss_file="${state_dir}/misses"
last_file="${state_dir}/last-audit"
cadence=600
offset=300
pull_offset=420
usage() { printf '%s\n' "usage: $0 [--dry-run] [--once|status|start|stop]" "  --once audit one midpoint window" "  --dry-run print without changing state" "  status show state" "  start run loop" "  stop write STOP sentinel"; }
log() { printf '[%s] %s\n' "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" "$*"; }
die() { log "ERROR: $*" >&2; exit 1; }
require_dirs() { mkdir -p -- "${state_dir}" "${mailbox_dir}"; }
read_number() { local path="$1" fallback="$2" value=''; [[ -r "${path}" ]] && value="$(sed -n '1p' "${path}")"; [[ "${value}" =~ ^[0-9]+$ ]] || value="${fallback}"; printf '%s\n' "${value}"; }
acquire_lock() {
  mkdir -p -- "${state_dir}"
  if ! mkdir -- "${lock_dir}" 2>/dev/null; then
    local owner=''; [[ -r "${lock_dir}/pid" ]] && owner="$(sed -n '1p' "${lock_dir}/pid")"
    if [[ "${owner}" =~ ^[0-9]+$ ]] && kill -0 "${owner}" 2>/dev/null; then die "audit lock held by pid ${owner}"; fi
    rm -f -- "${lock_dir}/pid"; rmdir -- "${lock_dir}" 2>/dev/null || die "stale audit lock cannot be cleared"; mkdir -- "${lock_dir}" || die "audit lock cannot be acquired"
  fi
  printf '%s\n' "$$" >"${lock_dir}/pid"
  trap 'rm -f -- "${lock_dir}/pid"; rmdir -- "${lock_dir}" 2>/dev/null || true' EXIT
}
read_only_rivals() {
  local configured="${RIVAL_MAILBOXES:-${root_dir}/rivals/mailbox-a:${root_dir}/rivals/mailbox-b}" path old_ifs="${IFS}"
  IFS=':'; read -ra paths <<<"${configured}"; IFS="${old_ifs}"
  for path in "${paths[@]}"; do
    [[ -d "${path}" ]] || continue
    find "${path}" -type f -print 2>/dev/null | sort | while IFS= read -r file; do stat -f '%N %z' "${file}" 2>/dev/null || stat -c '%n %s' "${file}" 2>/dev/null || true; done
  done
}
audit_once() {
  local dry_run="$1" now epoch slot due pull_due misses escalation receipt
  epoch="${PRESSURE_NOW:-$(date +%s)}"; [[ "${epoch}" =~ ^[0-9]+$ ]] || die "PRESSURE_NOW must be an epoch integer"
  now="$(date -u -r "${epoch}" +'%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -d "@${epoch}" +'%Y-%m-%dT%H:%M:%SZ')"; slot=$((epoch / cadence)); due=$((slot * cadence + offset)); pull_due=$((slot * cadence + pull_offset))
  while (( due > epoch )); do due=$((due - cadence)); done; while (( pull_due > epoch )); do pull_due=$((pull_due - cadence)); done
  misses="$(read_number "${miss_file}" 0)"; escalation='none'
  if [[ "${PRESSURE_LATE:-0}" == 1 ]] || (( epoch > due + cadence )); then misses=$((misses + 1)); case "${misses}" in 1) escalation='warn';; 2) escalation='critical';; *) escalation='stop-and-escalate';; esac; else misses=0; fi
  receipt="${now} midpoint_due=${due} pull_due=${pull_due} misses=${misses} escalation=${escalation} rivals=readonly"
  if (( dry_run )); then printf 'DRY-RUN %s\n' "${receipt}"; read_only_rivals; return 0; fi
  require_dirs; printf '%s\n' "${misses}" >"${miss_file}"; printf '%s\n' "${now}" >"${last_file}"; printf '%s\n' "${receipt}" >>"${receipt_file}"; read_only_rivals >/dev/null; printf '%s\n' "${receipt}"
}
status() { local pid='none' latest='none'; [[ -r "${pid_file}" ]] && pid="$(sed -n '1p' "${pid_file}")"; [[ -r "${receipt_file}" ]] && latest="$(tail -n 1 "${receipt_file}")"; printf 'state=%s\npid=%s\npid_alive=%s\nstop=%s\nmisses=%s\nlatest=%s\n' "${state_dir}" "${pid}" "$([[ "${pid}" =~ ^[0-9]+$ ]] && kill -0 "${pid}" 2>/dev/null && printf yes || printf no)" "$([[ -e "${stop_file}" ]] && printf yes || printf no)" "$(read_number "${miss_file}" 0)" "${latest}"; }
start_loop() { require_dirs; rm -f -- "${stop_file}"; acquire_lock; printf '%s\n' "$$" >"${pid_file}"; while [[ ! -e "${stop_file}" ]]; do audit_once 0; sleep "${cadence}"; done; rm -f -- "${pid_file}"; }
stop_loop() { require_dirs; : >"${stop_file}"; printf 'STOP sentinel written: %s\n' "${stop_file}"; }
dry_run=0; command_name='once'; if [[ "${1:-}" == '--dry-run' ]]; then dry_run=1; shift; fi; if [[ $# -gt 1 ]]; then usage >&2; exit 2; fi; [[ $# -eq 1 ]] && command_name="$1"
case "${command_name}" in --once|once) (( dry_run )) || require_dirs; (( dry_run )) || acquire_lock; audit_once "${dry_run}";; status) status;; start) (( dry_run )) && { printf '%s\n' 'DRY-RUN start: no process activated'; exit 0; }; start_loop;; stop) (( dry_run )) && { printf '%s\n' 'DRY-RUN stop: no sentinel written'; exit 0; }; stop_loop;; --help|-h|help) usage;; *) usage >&2; exit 2;; esac
