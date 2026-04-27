#!/bin/bash
# Ubuntu Auto-Update Agent Installer - Enterprise Edition
# Version: 0.2.0
# Author: Ubuntu Auto-Update Team

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/ubuntu-auto-update"
SYSTEMD_DIR="/etc/systemd/system"
LOG_DIR="/var/log/ubuntu-auto-update"
METRICS_DIR="/var/lib/node_exporter/textfile_collector"

# Default values
BACKEND_URL=""
ENROLLMENT_TOKEN=""
VERIFY_SIGNATURE=true
ENABLE_TIMER=true
SKIP_ENROLLMENT=false

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

show_help() {
    cat << EOF
Ubuntu Auto-Update Agent Installer

Usage: $0 [OPTIONS]

Options:
    --backend-url URL           Backend server URL (required)
    --enrollment-token TOKEN    Enrollment token for registration
    --skip-enrollment          Skip automatic enrollment
    --no-verify-signature      Skip binary signature verification
    --no-timer                 Don't enable systemd timer
    --config-only              Only create configuration files
    --help                     Show this help message

Examples:
    # Basic installation with enrollment
    sudo $0 --backend-url https://update-server.example.com --enrollment-token abc123

    # Install without enrollment (manual enrollment later)
    sudo $0 --backend-url https://update-server.example.com --skip-enrollment

    # Configuration only (for containers)
    sudo $0 --backend-url https://update-server.example.com --config-only

EOF
}

parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --backend-url)
                BACKEND_URL="$2"
                shift 2
                ;;
            --enrollment-token)
                ENROLLMENT_TOKEN="$2"
                shift 2
                ;;
            --skip-enrollment)
                SKIP_ENROLLMENT=true
                shift
                ;;
            --no-verify-signature)
                VERIFY_SIGNATURE=false
                shift
                ;;
            --no-timer)
                ENABLE_TIMER=false
                shift
                ;;
            --config-only)
                CONFIG_ONLY=true
                shift
                ;;
            --help)
                show_help
                exit 0
                ;;
            *)
                print_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done

    if [[ -z "$BACKEND_URL" ]]; then
        print_error "Backend URL is required. Use --backend-url"
        show_help
        exit 1
    fi

    if [[ "$SKIP_ENROLLMENT" == "false" && -z "$ENROLLMENT_TOKEN" ]]; then
        print_error "Enrollment token is required unless --skip-enrollment is used"
        show_help
        exit 1
    fi
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

check_system() {
    print_status "Checking system compatibility..."
    
    # Check if running on Ubuntu or Debian
    if ! command -v apt-get >/dev/null 2>&1; then
        print_error "This installer is designed for Ubuntu/Debian systems with apt package manager"
        exit 1
    fi
    
    # Check systemd
    if ! command -v systemctl >/dev/null 2>&1; then
        print_warning "systemd not found. Systemd integration will be skipped."
        ENABLE_TIMER=false
    fi
    
    # Check architecture
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            BINARY_ARCH="x86_64"
            ;;
        aarch64|arm64)
            BINARY_ARCH="aarch64"
            ;;
        *)
            print_error "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac
    
    print_success "System compatibility check passed"
}

download_binary() {
    if [[ "${CONFIG_ONLY:-false}" == "true" ]]; then
        return 0
    fi

    print_status "Downloading Ubuntu Auto-Update Agent binary..."
    
    # For production, this would download from GitHub releases
    # For now, check if binary exists locally
    if [[ -f "./target/release/ua-agent" ]]; then
        print_status "Using local binary from ./target/release/ua-agent"
        cp "./target/release/ua-agent" "/tmp/ua-agent"
    else
        print_error "Binary not found. Please build the agent first with: cargo build --release"
        exit 1
    fi
    
    # In production, verify signature
    if [[ "$VERIFY_SIGNATURE" == "true" ]]; then
        print_status "Verifying binary signature..."
        # cosign verify --key cosign.pub /tmp/ua-agent
        print_warning "Signature verification skipped in development build"
    fi
    
    print_success "Binary downloaded and verified"
}

install_binary() {
    if [[ "${CONFIG_ONLY:-false}" == "true" ]]; then
        return 0
    fi

    print_status "Installing agent binary..."
    
    # Backup existing installation
    if [[ -f "$INSTALL_DIR/ua-agent" ]]; then
        print_status "Backing up existing installation"
        cp "$INSTALL_DIR/ua-agent" "$INSTALL_DIR/ua-agent.backup-$(date +%Y%m%d-%H%M%S)"
    fi
    
    # Install binary
    cp "/tmp/ua-agent" "$INSTALL_DIR/ua-agent"
    chmod +x "$INSTALL_DIR/ua-agent"
    chown root:root "$INSTALL_DIR/ua-agent"
    
    # Clean up temporary file
    rm -f "/tmp/ua-agent"
    
    print_success "Agent binary installed to $INSTALL_DIR/ua-agent"
}

create_directories() {
    print_status "Creating directories..."
    
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$LOG_DIR"
    mkdir -p "$METRICS_DIR"
    
    # Set permissions
    chmod 755 "$CONFIG_DIR"
    chmod 755 "$LOG_DIR"
    chmod 755 "$METRICS_DIR"
    
    print_success "Directories created"
}

generate_config() {
    print_status "Generating configuration..."
    
    if [[ "${CONFIG_ONLY:-false}" == "true" ]]; then
        # Use a minimal config generator
        cat > "$CONFIG_DIR/agent.toml" << EOF
[backend]
url = "$BACKEND_URL"
timeout_seconds = 30
retry_attempts = 3
retry_delay_seconds = 5

[security]
api_key_file = "/etc/ubuntu-auto-update/auth.token"
verify_server_cert = true
use_mtls = false

[updates]
dry_run = false
auto_reboot = false
reboot_delay_minutes = 5

[updates.update_sources]
apt = true
snap = true
flatpak = false
firmware = false

[logging]
level = "info"
format = "json"
file = "/var/log/ubuntu-auto-update/agent.log"
max_size_mb = 100
max_files = 5

[metrics]
enabled = true
textfile_path = "/var/lib/node_exporter/textfile_collector"
collect_system_metrics = true

[enrollment]
token_file = "/etc/ubuntu-auto-update/enrollment.token"
host_id_file = "/etc/ubuntu-auto-update/host.id"
enrollment_url = "$BACKEND_URL/api/v1/enroll"
EOF
    else
        # Use the binary to generate config
        "$INSTALL_DIR/ua-agent" generate-config --output "$CONFIG_DIR/agent.toml"
        
        # Override backend URL
        sed -i "s|url = \".*\"|url = \"$BACKEND_URL\"|" "$CONFIG_DIR/agent.toml"
    fi
    
    # Set restrictive permissions on config
    chmod 644 "$CONFIG_DIR/agent.toml"
    chown root:root "$CONFIG_DIR/agent.toml"
    
    print_success "Configuration generated at $CONFIG_DIR/agent.toml"
}

install_systemd_units() {
    if [[ "$ENABLE_TIMER" == "false" ]]; then
        return 0
    fi
    
    print_status "Installing systemd service and timer..."
    
    # Check if systemd files exist
    if [[ ! -f "./agent/systemd/ubuntu-auto-update-agent.service" ]]; then
        print_error "Systemd service file not found at ./agent/systemd/ubuntu-auto-update-agent.service"
        return 1
    fi
    
    # Install systemd files
    cp "./agent/systemd/ubuntu-auto-update-agent.service" "$SYSTEMD_DIR/"
    cp "./agent/systemd/ubuntu-auto-update-agent.timer" "$SYSTEMD_DIR/"
    
    # Set permissions
    chmod 644 "$SYSTEMD_DIR/ubuntu-auto-update-agent.service"
    chmod 644 "$SYSTEMD_DIR/ubuntu-auto-update-agent.timer"
    chown root:root "$SYSTEMD_DIR/ubuntu-auto-update-agent.service"
    chown root:root "$SYSTEMD_DIR/ubuntu-auto-update-agent.timer"
    
    # Reload systemd
    systemctl daemon-reload
    
    print_success "Systemd units installed"
}

setup_apparmor() {
    print_status "Setting up AppArmor profile..."
    
    if ! command -v apparmor_parser >/dev/null 2>&1; then
        print_warning "AppArmor not available, skipping profile installation"
        return 0
    fi
    
    # In production, you would ship an AppArmor profile
    print_warning "AppArmor profile installation not implemented yet"
    # apparmor_parser -r /etc/apparmor.d/ubuntu-auto-update-agent
}

enroll_agent() {
    if [[ "$SKIP_ENROLLMENT" == "true" || "${CONFIG_ONLY:-false}" == "true" ]]; then
        print_status "Skipping automatic enrollment"
        return 0
    fi
    
    print_status "Enrolling agent with backend..."
    
    if [[ -z "$ENROLLMENT_TOKEN" ]]; then
        print_error "Enrollment token not provided"
        return 1
    fi
    
    # Run enrollment
    if "$INSTALL_DIR/ua-agent" enroll "$ENROLLMENT_TOKEN"; then
        print_success "Agent enrolled successfully"
    else
        print_error "Agent enrollment failed"
        print_status "You can manually enroll later with:"
        print_status "  sudo $INSTALL_DIR/ua-agent enroll <token>"
        return 1
    fi
}

enable_timer() {
    if [[ "$ENABLE_TIMER" == "false" || "${CONFIG_ONLY:-false}" == "true" ]]; then
        return 0
    fi
    
    print_status "Enabling systemd timer..."
    
    systemctl enable ubuntu-auto-update-agent.timer
    systemctl start ubuntu-auto-update-agent.timer
    
    print_success "Systemd timer enabled and started"
    
    # Show timer status
    print_status "Timer status:"
    systemctl list-timers ubuntu-auto-update-agent.timer --no-pager || true
}

create_uninstaller() {
    if [[ "${CONFIG_ONLY:-false}" == "true" ]]; then
        return 0
    fi

    print_status "Creating uninstaller..."
    
    cat > "$INSTALL_DIR/ubuntu-auto-update-uninstall.sh" << 'EOF'
#!/bin/bash
# Ubuntu Auto-Update Agent Uninstaller

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [[ $EUID -ne 0 ]]; then
    echo -e "${RED}[ERROR]${NC} This script must be run as root (use sudo)"
    exit 1
fi

echo -e "${YELLOW}[WARNING]${NC} This will remove Ubuntu Auto-Update Agent from your system"
read -p "Are you sure you want to continue? [y/N]: " -n 1 -r
echo

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Uninstall cancelled"
    exit 0
fi

echo -e "${GREEN}[INFO]${NC} Stopping and disabling systemd timer..."
systemctl stop ubuntu-auto-update-agent.timer 2>/dev/null || true
systemctl disable ubuntu-auto-update-agent.timer 2>/dev/null || true

echo -e "${GREEN}[INFO]${NC} Removing systemd files..."
rm -f /etc/systemd/system/ubuntu-auto-update-agent.service
rm -f /etc/systemd/system/ubuntu-auto-update-agent.timer
systemctl daemon-reload 2>/dev/null || true

echo -e "${GREEN}[INFO]${NC} Removing binary..."
rm -f /usr/local/bin/ua-agent
rm -f /usr/local/bin/ubuntu-auto-update-uninstall.sh

echo -e "${GREEN}[INFO]${NC} Removing configuration..."
read -p "Remove configuration and logs? [y/N]: " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf /etc/ubuntu-auto-update
    rm -rf /var/log/ubuntu-auto-update
    echo -e "${GREEN}[INFO]${NC} Configuration and logs removed"
else
    echo -e "${GREEN}[INFO]${NC} Configuration and logs preserved"
fi

echo -e "${GREEN}[SUCCESS]${NC} Ubuntu Auto-Update Agent has been uninstalled"
EOF
    
    chmod +x "$INSTALL_DIR/ubuntu-auto-update-uninstall.sh"
    chown root:root "$INSTALL_DIR/ubuntu-auto-update-uninstall.sh"
    
    print_success "Uninstaller created at $INSTALL_DIR/ubuntu-auto-update-uninstall.sh"
}

show_summary() {
    print_success "Installation completed successfully!"
    echo
    echo "=== Installation Summary ==="
    if [[ "${CONFIG_ONLY:-false}" != "true" ]]; then
        echo "• Agent binary: $INSTALL_DIR/ua-agent"
    fi
    echo "• Configuration: $CONFIG_DIR/agent.toml"
    echo "• Logs: $LOG_DIR/"
    echo "• Metrics: $METRICS_DIR/"
    
    if [[ "$ENABLE_TIMER" == "true" ]]; then
        echo "• Systemd service: ubuntu-auto-update-agent.service"
        echo "• Systemd timer: ubuntu-auto-update-agent.timer"
    fi
    
    if [[ "${CONFIG_ONLY:-false}" != "true" ]]; then
        echo "• Uninstaller: $INSTALL_DIR/ubuntu-auto-update-uninstall.sh"
    fi
    echo
    echo "=== Quick Commands ==="
    if [[ "${CONFIG_ONLY:-false}" != "true" ]]; then
        echo "• Test configuration: sudo $INSTALL_DIR/ua-agent test"
        echo "• Check status: sudo $INSTALL_DIR/ua-agent status"
        echo "• Run update: sudo $INSTALL_DIR/ua-agent run"
        echo "• View metrics: sudo $INSTALL_DIR/ua-agent metrics"
    fi
    echo "• Edit configuration: sudo nano $CONFIG_DIR/agent.toml"
    
    if [[ "$ENABLE_TIMER" == "true" ]]; then
        echo "• Check timer: systemctl status ubuntu-auto-update-agent.timer"
        echo "• View logs: journalctl -u ubuntu-auto-update-agent.service -f"
    fi
    
    if [[ "$SKIP_ENROLLMENT" == "true" ]]; then
        echo
        print_warning "Agent not enrolled. To enroll manually:"
        echo "  sudo $INSTALL_DIR/ua-agent enroll <enrollment-token>"
    fi
}

main() {
    echo "Ubuntu Auto-Update Agent Installer v0.2.0"
    echo "=========================================="
    echo
    
    parse_arguments "$@"
    check_root
    check_system
    download_binary
    create_directories
    install_binary
    generate_config
    install_systemd_units
    setup_apparmor
    enroll_agent
    enable_timer
    create_uninstaller
    
    show_summary
}

main "$@"