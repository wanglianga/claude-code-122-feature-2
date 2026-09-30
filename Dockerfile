# syntax=docker/dockerfile:1

# ---------- 阶段 1：构建前端 ----------
FROM node:18-bookworm-slim AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci --no-audit --no-fund || npm install --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---------- 阶段 2：构建 Go 后端（前端产物 embed 进二进制） ----------
FROM golang:1.19-bookworm AS backend
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# 用阶段 1 的构建产物覆盖占位的 web/dist
COPY --from=frontend /app/backend/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/kegplatform .

# ---------- 阶段 3：运行镜像 ----------
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates postgresql-client \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /out/kegplatform /app/kegplatform
EXPOSE 8080
# auto：数据库为空时自动建表 + 种子数据；KEG_DSN 由 compose 注入
ENTRYPOINT ["/app/kegplatform", "auto"]
