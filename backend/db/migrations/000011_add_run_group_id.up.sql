-- run_group_id ties a fan-out bulk update together so the UI can report
-- per-host progress under one logical run. NULL for single-host runs.
ALTER TABLE update_runs ADD COLUMN IF NOT EXISTS run_group_id UUID;
CREATE INDEX IF NOT EXISTS idx_update_runs_group_id ON update_runs(run_group_id);
