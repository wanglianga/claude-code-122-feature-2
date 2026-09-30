#!/bin/sh
# 应用容器入口：等待 PostgreSQL → 首次启动自动建库加载种子 → 启动服务
set -e

DSN="${KEG_DSN:-host=db port=5432 user=keg password=keg dbname=kegplatform sslmode=disable}"
export KEG_DSN="$DSN"
export KEG_ADDR="${KEG_ADDR:-:8080}"

# 从 DSN 中取 host/user 供 pg_isready 使用（默认 compose 服务名 db / 用户 keg）
HOST=$(echo "$DSN" | sed -n 's/.*host=\([^ ]*\).*/\1/p'); HOST=${HOST:-db}
USER=$(echo "$DSN" | sed -n 's/.*user=\([^ ]*\).*/\1/p'); USER=${USER:-keg}

echo "[entrypoint] 等待 PostgreSQL ${HOST} 就绪..."
until pg_isready -h "$HOST" -U "$USER" >/dev/null 2>&1; do
  sleep 1
done

# 仅在库为空（无业务表）时加载 schema + 种子，保证容器重启/卷持久化时不重复插入
TABLES=$(psql "$DSN" -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'" 2>/dev/null || echo 0)
if [ "$TABLES" = "0" ]; then
  echo "[entrypoint] 数据库为空，加载 schema 与种子数据"
  /app/kegplatform init
else
  echo "[entrypoint] 检测到已有 ${TABLES} 张表，跳过初始化"
fi

echo "[entrypoint] 启动服务 http://localhost${KEG_ADDR}"
exec /app/kegplatform
