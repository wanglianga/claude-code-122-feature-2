# 多阶段构建：前端 Vite 构建 → Go embed 单二进制 → postgres-alpine 运行（自带 psql/pg_isready）

# ---------- 1) 前端构建 ----------
FROM node:22-bookworm-slim AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci --cache /tmp/npm-cache; else npm install --cache /tmp/npm-cache; fi
COPY frontend/ ./
# 产物输出到 /backend/web/dist（vite.config.js 中 outDir=../backend/web/dist）
RUN npm run build

# ---------- 2) Go 后端（embed 前端产物与 SQL） ----------
FROM golang:1.21-bookworm AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=web /backend/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/kegplatform .

# ---------- 3) 运行镜像 ----------
FROM postgres:15-alpine
# postgres:15-alpine 已含 psql / pg_isready，供入口脚本等待数据库并判断是否初始化
COPY --from=api /out/kegplatform /app/kegplatform
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
ENV KEG_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/app/docker-entrypoint.sh"]
