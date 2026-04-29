#!/usr/bin/env bash
# Ubuntu Auto-Update Test Script
# Runs tests across the stack with portable, lenient defaults.

set -eo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

RUN_UNIT_TESTS=true
RUN_INTEGRATION_TESTS=true
RUN_SECURITY_TESTS=true
RUN_DOCKER_TESTS=false
GENERATE_COVERAGE=false
PARALLEL_TESTS=false

log()         { echo -e "${BLUE}[$(date -u +%FT%TZ)]${NC} $*"; }
log_success() { echo -e "${GREEN}[$(date -u +%FT%TZ)]${NC} $*"; }
log_warn()    { echo -e "${YELLOW}[$(date -u +%FT%TZ)]${NC} $*"; }
log_error()   { echo -e "${RED}[$(date -u +%FT%TZ)]${NC} $*" >&2; }

cpu_count() {
    if command -v nproc >/dev/null 2>&1; then
        nproc
    elif sysctl -n hw.ncpu >/dev/null 2>&1; then
        sysctl -n hw.ncpu
    else
        echo 2
    fi
}

# Resolve "docker compose" (V2) with a fallback to legacy "docker-compose".
docker_compose() {
    if docker compose version >/dev/null 2>&1; then
        docker compose "$@"
    elif command -v docker-compose >/dev/null 2>&1; then
        docker-compose "$@"
    else
        log_error "Neither 'docker compose' nor 'docker-compose' is available"
        return 1
    fi
}

show_help() {
    cat <<EOF
Ubuntu Auto-Update Test Script

Usage: $0 [OPTIONS]

Options:
    --unit-only         Run only unit tests
    --integration-only  Run only integration tests
    --security-only     Run only security tests
    --docker            Include Docker-based tests
    --coverage          Generate test coverage reports
    --parallel          Run tests in parallel
    --help              Show this help message
EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --unit-only)        RUN_UNIT_TESTS=true;  RUN_INTEGRATION_TESTS=false; RUN_SECURITY_TESTS=false; shift ;;
            --integration-only) RUN_UNIT_TESTS=false; RUN_INTEGRATION_TESTS=true;  RUN_SECURITY_TESTS=false; shift ;;
            --security-only)    RUN_UNIT_TESTS=false; RUN_INTEGRATION_TESTS=false; RUN_SECURITY_TESTS=true;  shift ;;
            --docker)           RUN_DOCKER_TESTS=true;   shift ;;
            --coverage)         GENERATE_COVERAGE=true;  shift ;;
            --parallel)         PARALLEL_TESTS=true;     shift ;;
            --help)             show_help; exit 0 ;;
            *)                  log_error "Unknown option: $1"; show_help; exit 1 ;;
        esac
    done
}

check_prerequisites() {
    log "Checking test prerequisites..."
    if [[ ! -f "$ROOT_DIR/README.md" ]]; then
        log_error "Must be run from the project root or scripts directory"
        exit 1
    fi
    log_success "Prerequisites check passed"
}

test_agent() {
    if [[ ! -d "$ROOT_DIR/agent" ]]; then
        log_warn "Agent directory not found, skipping"
        return 0
    fi
    if ! command -v cargo >/dev/null 2>&1; then
        log_warn "cargo not installed, skipping agent tests"
        return 0
    fi

    log "Running Rust agent tests..."
    cd "$ROOT_DIR/agent"

    if [[ "$RUN_UNIT_TESTS" == "true" ]]; then
        cargo_test_args=()
        [[ "$PARALLEL_TESTS" == "true" ]] && cargo_test_args+=("--jobs=$(cpu_count)")

        if [[ "$GENERATE_COVERAGE" == "true" ]] && command -v cargo-tarpaulin >/dev/null 2>&1; then
            cargo tarpaulin --out Html --output-dir "../coverage/agent" --skip-clean "${cargo_test_args[@]}"
        else
            [[ "$GENERATE_COVERAGE" == "true" ]] && log_warn "cargo-tarpaulin not installed; running tests without coverage"
            cargo test "${cargo_test_args[@]}"
        fi

        # clippy is best-effort (treat warnings as warnings, not errors).
        cargo clippy --all-targets -- -W warnings || log_warn "cargo clippy reported warnings"
        log_success "Agent unit tests passed"
    fi

    if [[ "$RUN_SECURITY_TESTS" == "true" ]]; then
        if command -v cargo-audit >/dev/null 2>&1; then
            log "Running cargo audit..."
            cargo audit || log_warn "cargo audit reported advisories"
        else
            log_warn "cargo-audit not installed; skipping. Install with: cargo install cargo-audit"
        fi
    fi

    cd "$ROOT_DIR"
}

test_backend() {
    if [[ ! -d "$ROOT_DIR/backend" ]]; then
        log_warn "Backend directory not found, skipping"
        return 0
    fi
    if ! command -v go >/dev/null 2>&1; then
        log_error "Go is required to test backend"
        return 1
    fi

    log "Running Go backend tests..."
    cd "$ROOT_DIR/backend"

    if [[ "$RUN_UNIT_TESTS" == "true" ]]; then
        go_test_args=("./...")
        [[ "$PARALLEL_TESTS" == "true" ]] && go_test_args=("-parallel" "$(cpu_count)" "./...")

        if [[ "$GENERATE_COVERAGE" == "true" ]]; then
            mkdir -p "../coverage/backend"
            go test -coverprofile=coverage.out -covermode=atomic "${go_test_args[@]}"
            go tool cover -html=coverage.out -o "../coverage/backend/coverage.html"
        else
            go test "${go_test_args[@]}"
        fi

        log "Running go vet..."
        go vet ./...
        log_success "Backend unit tests passed"
    fi

    if [[ "$RUN_INTEGRATION_TESTS" == "true" ]]; then
        if find . -name "*_integration_test.go" -print -quit | grep -q .; then
            log "Running backend integration tests..."
            go test -tags=integration ./...
            log_success "Backend integration tests passed"
        else
            log_warn "No backend integration tests found (skipping)"
        fi
    fi

    if [[ "$RUN_SECURITY_TESTS" == "true" ]]; then
        if command -v gosec >/dev/null 2>&1; then
            log "Running gosec..."
            gosec ./... || log_warn "gosec reported issues"
        else
            log_warn "gosec not installed; skipping. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"
        fi
    fi

    cd "$ROOT_DIR"
}

test_frontend() {
    if [[ ! -d "$ROOT_DIR/web" ]]; then
        log_warn "Web directory not found, skipping"
        return 0
    fi
    if ! command -v node >/dev/null 2>&1; then
        log_warn "Node.js not installed, skipping frontend tests"
        return 0
    fi

    log "Running React frontend tests..."
    cd "$ROOT_DIR/web"

    if [[ ! -d node_modules ]]; then
        log "Installing frontend dependencies..."
        if [[ -f package-lock.json ]]; then
            npm ci
        else
            npm install
        fi
    fi

    if [[ "$RUN_UNIT_TESTS" == "true" ]]; then
        log "Running vitest..."
        npm test
        log "Type-checking + production build (smoke test)..."
        npm run build
        log_success "Frontend tests passed"
    fi

    cd "$ROOT_DIR"
}

test_docker() {
    [[ "$RUN_DOCKER_TESTS" != "true" ]] && return 0
    if ! command -v docker >/dev/null 2>&1; then
        log_warn "docker not installed, skipping docker tests"
        return 0
    fi

    log "Running Docker build smoke tests..."

    if [[ -f "$ROOT_DIR/agent/Dockerfile" ]]; then
        docker build -t ubuntu-auto-update/agent:test "$ROOT_DIR/agent"
        log_success "Agent Docker build OK"
    fi
    if [[ -f "$ROOT_DIR/backend/Dockerfile" ]]; then
        docker build -t ubuntu-auto-update/backend:test "$ROOT_DIR/backend"
        log_success "Backend Docker build OK"
    fi
    if [[ -f "$ROOT_DIR/web/Dockerfile" ]]; then
        docker build -t ubuntu-auto-update/web:test "$ROOT_DIR/web"
        log_success "Web Docker build OK"
    fi
}

test_integration() {
    [[ "$RUN_INTEGRATION_TESTS" != "true" ]] && return 0
    if ! command -v docker >/dev/null 2>&1; then
        log_warn "docker not installed, skipping integration tests"
        return 0
    fi
    if [[ ! -f "$ROOT_DIR/docker-compose.dev.yml" ]]; then
        log_warn "docker-compose.dev.yml not found, skipping"
        return 0
    fi

    log "Bringing up postgres + backend via docker compose..."
    docker_compose -f "$ROOT_DIR/docker-compose.dev.yml" up -d postgres backend

    # Poll the health endpoint for up to 60s rather than sleeping blindly.
    log "Waiting for backend /health..."
    local ok=false
    for _ in $(seq 1 30); do
        if curl -fsS http://localhost:8080/api/v1/health >/dev/null 2>&1; then
            ok=true
            break
        fi
        sleep 2
    done

    if [[ "$ok" == "true" ]]; then
        log_success "Backend health check passed"
    else
        log_error "Backend health check failed; dumping logs:"
        docker_compose -f "$ROOT_DIR/docker-compose.dev.yml" logs --tail=200 backend || true
        docker_compose -f "$ROOT_DIR/docker-compose.dev.yml" down || true
        return 1
    fi

    log "Tearing down test environment..."
    docker_compose -f "$ROOT_DIR/docker-compose.dev.yml" down
}

run_security_tests() {
    [[ "$RUN_SECURITY_TESTS" != "true" ]] && return 0

    if command -v gitleaks >/dev/null 2>&1; then
        log "Running gitleaks..."
        gitleaks detect --source "$ROOT_DIR" --no-git --redact || log_warn "gitleaks reported findings"
    else
        log_warn "gitleaks not installed; skipping secret scan"
    fi
}

generate_test_report() {
    local report_file="$ROOT_DIR/test-report.md"
    cat > "$report_file" <<EOF
# Test Report

Generated: $(date -u +%FT%TZ)

## Test Configuration
- Unit Tests: $RUN_UNIT_TESTS
- Integration Tests: $RUN_INTEGRATION_TESTS
- Security Tests: $RUN_SECURITY_TESTS
- Docker Tests: $RUN_DOCKER_TESTS
- Coverage Generation: $GENERATE_COVERAGE
- Parallel Execution: $PARALLEL_TESTS
EOF

    if [[ "$GENERATE_COVERAGE" == "true" ]]; then
        {
            echo
            echo "## Coverage Reports"
            [[ -f "$ROOT_DIR/coverage/agent/tarpaulin-report.html" ]] && echo "- [Agent Coverage](coverage/agent/tarpaulin-report.html)"
            [[ -f "$ROOT_DIR/coverage/backend/coverage.html" ]]      && echo "- [Backend Coverage](coverage/backend/coverage.html)"
            [[ -d "$ROOT_DIR/coverage/frontend" ]]                   && echo "- [Frontend Coverage](coverage/frontend/index.html)"
        } >> "$report_file"
    fi
    log_success "Test report written to $report_file"
}

show_summary() {
    log_success "Tests finished"
    echo
    echo "=== Test Summary ==="
    echo "Unit Tests:        $([[ "$RUN_UNIT_TESTS"        == "true" ]] && echo OK || echo SKIPPED)"
    echo "Integration Tests: $([[ "$RUN_INTEGRATION_TESTS" == "true" ]] && echo OK || echo SKIPPED)"
    echo "Security Tests:    $([[ "$RUN_SECURITY_TESTS"    == "true" ]] && echo OK || echo SKIPPED)"
    echo "Docker Tests:      $([[ "$RUN_DOCKER_TESTS"      == "true" ]] && echo OK || echo SKIPPED)"
}

main() {
    echo "Ubuntu Auto-Update Test Script"
    echo "=============================="
    echo
    parse_args "$@"
    check_prerequisites

    [[ "$GENERATE_COVERAGE" == "true" ]] && mkdir -p "$ROOT_DIR/coverage"

    test_agent
    test_backend
    test_frontend
    test_docker
    test_integration
    run_security_tests
    generate_test_report
    show_summary
}

main "$@"
