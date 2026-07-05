-- Rollout knobs for scheduled runs. Zero = coordinator defaults, i.e. exactly
-- today's behavior; the columns just carry the same options the bulk API
-- already accepts per-request.
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS concurrency INTEGER NOT NULL DEFAULT 0;
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS canary_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS canary_wait_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS abort_on_failure_pct INTEGER NOT NULL DEFAULT 0;

-- Maintenance window, minutes since midnight UTC. NULL start = no window
-- (fire whenever due — today's behavior). A window that wraps midnight
-- (start > end, e.g. 22:00–02:00) belongs to the start day.
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS window_start_minute INTEGER;
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS window_end_minute INTEGER;

-- Days bitmask for the window start day: bit 0 = Sunday … bit 6 = Saturday.
-- 127 = every day.
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS window_days SMALLINT NOT NULL DEFAULT 127;

-- Security-only mode for apt schedules: unattended-upgrade instead of a
-- blanket apt-get upgrade. Meaningless (and rejected) for playbook schedules.
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS security_only BOOLEAN NOT NULL DEFAULT FALSE;

-- Reboot runs join the run-kind and webhook-event contracts.
ALTER TABLE update_runs DROP CONSTRAINT IF EXISTS update_runs_kind_check;
ALTER TABLE update_runs ADD CONSTRAINT update_runs_kind_check
    CHECK (kind IN ('preview', 'update', 'playbook', 'reboot'));

ALTER TABLE webhooks DROP CONSTRAINT IF EXISTS check_webhook_event_valid;
ALTER TABLE webhooks ADD CONSTRAINT check_webhook_event_valid
    CHECK (event IN ('update_success', 'update_failure', 'host_registered',
                     'host_offline', 'preview_success',
                     'playbook_success', 'playbook_failure',
                     'reboot_success', 'reboot_failure'));
