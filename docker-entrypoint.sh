#!/bin/bash
# Docker entrypoint script

set -e

if [ "$1" = "update" ]; then
    # Execute the update script on the host
    if [ -d /host ]; then
        echo "WARNING: Executing update script on the host system. This is a dangerous operation."
        chroot /host /app/update.sh
    else
        echo "WARNING: /host directory not found. Skipping update script execution."
    fi
else
    # Start the dashboard
    /app/.venv/bin/uvicorn app.main:app --host 0.0.0.0 --port 8080
fi
