# Multi-stage build producing an ultra-lightweight scratch Docker image
# Stage 1: Build static Go binary and prepare dependencies
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache ca-certificates curl tar

WORKDIR /src

# Copy dependency definitions and sources
COPY go.mod ./
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/

# Fetch architecture-specific static megatools binary (x86_64 or aarch64)
RUN if [ "$TARGETARCH" = "arm64" ]; then \
      curl -fsSL "https://megatools.megous.com/builds/megatools-1.11.1-linux-aarch64.tar.gz" -o /tmp/megatools.tar.gz || \
      curl -fsSL "https://xff.cz/megatools/builds/builds/megatools-1.11.1-linux-aarch64.tar.gz" -o /tmp/megatools.tar.gz || true; \
    else \
      curl -fsSL "https://megatools.megous.com/builds/megatools-1.11.1-linux-x86_64.tar.gz" -o /tmp/megatools.tar.gz || \
      curl -fsSL "https://xff.cz/megatools/builds/builds/megatools-1.11.1-linux-x86_64.tar.gz" -o /tmp/megatools.tar.gz || true; \
    fi && \
    if [ -f /tmp/megatools.tar.gz ] && [ -s /tmp/megatools.tar.gz ]; then \
      tar -xzf /tmp/megatools.tar.gz -C /tmp/ && \
      cp /tmp/megatools-*/megatools /megatools; \
    else \
      apk add --no-cache megatools && cp $(which megatools) /megatools; \
    fi

# Build static Go binary without CGO targeting TARGETOS / TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags="-s -w -extldflags '-static'" \
    -o /mega-manager ./cmd/server

# Stage 2: Minimal scratch image
FROM scratch

# Copy SSL root certificates for HTTPS calls to MEGA API & Mailgun
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy static megatools binary and Go server
COPY --from=builder /megatools /usr/local/bin/megatools
COPY --from=builder /mega-manager /mega-manager

# Default environment variables
ENV PORT=3000 \
    EMAIL_PATTERN=mega%d \
    SIGNIN_INTERVAL_DAYS=30 \
    MEGATOOLS_PATH=/usr/local/bin/megatools \
    DATA_DIR=/data

EXPOSE 3000

# Mountable volume for accounts.csv and accounts.json
VOLUME ["/data"]

ENTRYPOINT ["/mega-manager"]
