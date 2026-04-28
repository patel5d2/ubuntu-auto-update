# Ubuntu Auto-Update

Centralised patch-management for fleets of Ubuntu hosts. A small Rust agent
runs on each managed host, reports state to a Go API, and a React dashboard
lets an operator inspect hosts and trigger update previews over SSH.

## Architecture

```
┌──────────────┐     POST /api/v1/report       ┌──────────────┐
│  agent (Rust)│ ─────────────────────────────▶│ backend (Go) │
│  on each VM  │◀──── enrollment / config ─────│   + Postgres │
└──────────────┘                               └──────┬───────┘
                                                      │
                                          fetch / WS  │
                                                      ▼
                                              ┌──────────────┐
                                              │  web (React) │
                                              │   + Pico CSS │
                                              └──────────────┘
```

| Component | Path | Stack | What it does |
|-----------|------|-------|--------------|
| Agent     | [`agent/`](agent/README.md)     | Rust, systemd timer | Runs `apt update`/`apt upgrade`, posts results to the API. |
| Backend   | [`backend/`](backend/README.md) | Go 1.23, pgx, gorilla/mux | REST + WebSocket API; stores hosts, encrypted SSH keys, webhooks. |
| Web       | [`web/`](web/README.md)         | React 18, TypeScript, Vite, Pico CSS | Operator dashboard; lists hosts, streams update previews. |

## Quick start (Docker)

```bash
git clone https://github.com/patel5d2/ubuntu-auto-update.git
cd ubuntu-auto-update

cp .env.example .env             # then edit secrets
docker-compose up -d

# Web UI:  http://localhost:5173
# API:     http://localhost:8080/api/v1/health
```

The first login uses `ADMIN_USERNAME` / `ADMIN_PASSWORD` from `.env`.

## Quick start (local dev, no Docker)

```bash
# 1. Postgres running with DATABASE_URL set in your shell
cd backend && go run ./cmd/api          # API on :8080

# 2. In another shell
cd web && npm install && npm run dev    # Vite on :5173

# 3. (Optional) on a managed host
cd agent && cargo run -- run
```

See [`DEVELOPMENT.md`](DEVELOPMENT.md) for the full development workflow.

## Configuration

Every tunable is an environment variable. See [`.env.example`](.env.example)
for the complete contract; the most important ones are `DATABASE_URL`,
`ADMIN_USERNAME` / `ADMIN_PASSWORD`, `ENROLLMENT_TOKEN`, and
`ENCRYPTION_KEY_FILE`.

The backend will also pick up keys from `backend/config.conf` (via Viper)
and dump them into the process environment at startup; the process env
takes precedence over the file.

## Project layout

```
agent/      Rust agent + systemd units + Dockerfile
backend/    Go API + pkg/{config,crypto,db,middleware,models,ssh,webhook}
            cmd/api/main.go         HTTP/WebSocket server
            db/migrations/          golang-migrate up-only SQL
web/        Vite + React + TypeScript dashboard (Pico CSS)
scripts/    build.sh, test.sh wrappers for all three components
```

## Useful endpoints

| Method | Path                                              | Auth        | Purpose |
|--------|---------------------------------------------------|-------------|---------|
| GET    | `/api/v1/health`                                  | public      | Liveness + DB ping |
| POST   | `/api/v1/login`                                   | public      | Issues bearer token + Set-Cookie |
| POST   | `/api/v1/logout`                                  | public      | Best-effort token revocation |
| POST   | `/api/v1/enroll`                                  | enrollment  | Agent → long-lived bearer token |
| POST   | `/api/v1/report`                                  | bearer      | Agent uploads update output |
| GET    | `/api/v1/hosts`                                   | bearer      | List all hosts |
| GET    | `/api/v1/hosts/{id}`                              | bearer      | Host detail |
| POST   | `/api/v1/hosts/{id}/ssh-key`                      | bearer      | Store encrypted SSH key |
| GET    | `/api/v1/hosts/{id}/preview-updates` (WebSocket)  | bearer      | Stream `apt list --upgradable` |
| GET    | `/api/v1/hosts/{id}/execute-script` (WebSocket)   | bearer      | Stream output of a user-supplied script |
| POST   | `/api/v1/webhooks`                                | bearer      | Subscribe to events |

## Contributing

PRs welcome. Run `./scripts/build.sh` then `./scripts/test.sh` before
pushing. Open an issue first for anything substantial.

## License

MIT — see [`LICENSE`](LICENSE).
