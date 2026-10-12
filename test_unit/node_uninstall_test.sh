#!/usr/bin/env bash
set -Eeuo pipefail
TASK_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$TASK_ROOT/install-node.sh"
TASK_TMP=$(mktemp -d)
trap 'rc=$?; if ((rc)); then cat "$TASK_TMP/output"; cat "$TASK_LOG"; fi; command rm -rf -- "$TASK_TMP"' EXIT
TASK_LOG="$TASK_TMP/commands"
node_uninstall_path_exists() {
 case "$1" in
  /opt/vpn-ui/vpn-ui.db) [[ ${TEST_CENTRAL_DB:-0} == 1 ]] ;;
  /opt/vpn-ui-node*) [[ ${TEST_NODE_PRESENT:-1} == 1 ]] ;;
  *) return 1 ;;
 esac
}
node_uninstall_path_safe() { [[ ${TEST_SYMLINK:-0} == 0 ]]; }
require_node_root() { :; }
node_cleanup_supported() { [[ ${TEST_UNSUPPORTED:-0} == 0 ]]; }
realpath() { [[ $1 != -- ]] || shift; printf '%s\n' "$1"; }
run_node_cleanup() { printf 'cleanup %s\n' "$1" >> "$TASK_LOG"; [[ ${TEST_CLEANUP_FAIL:-0} == 0 ]]; }
systemctl() {
 printf 'systemctl %s\n' "$*" >> "$TASK_LOG"
 case "$1" in
  cat) [[ $2 == vpn-ui-node.service && ${TEST_NODE_PRESENT:-1} == 1 ]] ;;
  stop) [[ ${TEST_STOP_FAIL:-0} == 0 ]] || return 1; TEST_ACTIVE=0 ;;
  is-active) [[ ${TEST_ACTIVE:-1} == 1 ]] ;;
  *) return 0 ;;
 esac
}
rm() {
 printf 'remove %s\n' "$*" >> "$TASK_LOG"
 case "$*" in
  '-f -- /etc/systemd/system/vpn-ui-node.service'|'-rf -- /etc/systemd/system/vpn-ui-node.service.d /opt/vpn-ui-node') return 0 ;;
  *) printf 'Unexpected removal: %s\n' "$*" >&2; return 1 ;;
 esac
}
run_case() { : > "$TASK_LOG"; printf '%s' "$1" | uninstall_node > "$TASK_TMP/output" 2>&1; }
run_case $'No\n'; ! grep -q '^remove\|^cleanup\|systemctl stop' "$TASK_LOG"
run_case ''; ! grep -q '^remove\|^cleanup\|systemctl stop' "$TASK_LOG"
run_case $'Yes\n'; grep -q '^cleanup /opt/vpn-ui-node/vpn-ui-node' "$TASK_LOG"; grep -q '^remove .* /opt/vpn-ui-node$' "$TASK_LOG"
stop_line=$(grep -n 'systemctl stop' "$TASK_LOG" | cut -d: -f1)
cleanup_line=$(grep -n '^cleanup' "$TASK_LOG" | cut -d: -f1)
remove_line=$(grep -n '^remove' "$TASK_LOG" | head -1 | cut -d: -f1)
[[ $stop_line -lt $cleanup_line && $cleanup_line -lt $remove_line ]]
! grep -q 'disable vpn-ui.service\|apt-get\|pkill' "$TASK_LOG"
TEST_CLEANUP_FAIL=1; export TEST_CLEANUP_FAIL
if run_case $'Yes\n'; then echo 'failed cleanup accepted'; exit 1; fi
! grep -q '^remove' "$TASK_LOG"; unset TEST_CLEANUP_FAIL
TEST_STOP_FAIL=1; export TEST_STOP_FAIL
if run_case $'Yes\n'; then echo 'failed stop accepted'; exit 1; fi
! grep -q '^remove\|^cleanup' "$TASK_LOG"; unset TEST_STOP_FAIL
TEST_CENTRAL_DB=1; export TEST_CENTRAL_DB
if run_case $'Yes\n'; then echo 'central panel host accepted'; exit 1; fi
! grep -q '^remove\|^cleanup\|systemctl stop' "$TASK_LOG"; unset TEST_CENTRAL_DB
TEST_SYMLINK=1; export TEST_SYMLINK
if run_case $'Yes\n'; then echo 'symlink accepted'; exit 1; fi
! grep -q '^remove\|^cleanup\|systemctl stop' "$TASK_LOG"; unset TEST_SYMLINK
TEST_UNSUPPORTED=1; export TEST_UNSUPPORTED
: > "$TASK_LOG"
if (uninstall_node --uninstall --yes --binary /tmp/current-node) > "$TASK_TMP/output" 2>&1; then echo 'unsupported binary accepted'; exit 1; fi
! grep -q '^remove\|^cleanup\|systemctl stop' "$TASK_LOG"; unset TEST_UNSUPPORTED
TEST_NODE_PRESENT=0; export TEST_NODE_PRESENT
run_case ''; ! grep -q '^remove\|^cleanup' "$TASK_LOG"; unset TEST_NODE_PRESENT
: > "$TASK_LOG"; uninstall_node --uninstall --yes > "$TASK_TMP/output"
grep -q '^remove' "$TASK_LOG"
echo 'Node uninstall cancellation, ordering, scoped deletion, failure retention and host guards passed.'
