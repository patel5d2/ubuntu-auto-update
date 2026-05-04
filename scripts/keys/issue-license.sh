#!/usr/bin/env bash
# Issue a signed license token (JWT) for a tenant.
#
# Usage:
#   ./scripts/keys/issue-license.sh --tenant <id> --max-hosts <n> --days <validity> [--features feat1,feat2,...]
#
# Requires: openssl, base64 (GNU or BSD)
# The private key is expected at ./scripts/keys/license-private.pem (or set PRIV_KEY env var).

set -euo pipefail

PRIV_KEY="${PRIV_KEY:-./scripts/keys/license-private.pem}"
TENANT=""
MAX_HOSTS=0
DAYS=365
FEATURES=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --tenant)    TENANT="$2"; shift 2;;
        --max-hosts) MAX_HOSTS="$2"; shift 2;;
        --days)      DAYS="$2"; shift 2;;
        --features)  FEATURES="$2"; shift 2;;
        *) echo "Unknown: $1"; exit 1;;
    esac
done

if [[ -z "$TENANT" ]]; then
    echo "Usage: $0 --tenant <id> [--max-hosts N] [--days N] [--features feat1,feat2]"
    exit 1
fi

if [[ ! -f "$PRIV_KEY" ]]; then
    echo "Private key not found at $PRIV_KEY"
    echo "Run ./scripts/keys/generate-license-keys.sh first"
    exit 1
fi

NOW=$(date +%s)
EXP=$((NOW + DAYS * 86400))
LIC_ID="lic-$(openssl rand -hex 8)"

# Build features JSON array.
FEAT_JSON="[]"
if [[ -n "$FEATURES" ]]; then
    FEAT_JSON=$(echo "$FEATURES" | tr ',' '\n' | sed 's/.*/"&"/' | paste -sd, | sed 's/^/[/;s/$/]/')
fi

# Header.
HEADER='{"alg":"RS256","typ":"JWT"}'
HEADER_B64=$(echo -n "$HEADER" | openssl base64 -e -A | tr '+/' '-_' | tr -d '=')

# Payload.
PAYLOAD=$(cat <<EOF
{"iss":"ubuntu-auto-update","sub":"${TENANT}","iat":${NOW},"exp":${EXP},"max_hosts":${MAX_HOSTS},"features":${FEAT_JSON},"license_id":"${LIC_ID}"}
EOF
)
PAYLOAD_B64=$(echo -n "$PAYLOAD" | openssl base64 -e -A | tr '+/' '-_' | tr -d '=')

# Signature.
SIGNING_INPUT="${HEADER_B64}.${PAYLOAD_B64}"
SIG=$(echo -n "$SIGNING_INPUT" | openssl dgst -sha256 -sign "$PRIV_KEY" | openssl base64 -e -A | tr '+/' '-_' | tr -d '=')

TOKEN="${SIGNING_INPUT}.${SIG}"

echo "License Token (JWT):"
echo "===================="
echo "$TOKEN"
echo ""
echo "Details:"
echo "  Tenant:     $TENANT"
echo "  License ID: $LIC_ID"
echo "  Max Hosts:  $MAX_HOSTS"
echo "  Valid For:   $DAYS days"
echo "  Expires:    $(date -r $EXP 2>/dev/null || date -d @$EXP 2>/dev/null || echo $EXP)"
echo "  Features:   $FEATURES"
