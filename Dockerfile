# Multi-stage build supporting linux/amd64 and linux/arm64
# Stage 1: Build static Go server
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod ./
COPY go.sum ./
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags="-s -w" \
    -o /mega-manager ./cmd/server

# Stage 2: Minimal runtime image with native megatools on AMD64 & ARM64
FROM alpine:3.21

# Install official megatools, CA certs, and tzdata for both amd64 and arm64
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    megatools --repository=https://dl-cdn.alpinelinux.org/alpine/edge/testing

# Copy Go server
COPY --from=builder /mega-manager /usr/local/bin/mega-manager

ENV PORT=3000 \
    EMAIL_PATTERN=mega%d \
    SIGNIN_INTERVAL_DAYS=30 \
    MEGATOOLS_PATH=/usr/bin/megatools \
    DATA_DIR=/data

EXPOSE 3000

VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/mega-manager"]
