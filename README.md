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

./quickstart.sh                  # generates .env with random secrets, brings up the stack

# Web UI:  http://localhost:8080
# API:     http://localhost:8080/api/v1/health
```

`quickstart.sh up` is the one-command path (builds locally). To run the
pre-built image from GHCR instead of building:

```bash
cp .env.example .env                       # fill in ADMIN_PASSWORD + ENROLLMENT_TOKEN
docker compose pull app
docker compose up -d --no-build
```

Other helpers:

| Command                  | What it does |
|--------------------------|--------------|
| `./quickstart.sh up`     | Generate `.env` (if missing) + bring stack up. |
| `./quickstart.sh down`   | Stop containers, keep the DB volume. |
| `./quickstart.sh reset`  | Stop **and** delete the DB volume (typed-confirm). |
| `./quickstart.sh logs`   | Tail compose logs. |
| `./quickstart.sh status` | Show container health. |

The first login uses `ADMIN_USERNAME` (`admin`) / `ADMIN_PASSWORD` from
`.env`. The script prints the generated password once on first run — copy
it then or read it back from `.env` later.

## Quick start (local dev)

Hot-reload for all three services (Postgres + Go backend + Vite), with every
secret and connection string already baked in — no local toolchains, no `.env`
to fill in:

```bash
docker compose -f docker-compose.dev.yml up
# Web UI:  http://localhost:5173   (proxies /api to the backend on :8080)
```

Prefer running on the metal? The backend needs Postgres reachable and a few
env vars, or it exits immediately (`DATABASE_URL environment variable not set`,
and `ADMIN_PASSWORD` must be ≥12 chars). Set them explicitly:

```bash
# 1. Postgres (easiest: just the compose service — uses the uau/uau/uau_db
#    defaults below; if you override POSTGRES_USER/PASSWORD/DB, match them in
#    DATABASE_URL too)
docker compose -f docker-compose.dev.yml up -d postgres

# 2. Backend — export its required env, then run
export DATABASE_URL='postgres://uau:uau@localhost:5432/uau_db?sslmode=disable'
export ADMIN_USERNAME=admin ADMIN_PASSWORD=change-me-please ENROLLMENT_TOKEN=dev-enrollment-token
cd backend && go run ./cmd/api          # API on :8080

# 3. Frontend, in another shell (from the repo root)
cd web && npm install && npm run dev    # Vite on :5173

# 4. (Optional) on a managed host, from the repo root
cd agent && cargo run -- run
```

See [`DEVELOPMENT.md`](DEVELOPMENT.md) for the full development workflow.

## Configuration

Every tunable is an environment variable. See [`.env.example`](.env.example)
for the complete contract; the most important ones are `DATABASE_URL`,
`ADMIN_USERNAME` / `ADMIN_PASSWORD`, `ENROLLMENT_TOKEN`, and
`ENCRYPTION_KEY_FILE`. Operational tuning: `RUN_RETENTION_DAYS` (prune run
history older than N days; default 90, `0` disables) and
`OFFLINE_AFTER_MINUTES` (mark hosts offline and fire the `host_offline`
webhook after N minutes without a report; default 15).

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
| GET    | `/api/v1/hosts`                                   | bearer      | List hosts (optional `?limit=&offset=`) |
| GET    | `/api/v1/reports/compliance`                      | bearer      | Fleet patch-status report (`?format=csv` to export) |
| POST   | `/api/v1/hosts`                                   | bearer      | Operator-create a host (no agent) |
| GET    | `/api/v1/hosts/{id}`                              | bearer      | Host detail |
| PATCH  | `/api/v1/hosts/{id}`                              | bearer      | Edit `ssh_user` and/or `tags` |
| DELETE | `/api/v1/hosts/{id}`                              | bearer      | Delete host (requires `X-Confirm-Hostname`) |
| POST   | `/api/v1/hosts/{id}/ssh-key`                      | bearer      | Store encrypted SSH key |
| POST   | `/api/v1/hosts/{id}/test-connection`              | bearer      | Probe SSH + sudo, return latency |
| GET    | `/api/v1/hosts/{id}/preview-updates` (WebSocket)  | bearer      | Stream `apt list --upgradable` |
| GET    | `/api/v1/hosts/{id}/run-update` (WebSocket)       | bearer      | Stream a real `apt-get upgrade -y` |
| GET    | `/api/v1/hosts/{id}/execute-script` (WebSocket)   | bearer      | Stream output of a user-supplied script |
| GET    | `/api/v1/hosts/{id}/runs?limit=`                  | bearer      | Paginated update history for a host |
| POST   | `/api/v1/hosts/bulk/run-update`                   | bearer      | Fan out an update across many hosts (`security_only` for unattended-upgrade) |
| POST   | `/api/v1/hosts/bulk/run-playbook`                 | bearer      | Fan a playbook across many hosts |
| POST   | `/api/v1/hosts/bulk/reboot`                       | bearer      | Reboot hosts and verify they come back |
| GET/POST | `/api/v1/playbooks`                             | bearer      | Playbook library (CRUD) |
| GET/POST | `/api/v1/tokens`                                | admin       | Long-lived API tokens (`uat_…`, secret shown once) |
| GET    | `/api/v1/runs?group_id=`                          | bearer      | All runs in a bulk group |
| GET    | `/api/v1/runs/{id}`                               | bearer      | Single run record + full output |
| GET    | `/api/v1/events` (WebSocket)                      | bearer      | Multiplexed real-time channel (`{table, op, id}`) |
| POST   | `/api/v1/webhooks`                                | bearer      | Subscribe to events |
| GET    | `/api/v1/overview`                                | bearer      | Fleet stats for the dashboard landing page |
| GET    | `/api/v1/schedules`                               | bearer      | List recurring update schedules |
| POST   | `/api/v1/schedules`                               | bearer      | Create a schedule (`name`, `host_ids`, `interval_minutes`, optional `start_at`) |
| PATCH  | `/api/v1/schedules/{id}`                          | bearer      | Enable/disable a schedule |
| DELETE | `/api/v1/schedules/{id}`                          | bearer      | Delete a schedule |

The events WebSocket is fed by a Postgres `LISTEN` goroutine and a `pg_notify`
trigger on `hosts` and `update_runs` (migration 000012). The browser opens
exactly one connection per session and filters client-side. On reconnect the
server emits a `{op: "snapshot"}` event so clients re-fetch authoritative
state via REST.

## Contributing

PRs welcome. Run `./scripts/build.sh` then `./scripts/test.sh` before
pushing. Open an issue first for anything substantial.

## License

MIT — see [`LICENSE`](LICENSE).
