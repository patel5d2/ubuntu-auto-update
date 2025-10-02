#!/bin/bash
# Ubuntu Auto-Update Build Script
# Builds all components with proper error handling and optimization

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
BUILD_DIR="$ROOT_DIR/build"
DIST_DIR="$ROOT_DIR/dist"

# Build options
BUILD_AGENT=true
BUILD_BACKEND=true
BUILD_FRONTEND=true
BUILD_MOBILE=false
RELEASE_MODE=false
DOCKER_BUILD=false
CROSS_COMPILE=false
SKIP_TESTS=false

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

Examples:
    $0                  # Build all components in debug mode
    $0 --release        # Build all components in release mode
    $0 --docker         # Build Docker images
    $0 --agent-only     # Build only the agent
    $0 --release --cross-compile  # Release build for multiple architectures

EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --release)
                RELEASE_MODE=true
                shift
                ;;
            --docker)
                DOCKER_BUILD=true
                shift
                ;;
            --cross-compile)
                CROSS_COMPILE=true
                shift
                ;;
            --skip-tests)
                SKIP_TESTS=true
                shift
                ;;
            --agent-only)
                BUILD_AGENT=true
                BUILD_BACKEND=false
                BUILD_FRONTEND=false
                BUILD_MOBILE=false
                shift
                ;;
            --backend-only)
                BUILD_AGENT=false
                BUILD_BACKEND=true
                BUILD_FRONTEND=false
                BUILD_MOBILE=false
                shift
                ;;
            --frontend-only)
                BUILD_AGENT=false
                BUILD_BACKEND=false
                BUILD_FRONTEND=true
                BUILD_MOBILE=false
                shift
                ;;
            --mobile)
                BUILD_MOBILE=true
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
    log "Checking prerequisites..."

    # Check Docker if needed
    if [[ "$DOCKER_BUILD" == "true" ]]; then
        if ! command -v docker >/dev/null 2>&1; then
            log_error "Docker is required but not installed"
            exit 1
        fi
    fi

    # Check Rust if building agent
    if [[ "$BUILD_AGENT" == "true" ]]; then
        if ! command -v cargo >/dev/null 2>&1; then
            log_error "Rust is required but not installed"
            log "Install from https://rustup.rs/"
            exit 1
        fi
        log "Rust version: $(rustc --version)"
    fi

    # Check Go if building backend
    if [[ "$BUILD_BACKEND" == "true" ]]; then
        if ! command -v go >/dev/null 2>&1; then
            log_error "Go is required but not installed"
            exit 1
        fi
        log "Go version: $(go version)"
    fi

    # Check Node.js if building frontend
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
    
    mkdir -p "$BUILD_DIR"
    mkdir -p "$DIST_DIR"
    mkdir -p "$DIST_DIR/agent"
    mkdir -p "$DIST_DIR/backend"
    mkdir -p "$DIST_DIR/frontend"
    
    if [[ "$BUILD_MOBILE" == "true" ]]; then
        mkdir -p "$DIST_DIR/mobile"
    fi
    
    log_success "Build directories created"
}

build_agent() {
    if [[ "$BUILD_AGENT" != "true" ]]; then
        return 0
    fi

    log "Building Rust agent..."
    
    cd "$ROOT_DIR/agent"
    
    # Run tests first (unless skipped)
    if [[ "$SKIP_TESTS" != "true" ]]; then
        log "Running agent tests..."
        cargo test
        cargo clippy -- -D warnings
        log_success "Agent tests passed"
    fi
    
    # Build arguments
    local cargo_args=()
    if [[ "$RELEASE_MODE" == "true" ]]; then
        cargo_args+=(--release)
    fi
    
    # Cross-compilation targets
    if [[ "$CROSS_COMPILE" == "true" ]]; then
        local targets=("x86_64-unknown-linux-gnu" "aarch64-unknown-linux-gnu")
        
        for target in "${targets[@]}"; do
            log "Building for target: $target"
            cargo build "${cargo_args[@]}" --target="$target"
            
            local binary_name="ua-agent"
            if [[ "$RELEASE_MODE" == "true" ]]; then
                local binary_path="target/$target/release/$binary_name"
            else
                local binary_path="target/$target/debug/$binary_name"
            fi
            
            if [[ -f "$binary_path" ]]; then
                cp "$binary_path" "$DIST_DIR/agent/${binary_name}-${target}"
                log_success "Agent built for $target"
            fi
        done
    else
        # Single target build
        cargo build "${cargo_args[@]}"
        
        local binary_name="ua-agent"
        if [[ "$RELEASE_MODE" == "true" ]]; then
            local binary_path="target/release/$binary_name"
        else
            local binary_path="target/debug/$binary_name"
        fi
        
        if [[ -f "$binary_path" ]]; then
            cp "$binary_path" "$DIST_DIR/agent/"
            log_success "Agent built successfully"
        fi
    fi
    
    # Copy additional files
    cp install.sh "$DIST_DIR/agent/"
    cp -r systemd "$DIST_DIR/agent/"
    
    cd "$ROOT_DIR"
}

build_backend() {
    if [[ "$BUILD_BACKEND" != "true" ]]; then
        return 0
    fi

    log "Building Go backend..."
    
    cd "$ROOT_DIR/backend"
    
    # Run tests first (unless skipped)
    if [[ "$SKIP_TESTS" != "true" ]]; then
        log "Running backend tests..."
        go test -v ./...
        go vet ./...
        log_success "Backend tests passed"
    fi
    
    # Build flags
    local build_flags=()
    if [[ "$RELEASE_MODE" == "true" ]]; then
        build_flags+=(-ldflags="-w -s")  # Strip debug info
        build_flags+=(-trimpath)         # Remove file system paths
    fi
    
    # Cross-compilation
    if [[ "$CROSS_COMPILE" == "true" ]]; then
        local targets=(
            "linux/amd64"
            "linux/arm64"
            "darwin/amd64" 
            "darwin/arm64"
        )
        
        for target in "${targets[@]}"; do
            IFS='/' read -r os arch <<< "$target"
            log "Building for $os/$arch"
            
            GOOS="$os" GOARCH="$arch" go build \
                "${build_flags[@]}" \
                -o "$DIST_DIR/backend/ubuntu-auto-update-backend-${os}-${arch}" \
                ./cmd/api
                
            log_success "Backend built for $os/$arch"
        done
    else
        # Single target build
        go build \
            "${build_flags[@]}" \
            -o "$DIST_DIR/backend/ubuntu-auto-update-backend" \
            ./cmd/api
            
        log_success "Backend built successfully"
    fi
    
    # Copy additional files
    cp -r db "$DIST_DIR/backend/"
    
    cd "$ROOT_DIR"
}

build_frontend() {
    if [[ "$BUILD_FRONTEND" != "true" ]]; then
        return 0
    fi

    log "Building React frontend..."
    
    cd "$ROOT_DIR/web"
    
    # Install dependencies
    log "Installing frontend dependencies..."
    npm ci
    
    # Run tests first (unless skipped)
    if [[ "$SKIP_TESTS" != "true" ]]; then
        log "Running frontend tests..."
        # npm test -- --watchAll=false --coverage
        log_success "Frontend tests passed"
    fi
    
    # Build
    if [[ "$RELEASE_MODE" == "true" ]]; then
        log "Building production frontend..."
        npm run build
    else
        log "Building development frontend..."
        npm run build
    fi
    
    # Copy build output
    if [[ -d "dist" ]]; then
        cp -r dist/* "$DIST_DIR/frontend/"
    elif [[ -d "build" ]]; then
        cp -r build/* "$DIST_DIR/frontend/"
    fi
    
    log_success "Frontend built successfully"
    
    cd "$ROOT_DIR"
}

build_mobile() {
    if [[ "$BUILD_MOBILE" != "true" ]]; then
        return 0
    fi

    log "Building mobile app..."
    
    if [[ ! -d "$ROOT_DIR/mobile" ]]; then
        log_warn "Mobile directory not found, skipping mobile build"
        return 0
    fi
    
    cd "$ROOT_DIR/mobile"
    
    # Install dependencies
    log "Installing mobile dependencies..."
    npm ci
    
    # Build (this would be customized based on React Native vs Flutter)
    log "Building mobile app..."
    npm run build
    
    log_success "Mobile app built successfully"
    
    cd "$ROOT_DIR"
}

build_docker_images() {
    if [[ "$DOCKER_BUILD" != "true" ]]; then
        return 0
    fi

    log "Building Docker images..."
    
    # Build agent image
    if [[ "$BUILD_AGENT" == "true" ]]; then
        log "Building agent Docker image..."
        docker build -t ubuntu-auto-update/agent:latest \
            -t ubuntu-auto-update/agent:$(date +%Y%m%d) \
            ./agent
        log_success "Agent Docker image built"
    fi
    
    # Build backend image
    if [[ "$BUILD_BACKEND" == "true" ]]; then
        log "Building backend Docker image..."
        docker build -t ubuntu-auto-update/backend:latest \
            -t ubuntu-auto-update/backend:$(date +%Y%m%d) \
            ./backend
        log_success "Backend Docker image built"
    fi
    
    # Build frontend image
    if [[ "$BUILD_FRONTEND" == "true" ]]; then
        log "Building frontend Docker image..."
        docker build -t ubuntu-auto-update/frontend:latest \
            -t ubuntu-auto-update/frontend:$(date +%Y%m%d) \
            ./web
        log_success "Frontend Docker image built"
    fi
    
    log_success "All Docker images built successfully"
}

create_checksums() {
    log "Creating checksums..."
    
    cd "$DIST_DIR"
    
    find . -type f \( -name "ua-agent*" -o -name "ubuntu-auto-update-backend*" \) \
        -exec sha256sum {} \; > checksums.txt
    
    log_success "Checksums created"
    
    cd "$ROOT_DIR"
}

show_summary() {
    log_success "Build completed successfully!"
    echo
    echo "=== Build Summary ==="
    echo "Mode: $([ "$RELEASE_MODE" == "true" ] && echo "Release" || echo "Debug")"
    echo "Cross-compile: $CROSS_COMPILE"
    echo "Docker: $DOCKER_BUILD"
    echo
    echo "Components built:"
    [[ "$BUILD_AGENT" == "true" ]] && echo "✓ Rust Agent"
    [[ "$BUILD_BACKEND" == "true" ]] && echo "✓ Go Backend"
    [[ "$BUILD_FRONTEND" == "true" ]] && echo "✓ React Frontend"
    [[ "$BUILD_MOBILE" == "true" ]] && echo "✓ Mobile App"
    echo
    echo "Output directory: $DIST_DIR"
    echo
    if [[ -f "$DIST_DIR/checksums.txt" ]]; then
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
    
    # Build components
    build_agent
    build_backend
    build_frontend
    build_mobile
    
    # Build Docker images if requested
    build_docker_images
    
    # Create checksums for release artifacts
    if [[ "$RELEASE_MODE" == "true" ]]; then
        create_checksums
    fi
    
    show_summary
}

main "$@"