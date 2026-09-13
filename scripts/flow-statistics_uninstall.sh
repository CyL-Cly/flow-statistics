#!/bin/sh
# flow-statistics agent uninstaller
#
# Usage:
#   wget -qO- https://dagongren.tech/public/flow-statistics/flow-statistics_uninstall.sh | sh
#   or: sh flow-statistics_uninstall.sh
#
# Env:
#   FS_PURGE_CONF=1  also delete /etc/flow-statistics (including ROUTER_ID)

set -e

INIT_NAME="flow-statistics"
BIN_NAME="traffic-agent"
BIN_PATH="/usr/sbin/${BIN_NAME}"
INIT_PATH="/etc/init.d/${INIT_NAME}"
CONF_DIR="/etc/flow-statistics"

log() { echo "[flow-statistics] $*"; }

if [ -x "$INIT_PATH" ]; then
  log "stop and disable service"
  "$INIT_PATH" stop 2>/dev/null || true
  "$INIT_PATH" disable 2>/dev/null || true
  rm -f "$INIT_PATH"
fi

if command -v killall >/dev/null 2>&1; then
  killall "$BIN_NAME" 2>/dev/null || true
elif command -v pkill >/dev/null 2>&1; then
  pkill -f "$BIN_PATH" 2>/dev/null || true
fi

if [ -f "$BIN_PATH" ]; then
  log "remove binary $BIN_PATH"
  rm -f "$BIN_PATH"
fi

if [ "${FS_PURGE_CONF:-0}" = "1" ]; then
  if [ -d "$CONF_DIR" ]; then
    log "remove config $CONF_DIR"
    rm -rf "$CONF_DIR"
  fi
else
  log "keep config dir $CONF_DIR (reinstall can reuse ROUTER_ID)"
  log "purge with: FS_PURGE_CONF=1 sh flow-statistics_uninstall.sh"
fi

echo
echo "=========================================="
echo " flow-statistics uninstalled"
echo "=========================================="
