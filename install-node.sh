#!/usr/bin/env bash
# Install the dedicated agent; VPN cores are installed only from the central panel.
set -Eeuo pipefail
umask 077
say() { printf '%s\n' "$1"; }
die() { say "$1" >&2; exit 1; }
offer_apt_prerequisites() {
    if ! command -v apt-get >/dev/null 2>&1; then
        say 'apt is unavailable; use your distribution package manager for OS prerequisites.'
        return 0
    fi
    local answer pkg kernel_version
    local -a packages=(ca-certificates curl unzip python3 sqlite3 iproute2 nftables kmod openssl cron coreutils gawk build-essential dkms pkg-config)
    say 'Prerequisites available through apt:'
    printf '  %s\n' "${packages[*]}"
    say 'Matching running-kernel headers and extra modules are included when available in the repository.'
    while true; do
        say 'Install these prerequisites with apt? Internet access is required. Yes/No [No]:'
        if ! IFS= read -r answer; then answer=no; fi
        case "${answer,,}" in
            y|yes) break ;;
            ''|n|no)
                say 'apt installation skipped; continuing with the local node binary.'
                return 0 ;;
            *) say 'Please enter Yes or No.' ;;
        esac
    done
    # This is the only optional online step. It happens before stopping the node.
    local -a apt_options=(-o Acquire::Retries=0 -o Acquire::http::Timeout=20 -o Acquire::https::Timeout=20)
    if ! DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get "${apt_options[@]}" update; then
        say 'apt index refresh failed; prerequisites were not installed. Continuing local node installation.'
        return 0
    fi
    kernel_version=$(uname -r)
    for pkg in "linux-headers-$kernel_version" "linux-modules-extra-$kernel_version"; do
        if command -v apt-cache >/dev/null 2>&1 && apt-cache show "$pkg" >/dev/null 2>&1; then
            packages+=("$pkg")
        else
            say "Kernel package unavailable (optional, skipped): $pkg"
        fi
    done
    if DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get "${apt_options[@]}" \
        -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold \
        install -y --no-install-recommends "${packages[@]}"; then
        say 'apt prerequisites installed; verify each protocol in the cores page.'
    else
        say 'apt installation did not complete; check the output above. Continuing local node installation.'
    fi
    return 0
}

install_node() {
    local panel='' pair='' ca='' binary='' base=/opt/vpn-ui-node unit=vpn-ui-node
    while (($#)); do
        case "$1" in
            --panel-url) panel=${2:?}; shift 2 ;;
            --pair-token) pair=${2:?}; shift 2 ;;
            --ca-file) ca=${2:?}; shift 2 ;;
            --binary) binary=${2:?}; shift 2 ;;
            # Accept old commands without changing the English-only interface.
            --lang) [[ ${2:?} == en || $2 == fa ]] || die 'Unsupported legacy language option.'; shift 2 ;;
            *) die "Unknown option: $1" ;;
        esac
    done
    [[ $EUID == 0 ]] || die 'Run as root.'
    command -v systemctl >/dev/null || die 'systemd is required.'
    [[ -d /run/systemd/system ]] || die 'A running systemd host is required.'
    if systemctl is-active --quiet vpn-ui; then
        die 'Install the node on a separate location server, not alongside an active central panel.'
    fi
    case $(uname -m) in
        x86_64|amd64) arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        *) die 'Unsupported architecture.' ;;
    esac
    offer_apt_prerequisites
    command -v curl >/dev/null || [[ -n $binary ]] || die 'Install curl for downloads, or use --binary.'
    mkdir -p "$base"
    chmod 700 "$base"
    local stage
    stage=$(mktemp -d "$base/install.XXXXXXXX")
    trap 'rm -rf -- "$stage"' RETURN
    if [[ -n $binary ]]; then
        [[ -f $binary ]] || die 'Local binary not found.'
        cp -- "$binary" "$stage/vpn-ui-node"
    else
        say 'Downloading the agent; no VPN core will be activated.'
        curl -fL --retry 2 https://github.com/admin6501/vpn-ui/releases/latest/download/vpn-ui-node-"$arch" -o "$stage/vpn-ui-node"
        curl -fsSL --retry 2 https://github.com/admin6501/vpn-ui/releases/latest/download/SHA256SUMS -o "$stage/SHA256SUMS"
        local expected actual
        expected=$(awk -v name="vpn-ui-node-$arch" '$2==name {print $1}' "$stage/SHA256SUMS")
        [[ $expected =~ ^[[:xdigit:]]{64}$ ]] || die 'Binary checksum missing from release.'
        actual=$(sha256sum "$stage/vpn-ui-node" | awk '{print $1}')
        [[ $actual == "$expected" ]] || die 'Binary checksum mismatch.'
    fi
    chmod 755 "$stage/vpn-ui-node"
    "$stage/vpn-ui-node" -v >/dev/null || die 'Binary cannot run on this architecture.'
    local -a ca_args=()
    if [[ -n $ca ]]; then
        [[ -f $ca ]] || die 'CA file not found.'
        if [[ $ca != "$base/panel-ca.crt" ]]; then install -m 600 "$ca" "$base/panel-ca.crt"; fi
        ca_args=(--ca-file "$base/panel-ca.crt")
    fi
    if [[ ! -f $base/agent.json ]]; then
        if [[ -z $panel ]]; then say 'Exact HTTPS central panel URL, including its web path:'; IFS= read -r panel; fi
        if [[ -z $pair ]]; then say 'Single-use pairing token:'; IFS= read -r pair; fi
        "$stage/vpn-ui-node" enroll --config "$base/agent.json" --panel-url "$panel" --pair-token "$pair" "${ca_args[@]}"
    elif [[ -n $pair ]]; then
        # Retry a pending enrollment, but never replace an already paired agent.
        "$stage/vpn-ui-node" enroll --config "$base/agent.json" --panel-url "$panel" --pair-token "$pair" "${ca_args[@]}"
    fi
    local backup was_active=0
    backup=$(mktemp -d "$base/update-backup.XXXXXXXX")
    systemctl is-active --quiet "$unit" && was_active=1
    systemctl stop "$unit" 2>/dev/null || [[ $was_active == 0 ]]
    [[ ! -f $base/vpn-ui-node ]] || cp -a "$base/vpn-ui-node" "$backup/vpn-ui-node"
    [[ ! -f /etc/systemd/system/$unit.service ]] || cp -a /etc/systemd/system/$unit.service "$backup/unit"
    rollback_node() {
        trap - ERR
        systemctl stop "$unit" 2>/dev/null || true
        [[ ! -f $backup/vpn-ui-node ]] || cp -a "$backup/vpn-ui-node" "$base/vpn-ui-node"
        [[ ! -f $backup/unit ]] || cp -a "$backup/unit" /etc/systemd/system/$unit.service
        systemctl daemon-reload || true
        if [[ $was_active == 1 ]]; then systemctl start "$unit" || true; fi
        die "Installation failed; backup: $backup"
    }
    trap rollback_node ERR
    install -m 755 "$stage/vpn-ui-node" "$base/.vpn-ui-node.new"
    mv -f "$base/.vpn-ui-node.new" "$base/vpn-ui-node"
    cat > /etc/systemd/system/$unit.service <<UNIT
[Unit]
Description=VPN-UI dedicated node agent
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=root
WorkingDirectory=$base
ExecStart=$base/vpn-ui-node run --config $base/agent.json
Environment=VPNUI_DB_FOLDER=$base
Environment=VPNUI_BIN_FOLDER=$base/bin
Environment=VPNUI_NODE_AGENT=1
UMask=0077
Restart=on-failure
RestartSec=5
TimeoutStopSec=45
KillMode=mixed
LimitNOFILE=1048576
[Install]
WantedBy=multi-user.target
UNIT
    systemctl daemon-reload
    systemctl enable "$unit"
    systemctl start "$unit"
    sleep 2
    systemctl is-active --quiet "$unit"
    trap - ERR
    say 'Agent is running. Install the required cores from Nodes in the central panel.'
    say 'Logs: journalctl -u vpn-ui-node -f'
}
if [[ ${BASH_SOURCE[0]} == "$0" ]]; then install_node "$@"; fi
