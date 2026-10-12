#!/usr/bin/env bash
# Run only on an ephemeral release runner, before installing the central panel.
set -Eeuo pipefail
[[ $EUID == 0 ]] || { echo 'This smoke test requires root.'; exit 1; }
TASK_NODE_BINARY=$(realpath -- "${1:?node binary required}")
TASK_REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
[[ ! -e /opt/vpn-ui-node ]] || { echo 'Existing node installation; refusing test.'; exit 1; }
if systemctl cat vpn-ui.service >/dev/null 2>&1; then echo 'Central panel exists; refusing test.'; exit 1; fi
install -d -m 700 /opt/vpn-ui-node
printf '{"panelUrl":"https://127.0.0.1:9/","nodeId":1,"token":"%064d"}\n' 0 > /opt/vpn-ui-node/agent.json
printf 'No\n' | bash "$TASK_REPO/install-node.sh" --binary "$TASK_NODE_BINARY"
systemctl is-active --quiet vpn-ui-node
for attempt in $(seq 1 30); do
 [[ ! -f /opt/vpn-ui-node/vpn-ui.db ]] || break
 sleep 1
done
[[ -f /opt/vpn-ui-node/vpn-ui.db ]]
bash "$TASK_REPO/install-node.sh" --uninstall --yes --binary "$TASK_NODE_BINARY"
[[ ! -e /opt/vpn-ui-node ]]
[[ ! -e /etc/systemd/system/vpn-ui-node.service ]]
! systemctl is-active --quiet vpn-ui-node
[[ ! -e /usr/bin/vpn-ui ]]
bash "$TASK_REPO/install-node.sh" --uninstall --yes
printf 'Real systemd node installation and offline uninstall passed.\n'
