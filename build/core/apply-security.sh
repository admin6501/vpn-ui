#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
src="${1:-$root/third_party/Xray-core}"
patch="$root/build/core/security-dependencies.patch"
# Normalize a replacement left by an earlier fallback build.
if grep -Fq "replace github.com/apernet/quic-go => $root/third_party/quic-go" "$src/go.mod"; then
    (cd "$src" && go mod edit "-replace=github.com/apernet/quic-go=../quic-go")
fi
if git -C "$src" apply --check "$patch" 2>/dev/null; then
    git -C "$src" apply "$patch"
elif ! git -C "$src" apply --reverse --check "$patch" 2>/dev/null; then
    echo "Xray security dependency patch does not match this checkout; refusing an unpatched build." >&2
    exit 1
fi
# A fallback clone may live outside third_party; use an absolute module path there.
if [[ "$(cd "$src" && pwd)" != "$root/third_party/Xray-core" ]]; then
    (cd "$src" && go mod edit "-replace=github.com/apernet/quic-go=$root/third_party/quic-go")
fi
