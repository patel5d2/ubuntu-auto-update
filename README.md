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

### Installation

```bash
# Clone the repository
git clone https://github.com/patel5d2/ubuntu-auto-update.git
cd ubuntu-auto-update

# Run the installer
sudo ./install.sh
```

## 📋 Usage

### Command Line Options

```bash
update.sh [OPTIONS]

Options:
  -h, --help              Show this help message
  -q, --quiet             Run in quiet mode (minimal output)
  -c, --config FILE       Use custom configuration file
  --no-reboot-check       Skip reboot requirement check
  --dry-run               Show what would be done without executing
  --create-config         Create default configuration file
```

### Examples

```bash
# Basic run with default settings
sudo update.sh

# Run in quiet mode (perfect for cron jobs)
sudo update.sh --quiet

# Use custom configuration
sudo update.sh --config /etc/my-custom-update.conf

# Preview changes without executing
sudo update.sh --dry-run

# Create configuration file
update.sh --create-config
```

## ⚙️ Configuration

### Creating Configuration File

```bash
# Create default configuration
update.sh --create-config
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
DISABLE_CURL=false                # Disable curl command

# Advanced options
MAX_LOG_SIZE="10M"                # Maximum log file size
MAX_LOG_FILES=5                   # Number of log files to keep
```

## 📁 File Structure

```
ubuntu-auto-update/
├── dashboard/
│   ├── app/
│   │   ├── api/
│   │   │   ├── __init__.py
│   │   │   ├── config.py
│   │   │   ├── config_file.py
│   │   │   ├── reports.py
│   │   │   ├── script.py
│   │   │   ├── servers.py
│   │   │   └── stats.py
│   │   ├── core/
│   │   │   ├── __init__.py
│   │   │   ├── config_parser.py
│   │   │   ├── csrf.py
│   │   │   ├── database.py
│   │   │   ├── runner.c
│   │   │   ├── security.py
│   │   │   ├── settings.py
│   │   │   └── websockets.py
│   │   ├── templates/
│   │   │   ├── config.html
│   │   │   ├── index.html
│   │   │   └── stats.html
│   │   ├── __init__.py
│   │   └── main.py
│   └── dashboard.db
├── docs/
│   ├── CHANGELOG.md
│   └── TROUBLESHOOTING.md
├── systemd/
│   ├── ubuntu-auto-update-dashboard.service
│   ├── ubuntu-auto-update.service
│   └── ubuntu-auto-update.timer
├── .dockerignore
├── .env
├── .gitignore
├── CHANGELOG.md
├── config.conf
├── config.default.conf
├── Dockerfile
├── Dockerfile.host-update
├── install.sh
├── LICENSE
├── README.md
├── requirements.txt
├── uninstall.sh
├── update-host.sh
└── update.sh.tpl
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
- **Dashboard Security**: For production use, it is strongly recommended to run the dashboard behind a reverse proxy (e.g., Nginx, Caddy) that provides HTTPS. This will protect your API key and other sensitive data from being transmitted in plain text.

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

## Uninstalling

To uninstall Ubuntu Auto-Update, run the following commands:

```bash
# Stop and disable the systemd timer
sudo systemctl stop ubuntu-auto-update.timer
sudo systemctl disable ubuntu-auto-update.timer

# Remove the systemd files
sudo rm -f /etc/systemd/system/ubuntu-auto-update.service
sudo rm -f /etc/systemd/system/ubuntu-auto-update.timer
sudo systemctl daemon-reload

# Remove the script
sudo rm -f /usr/local/bin/update.sh

# Remove the configuration and logs (optional)
sudo rm -rf /etc/ubuntu-auto-update
sudo rm -rf /var/log/ubuntu-auto-update
sudo rm -f /etc/logrotate.d/ubuntu-auto-update
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

## 📖 How to use

1.  **Installation:**

    ```bash
    # Clone the repository
    git clone https://github.com/patel5d2/ubuntu-auto-update.git
    cd ubuntu-auto-update

    # Run the installer
    sudo ./install.sh
    ```

2.  **Configuration:**

    The configuration file is located at `/etc/ubuntu-auto-update/config.conf`. You can edit this file to change the behavior of the script.

    You can also configure the script from the web dashboard.

3.  **Web Dashboard:**

    The web dashboard allows you to view the update history, configure the script, and run the update script manually.

    To run the dashboard, you can use the Docker container or run it directly on the host.

    **Docker (recommended):**

    ```bash
    # Build the image
    docker build -t ubuntu-auto-update -f Dockerfile.host-update .

    # Run the container
    docker run -d --privileged -v /:/host -p 8080:8080 --name ubuntu-auto-update ubuntu-auto-update
    ```

    **Directly on the host:**

    ```bash
    # Install dependencies
    python3 -m venv .venv
    source .venv/bin/activate
    pip install -r requirements.txt

    # Set the API key
    export DASHBOARD_API_KEY="<your_api_key>"

    # Run the dashboard
    uvicorn app.main:app --host 0.0.0.0 --port 8080
    ```

4.  **Automation:**

    You can automate the execution of the update script using cron or systemd.

    **Cron:**

    ```bash
    # Edit crontab
    crontab -e

    # Add entry for daily updates at 2:00 AM
    0 2 * * * /usr/local/bin/update.sh --quiet >> /var/log/ubuntu-auto-update/cron.log 2>&1
    ```

    **Systemd:**

    ```bash
    # Enable and start the timer
    sudo systemctl enable ubuntu-auto-update.timer
    sudo systemctl start ubuntu-auto-update.timer
    ```

## 🌐 Web Dashboard (optional)
A lightweight dashboard is included under `dashboard/` to collect and view update results from multiple servers.

### Features
- HTTP API to ingest update results (POST /api/v1/reports)
- Basic table UI at `/` with auto-refresh and server filtering
- SQLite storage (file `dashboard/dashboard.db`)
- Simple API key auth via header `X-API-Key`

### Run the dashboard
1) Install dependencies (recommend a virtualenv):

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r dashboard/requirements.txt
```

2) Set an API key the server will accept:

The API key is automatically generated and stored in the `.env` file in the root of the project. You can view the API key by running the following command:

```bash
cat .env
```

Then, you need to set the `DASHBOARD_API_KEY` environment variable to the value of the API key.

```bash
export DASHBOARD_API_KEY="<your_api_key>"
```

Alternatively, you can set the API key in your browser's local storage. Open the developer console and run the following command:

```javascript
localStorage.setItem("apiKey", "<your_api_key>")
```

3) Start the server:

```bash
python -m uvicorn dashboard.app:app --host 0.0.0.0 --port 8080
```

Then open http://localhost:8080/ in your browser.

### Configure agents (servers)
In your `config.conf` (typically `/etc/ubuntu-auto-update/config.conf`) set:

```bash
SEND_DASHBOARD_URL="http://<dashboard-host>:8080/api/v1/reports"
DASHBOARD_API_KEY="<the-same-key-as-server>"
```

No restart of the timer is necessary; each run will POST a short JSON with:
- server hostname
- status (success/error)
- exit code
- duration (seconds)
- reboot required (true/false)
- timestamp

If the dashboard is unreachable, the update still completes; a warning is logged.

### Run the dashboard as a systemd service
1) Place the repo on the server (e.g., /opt/ubuntu-auto-update) and create a Python venv:

```bash
sudo mkdir -p /opt/ubuntu-auto-update
sudo chown -R $USER:$USER /opt/ubuntu-auto-update
rsync -a --delete ./ /opt/ubuntu-auto-update/
python3 -m venv /opt/ubuntu-auto-update/.venv
/opt/ubuntu-auto-update/.venv/bin/pip install -r /opt/ubuntu-auto-update/dashboard/requirements.txt
```

2) Create environment file:

```bash
cat | sudo tee /etc/ubuntu-auto-update-dashboard.env >/dev/null <<'EOF'
DASHBOARD_API_KEY="<a-strong-random-string>"
PORT=8080
EOF
```

3) Install service file and start:

```bash
sudo cp systemd/ubuntu-auto-update-dashboard.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable ubuntu-auto-update-dashboard
sudo systemctl start ubuntu-auto-update-dashboard
sudo systemctl --no-pager status ubuntu-auto-update-dashboard
```

### Docker deployment

The Dockerfile includes a security scanner that will scan the container for vulnerabilities. If any high or critical vulnerabilities are found, the build will fail. This ensures that the container is free of known vulnerabilities.

Build and run the dashboard with Docker:

```bash
# Build the image
docker build -t ubuntu-auto-update -f Dockerfile.host-update .

# Run the container
docker run -d --privileged -v /:/host -p 8080:8080 --name ubuntu-auto-update ubuntu-auto-update
```

**Security Warning:**

This Docker container is designed to update the host system, which is a dangerous operation. It runs with the `--privileged` flag and mounts the host's root filesystem into the container. This effectively breaks the isolation that Docker is designed to provide.

Use this container with extreme caution and only in trusted environments. If possible, it is recommended to use a different method to update the host system, such as running the `update.sh` script directly on the host.

## 🗺️ Roadmap

- [x] Web dashboard for monitoring multiple servers
- [ ] Integration with popular monitoring tools (Zabbix, Nagios, etc.)
- [ ] Support for other Debian-based distributions
- [ ] Rollback functionality for failed updates
- [ ] Package whitelist/blacklist support
- [ ] Integration with configuration management tools (Ansible, Puppet)

---

**⚠️ Important**: Always test this script in a non-production environment first. While designed to be safe and reliable, system updates can occasionally cause issues. Regular backups are recommended before running any automated update process.
