# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Overview

Ubuntu Auto-Update is evolving from a traditional bash-based update system into an **enterprise-grade, memory-safe, multi-component system** using Rust for agents, Go for backend services, and modern web/mobile frontends. The system provides centralized management of Ubuntu server updates across fleets with real-time monitoring, audit trails, and enterprise features.

## Current Architecture

The repository currently contains:
- **Backend API (Go)**: REST API with PostgreSQL, WebSocket support for real-time updates
- **Agent (Rust)**: Lightweight client for performing updates and reporting status
- **Web Dashboard (React/TypeScript)**: Management interface with Vite build system  
- **Legacy Shell Scripts**: Traditional update scripts with systemd integration
- **Container Infrastructure**: Docker Compose setup with PostgreSQL and Redis

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Go 1.22+
- Rust (latest stable)
- Node.js 20+
- PostgreSQL (for local development)

### Development Environment Setup

```bash
# Start infrastructure services
docker-compose up -d postgres redis

# Backend (Go)
cd backend
go mod download
go run cmd/api/main.go

# Agent (Rust) - build and test
cd agent
cargo build --release
# Test with dry run
sudo ./target/release/ua-agent --backend-url http://localhost:8080 run

# Web Frontend
cd web
npm install
npm run dev
# Access at http://localhost:3000

# Mobile/PWA (Future)
cd mobile  # Will contain React Native app
npm install && npm run start
```

## Component Guide

### Backend (Go) - `/backend/`

**Primary Entry Points:**
- `cmd/api/main.go` - Main API server with routing and middleware
- `cmd/api/main_test.go` - Integration tests with test database

**Key Commands:**
```bash
# Run API server
go run cmd/api/main.go

# Run all tests (requires test database)
go test ./... -v

# Build for production
go build -o bin/backend ./cmd/api

# Database migrations
migrate -path db/migrations -database "postgres://..." up
```

**Architecture:**
- REST API with Gorilla Mux routing
- PostgreSQL with pgx driver
- WebSocket support for real-time updates
- JWT/cookie-based authentication
- Multi-tenant data isolation

**Environment Variables:**
```bash
DATABASE_URL="postgres://user:password@localhost:5432/uau_db?sslmode=disable"
REDIS_URL="redis://localhost:6379"
ADMIN_USERNAME=admin
ADMIN_PASSWORD=password
ENROLLMENT_TOKEN="your-secret-enrollment-token"
API_PORT=8080
```

### Agent (Rust) - `/agent/`

**Primary Entry Points:**
- `src/main.rs` - CLI interface and main execution logic
- `Cargo.toml` - Dependencies and project configuration

**Key Commands:**
```bash
# Build agent
cargo build --release

# Run update check (requires sudo for actual updates)
sudo ./target/release/ua-agent --backend-url http://localhost:8080 run

# Enroll agent with backend
./target/release/ua-agent --backend-url http://localhost:8080 enroll TOKEN

# Run tests
cargo test

# Security analysis
cargo clippy
cargo audit
```

**Agent Capabilities:**
- Performs `apt update` and `apt --dry-run upgrade` 
- Reports system status to backend via REST API
- Stores authentication token locally
- Designed for memory safety and minimal resource usage

### Web Dashboard - `/web/`

**Primary Entry Points:**
- `src/App.tsx` - Main application component with routing
- `src/pages/` - Page components (HostList, HostDetail, LoginPage)
- `vite.config.ts` - Build configuration

**Key Commands:**
```bash
# Development server
npm run dev

# Production build  
npm run build

# Type checking
npm run build  # includes tsc

# Preview production build
npm run preview
```

**Architecture:**
- React 18 with TypeScript
- Vite for fast development and building
- React Router for navigation
- Protected routes with authentication
- Real-time updates via WebSocket connections

## Database Management

### Migrations
```bash
# Apply all migrations
migrate -path backend/db/migrations -database $DATABASE_URL up

# Rollback one migration
migrate -path backend/db/migrations -database $DATABASE_URL down 1

# Create new migration
migrate create -ext sql -dir backend/db/migrations -seq description_of_change
```

### Schema Overview
Current tables include:
- `hosts` - Registered systems with update status
- `ssh_keys` - SSH credentials for remote operations
- `webhooks` - Notification endpoints
- And migration history tracking

## Docker & Containerization

### Development
```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f backend

# Rebuild specific service
docker-compose up -d --build backend
```

### Production Images
Each component has its own Dockerfile:
- `backend/Dockerfile` - Go API server with migrations
- `web/Dockerfile` - Nginx serving React SPA
- `sshd/Dockerfile` - SSH daemon for remote operations
- `dashboard/Dockerfile` - Python Flask dashboard (legacy)

## Testing Strategy

### Backend Testing
```bash
# Unit tests
cd backend && go test ./...

# Integration tests (requires test database)
go test ./cmd/api -v

# Test specific handler
go test ./cmd/api -run TestHandleLogin
```

### Agent Testing
```bash
# Unit tests
cd agent && cargo test

# Security scanning
cargo clippy --all-targets --all-features
cargo audit

# Memory safety (future)
cargo test --features sanitizer
```

### Web Testing
```bash
cd web
# Currently using Vite's built-in testing
npm run build  # Validates TypeScript compilation
```

## Enterprise Roadmap & Implementation Priorities

### Phase 1: Core Infrastructure (Current → Month 1)
- [x] Go Backend with PostgreSQL
- [x] Rust Agent with basic functionality  
- [x] React Dashboard with authentication
- [ ] **Enhanced Security**: mTLS/HMAC for agent communication
- [ ] **Observability**: Prometheus metrics, structured logging
- [ ] **Hardening**: AppArmor profiles, seccomp filters

### Phase 2: Enterprise Features (Month 1-2)
- [ ] **Multi-tenancy**: Schema isolation and RBAC
- [ ] **License Management**: Signed JWT tokens, feature gating
- [ ] **SSO Integration**: OIDC/SAML support
- [ ] **Mobile/PWA**: React Native app with push notifications
- [ ] **Advanced UI**: Time-series dashboards, audit logs

### Phase 3: Production Grade (Month 2-3)
- [ ] **High Availability**: Redis clustering, DB replication
- [ ] **Container Orchestration**: Helm charts for Kubernetes
- [ ] **Security Compliance**: SOC2 readiness, penetration testing
- [ ] **SaaS Platform**: Stripe integration, managed hosting
- [ ] **Comprehensive Testing**: Fuzzing, load testing, E2E automation

## Security Considerations

### Current Security Measures
- PostgreSQL with connection encryption
- Cookie-based web authentication
- Agent authentication via Bearer tokens
- Docker container isolation

### Planned Security Enhancements
- **Agent Hardening**: Static binaries with cosign signatures
- **Communication Security**: mTLS between agent and backend
- **System Hardening**: systemd security features, AppArmor profiles
- **Audit Logging**: Immutable logs for compliance
- **Secret Management**: HashiCorp Vault integration for enterprise

## Configuration Management

### Backend Configuration
Environment variables and config files in `backend/config/`:
- Database connections
- Authentication settings  
- Feature flags for paid vs free features
- Integration endpoints (Stripe, notification services)

### Agent Configuration  
Configuration in `/etc/ubuntu-auto-update/`:
- Backend URL and authentication
- Update scheduling and policies
- Local logging and metrics

### Web Configuration
Build-time configuration in `web/`:
- API endpoints
- Feature flags
- Authentication providers

## Deployment Options

### Self-Hosted (Current Focus)
```bash
# Traditional installation
sudo ./install.sh

# Container deployment  
docker-compose -f docker-compose.prod.yml up -d

# Kubernetes (planned)
helm install ubuntu-auto-update ./charts/ubuntu-auto-update
```

### SaaS (Planned)
- Managed Kubernetes on AWS/GCP
- Multi-tenant PostgreSQL with RDS
- CDN for global dashboard access
- Enterprise SSO integration

## Troubleshooting

### Common Issues

**Agent enrollment fails:**
```bash
# Check backend connectivity
curl -k https://your-backend/api/v1/hosts

# Verify token storage
sudo cat /etc/ubuntu-auto-update/auth.token

# Check systemd status
sudo systemctl status ubuntu-auto-update.service
```

**Backend database connection:**
```bash
# Test database connection
psql $DATABASE_URL -c "SELECT 1"

# Check migrations status
migrate -path backend/db/migrations -database $DATABASE_URL version
```

**Web dashboard not loading:**
```bash
# Check API connectivity
curl http://localhost:8080/api/v1/hosts

# Verify authentication
# Check browser developer tools for CORS/auth errors
```

### Logging Locations
- **Agent**: `/var/log/ubuntu-auto-update/update.log`
- **Backend**: `journalctl -u ubuntu-auto-update-backend` (systemd) or Docker logs
- **Web**: Browser developer console
- **Database**: PostgreSQL logs via Docker or system logs

## Development Workflow

### Code Organization
```
/backend/           # Go API server and business logic
  /cmd/api/         # Main application entry point
  /pkg/             # Reusable packages (models, auth, etc.)
  /db/migrations/   # Database schema changes
/agent/             # Rust system agent
  /src/             # Source code
  /systemd/         # Service definitions
/web/               # React TypeScript dashboard
  /src/pages/       # Page components
  /src/services/    # API client code
/mobile/            # Future: React Native mobile app
/infrastructure/    # Docker, Kubernetes, Terraform
/scripts/           # Development and deployment helpers
```

### Git Workflow
- Feature branches for all changes
- Pull requests with automated testing
- Main branch protected with required reviews
- Semantic versioning for releases

### Local Development
1. Use `docker-compose` for external dependencies (PostgreSQL, Redis)
2. Run each service locally for faster iteration
3. Use environment variables for service discovery
4. Test integration with agent running against local backend

This documentation evolves with the codebase. For the latest enterprise features and deployment options, see the project roadmap and GitHub issues.