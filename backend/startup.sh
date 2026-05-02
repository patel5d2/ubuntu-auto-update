#!/bin/sh
set -eu

: "${DATABASE_URL:?DATABASE_URL must be set}"
: "${MIGRATIONS_PATH:=/app/migrations}"
: "${ENCRYPTION_KEY_FILE:=/app/encryption.key}"

# Generate a 32-byte AES-GCM key on first start if none is mounted.
# Re-deploys preserve the key by mounting it as a volume / secret.
if [ ! -f "${ENCRYPTION_KEY_FILE}" ] || [ ! -s "${ENCRYPTION_KEY_FILE}" ]; then
  mkdir -p "$(dirname "${ENCRYPTION_KEY_FILE}")"
  echo "[startup] generating new encryption key at ${ENCRYPTION_KEY_FILE}"
  head -c 32 /dev/urandom > "${ENCRYPTION_KEY_FILE}"
  chmod 0600 "${ENCRYPTION_KEY_FILE}"
fi

# Wait until Postgres accepts connections. The compose healthcheck already
# gates this, but we keep the loop for non-compose deployments.
echo "[startup] waiting for database..."
until pg_isready -d "${DATABASE_URL}" >/dev/null 2>&1; do
  sleep 1
done

echo "[startup] running migrations from ${MIGRATIONS_PATH}"
migrate -path "${MIGRATIONS_PATH}" -database "${DATABASE_URL}" up

echo "[startup] launching ua-backend"
exec /app/ua-backend
