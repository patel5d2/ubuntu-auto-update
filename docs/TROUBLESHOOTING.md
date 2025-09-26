# Troubleshooting Guide

This guide covers common issues and fixes for Ubuntu Auto-Update.

## 1) Service fails with sudo errors under systemd
- Symptoms in journal:
  - "sudo: PERM_SUDOERS: setresuid ... Operation not permitted"
  - "unable to open /etc/sudoers"
- Cause: systemd hardening (NoNewPrivileges/RestrictSUIDSGID) blocks sudo.
- Fix: Script now avoids sudo when already root. Ensure you’ve deployed the latest update.sh.
  - Reinstall: `sudo ./install.sh`
  - Or copy script: `sudo cp update.sh /usr/local/bin/update.sh && sudo chmod +x /usr/local/bin/update.sh`

## 2) DEBIAN_FRONTEND errors
- Symptom: `DEBIAN_FRONTEND=noninteractive: command not found`
- Fix: Updated script uses `env DEBIAN_FRONTEND=noninteractive ...`. Pull latest and redeploy.

## 3) No internet connection
- Symptom: `[ERROR] No internet connection available`
- Fix: Check connectivity: `ping -c 1 8.8.8.8`. Verify DNS and default route.

## 4) Low disk space warnings
- Symptom: `[WARN] Low disk space: ... available`
- Fix: `sudo apt-get autoremove -y && sudo apt-get autoclean` and clean large logs under `/var/log`.

## 5) Log file permission issues
- Symptom: script cannot append to `/var/log/ubuntu-auto-update/update.log`.
- Fix: Script now appends via tee under sudo when needed. Ensure `/var/log/ubuntu-auto-update` exists.

## 6) Systemd timer not running
- Check:
  - `systemctl list-timers ubuntu-auto-update.timer --no-pager`
  - `journalctl -u ubuntu-auto-update.service --no-pager -n 200`
- Fix:
  - `sudo systemctl enable --now ubuntu-auto-update.timer`

## 7) Dashboard doesn’t collect reports
- Check server URL and API key:
  - In `/etc/ubuntu-auto-update/config.conf`:
    - `SEND_DASHBOARD_URL="http://<host>:8080/api/v1/reports"`
    - `DASHBOARD_API_KEY="..."`
  - Dashboard env: `export DASHBOARD_API_KEY="..."`
- Check dashboard health: `curl http://<host>:8080/healthz`
- Check server logs: `journalctl -u ubuntu-auto-update-dashboard --no-pager -n 200`

## 8) Uninstalling
- Use `./uninstall.sh` from the repo root to remove service, timer, scripts, and optionally logs/configs. The installer also created `/usr/local/bin/ubuntu-auto-update-uninstall.sh`.
