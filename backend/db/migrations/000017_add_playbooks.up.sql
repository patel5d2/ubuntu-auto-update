-- Playbooks: named, ordered lists of shell steps run over SSH one-by-one
-- (stop-on-failure), reusing the existing run engine + bulk coordinator.

CREATE TABLE IF NOT EXISTS playbooks (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    steps       TEXT[] NOT NULL,
    use_sudo    BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Widen the run-kind CHECK. 000010 created it inline as
-- CHECK (kind IN ('preview','update')); an inline column CHECK gets the default
-- name <table>_<column>_check = update_runs_kind_check. DROP IF EXISTS keeps this
-- safe to re-run.
ALTER TABLE update_runs DROP CONSTRAINT IF EXISTS update_runs_kind_check;
ALTER TABLE update_runs ADD CONSTRAINT update_runs_kind_check
    CHECK (kind IN ('preview', 'update', 'playbook'));

-- History link: which playbook a run came from. SET NULL keeps old runs readable
-- after a playbook is deleted.
ALTER TABLE update_runs ADD COLUMN IF NOT EXISTS playbook_id INTEGER
    REFERENCES playbooks(id) ON DELETE SET NULL;

-- Schedule target: NULL = apt-update schedule (today's behavior). RESTRICT so
-- deleting a playbook a schedule depends on fails loudly instead of silently
-- turning the schedule into an apt run.
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS playbook_id INTEGER
    REFERENCES playbooks(id) ON DELETE RESTRICT;

-- Subscribable webhook events are CHECK-constrained (000009 named it
-- check_webhook_event_valid). Widen it for playbook run outcomes.
ALTER TABLE webhooks DROP CONSTRAINT IF EXISTS check_webhook_event_valid;
ALTER TABLE webhooks ADD CONSTRAINT check_webhook_event_valid
    CHECK (event IN ('update_success', 'update_failure', 'host_registered',
                     'host_offline', 'preview_success',
                     'playbook_success', 'playbook_failure'));
