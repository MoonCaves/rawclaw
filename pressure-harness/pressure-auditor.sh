#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
root_dir="${PRESSURE_HARNESS_ROOT:-${script_dir}}"
state_dir="${PRESSURE_STATE_DIR:-${root_dir}/state}"
mailbox_dir="${PRESSURE_MAILBOX_DIR:-${root_dir}/mailbox}"
lock_dir="${state_dir}/.lock"
lock_token="${lock_dir}/token"
pid_file="${state_dir}/pid"
stop_file="${state_dir}/STOP"
receipt_file="${state_dir}/receipts.log"
failure_file="${state_dir}/send-failures.log"
state_file="${state_dir}/state"
config_file="${state_dir}/config"
slot_file="${state_dir}/slot"
session_file="${state_dir}/session-id"
miss_file="${state_dir}/misses"
last_file="${state_dir}/last-audit"
cadence=600
offset=300
pull_offset=420
expected_session_id='01a03ca0-d617-7c90-bfa4-6dc2d0316f7e'
usage() { printf '%s\n' "usage: $0 [--dry-run] [--once|status|start|stop]" "  --once audit one midpoint window" "  --dry-run print without changing state" "  status show state" "  start run loop" "  stop write STOP sentinel"; }
log() { printf '[%s] %s\n' "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" "$*"; }
die() { log "ERROR: $*" >&2; exit 1; }
require_state_dir() { mkdir -p -- "${state_dir}"; }
require_dirs() { require_state_dir; mkdir -p -- "${mailbox_dir}"; }
atomic_write() {
  local path="$1" content="$2" temporary
  temporary="$(mktemp "${path}.tmp.XXXXXX")" || return 1
  if ! printf '%s\n' "${content}" >"${temporary}"; then rm -f -- "${temporary}"; return 1; fi
  if ! mv -- "${temporary}" "${path}"; then rm -f -- "${temporary}"; return 1; fi
}
field_from() { local path="$1" field="$2"; [[ -r "${path}" ]] || return 0; sed -n "s/^${field}=//p" "${path}" | sed -n '1p'; }
process_start_identity() { local pid="$1"; ps -p "${pid}" -o lstart= 2>/dev/null | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | sed -n '1p'; }
current_epoch() { local value="${PRESSURE_NOW:-$(date +%s)}"; [[ "${value}" =~ ^[0-9]+$ ]] || die 'PRESSURE_NOW must be an epoch integer'; printf '%s\n' "${value}"; }
file_epoch() {
  local path="$1" value=''; value="$(stat -f '%m' "${path}" 2>/dev/null || true)"
  [[ "${value}" =~ ^[0-9]+$ ]] || value="$(stat -c '%Y' "${path}" 2>/dev/null || true)"
  [[ "${value}" =~ ^[0-9]+$ ]] || value=0; printf '%s\n' "${value}"
}
read_number() { local path="$1" fallback="$2" value=''; [[ -r "${path}" ]] && value="$(sed -n '1p' "${path}")"; [[ "${value}" =~ ^[0-9]+$ ]] || value="${fallback}"; printf '%s\n' "${value}"; }
acquire_lock() {
  local now owner owner_start actual_start created age max_age
  mkdir -p -- "${state_dir}"
  if ! mkdir -- "${lock_dir}" 2>/dev/null; then
    owner="$(field_from "${lock_token}" pid)"; owner_start="$(field_from "${lock_token}" start)"; created="$(field_from "${lock_token}" created)"; now="$(current_epoch)"
    [[ "${created}" =~ ^[0-9]+$ ]] || created="$(file_epoch "${lock_token}")"; age=$((now - created)); max_age=$((cadence * 2)); actual_start=''
    [[ "${owner}" =~ ^[0-9]+$ ]] && actual_start="$(process_start_identity "${owner}")"
    if [[ "${owner}" =~ ^[0-9]+$ ]] && kill -0 "${owner}" 2>/dev/null; then
      if [[ -n "${owner_start}" && -n "${actual_start}" && "${owner_start}" == "${actual_start}" ]]; then die "audit lock held by pid ${owner}"; fi
      if [[ -z "${owner_start}" || -z "${actual_start}" ]] && (( age >= 0 && age < max_age )); then die "audit lock held by pid ${owner} (bounded-age fallback)"; fi
    fi
    rm -f -- "${lock_token}"; rmdir -- "${lock_dir}" 2>/dev/null || die "stale audit lock cannot be cleared"; mkdir -- "${lock_dir}" || die "audit lock cannot be acquired"
  fi
  now="$(current_epoch)"; printf 'pid=%s\nstart=%s\ncreated=%s\n' "$$" "$(process_start_identity "$$")" "${now}" >"${lock_token}"
  trap 'rm -f -- "${lock_token}"; rmdir -- "${lock_dir}" 2>/dev/null || true; if [[ -r "${pid_file}" ]] && [[ "$(sed -n "1p" "${pid_file}")" == "$$" ]]; then rm -f -- "${pid_file}"; fi' EXIT
}
validate_identity() {
  local configured state_session config_session stored_session
  configured="${OZZY_SESSION_ID:-${PRESSURE_SESSION_ID:-${expected_session_id}}}"
  [[ "${configured}" == "${expected_session_id}" ]] || die "session identity mismatch: ${configured}"
  if [[ -r "${session_file}" ]]; then
    stored_session="$(sed -n '1p' "${session_file}")"; [[ "${stored_session}" == "${expected_session_id}" ]] || die "persisted session identity mismatch: ${stored_session}"
  fi
  if [[ -r "${config_file}" ]]; then
    config_session="$(field_from "${config_file}" session_id)"; [[ "${config_session}" == "${expected_session_id}" ]] || die "persisted config session identity mismatch: ${config_session}"
    [[ "$(field_from "${config_file}" cadence)" == "${cadence}" ]] || die "persisted cadence mismatch"; [[ "$(field_from "${config_file}" offset)" == "${offset}" ]] || die "persisted offset mismatch"; [[ "$(field_from "${config_file}" pull_offset)" == "${pull_offset}" ]] || die "persisted pull offset mismatch"
  fi
  if [[ -r "${state_file}" ]]; then
    state_session="$(field_from "${state_file}" session_id)"; [[ "${state_session}" == "${expected_session_id}" ]] || die "state session identity mismatch: ${state_session}"
    [[ "$(field_from "${state_file}" cadence)" == "${cadence}" ]] || die "state cadence mismatch"; [[ "$(field_from "${state_file}" offset)" == "${offset}" ]] || die "state offset mismatch"; [[ "$(field_from "${state_file}" pull_offset)" == "${pull_offset}" ]] || die "state pull offset mismatch"
  fi
  if [[ -r "${slot_file}" ]]; then
    stored_session="$(field_from "${slot_file}" session_id)"; [[ "${stored_session}" == "${expected_session_id}" ]] || die "persisted slot session identity mismatch: ${stored_session}"
  fi
}
read_only_rivals() {
  local configured="${RIVAL_MAILBOXES:-${root_dir}/rivals/mailbox-a:${root_dir}/rivals/mailbox-b}" path old_ifs="${IFS}"
  IFS=':'; read -ra paths <<<"${configured}"; IFS="${old_ifs}"
  for path in "${paths[@]}"; do
    [[ -d "${path}" ]] || continue
    find "${path}" -type f -print 2>/dev/null | sort | while IFS= read -r file; do stat -f '%N %z' "${file}" 2>/dev/null || stat -c '%n %s' "${file}" 2>/dev/null || true; done
  done
}
send_to_ozzy_mailbox() {
  local slot="$1" receipt="$2" destination temporary
  mkdir -p -- "${mailbox_dir}" || return 1
  destination="${mailbox_dir}/slot-${slot}.receipt"; [[ "${PRESSURE_SEND_FAIL:-0}" == 1 ]] && return 1; [[ -e "${destination}" ]] && return 0
  temporary="$(mktemp "${mailbox_dir}/.receipt.XXXXXX")" || return 1
  if ! printf '%s\n' "${receipt}" >"${temporary}"; then rm -f -- "${temporary}"; return 1; fi
  if ! mv -- "${temporary}" "${destination}"; then rm -f -- "${temporary}"; return 1; fi
}
record_send_failure() { local receipt="$1" reason="$2"; printf '%s reason=%s receipt=%s\n' "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" "${reason}" "${receipt}" >>"${failure_file}" || true; }
audit_once() {
  local dry_run="$1" now epoch slot due pull_due misses new_misses escalation receipt persisted_slot state_payload config_payload slot_payload
  epoch="$(current_epoch)"; [[ "${epoch}" =~ ^[0-9]+$ ]] || die "PRESSURE_NOW must be an epoch integer"
  now="$(date -u -r "${epoch}" +'%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -d "@${epoch}" +'%Y-%m-%dT%H:%M:%SZ')"; slot=$((epoch / cadence)); due=$((slot * cadence + offset)); pull_due=$((slot * cadence + pull_offset))
  while (( due > epoch )); do due=$((due - cadence)); done; while (( pull_due > epoch )); do pull_due=$((pull_due - cadence)); done
  persisted_slot="$(field_from "${state_file}" slot)"; [[ -n "${persisted_slot}" ]] || persisted_slot="$(sed -n '1p' "${slot_file}" 2>/dev/null || true)"
  if [[ -n "${persisted_slot}" && "${persisted_slot}" == "${slot}" ]]; then printf '%s slot=%s idempotent=1 misses=%s escalation=none\n' "${now}" "${slot}" "$(read_number "${miss_file}" 0)"; return 0; fi
  if [[ -e "${stop_file}" ]]; then printf '%s slot=%s stopped=1\n' "${now}" "${slot}"; return 0; fi
  misses="$(field_from "${state_file}" misses)"; [[ "${misses}" =~ ^[0-9]+$ ]] || misses="$(read_number "${miss_file}" 0)"; escalation='none'; new_misses=0
  if [[ "${PRESSURE_LATE:-0}" == 1 ]] || (( epoch > due + cadence )); then new_misses=$((misses + 1)); case "${new_misses}" in 1) escalation='warn';; 2) escalation='critical';; *) escalation='stop-and-escalate';; esac; fi
  receipt="${now} session=${expected_session_id} slot=${slot} midpoint_due=${due} pull_due=${pull_due} misses=${new_misses} escalation=${escalation}"
  if (( dry_run )); then printf 'DRY-RUN %s\n' "${receipt}"; read_only_rivals; return 0; fi
  require_state_dir; validate_identity
  if ! send_to_ozzy_mailbox "${slot}" "${receipt}"; then record_send_failure "${receipt}" send_failed; printf '%s\n' 'send failed; state not advanced' >&2; return 1; fi
  state_payload="session_id=${expected_session_id}"$'\n'"cadence=${cadence}"$'\n'"offset=${offset}"$'\n'"pull_offset=${pull_offset}"$'\n'"slot=${slot}"$'\n'"misses=${new_misses}"$'\n'"updated=${now}"
  config_payload="session_id=${expected_session_id}"$'\n'"cadence=${cadence}"$'\n'"offset=${offset}"$'\n'"pull_offset=${pull_offset}"
  slot_payload="slot=${slot}"$'\n'"session_id=${expected_session_id}"
  atomic_write "${state_file}" "${state_payload}" || die 'state write failed after mailbox send'; atomic_write "${config_file}" "${config_payload}" || die 'config write failed after state send'; atomic_write "${miss_file}" "${new_misses}" || die 'miss state write failed'; atomic_write "${slot_file}" "${slot_payload}" || die 'slot state write failed'; atomic_write "${session_file}" "${expected_session_id}" || die 'session state write failed'
  printf '%s\n' "${now}" >"${last_file}"; read_only_rivals >/dev/null; printf '%s\n' "${receipt}" >>"${receipt_file}"; if (( new_misses >= 3 )); then : >"${stop_file}"; fi
  printf '%s\n' "${receipt} stop=$([[ -e "${stop_file}" ]] && printf yes || printf no)"
}
status() { local pid='none' latest='none'; [[ -r "${pid_file}" ]] && pid="$(sed -n '1p' "${pid_file}")"; [[ -r "${receipt_file}" ]] && latest="$(tail -n 1 "${receipt_file}")"; printf 'state=%s\npid=%s\npid_alive=%s\nstop=%s\nmisses=%s\nlatest=%s\n' "${state_dir}" "${pid}" "$([[ "${pid}" =~ ^[0-9]+$ ]] && kill -0 "${pid}" 2>/dev/null && printf yes || printf no)" "$([[ -e "${stop_file}" ]] && printf yes || printf no)" "$(read_number "${miss_file}" 0)" "${latest}"; }
start_loop() { require_dirs; validate_identity; [[ -e "${stop_file}" ]] && die 'STOP sentinel is present'; acquire_lock; printf '%s\n' "$$" >"${pid_file}"; while [[ ! -e "${stop_file}" ]]; do audit_once 0 || return 1; [[ -e "${stop_file}" ]] && break; sleep "${PRESSURE_SLEEP_SECONDS:-${cadence}}"; done; }
stop_loop() { require_dirs; : >"${stop_file}"; printf 'STOP sentinel written: %s\n' "${stop_file}"; }
dry_run=0; command_name='once'; if [[ "${1:-}" == '--dry-run' ]]; then dry_run=1; shift; fi; if [[ $# -gt 1 ]]; then usage >&2; exit 2; fi; [[ $# -eq 1 ]] && command_name="$1"
case "${command_name}" in --once|once) (( dry_run )) || require_state_dir; (( dry_run )) || { validate_identity; acquire_lock; }; audit_once "${dry_run}";; status) status;; start) (( dry_run )) && { printf '%s\n' 'DRY-RUN start: no process activated'; exit 0; }; start_loop;; stop) (( dry_run )) && { printf '%s\n' 'DRY-RUN stop: no sentinel written'; exit 0; }; stop_loop;; --help|-h|help) usage;; *) usage >&2; exit 2;; esac
