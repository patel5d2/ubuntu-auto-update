#!/usr/bin/env bash
# test-container.sh — smoke-test the production compose stack (docker-compose.yml).
#
# The stack is two services: `postgres` and `app` (the single "dark container"
# that serves both the React UI and the Go API on :8080). Run it after
# ./quickstart.sh to confirm everything actually works.

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$REPO_ROOT"

if docker compose version >/dev/null 2>&1; then
    COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE="docker-compose"
else
    echo "❌ Neither 'docker compose' nor 'docker-compose' found." >&2
    exit 1
fi

FAILURES=0
check() { # check <label> <command...>
    local label=$1; shift
    if "$@" >/dev/null 2>&1; then
        echo "  ✅ $label"
    else
        echo "  ❌ $label"
        FAILURES=$((FAILURES + 1))
    fi
}

echo "📦 Container status:"
$COMPOSE ps

echo
echo "🔍 Testing services..."
check "API health   (http://localhost:8080/api/v1/health)" \
    curl -sf http://localhost:8080/api/v1/health
check "Web UI       (http://localhost:8080/)" \
    curl -sf -o /dev/null http://localhost:8080/
check "PostgreSQL   (pg_isready inside the postgres container)" \
    $COMPOSE exec -T postgres pg_isready

if [ "$FAILURES" -gt 0 ]; then
    echo
    echo "📝 Recent app logs (troubleshooting):"
    $COMPOSE logs --tail=20 app 2>/dev/null | sed 's/^/  /'
    echo
    echo "❌ $FAILURES check(s) failed. Common fixes:"
    echo "  • Stack not up yet?         ./quickstart.sh   (first boot needs ~10s)"
    echo "  • Missing .env?             ./quickstart.sh generates it"
    echo "  • Migrations failed?        $COMPOSE logs app | grep migrate"
    exit 1
fi

echo
echo "✨ All checks passed."
echo "  • Web UI + API: http://localhost:8080  (login with credentials from .env)"
echo "  • Tail logs:    ./quickstart.sh logs"
echo "  • Tear down:    ./quickstart.sh down"
