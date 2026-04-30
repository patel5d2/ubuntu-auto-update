# syntax=docker/dockerfile:1

# Stage 1: build the React frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /web
# Copy web dependencies and install
COPY web/package.json ./
RUN npm install
# Copy web source and build
COPY web/ ./
RUN npm run build

# Stage 2: build the Go binary
FROM golang:1.23-alpine AS backend-builder
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ua-backend ./cmd/api

# Stage 3: download golang-migrate to a known location
FROM alpine:3.20 AS migrate
ARG MIGRATE_VERSION=v4.18.1
# TARGETARCH is injected automatically by BuildKit (amd64 / arm64).
# On a plain `docker build` without --platform it resolves to the host arch.
ARG TARGETARCH
RUN apk add --no-cache curl ca-certificates && \
    curl -fsSL "https://github.com/golang-migrate/migrate/releases/download/${MIGRATE_VERSION}/migrate.linux-${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin migrate

# Stage 4: runtime dark container
FROM alpine:3.20
RUN apk add --no-cache ca-certificates postgresql-client wget && \
    adduser -D -u 1000 uau
WORKDIR /app

# Copy backend binary and scripts
COPY --from=backend-builder /out/ua-backend ./ua-backend
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY backend/db/migrations /app/migrations
COPY backend/config.conf ./config.conf
COPY backend/startup.sh ./startup.sh

# Copy frontend static assets
COPY --from=frontend-builder /web/dist /app/public

RUN chmod +x /app/startup.sh && \
    touch /app/known_hosts && \
    chown -R uau:uau /app

USER uau
EXPOSE 8080

ENV ENCRYPTION_KEY_FILE=/app/encryption.key \
    KNOWN_HOSTS_FILE=/app/known_hosts \
    MIGRATIONS_PATH=/app/migrations

CMD ["./startup.sh"]
