-- update_runs persists each update attempt so operators can see history
-- without keeping a tab open on the live WebSocket. The output column is
-- truncated by the application at 1 MiB.
CREATE TABLE IF NOT EXISTS update_runs (
    id            SERIAL PRIMARY KEY,
    host_id       INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    triggered_by  TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('preview', 'update')),
    status        TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled')),
    exit_code     INTEGER,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ,
    output        TEXT NOT NULL DEFAULT '',
    error         TEXT
);

CREATE INDEX IF NOT EXISTS idx_update_runs_host_id_started
    ON update_runs(host_id, started_at DESC);
