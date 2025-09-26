#!/bin/bash
# Ubuntu Auto-Update Installation Script
# Author: Dharmin Patel
# Version: 2.0.0

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
CONFIG_DIR="/etc/ubuntu-auto-update"
SYSTEMD_DIR="/etc/systemd/system"
LOG_DIR="/var/log/ubuntu-auto-update"

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
        print_error "This script is designed for Ubuntu/Debian systems with apt package manager"
        exit 1
    fi
    
    # Check systemd
    if ! command -v systemctl >/dev/null 2>&1; then
        print_warning "systemd not found. Systemd integration will be skipped."
        return 1
    fi
    
    return 0
}

validate_dirs() {
    print_status "Validating directories..."
    
    if [[ ! -d "$INSTALL_DIR" || ! -w "$INSTALL_DIR" ]]; then
        print_error "Installation directory is not valid or not writable: $INSTALL_DIR"
        exit 1
    fi
    
    if [[ ! -d "$CONFIG_DIR" || ! -w "$CONFIG_DIR" ]]; then
        print_error "Configuration directory is not valid or not writable: $CONFIG_DIR"
        exit 1
    fi
    
    if [[ ! -d "$SYSTEMD_DIR" || ! -w "$SYSTEMD_DIR" ]]; then
        print_error "Systemd directory is not valid or not writable: $SYSTEMD_DIR"
        exit 1
    fi
    
    if [[ ! -d "$LOG_DIR" || ! -w "$LOG_DIR" ]]; then
        print_error "Log directory is not valid or not writable: $LOG_DIR"
        exit 1
    fi
}

find_install_dir() {
    print_status "Finding suitable installation directory..."
    local path_dirs=$(echo $PATH | tr ":" "\n")
    for dir in $path_dirs; do
        if [[ -w "$dir" ]]; then
            INSTALL_DIR="$dir"
            print_success "Found writable directory in PATH: $INSTALL_DIR"
            return 0
        fi
    done

    print_warning "No writable directory found in PATH. Defaulting to /usr/local/bin"
    INSTALL_DIR="/usr/local/bin"
    mkdir -p "$INSTALL_DIR"
}

install_script() {
    print_status "Installing main update script..."
    
    if [[ ! -f "update.sh.tpl" ]]; then
        print_error "update.sh.tpl not found in current directory"
        exit 1
    fi
    
    # Backup existing installation
    backup_existing "$INSTALL_DIR/update.sh"
    
    # Create the script from the template
    sed "s|__CONFIG_FILE__|$CONFIG_DIR/config.conf|g" update.sh.tpl > "$INSTALL_DIR/update.sh"
    
    chmod +x "$INSTALL_DIR/update.sh"
    chown root:root "$INSTALL_DIR/update.sh"
    
    print_success "Main script installed to $INSTALL_DIR/update.sh"
}

install_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        print_warning "Systemd not available, skipping systemd integration"
        return 0
    fi
    
    print_status "Installing systemd service and timer..."
    
    # Check if systemd files exist
    if [[ ! -f "systemd/ubuntu-auto-update.service" ]] || [[ ! -f "systemd/ubuntu-auto-update.timer" ]]; then
        print_error "Systemd files not found in systemd/ directory"
        return 1
    fi
    
    # Backup existing files
    backup_existing "$SYSTEMD_DIR/ubuntu-auto-update.service"
    backup_existing "$SYSTEMD_DIR/ubuntu-auto-update.timer"
    
    # Install systemd files
    cp systemd/ubuntu-auto-update.service "$SYSTEMD_DIR/"
    cp systemd/ubuntu-auto-update.timer "$SYSTEMD_DIR/"
    chmod 644 "$SYSTEMD_DIR/ubuntu-auto-update.service"
    chmod 644 "$SYSTEMD_DIR/ubuntu-auto-update.timer"
    chown root:root "$SYSTEMD_DIR/ubuntu-auto-update.service"
    chown root:root "$SYSTEMD_DIR/ubuntu-auto-update.timer"
    
    # Update service file with correct script path
    sed -i "s|/usr/local/bin/update.sh|$INSTALL_DIR/update.sh|g" "$SYSTEMD_DIR/ubuntu-auto-update.service"
    
    # Reload systemd
    systemctl daemon-reload
    
    print_success "Systemd service and timer installed"
    
    # Ask user if they want to enable the timer
    read -p "Do you want to enable the systemd timer for automatic updates? [y/N]: " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        systemctl enable ubuntu-auto-update.timer
        systemctl start ubuntu-auto-update.timer
        print_success "Systemd timer enabled and started"
        
        # Show timer status
        print_status "Timer status:"
        systemctl list-timers ubuntu-auto-update.timer --no-pager
    else
        print_status "Timer not enabled. You can enable it later with:"
        print_status "  sudo systemctl enable ubuntu-auto-update.timer"
        print_status "  sudo systemctl start ubuntu-auto-update.timer"
    fi
}

setup_logging() {
    print_status "Setting up logging..."
    
    # Create log directory
    mkdir -p "$LOG_DIR"
    chmod 755 "$LOG_DIR"
    
    # Create logrotate configuration
    cat > /etc/logrotate.d/ubuntu-auto-update << EOF
$LOG_DIR/*.log {
    daily
    missingok
    rotate 5
    compress
    delaycompress
    notifempty
    create 0644 root root
    postrotate
        # Nothing needed for our log files
    endscript
}
EOF
    
    chmod 644 /etc/logrotate.d/ubuntu-auto-update
    
    print_success "Logging configured in $LOG_DIR"
}



show_summary() {
    print_success "Installation completed successfully!"
    echo
    echo "=== Installation Summary ==="
    echo "• Main script: $INSTALL_DIR/update.sh"
    echo "• Configuration: $CONFIG_DIR/config.conf"
    echo "• Logs: $LOG_DIR/"
    
    if command -v systemctl >/dev/null 2>&1; then
        echo "• Systemd service: ubuntu-auto-update.service"
        echo "• Systemd timer: ubuntu-auto-update.timer"
    fi
    echo
    echo "=== Quick Start ==="
    echo "• Test the script: sudo $INSTALL_DIR/update.sh --dry-run"
    echo "• Run manually: sudo $INSTALL_DIR/update.sh"
    echo "• Edit configuration: sudo nano $CONFIG_DIR/config.conf"
    
    if command -v systemctl >/dev/null 2>&1; then
        echo "• Check timer status: systemctl status ubuntu-auto-update.timer"
        echo "• View logs: journalctl -u ubuntu-auto-update.service"
    fi
    
    echo "• View update logs: sudo tail -f $LOG_DIR/update.log"
    echo
    print_warning "Remember to review and customize the configuration file!"
}

main() {
    echo "Ubuntu Auto-Update Installation Script v2.0.0"
    echo "============================================="
    echo
    
    check_root
    check_system
    
    find_install_dir
    validate_dirs
    install_script
    setup_logging
    install_systemd || print_warning "Systemd integration skipped"
    
    show_summary
}

main "$@"