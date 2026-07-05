#!/usr/bin/env bash
# e2e.sh — end-to-end test of the real SSH engine against a live stack.
#
# Boots the production compose stack under an isolated project name
# (uau-e2e: own volumes, own network — your dev stack's data is never
# touched), adds an sshd-enabled Ubuntu container as a managed host, then
# drives the actual product flow over the API:
#
#   enroll (password bootstrap) → test-connection → failing playbook
#   (stop-on-failure asserted) → green playbook → real apt-get update run
#   → playbook_failure webhook dispatch
#
# Requirements: docker + compose, python3, port 8080 free.
# Used by the CI "integration" job and runnable locally: ./scripts/e2e.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

export COMPOSE_PROJECT_NAME=uau-e2e
ENV_FILE=".env.e2e"
COMPOSE=(docker compose --env-file "$ENV_FILE" -f docker-compose.yml)
TARGET=uau-e2e-target
API=http://localhost:8080/api/v1
ADMIN_PW=e2e-admin-password-123

fail() { echo "❌ $*" >&2; echo "--- app logs ---"; "${COMPOSE[@]}" logs --tail=40 app || true; exit 1; }
jsonget() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }

teardown() {
    docker rm -f "$TARGET" >/dev/null 2>&1 || true
    "${COMPOSE[@]}" down -v >/dev/null 2>&1 || true
    rm -f "$ENV_FILE"
}
trap teardown EXIT

cat > "$ENV_FILE" <<EOF
ADMIN_USERNAME=admin
ADMIN_PASSWORD=$ADMIN_PW
ENROLLMENT_TOKEN=e2e-enrollment-token
POSTGRES_PASSWORD=e2e-db-password
EOF

echo "== boot stack (isolated project: $COMPOSE_PROJECT_NAME) =="
"${COMPOSE[@]}" up -d --build

echo "== build + start ssh target =="
docker build -q -t uau-ssh-target -f scripts/e2e-target.Dockerfile scripts >/dev/null
docker run -d --name "$TARGET" --network "${COMPOSE_PROJECT_NAME}_default" \
    --network-alias e2e-target uau-ssh-target >/dev/null

echo "== wait for API health =="
for i in $(seq 1 45); do
    curl -sf "$API/health" >/dev/null 2>&1 && break
    [ "$i" = 45 ] && fail "backend never became healthy"
    sleep 2
done

TOKEN=$(curl -sf -X POST "$API/login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PW\"}" | jsonget '["token"]')
AUTH="Authorization: Bearer $TOKEN"
echo "== logged in =="

echo "== enroll target via password bootstrap =="
HID=$(curl -sf -X POST "$API/hosts" -H "$AUTH" -H 'Content-Type: application/json' \
    -d '{"hostname":"e2e-target","ssh_user":"root","password":"e2e-test-pass"}' \
    | jsonget '["id"]') || fail "enrollment failed"

curl -sf -X POST "$API/hosts/$HID/test-connection" -H "$AUTH" -d '{}' \
    | jsonget '["ok"]' | grep -q True || fail "test-connection failed"
echo "== test-connection ok =="

# wait_run <group_id> <timeout_seconds>: waits for the single run in the
# group to leave 'running', then echoes the run JSON.
wait_run() {
    local group=$1 budget=$2 status
    for i in $(seq 1 "$budget"); do
        status=$(curl -sf "$API/runs?group_id=$group" -H "$AUTH" | jsonget '[0]["status"]')
        [ "$status" != "running" ] && break
        sleep 2
    done
    curl -sf "$API/runs?group_id=$group" -H "$AUTH"
}

echo "== failing playbook: stop-on-failure =="
PB_FAIL=$(curl -sf -X POST "$API/playbooks" -H "$AUTH" -H 'Content-Type: application/json' \
    -d '{"name":"e2e-fail-fast","steps":["echo one","false","echo three"],"use_sudo":true}' | jsonget '["id"]')
GROUP=$(curl -sf -X POST "$API/hosts/bulk/run-playbook" -H "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"host_ids\":[$HID],\"playbook_id\":$PB_FAIL}" | jsonget '["group_id"]')
wait_run "$GROUP" 30 | python3 -c '
import sys, json
r = json.load(sys.stdin)[0]
assert r["kind"] == "playbook" and r["status"] == "failed" and r["exit_code"] == 1, "terminal state wrong: %s/%s" % (r["status"], r["exit_code"])
assert "echo one" in r["output"] and "three" not in r["output"], "stop-on-failure broken"
print("   stop-on-failure OK")' || fail "failing-playbook assertions"

echo "== green playbook =="
PB_OK=$(curl -sf -X POST "$API/playbooks" -H "$AUTH" -H 'Content-Type: application/json' \
    -d '{"name":"e2e-green","steps":["echo hello","uname -a"],"use_sudo":true}' | jsonget '["id"]')
GROUP=$(curl -sf -X POST "$API/hosts/bulk/run-playbook" -H "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"host_ids\":[$HID],\"playbook_id\":$PB_OK}" | jsonget '["group_id"]')
wait_run "$GROUP" 30 | python3 -c '
import sys, json
r = json.load(sys.stdin)[0]
assert r["status"] == "succeeded" and r["exit_code"] == 0, "green playbook: %s" % r["status"]
print("   green playbook OK")' || fail "green-playbook assertions"

echo "== real apt-get update run =="
GROUP=$(curl -sf -X POST "$API/hosts/bulk/run-update" -H "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"host_ids\":[$HID]}" | jsonget '["group_id"]')
wait_run "$GROUP" 150 | python3 -c '
import sys, json
r = json.load(sys.stdin)[0]
assert r["kind"] == "update" and r["status"] == "succeeded", "apt run: %s: %s" % (r["status"], r["error"])
print("   apt update run OK")' || fail "apt-run assertions"

echo "== security-only update (unattended-upgrade) =="
GROUP=$(curl -sf -X POST "$API/hosts/bulk/run-update" -H "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"host_ids\":[$HID],\"security_only\":true}" | jsonget '["group_id"]')
wait_run "$GROUP" 150 | python3 -c '
import sys, json
r = json.load(sys.stdin)[0]
assert r["status"] == "succeeded", "security run: %s: %s" % (r["status"], r["error"])
assert "security-only update" in r["output"], "security banner missing"
print("   security-only update OK")' || fail "security-only assertions"

echo "== API token (PAT) auth =="
SECRET=$(curl -sf -X POST "$API/tokens" -H "$AUTH" -H 'Content-Type: application/json' \
    -d '{"name":"e2e-ci","role":"viewer"}' | jsonget '["secret"]')
case "$SECRET" in uat_*) ;; *) fail "token secret missing uat_ prefix: $SECRET";; esac
curl -sf "$API/hosts" -H "Authorization: Bearer $SECRET" >/dev/null || fail "PAT rejected on viewer endpoint"
TOK_ID=$(curl -sf "$API/tokens" -H "$AUTH" | jsonget '[0]["id"]')
curl -sf -X DELETE "$API/tokens/$TOK_ID" -H "$AUTH" || fail "token revoke failed"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$API/hosts" -H "Authorization: Bearer $SECRET")
[ "$STATUS" = "401" ] || fail "revoked PAT still accepted (HTTP $STATUS)"
echo "   PAT mint/use/revoke OK"

echo "== playbook_failure webhook dispatch =="
curl -sf -X POST "$API/webhooks" -H "$AUTH" -H 'Content-Type: application/json' \
    -d '{"url":"http://e2e-sink.invalid/hook","event":"playbook_failure"}' >/dev/null
GROUP=$(curl -sf -X POST "$API/hosts/bulk/run-playbook" -H "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"host_ids\":[$HID],\"playbook_id\":$PB_FAIL}" | jsonget '["group_id"]')
wait_run "$GROUP" 30 >/dev/null
sleep 5
# Plain grep (not -q): -q exits at first match and SIGPIPEs the producer,
# which pipefail would misread as a failure.
"${COMPOSE[@]}" logs app 2>/dev/null | grep "webhook to http://e2e-sink.invalid/hook" >/dev/null \
    || fail "playbook_failure webhook never dispatched"
echo "   webhook dispatch OK"

echo
echo "✨ e2e PASSED"
