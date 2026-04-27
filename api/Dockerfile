FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/app && \
    go install github.com/pressly/goose/v3/cmd/goose@latest

# ─────────────────────────────────────────────────────────────────────────────

FROM debian:bookworm-slim
ARG TARGETARCH
ARG RAILPACK_VERSION=v0.23.0
ARG MISE_VERSION=v2026.4.24
ARG BUILDX_VERSION=v0.33.0

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl git docker.io bash; \
    rm -rf /var/lib/apt/lists/*; \
    case "${TARGETARCH}" in \
      amd64) railpack_arch="x86_64" ;; \
      arm64) railpack_arch="arm64" ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    case "${TARGETARCH}" in \
      amd64) mise_arch="x64" ;; \
      arm64) mise_arch="arm64" ;; \
      *) echo "unsupported TARGETARCH for mise: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    case "${TARGETARCH}" in \
      amd64|arm64) buildx_arch="${TARGETARCH}" ;; \
      *) echo "unsupported TARGETARCH for buildx: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/railwayapp/railpack/releases/download/${RAILPACK_VERSION}/railpack-${RAILPACK_VERSION}-${railpack_arch}-unknown-linux-musl.tar.gz" -o /tmp/railpack.tar.gz; \
    curl -fsSL "https://github.com/jdx/mise/releases/download/${MISE_VERSION}/mise-${MISE_VERSION}-linux-${mise_arch}.tar.gz" -o /tmp/mise.tar.gz; \
    mkdir -p /usr/libexec/docker/cli-plugins; \
    curl -fsSL "https://github.com/docker/buildx/releases/download/${BUILDX_VERSION}/buildx-${BUILDX_VERSION}.linux-${buildx_arch}" -o /usr/libexec/docker/cli-plugins/docker-buildx; \
    tar -xzf /tmp/railpack.tar.gz -C /usr/local/bin railpack; \
    mkdir -p /opt/mise /var/lib/mise /var/cache/mise /var/tmp/mise; \
    tar -xzf /tmp/mise.tar.gz -C /opt/mise; \
    ln -sf /opt/mise/mise/bin/mise /usr/local/bin/mise; \
    chmod +x /opt/mise/mise/bin/mise /usr/local/bin/mise /usr/libexec/docker/cli-plugins/docker-buildx; \
    rm -f /tmp/railpack.tar.gz /tmp/mise.tar.gz

ENV MISE_DATA_DIR=/var/lib/mise \
    MISE_CACHE_DIR=/var/cache/mise \
    MISE_TMP_DIR=/var/tmp/mise \
    TMPDIR=/var/tmp/mise

WORKDIR /app
COPY --from=builder /app/server .
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY migrations ./migrations
COPY entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

EXPOSE 8080
CMD ["./entrypoint.sh"]
