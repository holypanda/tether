#!/usr/bin/env bash
# ============================================
# 一键脚本: 挂载 Windows 本地目录并准备使用 (VPS 端)
# - 自动等待 Windows 端反向隧道就绪
# - 自动读取 Windows 端推来的配置
# - 自动挂载, 幂等 (重复运行无副作用)
# ============================================
set -euo pipefail

CONFIG_FILE="$HOME/.windows-auto-stim.env"

if [ ! -f "$CONFIG_FILE" ]; then
    echo "[ERROR] 配置文件 $CONFIG_FILE 不存在"
    echo "请先在 Windows 上双击 tunnel.bat 完成首次配置 (会自动推送配置到 VPS)"
    exit 1
fi

# shellcheck disable=SC1090
source "$CONFIG_FILE"
: "${WIN_USER:?WIN_USER 未设置}"
: "${WIN_PATH:?WIN_PATH 未设置}"
: "${TUNNEL_PORT:?TUNNEL_PORT 未设置}"
: "${MOUNT_POINT:=$HOME/local-code}"

if ! command -v sshfs >/dev/null 2>&1; then
    echo "[ERROR] sshfs 未安装, 运行: sudo apt install sshfs"
    exit 1
fi

# 已挂载直接退出
if mountpoint -q "$MOUNT_POINT" 2>/dev/null; then
    echo "[INFO] $MOUNT_POINT 已挂载"
    echo "进入: cd $MOUNT_POINT && claude"
    exit 0
fi

# 等待隧道 (最多 30s)
echo "[INFO] 等待 Windows 反向隧道 (端口 $TUNNEL_PORT)..."
for i in $(seq 1 30); do
    if ss -tln 2>/dev/null | grep -q ":$TUNNEL_PORT "; then
        echo "[OK] 隧道已就绪"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "[ERROR] 30s 内未检测到隧道, 请确认 Windows 端 tunnel.bat 正在运行"
        exit 1
    fi
    sleep 1
done

# 挂载
mkdir -p "$MOUNT_POINT"
echo "[INFO] 挂载 ${WIN_USER}@localhost:${WIN_PATH} -> ${MOUNT_POINT}"

sshfs \
    -p "$TUNNEL_PORT" \
    "${WIN_USER}@localhost:${WIN_PATH}" \
    "$MOUNT_POINT" \
    -o reconnect \
    -o ServerAliveInterval=15 \
    -o ServerAliveCountMax=3 \
    -o StrictHostKeyChecking=accept-new \
    -o cache=yes \
    -o compression=no \
    -o transform_symlinks \
    -o follow_symlinks

echo ""
echo "========================================"
echo "[OK] 挂载完成"
echo "   挂载点: $MOUNT_POINT"
echo "   进入:   cd $MOUNT_POINT && claude"
echo "   卸载:   $(dirname "$0")/unmount.sh"
echo "========================================"
