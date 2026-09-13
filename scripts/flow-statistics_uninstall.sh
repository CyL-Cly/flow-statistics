#!/bin/sh
# flow-statistics agent uninstaller
#
# 用法:
#   wget -qO- https://dagongren.tech/public/flow-statistics/flow-statistics_uninstall.sh | sh
#   或: sh flow-statistics_uninstall.sh
#
# 环境变量:
#   FS_PURGE_CONF=1  同时删除 /etc/flow-statistics（含 ROUTER_ID）

set -e

INIT_NAME="flow-statistics"
BIN_NAME="traffic-agent"
BIN_PATH="/usr/sbin/${BIN_NAME}"
INIT_PATH="/etc/init.d/${INIT_NAME}"
CONF_DIR="/etc/flow-statistics"

log() { echo "[flow-statistics] $*"; }

if [ -x "$INIT_PATH" ]; then
  log "停止并禁用服务"
  "$INIT_PATH" stop 2>/dev/null || true
  "$INIT_PATH" disable 2>/dev/null || true
  rm -f "$INIT_PATH"
fi

# kill leftover
if command -v killall >/dev/null 2>&1; then
  killall "$BIN_NAME" 2>/dev/null || true
elif command -v pkill >/dev/null 2>&1; then
  pkill -f "$BIN_PATH" 2>/dev/null || true
fi

if [ -f "$BIN_PATH" ]; then
  log "删除二进制 $BIN_PATH"
  rm -f "$BIN_PATH"
fi

if [ "${FS_PURGE_CONF:-0}" = "1" ]; then
  if [ -d "$CONF_DIR" ]; then
    log "删除配置 $CONF_DIR"
    rm -rf "$CONF_DIR"
  fi
else
  log "保留配置目录 $CONF_DIR（重装可复用 ROUTER_ID）"
  log "彻底清除请: FS_PURGE_CONF=1 sh flow-statistics_uninstall.sh"
fi

echo
echo "=========================================="
echo " flow-statistics 已卸载"
echo "=========================================="
