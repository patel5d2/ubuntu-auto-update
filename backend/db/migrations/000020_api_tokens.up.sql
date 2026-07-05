-- Operator API tokens (PATs): admin-minted, role-scoped, hash-only at rest.
-- The raw token (uat_…) is returned exactly once at creation.
CREATE TABLE IF NOT EXISTS api_tokens (
    id           SERIAL PRIMARY KEY,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    role         TEXT NOT NULL CHECK (role IN ('viewer', 'operator', 'admin')),
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);
