#!/usr/bin/env bash
# Exercise the real helper with fake apt executables; never installs host packages.
set -Eeuo pipefail
TASK_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$TASK_ROOT/install-offline.sh"
lang=en
APT_TEST_DIR=$(mktemp -d)
trap 'rm -rf "$APT_TEST_DIR"' EXIT
mkdir -p "$APT_TEST_DIR/bin" "$APT_TEST_DIR/empty"
export APT_TEST_LOG="$APT_TEST_DIR/commands"
cat > "$APT_TEST_DIR/bin/apt-get" <<'FIXTURE'
#!/usr/bin/env bash
printf 'apt-get %s\n' "$*" >> "$APT_TEST_LOG"
if [[ " $* " == *" update "* ]]; then exit "${APT_TEST_UPDATE_FAILURE:-0}"; fi
exit "${APT_TEST_INSTALL_FAILURE:-0}"
FIXTURE
cat > "$APT_TEST_DIR/bin/apt-cache" <<'FIXTURE'
#!/usr/bin/env bash
printf 'apt-cache %s\n' "$*" >> "$APT_TEST_LOG"
exit "${APT_TEST_MISSING_KERNEL:-0}"
FIXTURE
chmod +x "$APT_TEST_DIR/bin/apt-get" "$APT_TEST_DIR/bin/apt-cache"
export PATH="$APT_TEST_DIR/bin:$PATH"
run_offer() {
    : > "$APT_TEST_LOG"
    printf '%s' "$1" | offer_apt_prerequisites > "$APT_TEST_DIR/output"
}
run_offer $'No\n'
[[ ! -s $APT_TEST_LOG ]]
run_offer ''
[[ ! -s $APT_TEST_LOG ]]
run_offer $'invalid\nNO\n'
[[ ! -s $APT_TEST_LOG ]]
run_offer $'YES\n'
grep -q 'apt-get .*update' "$APT_TEST_LOG"
grep -q 'apt-get .*install .*python3 .*iproute2 .*nftables' "$APT_TEST_LOG"
grep -q "linux-headers-$(uname -r)" "$APT_TEST_LOG"
grep -q "linux-modules-extra-$(uname -r)" "$APT_TEST_LOG"
export APT_TEST_UPDATE_FAILURE=1
run_offer $'yes\n'
! grep -q 'apt-get .*install' "$APT_TEST_LOG"
grep -q 'index refresh failed' "$APT_TEST_DIR/output"
unset APT_TEST_UPDATE_FAILURE
export APT_TEST_INSTALL_FAILURE=1
run_offer $'yes\n'
grep -q 'installation did not complete' "$APT_TEST_DIR/output"
unset APT_TEST_INSTALL_FAILURE
export APT_TEST_MISSING_KERNEL=1
run_offer $'yes\n'
! grep 'apt-get .*install' "$APT_TEST_LOG" | grep -q 'linux-headers-'
grep -q 'Kernel package unavailable' "$APT_TEST_DIR/output"
unset APT_TEST_MISSING_KERNEL
: > "$APT_TEST_LOG"
PATH="$APT_TEST_DIR/empty" offer_apt_prerequisites > "$APT_TEST_DIR/output"
[[ ! -s $APT_TEST_LOG ]]
grep -q 'apt is unavailable' "$APT_TEST_DIR/output"
printf 'Optional apt prerequisite tests passed (no real apt operations).\n'
