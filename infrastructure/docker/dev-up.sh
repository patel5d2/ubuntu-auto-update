#!/usr/bin/env bash
# Development environment starter — brings up the full stack via docker-compose.
#
# Usage:
#   ./infrastructure/docker/dev-up.sh              # core stack only
#   ./infrastructure/docker/dev-up.sh --monitoring  # + Prometheus + Grafana
#   ./infrastructure/docker/dev-up.sh --all         # everything including agent
#   ./infrastructure/docker/dev-up.sh --down        # tear down

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/docker-compose.dev.yml"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${BLUE}[dev-up]${NC} $1"; }
ok()  { echo -e "${GREEN}[dev-up]${NC} $1"; }
warn(){ echo -e "${YELLOW}[dev-up]${NC} $1"; }

PROFILES=""

case "${1:-}" in
    --monitoring)
        PROFILES="--profile monitoring"
        log "Including monitoring stack (Prometheus + Grafana)"
        ;;
    --agent)
        PROFILES="--profile agent"
        log "Including non-privileged agent container"
        ;;
    --all)
        PROFILES="--profile agent --profile monitoring --profile proxy"
        log "Bringing up full stack (agent + monitoring + proxy)"
        ;;
    --down)
        log "Tearing down all containers..."
        docker compose -f "$COMPOSE_FILE" --profile agent --profile monitoring --profile proxy --profile system-agent down -v
        ok "All containers stopped and volumes removed."
        exit 0
        ;;
    --help|-h)
        echo "Usage: $0 [--monitoring|--agent|--all|--down|--help]"
        echo ""
        echo "  (no args)      Core stack: postgres + backend + frontend"
        echo "  --monitoring   Add Prometheus + Grafana"
        echo "  --agent        Add Rust agent (non-privileged)"
        echo "  --all          Everything: agent + monitoring + proxy"
        echo "  --down         Tear down all containers + volumes"
        exit 0
        ;;
    "")
        log "Starting core stack (postgres + backend + frontend)"
        ;;
    *)
        warn "Unknown option: $1"
        exit 1
        ;;
esac

# Create .env from example if it doesn't exist.
if [[ ! -f "$ROOT_DIR/.env" && -f "$ROOT_DIR/.env.example" ]]; then
    cp "$ROOT_DIR/.env.example" "$ROOT_DIR/.env"
    warn "Created .env from .env.example — review and update secrets."
fi

# Generate self-signed SSL certs for the proxy profile if missing.
SSL_DIR="$ROOT_DIR/infrastructure/nginx/ssl"
if [[ ! -f "$SSL_DIR/cert.pem" ]]; then
    log "Generating self-signed SSL certificate for dev..."
    mkdir -p "$SSL_DIR"
    openssl req -x509 -nodes -days 365 \
        -newkey rsa:2048 \
        -keyout "$SSL_DIR/key.pem" \
        -out "$SSL_DIR/cert.pem" \
        -subj "/CN=localhost" 2>/dev/null
    ok "Self-signed cert created at $SSL_DIR/"
fi

log "Building and starting containers..."
docker compose -f "$COMPOSE_FILE" $PROFILES up --build -d

echo ""
ok "Development stack is running!"
echo ""
echo "  Endpoints:"
echo "    Frontend:    http://localhost:5173"
echo "    Backend API: http://localhost:8080"
echo "    Metrics:     http://localhost:8080/metrics"

if echo "$PROFILES" | grep -q "monitoring"; then
    echo "    Prometheus:  http://localhost:9091"
    echo "    Grafana:     http://localhost:3001  (admin / admin)"
fi

if echo "$PROFILES" | grep -q "proxy"; then
    echo "    Nginx:       https://localhost  (self-signed cert)"
fi

echo ""
echo "  Useful commands:"
echo "    Logs:        docker compose -f docker-compose.dev.yml logs -f"
echo "    Stop:        ./infrastructure/docker/dev-up.sh --down"
echo ""
