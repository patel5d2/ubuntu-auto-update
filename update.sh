#!/bin/bash
# Ubuntu Auto Update & Upgrade Script - Enhanced Version
# Author: Dharmin Patel
# Version: 2.0.0
# Description: Comprehensive system update script with logging, error handling, and configuration options

set -euo pipefail  # Exit on error, undefined vars, pipe failures

# Privilege escalation helper
if [[ $EUID -eq 0 ]]; then
    SUDO=""
else
    if ! sudo -n true 2>/dev/null; then
        echo "[ERROR] This script requires sudo privileges" >&2
        exit 1
    fi
    SUDO="sudo"
fi

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="${SCRIPT_DIR}/config.conf"
LOG_DIR="/var/log/ubuntu-auto-update"
LOG_FILE="${LOG_DIR}/update.log"
LOCK_FILE="/tmp/ubuntu-auto-update.lock"
MAX_LOG_SIZE="10M"
MAX_LOG_FILES=5

# Default configuration (can be overridden by config file)
ENABLE_FULL_UPGRADE=true
ENABLE_AUTOREMOVE=true
ENABLE_AUTOCLEAN=true
ENABLE_SNAP_UPDATES=true
ENABLE_FLATPAK_UPDATES=true
ENABLE_AUTO_REBOOT=false
AUTO_REBOOT_TIME="03:00"
NOTIFICATION_EMAIL=""
SEND_DISCORD_WEBHOOK=""
# Dashboard reporting (optional)
SEND_DASHBOARD_URL=""
DASHBOARD_API_KEY=""
UPDATE_FIRMWARE=false
QUIET_MODE=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Functions
log_message() {
    local level=$1
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    if [[ "$QUIET_MODE" != "true" ]] || [[ "$level" == "ERROR" ]]; then
        case $level in
            "INFO")  echo -e "${GREEN}[INFO]${NC} $message" ;;
            "WARN")  echo -e "${YELLOW}[WARN]${NC} $message" ;;
            "ERROR") echo -e "${RED}[ERROR]${NC} $message" >&2 ;;
            "DEBUG") echo -e "${BLUE}[DEBUG]${NC} $message" ;;
            *)       echo -e "${PURPLE}[$level]${NC} $message" ;;
        esac
    fi
    
    # Always log to file
    if [[ -w "$LOG_FILE" ]]; then
        echo "[$timestamp] [$level] $message" >> "$LOG_FILE"
    else
        if [[ -n "${SUDO:-}" ]]; then
            echo "[$timestamp] [$level] $message" | $SUDO tee -a "$LOG_FILE" >/dev/null
        else
            echo "[$timestamp] [$level] $message" | tee -a "$LOG_FILE" >/dev/null
        fi
    fi
}

setup_logging() {
    # Create log directory
    $SUDO mkdir -p "$LOG_DIR"
    $SUDO touch "$LOG_FILE"
    $SUDO chmod 644 "$LOG_FILE"
    
    # Rotate logs if they get too large
    if [[ -f "$LOG_FILE" ]] && [[ $(stat -c%s "$LOG_FILE" 2>/dev/null || stat -f%z "$LOG_FILE") -gt $(numfmt --from=iec "$MAX_LOG_SIZE") ]]; then
        $SUDO logrotate -f /etc/logrotate.conf 2>/dev/null || {
            # Manual log rotation if logrotate fails
            for i in $(seq $((MAX_LOG_FILES-1)) -1 1); do
                [[ -f "${LOG_FILE}.$i" ]] && $SUDO mv "${LOG_FILE}.$i" "${LOG_FILE}.$((i+1))"
            done
            [[ -f "$LOG_FILE" ]] && $SUDO mv "$LOG_FILE" "${LOG_FILE}.1"
            $SUDO touch "$LOG_FILE"
            $SUDO chmod 644 "$LOG_FILE"
        }
    fi
}

load_config() {
    if [[ -f "$CONFIG_FILE" ]]; then
        log_message "INFO" "Loading configuration from $CONFIG_FILE"
        source "$CONFIG_FILE"
    else
        log_message "WARN" "Configuration file not found at $CONFIG_FILE, using defaults"
    fi
}

create_lock_file() {
    if [[ -f "$LOCK_FILE" ]]; then
        local lock_pid=$(cat "$LOCK_FILE")
        if kill -0 "$lock_pid" 2>/dev/null; then
            log_message "ERROR" "Another instance is already running (PID: $lock_pid)"
            exit 1
        else
            log_message "WARN" "Removing stale lock file"
            rm -f "$LOCK_FILE"
        fi
    fi
    
    echo $$ > "$LOCK_FILE"
    trap 'rm -f "$LOCK_FILE"; exit' INT TERM EXIT
}

check_prerequisites() {
    # Check if running as root or with sudo privileges
    if [[ $EUID -eq 0 ]]; then
        log_message "WARN" "Running as root user"
    elif ! $SUDO -n true 2>/dev/null; then
        log_message "ERROR" "This script requires sudo privileges"
        exit 1
    fi
    
    # Check internet connectivity
    if ! ping -c 1 8.8.8.8 >/dev/null 2>&1; then
        log_message "ERROR" "No internet connection available"
        exit 1
    fi
    
    # Check available disk space (require at least 1GB free)
    local available_space=$(df / | awk 'NR==2 {print $4}')
    if [[ $available_space -lt 1048576 ]]; then  # 1GB in KB
        log_message "WARN" "Low disk space: $(df -h / | awk 'NR==2 {print $4}') available"
    fi
}

update_package_lists() {
    log_message "INFO" "Updating package lists..."
    
    # Clean package cache if it's getting large
    local cache_size=$(du -sm /var/cache/apt/archives 2>/dev/null | cut -f1 || echo 0)
    if [[ $cache_size -gt 1000 ]]; then  # More than 1GB
        log_message "INFO" "Cleaning large package cache ($cache_size MB)"
        $SUDO apt-get clean
    fi
    
    if ! $SUDO apt-get update -y; then
        log_message "ERROR" "Failed to update package lists"
        return 1
    fi
    
    log_message "INFO" "Package lists updated successfully"
}

upgrade_packages() {
    log_message "INFO" "Upgrading installed packages..."
    
    # Check for packages that can be upgraded
    local upgradeable_count=$(apt list --upgradable 2>/dev/null | grep -c upgradable || echo 0)
    
    if [[ $upgradeable_count -eq 0 ]]; then
        log_message "INFO" "No packages need upgrading"
        return 0
    fi
    
    log_message "INFO" "Found $upgradeable_count packages to upgrade"
    
    if ! $SUDO DEBIAN_FRONTEND=noninteractive apt-get upgrade -y; then
        log_message "ERROR" "Package upgrade failed"
        return 1
    fi
    
    log_message "INFO" "Package upgrade completed successfully"
}

full_upgrade() {
    if [[ "$ENABLE_FULL_UPGRADE" == "true" ]]; then
        log_message "INFO" "Performing full distribution upgrade..."
        
        if ! $SUDO DEBIAN_FRONTEND=noninteractive apt-get full-upgrade -y; then
            log_message "ERROR" "Full upgrade failed"
            return 1
        fi
        
        log_message "INFO" "Full distribution upgrade completed"
    else
        log_message "INFO" "Full upgrade disabled in configuration"
    fi
}

cleanup_packages() {
    if [[ "$ENABLE_AUTOREMOVE" == "true" ]]; then
        log_message "INFO" "Removing unnecessary packages..."
        $SUDO apt-get autoremove -y
        log_message "INFO" "Unnecessary packages removed"
    fi
    
    if [[ "$ENABLE_AUTOCLEAN" == "true" ]]; then
        log_message "INFO" "Cleaning package cache..."
        $SUDO apt-get autoclean
        log_message "INFO" "Package cache cleaned"
    fi
}

update_snap_packages() {
    if [[ "$ENABLE_SNAP_UPDATES" == "true" ]] && command -v snap >/dev/null 2>&1; then
        log_message "INFO" "Updating snap packages..."
        if $SUDO snap refresh; then
            log_message "INFO" "Snap packages updated successfully"
        else
            log_message "WARN" "Some snap updates may have failed"
        fi
    fi
}

update_flatpak_packages() {
    if [[ "$ENABLE_FLATPAK_UPDATES" == "true" ]] && command -v flatpak >/dev/null 2>&1; then
        log_message "INFO" "Updating Flatpak packages..."
        if flatpak update -y; then
            log_message "INFO" "Flatpak packages updated successfully"
        else
            log_message "WARN" "Some Flatpak updates may have failed"
        fi
    fi
}

update_firmware() {
    if [[ "$UPDATE_FIRMWARE" == "true" ]] && command -v fwupdmgr >/dev/null 2>&1; then
        log_message "INFO" "Checking for firmware updates..."
        if $SUDO fwupdmgr refresh && $SUDO fwupdmgr update -y; then
            log_message "INFO" "Firmware updated successfully"
        else
            log_message "WARN" "Firmware update failed or no updates available"
        fi
    fi
}

check_reboot_required() {
    if [[ -f /var/run/reboot-required ]]; then
        log_message "WARN" "System reboot is required"
        
        if [[ -f /var/run/reboot-required.pkgs ]]; then
            local packages=$(cat /var/run/reboot-required.pkgs | tr '\n' ' ')
            log_message "INFO" "Packages requiring reboot: $packages"
        fi
        
        if [[ "$ENABLE_AUTO_REBOOT" == "true" ]]; then
            log_message "INFO" "Auto-reboot enabled, scheduling reboot for $AUTO_REBOOT_TIME"
            echo "shutdown -r $AUTO_REBOOT_TIME" | $SUDO at "$AUTO_REBOOT_TIME" 2>/dev/null || {
                log_message "WARN" "'at' command not available, scheduling immediate reboot in 1 minute"
                $SUDO shutdown -r +1
            }
        else
            log_message "WARN" "Please reboot the system to complete the update process"
        fi
        
        return 1
    else
        log_message "INFO" "No reboot required"
        return 0
    fi
}

send_notification() {
    local status=$1
    local message="Ubuntu Auto-Update on $(hostname): $status at $(date)"
    
    # Email notification
    if [[ -n "$NOTIFICATION_EMAIL" ]] && command -v mail >/dev/null 2>&1; then
        echo "$message" | mail -s "Ubuntu Auto-Update Report" "$NOTIFICATION_EMAIL"
        log_message "INFO" "Email notification sent to $NOTIFICATION_EMAIL"
    fi
    
    # Discord webhook notification
    if [[ -n "$SEND_DISCORD_WEBHOOK" ]]; then
        curl -H "Content-Type: application/json" \
             -d "{\"content\":\"$message\"}" \
             "$SEND_DISCORD_WEBHOOK" >/dev/null 2>&1 || \
        log_message "WARN" "Failed to send Discord notification"
    fi
}

post_dashboard_update() {
    local status=$1
    local exit_code=$2
    local start_time=$3

    # Only post if configured
    if [[ -z "${SEND_DASHBOARD_URL}" ]]; then
        return 0
    fi

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    local reboot_required=false
    [[ -f /var/run/reboot-required ]] && reboot_required=true

    local hostname
    hostname=$(hostname)
    local timestamp
    timestamp=$(date -Iseconds)

    # Build JSON payload without requiring jq
    local payload
    payload=$(printf '{"server":"%s","timestamp":"%s","status":"%s","exit_code":%d,"duration_seconds":%d,"reboot_required":%s,"version":"%s"}' \
        "$hostname" "$timestamp" "$status" "$exit_code" "$duration" "$reboot_required" "2.0.0")

    curl -sS -X POST \
        -H "Content-Type: application/json" \
        -H "X-API-Key: ${DASHBOARD_API_KEY:-}" \
        -d "$payload" \
        "$SEND_DASHBOARD_URL" >/dev/null 2>&1 || \
        log_message "WARN" "Failed to POST update status to dashboard"
}

show_summary() {
    local start_time=$1
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    log_message "INFO" "======================================"
    log_message "INFO" "Update Summary:"
    log_message "INFO" "- Duration: ${duration}s"
    log_message "INFO" "- Log file: $LOG_FILE"
    
    if [[ -f /var/run/reboot-required ]]; then
        log_message "WARN" "- Reboot required: YES"
    else
        log_message "INFO" "- Reboot required: NO"
    fi
    
    log_message "INFO" "======================================"
}

show_help() {
    cat << EOF
Ubuntu Auto Update & Upgrade Script - Enhanced Version

Usage: $0 [OPTIONS]

Options:
  -h, --help              Show this help message
  -q, --quiet             Run in quiet mode (minimal output)
  -c, --config FILE       Use custom configuration file
  --no-reboot-check       Skip reboot requirement check
  --dry-run               Show what would be done without executing
  --create-config         Create default configuration file

Examples:
  $0                      Run with default settings
  $0 --quiet              Run silently
  $0 --config /etc/my-update.conf  Use custom config

Configuration file location: $CONFIG_FILE
Log file location: $LOG_FILE

EOF
}

create_default_config() {
    cat > "$CONFIG_FILE" << 'EOF'
# Ubuntu Auto-Update Configuration File

# Package management options
ENABLE_FULL_UPGRADE=true
ENABLE_AUTOREMOVE=true
ENABLE_AUTOCLEAN=true

# Third-party package managers
ENABLE_SNAP_UPDATES=true
ENABLE_FLATPAK_UPDATES=true

# System options
ENABLE_AUTO_REBOOT=false
AUTO_REBOOT_TIME="03:00"
UPDATE_FIRMWARE=false

# Logging and notifications
QUIET_MODE=false
NOTIFICATION_EMAIL=""
SEND_DISCORD_WEBHOOK=""

# Dashboard reporting
SEND_DASHBOARD_URL=""
DASHBOARD_API_KEY=""

# Advanced options
MAX_LOG_SIZE="10M"
MAX_LOG_FILES=5
EOF
    
    log_message "INFO" "Default configuration created at $CONFIG_FILE"
    log_message "INFO" "Please review and customize the settings as needed"
}

main() {
    local start_time=$(date +%s)
    local dry_run=false
    local skip_reboot_check=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -q|--quiet)
                QUIET_MODE=true
                shift
                ;;
            -c|--config)
                CONFIG_FILE="$2"
                shift 2
                ;;
            --no-reboot-check)
                skip_reboot_check=true
                shift
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            --create-config)
                create_default_config
                exit 0
                ;;
            *)
                log_message "ERROR" "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    # Initialize
    setup_logging
    load_config
    create_lock_file
    
    log_message "INFO" "Starting Ubuntu Auto-Update v2.0.0"
    log_message "INFO" "Timestamp: $(date)"
    log_message "INFO" "Hostname: $(hostname)"
    log_message "INFO" "User: $(whoami)"
    
    if [[ "$dry_run" == "true" ]]; then
        log_message "INFO" "DRY RUN MODE - No changes will be made"
        exit 0
    fi
    
    # Pre-flight checks
    check_prerequisites
    
    # Main update process
    local exit_code=0
    
    update_package_lists || exit_code=1
    upgrade_packages || exit_code=1
    full_upgrade || exit_code=1
    cleanup_packages || exit_code=1
    update_snap_packages || true  # Don't fail on snap errors
    update_flatpak_packages || true  # Don't fail on flatpak errors
    update_firmware || true  # Don't fail on firmware errors
    
    # Post-update tasks
    if [[ "$skip_reboot_check" != "true" ]]; then
        check_reboot_required || true  # Don't fail if reboot is required
    fi
    
    # Send notifications
    if [[ $exit_code -eq 0 ]]; then
        send_notification "COMPLETED SUCCESSFULLY"
        log_message "INFO" "Update process completed successfully"
    else
        send_notification "COMPLETED WITH ERRORS"
        log_message "ERROR" "Update process completed with errors"
    fi

    # Report to dashboard if configured
    if [[ $exit_code -eq 0 ]]; then
        post_dashboard_update "COMPLETED SUCCESSFULLY" "$exit_code" "$start_time"
    else
        post_dashboard_update "COMPLETED WITH ERRORS" "$exit_code" "$start_time"
    fi
    
    show_summary "$start_time"
    exit $exit_code
}

# Run main function with all arguments
main "$@"