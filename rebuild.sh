#!/usr/bin/env bash
set -euo pipefail

export PATH=$PATH:/usr/local/go/bin:/usr/local/node/bin
export GOPROXY=https://goproxy.cn,direct
export NODE_OPTIONS=--max-old-space-size=4096
VERSION="${VERSION:-v2.4.1}"

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
cd "$ROOT_DIR"

if [ "${SKIP_GIT:-0}" != "1" ]; then
    git fetch origin pro 2>&1 | tail -1
    git reset --hard origin/pro 2>&1 | tail -1
fi
git log --oneline -1

rm -rf "$ROOT_DIR/core/cmd/server/web/assets"

cd "$ROOT_DIR/frontend"
npm run build:pro > /tmp/fe.log 2>&1 && echo "FE_OK" || { tail -20 /tmp/fe.log; exit 1; }

CORE_CONF="$ROOT_DIR/core/cmd/server/conf/app.yaml"
AGENT_CONF="$ROOT_DIR/agent/cmd/server/conf/app.yaml"
CORE_CONF_BAK="$(mktemp)"
AGENT_CONF_BAK="$(mktemp)"
cp "$CORE_CONF" "$CORE_CONF_BAK"
cp "$AGENT_CONF" "$AGENT_CONF_BAK"

restore_conf() {
    cp "$CORE_CONF_BAK" "$CORE_CONF" 2>/dev/null || true
    cp "$AGENT_CONF_BAK" "$AGENT_CONF" 2>/dev/null || true
    rm -f "$CORE_CONF_BAK" "$AGENT_CONF_BAK"
}
trap restore_conf EXIT

patch_conf() {
    local f="$1"
    local has_ver=0
    grep -q '^[[:space:]]*version:' "$f" && has_ver=1 || true
    awk -v ver="$VERSION" -v hv="$has_ver" '
        /^[[:space:]]*mode:/ {
            print "  mode: stable"
            if (hv == 0) print "  version: " ver
            next
        }
        /^[[:space:]]*version:/ { print "  version: " ver; next }
        /^[[:space:]]*level:/ { print "  level: info"; next }
        { print }
    ' "$f" > "$f.tmp" && cat "$f.tmp" > "$f" && rm -f "$f.tmp"
}

patch_conf "$CORE_CONF"
patch_conf "$AGENT_CONF"

mkdir -p "$ROOT_DIR/bin"

cd "$ROOT_DIR/core"
CGO_ENABLED=0 go build -trimpath -tags xpack -ldflags "-s -w" -o "$ROOT_DIR/bin/1panel-core" ./cmd/server > /tmp/core.log 2>&1 && echo "CORE_OK" || { tail -20 /tmp/core.log; exit 1; }

cd "$ROOT_DIR/agent"
CGO_ENABLED=0 go build -trimpath -tags xpack -ldflags "-s -w" -o "$ROOT_DIR/bin/1panel-agent" ./cmd/server > /tmp/agent.log 2>&1 && echo "AGENT_OK" || { tail -20 /tmp/agent.log; exit 1; }

ls -la --time-style=+%H:%M "$ROOT_DIR/bin/"
echo "BUILD_DONE"
