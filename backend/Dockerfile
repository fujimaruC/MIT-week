# syntax=docker/dockerfile:1.7
# ponytail: multi-stage dev/prod in single Dockerfile; split when CI matrices diverge
FROM golang:1.27-alpine AS base
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata ffmpeg wget && \
    wget -q https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.26/yt-dlp_musllinux -O /usr/local/bin/yt-dlp && \
    echo "e97a4dd919221d233e7720e18fa3f66db60fbd1d8c0846bf6deb8e88e04475a8  /usr/local/bin/yt-dlp" | sha256sum -c - && \
    chmod a+rx /usr/local/bin/yt-dlp

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Target: dev (Hot reload with Air)
FROM base AS dev
RUN go install github.com/air-verse/air@v1.61.7
CMD ["air", "-c", ".air.toml"]

# Target: build (Static Go compilation)
FROM base AS build
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/videoprobe ./cmd/videoprobe

# Target: prod (Minimal Alpine runtime with ffmpeg and pinned yt-dlp)
FROM alpine:3.21 AS prod
RUN apk add --no-cache ca-certificates tzdata ffmpeg wget && \
    wget -q https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.26/yt-dlp_musllinux -O /usr/local/bin/yt-dlp && \
    echo "e97a4dd919221d233e7720e18fa3f66db60fbd1d8c0846bf6deb8e88e04475a8  /usr/local/bin/yt-dlp" | sha256sum -c - && \
    chmod a+rx /usr/local/bin/yt-dlp && \
    addgroup -g 10001 -S amba && \
    adduser -u 10001 -S amba -G amba

USER 10001:10001
WORKDIR /app
COPY --from=build --chown=10001:10001 /bin/server /app/server
COPY --from=build --chown=10001:10001 /bin/videoprobe /usr/local/bin/videoprobe

# Safe local defaults. Hosted deployments override these (fly.toml [env],
# Railway variables); see docs/DEPLOY.md. SERVER_PORT is left unset so a
# PaaS-injected PORT is honoured, with 8080 as the fallback.
ENV SERVER_HOST=0.0.0.0 \
    ENVIRONMENT=development \
    LLM_MOCK=true

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${SERVER_PORT:-${PORT:-8080}}/healthz" || exit 1

ENTRYPOINT ["/app/server"]
