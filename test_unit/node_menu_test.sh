#!/usr/bin/env bash
set -Eeuo pipefail
TASK_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$TASK_ROOT/install-node.sh"
TASK_TMP=$(mktemp -d)
trap 'command rm -rf -- "$TASK_TMP"' EXIT
TASK_LOG="$TASK_TMP/actions"
export TASK_LOG
require_node_root() { :; }
# The parent handles failure, but the fresh install shell must still stop at
# the first failing command (including with a process-substitution entrypoint).
install_node() { false; printf 'unsafe continuation\n' >> "$TASK_LOG"; }
if node_menu_run --install > "$TASK_TMP/child-output" 2>&1; then
    echo 'Menu child ignored install failure'; exit 1
fi
[[ ! -s $TASK_LOG ]]
node_menu_run() { printf 'script %s\n' "$*" >> "$TASK_LOG"; [[ $1 != --install ]]; }
systemctl() { printf 'systemctl %s\n' "$*" >> "$TASK_LOG"; }
journalctl() { printf 'journalctl %s\n' "$*" >> "$TASK_LOG"; }
printf 'invalid\n1\n2\n3\n4\n5\n6\n7\n0\n' | node_main > "$TASK_TMP/output"
rg -q 'Invalid option' "$TASK_TMP/output"
rg -q 'Installation failed' "$TASK_TMP/output"
diff -u <(cat <<'EXPECTED'
script --install
systemctl --no-pager --full status vpn-ui-node.service
systemctl start vpn-ui-node.service
systemctl stop vpn-ui-node.service
systemctl restart vpn-ui-node.service
journalctl --no-pager -u vpn-ui-node.service -n 100
script --uninstall
EXPECTED
) "$TASK_LOG"
: > "$TASK_LOG"
node_main --menu < /dev/null > "$TASK_TMP/output"
[[ ! -s $TASK_LOG ]]
install_node() { printf 'direct %s\n' "$*" >> "$TASK_LOG"; }
node_main --panel-url https://example.com --pair-token example
rg -q '^direct --panel-url https://example.com --pair-token example$' "$TASK_LOG"
echo 'Node menu dispatch, failed-install recovery, EOF and existing pairing commands passed.'
