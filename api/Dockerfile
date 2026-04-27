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
      amd64) buildx_arch="amd64" ;; \
      arm64) buildx_arch="arm64" ;; \
      *) echo "unsupported TARGETARCH for buildx: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    railpack_version="$(curl --fail --silent --show-error https://api.github.com/repos/railwayapp/railpack/releases/latest | grep -o '"tag_name": "v[^"]*"' | cut -d'"' -f4)"; \
    mise_version="$(curl --fail --silent --show-error https://api.github.com/repos/jdx/mise/releases/latest | grep -o '"tag_name": "v[^"]*"' | cut -d'"' -f4)"; \
    buildx_version="$(curl --fail --silent --show-error https://api.github.com/repos/docker/buildx/releases/latest | grep -o '"tag_name": "v[^"]*"' | cut -d'"' -f4)"; \
    curl -fsSL "https://github.com/railwayapp/railpack/releases/download/${railpack_version}/railpack-${railpack_version}-${railpack_arch}-unknown-linux-musl.tar.gz" -o /tmp/railpack.tar.gz; \
    curl -fsSL "https://github.com/jdx/mise/releases/download/${mise_version}/mise-${mise_version}-linux-${mise_arch}.tar.gz" -o /tmp/mise.tar.gz; \
    mkdir -p /usr/libexec/docker/cli-plugins; \
    curl -fsSL "https://github.com/docker/buildx/releases/download/${buildx_version}/buildx-${buildx_version}.linux-${buildx_arch}" -o /usr/libexec/docker/cli-plugins/docker-buildx; \
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
