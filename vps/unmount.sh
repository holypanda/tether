#!/usr/bin/env bash
# ============================================
# 卸载 Windows SSHFS 挂载点
# ============================================
set -euo pipefail

# ==== 配置区: 保持与 mount.sh 一致 ====
MOUNT_POINT="$HOME/local-code"

# ==== 执行区 ====
if ! mountpoint -q "$MOUNT_POINT" 2>/dev/null; then
    echo "[INFO] $MOUNT_POINT 未挂载"
    exit 0
fi

echo "[INFO] 卸载 $MOUNT_POINT"

if fusermount -u "$MOUNT_POINT" 2>/dev/null; then
    echo "[OK] 已卸载"
    exit 0
fi

echo "[WARN] 普通卸载失败, 可能有进程占用, 尝试懒卸载..."
fusermount -uz "$MOUNT_POINT"
echo "[OK] 已懒卸载 (延迟释放)"
