#!/bin/sh
# init.sh — RDev 服务启动初始化脚本
# 职责：
#   1. 生成 RDEV_ENCRYPTION_KEY（若 .env 未设置）
#   2. 等待 PostgreSQL 就绪
#   3. 执行数据库迁移
#   4. 启动 server

set -e

# ─── 1. 检查并生成加密密钥 ───────────────────────────────────────
if [ -z "$RDEV_ENCRYPTION_KEY" ] || [ "$RDEV_ENCRYPTION_KEY" = "CHANGE_ME_run_init_sh_to_auto_generate" ]; then
  echo "[init] RDEV_ENCRYPTION_KEY 未设置，正在生成随机密钥..."
  KEY=$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | head -c 32)
  export RDEV_ENCRYPTION_KEY="$KEY"
  echo "[init] 生成的 RDEV_ENCRYPTION_KEY: $KEY"
  echo "[init] 警告：重启后密钥将重新生成，建议将此值持久化到 .env"
fi

# ─── 2. 等待 PostgreSQL 就绪 ─────────────────────────────────────
DB_HOST="${DB_HOST:-postgres}"
DB_PORT="${DB_PORT:-5432}"

echo "[init] 等待 PostgreSQL ($DB_HOST:$DB_PORT) 就绪..."
MAX_WAIT=60
WAITED=0
until pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "${POSTGRES_USER:-rdev}" 2>/dev/null; do
  if [ $WAITED -ge $MAX_WAIT ]; then
    echo "[init] 错误：等待 PostgreSQL 超时（${MAX_WAIT}s）"
    exit 1
  fi
  sleep 2
  WAITED=$((WAITED + 2))
  echo "[init] 等待中... (${WAITED}s)"
done
echo "[init] PostgreSQL 已就绪"

# ─── 3. 数据库迁移 ──────────────────────────────────────────────
echo "[init] 执行数据库迁移..."
./migrate up
echo "[init] 迁移完成"

# ─── 4. 启动服务 ────────────────────────────────────────────────
echo "[init] 启动 rdev-server..."
exec ./server
