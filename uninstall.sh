#!/bin/bash
# Ubuntu Auto-Update Uninstaller (repo script)
# This removes installed scripts, services, and optionally configuration/logs.
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [[ $EUID -ne 0 ]]; then
  echo -e "${RED}[ERROR]${NC} This script must be run as root (use sudo)" >&2
  exit 1
fi

echo -e "${YELLOW}[WARNING]${NC} This will remove Ubuntu Auto-Update from your system"
read -p "Are you sure you want to continue? [y/N]: " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
  echo "Uninstall cancelled"
  exit 0
fi

# Stop timers/services if present
systemctl stop ubuntu-auto-update.timer 2>/dev/null || true
systemctl disable ubuntu-auto-update.timer 2>/dev/null || true
systemctl stop ubuntu-auto-update.service 2>/dev/null || true
systemctl disable ubuntu-auto-update.service 2>/dev/null || true

# Dashboard service (if installed)
systemctl stop ubuntu-auto-update-dashboard 2>/dev/null || true
systemctl disable ubuntu-auto-update-dashboard 2>/dev/null || true

# Remove systemd units
rm -f /etc/systemd/system/ubuntu-auto-update.service || true
rm -f /etc/systemd/system/ubuntu-auto-update.timer || true
rm -f /etc/systemd/system/ubuntu-auto-update-dashboard.service || true
systemctl daemon-reload 2>/dev/null || true

# Remove installed scripts
rm -f /usr/local/bin/update.sh || true
rm -f /usr/local/bin/ubuntu-auto-update-uninstall.sh || true

# Ask whether to remove configuration and logs
echo -e "${YELLOW}[PROMPT]${NC} Remove configuration, logs, and dashboard env?"
read -p "This removes /etc/ubuntu-auto-update, /var/log/ubuntu-auto-update, and /etc/ubuntu-auto-update-dashboard.env [y/N]: " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
  rm -rf /etc/ubuntu-auto-update || true
  rm -rf /var/log/ubuntu-auto-update || true
  rm -f /etc/logrotate.d/ubuntu-auto-update || true
  rm -f /etc/ubuntu-auto-update-dashboard.env || true
  echo -e "${GREEN}[INFO]${NC} Configuration and logs removed"
else
  echo -e "${GREEN}[INFO]${NC} Configuration and logs preserved"
fi

echo -e "${GREEN}[SUCCESS]${NC} Ubuntu Auto-Update has been uninstalled"
