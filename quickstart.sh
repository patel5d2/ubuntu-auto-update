#!/usr/bin/env bash
# quickstart.sh — bring up Ubuntu Auto-Update with sensible defaults.
#
# Usage:
#   ./quickstart.sh             # generates .env if missing, brings everything up
#   ./quickstart.sh down        # stops and removes containers (keeps DB volume)
#   ./quickstart.sh reset       # stops AND removes the DB volume
#   ./quickstart.sh logs        # tails compose logs
#   ./quickstart.sh status      # shows compose ps + health
#
# Designed so a fresh checkout becomes a running stack in one command. We
# avoid heroics: if anything looks risky (existing .env, running containers
# that we don't own) we bail and ask, rather than overwriting state.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$REPO_ROOT/.env"
ENV_EXAMPLE="$REPO_ROOT/.env.example"

color() { printf '\033[%sm%s\033[0m\n' "$1" "$2"; }
info()  { color "0;34" "[info]  $*"; }
ok()    { color "0;32" "[ok]    $*"; }
warn()  { color "1;33" "[warn]  $*"; }
fail()  { color "0;31" "[fail]  $*"; exit 1; }

# pick_compose finds whichever invocation works; v2 is `docker compose`,
# legacy v1 is `docker-compose`. We stop early if neither is available so the
# user gets one clear message instead of three confusing ones.
pick_compose() {
    if docker compose version >/dev/null 2>&1; then
        echo "docker compose"
    elif command -v docker-compose >/dev/null 2>&1; then
        echo "docker-compose"
    else
        fail "Neither 'docker compose' nor 'docker-compose' is available. Install Docker Desktop or the docker-compose plugin first."
    fi
}

gen_secret() {
    # 32 bytes of base64, stripped of slashes/pluses so it's easy to paste
    # into shell + URL contexts. openssl is on every reasonable host.
    openssl rand -base64 32 | tr -d '=+/' | cut -c1-32
}

ensure_env() {
    if [[ -f "$ENV_FILE" ]]; then
        info ".env already exists — leaving it alone."
        return
    fi
    if [[ ! -f "$ENV_EXAMPLE" ]]; then
        fail ".env.example missing — repo looks broken."
    fi

    info "Generating .env with random secrets…"
    local admin_pw enroll_token db_pw
    admin_pw=$(gen_secret)
    enroll_token=$(gen_secret)
    db_pw=$(gen_secret)

    # sed -i is portable-hostile; use a tmp file and rename.
    local tmp; tmp=$(mktemp)
    awk \
        -v admin_pw="$admin_pw" \
        -v enroll_token="$enroll_token" \
        -v db_pw="$db_pw" \
        '
        /^ADMIN_PASSWORD=/    { print "ADMIN_PASSWORD=" admin_pw; next }
        /^ENROLLMENT_TOKEN=/  { print "ENROLLMENT_TOKEN=" enroll_token; next }
        /^POSTGRES_PASSWORD=/ { print "POSTGRES_PASSWORD=" db_pw; next }
        { print }
        ' "$ENV_EXAMPLE" > "$tmp"
    mv "$tmp" "$ENV_FILE"
    chmod 600 "$ENV_FILE"

    ok "Wrote $ENV_FILE (chmod 600). Capture these now if you need them elsewhere:"
    echo "    ADMIN_USERNAME=admin"
    echo "    ADMIN_PASSWORD=$admin_pw"
    echo "    ENROLLMENT_TOKEN=$enroll_token"
}

cmd_up() {
    ensure_env
    local compose; compose=$(pick_compose)
    info "Bringing up the stack with: $compose up -d --build"
    # shellcheck disable=SC2086
    $compose -f "$REPO_ROOT/docker-compose.yml" up -d --build
    ok "Stack is starting. Health checks below — give it ~10 s on first boot."
    sleep 2
    cmd_status
    cat <<EOF

  Web UI:  http://localhost:5173
  API:     http://localhost:8080/api/v1/health

  Login with the credentials in $ENV_FILE.
  Tail logs:    ./quickstart.sh logs
  Tear down:    ./quickstart.sh down

EOF
}

cmd_down() {
    local compose; compose=$(pick_compose)
    info "Stopping containers (DB volume kept)…"
    # shellcheck disable=SC2086
    $compose -f "$REPO_ROOT/docker-compose.yml" down
    ok "Stopped."
}

cmd_reset() {
    warn "This deletes the Postgres volume — every host record, run, and SSH key goes with it."
    read -r -p "Type 'reset' to confirm: " confirm
    [[ "$confirm" == "reset" ]] || fail "Aborted."
    local compose; compose=$(pick_compose)
    # shellcheck disable=SC2086
    $compose -f "$REPO_ROOT/docker-compose.yml" down -v
    ok "Stack and DB volume removed."
}

cmd_logs() {
    local compose; compose=$(pick_compose)
    # shellcheck disable=SC2086
    $compose -f "$REPO_ROOT/docker-compose.yml" logs -f --tail=100
}

cmd_status() {
    local compose; compose=$(pick_compose)
    # shellcheck disable=SC2086
    $compose -f "$REPO_ROOT/docker-compose.yml" ps
}

case "${1:-up}" in
    up)     cmd_up ;;
    down)   cmd_down ;;
    reset)  cmd_reset ;;
    logs)   cmd_logs ;;
    status) cmd_status ;;
    -h|--help|help)
        sed -n '2,12p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
        ;;
    *) fail "Unknown command: $1 (try ./quickstart.sh help)" ;;
esac
