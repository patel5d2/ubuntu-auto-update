# Development Guide

This document provides comprehensive instructions for developing, building, and testing the Ubuntu Auto-Update system.

## 🏗️ Quick Start

### Prerequisites

- **Rust**: Latest stable (install from [rustup.rs](https://rustup.rs/))
- **Go**: 1.22+ 
- **Node.js**: 20+
- **Docker**: Latest stable
- **Docker Compose**: v2.0+

### One-Command Setup

```bash
# Clone and setup the entire stack
git clone https://github.com/patel5d2/ubuntu-auto-update.git
cd ubuntu-auto-update

# Make scripts executable
chmod +x scripts/*.sh

# Build everything
./scripts/build.sh

# Run tests
./scripts/test.sh

# Start development environment
docker compose -f docker-compose.dev.yml up -d
```

## 🔧 Component Development

### Rust Agent (`/agent`)

```bash
# Development
cd agent
cargo check                    # Fast syntax check
cargo test                     # Run tests
cargo clippy                   # Linting
cargo run -- status           # Run agent

# Build release
cargo build --release

# Security audit
cargo audit

# Generate config
cargo run -- generate-config --output agent.toml
```

**Key Files:**
- `src/main.rs` - CLI interface and orchestration
- `src/config.rs` - Configuration management
- `src/updater.rs` - Update execution logic
- `src/http_client.rs` - Secure backend communication
- `src/metrics.rs` - Prometheus metrics collection

### Go Backend (`/backend`)

```bash
# Development
cd backend
go run cmd/api/main.go         # Start server
go test ./...                  # Run tests
go vet ./...                   # Static analysis

# Hot reload (with Air)
air -c .air.toml

# Build
go build -o bin/backend ./cmd/api

# Database migrations
migrate -path db/migrations -database "$DATABASE_URL" up
```

**Key Files:**
- `cmd/api/main.go` - API server entry point
- `pkg/config/` - Configuration management
- `pkg/models/` - Data models
- `db/migrations/` - Database schema changes

### React Frontend (`/web`)

```bash
# Development
cd web
npm install                    # Install dependencies
npm run dev                    # Start dev server
npm run build                  # Production build

# Testing
npm test                       # Run tests
npm run lint                   # ESLint

# Type checking
npm run type-check
```

## 🐳 Docker Development

### Building Images

The production image is a single unified "dark container" built from the
root `Dockerfile`. The Go backend serves the React bundle as static assets
out of `/app/public`, so there is no separate frontend container in prod.

```bash
# Build the unified production image (tags: ghcr.io/patel5d2/ubuntu-auto-update)
./scripts/build.sh --docker

# Or build it directly
docker build -t ghcr.io/patel5d2/ubuntu-auto-update:latest .

# Build the standalone agent image (used by docker-compose.dev.yml)
docker build -t ghcr.io/patel5d2/ubuntu-auto-update/agent:latest ./agent

# Dev-only images (hot-reload variants used by docker-compose.dev.yml)
docker build -f backend/Dockerfile.dev -t uau-backend:dev ./backend
docker build -f web/Dockerfile.dev     -t uau-frontend:dev ./web
```

### Development Environment

```bash
# Start core services (Postgres + Go backend with hot-reload)
docker compose -f docker-compose.dev.yml up -d postgres backend

# Start full stack (postgres + backend + Vite frontend)
docker compose -f docker-compose.dev.yml up -d

# View logs
docker compose -f docker-compose.dev.yml logs -f backend

# Start with monitoring (adds Prometheus + Grafana)
docker compose -f docker-compose.dev.yml --profile monitoring up -d
```

### Testing in Containers

```bash
# Test agent in container (non-privileged: enroll/status only)
docker compose -f docker-compose.dev.yml --profile agent run --rm agent status

# Test with actual system access (privileged: runs apt on host)
docker compose -f docker-compose.dev.yml --profile system-agent run --rm agent-system run --dry-run
```

## 🧪 Testing Strategy

### Automated Testing

```bash
# Run all tests
./scripts/test.sh

# Unit tests only
./scripts/test.sh --unit-only

# With coverage reports
./scripts/test.sh --coverage

# Include Docker tests
./scripts/test.sh --docker

# Security-focused testing
./scripts/test.sh --security-only
```

### Manual Testing Workflows

#### Agent Testing

```bash
# 1. Build agent
cd agent && cargo build --release

# 2. Test configuration
./target/release/ua-agent generate-config --output /tmp/test-config.toml

# 3. Test connectivity (with backend running)
./target/release/ua-agent --config /tmp/test-config.toml test

# 4. Test enrollment
./target/release/ua-agent enroll "test-token-123"

# 5. Test dry run
./target/release/ua-agent run --dry-run

# 6. Check metrics
./target/release/ua-agent metrics
```

#### Backend Testing

```bash
# 1. Start backend
cd backend && go run cmd/api/main.go

# 2. Test health endpoint
curl http://localhost:8080/api/v1/health

# 3. Test authentication
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"password"}'

# 4. Test agent enrollment
curl -X POST http://localhost:8080/api/v1/enroll \
  -H "Content-Type: application/json" \
  -d '{"enrollment_token":"dev-enrollment-token-12345","hostname":"test-host"}'

# 5. Check metrics
curl http://localhost:9090/metrics
```

#### Integration Testing

```bash
# 1. Start full environment
docker compose -f docker-compose.dev.yml up -d

# 2. Wait for services
sleep 30

# 3. Test agent enrollment and run
docker compose -f docker-compose.dev.yml --profile agent run --rm agent enroll "dev-enrollment-token"
docker compose -f docker-compose.dev.yml --profile agent run --rm agent run --dry-run

# 4. Check web UI (Vite dev server)
open http://localhost:5173

# 5. Check metrics
open http://localhost:3001  # Grafana (if monitoring profile enabled)
```

## 🔒 Security Testing

### Static Analysis

```bash
# Rust security audit
cd agent && cargo audit

# Go security scan
cd backend && gosec ./...

# Dependency scanning
./scripts/test.sh --security-only
```

### Container Security

```bash
# Scan the unified image for CVEs
docker scout cves ghcr.io/patel5d2/ubuntu-auto-update:latest

# Inspect image layers for accidentally-committed secrets
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  wagoodman/dive:latest ghcr.io/patel5d2/ubuntu-auto-update:latest
```

## 🚀 Production Builds

### Release Builds

```bash
# Build optimized release versions
./scripts/build.sh --release --cross-compile

# Output will be in ./dist/
ls -la dist/
```

### Multi-Architecture Support

```bash
# Cross-compile for different architectures
./scripts/build.sh --release --cross-compile

# Available targets:
# - Linux x86_64 (Intel/AMD)
# - Linux ARM64 (Raspberry Pi 4, ARM servers)
# - macOS x86_64 (Intel Mac)
# - macOS ARM64 (M1/M2 Mac)
```

## 📊 Monitoring and Observability

### Metrics Collection

The system provides comprehensive metrics at multiple levels:

1. **Agent Metrics** (Prometheus format):
   ```bash
   # View agent metrics
   ua-agent metrics
   
   # Metrics are also written to textfile collector
   cat /var/lib/node_exporter/textfile_collector/ubuntu-auto-update.prom
   ```

2. **Backend Metrics**:
   ```bash
   # API metrics
   curl http://localhost:9090/metrics
   ```

3. **System Metrics**:
   ```bash
   # Start with monitoring profile
   docker compose -f docker-compose.dev.yml --profile monitoring up -d
   
   # Access Grafana
   open http://localhost:3001
   # Login: admin/admin
   ```

### Log Analysis

```bash
# Agent logs
tail -f /var/log/ubuntu-auto-update/agent.log

# Backend logs (Docker)
docker compose -f docker-compose.dev.yml logs -f backend

# System logs
journalctl -u ubuntu-auto-update-agent.service -f
```

## 🔧 Troubleshooting

### Common Issues

#### Build Issues

```bash
# Rust build fails
cd agent
cargo clean && cargo build

# Go build fails
cd backend
go mod download && go build ./cmd/api

# Node build fails
cd web
rm -rf node_modules package-lock.json
npm install
```

#### Runtime Issues

```bash
# Agent can't connect to backend
ua-agent test  # Test connectivity

# Backend database issues
docker compose -f docker-compose.dev.yml logs postgres

# Permission issues (Linux)
sudo chown -R $USER:$USER /etc/ubuntu-auto-update
```

#### Container Issues

```bash
# Docker build fails
docker system prune -a  # Clean up

# Container won't start
docker compose -f docker-compose.dev.yml down -v
docker compose -f docker-compose.dev.yml up -d --force-recreate
```

### Debug Mode

```bash
# Enable debug logging
export RUST_LOG=debug
export LOG_LEVEL=debug
export UA_LOGGING__LEVEL=debug

# Run with verbose output
ua-agent -vv run --dry-run
```

## 📝 Contributing

### Code Style

- **Rust**: Follow `rustfmt` and `clippy` recommendations
- **Go**: Use `gofmt` and follow Go conventions  
- **TypeScript**: Follow project ESLint configuration
- **Shell**: Use `shellcheck` for script validation

### Commit Guidelines

```bash
# Commit message format
git commit -m "component: description

Longer explanation if needed.

Closes #issue-number"

# Examples
git commit -m "agent: add mTLS support for backend communication"
git commit -m "backend: implement RBAC authorization middleware"
git commit -m "web: add real-time update status notifications"
```

### Pull Request Process

1. Create feature branch: `git checkout -b feature/description`
2. Make changes and add tests
3. Run full test suite: `./scripts/test.sh --coverage`
4. Build and verify: `./scripts/build.sh --release`
5. Submit PR with clear description

## 🎯 Performance Tuning

### Agent Performance

```bash
# Profile memory usage
RUST_LOG=debug cargo run --release -- run --dry-run

# Optimize binary size
cargo build --release
strip target/release/ua-agent
```

### Backend Performance

```bash
# Profile with pprof
go run cmd/api/main.go &
go tool pprof http://localhost:8080/debug/pprof/profile

# Load testing
hey -n 1000 -c 10 http://localhost:8080/api/v1/health
```

### Database Tuning

```bash
# Connection pool tuning
export DATABASE_MAX_OPEN_CONNS=25
export DATABASE_MAX_IDLE_CONNS=10

# Query performance
EXPLAIN ANALYZE SELECT * FROM hosts WHERE last_seen > NOW() - INTERVAL '1 hour';
```

## 🌍 Deployment

### Self-Hosted Deployment

The production stack is one Postgres container plus the unified app
container (built from the root `Dockerfile`). `docker-compose.yml` wires
them together.

```bash
# Easiest: generate .env with random secrets and bring the stack up
./quickstart.sh up

# Or do it manually
cp .env.example .env             # then fill in ADMIN_*/ENROLLMENT_TOKEN
docker compose up -d             # uses docker-compose.yml + root Dockerfile

# Pull the pre-built image instead of building locally
docker pull ghcr.io/patel5d2/ubuntu-auto-update:latest
```

The image is published by `.github/workflows/docker-image.yml` for both
`linux/amd64` and `linux/arm64` on every push to `main`.

To install the Rust agent on a managed Ubuntu host:

```bash
# Build a release binary, then install the systemd unit + timer
./scripts/build.sh --release --agent-only
sudo ./install.sh --backend-url https://your-server.com \
                  --enrollment-token "$ENROLLMENT_TOKEN"
```

This development guide covers the essentials for building, testing, and
deploying the Ubuntu Auto-Update system. For the user-facing quick start,
see [`README.md`](README.md).