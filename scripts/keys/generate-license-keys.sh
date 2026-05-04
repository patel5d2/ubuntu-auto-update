#!/usr/bin/env bash
# License Key Generation Script
# Generates an RSA keypair for signing/verifying license tokens.
#
# Usage:
#   ./scripts/keys/generate-license-keys.sh [output-dir]
#
# Produces:
#   license-private.pem   — vendor private key (keep secret!)
#   license-public.pem    — public key (embed in backend binary / distribute)

set -euo pipefail

OUTPUT_DIR="${1:-./scripts/keys}"
mkdir -p "$OUTPUT_DIR"

PRIV="$OUTPUT_DIR/license-private.pem"
PUB="$OUTPUT_DIR/license-public.pem"

if [[ -f "$PRIV" ]]; then
    echo "⚠  Private key already exists at $PRIV — skipping to avoid overwrite."
    echo "   Delete it manually if you want to regenerate."
    exit 0
fi

echo "Generating 2048-bit RSA keypair for license signing..."

openssl genrsa -out "$PRIV" 2048 2>/dev/null
openssl rsa -in "$PRIV" -pubout -out "$PUB" 2>/dev/null

chmod 600 "$PRIV"
chmod 644 "$PUB"

echo "✓ Private key: $PRIV  (mode 600 — keep secret!)"
echo "✓ Public key:  $PUB   (distribute with backend)"
echo ""
echo "Next steps:"
echo "  1. Set LICENSE_PUBLIC_KEY_FILE=$PUB in your backend config"
echo "  2. Use the private key to sign license JWTs (see scripts/keys/issue-license.sh)"
echo "  3. NEVER commit the private key to version control"
