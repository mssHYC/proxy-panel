# syntax=docker/dockerfile:1.7

# ============================================================
# Stage 1: 前端构建
# ============================================================
FROM node:20-alpine AS web-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ============================================================
# Stage 2: Go 后端构建（内嵌前端 dist）
# ============================================================
FROM golang:1.25-alpine AS go-builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web-builder /src/web/dist ./web/dist
ENV CGO_ENABLED=0 GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/proxy-panel ./cmd/server

# ============================================================
# Stage 3: 运行时（镜像中仅面板自身；Xray/Sing-box 内核建议宿主机或 sidecar 容器提供）
# ============================================================
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -h /app -u 1000 panel
WORKDIR /app
COPY --from=go-builder /out/proxy-panel /app/proxy-panel
COPY config.example.yaml /app/config.example.yaml

# 数据目录（挂卷持久化），备份/恢复 API 会在此目录原子替换 SQLite 文件
RUN mkdir -p /app/data /app/kernel && chown -R panel:panel /app
USER panel

EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/app/proxy-panel"]
CMD ["-config", "/app/config.yaml"]
