-- Host tags (flat labels, filterable in the UI) and server-side schedules.
--
-- ponytail: host_ids is an INTEGER[] snapshot, not a join table — a schedule
-- referencing a deleted host just skips it at fire time. Add a join table if
-- schedules ever need per-host state.
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS schedules (
    id               SERIAL PRIMARY KEY,
    name             TEXT NOT NULL,
    host_ids         INTEGER[] NOT NULL,
    interval_minutes INTEGER NOT NULL CHECK (interval_minutes >= 5),
    next_run_at      TIMESTAMPTZ NOT NULL,
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_schedules_due
    ON schedules(next_run_at) WHERE enabled;
