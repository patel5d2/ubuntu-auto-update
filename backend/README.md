# Backend (Go API)

REST + WebSocket API for the Ubuntu Auto-Update dashboard.

## Layout

```
cmd/api/main.go         HTTP server, route registration, graceful shutdown
pkg/config/             Viper-based loader for backend/config.conf
pkg/crypto/             AES-GCM helpers; reads ENCRYPTION_KEY_FILE
pkg/db/                 pgx queries (uses pgx.CollectRows)
pkg/middleware/         Auth, CORS, ErrorHandler, structured request logging
pkg/models/             DB-tagged Go structs (Host, SSHKey, Webhook, HostReport)
pkg/ssh/                Cached known_hosts callback + ConnectToHost helper
pkg/webhook/            Sender + retrying async Dispatcher
db/migrations/          golang-migrate up-only SQL (000001…000009)
```

## Running locally

```bash
export DATABASE_URL="postgres://uau:uau@localhost:5432/uau_db?sslmode=disable"
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD=change-me
export ENROLLMENT_TOKEN=dev-enrollment-token
go run ./cmd/api
```

Migrations are applied separately:

```bash
go run ./migrate                          # if you wired up a migrate binary
# or with the official CLI:
migrate -path db/migrations -database "$DATABASE_URL" up
```

## Tests

```bash
go test ./...
```

`pkg/middleware`, `pkg/crypto`, and `pkg/webhook` have unit tests. `pkg/db`
needs a real Postgres — wire it up with `testcontainers` if you add coverage.

## Environment variables

See the project root [`.env.example`](../.env.example) for the full list.
Required at startup: `DATABASE_URL`, `ADMIN_USERNAME`, `ADMIN_PASSWORD`,
`ENROLLMENT_TOKEN`. Optional: `API_PORT`, `CORS_ALLOWED_ORIGINS`,
`ENVIRONMENT`, `ENCRYPTION_KEY_FILE`, `KNOWN_HOSTS_FILE`.

## Adding a new endpoint

1. Add the handler method on `*Application` in `cmd/api/main.go`.
2. Register it under the appropriate router (`r` for public, `api` for
   authenticated).
3. Add `.Methods(http.MethodGet|Post|...)` — never leave a route
   verb-unrestricted; WebSocket handshakes are GET only.
4. If it touches the DB, add a function to `pkg/db/db.go` and use the
   `pgx.CollectRows` / `CollectExactlyOneRow` pattern rather than manual
   `Scan` chains.
