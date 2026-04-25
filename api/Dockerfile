FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/app

# ─────────────────────────────────────────────────────────────────────────────

FROM alpine:3.22

RUN apk add --no-cache ca-certificates curl git docker-cli bash && \
    curl -fsSL https://railpack.io/install.sh | bash

WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 8080
CMD ["./server"]
