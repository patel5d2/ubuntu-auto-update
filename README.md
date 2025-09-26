# Ubuntu Auto-Update Script - Enhanced Version

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Bash](https://img.shields.io/badge/Made%20with-Bash-1f425f.svg)](https://www.gnu.org/software/bash/)
[![Ubuntu](https://img.shields.io/badge/Ubuntu-E95420?style=flat&logo=ubuntu&logoColor=white)](https://ubuntu.com/)

A comprehensive, production-ready Ubuntu system update script with advanced features including logging, configuration management, notification support, and robust error handling.

## ✨ Features

### Core Functionality
- **Automated Package Management**: Updates package lists, upgrades packages, and performs full distribution upgrades
- **Multi-Package Manager Support**: Handles APT, Snap, and Flatpak packages
- **Intelligent Cleanup**: Removes unnecessary packages and cleans package cache
- **Firmware Updates**: Optional firmware updating via fwupdmgr

### Advanced Features
- **Comprehensive Logging**: Detailed logs with rotation and configurable retention
- **Configuration File Support**: Customize behavior without modifying the script
- **Lock File Management**: Prevents multiple instances from running simultaneously
- **Pre-flight Checks**: Validates prerequisites before starting updates
- **Reboot Detection**: Automatically detects when reboots are required
- **Notification System**: Email and Discord webhook notifications
- **Dry Run Mode**: Preview what will be done without making changes
- **Quiet Mode**: Minimal output for automated runs

### Safety & Reliability
- **Error Handling**: Robust error handling with graceful degradation
- **Disk Space Monitoring**: Warns about low disk space
- **Internet Connectivity Check**: Ensures connection before starting
- **Process Isolation**: Uses lock files to prevent conflicts
- **Auto-reboot Scheduling**: Optional scheduled reboots for kernel updates

## 🚀 Quick Start

### Basic Installation
```bash
# Download the script
curl -O https://raw.githubusercontent.com/patel5d2/ubuntu-auto-update/main/update.sh
chmod +x update.sh

# Run with default settings
sudo ./update.sh
```

### Advanced Installation
```bash
# Clone the repository
git clone https://github.com/patel5d2/ubuntu-auto-update.git
cd ubuntu-auto-update

# Make scripts executable
chmod +x update.sh install.sh

# Run the installer
sudo ./install.sh
```

## 📋 Usage

### Command Line Options
```bash
./update.sh [OPTIONS]

Options:
  -h, --help              Show help message
  -q, --quiet             Run in quiet mode (minimal output)
  -c, --config FILE       Use custom configuration file
  --no-reboot-check       Skip reboot requirement check
  --dry-run               Show what would be done without executing
  --create-config         Create default configuration file
```

### Examples
```bash
# Basic run with default settings
sudo ./update.sh

# Run in quiet mode (perfect for cron jobs)
sudo ./update.sh --quiet

# Use custom configuration
sudo ./update.sh --config /etc/my-custom-update.conf

# Preview changes without executing
sudo ./update.sh --dry-run

# Create configuration file
./update.sh --create-config
```

## ⚙️ Configuration

### Creating Configuration File
```bash
# Create default configuration
./update.sh --create-config
```

### Configuration Options
```bash
# Package management options
ENABLE_FULL_UPGRADE=true          # Enable full distribution upgrades
ENABLE_AUTOREMOVE=true            # Remove unnecessary packages
ENABLE_AUTOCLEAN=true             # Clean package cache

# Third-party package managers
ENABLE_SNAP_UPDATES=true          # Update snap packages
ENABLE_FLATPAK_UPDATES=true       # Update flatpak packages

# System options
ENABLE_AUTO_REBOOT=false          # Automatically reboot if required
AUTO_REBOOT_TIME="03:00"          # Time for scheduled reboots
UPDATE_FIRMWARE=false             # Update system firmware

# Logging and notifications
QUIET_MODE=false                  # Enable quiet mode
NOTIFICATION_EMAIL=""             # Email for notifications
SEND_DISCORD_WEBHOOK=""           # Discord webhook URL

# Advanced options
MAX_LOG_SIZE="10M"                # Maximum log file size
MAX_LOG_FILES=5                   # Number of log files to keep
```

## 📁 File Structure

```
ubuntu-auto-update/
├── update.sh              # Main update script
├── config.conf           # Configuration file (created on first run)
├── install.sh            # Installation script
├── uninstall.sh          # Uninstallation script
├── systemd/
│   ├── ubuntu-auto-update.service  # Systemd service
│   └── ubuntu-auto-update.timer    # Systemd timer
├── docs/
│   ├── CHANGELOG.md       # Version history
│   └── TROUBLESHOOTING.md # Common issues and solutions
└── README.md             # This file
```

## 🔄 Automation Options

### Method 1: Cron (Traditional)
```bash
# Edit crontab
crontab -e

# Add entry for daily updates at 2:00 AM
0 2 * * * /usr/local/bin/update.sh --quiet >> /var/log/ubuntu-auto-update/cron.log 2>&1
```

### Method 2: Systemd (Recommended)
```bash
# Install systemd files
sudo ./install.sh

# Enable and start the timer
sudo systemctl enable ubuntu-auto-update.timer
sudo systemctl start ubuntu-auto-update.timer

# Check timer status
sudo systemctl status ubuntu-auto-update.timer
```

## 📊 Logging

### Log Locations
- **Main log**: `/var/log/ubuntu-auto-update/update.log`
- **Cron log**: `/var/log/ubuntu-auto-update/cron.log` (if using cron)
- **System log**: Check with `journalctl -u ubuntu-auto-update.service`

### Log Rotation
- Automatic rotation when logs exceed configured size
- Configurable retention (default: 5 files)
- Integration with system logrotate

### Sample Log Output
```
[2024-01-15 02:00:01] [INFO] Starting Ubuntu Auto-Update v2.0.0
[2024-01-15 02:00:01] [INFO] Timestamp: Mon Jan 15 02:00:01 UTC 2024
[2024-01-15 02:00:01] [INFO] Hostname: ubuntu-server
[2024-01-15 02:00:02] [INFO] Updating package lists...
[2024-01-15 02:00:10] [INFO] Found 15 packages to upgrade
[2024-01-15 02:00:45] [INFO] Package upgrade completed successfully
[2024-01-15 02:01:20] [INFO] Update process completed successfully
```

## 🔔 Notifications

### Email Notifications
```bash
# Install mail utilities
sudo apt-get install mailutils

# Configure in config.conf
NOTIFICATION_EMAIL="admin@yourdomain.com"
```

### Discord Webhooks
```bash
# Set webhook URL in config.conf
SEND_DISCORD_WEBHOOK="https://discord.com/api/webhooks/YOUR_WEBHOOK_URL"
```

## 🛡️ Security Considerations

- **Sudo Requirements**: Script requires sudo privileges for system modifications
- **Lock File Protection**: Prevents multiple instances and potential conflicts
- **Configuration Validation**: Validates configuration options before execution
- **Error Isolation**: Failures in non-critical components don't stop the entire process
- **Secure Notifications**: Webhook URLs and email credentials are handled securely

## 🔧 Troubleshooting

### Common Issues

**Script fails with permission errors**
```bash
# Ensure script is executable
chmod +x update.sh

# Ensure sudo access
sudo -v
```

**Lock file errors**
```bash
# Remove stale lock file
sudo rm -f /tmp/ubuntu-auto-update.lock
```

**Logging issues**
```bash
# Check log directory permissions
sudo ls -la /var/log/ubuntu-auto-update/

# Create log directory if missing
sudo mkdir -p /var/log/ubuntu-auto-update
```

### Debug Mode
```bash
# Run with bash debug mode
bash -x ./update.sh
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

### Development Setup
```bash
git clone https://github.com/patel5d2/ubuntu-auto-update.git
cd ubuntu-auto-update

# Make changes
# Test thoroughly on a VM before submitting

# Submit pull request
```

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Original inspiration from the basic Ubuntu update script
- Community feedback and contributions
- Ubuntu documentation and best practices

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/patel5d2/ubuntu-auto-update/issues)
- **Discussions**: [GitHub Discussions](https://github.com/patel5d2/ubuntu-auto-update/discussions)
- **Documentation**: [Wiki](https://github.com/patel5d2/ubuntu-auto-update/wiki)

## 🗺️ Roadmap

- [ ] Web dashboard for monitoring multiple servers
- [ ] Integration with popular monitoring tools (Zabbix, Nagios, etc.)
- [ ] Support for other Debian-based distributions
- [ ] Rollback functionality for failed updates
- [ ] Package whitelist/blacklist support
- [ ] Integration with configuration management tools (Ansible, Puppet)

---

**⚠️ Important**: Always test this script in a non-production environment first. While designed to be safe and reliable, system updates can occasionally cause issues. Regular backups are recommended before running any automated update process.
