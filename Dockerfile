# Multi-stage multi-arch Dockerfile
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS TARGETARCH VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Cache buster: 2026-04-17-rc5

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X main.version=${VERSION}" \
    -o /nas-doctor ./cmd/nas-doctor

# Runtime
FROM alpine:3.21

LABEL org.opencontainers.image.title="NAS Doctor" \
      org.opencontainers.image.description="Local NAS diagnostic and monitoring tool with SMART analysis, Prometheus metrics, and webhook alerts" \
      org.opencontainers.image.url="https://github.com/mcdays94/nas-doctor" \
      org.opencontainers.image.source="https://github.com/mcdays94/nas-doctor" \
      org.opencontainers.image.vendor="mcdays94" \
      org.opencontainers.image.licenses="MIT" \
      net.unraid.docker.icon="https://raw.githubusercontent.com/mcdays94/nas-doctor/main/icons/icon3.png" \
      net.unraid.docker.webui="http://[IP]:8060/" \
      net.unraid.docker.managed="dockerman"

COPY --from=builder /nas-doctor /app/nas-doctor

# Critical packages
RUN apk add --no-cache smartmontools docker-cli util-linux procps ca-certificates tzdata curl apcupsd nut-client
# Optional packages (may not be available on all architectures)
RUN apk add --no-cache hdparm iproute2 || true
RUN apk add --no-cache dmidecode ethtool || true
# Ookla speedtest CLI (for network speed test feature)
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then SARCH="x86_64"; \
    elif [ "$ARCH" = "aarch64" ]; then SARCH="aarch64"; \
    else SARCH=""; fi && \
    if [ -n "$SARCH" ]; then \
      curl -sL "https://install.speedtest.net/app/cli/ookla-speedtest-1.2.0-linux-${SARCH}.tgz" | tar xz -C /usr/local/bin speedtest && \
      chmod +x /usr/local/bin/speedtest; \
    fi || true

WORKDIR /app
VOLUME /data

ENV NAS_DOCTOR_LISTEN=":8060" \
    NAS_DOCTOR_DATA="/data" \
    NAS_DOCTOR_INTERVAL="30m" \
    TZ="UTC"

EXPOSE 8060

# Port-aware healthcheck: derives the port from NAS_DOCTOR_LISTEN so setting
# the env var (e.g. ":8067", "8067", "0.0.0.0:8067") keeps the healthcheck
# aligned with the actual listen address. Falls back to 8060 if unset/empty.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD sh -c 'P="${NAS_DOCTOR_LISTEN##*:}"; P="${P:-8060}"; wget -q --tries=1 --spider "http://localhost:${P}/api/v1/health"' || exit 1

ENTRYPOINT ["/app/nas-doctor"]
