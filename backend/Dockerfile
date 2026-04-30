# syntax=docker/dockerfile:1
#
# Stage 1: build the Go binary.
FROM golang:1.23-alpine AS builder

WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a fully static binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ua-backend ./cmd/api


# Stage 2: download golang-migrate to a known location.
FROM alpine:3.20 AS migrate
ARG MIGRATE_VERSION=v4.18.1
# TARGETARCH is injected automatically by BuildKit (amd64 / arm64).
ARG TARGETARCH
RUN apk add --no-cache curl ca-certificates && \
    curl -fsSL "https://github.com/golang-migrate/migrate/releases/download/${MIGRATE_VERSION}/migrate.linux-${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin migrate


# Stage 3: runtime image.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates postgresql-client wget && \
    adduser -D -u 1000 uau

WORKDIR /app

COPY --from=builder /out/ua-backend ./ua-backend
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY db/migrations /app/migrations
COPY config.conf ./config.conf
COPY startup.sh ./startup.sh

# Always start with empty known_hosts and a 32-byte random encryption key
# unless the operator mounts their own. Persisting these to a volume would
# also work; for the README quickstart we generate them on first start.
RUN chmod +x /app/startup.sh && \
    touch /app/known_hosts && \
    chown -R uau:uau /app

USER uau

EXPOSE 8080

ENV ENCRYPTION_KEY_FILE=/app/encryption.key \
    KNOWN_HOSTS_FILE=/app/known_hosts \
    MIGRATIONS_PATH=/app/migrations

CMD ["./startup.sh"]
