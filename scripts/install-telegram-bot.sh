#!/usr/bin/env bash
# Fetch the bot files from the default branch and launch its management menu.
set -Eeuo pipefail
[[ $EUID -eq 0 ]] || { echo 'Run with sudo.' >&2; exit 1; }
command -v curl >/dev/null || { echo 'curl is required: apt-get update && apt-get install -y curl' >&2; exit 1; }
command -v tar >/dev/null || { echo 'tar is required.' >&2; exit 1; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl --fail --location --silent --show-error --retry 2 https://codeload.github.com/admin6501/vpn-ui/tar.gz/refs/heads/main -o "$tmp/repo.tar.gz"
tar -xzf "$tmp/repo.tar.gz" -C "$tmp" --wildcards 'vpn-ui-main/telegram-bot/*'
cd "$tmp/vpn-ui-main/telegram-bot"
bash ./manage-server.sh
