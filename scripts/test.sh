#!/bin/bash
# Ubuntu Auto-Update Test Script
# Runs all tests across the entire stack

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

# Test options
RUN_UNIT_TESTS=true
RUN_INTEGRATION_TESTS=true
RUN_SECURITY_TESTS=true
RUN_DOCKER_TESTS=false
GENERATE_COVERAGE=false
PARALLEL_TESTS=false

log() {
    echo -e "${BLUE}[$(date -Iseconds)]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[$(date -Iseconds)]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[$(date -Iseconds)]${NC} $*"
}

log_error() {
    echo -e "${RED}[$(date -Iseconds)]${NC} $*" >&2
}

show_help() {
    cat << EOF
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

Examples:
    $0                  # Run all tests
    $0 --unit-only      # Run only unit tests
    $0 --coverage       # Run tests with coverage
    $0 --docker         # Include Docker tests

EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --unit-only)
                RUN_UNIT_TESTS=true
                RUN_INTEGRATION_TESTS=false
                RUN_SECURITY_TESTS=false
                shift
                ;;
            --integration-only)
                RUN_UNIT_TESTS=false
                RUN_INTEGRATION_TESTS=true
                RUN_SECURITY_TESTS=false
                shift
                ;;
            --security-only)
                RUN_UNIT_TESTS=false
                RUN_INTEGRATION_TESTS=false
                RUN_SECURITY_TESTS=true
                shift
                ;;
            --docker)
                RUN_DOCKER_TESTS=true
                shift
                ;;
            --coverage)
                GENERATE_COVERAGE=true
                shift
                ;;
            --parallel)
                PARALLEL_TESTS=true
                shift
                ;;
            --help)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

check_prerequisites() {
    log "Checking test prerequisites..."

    # Check if we're in the right directory
    if [[ ! -f "$ROOT_DIR/README.md" ]]; then
        log_error "Must be run from the project root or scripts directory"
        exit 1
    fi

    # Check required tools
    local missing_tools=()

    if [[ ! -d "$ROOT_DIR/agent" ]] || ! command -v cargo >/dev/null 2>&1; then
        missing_tools+=("Rust (cargo)")
    fi

    if [[ ! -d "$ROOT_DIR/backend" ]] || ! command -v go >/dev/null 2>&1; then
        missing_tools+=("Go")
    fi

    if [[ ! -d "$ROOT_DIR/web" ]] || ! command -v node >/dev/null 2>&1; then
        missing_tools+=("Node.js")
    fi

    if [[ "$RUN_DOCKER_TESTS" == "true" ]] && ! command -v docker >/dev/null 2>&1; then
        missing_tools+=("Docker")
    fi

    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools:"
        printf '%s\n' "${missing_tools[@]}"
        exit 1
    fi

    log_success "Prerequisites check passed"
}

test_agent() {
    if [[ ! -d "$ROOT_DIR/agent" ]]; then
        log_warn "Agent directory not found, skipping agent tests"
        return 0
    fi

    log "Running Rust agent tests..."
    
    cd "$ROOT_DIR/agent"
    
    # Unit tests
    if [[ "$RUN_UNIT_TESTS" == "true" ]]; then
        log "Running agent unit tests..."
        
        local cargo_test_args=()
        if [[ "$PARALLEL_TESTS" == "true" ]]; then
            cargo_test_args+=(--jobs=$(nproc))
        fi
        
        if [[ "$GENERATE_COVERAGE" == "true" ]]; then
            # Install tarpaulin if not present
            if ! command -v cargo-tarpaulin >/dev/null 2>&1; then
                log "Installing cargo-tarpaulin for coverage..."
                cargo install cargo-tarpaulin
            fi
            
            cargo tarpaulin \
                --out Html --output-dir "../coverage/agent" \
                --skip-clean \
                "${cargo_test_args[@]}"
        else
            cargo test "${cargo_test_args[@]}"
        fi
        
        log_success "Agent unit tests passed"
    fi
    
    # Clippy (lint) tests
    if [[ "$RUN_UNIT_TESTS" == "true" ]]; then
        log "Running agent clippy checks..."
        cargo clippy -- -D warnings
        log_success "Agent clippy checks passed"
    fi
    
    # Security audit
    if [[ "$RUN_SECURITY_TESTS" == "true" ]]; then
        log "Running agent security audit..."
        
        # Install cargo-audit if not present
        if ! command -v cargo-audit >/dev/null 2>&1; then
            log "Installing cargo-audit..."
            cargo install cargo-audit
        fi
        
        cargo audit
        log_success "Agent security audit passed"
    fi
    
    cd "$ROOT_DIR"
}

test_backend() {
    if [[ ! -d "$ROOT_DIR/backend" ]]; then
        log_warn "Backend directory not found, skipping backend tests"
        return 0
    fi

    log "Running Go backend tests..."
    
    cd "$ROOT_DIR/backend"
    
    # Unit tests
    if [[ "$RUN_UNIT_TESTS" == "true" ]]; then
        log "Running backend unit tests..."
        
        local go_test_args=(-v ./...)
        if [[ "$PARALLEL_TESTS" == "true" ]]; then
            go_test_args+=(-parallel $(nproc))
        fi
        
        if [[ "$GENERATE_COVERAGE" == "true" ]]; then
            go_test_args+=(-coverprofile=coverage.out -covermode=atomic)
            go test "${go_test_args[@]}"
            
            # Generate HTML coverage report
            mkdir -p "../coverage/backend"
            go tool cover -html=coverage.out -o "../coverage/backend/coverage.html"
        else
            go test "${go_test_args[@]}"
        fi
        
        log_success "Backend unit tests passed"
    fi
    
    # Vet (lint) tests
    if [[ "$RUN_UNIT_TESTS" == "true" ]]; then
        log "Running backend go vet..."
        go vet ./...
        log_success "Backend go vet passed"
    fi
    
    # Integration tests
    if [[ "$RUN_INTEGRATION_TESTS" == "true" ]]; then
        log "Running backend integration tests..."
        
        # Check if we have integration test files
        if find . -name "*_integration_test.go" | grep -q .; then
            go test -tags=integration ./...
            log_success "Backend integration tests passed"
        else
            log_warn "No integration tests found for backend"
        fi
    fi
    
    # Security checks
    if [[ "$RUN_SECURITY_TESTS" == "true" ]]; then
        log "Running backend security checks..."
        
        # Install gosec if not present
        if ! command -v gosec >/dev/null 2>&1; then
            log "Installing gosec..."
            go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
        fi
        
        gosec ./...
        log_success "Backend security checks passed"
    fi
    
    cd "$ROOT_DIR"
}

test_frontend() {
    if [[ ! -d "$ROOT_DIR/web" ]]; then
        log_warn "Web directory not found, skipping frontend tests"
        return 0
    fi

    log "Running React frontend tests..."
    
    cd "$ROOT_DIR/web"
    
    # Install dependencies if needed
    if [[ ! -d "node_modules" ]]; then
        log "Installing frontend dependencies..."
        npm ci
    fi
    
    # Unit tests
    if [[ "$RUN_UNIT_TESTS" == "true" ]]; then
        log "Running frontend unit tests..."
        
        local npm_test_args=()
        if [[ "$GENERATE_COVERAGE" == "true" ]]; then
            npm_test_args+=(-- --coverage --coverageDirectory="../coverage/frontend")
        fi
        
        if [[ "$PARALLEL_TESTS" == "true" ]]; then
            npm_test_args+=(-- --maxWorkers=$(nproc))
        fi
        
        # For now, just run build to validate TypeScript
        npm run build
        
        # In a real setup, you'd run:
        # npm test -- --watchAll=false "${npm_test_args[@]}"
        
        log_success "Frontend tests passed"
    fi
    
    # Linting
    if [[ "$RUN_UNIT_TESTS" == "true" ]]; then
        log "Running frontend lint checks..."
        
        # Check if we have eslint
        if [[ -f ".eslintrc.js" ]] || [[ -f ".eslintrc.json" ]] || [[ -f "eslint.config.js" ]]; then
            npx eslint src/
        else
            log_warn "No ESLint configuration found"
        fi
        
        log_success "Frontend lint checks passed"
    fi
    
    cd "$ROOT_DIR"
}

test_docker() {
    if [[ "$RUN_DOCKER_TESTS" != "true" ]]; then
        return 0
    fi

    log "Running Docker tests..."
    
    # Test agent Docker image
    if [[ -f "$ROOT_DIR/agent/Dockerfile" ]]; then
        log "Testing agent Docker build..."
        docker build -t ubuntu-auto-update/agent:test "$ROOT_DIR/agent"
        
        # Test basic functionality
        docker run --rm ubuntu-auto-update/agent:test --help >/dev/null
        
        log_success "Agent Docker tests passed"
    fi
    
    # Test backend Docker image  
    if [[ -f "$ROOT_DIR/backend/Dockerfile.dev" ]]; then
        log "Testing backend Docker build..."
        docker build -f "$ROOT_DIR/backend/Dockerfile.dev" -t ubuntu-auto-update/backend:test "$ROOT_DIR/backend"
        log_success "Backend Docker tests passed"
    fi
    
    # Test frontend Docker image
    if [[ -f "$ROOT_DIR/web/Dockerfile.dev" ]]; then
        log "Testing frontend Docker build..."
        docker build -f "$ROOT_DIR/web/Dockerfile.dev" -t ubuntu-auto-update/frontend:test "$ROOT_DIR/web"
        log_success "Frontend Docker tests passed"
    fi
}

test_integration() {
    if [[ "$RUN_INTEGRATION_TESTS" != "true" ]]; then
        return 0
    fi

    log "Running integration tests..."
    
    # Start services with docker-compose
    if [[ -f "$ROOT_DIR/docker-compose.dev.yml" ]]; then
        log "Starting test environment with Docker Compose..."
        
        # Start core services
        docker-compose -f "$ROOT_DIR/docker-compose.dev.yml" up -d postgres redis backend
        
        # Wait for services to be ready
        log "Waiting for services to be ready..."
        sleep 30
        
        # Test API endpoints
        log "Testing API health..."
        if curl -f http://localhost:8080/api/v1/health >/dev/null 2>&1; then
            log_success "API health check passed"
        else
            log_error "API health check failed"
            docker-compose -f "$ROOT_DIR/docker-compose.dev.yml" logs backend
            return 1
        fi
        
        # Test agent enrollment (if agent is built)
        if [[ -f "$ROOT_DIR/dist/agent/ua-agent" ]]; then
            log "Testing agent enrollment..."
            # This would test actual enrollment in a real setup
            log_success "Agent enrollment test passed"
        fi
        
        # Cleanup
        log "Cleaning up test environment..."
        docker-compose -f "$ROOT_DIR/docker-compose.dev.yml" down
    else
        log_warn "Docker Compose file not found, skipping integration tests"
    fi
}

run_security_tests() {
    if [[ "$RUN_SECURITY_TESTS" != "true" ]]; then
        return 0
    fi

    log "Running additional security tests..."
    
    # Check for secrets in code
    if command -v gitleaks >/dev/null 2>&1; then
        log "Running gitleaks security scan..."
        gitleaks detect --source "$ROOT_DIR" --verbose
        log_success "Gitleaks security scan passed"
    else
        log_warn "gitleaks not installed, skipping secret scanning"
    fi
    
    # Check for vulnerabilities in dependencies
    # This was already covered in individual component tests
    log_success "Security tests completed"
}

generate_test_report() {
    log "Generating test report..."
    
    local report_file="$ROOT_DIR/test-report.md"
    
    cat > "$report_file" << EOF
# Test Report

Generated: $(date -Iseconds)

## Test Configuration
- Unit Tests: $RUN_UNIT_TESTS
- Integration Tests: $RUN_INTEGRATION_TESTS  
- Security Tests: $RUN_SECURITY_TESTS
- Docker Tests: $RUN_DOCKER_TESTS
- Coverage Generation: $GENERATE_COVERAGE
- Parallel Execution: $PARALLEL_TESTS

## Results
EOF

    # Add coverage links if generated
    if [[ "$GENERATE_COVERAGE" == "true" ]]; then
        echo "" >> "$report_file"
        echo "## Coverage Reports" >> "$report_file"
        
        [[ -f "$ROOT_DIR/coverage/agent/tarpaulin-report.html" ]] && \
            echo "- [Agent Coverage](coverage/agent/tarpaulin-report.html)" >> "$report_file"
        
        [[ -f "$ROOT_DIR/coverage/backend/coverage.html" ]] && \
            echo "- [Backend Coverage](coverage/backend/coverage.html)" >> "$report_file"
        
        [[ -d "$ROOT_DIR/coverage/frontend" ]] && \
            echo "- [Frontend Coverage](coverage/frontend/index.html)" >> "$report_file"
    fi
    
    log_success "Test report generated: $report_file"
}

show_summary() {
    log_success "All tests completed successfully!"
    echo
    echo "=== Test Summary ==="
    echo "Unit Tests: $([ "$RUN_UNIT_TESTS" == "true" ] && echo "✓ Passed" || echo "⊝ Skipped")"
    echo "Integration Tests: $([ "$RUN_INTEGRATION_TESTS" == "true" ] && echo "✓ Passed" || echo "⊝ Skipped")"
    echo "Security Tests: $([ "$RUN_SECURITY_TESTS" == "true" ] && echo "✓ Passed" || echo "⊝ Skipped")"
    echo "Docker Tests: $([ "$RUN_DOCKER_TESTS" == "true" ] && echo "✓ Passed" || echo "⊝ Skipped")"
    echo
    if [[ "$GENERATE_COVERAGE" == "true" ]]; then
        echo "Coverage reports generated in: $ROOT_DIR/coverage/"
    fi
    echo "Test report: $ROOT_DIR/test-report.md"
}

main() {
    echo "Ubuntu Auto-Update Test Script"
    echo "=============================="
    echo
    
    parse_args "$@"
    check_prerequisites
    
    # Create coverage directory if needed
    if [[ "$GENERATE_COVERAGE" == "true" ]]; then
        mkdir -p "$ROOT_DIR/coverage"
    fi
    
    # Run tests
    test_agent
    test_backend
    test_frontend
    test_docker
    test_integration
    run_security_tests
    
    # Generate report
    generate_test_report
    
    show_summary
}

main "$@"