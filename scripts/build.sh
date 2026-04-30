#!/usr/bin/env bash
# Ubuntu Auto-Update Build Script
# Builds all components with proper error handling and optimization.

set -eo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
DIST_DIR="$ROOT_DIR/dist"

BUILD_AGENT=true
BUILD_BACKEND=true
BUILD_FRONTEND=true
BUILD_MOBILE=false
RELEASE_MODE=false
DOCKER_BUILD=false
CROSS_COMPILE=false
SKIP_TESTS=false

log()         { echo -e "${BLUE}[$(date -u +%FT%TZ)]${NC} $*"; }
log_success() { echo -e "${GREEN}[$(date -u +%FT%TZ)]${NC} $*"; }
log_warn()    { echo -e "${YELLOW}[$(date -u +%FT%TZ)]${NC} $*"; }
log_error()   { echo -e "${RED}[$(date -u +%FT%TZ)]${NC} $*" >&2; }

# Portable CPU count (nproc on Linux, sysctl on macOS, falls back to 2).
cpu_count() {
    if command -v nproc >/dev/null 2>&1; then
        nproc
    elif sysctl -n hw.ncpu >/dev/null 2>&1; then
        sysctl -n hw.ncpu
    else
        echo 2
    fi
}

show_help() {
    cat <<EOF
Ubuntu Auto-Update Build Script

Usage: $0 [OPTIONS]

Options:
    --release           Build in release mode with optimizations
    --docker            Build Docker images
    --cross-compile     Cross-compile for multiple architectures
    --skip-tests        Skip running tests during build
    --agent-only        Build only the Rust agent
    --backend-only      Build only the Go backend
    --frontend-only     Build only the React frontend
    --mobile            Include mobile app build
    --help              Show this help message
EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --release)        RELEASE_MODE=true;   shift ;;
            --docker)         DOCKER_BUILD=true;   shift ;;
            --cross-compile)  CROSS_COMPILE=true;  shift ;;
            --skip-tests)     SKIP_TESTS=true;     shift ;;
            --agent-only)     BUILD_AGENT=true;  BUILD_BACKEND=false; BUILD_FRONTEND=false; BUILD_MOBILE=false; shift ;;
            --backend-only)   BUILD_AGENT=false; BUILD_BACKEND=true;  BUILD_FRONTEND=false; BUILD_MOBILE=false; shift ;;
            --frontend-only)  BUILD_AGENT=false; BUILD_BACKEND=false; BUILD_FRONTEND=true;  BUILD_MOBILE=false; shift ;;
            --mobile)         BUILD_MOBILE=true;   shift ;;
            --help)           show_help; exit 0 ;;
            *)                log_error "Unknown option: $1"; show_help; exit 1 ;;
        esac
    done
}

check_prerequisites() {
    log "Checking prerequisites..."

    if [[ "$DOCKER_BUILD" == "true" ]] && ! command -v docker >/dev/null 2>&1; then
        log_error "Docker is required but not installed"
        exit 1
    fi

    if [[ "$BUILD_AGENT" == "true" ]]; then
        if ! command -v cargo >/dev/null 2>&1; then
            log_warn "Rust (cargo) not found — skipping agent build. Install from https://rustup.rs/"
            BUILD_AGENT=false
        else
            log "Rust version: $(rustc --version)"
        fi
    fi

    if [[ "$BUILD_BACKEND" == "true" ]]; then
        if ! command -v go >/dev/null 2>&1; then
            log_error "Go is required but not installed"
            exit 1
        fi
        log "Go version: $(go version)"
    fi

    if [[ "$BUILD_FRONTEND" == "true" ]]; then
        if ! command -v node >/dev/null 2>&1; then
            log_error "Node.js is required but not installed"
            exit 1
        fi
        log "Node.js version: $(node --version)"
        log "npm version: $(npm --version)"
    fi

    log_success "Prerequisites check passed"
}

create_build_dirs() {
    log "Creating build directories..."
    mkdir -p "$DIST_DIR/agent" "$DIST_DIR/backend" "$DIST_DIR/frontend"
    [[ "$BUILD_MOBILE" == "true" ]] && mkdir -p "$DIST_DIR/mobile"
    log_success "Build directories created"
}

build_agent() {
    [[ "$BUILD_AGENT" != "true" ]] && return 0

    log "Building Rust agent..."
    cd "$ROOT_DIR/agent"

    if [[ "$SKIP_TESTS" != "true" ]]; then
        log "Running agent tests..."
        cargo test
        # clippy is best-effort: warnings on the agent's older deps shouldn't fail the build.
        cargo clippy --all-targets -- -W warnings || log_warn "cargo clippy reported warnings"
        log_success "Agent tests passed"
    fi

    cargo_args=()
    [[ "$RELEASE_MODE" == "true" ]] && cargo_args+=(--release)
    profile_dir="debug"
    [[ "$RELEASE_MODE" == "true" ]] && profile_dir="release"

    if [[ "$CROSS_COMPILE" == "true" ]]; then
        for target in x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu; do
            log "Building for target: $target"
            if ! cargo build "${cargo_args[@]}" --target="$target" 2>&1; then
                log_warn "Cross-build for $target skipped (toolchain likely missing)"
                continue
            fi
            local binary_path="target/$target/$profile_dir/ua-agent"
            if [[ -f "$binary_path" ]]; then
                cp "$binary_path" "$DIST_DIR/agent/ua-agent-$target"
                log_success "Agent built for $target"
            fi
        done
    else
        cargo build "${cargo_args[@]}"
        local binary_path="target/$profile_dir/ua-agent"
        if [[ -f "$binary_path" ]]; then
            cp "$binary_path" "$DIST_DIR/agent/"
            log_success "Agent built successfully"
        else
            log_error "Expected binary at $binary_path was not produced"
            exit 1
        fi
    fi

    [[ -f install.sh ]] && cp install.sh "$DIST_DIR/agent/"
    [[ -d systemd ]] && cp -r systemd "$DIST_DIR/agent/"

    cd "$ROOT_DIR"
}

build_backend() {
    [[ "$BUILD_BACKEND" != "true" ]] && return 0

    log "Building Go backend..."
    cd "$ROOT_DIR/backend"

    if [[ "$SKIP_TESTS" != "true" ]]; then
        log "Running backend tests..."
        go test ./...
        go vet ./...
        log_success "Backend tests passed"
    fi

    # Build flags as a positional array we always pass — empty array OK with set -e (no -u).
    build_flags=()
    if [[ "$RELEASE_MODE" == "true" ]]; then
        build_flags+=("-ldflags=-w -s")
        build_flags+=("-trimpath")
    fi

    if [[ "$CROSS_COMPILE" == "true" ]]; then
        for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
            local os="${target%/*}" arch="${target#*/}"
            log "Building for $os/$arch"
            GOOS="$os" GOARCH="$arch" go build "${build_flags[@]}" \
                -o "$DIST_DIR/backend/ubuntu-auto-update-backend-${os}-${arch}" \
                ./cmd/api
            log_success "Backend built for $os/$arch"
        done
    else
        go build "${build_flags[@]}" \
            -o "$DIST_DIR/backend/ubuntu-auto-update-backend" \
            ./cmd/api
        log_success "Backend built successfully"
    fi

    cp -r db "$DIST_DIR/backend/"
    cd "$ROOT_DIR"
}

build_frontend() {
    [[ "$BUILD_FRONTEND" != "true" ]] && return 0

    log "Building React frontend..."
    cd "$ROOT_DIR/web"

    log "Installing frontend dependencies..."
    if [[ -f package-lock.json ]]; then
        npm ci
    else
        npm install
    fi

    if [[ "$SKIP_TESTS" != "true" ]]; then
        log "Running frontend tests (vitest)..."
        npm test
        log_success "Frontend tests passed"
    fi

    log "Building frontend..."
    npm run build

    if [[ -d "dist" ]]; then
        # cp -r dist/* fails if dist contains hidden files; use a glob-safe copy.
        ( cd dist && tar cf - . ) | ( cd "$DIST_DIR/frontend" && tar xf - )
    elif [[ -d "build" ]]; then
        ( cd build && tar cf - . ) | ( cd "$DIST_DIR/frontend" && tar xf - )
    else
        log_warn "No frontend build output directory found"
    fi

    log_success "Frontend built successfully"
    cd "$ROOT_DIR"
}

build_mobile() {
    [[ "$BUILD_MOBILE" != "true" ]] && return 0

    log "Building mobile app..."
    if [[ ! -d "$ROOT_DIR/mobile" ]]; then
        log_warn "Mobile directory not found, skipping mobile build"
        return 0
    fi
    cd "$ROOT_DIR/mobile"
    [[ -f package-lock.json ]] && npm ci || npm install
    npm run build
    log_success "Mobile app built successfully"
    cd "$ROOT_DIR"
}

build_docker_images() {
    [[ "$DOCKER_BUILD" != "true" ]] && return 0
    log "Building Docker images..."

    local datestamp
    datestamp="$(date +%Y%m%d)"

    # The project uses a single "dark container" (root Dockerfile) that bakes
    # the React frontend + Go backend into one image.  Per-component images
    # (agent/backend/frontend) are not published; only this unified image is.
    if [[ -f "$ROOT_DIR/Dockerfile" ]]; then
        log "Building dark container image (unified backend + frontend)..."
        docker build \
            -t "ghcr.io/patel5d2/ubuntu-auto-update:latest" \
            -t "ghcr.io/patel5d2/ubuntu-auto-update:$datestamp" \
            "$ROOT_DIR"
        log_success "Dark container image built"
    else
        log_error "Root Dockerfile not found at $ROOT_DIR/Dockerfile"
        exit 1
    fi

    # The Rust agent has its own standalone image (used by docker-compose.dev.yml).
    if [[ "$BUILD_AGENT" == "true" && -f "$ROOT_DIR/agent/Dockerfile" ]]; then
        log "Building agent Docker image..."
        docker build \
            -t "ghcr.io/patel5d2/ubuntu-auto-update/agent:latest" \
            -t "ghcr.io/patel5d2/ubuntu-auto-update/agent:$datestamp" \
            "$ROOT_DIR/agent"
        log_success "Agent Docker image built"
    fi
}

create_checksums() {
    log "Creating checksums..."
    cd "$DIST_DIR"
    local sha
    if command -v sha256sum >/dev/null 2>&1; then
        sha="sha256sum"
    elif command -v shasum >/dev/null 2>&1; then
        sha="shasum -a 256"
    else
        log_warn "No sha256 tool found; skipping checksums"
        cd "$ROOT_DIR"
        return 0
    fi
    find . -type f \( -name "ua-agent*" -o -name "ubuntu-auto-update-backend*" \) -exec $sha {} \; > checksums.txt
    log_success "Checksums created"
    cd "$ROOT_DIR"
}

show_summary() {
    log_success "Build completed successfully!"
    echo
    echo "=== Build Summary ==="
    echo "Mode: $([[ "$RELEASE_MODE" == "true" ]] && echo "Release" || echo "Debug")"
    echo "Cross-compile: $CROSS_COMPILE"
    echo "Docker: $DOCKER_BUILD"
    echo
    echo "Components built:"
    [[ "$BUILD_AGENT"    == "true" ]] && echo "- Rust Agent"
    [[ "$BUILD_BACKEND"  == "true" ]] && echo "- Go Backend"
    [[ "$BUILD_FRONTEND" == "true" ]] && echo "- React Frontend"
    [[ "$BUILD_MOBILE"   == "true" ]] && echo "- Mobile App"
    echo
    echo "Output directory: $DIST_DIR"
    if [[ -f "$DIST_DIR/checksums.txt" ]]; then
        echo
        echo "Checksums:"
        cat "$DIST_DIR/checksums.txt"
    fi
}

main() {
    echo "Ubuntu Auto-Update Build Script"
    echo "==============================="
    echo
    parse_args "$@"
    check_prerequisites
    create_build_dirs
    build_agent
    build_backend
    build_frontend
    build_mobile
    build_docker_images
    [[ "$RELEASE_MODE" == "true" ]] && create_checksums
    show_summary
}

main "$@"
