# syntax=docker/dockerfile:1

# Stage 1: build the React frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /web
# Copy web dependencies and install from the lockfile for reproducible builds
COPY web/package.json web/package-lock.json ./
RUN npm ci
# Copy web source and build
COPY web/ ./
RUN npm run build

# Stage 2: build the Go binary
FROM golang:1.26-alpine AS backend-builder
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ua-backend ./cmd/api

# Stage 3: build golang-migrate from source with our (patched) toolchain.
# The prebuilt release binaries ship compiled with an old Go and a huge dep
# tree (every driver linked), which Trivy rightly flags; building with only
# the postgres driver prunes all of that.
FROM golang:1.26-alpine AS migrate
ARG MIGRATE_VERSION=v4.19.1
RUN CGO_ENABLED=0 GOBIN=/usr/local/bin go install -trimpath -ldflags="-s -w" \
    -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_VERSION}

# Stage 4: runtime dark container
FROM alpine:3.22
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
    mkdir -p /app/keys && \
    chown -R uau:uau /app

USER uau
EXPOSE 8080

ENV ENCRYPTION_KEY_FILE=/app/keys/encryption.key \
    KNOWN_HOSTS_FILE=/app/known_hosts \
    MIGRATIONS_PATH=/app/migrations

CMD ["./startup.sh"]
