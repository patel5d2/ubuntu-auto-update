# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY . .
COPY config.conf .

RUN go mod tidy
RUN go get golang.org/x/crypto/ssh
RUN go get github.com/gorilla/websocket

RUN CGO_ENABLED=0 GOOS=linux go build -o /ua-backend ./cmd/api

# Stage 2: Create the final image
FROM alpine:latest

RUN apk add --no-cache postgresql-client

# Download and install migrate
RUN wget https://github.com/golang-migrate/migrate/releases/download/v4.15.2/migrate.linux-amd64.tar.gz -O migrate.tar.gz && \
    tar -xvf migrate.tar.gz && \
    mv migrate /usr/local/bin/migrate

WORKDIR /

COPY --from=builder /ua-backend .
COPY config.conf .
COPY encryption.key .
COPY known_hosts .
COPY db/migrations /migrations
COPY startup.sh .

CMD ["./startup.sh"]
