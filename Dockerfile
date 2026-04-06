# ---- Build Stage ----
FROM golang:1-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nas-doctor ./cmd/nas-doctor

# ---- Runtime Stage ----
FROM alpine:3.21

# OCI labels (used by Unraid, Portainer, GHCR, etc.)
LABEL org.opencontainers.image.title="NAS Doctor" \
      org.opencontainers.image.description="Local NAS diagnostic and monitoring tool with SMART analysis, Prometheus metrics, and webhook alerts" \
      org.opencontainers.image.url="https://github.com/mcdays94/nas-doctor" \
      org.opencontainers.image.source="https://github.com/mcdays94/nas-doctor" \
      org.opencontainers.image.vendor="mcdays94" \
      org.opencontainers.image.licenses="MIT" \
      net.unraid.docker.icon="https://raw.githubusercontent.com/mcdays94/nas-doctor/main/icons/icon3.png" \
      net.unraid.docker.webui="http://[IP]:[PORT:8080]/" \
      net.unraid.docker.managed="dockerman"

RUN apk add --no-cache \
    smartmontools \
    hdparm \
    ethtool \
    iproute2 \
    docker-cli \
    dmidecode \
    util-linux \
    procps \
    ca-certificates \
    tzdata

# Create non-root user (though we need root for smartctl/dmesg)
# Run as root for hardware access
WORKDIR /app
COPY --from=builder /nas-doctor /app/nas-doctor

# Data volume
VOLUME /data

# Environment defaults
ENV NAS_DOCTOR_LISTEN=":8080" \
    NAS_DOCTOR_DATA="/data" \
    NAS_DOCTOR_INTERVAL="6h" \
    TZ="UTC"

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1

ENTRYPOINT ["/app/nas-doctor"]
