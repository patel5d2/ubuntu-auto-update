-- Per-user accounts, DB-backed sessions, audit log, persistent host keys.
--
-- Why all four in one migration: they share an admin-onboarding rollout. The
-- backend bootstraps a default admin row from ADMIN_USERNAME/ADMIN_PASSWORD on
-- startup if `users` is empty, so deploying these tables cannot brick an
-- existing operator out of the UI.

-- ---------------------------------------------------------------------------
-- users: replaces the single ADMIN_USERNAME/ADMIN_PASSWORD env pair.
-- Passwords are stored as bcrypt hashes (golang.org/x/crypto/bcrypt).
-- Roles: 'viewer' (read-only), 'operator' (run updates), 'admin' (manage all).
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id              SERIAL PRIMARY KEY,
    username        TEXT        NOT NULL UNIQUE,
    password_hash   TEXT        NOT NULL,
    role            TEXT        NOT NULL DEFAULT 'viewer'
                    CHECK (role IN ('viewer', 'operator', 'admin')),
    disabled_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at   TIMESTAMPTZ,
    failed_logins   INTEGER     NOT NULL DEFAULT 0,
    locked_until    TIMESTAMPTZ
);

CREATE INDEX idx_users_username ON users (username);

-- ---------------------------------------------------------------------------
-- sessions: replaces middleware/auth.go's in-memory token map. The backend
-- stores only the token's hex SHA-256 digest, so a database leak does not
-- yield live tokens. Tokens themselves are 64-char hex random strings, never
-- stored in plaintext.
-- ---------------------------------------------------------------------------
CREATE TABLE sessions (
    id              SERIAL PRIMARY KEY,
    token_hash      TEXT        NOT NULL UNIQUE,
    user_id         INTEGER     REFERENCES users(id) ON DELETE CASCADE,
    -- agent_label is set when the session is an agent enrollment token instead
    -- of a logged-in human; users.id is null in that case. We carry the agent
    -- hostname so /api/v1/report can attribute its writes.
    agent_label     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip              TEXT,
    user_agent      TEXT,
    -- Exactly one of (user_id, agent_label) must be populated.
    CHECK ((user_id IS NULL) <> (agent_label IS NULL))
);

CREATE INDEX idx_sessions_token_hash ON sessions (token_hash);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- ---------------------------------------------------------------------------
-- audit_log: every state-changing action is recorded so we can answer "who
-- triggered the upgrade on prod-web-01 last Thursday?" without grepping logs.
-- Bound is intentionally loose: we keep up to one row per write op.
-- ---------------------------------------------------------------------------
CREATE TABLE audit_log (
    id              BIGSERIAL PRIMARY KEY,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_user_id   INTEGER     REFERENCES users(id) ON DELETE SET NULL,
    actor_label     TEXT,        -- e.g. 'admin', 'agent:host01' — durable copy
    action          TEXT        NOT NULL,
    target_type     TEXT,        -- 'host', 'user', 'session', 'webhook', 'run'
    target_id       TEXT,
    request_id      TEXT,
    ip              TEXT,
    user_agent      TEXT,
    details         JSONB        NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_audit_log_occurred_at ON audit_log (occurred_at DESC);
CREATE INDEX idx_audit_log_action ON audit_log (action);
CREATE INDEX idx_audit_log_target ON audit_log (target_type, target_id);

-- ---------------------------------------------------------------------------
-- host_keys: replaces the on-disk known_hosts file. With known_hosts in the
-- DB any backend replica sees the same host fingerprints, and bootstrap's
-- TOFU capture immediately becomes visible to every other backend.
-- ---------------------------------------------------------------------------
CREATE TABLE host_keys (
    id              SERIAL PRIMARY KEY,
    hostname        TEXT        NOT NULL,
    -- Marshalled SSH public key in OpenSSH wire format (algo + base64).
    key_line        TEXT        NOT NULL,
    fingerprint_sha256 TEXT     NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (hostname, fingerprint_sha256)
);

CREATE INDEX idx_host_keys_hostname ON host_keys (hostname);
