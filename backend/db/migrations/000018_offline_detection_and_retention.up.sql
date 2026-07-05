-- Server-side offline detection: NULL = online (or never evaluated), set when
-- last_seen crosses the offline threshold. Cleared when the host reports again.
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS offline_since TIMESTAMPTZ;

-- Run retention prunes by age; give the DELETE an index to walk.
CREATE INDEX IF NOT EXISTS idx_update_runs_started_at ON update_runs (started_at);
