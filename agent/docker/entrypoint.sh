#!/bin/bash
set -euo pipefail

# Ubuntu Auto-Update Agent Docker Entrypoint
# Handles configuration templating and command execution

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[$(date -Iseconds)]${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}[$(date -Iseconds)]${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}[$(date -Iseconds)]${NC} $*" >&2
}

log_error() {
    echo -e "${RED}[$(date -Iseconds)]${NC} $*" >&2
}

# Function to substitute environment variables in config
substitute_env_vars() {
    local file="$1"
    local temp_file=$(mktemp)
    
    # Use envsubst to replace environment variables
    envsubst < "$file" > "$temp_file"
    mv "$temp_file" "$file"
}

# Initialize configuration
init_config() {
    local config_file="/etc/ubuntu-auto-update/agent.toml"
    local example_file="/etc/ubuntu-auto-update/agent.toml.example"
    
    if [[ ! -f "$config_file" ]]; then
        if [[ -f "$example_file" ]]; then
            log "Creating configuration from template..."
            cp "$example_file" "$config_file"
            substitute_env_vars "$config_file"
            log_success "Configuration created at $config_file"
        else
            log "Generating default configuration..."
            ua-agent generate-config --output "$config_file"
            substitute_env_vars "$config_file"
            log_success "Default configuration generated"
        fi
    fi
    
    # Ensure proper permissions (if we're root)
    if [[ $(id -u) -eq 0 ]]; then
        chmod 644 "$config_file"
    fi
}

# Check if backend is reachable
check_backend() {
    if [[ -n "${BACKEND_URL:-}" ]]; then
        log "Checking backend connectivity to $BACKEND_URL..."
        
        # Wait for backend to be available (with timeout)
        local max_attempts=30
        local attempt=1
        
        while [[ $attempt -le $max_attempts ]]; do
            if ua-agent test >/dev/null 2>&1; then
                log_success "Backend is reachable"
                return 0
            fi
            
            log "Backend not ready (attempt $attempt/$max_attempts)..."
            sleep 2
            ((attempt++))
        done
        
        log_warn "Backend connectivity check failed after $max_attempts attempts"
        return 1
    fi
    
    return 0
}

# Auto-enroll if token is provided
auto_enroll() {
    if [[ -n "${ENROLLMENT_TOKEN:-}" ]]; then
        if [[ ! -f "/etc/ubuntu-auto-update/auth.token" ]]; then
            log "Auto-enrolling with backend using provided token..."
            
            if ua-agent enroll "$ENROLLMENT_TOKEN"; then
                log_success "Auto-enrollment successful"
            else
                log_error "Auto-enrollment failed"
                return 1
            fi
        else
            log "Already enrolled (auth token exists)"
        fi
    fi
}

# Show helpful information
show_info() {
    log "Ubuntu Auto-Update Agent Container"
    log "=================================="
    log "Version: $(ua-agent --version 2>/dev/null || echo 'unknown')"
    log "Backend URL: ${BACKEND_URL:-not set}"
    log "Enrollment Token: ${ENROLLMENT_TOKEN:+set}"
    log "Command: $*"
    log ""
}

# Handle special commands
handle_command() {
    case "$1" in
        "daemon"|"schedule")
            log_error "Daemon mode not supported in containers"
            log "Use a CronJob or external scheduler to run 'ua-agent run'"
            exit 1
            ;;
        "run")
            # For actual updates, we need root privileges
            if [[ $(id -u) -ne 0 ]]; then
                log_warn "Running updates requires root privileges"
                log "Add '--user root' to docker run command for actual updates"
            fi
            ;;
        "help"|"--help"|"-h")
            ua-agent --help
            exit 0
            ;;
    esac
}

# Main execution
main() {
    show_info "$@"
    
    # Initialize configuration
    init_config
    
    # Handle special commands
    if [[ $# -gt 0 ]]; then
        handle_command "$1"
    fi
    
    # Check backend connectivity (non-blocking)
    check_backend || log_warn "Proceeding without backend connectivity"
    
    # Auto-enroll if configured
    auto_enroll || log_warn "Proceeding without enrollment"
    
    # Execute the command
    if [[ $# -eq 0 ]]; then
        log "No command provided, showing status..."
        exec ua-agent status
    else
        log "Executing: ua-agent $*"
        exec ua-agent "$@"
    fi
}

# Run main function with all arguments
main "$@"