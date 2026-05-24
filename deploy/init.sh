#!/bin/sh
set -e

# ─── 生成 RDEV_ENCRYPTION_KEY（若未设置）─────────
if [ -z "${RDEV_ENCRYPTION_KEY:-}" ]; then
  # 容器重启不应每次生成新 key（会导致已加密数据无法解密）
  # 生产环境必须在 .env 中显式设置此变量
  echo "ERROR: RDEV_ENCRYPTION_KEY is not set." >&2
  echo "       Generate one with: openssl rand -hex 32" >&2
  echo "       Then add RDEV_ENCRYPTION_KEY=<value> to your .env file." >&2
  exit 1
fi

# ─── 等待 PostgreSQL 就绪 ─────────────────────────
echo "Waiting for postgres..."
until pg_isready_check; do
  sleep 2
done 2>/dev/null || true

# 使用内置的 healthcheck 逻辑等待数据库连接
MAX_WAIT=60
WAITED=0
while ! ./migrate status 2>/dev/null | grep -q "migrations"; do
  if [ "$WAITED" -ge "$MAX_WAIT" ]; then
    echo "Timed out waiting for database"
    exit 1
  fi
  echo "Waiting for database... (${WAITED}s)"
  sleep 2
  WAITED=$((WAITED + 2))
done 2>/dev/null || true

# ─── 数据库迁移 ────────────────────────────────────
echo "Running database migrations..."
./migrate up

# ─── 启动服务 ──────────────────────────────────────
echo "Starting rdev-server..."
exec ./server
