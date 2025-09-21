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
INSTALL_DIR="/usr/local/bin"
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

backup_existing() {
    local file_path="$1"
    local backup_path="${file_path}.backup-$(date +%Y%m%d-%H%M%S)"
    
    if [[ -f "$file_path" ]]; then
        print_status "Backing up existing file: $file_path"
        cp "$file_path" "$backup_path"
        print_success "Backup created: $backup_path"
    fi
}

install_script() {
    print_status "Installing main update script..."
    
    if [[ ! -f "update.sh" ]]; then
        print_error "update.sh not found in current directory"
        exit 1
    fi
    
    # Backup existing installation
    backup_existing "$INSTALL_DIR/update.sh"
    
    # Copy script
    cp update.sh "$INSTALL_DIR/update.sh"
    chmod +x "$INSTALL_DIR/update.sh"
    chown root:root "$INSTALL_DIR/update.sh"
    
    print_success "Main script installed to $INSTALL_DIR/update.sh"
}

install_config() {
    print_status "Setting up configuration..."
    
    # Create config directory
    mkdir -p "$CONFIG_DIR"
    
    # Install default config if it doesn't exist
    if [[ ! -f "$CONFIG_DIR/config.conf" ]]; then
        if [[ -f "config.conf" ]]; then
            cp config.conf "$CONFIG_DIR/config.conf"
            chmod 644 "$CONFIG_DIR/config.conf"
            chown root:root "$CONFIG_DIR/config.conf"
            print_success "Configuration installed to $CONFIG_DIR/config.conf"
        else
            print_status "Creating default configuration..."
            "$INSTALL_DIR/update.sh" --create-config
            if [[ -f "config.conf" ]]; then
                mv config.conf "$CONFIG_DIR/config.conf"
                chmod 644 "$CONFIG_DIR/config.conf"
                chown root:root "$CONFIG_DIR/config.conf"
            fi
        fi
    else
        print_success "Configuration already exists at $CONFIG_DIR/config.conf"
    fi
    
    # Update script to use system config location
    sed -i "s|CONFIG_FILE=\"\${SCRIPT_DIR}/config.conf\"|CONFIG_FILE=\"$CONFIG_DIR/config.conf\"|" "$INSTALL_DIR/update.sh"
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

create_uninstaller() {
    print_status "Creating uninstaller..."
    
    cat > "$INSTALL_DIR/ubuntu-auto-update-uninstall.sh" << 'EOF'
#!/bin/bash
# Ubuntu Auto-Update Uninstaller

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [[ $EUID -ne 0 ]]; then
    echo -e "${RED}[ERROR]${NC} This script must be run as root (use sudo)"
    exit 1
fi

echo -e "${YELLOW}[WARNING]${NC} This will remove Ubuntu Auto-Update from your system"
read -p "Are you sure you want to continue? [y/N]: " -n 1 -r
echo

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Uninstall cancelled"
    exit 0
fi

echo -e "${GREEN}[INFO]${NC} Stopping and disabling systemd timer..."
systemctl stop ubuntu-auto-update.timer 2>/dev/null || true
systemctl disable ubuntu-auto-update.timer 2>/dev/null || true

echo -e "${GREEN}[INFO]${NC} Removing systemd files..."
rm -f /etc/systemd/system/ubuntu-auto-update.service
rm -f /etc/systemd/system/ubuntu-auto-update.timer
systemctl daemon-reload 2>/dev/null || true

echo -e "${GREEN}[INFO]${NC} Removing scripts..."
rm -f /usr/local/bin/update.sh
rm -f /usr/local/bin/ubuntu-auto-update-uninstall.sh

echo -e "${GREEN}[INFO]${NC} Removing configuration..."
read -p "Remove configuration and logs? [y/N]: " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf /etc/ubuntu-auto-update
    rm -rf /var/log/ubuntu-auto-update
    rm -f /etc/logrotate.d/ubuntu-auto-update
    echo -e "${GREEN}[INFO]${NC} Configuration and logs removed"
else
    echo -e "${GREEN}[INFO]${NC} Configuration and logs preserved"
fi

echo -e "${GREEN}[SUCCESS]${NC} Ubuntu Auto-Update has been uninstalled"
EOF
    
    chmod +x "$INSTALL_DIR/ubuntu-auto-update-uninstall.sh"
    chown root:root "$INSTALL_DIR/ubuntu-auto-update-uninstall.sh"
    
    print_success "Uninstaller created at $INSTALL_DIR/ubuntu-auto-update-uninstall.sh"
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
    
    echo "• Uninstaller: $INSTALL_DIR/ubuntu-auto-update-uninstall.sh"
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
    
    install_script
    install_config
    setup_logging
    install_systemd || print_warning "Systemd integration skipped"
    create_uninstaller
    
    show_summary
}

main "$@"