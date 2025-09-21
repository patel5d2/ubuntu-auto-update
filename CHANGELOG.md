# Changelog

All notable changes to the Ubuntu Auto-Update script will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2024-01-15

### Added
- Complete rewrite of the original script with enhanced functionality
- Configuration file support (`config.conf`)
- Comprehensive logging system with log rotation
- Lock file management to prevent multiple instances
- Pre-flight checks (internet connectivity, disk space, sudo privileges)
- Support for Snap and Flatpak package updates
- Firmware update support via fwupdmgr
- Email and Discord webhook notifications
- Automatic reboot detection and optional scheduling
- Command line argument parsing with multiple options
- Systemd service and timer integration
- Professional installation script
- Dry-run mode for testing
- Quiet mode for automated runs
- Colored output for better readability
- Detailed error handling and graceful degradation
- Security hardening in systemd service
- Comprehensive documentation

### Enhanced
- Better error handling with proper exit codes
- Robust package cache management
- Intelligent log rotation
- Resource usage monitoring
- Process isolation and security

### Security
- Systemd service hardening with security options
- Lock file protection against race conditions
- Configuration validation
- Secure handling of notification credentials

### Documentation
- Complete README with installation and usage instructions
- Configuration examples for different use cases
- Troubleshooting guide
- Contributing guidelines
- Professional project structure

## [1.0.0] - 2024-01-01

### Added
- Basic Ubuntu update script functionality
- Simple package list updates
- Package upgrades and full-upgrades
- Automatic cleanup (autoremove and autoclean)
- Basic cron scheduling instructions
- Simple README documentation

### Features
- `apt update` - Update package lists
- `apt upgrade` - Upgrade installed packages
- `apt full-upgrade` - Full distribution upgrade
- `apt autoremove` - Remove unnecessary packages
- `apt autoclean` - Clean package cache

### Documentation
- Basic usage instructions
- Cron scheduling example
- Simple installation steps